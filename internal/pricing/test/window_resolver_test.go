package test

import (
	"math"
	"testing"
	"time"

	"cpa-usage-keeper/internal/helper"
	"cpa-usage-keeper/internal/pricing"
)

// testPeakWindows 构造 DeepSeek 风格的高峰时段：工作日 09:00-12:00 与 14:00-18:00（北京时间）。
func testPeakWindows(t *testing.T) *pricing.TimeWindowSet {
	t.Helper()
	set, err := pricing.CompileTimeWindows(pricing.TimeWindowConfig{
		Timezone: "Asia/Shanghai",
		Windows: []pricing.TimeWindowSpec{{
			Name:     "peak",
			Weekdays: []int{1, 2, 3, 4, 5},
			Ranges:   []string{"09:00-12:00", "14:00-18:00"},
		}},
	})
	if err != nil {
		t.Fatalf("compile time windows: %v", err)
	}
	return set
}

func compileResolverWithWindows(t *testing.T, windows *pricing.TimeWindowSet, configs ...pricing.ModelConfig) pricing.Resolver {
	t.Helper()
	snapshot, err := pricing.CompileSnapshot(configs, windows)
	if err != nil {
		t.Fatalf("CompileSnapshot returned error: %v", err)
	}
	return pricing.NewCatalog(snapshot).NewResolver()
}

func beijingTime(t *testing.T, year int, month time.Month, day, hour int) time.Time {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	return time.Date(year, month, day, hour, 0, 0, 0, loc)
}

func TestResolverAppliesTimeWindowMultiplier(t *testing.T) {
	t.Parallel()

	config := pricing.ModelConfig{
		Pricing: testPricingWithPrompt("deepseek-v4.1-flash", 0.15),
		Rules: []pricing.RuleConfig{
			// 高峰倍率 2×：0.15 → 0.30，与 DeepSeek 官方高峰价一致。
			{Key: "time_window", Value: "peak", Multiplier: 2},
		},
	}
	resolver := compileResolverWithWindows(t, testPeakWindows(t), config)
	tokens := helper.UsageTokenCostInput{InputTokens: 1_000_000}

	// 周一 10:00（北京）属于高峰。
	peak := resolver.Calculate(pricing.NewCostSubject(
		pricing.UsageDimensions{Model: "deepseek-v4.1-flash"}, tokens,
	).WithOccurredAt(beijingTime(t, 2026, 11, 2, 10)))
	assertResultCost(t, peak, 0.30)
	if peak.TimeWindow != "peak" || peak.RuleMultiplier != 2 {
		t.Fatalf("expected peak window with multiplier 2, got %+v", peak)
	}

	// 周一 12:30 属于空闲，不应加成。
	offPeak := resolver.Calculate(pricing.NewCostSubject(
		pricing.UsageDimensions{Model: "deepseek-v4.1-flash"}, tokens,
	).WithOccurredAt(beijingTime(t, 2026, 11, 2, 12)))
	assertResultCost(t, offPeak, 0.15)
	if offPeak.TimeWindow != "" || offPeak.RuleMultiplier != 1 {
		t.Fatalf("expected off-peak without multiplier, got %+v", offPeak)
	}

	// 周六 10:00 虽在小时区间内，但周末全天空闲。
	weekend := resolver.Calculate(pricing.NewCostSubject(
		pricing.UsageDimensions{Model: "deepseek-v4.1-flash"}, tokens,
	).WithOccurredAt(beijingTime(t, 2026, 11, 7, 10)))
	assertResultCost(t, weekend, 0.15)
}

func TestResolverWithoutOccurredAtIgnoresTimeWindowRules(t *testing.T) {
	t.Parallel()

	config := pricing.ModelConfig{
		Pricing: testPricingWithPrompt("deepseek-v4.1-flash", 0.15),
		Rules: []pricing.RuleConfig{
			{Key: "time_window", Value: "peak", Multiplier: 2},
		},
	}
	resolver := compileResolverWithWindows(t, testPeakWindows(t), config)
	// 没有事件时刻（例如日粒度汇总）时不能猜时段，应保持倍率 1。
	result := resolver.Calculate(pricing.NewCostSubject(
		pricing.UsageDimensions{Model: "deepseek-v4.1-flash"},
		helper.UsageTokenCostInput{InputTokens: 1_000_000},
	))
	assertResultCost(t, result, 0.15)
	if result.RuleMultiplier != 1 {
		t.Fatalf("expected multiplier 1 without timestamp, got %+v", result)
	}
}

func TestResolverWithoutWindowsKeepsLegacyBehaviour(t *testing.T) {
	t.Parallel()

	// 未配置时段时，带时间戳也不会产生任何加成，保证与既有行为完全一致。
	config := pricing.ModelConfig{Pricing: testPricingWithPrompt("deepseek-v4.1-flash", 0.15)}
	resolver := compileResolver(t, config)
	result := resolver.Calculate(pricing.NewCostSubject(
		pricing.UsageDimensions{Model: "deepseek-v4.1-flash"},
		helper.UsageTokenCostInput{InputTokens: 1_000_000},
	).WithOccurredAt(beijingTime(t, 2026, 11, 2, 10)))
	assertResultCost(t, result, 0.15)
	if result.TimeWindow != "" {
		t.Fatalf("expected no window without configuration, got %q", result.TimeWindow)
	}
}

func TestResolverCombinesTimeWindowWithOtherRuleFields(t *testing.T) {
	t.Parallel()

	// 时段倍率要能与既有维度规则相乘，而不是互相覆盖。
	config := pricing.ModelConfig{
		Pricing: testPricingWithPrompt("deepseek-v4.1-flash", 0.15),
		Rules: []pricing.RuleConfig{
			{Key: "time_window", Value: "peak", Multiplier: 2},
			{Key: "endpoint", Value: "/v1/messages", Multiplier: 1.5},
		},
	}
	resolver := compileResolverWithWindows(t, testPeakWindows(t), config)
	result := resolver.Calculate(pricing.NewCostSubject(
		pricing.UsageDimensions{Model: "deepseek-v4.1-flash", Endpoint: "/v1/messages"},
		helper.UsageTokenCostInput{InputTokens: 1_000_000},
	).WithOccurredAt(beijingTime(t, 2026, 11, 2, 10)))
	assertResultCost(t, result, 0.45)
	if math.Abs(result.RuleMultiplier-3) > 1e-9 {
		t.Fatalf("expected combined multiplier 3, got %v", result.RuleMultiplier)
	}
}
