# FORK_NOTES — 本 fork（main）相对上游 Willxup/cpa-usage-keeper 的改动留档

> 目的：记录 `wzacolemak/cpa-usage-keeper` 分支 `main` 在上游
> [Willxup/cpa-usage-keeper](https://github.com/Willxup/cpa-usage-keeper) 之上叠加的改动，
> 供未来合并上游时定位冲突与决策。
>
> 上游基线：fork 点 ≈ v1.15.4（885a671d）→ 已合并 **v1.15.11**（2026-11-07）。
> **上游 v2.0 是整体重写（v1.15.11→v2.0.0 改 644 文件），fork 的挂载点大半不存在，
> 升 v2 是迁移项目不是 merge** —— 详见第四节。

## 一、合并操作流程（v1 线已验证）

1. 部署树 `source/` 本身就是 git 仓库（branch main，origin = 本 fork）。
2. 提交部署树漂移（`git add -A && git commit`），保持「线上跑的 = 仓库里有的」。
3. `git fetch upstream --tags` → merge 最新 v1.x tag。
   **行尾注意**：仓库历史曾混入 CRLF blob（部署树拷贝所致），2026-11-07 已加
   `.gitattributes`（`* text=auto`）并 renormalize。之后一切编辑保持 LF；
   如见全文件冲突先查 `git ls-files --eol`。
4. 合并后 `go build ./...` + 定向测试（**整包 `go test ./internal/quota/...` 有无关长测会
   超时，务必用 `-run`**），再推送、`docker compose build && up -d`。

## 二、fork 功能清单（合并时保 ours 的部分）

### 后端（internal/）
- **zcode provider**（bigmodel GLM 编程套餐）：`internal/cpa/zcode.go`、
  `internal/quota/zcode.go`、registry 接线；summary 端点聚合、重置卡
  （`QuotaResetCardInfo`/`ResetCard` 字段、`reset.go` zcode 分支、`api/quota.go`
  window_key 透传）；`inspection.go` 的 zcode/zcode-oauth case。
- **tokenprocessor**：identity 别名 `zcode`/`zcode-oauth` → HandlerClaude
  （`internal/service/tokenprocessor/registry.go`）—— GLM 事件的 input 必须折叠
  cache read/creation，否则前端展示与计价全错（2026-11-04 修复）。
- **antigravity UA ≥ 2.9.1**（`internal/quota/config.go`：UA 低于 2.9.0 时 Google 不下发
  argon 额度；`AntigravityModels` 逐模型接口 + config_test 的 17 模板断言）。
- **normalize.go**：`antigravity-gemini-4-argon` 专用 groupKey；已耗尽模型按 0 处理。
- **分时段计价**（time-of-day pricing）：`usage_overview_pricing.go` 的 PricingBucket
  小时桶 + `usage_overview_comparisons.go` 的 WithOccurredAt + 前端 PriceTimeWindowsPanel。
- **OpenCode Go / DeepSeek 余额面板**的后端（cpa/client.go、endpoints.go、quota/zcode.go）。

### 前端（web/src/）
- `AuthFileCredentialsSection.tsx`：QUOTA_GROUP_KEYS/QUOTA_GROUP_LABEL_KEYS 白名单
  （未登记的 groupKey 前端整行丢弃）、SharedQuotaGroup + ZcodeWindowResetAction。
- `CredentialDetailDrawer.tsx`：OpenCode Go / DeepSeek 余额板块分支。
- `credentialViewModels.ts`、`credentialProviderFilters.ts`、i18n 三 locale
  （credentials_quota_group_gemini_argon、pricing_time_windows_* 等）。
- `lib/api.ts`、`lib/types.ts`：windowKey 重置参数、OpencodeGo/DeepSeek/PricingTimeWindows 类型。
- `assets/icons/zcode.svg`、`ProviderBrandIcon.tsx`。

## 三、冲突高发点与解法（v1 线）

1. `internal/quota/types.go` — 上游加字段（如 v1.15.11 的 `UsageBreakdown`）时，
   取上游字段 + 追加 fork 的 ResetCard/QuotaResetCardInfo。
2. `entities/usage_event*.go`、`redis_usage.go`、`migration.go` — 上游加会话/追踪列，
   fork 无对应改动 → 直接取上游（fork 的 session 列上游也有）。
3. `Dockerfile` — fork 是定制构建（golang 基础镜像 + GOTOOLCHAIN/GOPROXY/离线层），
   上游换基础镜像也不跟，保 ours。
4. web 组件 — fork 未真改的文件（对照 `git diff v1.15.10 HEAD --stat -- web/src`）
   一律取上游；fork 真改的保 ours 并吸收上游周边修复。
5. go.mod — 上游依赖树变化后跑 `go build ./...` 验证。

## 四、v2 迁移须知（升级到 v2.0.x 前必读）

- v2 = `next` 分支整体晋升，**v1.15.11→v2.0.0 改 644 文件（±68k 行）**，不是增量演进。
- fork 挂载点在 v2 中已消失：`internal/cpa/zcode.go`、
  `web/src/components/usage/credentials/AuthFileCredentialsSection.tsx` 均被重写。
  仍存在的：`internal/service/tokenprocessor/`、`internal/quota/registry.go`（GLM 修复可平移）。
- v2 升级 = **迁移项目**：zcode provider 重写接入新 CPA client 结构、凭据/配额页 UI 重做、
  `app.db` 自动迁移（有独立回滚指引；升级前必须备份 /data/app.db）。
- 上游 v1 线止于 v1.15.11（已 EOL），v1 通道不再发版。
