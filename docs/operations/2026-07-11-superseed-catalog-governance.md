# 超级种子模型目录治理与渠道商筛选

## 范围

本轮将 CarLab API 从导入渠道的全量上游模型收敛为超级种子相关白名单，并在管理员模型页与公开模型广场同时增加 API 渠道商筛选。同步补齐保留模型的模型供应商元数据、记录账号用途，并验证清理后仍可按渠道测试模型。

本轮不修改超级种子、TapDash 或 Supabase 代码，不迁移超级种子调用路径，不输出或提交密码、API Key、Token、Cookie 等秘密。

## 假设与关键纠正

- “未知供应商”表示可路由模型缺少 `vendor_id` 模型元数据，不表示渠道未知。渠道归属来自 `channels.channel_provider`。
- TapLater 与 TapDash 当前各有 50 个静态模型且并不完全一致，初始 CarLab 目录另有 15 条；删除前必须取三者并集，不能只按初始 15 条处理。
- New API 的 `ModelPrice`、`ModelRatio` 与 `CompletionRatio` 按模型全局生效。同一模型同时绑定多个成本不同的渠道时，无法同时等于两家渠道原价；必须选主成本渠道、拆渠道限定模型 ID，或增加渠道级成本账本。
- 本轮删除的是导入渠道暴露的非白名单 abilities，不删除 New API 源代码中的默认倍率表。后者是兼容能力，不是当前已启用模型目录。
- 原定价接口会给未配置价格的模型显示兜底倍率 `37.5`，但真实调用层会拒绝该模型，形成“前端看似有价、API 实际不可用”的矛盾。本轮改为仅公开显式配置了价格、倍率或有效分层计费表达式的模型。
- `kling-v3-video-generation` 虽属于超级种子历史目录，但 DMXAPI 已标记下架并要求迁移至 `kling-v3`，因此保留管理元数据且维持禁用，不作为可售模型。

## 变更文件

- `model/ability.go`：定价能力查询同时返回 `channels.channel_provider`。
- `model/model_meta.go`、`controller/model_meta.go`：增加 API 渠道商计数和 `channel_provider`、`status`、`sync_official` 管理筛选。
- `model/pricing.go`、`controller/pricing.go`：定价数据返回渠道商集合，公开接口过滤没有真实计费配置的模型。
- `web/default/src/features/models/`：管理员模型表增加模型供应商和 API 渠道商筛选，默认展示绑定渠道。
- `web/default/src/features/pricing/`：模型广场增加渠道商筛选、渠道商列和详情信息。
- `web/default/src/routes/`、`web/default/src/i18n/locales/`：增加筛选 URL 状态和 7 份语言包文案。
- `docs/catalog/superseed-model-allowlist.json`：记录 65 个超级种子业务模型并定义各标准渠道精确白名单。
- `docs/catalog/superseed-routable-model-catalog.json`：记录当前 28 个可路由模型的模型供应商、渠道商、类型与证据状态。
- `AGENTS.md`：登记本维护记录及对应代码、目录和云资源。

## 云资源与配置

| 资源或配置 | 本轮处理 |
| --- | --- |
| Railway `carlab-api/new-api` | 构建并部署渠道筛选与定价可见性代码 |
| `api.carlab.top` | 验证管理 API、定价 API、后台登录和模型调用 |
| 5 个现有渠道 | 备份后将启用能力收敛到 28 个超级种子模型；空白名单的图片渠道手动禁用 |
| 模型供应商元数据 | 新增 4 个公司级供应商和 16 条模型元数据，修正 `viduq2` 的公司归属 |
| New API 全局价格与分组倍率 | 不在多渠道成本未决时批量覆盖 |
| `theme.frontend` | 云端主题已切换为 `default`，不再使用即将废弃的 `classic` |

主题切换前的无密钥备份：

