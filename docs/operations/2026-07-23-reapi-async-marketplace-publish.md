# RE Async 模型广场发布

## 范围

- 只发布 RE 的图片、视频、音频和异步文本工具模型，不创建、不启用 RE Chat。
- 从 RE 公开模型页的 `window.__pricing_snapshot__` 固化 96 条 Async 价格证据；当前 95 条有可调用 SKU，`seedance-2.5` 仍为 Coming Soon。另有 5 个按输入音频分钟/音轨和 2 个按输入视频时长计费的工具缺少可信预估量，因此本轮先发布 88 条，其余保留元数据和路由但不开放。
- 新增显式 `go run ./cmd/reapi-onboard --publish-async` 开闸流程。普通 onboarding 重跑不会启用新渠道，也不会改变已存在渠道的状态、模型集合或映射。

## 假设与定价边界

- `REAPI_TASK_API_KEY` 已在 Railway `new-api` 服务中配置；本轮不需要 `REAPI_CHAT_API_KEY`。
- `base_price_usd` 使用当前模型匹配 SKU 的最高非零采购单价，避免宽松参数透传时选到更高 SKU 却仍按最低价预扣。
- RE 适配器对明确的每秒、按图片数量和按字数模型追加请求倍率，并限制可计费的图片数量和显式时长；每秒计费模型必须显式传递 `duration` 或 `seconds`。按开始分钟、音轨数或输入媒体时长计费的 7 个工具保持停用，等待真人调用样本后再补完成态结算。
- 模型广场公开价格代表当前 CarLab 基础计费配置，不替代上游各 SKU 的完整参数报价表。

## 变更文件

- `docs/catalog/reapi-async-pricing.json`：RE Async 价格、单位、SKU 和公开来源快照。
- `cmd/reapi-pricing-snapshot/`：从公开模型页可重跑生成价格目录。
- `cmd/reapi-onboard/`：价格目录校验、只发布非 Chat 且当前可用的 Async 模型、合并 `ModelPrice`、启用模型与渠道。
- `relay/channel/task/reapi/`：视频时长、图片数量和异步文本字数计费倍率及边界校验。

## 云资源与配置

- Railway 项目：`carlab-api`
- 环境：`production`
- 服务：`new-api`、`Postgres`
- 配置名：`REAPI_TASK_API_KEY`、`SQL_DSN`、`ModelPrice`
- 生产渠道：`RE Async`
- 公网入口：`https://api.carlab.top/v1/re/generations`、`https://api.carlab.top/v1/re/tasks/:task_id`

所有密钥只由 Railway 环境注入，不写入 Git、命令输出或本文档。

## 发布步骤

1. 提交并推送 CarLabAPI `main`，等待 Railway `new-api` 部署成功。
2. 从 Railway `Postgres` 读取 `DATABASE_PUBLIC_URL` 到本机临时环境变量，通过 `railway run --service new-api --no-local` 注入已有 `REAPI_TASK_API_KEY`，执行 `go run ./cmd/reapi-onboard --publish-async`。
3. 重启 `new-api`，让运行中实例重新加载渠道缓存、能力和 `ModelPrice`。
4. 检查 `/api/pricing`：应出现 88 条 `re/*`，且不含 Chat、`re/seedance-2.5` 与 7 条 `deferred_billing` 模型。
5. 使用受控 Token 提交一条低成本图片任务并轮询公开任务 ID，确认上游任务 ID 未泄露。

## 验证

- `go test ./relay/channel/task/reapi ./cmd/reapi-onboard ./cmd/reapi-pricing-snapshot -count=1`
- `go run ./cmd/reapi-onboard --dry-run`
- 目录集合校验：96 Async、95 upstream-publishable、88 CarLab-ready、7 deferred billing、1 Coming Soon、8 Chat excluded、所有基础价格为有限正数。
- `git diff --check`

## 回滚

1. 在 `api.carlab.top` 渠道管理中手动停用 `RE Async`，abilities 会同步停用，模型立即退出可路由集合。
2. 如需从模型广场撤下元数据，将对应 `re/*` 模型状态设为停用；保留价格快照和渠道密钥，便于恢复。
3. 回滚 Git 提交并推送 `main`，等待 Railway 自动部署；不要删除其他渠道的 `ModelPrice`。

## 剩余风险

- RE 价格和可用模型会变化；重跑快照生成器后必须审查 diff，再执行发布命令。
- `seedance-2.5` 上游正式提供 SKU 前不会进入模型广场。
- 本轮只保证直接 RE Async API 与公开模型广场，不包含 CarLab 内置 Playground 或 TapLater 合同表单接线。
- 7 个输入媒体动态计费工具需要在真实样本返回可审计用量后继续收口，完成前不会进入模型广场。
