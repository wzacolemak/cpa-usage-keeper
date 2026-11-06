package cpa

const (
	cpaManagementCredentialsEndpoint         = "/v8/management/credentials"
	cpaManagementCredentialsDownloadEndpoint = "/v8/management/credentials/download"
	cpaManagementCredentialsStatusEndpoint   = "/v8/management/credentials/status"
	cpaManagementCredentialsFieldsEndpoint   = "/v8/management/credentials/fields"
	cpaManagementAPIKeysEndpoint             = "/v8/management/config/access/api-keys"
	cpaManagementVertexAPIKeyEndpoint        = "/v8/management/config/api-keys/vertex"
	cpaManagementGeminiAPIKeyEndpoint        = "/v8/management/config/api-keys/gemini"
	cpaManagementCodexAPIKeyEndpoint         = "/v8/management/config/api-keys/codex"
	cpaManagementClaudeAPIKeyEndpoint        = "/v8/management/config/api-keys/claude"
	cpaManagementMetaAPIKeyEndpoint          = "/v8/management/config/api-keys/meta"
	cpaManagementOpenAICompatibilityEndpoint = "/v8/management/config/api-keys/openai-compatibility"
	cpaManagementUsageQueueEndpoint          = "/v8/management/observability/usage/queue"
	cpaManagementAPICallEndpoint             = "/v8/management/requests/api-call"
	cpaManagementResetQuotaEndpoint          = "/v8/management/routing/cooldown/reset"
	cpaManagementRequestLogByIDEndpoint      = "/v8/management/observability/logs/requests"
	// cpaManagementPluginsEndpoint 用于发现当前已注册插件的真实 ID。
	cpaManagementPluginsEndpoint = "/v0/management/plugins"
	// cpaManagementZcodeResetEndpoint 是 zcode 插件的管理路由：宿主对 resource
	// 路由只转发 GET，程序化消耗重置卡必须走带管理密钥的管理 POST。
	cpaManagementZcodeResetEndpoint = "/v0/management/zcode/reset"
	cpaModelsEndpoint                        = "/v1/models"
	// cpaZcodeSummaryEndpoint 是 CPA zcode 插件自己聚合的用量摘要，属于 CPA 内部路由。
	//
	// 插件 ID 由插件文件名决定，运维改过名（zcode -> zcode-oauth）后这里写死就会 404，
	// 所以真正的端点由 resolveZcodeSummaryEndpoint 在运行时发现；此常量只作为兜底候选。
	cpaZcodeSummaryEndpoint = "/v0/resource/plugins/zcode/summary"
	// cpaZcodeSummaryResourceSuffix 是插件声明的 resource 路径，与插件 ID 无关。
	cpaZcodeSummaryResourceSuffix = "/summary"

	cpaManagementRedisNetwork       = "tcp"
	ManagementRedisDefaultPort      = "8317"
	ManagementRedisAuthCommand      = "AUTH"
	ManagementRedisPopCommand       = "LPOP"
	ManagementRedisSubscribeCommand = "SUBSCRIBE"
	ManagementUsageQueueKey         = "usage"
	ManagementUsageLegacyQueueKey   = "queue"
	ManagementUsageSubscribeChannel = "usage"
	// ManagementErrorsSubscribeChannel 是 CPA 只广播、不提供 LPOP 补偿的凭证错误 channel。
	ManagementErrorsSubscribeChannel = "errors"
	ManagementUsageQueueMaxBatchSize = 10000
)

const (
	// cpaManagementInteractionsAPIKeyEndpoint 只读取 Gemini Interactions metadata，不参与 usage 拉取。
	cpaManagementInteractionsAPIKeyEndpoint = "/v8/management/config/api-keys/interactions"
	// cpaManagementXAIAPIKeyEndpoint 只读取 xAI API Key metadata，不改变现有 xAI OAuth 或 quota 路径。
	cpaManagementXAIAPIKeyEndpoint = "/v8/management/config/api-keys/xai"
)
