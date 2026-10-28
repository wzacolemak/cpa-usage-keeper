package test

import (
	"context"
	"testing"

	"cpa-usage-keeper/internal/cpa/dto/apicall"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/quota"
)

// TestAntigravityProviderCollectsGeminiGroupFromModels 验证 Gemini 组来自逐模型接口，
// 并按其 resetTime 分成 5 小时 / 每周两个窗口（依据见 antigravity_models.go 顶部）。
func TestAntigravityProviderCollectsGeminiGroupFromModels(t *testing.T) {
	caller := &recordingManagementCaller{responses: []*apicall.Response{
		// 1) retrieveUserQuotaSummary：只有 Claude/GPT 组（线上真实行为）。
		quotaAPIResponse(200, `{"body":{"groups":[{"displayName":"Claude and GPT models","description":"Models within this group: Claude Opus, Claude Sonnet, GPT-OSS","buckets":[{"bucketId":"3p-weekly","displayName":"Weekly Limit Remaining","window":"weekly","remainingFraction":0.25,"resetTime":"2026-11-03T17:07:47Z"},{"bucketId":"3p-5h","displayName":"Five Hour Limit Remaining","window":"5h","remainingFraction":1,"resetTime":"2026-10-28T15:59:10Z"}]}]}}`),
		// 2) retrieveUserQuota：逐模型桶。Gemini 有两条不同 resetTime：
		//    约 5 小时后（5 小时窗口）与约 6 天后（每周窗口）。
		quotaAPIResponse(200, `{"body":{"buckets":[
			{"modelId":"gemini-3-flash","tokenType":"WTUS","remainingFraction":0.9999,"resetTime":"2026-10-28T16:17:14Z"},
			{"modelId":"gemini-2.5-pro","tokenType":"WTUS","remainingFraction":0.9,"resetTime":"2026-10-28T16:17:14Z"},
			{"modelId":"gemini-3.1-pro","tokenType":"WTUS","remainingFraction":0.4,"resetTime":"2026-11-03T17:07:47Z"},
			{"modelId":"claude-sonnet-5-5-high","tokenType":"WTUS","remainingFraction":0.2416,"resetTime":"2026-11-03T17:07:47Z"},
			{"modelId":"chat_20706","tokenType":"WTUS","remainingFraction":1}
		]}}`),
		// 3) loadCodeAssist：套餐等级。
		quotaAPIResponse(200, `{"body":{"paidTier":{"id":"g1-pro-tier","name":"Google AI Pro"}}}`),
	}}
	configs := quota.DefaultProviderConfigs()
	provider := quota.NewAntigravityProvider(caller, configs.Antigravity[:1], configs.AntigravitySubscriptions, configs.AntigravityModels[:1])

	output, err := provider.Check(context.Background(), quota.ProviderInput{Identity: entities.UsageIdentity{
		Identity:  "ag-auth",
		ProjectID: new("aicode-consumers"),
	}})
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	result := output.Result.(quota.AntigravityResult)
	byName := make(map[string]quota.AntigravityQuotaGroup, len(result.Quota.Groups))
	for _, group := range result.Quota.Groups {
		byName[group.DisplayName] = group
	}

	if _, ok := byName["Claude and GPT models"]; !ok {
		t.Fatalf("expected Claude/GPT group from summary, got %v", byName)
	}
	gemini, ok := byName["Gemini models"]
	if !ok {
		t.Fatalf("expected Gemini group from per-model quota, got %v", byName)
	}
	// 官方文档：每组都有 Weekly + Five Hour 两个窗口。
	if len(gemini.Buckets) != 2 {
		t.Fatalf("expected 2 windows for Gemini group, got %d: %#v", len(gemini.Buckets), gemini.Buckets)
	}
	if gemini.Buckets[0].Window != "5h" || gemini.Buckets[1].Window != "weekly" {
		t.Fatalf("expected 5h then weekly ordering, got %q then %q", gemini.Buckets[0].Window, gemini.Buckets[1].Window)
	}
	// 5 小时窗口取同窗口内最紧的一条（0.9 比 0.9999 更紧）。
	fiveHour := gemini.Buckets[0]
	if fiveHour.RemainingFraction == nil || *fiveHour.RemainingFraction != 0.9 {
		t.Fatalf("expected tightest 5h remaining 0.9, got %v", fiveHour.RemainingFraction)
	}
	if fiveHour.ResetTime != "2026-10-28T16:17:14Z" {
		t.Fatalf("expected 5h resetTime, got %q", fiveHour.ResetTime)
	}
	// 每周窗口来自 resetTime 明显更晚的那批模型。
	weekly := gemini.Buckets[1]
	if weekly.RemainingFraction == nil || *weekly.RemainingFraction != 0.4 {
		t.Fatalf("expected tightest weekly remaining 0.4, got %v", weekly.RemainingFraction)
	}
	if weekly.ResetTime != "2026-11-03T17:07:47Z" {
		t.Fatalf("expected weekly resetTime, got %q", weekly.ResetTime)
	}

	// Claude/GPT 已由 summary 提供，不能因逐模型聚合而重复出现。
	count := 0
	for _, group := range result.Quota.Groups {
		if group.DisplayName == "Claude and GPT models" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one Claude/GPT group, got %d", count)
	}
}

// TestAntigravityProviderWithoutModelConfigsSkipsGeminiGroup 验证不传模型配置时
// 行为与旧版一致：只有 summary 的组，不额外发起 fetchAvailableModels 请求。
func TestAntigravityProviderWithoutModelConfigsSkipsGeminiGroup(t *testing.T) {
	caller := &recordingManagementCaller{responses: []*apicall.Response{
		quotaAPIResponse(200, `{"body":{"groups":[{"displayName":"Claude and GPT models","buckets":[{"bucketId":"3p-5h","displayName":"Five Hour Limit Remaining","window":"5h","remainingFraction":0.5,"resetTime":"2026-10-28T15:59:10Z"}]}]}}`),
		quotaAPIResponse(200, `{"body":{"paidTier":{"id":"g1-pro-tier","name":"Google AI Pro"}}}`),
	}}
	configs := quota.DefaultProviderConfigs()
	provider := quota.NewAntigravityProvider(caller, configs.Antigravity[:1], configs.AntigravitySubscriptions)

	output, err := provider.Check(context.Background(), quota.ProviderInput{Identity: entities.UsageIdentity{
		Identity:  "ag-auth",
		ProjectID: new("aicode-consumers"),
	}})
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	result := output.Result.(quota.AntigravityResult)
	if len(result.Quota.Groups) != 1 || result.Quota.Groups[0].DisplayName != "Claude and GPT models" {
		t.Fatalf("expected only summary group, got %#v", result.Quota.Groups)
	}
	for _, request := range caller.requests {
		if contains(request.URL, "fetchAvailableModels") {
			t.Fatalf("expected no fetchAvailableModels request without model configs, got %s", request.URL)
		}
	}
}