- 文件：`~/Library/Application Support/CarLabAPI/backups/2026-07-11-pre-default-frontend-theme.json`
- SHA-256：`9b3e109fc71ebec8a758d813785745ac1e1e7b69536a1e478c0b24301bffd314`
- 备份内容：`theme.frontend=classic`，`contains_secrets=false`

## 云端数据操作

清理前通过 Root 管理 API 导出无密钥快照：

- 文件：`~/Library/Application Support/CarLabAPI/backups/2026-07-11-pre-superseed-prune-channels.json`
- SHA-256：`6fe1baee02b79d91f9998d1c9ddb7bebae2d223e1257e8a5f0cd834b507cc386`
- 字段：渠道 ID、名称、状态、类型、渠道商、模型列表、分组；明确不含 Key、Token 或 Cookie。

| 渠道 | 清理前 | 清理后 | 处理 |
| --- | ---: | ---: | --- |
| SiliconFlow Production | 91 | 2 | 保留 SiliconFlow 白名单，并把测试模型改为 `Qwen/Qwen3-VL-32B-Instruct` |
| DMXAPI Production | 640 | 16 | 保留 DMXAPI 白名单 |
| Nodyhub Production | 725 | 20 | 保留 Nodyhub 白名单 |
| VolcEngine Seedance Video | 2 | 2 | 原样保留视频白名单 |
| VolcEngine Production | 1 | 0 个启用能力 | 渠道状态改为手动禁用（状态 2），不删除渠道或密钥 |

四个新增模型供应商为 `Alibaba`、`Black Forest Labs`、`ShengShu AI`、`Tencent`。模型元数据由 15 条增至 31 条，其中 28 条对应当前可路由集合，另外 3 条仍属于 65 模型超级种子业务目录。公开定价中的未知模型供应商已从 513 条降为 0 条。

## 账号清单

| ID | 用户名 | 角色 | 用途 |
| --- | --- | --- | --- |
| 1 | `root` | Root（100） | 平台全局管理 |
| 2 | `superseed-service` | 普通用户（1） | 超级种子服务令牌归属，不用于人工后台管理 |
| 3 | `washedyirgacheffe` | 普通用户（1） | 当前个人账号，默认不能访问管理员模型与渠道页面 |

密码采用不可逆哈希存储，数据库不存在可读取的明文密码。Root 的随机密码只保存在本机 macOS 钥匙串服务 `carlab-api-railway-admin`，不得写入本文档或聊天记录。

平台管理登录方式：

1. 打开 `https://api.carlab.top/sign-in`。`/login` 当前仍会返回前端壳，但不是新前端的规范路由。
2. 用户名填写 `root`。
3. 在本机终端执行 `security find-generic-password -a root -s carlab-api-railway-admin -w` 读取密码并直接登录，不要把输出粘贴到聊天或文档。
4. 登录后通过 `/channels` 管理和逐模型测试渠道，通过 `/models/metadata` 按模型供应商或 API 渠道商筛选，通过 `/system-settings/models` 配置模型价格，通过 `/system-settings/billing` 配置下游分组倍率。

ID 3 的 `washedyirgacheffe` 是普通用户，不能访问上述管理员页面。若要把它升级为管理员，应由 Root 在用户管理中显式改角色；本轮不擅自提权。

## 验证证据

