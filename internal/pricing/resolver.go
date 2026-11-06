package pricing

import (
	"time"

	"cpa-usage-keeper/internal/helper"
)

// CostSubject 是所有 usage 来源进入计价领域的唯一固定输入。
type CostSubject struct {
	Dimensions UsageDimensions
	Tokens     helper.UsageTokenCostInput
	// OccurredAt 是该笔用量发生的时刻（事件时间或聚合桶起点）。
	// 分时段计价用它推导 TimeWindow；零值表示无法确定时刻，此时不做分时段加成。
	OccurredAt time.Time
}

// WithOccurredAt 返回带上发生时刻的新 subject，用于分时段计价。
func (s CostSubject) WithOccurredAt(at time.Time) CostSubject {
	s.OccurredAt = at
	return s
}

func NewCostSubject(dimensions UsageDimensions, tokens helper.UsageTokenCostInput) CostSubject {
	return CostSubject{
		Dimensions: canonicalizeUsageDimensions(dimensions),
		Tokens:     tokens,
	}
}

type CostResult struct {
	Cost           helper.UsageTokenCostBreakdown
	Available      bool
	PricingStyle   string
	MatchedModel   string
	MatchedBy      string
	RuleMultiplier float64
	// TimeWindow 是本次计价命中的时段名，空表示未命中任何时段。
	TimeWindow string
}

// Resolver 在创建时固定绑定一个 Snapshot，确保单个响应不会混用新旧价格。
type Resolver struct {
	snapshot *Snapshot
}

func (r Resolver) ActiveFields() ActiveFields {
	if r.snapshot == nil {
		return 0
	}
	return r.snapshot.activeFields
}

func (r Resolver) Calculate(subject CostSubject) CostResult {
	// 先按事件时刻推导时段，再据此匹配规则，使 time_window 规则与其他维度规则同构。
	if r.snapshot != nil && r.snapshot.windows != nil && !subject.OccurredAt.IsZero() {
		subject.Dimensions.TimeWindow = r.snapshot.windows.WindowAt(subject.OccurredAt)
	}
	model, matchedModel, matchedBy, found := r.matchModel(subject.Dimensions)
	if !found {
		return CostResult{
			Available:      !helper.UsageTokenInputRequiresPricing(subject.Tokens),
			RuleMultiplier: 1,
		}
	}

	breakdown := helper.CalculateUsageTokenCostBreakdown(subject.Tokens, model.pricing)
	ruleMultiplier := 1.0
	if model.pricing.PriceMultiplier == nil || *model.pricing.PriceMultiplier != 0 {
		ruleMultiplier = matchingRuleMultiplier(model.rules, subject.Dimensions)
		breakdown = helper.ScaleUsageTokenCostBreakdown(breakdown, ruleMultiplier)
	}
	return CostResult{
		Cost:           breakdown,
		Available:      true,
		PricingStyle:   model.pricing.PricingStyle,
		MatchedModel:   matchedModel,
		MatchedBy:      matchedBy,
		RuleMultiplier: ruleMultiplier,
		TimeWindow:     subject.Dimensions.TimeWindow,
	}
}

func (r Resolver) matchModel(dimensions UsageDimensions) (compiledModel, string, string, bool) {
	if r.snapshot == nil {
		return compiledModel{}, "", "", false
	}
	if model, ok := r.snapshot.modelsByName[dimensions.Model]; ok {
		return model, dimensions.Model, "model", true
	}
	if model, ok := r.snapshot.modelsByName[dimensions.ModelAlias]; ok {
		return model, dimensions.ModelAlias, "model_alias", true
	}
	return compiledModel{}, "", "", false
}

func matchingRuleMultiplier(rules []compiledRule, dimensions UsageDimensions) float64 {
	multiplier := 1.0
	for _, rule := range rules {
		if dimensions.Value(rule.field) != rule.value {
			continue
		}
		if rule.multiplier == 0 {
			return 0
		}
		multiplier *= rule.multiplier
	}
	return multiplier
}
