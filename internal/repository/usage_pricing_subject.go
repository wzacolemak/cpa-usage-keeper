package repository

import (
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/helper"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/repository/dto"
)

func UsageEventCostSubject(event entities.UsageEvent) pricing.CostSubject {
	modelAlias := ""
	if event.ModelAlias != nil {
		modelAlias = *event.ModelAlias
	}
	return newUsagePricingCostSubject(
		event.APIGroupKey,
		event.Model,
		event.AuthIndex,
		modelAlias,
		event.ServiceTier,
		event.ResponseServiceTier,
		event.ReasoningEffort,
		event.Endpoint,
		event.ExecutorType,
		event.InputTokens,
		event.OutputTokens,
		event.CacheReadTokens,
		event.CacheCreationTokens,
	).WithOccurredAt(event.Timestamp)
}

func UsageEventRecordCostSubject(record dto.UsageEventRecord) pricing.CostSubject {
	return newUsagePricingCostSubject(
		record.APIGroupKey,
		record.Model,
		record.AuthIndex,
		record.ModelAlias,
		record.ServiceTier,
		record.ResponseServiceTier,
		record.ReasoningEffort,
		record.Endpoint,
		record.ExecutorType,
		record.InputTokens,
		record.OutputTokens,
		record.CacheReadTokens,
		record.CacheCreationTokens,
	).WithOccurredAt(record.Timestamp)
}

func UsageOverviewHourlyCostSubject(row entities.UsageOverviewHourlyStat) pricing.CostSubject {
	return newUsagePricingCostSubject(
		row.APIGroupKey,
		row.Model,
		row.AuthIndex,
		row.ModelAlias,
		row.ServiceTier,
		row.ResponseServiceTier,
		row.ReasoningEffort,
		row.Endpoint,
		row.ExecutorType,
		row.InputTokens,
		row.OutputTokens,
		row.CacheReadTokens,
		row.CacheCreationTokens,
	).WithOccurredAt(row.BucketStart)
}

// UsageOverviewDailyCostSubject 构造日粒度成本主体。
//
// 日桶只保留日期，无法还原日内峰谷，因此这里不设置 OccurredAt：分时段倍率不会生效。
// 需要分时段精确计价的分析视图由上层在启用时段规则时改用小时桶聚合。
func UsageOverviewDailyCostSubject(row entities.UsageOverviewDailyStat) pricing.CostSubject {
	return newUsagePricingCostSubject(
		row.APIGroupKey,
		row.Model,
		row.AuthIndex,
		row.ModelAlias,
		row.ServiceTier,
		row.ResponseServiceTier,
		row.ReasoningEffort,
		row.Endpoint,
		row.ExecutorType,
		row.InputTokens,
		row.OutputTokens,
		row.CacheReadTokens,
		row.CacheCreationTokens,
	)
}

func newUsagePricingCostSubject(
	apiGroupKey, model, authIndex, modelAlias, serviceTier, responseServiceTier, reasoningEffort, endpoint, executorType string,
	inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens int64,
) pricing.CostSubject {
	return pricing.NewCostSubject(pricing.UsageDimensions{
		APIGroupKey:         apiGroupKey,
		Model:               model,
		AuthIndex:           authIndex,
		ModelAlias:          modelAlias,
		ServiceTier:         serviceTier,
		ResponseServiceTier: responseServiceTier,
		ReasoningEffort:     reasoningEffort,
		Endpoint:            endpoint,
		ExecutorType:        executorType,
	}, helper.UsageTokenCostInput{
		InputTokens:         inputTokens,
		OutputTokens:        outputTokens,
		CacheReadTokens:     cacheReadTokens,
		CacheCreationTokens: cacheCreationTokens,
	})
}