- 前端 `bun run typecheck`、定向 oxlint、7 份 locale JSON、65 模型白名单与 28 模型可路由目录交叉校验全部通过。
- Railway 部署 `d9533046-4f05-41ab-b4b6-e0f14b4fd55e` 曾完成渠道商筛选版本，后被后续部署替换；当前 Railway 活跃部署为 `2a4dea53-81ed-4552-b5b5-f4a8ff97f228`，状态 `SUCCESS`，包含筛选与定价可见性修复。
- `/api/models/search?channel_provider=dmxapi` 返回 16 条，证明管理筛选已接入后端，不是仅在当前分页做前端过滤。
- 渠道模型数最终为 SiliconFlow 2、DMXAPI 16、Nodyhub 20、VolcEngine 视频 2；VolcEngine 图片渠道状态为 2。
- 数据清理后的管理元数据为 31 条，4 类 API 渠道商且未知模型供应商为 0 条；公开 `/api/pricing` 只返回 4 个显式计费模型。
- 现有 `superseed-production` Token 未启用 Token 模型白名单；带令牌访问 `/v1/models` 返回 4 个模型：`gpt-4o-mini`、`gpt-image-1`、`sora-2`、`sora-2-pro`。这是计费配置筛选，不是 Token 权限遗漏；无令牌访问返回 HTTP 401 属于预期保护。
- 使用现有业务 Token 调用 `gpt-4o-mini` 的 `/v1/chat/completions` 返回 HTTP 200 和预期文本 `carlab-final-ok`。
- 浏览器实际打开 `https://api.carlab.top/models/metadata`：管理员页显示“模型供应商”和“API 渠道商”筛选；渠道商菜单显示 `dmxapi (16)`、`nodyhub (20)`、`siliconflow (2)`、`volcengine (2)`。选择 `dmxapi (16)` 后 URL 写入 `channelProvider=["dmxapi"]` 的编码查询参数，表格总计和行数均为 16，绑定渠道列显示 `dmxapi · DMXAPI Production`。
- 云端 `/api/status` 返回 `theme=default`，确认当前前端主题配置已生效。

## 模型调试顺序

1. 在渠道页选择单个渠道和模型执行测试，记录端点类型、HTTP 状态、上游请求 ID、响应耗时和错误正文摘要。
2. 测试通过后再核对该渠道官方价格页、计价单位、币种、输入输出差价、图片规格或视频时长倍率。
3. 只有完成可用性和核价的模型才写入 New API 全局模型价格，并确认 `/v1/models` 出现该模型。
4. 最后配置 OEM 分组倍率和额度，使用独立下游用户 Token 验证账单；不得使用 Root Token 或共享 `superseed-production` Token 作为 OEM 客户凭据。

## 回滚

1. 校验快照 SHA-256 后，从无密钥渠道快照恢复各渠道 `models` 与 `status`；模型更新接口会重建 abilities。
2. Railway 回滚时优先从 Git 分支重新部署：回到 `c46b2b5f` 可保留筛选但撤销定价可见性修复，回到 `41661832` 可撤销本轮筛选代码；平台上 `2a4dea53-81ed-4552-b5b5-f4a8ff97f228` 是当前成功部署，旧部署可能显示为 `REMOVED`，不要把旧 ID 当成可直接恢复的在线服务。
3. 删除本轮新增的 16 条模型元数据和 4 个供应商，或恢复 `viduq2` 原 `vendor_id=17`；本轮未修改任何供应商 Key。
4. 如需恢复全量上游模型，只恢复渠道 abilities，不修改 New API 源码默认倍率。

## 剩余风险与权限

- DeepWL/FOX 和 Wuyinkeji 私有协议尚未接入标准 New API 渠道，相应白名单模型只能保留为待接入目录，不能标记为已验证可用。
- DMXAPI 与 Nodyhub 当前有 10 个同名保留模型重叠。多渠道同模型的真实成本冲突尚未解决，不能把任一渠道价格静默写成全局基准。
- 当前 28 个渠道能力中只有 4 个具备显式 New API 计费配置，且这些配置仍需与实际上游成本复核；本轮不声称价格已与上游完全一致。
- 9 条可路由目录记录只有实时渠道模型目录证据，尚缺公开上游文档；标记为 `channel-catalog` 或 `channel-catalog-only`，不能升级为已核价状态。
- Root 尚未启用两步验证，完成本轮登录后应优先开启 2FA 并保存恢复码。
- 超级种子迁移仍需按文本、图片、视频逐阶段确认，本轮不启动迁移。
