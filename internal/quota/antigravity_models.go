package quota

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"cpa-usage-keeper/internal/cpa/dto/apicall"
	"cpa-usage-keeper/internal/timeutil"
)

// 依据（均来自官方，不靠推测）：
//
//  1. https://antigravity.google/docs/plans
//     Google AI Pro: "A high, generous quota, refreshed every five hours until
//     the weekly limit is reached" —— 每组都同时存在【5 小时】与【每周】两个窗口。
//
//  2. https://antigravity.google/docs/models（Choose a model → View Usage）
//     UI 里就是两组、每组两个窗口：
//     "Gemini Models / Weekly Limit Remaining / Five Hour Limit Remaining"
//     "Claude and GPT models / Weekly Limit Remaining / Five Hour Limit Remaining"
//
//  3. 上游接口分工（实测）：
//     - retrieveUserQuotaSummary 只返回 Claude/GPT 组（含 3p-weekly、3p-5h 两个桶）。
//     - retrieveUserQuota / fetchAvailableModels 返回逐模型 remainingFraction + resetTime，
//       且 resetTime 只有两种取值：约 5 小时后（5h 窗口）与约 7 天后（每周窗口）。
//
// 因此 Gemini 组的两个窗口按 resetTime 距当前时间的远近分类：
// 最近的 resetTime 属于 5 小时窗口，较远的属于每周窗口。这不是按量猜测，
// 而是上游每个窗口只暴露一个 resetTime 的客观结构决定的。

const (
	antigravityGeminiAPIP  = "API_PROVIDER_GOOGLE_GEMINI"
	antigravityWindowFiveH = "5h"
	antigravityWindowWeek  = "weekly"
	// antigravityWeekWindowThreshold 用于区分两个窗口：5 小时窗口的 resetTime 不会
	// 超过 5 小时，每周窗口不会少于 1 天，取中间值即可稳定区分，且两窗口即便同日
	// 到期也不会互相误判。
	antigravityWeekWindowThreshold = 24 * time.Hour
)

// antigravityModelBucket 是某个 apiProvider 下、某个窗口的聚合结果。
type antigravityModelBucket struct {
	models    []string
	remaining *float64
	resetTime string
}

