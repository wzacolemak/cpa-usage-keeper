package cpa

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// zcodeSummaryEndpointCacheTTL 是插件 ID 发现结果的缓存时长。
//
// 插件 ID 只有在运维改插件文件名时才变化，缓存可以让每次额度刷新只发 1 个请求，
// 同时在改名后最多 1 分钟自动恢复，不需要重启 keeper。
const zcodeSummaryEndpointCacheTTL = time.Minute

// cpaPluginDescriptor 只保留插件 ID 发现需要的字段。
type cpaPluginDescriptor struct {
	ID               string `json:"id"`
	OauthProvider    string `json:"oauth_provider"`
	Registered       bool   `json:"registered"`
	Enabled          bool   `json:"enabled"`
	EffectiveEnabled bool   `json:"effective_enabled"`
}

type cpaPluginListResponse struct {
	Plugins []cpaPluginDescriptor `json:"plugins"`
}

// FetchZcodeSummary 读取 CPA zcode 插件的用量摘要原始 JSON。
//
// 该端点是 CPA 自己的路由（插件通过管理密钥转发到 zcode 上游），
// 不能用 api-call 代理，所以这里直接带管理密钥请求。
//
// 插件的 resource 路由形如 /v0/resource/plugins/<插件ID>/<resource>,
// 其中 <插件ID> 就是插件文件名，历史上从 zcode 改成了 zcode-oauth。
// 因此这里不写死 ID，而是先发现再请求，并对发现结果做短期缓存。
func (c *Client) FetchZcodeSummary(ctx context.Context) (json.RawMessage, error) {
	var lastErr error
	// 第一轮：用上次成功的端点，正常情况下只有 1 个请求。
	if cached := c.cachedZcodeSummaryEndpoint(); cached != "" {
		raw, err := c.fetchZcodeSummaryAt(ctx, cached)
		if err == nil {
			return raw, nil
		}
		// 插件又改名或暂时不可用：丢弃缓存，本轮立即重新发现。
		lastErr = err
		c.invalidateZcodeSummaryEndpoint()
	}
	// 第二轮：发现当前插件 ID，最后回退到历史端点。
	for _, endpoint := range c.zcodeSummaryEndpointCandidates(ctx) {
		raw, err := c.fetchZcodeSummaryAt(ctx, endpoint)
		if err != nil {
			lastErr = err
			continue
		}
		c.cacheZcodeSummaryEndpoint(endpoint)
		return raw, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no zcode plugin endpoint is available")
	}
	return nil, lastErr
}

// fetchZcodeSummaryAt 请求单个候选端点。
func (c *Client) fetchZcodeSummaryAt(ctx context.Context, endpoint string) (json.RawMessage, error) {
	var raw json.RawMessage
	if _, _, err := c.doManagementJSONRequest(ctx, endpoint, &raw, "zcode summary"); err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("zcode summary response is empty (endpoint %s)", endpoint)
	}
	return raw, nil
}

// zcodeSummaryEndpointCandidates 返回发现出的候选端点（不含缓存项）。
//
// 顺序为：当前已注册插件发现的端点、历史兜底常量。
func (c *Client) zcodeSummaryEndpointCandidates(ctx context.Context) []string {
	candidates := make([]string, 0, 4)
	seen := make(map[string]struct{}, 4)
	appendCandidate := func(endpoint string) {
		endpoint = strings.TrimSpace(endpoint)
		if endpoint == "" {
			return
		}
		if _, ok := seen[endpoint]; ok {
			return
		}
		seen[endpoint] = struct{}{}
		candidates = append(candidates, endpoint)
	}

	for _, pluginID := range c.discoverZcodePluginIDs(ctx) {
		appendCandidate(zcodeSummaryEndpointFor(pluginID))
	}
	// 兜底：插件列表不可用（老版本 CPA 没有该接口）时仍按历史 ID 尝试。
	appendCandidate(cpaZcodeSummaryEndpoint)
	return candidates
}

