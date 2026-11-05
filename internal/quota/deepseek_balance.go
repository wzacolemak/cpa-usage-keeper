package quota

// DeepSeek 账户余额查询。
//
// DeepSeek 官方提供 GET /user/balance（无用量/账单 API），返回各币种余额。
// 依据 https://api-docs.deepseek.com/api/get-user-balance：
//
//	{"is_available":true,
//	 "balance_infos":[{"currency":"CNY","total_balance":"12.52",
//	                   "granted_balance":"0.00","topped_up_balance":"12.52"}]}
//
// total_balance = granted_balance(赠送) + topped_up_balance(充值)。

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DeepSeekBalancePayload 是官方余额接口的原始响应。
type DeepSeekBalancePayload struct {
	IsAvailable  bool                     `json:"is_available"`
	BalanceInfos []DeepSeekBalanceAccount `json:"balance_infos"`
}

// DeepSeekBalanceAccount 是单个币种的余额明细。
type DeepSeekBalanceAccount struct {
	Currency        string `json:"currency"`
	TotalBalance    string `json:"total_balance"`
	GrantedBalance  string `json:"granted_balance"`
	ToppedUpBalance string `json:"topped_up_balance"`
}

// DeepSeekBalanceResult 是 /quota/deepseek-balance 的返回体。
type DeepSeekBalanceResult struct {
	Available bool                     `json:"available"`
	Balances  []DeepSeekBalanceAccount `json:"balances,omitempty"`
	// LowBalance 为 true 表示任一币种余额低于阈值，前端据此高亮。
	LowBalance bool `json:"low_balance"`
	// Threshold 回显当前阈值（与币种同单位），便于前端展示。
	Threshold float64 `json:"threshold"`
}

// defaultDeepSeekLowBalanceThreshold 是余额告警阈值（与币种同单位）。
const defaultDeepSeekLowBalanceThreshold = 5

// FetchDeepSeekBalance 直连官方 /user/balance。
func FetchDeepSeekBalance(ctx context.Context, endpoint string, apiKey string, timeout time.Duration) (*DeepSeekBalancePayload, error) {
	endpoint = strings.TrimSpace(endpoint)
	apiKey = strings.TrimSpace(apiKey)
	if endpoint == "" {
		return nil, fmt.Errorf("deepseek balance url is required")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("deepseek api key is required")
	}
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build deepseek balance request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request deepseek balance: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read deepseek balance response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// 不回显响应体，避免把 key 或内部信息带进错误信息。
		return nil, fmt.Errorf("deepseek balance returned HTTP %d", resp.StatusCode)
	}
	var payload DeepSeekBalancePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode deepseek balance response: %w", err)
	}
	return &payload, nil
}

// FetchDeepSeekBalanceWithRetry 对瞬时网络抖动做有限重试。
// DeepSeek 官方接口在国内直连稳定，但仍复用与 OpenCode Go 一致的退避策略，
// 避免一次抖动就把错误顶到面板。
func FetchDeepSeekBalanceWithRetry(ctx context.Context, endpoint, apiKey string) (*DeepSeekBalanceResult, error) {
	const attempts = 3
	const perAttemptTimeout = 15 * time.Second
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		payload, err := FetchDeepSeekBalance(ctx, endpoint, apiKey, perAttemptTimeout)
		if err == nil {
			return buildDeepSeekBalanceResult(payload), nil
		}
		lastErr = err
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

func buildDeepSeekBalanceResult(payload *DeepSeekBalancePayload) *DeepSeekBalanceResult {
	result := &DeepSeekBalanceResult{
		Threshold: defaultDeepSeekLowBalanceThreshold,
	}
	if payload == nil {
		return result
	}
	result.Available = payload.IsAvailable
	result.Balances = payload.BalanceInfos
	for _, account := range payload.BalanceInfos {
		if total, ok := parseDeepSeekAmount(account.TotalBalance); ok && total < defaultDeepSeekLowBalanceThreshold {
			result.LowBalance = true
		}
	}
	return result
}

func parseDeepSeekAmount(raw string) (float64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	var value float64
	if _, err := fmt.Sscanf(raw, "%f", &value); err != nil {
		return 0, false
	}
	return value, true
}
