# RE Seedance 2.0 合同与价格校准

## 范围

- 仅校准 RE 渠道当前已发布的 `doubao-seedance-2.0-face`、`doubao-seedance-2.0-fast-face` 和 `seedance-2.0-mini`。
- 以具体模型 API reference 和用户确认的模型页价格表作为本轮最高优先级证据，覆盖旧版前端 manifest 和公开价格快照中的冲突值。
- TapLater 不新增供应商参数覆盖；发布后继续从 CarLab `effective_contract` 获取参数、素材槽位和默认值，从报价接口获取实际组合价格。
- 本轮不执行付费上游生成；代码和合同已发布生产并完成只读验证，合同证据等级仍为 `documented`。

## 假设与边界

- Face、Fast Face 和 Mini 均支持 `nsfw_checker:boolean`，合同保留高级开关，默认值沿用上游配置 `true`。
- Face 支持 `480p/720p/1080p/4k`，Fast Face 和 Mini 仅支持 `480p/720p`；Face 与 Fast Face 默认 `480p`，Mini 默认 `720p`，合同枚举与现有价格 SKU 完整闭合。
- “与上传视频”价格只在 `video_urls` 或 `reference_video_urls` 存在时使用。图片、首尾帧和音频素材均按无视频价格计费。
- Face 使用 `size`，Mini 使用 `aspect_ratio`；三者时长均为 4 到 15 秒。所有素材只允许公开 HTTP(S) URL，不接受 Base64 或 data URI。
- 旧 RE 全量快照抓取链的固定 manifest URL 已漂移；本轮不扩大为 88 个模型的无关重抓，而是在现有生成器末端应用可测试的三模型权威覆盖。

## 变更文件

- `cmd/reapi-contract-snapshot/seedance_overrides.mjs`：三模型的文档级最终合同、素材模式白名单，以及证据来源和 `documented` 状态。
- `cmd/reapi-contract-snapshot/main.mjs`：在通用快照提取后应用 Seedance 权威覆盖。
- `cmd/reapi-contract-snapshot/seedance_overrides.test.mjs`：参数枚举、素材模式、`tools` 类型和 `nsfw_checker` 模型差异测试。
- `cmd/reapi-pricing-snapshot/main.go`、`main_test.go`：三模型人工价格覆盖及精确费率测试。
- `docs/catalog/reapi-async-contracts.json`：TapLater 最终消费的三模型参数与素材合同。
- `docs/catalog/reapi-async-pricing.json`、`reapi-async-pricing-table.csv`：采购价格证据和可筛选 SKU 表。
- `relay/channel/task/reapi/pricing/reapi-async-pricing.json`：报价与真实任务预扣共用的运行时价格目录。
- `controller/model_catalog.go`：报价在模式解析后执行同一套材料合同校验，不对非法素材组合返回可用报价。
- `relay/channel/task/reapi/adaptor_test.go`、`pricing_catalog_test.go`、`cmd/reapi-onboard/main_test.go`：`nsfw_checker=false` 出站保留、视频素材折扣、时长倍率、Fast Face 高分辨率拒绝和素材模式回归。

## 云资源与配置

- 关联资源：Railway `new-api`、CarLab PostgreSQL、`api.carlab.top`，以及消费合同的 Vercel `superseed`。
- 关联配置：`ModelPrice`、RE Async 渠道、模型 Operation Binding revision、`REAPI_TASK_API_KEY`。
- 不新增环境变量，不修改数据库 schema，不在文档、日志或提交中记录任何密钥。
- 代码通过 Git 推送触发 CarLab 部署；生产价格从本轮目录加载，三个 Seedance Binding 通过短事务定向同步，未再次运行全量 onboarding。

## 生产发布与恢复

- 生产提交为 `ac98b0626b545c3541591c753c9cc3bf45652a3a`；Railway `new-api` 部署 `83b10be1-7cad-425b-b6ff-91fdf21741f8` 状态为 `SUCCESS`，实例为 `RUNNING`，`https://api.carlab.top/api/status` 返回 HTTP 200。
- 首次生产同步时，全量 onboarding 两次均被本地前台命令约 30 秒的执行上限中断，曾暂时留下 RE 渠道禁用的中间状态。随后已恢复 RE 渠道 `9` 为 `status=1`，并恢复该渠道 `176/176` 条 ability 为启用状态。
- 恢复后仅对三个目标 Binding 执行短事务定向同步；第二次复跑保持幂等，三者最新 revision、`contract_version` 均仍为 `3`，没有新增 revision。
- 最终 Binding hash：Face 为 `28975370d3d45b4d8efa044b552121b5552b75864887169c2557d803bc0d8c32`，Fast Face 为 `29af149f129e48576d095f8e2714d68b3468aaac9bb8c98d44722128d7f403fc`，Mini 为 `02085e902d835bab3fb4ae3c9715e1c58c7b01ec5539705629a837b15ee1c9a6`。
- 写入前备份位于 `/Users/washed/.codex/backups/carlab-reapi-seedance-20260803-q6a3hn`；目录内 6 个文件权限均为 `0600`。

