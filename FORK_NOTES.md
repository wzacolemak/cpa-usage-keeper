# FORK_NOTES — 本 fork（main）相对上游 Willxup/cpa-usage-keeper 的改动留档

> 目的：记录 `wzacolemak/cpa-usage-keeper` 分支 `main` 在上游
> [Willxup/cpa-usage-keeper](https://github.com/Willxup/cpa-usage-keeper) 之上叠加的改动，
> 供未来合并上游时定位冲突与决策。
>
> 上游基线：fork 点 ≈ v1.15.4（885a671d）→ v2.0.3（2026-11-07，v1→v2 迁移）
> → **v2.1.0（2026-11-13）**。
> v2 是 next 分支整体晋升（v1.15.11→v2.0.0 改 644 文件）；v2.0.3→v2.1.0 为常规增量
> （69 提交），取舍见第四、五节与新增的第六节。

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

## 五、v2.0.3 迁移实录（2026-11-07）

### 被上游 v2 原生替代、已删除的 fork 实现
- **分时段计价整套**（internal/pricing/window.go、三处 pricing_time_windows_config.go、
  usage_overview_pricing 的 PricingBucket、WithOccurredAt、前端 PriceTimeWindowsPanel*、
  PriceSettingsCard/PriceRulesHelp 的 fork 接线、api.ts 的 PricingTimeWindows*）——
  v2 原生有 ModelPricingBranchEditor + Snapshot.location + Subject.Timestamp。
  **注意：v1 时代配置的 time_window 规则（3 条 peak×2.0）与 pricing_time_windows
  配置在迁移前手动清除，升级后需用 v2 的分支编辑器重建。**
- **antigravity 逐模型兜底**（antigravity_models.go、checkModelGroups、dedupe 循环、
  AntigravityModels 模板、两个 argon 测试中的 per-model 部分）——v2 的
  antigravityUserAgent = hub/2.23.0（≥2.9.0），summary 原生下发含 argon 的全部组。
- **ranking**（v2 删除并有专用 migration 20261009_remove_ranking）。

### 保留并移植的 fork 功能
- zcode provider 全栈（internal/cpa/zcode.go + internal/quota/zcode.go + reset 链路 +
  前端 SharedQuotaGroup/ZcodeWindowResetAction + i18n）——endpoint 常量并入 v2 的
  /v8/management 路径表；plugin summary 端点运行时发现逻辑不变。
- OpenCode Go / DeepSeek 余额面板（后端 + 前端 + env 配置）。
- tokenprocessor identity 别名 zcode/zcode-oauth → HandlerClaude（GLM input 折叠）。
- antigravity groupKey 规范化（含 antigravity-gemini-4-argon；已实测 v2 hub summary 下
  argon 独立组键正常）。
- provider display-name（ProviderKeyConfig/OpenAICompatibilityConfig 的 DisplayName 字段）。

### v2 迁移的数据坑
- app.db v1→v2 迁移会校验已存定价规则：**key=time_window 的规则会让迁移失败**
  （v2 规则字段表没有 time_window）。迁移失败时 v2 自动留备份（/data/backups/），
  恢复流程 = 停容器 → 用 pre-upgrade 备份覆盖 app.db → 清理不认识的规则 → 重启 v2。
- 本次迁移前手动删除了 3 条 time_window 测试规则（2026-11-05 由另一会话创建）。

### v2 后的合并基线
未来 merge 上游 v2.x：merge-base 已正确落在 v2 历史，冲突面回到常规规模；
行尾已全 LF（.gitattributes），照第一节流程走即可。

## 六、v2.1.0 合并实录（2026-11-13）

69 个上游提交（v2.0.3 → v2.1.0）。**4 个冲突，全部在前端**，且都属于「上游重构了
fork 也改过的同一处代码」。

### 冲突与解法

| 文件 | 冲突实质 | 解法 |
|---|---|---|
| `ProviderBrandIcon.tsx` | fork 用 `zcode` 品牌键；上游新增 `zai` 规范键 + `z.ai`/`z-ai`/`zhipu`/`glm`/`chatglm` 别名 | 取上游 `zai` 为规范键并保留其全部别名，**追加** fork 的 `zcode`/`zcode-oauth` 与 `zcode.svg`（两种图标并存：上游字形版、fork 圆角方块版） |
| `CredentialSections.module.scss` | fork 的 zcode 重置按钮样式 / 上游新增卡片紧凑变体 | 两块互不相关，**全部保留**。注意 ours 段末尾的 `}` 原本由冲突标记后的内容补齐，拼接时必须显式补回 |
| `CredentialSections.tsx` | 上游把 `SharedQuotaGroup` 重构为 `AntigravityQuotaGroup`（仅 antigravity）并加 `compact`；fork 的版本还承载分组标题本地化 + zcode 重置按钮 | 保留 fork 实现（是上游版本的功能超集），吸收上游的 `compact` 参数与 `fullWidth` 计算；`CredentialQuotaPanel` 签名合并两侧参数（`variant` + `onZcodeAfterReset`） |
| `i18n/index.ts` | 上游删除了 `credentials_auth_files_*` / `credentials_display_mode_*` 等键 | 这些键仍被 fork 的 display-mode 切换与 `credentialProviderFilters` 使用，**保留 ours**（theirs 侧为空块） |

### 合并时被静默覆盖、需手工补回的 fork 代码

冲突标记之外的自动合并也会吞掉 fork 内容，本次实测有 3 处，`npx tsc --noEmit` 全部抓到：

- `IconChartLine` 图标 import（display mode 切换按钮）
- `resetUsageQuota` API import（zcode 重置卡片）
- `type CredentialDisplayMode` 类型别名

**教训：合并后必须跑 `npx tsc --noEmit`**，Go 侧能编译不代表前端完整。

### 语义性回归（编译通过但行为错）

`internal/quota/normalize.go` 的 `knownAntigravityQuotaGroupKey`：fork 用宽松的
`hasGemini && hasArgon` 把任何带 argon 词的名字都归到独立 Argon 组；上游 v2.1.0 收紧为
**只有精确的 `gemini 4 argon`** 才归 Argon，带额外词的旧名（`Gemini 4 Argon Extra`）
落回通用 Gemini 组。合并保留了 fork 的 `hasArgon` 分支，导致
`TestNormalizeAntigravityQuotaRowsPreservesLegacyGroupNames/Gemini_4_Argon_Extra` 失败。
→ 删除 `hasArgon` 分支与重复的 `antigravityGeminiArgonGroupKey` 常量，取上游判据。

**教训：Go 侧除 `go build` 外必须跑 `go test ./...`**，这类归一化逻辑的错误只有测试能发现。

### 上游架构替代、需同步更新测试的 fork 定制

`AntigravityModels`（逐模型接口兜底）在 v2.0.3 迁移中已被 `retrieveUserQuotaSummary`
取代（第五节已记录），但 `internal/quota/test/config_test.go` 仍按 17 个模板断言，
长期处于失败状态（合并前即失败）。本次一并修正为 14 个模板并移除 `AntigravityModels` 断言。

### 测试基线（合并后）

- `go test ./...`：通过（除下述已知项）
- 前端 `npm test`：**1727 passed / 203 files**
- 前端 `npx tsc --noEmit`、`npm run lint`：通过

前端另有 2 个用例（`CredentialsSection.test.ts` 的 antigravity 分组渲染）在合并前即失败，
根因是测试的 `t` mock 返回 i18n 键名而非文案，而代码已把分组标题映射到
`QUOTA_GROUP_LABEL_KEYS`。本次给 mock 补了分组标题映射，使断言重新校验真实渲染结果。

### 环境坑：node_modules 属 root

部署树的 `web/node_modules` 由 root 构建产生，普通用户跑 `npm test` 会因
`node_modules/.vite-temp` 不可写而 `EACCES`。验证时把 `web/` 复制到可写目录再跑，
不要为此改部署树权限。