// discoverZcodePluginIDs 从 CPA 插件列表里找出 zcode 相关插件 ID。
//
// 命中条件放宽为 ID 或 oauth_provider 含 "zcode"，这样插件再次改名也不会失效。
// 已启用且已注册的插件排在前面，避免先请求到被禁用的同名插件。
func (c *Client) discoverZcodePluginIDs(ctx context.Context) []string {
	var list cpaPluginListResponse
	if _, _, err := c.doManagementJSONRequest(ctx, cpaManagementPluginsEndpoint, &list, "plugins"); err != nil {
		return nil
	}
	type ranked struct {
		id      string
		enabled bool
	}
	matches := make([]ranked, 0, len(list.Plugins))
	for _, plugin := range list.Plugins {
		id := strings.TrimSpace(plugin.ID)
		if id == "" {
			continue
		}
		if !strings.Contains(strings.ToLower(id), "zcode") &&
			!strings.Contains(strings.ToLower(plugin.OauthProvider), "zcode") {
			continue
		}
		matches = append(matches, ranked{id: id, enabled: plugin.Enabled || plugin.EffectiveEnabled})
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].enabled != matches[j].enabled {
			return matches[i].enabled
		}
		return matches[i].id < matches[j].id
	})
	ids := make([]string, 0, len(matches))
	for _, match := range matches {
		ids = append(ids, match.id)
	}
	return ids
}

// zcodeSummaryEndpointFor 拼出指定插件 ID 的 summary 端点。
func zcodeSummaryEndpointFor(pluginID string) string {
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return ""
	}
	return "/v0/resource/plugins/" + pluginID + cpaZcodeSummaryResourceSuffix
}

// cachedZcodeSummaryEndpoint 返回仍在有效期内的发现结果。
func (c *Client) cachedZcodeSummaryEndpoint() string {
	if c == nil {
		return ""
	}
	c.zcodeEndpointMu.Lock()
	defer c.zcodeEndpointMu.Unlock()
	if c.zcodeSummaryEndpoint == "" || time.Now().After(c.zcodeEndpointExpiry) {
		return ""
	}
	return c.zcodeSummaryEndpoint
}

// cacheZcodeSummaryEndpoint 记录一次成功的端点发现结果。
func (c *Client) cacheZcodeSummaryEndpoint(endpoint string) {
	if c == nil {
		return
	}
	c.zcodeEndpointMu.Lock()
	defer c.zcodeEndpointMu.Unlock()
	c.zcodeSummaryEndpoint = endpoint
	c.zcodeEndpointExpiry = time.Now().Add(zcodeSummaryEndpointCacheTTL)
}

// invalidateZcodeSummaryEndpoint 丢弃缓存，下次请求重新发现插件 ID。
func (c *Client) invalidateZcodeSummaryEndpoint() {
	if c == nil {
		return
	}
	c.zcodeEndpointMu.Lock()
	defer c.zcodeEndpointMu.Unlock()
	c.zcodeSummaryEndpoint = ""
	c.zcodeEndpointExpiry = time.Time{}
}

// ResetZcodeWindow 消耗一张重置卡重置指定窗口 (five_hour|week)。
//
// 走 zcode 插件的管理 POST 路由（宿主对 resource 路由只转发 GET，POST 会 404）。
func (c *Client) ResetZcodeWindow(ctx context.Context, windowKey string) error {
	var out struct {
		Ok      bool   `json:"ok"`
		Message string `json:"message"`
	}
	body := map[string]string{"type": windowKey}
	status, _, err := c.doManagementJSONPostRequest(ctx, cpaManagementZcodeResetEndpoint, body, &out, "zcode reset")
	if err != nil {
		if out.Message != "" {
			return fmt.Errorf("zcode reset failed: %s", out.Message)
		}
		return err
	}
	if status >= 200 && status < 300 && out.Ok {
		return nil
	}
	if out.Message != "" {
		return fmt.Errorf("zcode reset failed: %s", out.Message)
	}
	return fmt.Errorf("zcode reset failed with HTTP %d", status)
}
