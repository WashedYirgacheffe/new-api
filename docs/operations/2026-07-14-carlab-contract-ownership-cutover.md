# CarLab 模型合同职责切换维护记录

- 日期：2026-07-14
- 状态：代码、数据库与配置已完成，等待云端部署验收
- CarLab API 分支：`codex/oem-api-hub`
- Superseed 分支：`codex/ai-service-migration`
- 公网入口：`https://api.carlab.top`

## 范围

本轮把 Profile、模型 Binding、请求映射、参数倍率、端点和响应契约的编辑职责全部收口到 CarLab API。TapDash 不再提供模型合同编辑，只保留只读合同状态、应用采用、Superseed 端到端烟测、发布状态和 OEM 模型集合。

同时延续上一轮已经确认的 DeepWL 经典模型接入：为 GPT Image 2、三条 canonical Banana 图片模型和 Omni Video 建立或补齐能力合同；Seedance 2.0 仅保存未绑定文档模板，因为当前三把 DeepWL Key 均未获得该模型授权。

## 假设与职责边界

- CarLab API 是 Profile、Binding、请求/响应协议和采购计价的唯一真相源。
- TapDash 的“采用合同”只固定 Superseed 应用模型使用的 `contract_hash`，不修改 CarLab 合同。
- TapDash 的真实烟测继续验证 Superseed、Generation BFF、CarLab 路由和结果回收的端到端链路。
- TapLater 只消费动态目录和有效合同，不保存供应商价格副本，也不显示 DeepWL/FOX 渠道文字标签。
- 供应商公开 Pricing 页面只作为目录证据；生产成本仍以 CarLab Quote 与真实消费日志为准。
- 本文档、Git 提交和数据库快照均不包含 API Key、Token、Cookie、密码或渠道密钥。

## 代码改动

### CarLab API

- `web/default/src/features/models/components/model-contracts.tsx`：新增模型合同管理页，可创建/版本化 Profile，按完整 Gateway Model ID 读取和保存 Binding，并编辑结构化 Overrides。
- `web/default/src/features/models/api.ts`、`types.ts`、`index.tsx`、`section-registry.tsx`：接入合同 API、表单校验、模型页签和 `/models/contracts` 路由。
- `web/default/src/i18n/locales/en.json`、`zh.json`：补齐合同页中英文文案。
- `model/model_operation_contract.go`：允许受控 `/v1beta/` 路径并注册 `gemini-image` 请求适配器；仍拒绝查询串、目录穿越和未注册适配器。
- `controller/model_catalog.go`：将同步 Gemini 图片合同标记为可调度。
- `model/pricing.go`：同时解析模型元数据中对象和数组两种 `endpoints` 格式，避免数组形式静默丢失端点能力。
- `model/model_operation_profile.go`：新增 GPT Image 2、Gemini 原生图片、Gemini OpenAI 图片、Omni Video 和 Seedance 2.0 文档模板。Seedance 模板没有 `ModelNames`，不会产生 Binding 或上架候选。
- `model/model_operation_contract_test.go`、`model/pricing_test.go`：覆盖 Gemini 适配器、`/v1beta/` 路径和数组端点解析。

### Superseed

- `CarLab/TapDash/api/ai-profiles.js`：`save`、`bind`、`delete_binding` 固定返回 HTTP 403；只保留 Profile 和 Binding 查询。
- `CapabilityProfilesView.vue`：移除新建、编辑和新版本抽屉，改为只读清单与 CarLab API 跳转。
- `ModelContractLabDrawer.vue`：移除字段、倍率、Quote 和 Binding 编辑表单，改为只读合同、计价和验证记录；保留应用级合同采用。
- `AppModelsView.vue`、`ModelApiGuideView.vue`、`guide.ts`、`docs/MODEL_API_WORKFLOW.md`：统一职责说明与操作顺序。
- `CarLab/TapLater/packages/taplater/api/_generationDispatch.js`：支持 Gemini 原生图片请求、`{model}` 路径替换、Gemini 图片响应和根级视频 URL。

## 云端数据与配置

### 数据库快照

- 路径：`~/Library/Application Support/CarLabAPI/backups/2026-07-14-pre-contract-ownership-cutover.json`
- SHA-256：`e19a0e51cb4e00b215aec60a7ca5b6af1dbf03e028e8c3e36cef980593d16deb`
- 内容：渠道 6/7/8 的模型与映射、目标模型元数据、ability、渠道商关联和价格选项。
- `contains_secrets=false`；快照不含渠道 `key` 字段。

### Railway PostgreSQL

单事务完成以下更新：

