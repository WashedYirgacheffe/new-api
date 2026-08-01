# 三渠道上游合同审计与合同模式收口

## 范围

本轮在 `codex/oem-api-hub` 分支完成 CarLab 合同模式和 TapLater 合同消费的收口，并建立 RE、DeepWL、NodyHub 的文档证据审计产物。目标是让 `effective_contract` 成为参数、素材、报价和派发的唯一事实源；本轮不执行付费烟测、不写入生产模型或价格配置。

## 权威资料与假设

- RE：`https://reapi.ai/zh/models`、具体模型页、`docs/catalog/reapi-async-contracts.json`、`docs/catalog/reapi-async-pricing.json`。
- DeepWL：`https://doc.deepwl.cn/`、`https://doc.deepwl.cn/llms.txt`、`https://zx1.deepwl.net/pricing` 和公开 `https://zx1.deepwl.net/api/pricing`。
- NodyHub：用户提供的 `NODYHUB_API_DOC (1).md` 是提交/轮询/素材传输协议的权威文档，`https://nodyhub.com/pricing` 是价格入口，`https://nodyhub.com/v1/models` 只作为目录证据。
- 证据优先级：具体模型 API 文档 > 同端点 OpenAPI > 渠道通用文档 > CarLab 合同 > TapLater 本地覆盖。
- 当前无 token-scoped 生产目录导出，因此 DeepWL 公开价格返回和 NodyHub 目录计数不能冒充逐模型发布合同；缺证据条目标记 `missing_documentation` 并保持 fail-closed。

## 变更文件与产物

CarLab：

- `model/model_operation_contract.go`：模式条件、互斥/顺序素材字段和 `dispatch_model` 合同校验及 hash。
- `model/model_operation_profile.go`：Omni/Grok 条件模式默认合同和旧覆盖迁移。
- `relay/helper/model_contract_pricing.go`、`controller/model_catalog.go`：报价/目录使用有效合同模式。
- `cmd/contract-audit/main.go`：可重复生成三渠道 JSON/CSV 审计快照。
- `cmd/contract-audit/export_xlsx.mjs`：使用 artifact-tool 生成可筛选 XLSX，并检查公式错误和各 sheet 渲染。
- `docs/catalog/reapi-published-contracts.{json,csv}`、`deepwl-published-contracts.{json,csv}`、`nodyhub-published-contracts.{json,csv}`：本轮审计基线。

TapLater：

- `api/_generationContractMode.js`、`api/_generationModelVariant.js`、`api/_generationBilling.js`、`api/_generationDispatch.js`、`api/_generationProfile.js`、`api/model-catalog.js`：从 CarLab 合同模式解析报价、派发和隐藏变体。
- `api/_canonicalModelContracts.js`、`src/features/canvas/model-contract/{canonicalModelContracts,effectiveModelContract}.ts`：canonical 仅负责展示，不再覆盖供应商参数/素材合同。
- 对应合同、目录、报价和派发测试。

## 云资源与配置

- CarLab API：`https://api.carlab.top`，生产服务为 Railway `carlab-api` / `new-api`。
- TapLater：Vercel 项目 `superseed`，Root Directory 为 `CarLab/TapLater/packages/taplater`，通过 GitHub 自动部署；本轮没有执行 `vercel deploy`。
- 审计命令只读公开 DeepWL 价格接口和本地证据文件；不会读取、打印或保存数据库密码、API key、publish key、service role key 或 session cookie。

## 验证证据

- `go test ./model ./relay/helper -count=1` 通过。
- `go test ./cmd/contract-audit` 通过；生成 RE 88 行、DeepWL 123 个公开价格候选行/67 个 CarLab 已发布计数、NodyHub 19 个文档模型行加 707 个剩余目录模型聚合行（目录总数 726）。
- artifact-tool XLSX 检查通过：Summary 关键范围可读，公式错误扫描为 0，各渠道 sheet 均完成渲染检查。
- TapLater `npm run test:canvas` 为 `291/291`，`npm test` 为 `493/493`；`npx vue-tsc --noEmit`、`npm run build` 和两仓库 `git diff --check` 通过。构建只保留既有 chunk 大小和 Browserslist 提示。
- 全量测试中修正了一个既有 migration 文件名断言：测试引用改为仓库实际存在的 `20260711000000_spark_learning_library.sql`，未修改迁移内容。
- NodyHub 协议快照保存本地 Markdown SHA-256；未把协议示例模型当作 726 个已发布模型合同。

## 回滚

1. 代码回滚：分别回退本轮 CarLab/TapLater Git 提交；不修改数据库迁移和用户媒体。
2. 合同回滚：恢复历史 Binding revision、`contract_version/hash`，让旧报价继续按旧 hash 校验。
3. 线上回滚：通过 Vercel Deployments Promote 历史部署；Railway 使用历史稳定部署，不执行未审查的模型/价格批量写入。
4. 审计产物回滚：删除或恢复对应 JSON/CSV/XLSX 文件即可，不影响运行时合同。

## 剩余风险与权限边界

- DeepWL 公开价格接口当前返回 123 个候选，而 CarLab 证据只确认 67 个已发布命名空间；需要 token-scoped 目录导出才能逐模型补齐 profile/binding 身份。
- NodyHub 权威 Markdown 已覆盖协议，但没有提供匿名逐模型合同和可程序化价格 SKU；726 个模型保持缺证据，不得自动上线。
- 定时抓取只应生成差异报告，必须经过人工审核、生成新合同版本和 Preview 验收后才能发布。
- 需要 CarLab 生产 token-scoped 目录读取权限的后续操作由有权限的运维人员执行；本轮不请求或保存任何新凭据。
