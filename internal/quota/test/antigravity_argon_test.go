package test

import (
	"context"
	"strings"
	"testing"

	"cpa-usage-keeper/internal/cpa/dto/apicall"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/quota"
)

// TestAntigravityProviderKeepsExhaustedArgonModels 验证 models 映射形态下
// 已耗尽的 argon 模型（fetchAvailableModels 省略 remainingFraction，只给
// resetTime）仍按 0 参与聚合，周窗口组不会整组消失。
func TestAntigravityProviderKeepsExhaustedArgonModels(t *testing.T) {
	caller := &recordingManagementCaller{responses: []*apicall.Response{
		// 1) retrieveUserQuotaSummary：只有 Claude/GPT 组。
		quotaAPIResponse(200, `{"body":{"groups":[{"displayName":"Claude and GPT models","buckets":[{"bucketId":"3p-weekly","displayName":"Weekly Limit Remaining","window":"weekly","remainingFraction":0.25,"resetTime":"2026-11-03T17:07:47Z"}]}]}}`),
		// 2) fetchAvailableModels：models 映射。argon 系列已耗尽——没有
		//    remainingFraction 字段，只有周窗口 resetTime。
		quotaAPIResponse(200, `{"body":{"models":{
			"gemini-3-flash":{"apiProvider":"API_PROVIDER_GOOGLE_GEMINI","displayName":"gemini-3-flash","quotaInfo":{"remainingFraction":1,"resetTime":"2026-11-01T10:41:55Z"}},
			"gemini-4-argon-low":{"apiProvider":"API_PROVIDER_GOOGLE_GEMINI","displayName":"gemini-4-argon-low","quotaInfo":{"resetTime":"2026-11-03T16:26:03Z"}},
			"gemini-4-argon-medium":{"apiProvider":"API_PROVIDER_GOOGLE_GEMINI","displayName":"gemini-4-argon-medium","quotaInfo":{"resetTime":"2026-11-03T16:26:03Z"}},
			"gemini-4-argon-high":{"apiProvider":"API_PROVIDER_GOOGLE_GEMINI","displayName":"gemini-4-argon-high","quotaInfo":{"resetTime":"2026-11-03T16:26:03Z"}}
		}}}`),
		// 3) loadCodeAssist：套餐等级。
		quotaAPIResponse(200, `{"body":{"paidTier":{"id":"g1-pro-tier","name":"Google AI Pro"}}}`),
	}}
	configs := quota.DefaultProviderConfigs()
	provider := quota.NewAntigravityProvider(caller, configs.Antigravity[:1], configs.AntigravitySubscriptions, configs.AntigravityModels[1:2])

	output, err := provider.Check(context.Background(), quota.ProviderInput{Identity: entities.UsageIdentity{
		Identity:  "ag-auth",
		ProjectID: new("aicode-consumers"),
	}})
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	result := output.Result.(quota.AntigravityResult)
	var gemini *quota.AntigravityQuotaGroup
	for i, group := range result.Quota.Groups {
		if group.DisplayName == "Gemini models" {
			gemini = &result.Quota.Groups[i]
		}
	}
	if gemini == nil {
		t.Fatalf("expected Gemini group, got %#v", result.Quota.Groups)
	}
	if len(gemini.Buckets) != 2 {
		t.Fatalf("expected 5h + weekly buckets, got %#v", gemini.Buckets)
	}
	weekly := gemini.Buckets[1]
	if weekly.Window != "weekly" {
		t.Fatalf("expected weekly bucket second, got %#v", gemini.Buckets)
	}
	if weekly.RemainingFraction == nil || *weekly.RemainingFraction != 0 {
		t.Fatalf("expected weekly remaining 0 for exhausted argon, got %v", weekly.RemainingFraction)
	}
	if weekly.ResetTime != "2026-11-03T16:26:03Z" {
		t.Fatalf("expected weekly resetTime, got %q", weekly.ResetTime)
	}
	for _, name := range []string{"gemini-4-argon-low", "gemini-4-argon-medium", "gemini-4-argon-high"} {
		if !strings.Contains(gemini.Description, name) {
			t.Fatalf("expected %s listed in group description %q", name, gemini.Description)
		}
	}
}

