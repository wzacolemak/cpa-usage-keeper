package quota

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"cpa-usage-keeper/internal/timeutil"
)

// opencodeGoUsageEndpoint 返回三个滚动窗口的已用百分比与重置时间。
//
// 依据 https://opencode.ai/v2/docs/console/go 「Usage limits」：
// 限额按月度美元金额定义，5 小时窗口占月额度 20%、每周占 50%、每月占 100%。
// 响应形如：
//
//	{"usage":{"rolling":{"status":"ok","percent":1,"resetsAt":"..."},
//	          "weekly":{...},"monthly":{...}}}
type OpencodeGoUsagePayload struct {
	Usage map[string]OpencodeGoUsageWindow `json:"usage"`
}

type OpencodeGoUsageWindow struct {
	Status   string  `json:"status,omitempty"`
	Percent  float64 `json:"percent"`
	ResetsAt string  `json:"resetsAt,omitempty"`
}

// OpencodeGoResult 是 OpenCode Go 订阅的额度结果。
type OpencodeGoResult struct {
	Usage *OpencodeGoUsagePayload `json:"usage,omitempty"`
}

// opencodeGoUsageOrder 固定窗口展示顺序：5 小时 → 每周 → 每月。
func opencodeGoUsageOrder(window string) int {
	switch window {
	case "rolling":
		return 0
	case "weekly":
		return 1
	case "monthly":
		return 2
	default:
		return 3
	}
}

// opencodeGoUsageLabel 是窗口的展示名，沿用 CPA 面板的既有叫法。
func opencodeGoUsageLabel(window string) string {
	switch window {
	case "rolling":
		return "5h"
	case "weekly":
		return "Weekly"
	case "monthly":
		return "Monthly"
	default:
		return window
	}
}

// opencodeGoUsageWindow 把窗口映射成统一的 QuotaWindow 秒数，供前端算窗口用量。
func opencodeGoUsageWindow(window string) *QuotaWindow {
	switch window {
	case "rolling":
		return &QuotaWindow{Seconds: intPtr(quotaWindowFiveHourSeconds)}
	case "weekly":
		return &QuotaWindow{Seconds: intPtr(quotaWindowSevenDaySeconds)}
	case "monthly":
		return &QuotaWindow{Seconds: intPtr(quotaWindowThirtyDaySeconds)}
	default:
		return nil
	}
}

// FetchOpencodeGoUsage 直连 OpenCode Go 的 usage 端点。
//
// 不走 CPA：该订阅是 CPA config 里的 API key 渠道，不在 auth-files 清单里，
// 所以 CPA 的通用额度接口拿不到它，只能由 Keeper 用同一把 key 直查。
//
// fetchOpencodeGoUsageWithRetry 对瞬时网络抖动（TLS 握手超时、连接重置等）
// 做有限重试。上游 opencode.ai 在校园网出口下 RTT 偏高（TLS 完整握手 ~2.5s），
// 偶发抖动不应直接把错误顶到面板。
func FetchOpencodeGoUsageWithRetry(ctx context.Context, endpoint, apiKey string) (*OpencodeGoUsagePayload, error) {
	const attempts = 3
	const perAttemptTimeout = 15 * time.Second
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		payload, err := FetchOpencodeGoUsage(ctx, endpoint, apiKey, perAttemptTimeout)
		if err == nil {
			return payload, nil
		}
		lastErr = err
		// 只有网络层瞬时错误才重试；HTTP 4xx/5xx、解码错误重试无意义
		if !isTransientUsageError(err) || attempt == attempts {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt) * 500 * time.Millisecond):
		}
	}
	return nil, lastErr
}

func isTransientUsageError(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true // 超时、临时性网络错误
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true // DNS 抖动
	}
	msg := err.Error()
	for _, marker := range []string{
		"connection reset",
		"connection refused",
		"broken pipe",
		"EOF",
		"tls:",
		"handshake",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}
func FetchOpencodeGoUsage(ctx context.Context, endpoint string, apiKey string, timeout time.Duration) (*OpencodeGoUsagePayload, error) {
	endpoint = strings.TrimSpace(endpoint)
	apiKey = strings.TrimSpace(apiKey)
	if endpoint == "" {
		return nil, fmt.Errorf("opencode go usage url is required")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("opencode go api key is required")
	}
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build opencode go usage request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request opencode go usage: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read opencode go usage response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// 不回显响应体，避免把 key 或内部信息带进错误信息。
		return nil, fmt.Errorf("opencode go usage returned HTTP %d", resp.StatusCode)
	}
	var payload OpencodeGoUsagePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode opencode go usage response: %w", err)
	}
	if len(payload.Usage) == 0 {
		return nil, fmt.Errorf("opencode go usage response is empty")
	}
	return &payload, nil
}

// normalizeOpencodeGoQuotaRows 把 usage 的三个窗口折算成统一 quota row。
//
// percent 是「已用百分比」，与 keeper 的 usedPercent 同义，进度条会自动按剩余展示。
func normalizeOpencodeGoQuotaRows(result OpencodeGoResult) []QuotaRow {
	if result.Usage == nil || len(result.Usage.Usage) == 0 {
		return nil
	}
	windows := make([]string, 0, len(result.Usage.Usage))
	for window := range result.Usage.Usage {
		windows = append(windows, window)
	}
	sort.SliceStable(windows, func(i, j int) bool {
		return opencodeGoUsageOrder(windows[i]) < opencodeGoUsageOrder(windows[j])
	})

	rows := make([]QuotaRow, 0, len(windows))
	for _, window := range windows {
		entry := result.Usage.Usage[window]
		row := QuotaRow{
			Key:          "opencode-go." + window,
			Label:        opencodeGoUsageLabel(window),
			Scope:        "window",
			Metric:       window,
			UsedPercent:  floatPtr(entry.Percent),
			LimitReached: boolPtr(entry.Percent >= 100),
			Allowed:      boolPtr(entry.Percent < 100),
			// 上游给的是 RFC3339；统一转成项目时区的存储格式，前端才能正确算倒计时。
			ResetAt: opencodeGoResetAtDisplay(entry.ResetsAt),
			Window:  opencodeGoUsageWindow(window),
		}
		// 上游若报告非 ok 状态，前端应能看到该窗口不可用而不是显示成 0%。
		if status := strings.TrimSpace(entry.Status); status != "" && !strings.EqualFold(status, "ok") {
			row.LimitReached = boolPtr(true)
			row.Allowed = boolPtr(false)
		}
		rows = append(rows, row)
	}
	return rows
}

// opencodeGoResetAtDisplay 把上游 RFC3339 重置时间转成项目时区的展示格式。
func opencodeGoResetAtDisplay(value string) string {
	parsed, err := timeutil.ParseStorageTime(value)
	if err != nil {
		return value
	}
	return timeutil.FormatStorageTime(parsed)
}