// parseAntigravityModelQuotaGroups 从逐模型响应聚合出 Gemini 组的两个窗口。
func parseAntigravityModelQuotaGroups(response *apicall.Response) ([]AntigravityQuotaGroup, error) {
	object, err := parseResponseObject(response)
	if err != nil {
		return nil, err
	}
	// 兼容两种上游形态：fetchAvailableModels 用 models 映射，
	// retrieveUserQuota 用 buckets 数组；两者字段语义一致。
	if _, ok := object["models"]; !ok {
		if nested := objectField(object, "body"); nested != nil {
			object = nested
		}
	}

	type modelQuota struct {
		apiProvider string
		displayName string
		remaining   *float64
		resetTime   string
	}
	models := make([]modelQuota, 0, 32)
	if modelMap := objectField(object, "models"); modelMap != nil {
		for modelName, raw := range modelMap {
			modelObject := rawObject(raw)
			if modelObject == nil {
				continue
			}
			quotaInfo := objectField(modelObject, "quotaInfo", "quota_info")
			if quotaInfo == nil {
				continue
			}
			models = append(models, modelQuota{
				apiProvider: strings.TrimSpace(stringField(modelObject, "apiProvider", "api_provider")),
				displayName: firstNonEmpty(stringField(modelObject, "displayName", "display_name"), modelName),
				remaining:   quotaFractionPtrField(quotaInfo, "remainingFraction", "remaining_fraction"),
				resetTime:   stringField(quotaInfo, "resetTime", "reset_time"),
			})
		}
	} else {
		for _, raw := range arrayField(object, "buckets") {
			bucketObject := rawObject(raw)
			if bucketObject == nil {
				continue
			}
			models = append(models, modelQuota{
				// retrieveUserQuota 的 bucket 不带 apiProvider，按模型名判断。
				apiProvider: antigravityModelAPIProvider(stringField(bucketObject, "modelId", "model_id")),
				displayName: stringField(bucketObject, "modelId", "model_id"),
				remaining:   quotaFractionPtrField(bucketObject, "remainingFraction", "remaining_fraction"),
				resetTime:   stringField(bucketObject, "resetTime", "reset_time"),
			})
		}
	}

	// 收集 Gemini 模型的全部 resetTime，用于确定两个窗口的分界。
	resetTimes := make([]string, 0, len(models))
	for _, model := range models {
		if model.apiProvider == antigravityGeminiAPIP && model.remaining != nil && model.resetTime != "" {
			resetTimes = append(resetTimes, model.resetTime)
		}
	}
	if len(resetTimes) == 0 {
		return nil, nil
	}
	earliest := ""
	for _, resetTime := range resetTimes {
		if earliest == "" || resetTime < earliest {
			earliest = resetTime
		}
	}

	// 按窗口分桶：resetTime 与最早者相差不超过阈值的算 5 小时窗口，其余算每周窗口。
	buckets := map[string]*antigravityModelBucket{}
	for _, model := range models {
		if model.apiProvider != antigravityGeminiAPIP || model.remaining == nil {
			continue
		}
		window := antigravityWindowFiveH
		if model.resetTime != "" && !antigravitySameWindow(earliest, model.resetTime) {
			window = antigravityWindowWeek
		}
		entry, ok := buckets[window]
		if !ok {
			entry = &antigravityModelBucket{}
			buckets[window] = entry
		}
		if model.displayName != "" {
			entry.models = append(entry.models, model.displayName)
		}
		// 同窗口内取最紧的一条：剩余最小者代表该窗口的真实约束。
		if entry.remaining == nil || *model.remaining < *entry.remaining {
			entry.remaining = model.remaining
			entry.resetTime = model.resetTime
		}
	}
	if len(buckets) == 0 {
		return nil, nil
	}

	group := AntigravityQuotaGroup{
		DisplayName: "Gemini models",
		// 与 summary 组的 description 同风格，列出该组覆盖的模型。
		Description: "",
	}
	groupModels := map[string]struct{}{}
	for _, entry := range buckets {
		for _, name := range entry.models {
			groupModels[name] = struct{}{}
		}
	}
	if len(groupModels) > 0 {
		names := make([]string, 0, len(groupModels))
		for name := range groupModels {
			names = append(names, name)
		}
		sort.Strings(names)
		group.Description = fmt.Sprintf("Models within this group: %s", strings.Join(names, ", "))
	}

	// 固定顺序：先 5 小时、后每周，与官方 UI 的显示顺序一致。
	for _, window := range []string{antigravityWindowFiveH, antigravityWindowWeek} {
		entry, ok := buckets[window]
		if !ok || entry.remaining == nil {
			continue
		}
		displayName := "5h"
		if window == antigravityWindowWeek {
			displayName = "Weekly"
		}
		group.Buckets = append(group.Buckets, AntigravityQuotaBucket{
			BucketID:          "models-" + window,
			DisplayName:       displayName,
			Window:            window,
			RemainingFraction: entry.remaining,
			ResetTime:         entry.resetTime,
		})
	}
	return []AntigravityQuotaGroup{group}, nil
}

// antigravitySameWindow 判断两个 resetTime 是否属于同一个额度窗口。
func antigravitySameWindow(earliest string, candidate string) bool {
	earliestTime, err := timeutil.ParseStorageTime(earliest)
	if err != nil {
		return true
	}
	candidateTime, err := timeutil.ParseStorageTime(candidate)
	if err != nil {
		return true
	}
	diff := candidateTime.Sub(earliestTime)
	if diff < 0 {
		diff = -diff
	}
	return diff < antigravityWeekWindowThreshold
}

// antigravityModelAPIProvider 从模型名推断 provider，用于 retrieveUserQuota 的 bucket。
func antigravityModelAPIProvider(modelID string) string {
	switch {
	case strings.HasPrefix(modelID, "gemini-"):
		return antigravityGeminiAPIP
	default:
		return ""
	}
}