// TestAntigravityProviderPrefersSummaryGroupsAndSeparateArgonKey 验证：
// summary（UA>=2.9.0）已返回 Gemini Models 与独立的 Gemini 4 Argon 组时，
// 逐模型聚合的同名组被去重；argon 组拿到独立 groupKey，不并入通用 Gemini 组。
func TestAntigravityProviderPrefersSummaryGroupsAndSeparateArgonKey(t *testing.T) {
	caller := &recordingManagementCaller{responses: []*apicall.Response{
		// 1) retrieveUserQuotaSummary：三组，含独立的 argon 组。
		quotaAPIResponse(200, `{"body":{"groups":[
			{"displayName":"Gemini Models","description":"Models within this group: Gemini Flash, Gemini Pro","buckets":[
				{"bucketId":"gemini-weekly","displayName":"Weekly Limit Remaining","window":"weekly","resetTime":"2026-11-03T20:58:20Z","remainingFraction":0.9992977},
				{"bucketId":"gemini-5h","displayName":"Five Hour Limit Remaining","window":"5h","resetTime":"2026-11-01T11:18:18Z","remainingFraction":1}]},
			{"displayName":"Claude and GPT models","buckets":[
				{"bucketId":"3p-weekly","displayName":"Weekly Limit Remaining","window":"weekly","resetTime":"2026-11-03T17:07:47Z","remainingFraction":0.0051556}]},
			{"displayName":"Gemini 4 Argon","description":"For paid Pro plans, Argon quota is separate.","buckets":[
				{"bucketId":"gemini-argon-wtu-weekly","displayName":"Gemini 4 Argon Weekly Limit","window":"weekly","resetTime":"2026-11-03T16:26:03Z","remainingFraction":0}]}
		]}}`),
		// 2) 逐模型接口：聚合出与 summary 同名的 Gemini models 组。
		quotaAPIResponse(200, `{"body":{"buckets":[
			{"modelId":"gemini-3-flash","remainingFraction":1,"resetTime":"2026-11-01T11:18:18Z"},
			{"modelId":"gemini-4-argon-high","remainingFraction":0,"resetTime":"2026-11-03T16:26:03Z"}]}}`),
		// 3) loadCodeAssist。
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
	names := make([]string, 0, len(result.Quota.Groups))
	for _, group := range result.Quota.Groups {
		names = append(names, group.DisplayName)
	}
	if len(names) != 3 {
		t.Fatalf("expected exactly 3 groups after dedupe, got %v", names)
	}
	var argonGroup *quota.AntigravityQuotaGroup
	for i := range result.Quota.Groups {
		if result.Quota.Groups[i].DisplayName == "Gemini 4 Argon" {
			argonGroup = &result.Quota.Groups[i]
		}
	}
	if argonGroup == nil || len(argonGroup.Buckets) != 1 || argonGroup.Buckets[0].Window != "weekly" {
		t.Fatalf("expected argon group with one weekly bucket, got %#v", argonGroup)
	}

	rows := quota.NormalizeQuotaRows(output)
	argonKeyRows := 0
	for _, row := range rows {
		if row.GroupLabel == "Gemini 4 Argon" {
			if row.GroupKey != "antigravity-gemini-4-argon" {
				t.Fatalf("expected dedicated argon group key, got %q", row.GroupKey)
			}
			argonKeyRows++
		}
		if row.GroupKey == "antigravity-gemini-models" && row.GroupLabel == "Gemini models" {
			t.Fatalf("per-model duplicate group leaked into rows: %#v", row)
		}
	}
	if argonKeyRows == 0 {
		t.Fatalf("expected argon rows in normalized output, got %#v", rows)
	}
}