| 备份文件 | SHA-256 |
| --- | --- |
| `model-price-full.json` | `a2703b503f5f0cf0a31320f5a8d3c9a3f34e72b60dc848f902f13f7b1d496b44` |
| `model-price-re-only.json` | `f78d3ff142872c93bfacac918a327505cbf8b74718229b5944da1d9e688a910a` |
| `re-abilities-before.json` | `b3ddb0354359f4a7278b36253ee90fc9e140416a5f4ce5a0432a23dca42dd2f9` |
| `re-channel-before.csv` | `4ee65d2ffb93919ce621497d37ca0714869fcc7a525d39339d45a37b576360b3` |
| `seedance-binding-revisions-before.json` | `56fa170b7d267d08232922bb1371d3090db53c1f131e4ca2594930f3ac3fafa7` |
| `seedance-bindings-before.json` | `4535a9dd19e2ff78ec754433f20916011dc7095967db2485a2688fc2a23a084e` |

## 验证证据

- `node --test cmd/reapi-contract-snapshot/seedance_overrides.test.mjs`
- `go test ./relay/channel/task/reapi ./cmd/reapi-onboard ./cmd/reapi-pricing-snapshot -count=1`
- `go test ./controller -run 'TestProfileDispatchReadyAcceptsREAsyncContracts|TestNormalizeModelQuoteParameters' -count=1`
- 合同断言：三个模型均有 `nsfw_checker`；Face 开放 4K 且 `tools` 为 boolean；Fast Face 只开放 480p/720p，默认 480p，并在报价层拒绝 1080p/4K；Mini 无 `return_last_frame`。
- 素材断言：Face/Fast Face 仅允许图片、首尾帧、视频、音频的已证明组合；音频不能单独使用，首尾帧不能与普通图片、视频或音频混用。Mini 的首尾帧与任意 reference 素材互斥。报价与真实派发都使用同一合同模式和材料校验。
- 报价断言：图片素材保持 `noVideo`；视频素材命中 `withVideo`；数量倍率按 4 到 15 秒中的实际 duration 计算。
- 生产业务 Token 合同接口确认：Face 的分辨率为 `480p/720p/1080p/4k`，Fast Face 和 Mini 均仅为 `480p/720p`；三者均返回 `nsfw_checker:boolean` 且默认开启。Fast Face 使用 `1080p` 的生产报价按合同拒绝。
- 生产认证报价确认 5 秒无视频 SKU：Face 4K 为 `$0.936/second`、总额 `$4.68`；Fast Face 720P 为 `$0.149/second`、总额 `$0.745`；Mini 720P 为 `$0.098/second`、总额 `$0.49`。
- 生产认证报价确认 5 秒上传视频 SKU：Face 4K 为 `$0.576/second`、总额 `$2.88`；Fast Face 720P 为 `$0.09/second`、总额 `$0.45`；Mini 720P 为 `$0.06/second`、总额 `$0.30`。以上仅调用报价接口，不提交生成任务、不产生上游生成费用。
- 文档价格目录与运行时内嵌目录 SHA-256 必须一致；CSV 必须保持 389 行（表头 1 行、SKU 388 行）。
- 未执行付费上游生成，因此不把本轮状态标记为 `tested`。
- `go test ./...` 中本轮相关包均通过；全仓仍有两个既有基线/环境失败：根包缺少未构建的 `web/classic/dist`，`controller.TestListModelsTokenLimitIncludesTieredBillingModel` 未发现其测试模型。后者已在 2026-07-29 的运维记录中登记，并非本轮引入。

## 回滚

1. 立即止损时先在 CarLab 管理端停用受影响的 RE 模型或整个 RE Async 渠道，阻止新任务进入。
2. 回退本轮 Git 提交并重新部署 CarLab；不要通过当前本地前台工具再次运行全量 onboarding，避免命令中断后留下渠道禁用的中间状态。
3. 若只回退合同，依据写入前备份用短事务定向恢复三个 Operation Binding；保留 Binding 历史，不删除 revision。
4. 若只回退价格，依据 `model-price-full.json` 恢复完整 `ModelPrice`，并重启 CarLab 使内存价格表重新加载；渠道或 ability 异常时分别用 `re-channel-before.csv` 和 `re-abilities-before.json` 定向恢复。
5. TapLater 不含本轮供应商参数代码；CarLab 回退后刷新目录即可恢复旧 `effective_contract`，无需单独回退前端。

## 剩余风险

- 上游若新增 Fast Face 的 1080P 或 4K，必须同时取得两种视频素材模式的价格证据，再通过新合同版本开放。
- 本轮将 Face/Fast Face 无素材时的 `prompt` 条件必填收敛为默认模式；Mini 按权威 API Reference 保留可选 `prompt`。上游若新增未记录的素材组合，合同会 fail-closed，直到取得证据后新增模式。
- 上游模型页、manifest 和价格快照仍可能继续漂移。定时检查只能生成差异，不得自动覆盖本轮人工审查后的合同或价格。
- 本次验证发现 RE 公开全量价格快照已由基线的 476 个 SKU 漂移到 499 个 SKU。未将其余模型的未审计价格变化混入本轮；下次全量 RE 审计应单独复核该差异后再重生成整个目录。
- 真实付费调用和账单对账尚未执行；后续若进行烟测，应使用受控额度并保留任务、报价和结算证据。