- Channel 6 `DeepWL Text` 新增三个 canonical 模型及上游映射：
  - `deepwl/gemini-3-pro-image -> gemini-3-pro-image`
  - `deepwl/gemini-3.1-flash-image-preview -> gemini-3.1-flash-image-preview`
  - `deepwl/gemini-2.5-flash-image -> gemini-2.5-flash-image`
- 新增 Google 模型元数据，显示名为 `nanoBananaPRO`、`nanoBanana2`、`nanoBanana`，端点均为 `["gemini"]`。
- 每个模型为 `default`、`gold`、`silver`、`bronze` 创建 4 条启用 ability，并记录 `deepwl` 渠道商关系。
- `ModelPrice` 写入 `0.60`、`0.25`、`0.10`，与 DeepWL 公共价格 API 当前值一致。

### Railway 变量

`TASK_PRICE_PATCH` 增加 `deepwl/omni-fast` 和 `deepwl/omni-fast-v2v`，防止固定按次采购价再次乘以视频时长。原有 `fox-grok-video-3` 和 `deepwl/grok-video-3` 保持不变。变量设置使用 `--skip-deploys`，由本轮代码部署统一触发一次新版本。

### 云资源

- Railway 项目 `carlab-api`，服务 `new-api`、PostgreSQL、Redis。
- Cloudflare 入口 `api.carlab.top`；本轮不修改 Worker、DNS 或源站密钥。
- Vercel 项目 `superseed-dash`；按仓库规则由 `CarLab/TapDash` 目录手动生产部署。
- Vercel 项目 `superseed`；由 GitHub 分支推送触发 TapLater 预览/生产策略，不使用遗留 `taplater` 项目。
- BugBoard `bug.cjzz.top`；记录 ID `9d82c6a1` 随 Superseed Git 推送更新。

## 验证证据

- CarLab 前端 `bun run typecheck`：通过。
- CarLab 本轮 5 个模型前端文件定向 `oxlint`：通过。
- CarLab 前端 `bun run build`：通过。
- Go 定向测试 `TestModelOperationContractAcceptsGeminiImageAdapterAndPath` 与 `TestParseConfiguredEndpointTypesSupportsArrayAndObject`：通过。
- Controller 包 `go test ./controller -run '^$'`：编译通过。
- TapDash `npm run build`：通过。
- `_generationDispatch.js` 与 `ai-profiles.js`：`node --check` 通过。
- PostgreSQL 验证：三条 canonical Banana 各有 4 条启用 ability，映射、渠道商、`["gemini"]` 端点和固定价格均正确。
- DeepWL Text Key `/v1/models` 实际返回三条 canonical 上游模型，不依赖 `-c` 兼容模型。
- 首次公网 Quote 验证发现 `gemini-2.5-flash-image` 被默认绑定到 OpenAI Images Profile，而模型元数据只声明 Gemini 端点。DeepWL 官方 Markdown 明确 Gemini 图片统一使用 `/v1beta/models/{model}:generateContent`，因此修正为 Gemini Native Profile，未放宽端点门禁。

全量 `go test ./model ./controller` 仍有两个当前分支既有失败：旧合同测试期望 `unsupported override field`，而基线代码返回 `overrides contains unsupported field`；`TestListModelsTokenLimitIncludesTieredBillingModel` 的 tiered-billing 可见性断言失败。本轮没有修改这两条行为，定向测试与云构建用于隔离本轮回归。

## 回滚

1. 在 TapDash 先关闭受影响应用模型，避免新调用进入变更合同。
2. 使用 Git 回退 CarLab API 与 Superseed 本轮提交，并重新部署 Railway 与 `superseed-dash`。
3. 校验快照 SHA-256 后，恢复 Channel 6 的 `models`、`model_mapping`，删除三条 canonical 模型的 ability、渠道商关联和新增元数据，并恢复 `ModelPrice` 原值。
4. 将 `TASK_PRICE_PATCH` 恢复为 `fox-grok-video-3,deepwl/grok-video-3`，等待 Railway 新部署成功。
5. 回滚后验证 `/api/status`、模型目录、合同读取、TapDash 只读状态和至少一条已知稳定模型调用。

## 剩余风险与权限

- 云端部署 ID、运行实例和浏览器验收将在部署完成后补入本文档。
- 三条 canonical Banana 尚需在新版本上执行最低成本真实生成、Quote 与消费日志对账；在此之前不应进入 Production 模型集合。
- `gpt-image-2` 的实际上游结算可能按 usage 而不是公开固定价，既有审计结论仍有效，不能承诺完全透传成本。
- Omni 两档只完成按次计费保护和合同建立，尚未为本轮支付高成本真实视频测试。
- Seedance 2.0 当前 Key 未授权，仅有未绑定模板，不能宣称已开通或加入生产集合。
- TapDash 仍保留应用合同“采用”操作，这是 Superseed 发布状态变更，不是合同编辑旁路。
