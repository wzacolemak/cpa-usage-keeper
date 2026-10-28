package cpa

import (
	"context"
	"encoding/json"
	"fmt"
)

// FetchZcodeSummary 读取 CPA zcode 插件的用量摘要原始 JSON。
//
// 该端点是 CPA 自己的路由（插件通过管理密钥转发到 zcode 上游），
// 不能用 api-call 代理，所以这里直接带管理密钥请求。
func (c *Client) FetchZcodeSummary(ctx context.Context) (json.RawMessage, error) {
	var raw json.RawMessage
	if _, _, err := c.doManagementJSONRequest(ctx, cpaZcodeSummaryEndpoint, &raw, "zcode summary"); err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("zcode summary response is empty")
	}
	return raw, nil
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
