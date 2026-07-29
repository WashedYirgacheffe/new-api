# RE SKU 级报价与任务预扣统一

## 范围

- 以 RE 模型卡片页及 `https://reapi.ai/models` 的公开价格快照为采购价证据，重新生成异步模型价格目录和逐 SKU 表格。
- 让 CarLab `/api/user/models/quote` 与 RE 真实任务预扣使用同一套 SKU 选择逻辑，按用户实际选择的尺寸、质量、清晰度、素材模式、音频开关、时长和生成数量计价。
- 让 TapLater 报价复用真实派发字段映射，把参考图片、视频等素材请求字段一并传给 CarLab，避免文生与参考素材模式选到同一价格。
- 本轮未修改数据库结构；已将 88 个具备精确合同的 RE Async 模型基础价写入生产 `ModelPrice`，并通过 GitHub 自动部署发布运行时 SKU 逻辑。

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
- 代码提交 `4e1058eb47b8fd88d009b7144697b60ae90c0eeb` 已推送到 `WashedYirgacheffe/CarLabAPI` 的 `main`。Railway GitHub 自动部署 `a8e4e898-9557-4cee-8ccd-2955404d9bd2` 成功后，使用同一提交重启部署 `a085bca7-4a9d-444c-92ef-83b26ff5a47f`，使运行实例重新加载价格表。
- 使用运行环境中的 `REAPI_TASK_API_KEY` 完成 onboarding，但没有输出密钥原文。生产写入仅涉及 `ModelPrice`、RE 渠道/能力开关和既有 RE 模型合同元数据刷新。

## 生产写入与备份

- 写入前 `ModelPrice`：MD5 `af107ee8347c21eaf314131550f0ff93`，长度 `21073`，RE 基础价 88 条。
- 写入后 `ModelPrice`：MD5 `d4f0ba13c90dbb502ff893d878c3b4e4`，长度 `21073`，RE 基础价 88 条，逐项与本目录一致。
- RE Async 渠道为 ID `9`、类型 `59`、状态 `1`；已启用 88 个模型和 176 条分组能力记录。7 个 deferred billing 模型仍未启用。
- 生产写入前备份目录为 `/Users/washed/.codex/backups/carlab-reapi-pricing-20260729-104205`，所有文件权限为 `0600`：`model-price-full.json` SHA-256 `06da33f4cd5979db6db176442b5588b6fad4239d8ca8c28f8a69e920e27edde9`，`model-price-re-only.json` SHA-256 `60779abbfb511c5917bba01f72c2c6a3d19ed1a38c505fdedfe5ae4355d16593`，`re-channel-before.csv` SHA-256 `4ee65d2ffb93919ce621497d37ca0714869fcc7a525d39339d45a37b576360b3`。

## 验证证据

- `go test ./relay/channel/task/reapi`
- `go test ./relay/channel/task/reapi ./controller ./relay -run 'Quote|RE|ReAPI|TaskAdaptor|RelayTask|ModelOperation|PricingSKU'`
- `go test ./cmd/reapi-pricing-snapshot ./cmd/reapi-onboard ./relay/helper -run 'ReAPI|Pricing|Contract|Quote|ModelOperation|Billing'`
- TapLater：`node --test tests/generation-material-contract.test.mjs`
- 两个仓库执行 `git diff --check`。
- 价格目录与运行时内嵌文件 SHA-256 一致；CSV 共 389 行（表头 1 行、SKU 数据 388 行）。
- 在干净生产基线 `35f012a8` 上 cherry-pick 后，`go test ./relay/channel/task/reapi`、onboarding/helper 定向测试通过。`controller` 中既有 `TestListModelsTokenLimitIncludesTieredBillingModel` 在基线和集成分支均失败，未由本轮引入。
- 生产数据库在同一 C 排序规则下与价格目录逐项一致（88/88）；`/api/status` 和 Railway healthcheck 成功。
- 生产 token 报价：Gemini 3.1 Flash Image Preview 2K 命中 `gemini-3.1-flash-image-preview:2k`，`$0.0477/张`；Seedance 2.0 720p 参考图命中 `doubao-seedance-2.0:720p:ref`，`$0.10384/秒`，5 秒总价 `$0.5192`；Wan 2.7 Video 720P 命中 `wan2.7-video:720P`，`$0.07304/秒`，5 秒总价 `$0.3652`。

## 回滚

1. 若需立即止损，在 CarLab 管理端停用 RE Async 渠道 ID `9`，避免新请求继续进入 RE；已有任务和账单日志保持原样。
2. 从上述受限备份恢复 `model-price-full.json` 到生产 `options.ModelPrice`，并使用 `re-channel-before.csv` 恢复渠道的模型列表、状态、分组、优先级和权重；恢复后重启 Railway `new-api` 使内存价格表重新加载。
3. 回退 `4e1058eb` 及后续本轮文档提交，通过 GitHub 推送 `main` 触发 Railway 自动部署；不要日常使用 Railway 直推镜像。
4. 本轮没有数据库 schema migration。模型和合同记录为幂等刷新；若需要恢复到写入前的历史合同版本，应使用对应的先前 Git 目录与既有版本化合同记录，而不是删除生产数据。
5. Vercel 前端如需立即回档，在 Dashboard 将上一个 Git 对应部署 Promote 为 Production。

## 剩余风险与权限

- RE 会持续调整模型、SKU 和价格；重跑快照后必须审查源版本、哈希和价格 diff，再更新运行时内嵌目录。
- 上游允许但价格快照没有对应 SKU 的组合会返回明确报价错误，不会回退到较低采购价；需等 RE 页面提供可审计价格后再开放。
- 已完成报价和数据库验收；真实上游付费任务与后续账单对账仍应在低风险模型和受控额度下单独执行。
