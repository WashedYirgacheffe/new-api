# RE SKU 级报价与任务预扣统一

## 范围

- 以 RE 模型卡片页及 `https://reapi.ai/models` 的公开价格快照为采购价证据，重新生成异步模型价格目录和逐 SKU 表格。
- 让 CarLab `/api/models/quote` 与 RE 真实任务预扣使用同一套 SKU 选择逻辑，按用户实际选择的尺寸、质量、清晰度、素材模式、音频开关、时长和生成数量计价。
- 让 TapLater 报价复用真实派发字段映射，把参考图片、视频等素材请求字段一并传给 CarLab，避免文生与参考素材模式选到同一价格。
- 本轮不修改数据库结构、不写入生产 `ModelPrice`、不部署 Railway 或 Vercel。

## 假设与价格边界

- 上游证据版本为 `1785200182468`，源快照 SHA-256 为 `ff799d92fbc45e7633efa8996a6bd6c07be6aee9a7ca76e8734b8cebd3603a36`。
- 目录包含 96 个 RE Async 模型，其中 95 个当前有上游 SKU、1 个 Coming Soon；上游快照共 476 个 SKU，当前目录匹配 386 个唯一上游 SKU。按 CarLab 模型身份展开后导出 388 条价格行，复用同一上游 SKU 的模型别名分别保留。
- `sku_unit_price` 表示 RE 公布的 USD 采购单价；`estimated_amount` 仍会叠加请求数量和 CarLab 生效分组倍率，是最终报价字段。模型广场 `base_price` 继续使用安全上界，不代表用户当前参数组合的实际报价。
- 7 个依赖输入媒体真实时长、分钟或音轨数量的模型仍保持 deferred billing，不因本轮价格表而开放。

## 变更文件

- `docs/catalog/reapi-async-pricing.json`：最新 RE Async 价格证据和完整 SKU 列表。
- `docs/catalog/reapi-async-pricing-table.csv`：按模型身份展开的 388 条 SKU 价格行。
- `relay/channel/task/reapi/pricing/reapi-async-pricing.json`：运行时内嵌的同版本价格目录。
- `relay/channel/task/reapi/pricing_catalog.go`：SKU 匹配、采购单价覆盖和数量倍率；覆盖图片、视频、文本工具、参考素材、音频和 Midjourney/Veo 复合维度。
- `relay/channel/task/reapi/pricing_catalog_test.go`：代表性精确 SKU、数量倍率和不支持组合回归。
- `controller/model_catalog.go`：报价参数保留素材字段，并返回匹配 SKU、采购单价、数量和来源页。
- `relay/relay_task.go`、`relay/channel/task/reapi/adaptor.go`：真实任务预扣复用 SKU 价格，修正文本工具单位。
- Superseed TapLater `api/_generationBilling.js`、`api/_generationDispatch.js` 与 `tests/generation-material-contract.test.mjs`：报价和派发复用同一请求字段映射。

## 云资源与配置

- 关联资源：Railway `new-api`、PostgreSQL、`api.carlab.top`，以及 Vercel `superseed`。
- 关联配置名：`ModelPrice`、RE Async 渠道、`REAPI_TASK_API_KEY`、CarLab 分组倍率。
- 本轮未读取、写入或输出任何密钥；未更改以上云资源和配置。

## 验证证据

- `go test ./relay/channel/task/reapi`
- `go test ./relay/channel/task/reapi ./controller ./relay -run 'Quote|RE|ReAPI|TaskAdaptor|RelayTask|ModelOperation|PricingSKU'`
- `go test ./cmd/reapi-pricing-snapshot ./cmd/reapi-onboard ./relay/helper -run 'ReAPI|Pricing|Contract|Quote|ModelOperation|Billing'`
- TapLater：`node --test tests/generation-material-contract.test.mjs`
- 两个仓库执行 `git diff --check`。
- 价格目录与运行时内嵌文件 SHA-256 一致；CSV 共 389 行（表头 1 行、SKU 数据 388 行）。

## 回滚

1. 回退本轮 CarLabAPI 和 TapLater Git 变更并通过各自既有 GitHub 自动部署流程发布；不要手工修改生产数据库价格记录。
2. 若仅需暂时止损，在 CarLab 管理端停用 RE Async 渠道或相关模型，避免新请求继续进入 RE。
3. 已产生的任务和账单日志保持原样，不执行删除或反向迁移；本轮没有数据库迁移需要回滚。
4. Vercel 前端如需立即回档，在 Dashboard 将上一个 Git 对应部署 Promote 为 Production。

## 剩余风险与权限

- RE 会持续调整模型、SKU 和价格；重跑快照后必须审查源版本、哈希和价格 diff，再更新运行时内嵌目录。
- 上游允许但价格快照没有对应 SKU 的组合会返回明确报价错误，不会回退到较低采购价；需等 RE 页面提供可审计价格后再开放。
- 生产发布、真实付费请求和账单对账尚未在本轮执行，需要拥有 Railway、CarLab 管理端和 Vercel 权限的操作者完成。
