package quota

import (
	"context"
	"encoding/json"
	"fmt"
)

// zcodeSummaryFetcher 由 cpa.Client 实现：直接请求 CPA 自身的插件端点。
//
// 不能用 ManagementAPICaller.CallManagementAPI 读这个端点：api-call 要求传入
// 完整的上游 URL，而 /v0/resource/plugins/zcode/summary 是 CPA 自己的路由，
// 由 CPA 带凭证转发到 zcode 插件。
type zcodeSummaryFetcher interface {
	FetchZcodeSummary(ctx context.Context) (json.RawMessage, error)
}

// zcodeProvider 读取 CPA zcode 插件暴露的 Coding Plan 用量摘要。
//
// 与其它 provider 不同，zcode 的额度不是 CPA 管理 API 的通用 quota 字段，
// 而是插件自己聚合的 summary 响应；因此这里直接请求插件端点，并把 windows
// 折算成统一的 QuotaRow。
type zcodeProvider struct {
	caller ManagementAPICaller
}

func NewZcodeProvider(caller ManagementAPICaller) ProviderHandler {
	return zcodeProvider{caller: caller}
}

func (p zcodeProvider) Check(ctx context.Context, input ProviderInput) (ProviderOutput, error) {
	fetcher, ok := p.caller.(zcodeSummaryFetcher)
	if !ok {
		return ProviderOutput{}, fmt.Errorf("zcode summary reader is unavailable")
	}
	raw, err := fetcher.FetchZcodeSummary(ctx)
	if err != nil {
		return ProviderOutput{}, err
	}
	summary, err := parseZcodeSummaryPayload(raw)
	if err != nil {
		return ProviderOutput{}, err
	}
	if !summary.LoggedIn {
		return ProviderOutput{}, fmt.Errorf("zcode credential is not logged in")
	}
	return ProviderOutput{Provider: "zcode", Result: ZcodeResult{Summary: summary}}, nil
}

// parseZcodeSummaryPayload 把插件 summary 响应折算成 ZcodeSummaryPayload。
func parseZcodeSummaryPayload(raw json.RawMessage) (*ZcodeSummaryPayload, error) {
	object := rawObject(raw)
	if object == nil {
		return nil, fmt.Errorf("parse zcode summary response")
	}
	// 兼容 CPA 可能把插件响应再包一层 body 的情况。
	if _, ok := object["windows"]; !ok {
		if nested := objectField(object, "body"); nested != nil {
			object = nested
		}
	}
	payload := &ZcodeSummaryPayload{
		// 插件在未登录时才会返回 logged_in=false；缺失字段按已登录处理，避免误判。
		LoggedIn: boolFieldDefault(object, true, "logged_in", "loggedIn"),
		Account:  stringField(object, "account"),
		Level:    stringField(object, "level"),
	}
	for _, rawWindow := range arrayField(object, "windows") {
		windowObject := rawObject(rawWindow)
		if windowObject == nil {
			continue
		}
		// percentage 缺失说明该窗口没有额度信息，跳过而不是显示成 0%。
		percentage, ok := floatValue(windowObject, "percentage")
		if !ok {
			continue
		}
		window := ZcodeWindow{
			Key:         stringField(windowObject, "key"),
			Title:       stringField(windowObject, "title"),
			Description: stringField(windowObject, "desc", "description"),
			Unit:        stringField(windowObject, "unit"),
			Percentage:  percentage,
			CurrentText: stringField(windowObject, "current_text", "currentText"),
			ResetTime:   stringField(windowObject, "reset_time", "resetTime"),
		}
		for _, rawDetail := range arrayField(windowObject, "details") {
			detailObject := rawObject(rawDetail)
			if detailObject == nil {
				continue
			}
			window.Details = append(window.Details, ZcodeUsageItem{
				Name:  stringField(detailObject, "name"),
				Usage: stringField(detailObject, "usage"),
			})
		}
		// reset_cards: count=0 也会下发（count 缺失视为无卡），前端据此渲染置灰重置按钮。
		if rawCards := objectField(windowObject, "reset_cards"); rawCards != nil {
			if count, ok := floatValue(rawCards, "count"); ok {
				window.ResetCards = &ZcodeResetCards{Count: int(count), Earliest: stringField(rawCards, "earliest")}
			}
		}
		payload.Windows = append(payload.Windows, window)
	}
	return payload, nil
}

// boolFieldDefault 读取布尔字段；缺失时返回 fallback。
func boolFieldDefault(object map[string]json.RawMessage, fallback bool, keys ...string) bool {
	if value := boolPtrField(object, keys...); value != nil {
		return *value
	}
	return fallback
}

// ZcodeResetProvider 由 zcodeProvider 实现：消耗一张重置卡重置指定窗口。
type ZcodeResetProvider interface {
	ResetZcode(ctx context.Context, input ProviderInput, windowKey string) (ZcodeResetOutcome, error)
}

// zcodeResetCaller 由 cpa.Client 实现：直连插件 resource 重置路由。
type zcodeResetCaller interface {
	ResetZcodeWindow(ctx context.Context, windowKey string) error
}

// ZcodeResetOutcome 是 zcode 窗口重置的结果。
type ZcodeResetOutcome struct {
	Code string
}

// ValidateZcodeWindowKey 校验窗口参数，防止把任意字符串透传给插件。
func ValidateZcodeWindowKey(windowKey string) bool {
	return windowKey == "five_hour" || windowKey == "week"
}

func (p zcodeProvider) ResetZcode(ctx context.Context, input ProviderInput, windowKey string) (ZcodeResetOutcome, error) {
	if !ValidateZcodeWindowKey(windowKey) {
		return ZcodeResetOutcome{}, fmt.Errorf("%w: zcode reset requires window_key five_hour|week", ErrValidation)
	}
	caller, ok := p.caller.(zcodeResetCaller)
	if !ok {
		return ZcodeResetOutcome{}, fmt.Errorf("zcode reset caller is unavailable")
	}
	if err := caller.ResetZcodeWindow(ctx, windowKey); err != nil {
		return ZcodeResetOutcome{}, err
	}
	return ZcodeResetOutcome{Code: "reset"}, nil
}
