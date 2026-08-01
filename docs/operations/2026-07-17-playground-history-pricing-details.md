# Playground 付费验收、生成历史与模型详情维护记录

- 日期：2026-07-17
- 状态：代码、Railway 发布、真实付费验收、刷新恢复和放大预览均已完成；Omni 实际时长偏差与凭据轮换待处理
- 分支：`codex/oem-api-hub`
- 基线提交：`5e2375de`
- 功能提交：`2903af0c`
- 当前提交：`0509c550`

## 本轮目标

本轮收口三个用户可感知功能：Playground 的图片、视频生成继续走真实钱包或订阅计费；每次生成结果保存为用户自己的历史记录并可在刷新后恢复；公开 `/pricing` 模型详情增加“模型参数”入口，管理员可在原详情抽屉中直接管理参数合同与候选路由。

## 最终验收结论

- Playground 图片和视频均完成一次真实付费生成，Quote、钱包余额变化和消费日志一致，没有重复生成或重复扣费。
- 图片和视频记录均在刷新后恢复，且可再次打开大尺寸预览；验收记录未删除，保留为线上证据。
- `/pricing` 当前线上版本已显示“概览 / 性能 / 模型参数 / API”四个标签。管理员进入“模型参数”后可维护参数合同、证据、修订和候选路由。
- 视频第一次轮询误用了响应中的内部数字 `id`；`0509c550` 已修复为优先使用公开 `task_id`，并复用原已付费任务恢复成功。
- DeepWL 官方 Omni 文档确认正确字段为字符串 `seconds`，推荐范围为 4 到 30 秒。当前代码和回归测试均会向上游发送 `"seconds":"4"`；本次结果媒体仍为 `10.005` 秒，应视为待上游核实的参数遵循问题，不能擅自改成 `duration`。

## 宏观功能与实现逻辑

### 1. Playground 付费生成保持原有结算链路

- 生成前仍先调用 CarLab Quote，页面展示当前合同版本、预计额度和预计金额。
- Quote 成功后先创建一条 `pending` 生成记录；只有记录创建成功才会调用图片或视频 Relay，避免已经付费却完全没有产品记录。
- 每次付费 Relay 使用生成记录 ID 派生稳定 `Idempotency-Key`，由现有 Relay 继续向上游传递；不会自动重试付费请求。
- 图片、视频 Relay、预扣、结算、退款和消费日志继续使用既有实现。本轮历史表不参与余额运算，也不能修改账单。
- 页面不会因为历史写入失败而重复发起付费生成；历史更新失败会明确提示“生成请求未重复”。
- 网络中断导致结果不确定时保留 `pending`，不伪报模型失败；页面会阻止直接再次付费，要求先核对消费日志或确认后删除待处理记录。

### 2. 图片、视频生成历史

- 新增 `playground_generations` 与 `playground_generation_assets` 两张表，分别保存生成历史和自有图片资产元数据。
- 记录模型、分组、提示词、参数快照、合同 hash/version、价格版本、报价额度/金额、视频任务 ID、状态、结果地址和时间。
- 新增 Session `UserAuth` API：
  - `GET /pg/generations`
  - `POST /pg/generations`
  - `PATCH /pg/generations/:id`
  - `DELETE /pg/generations/:id`
- 创建接口永远建立 `pending` 记录，不接受客户端伪造完成态；更新和删除均校验记录所有者。
- 完成态不可被后续请求改写，重复提交完全相同的完成态允许幂等返回。
- 视频提交后立即保存 task ID；刷新页面后，前端最多并行恢复 3 条待完成视频的状态轮询，不会再次创建上游任务或再次扣费。
- 每用户最多保留 500 条历史；历史创建和媒体上传均有限流。用户行锁串行保护历史条数和 200 MB 自有媒体额度，避免不同生成记录并发突破上限。

### 3. 历史查看与放大

- 图片、视频 Playground 顶部显示最近 20 条同类型生成记录，包含状态、模型、提示词、报价金额和时间。
- 当前生成结果及历史缩略图均可打开 90vw × 90vh 预览层；图片和视频使用 `object-contain` 保留原比例，视频保留浏览器媒体控制。
- 历史支持手动刷新和单条删除；生成进行中时阻止删除当前记录。
- 上游 HTTP(S) 结果保存地址快照；Base64 图片先做 MIME 与内容探测，解码后写入 Railway 私有持久卷，数据库只保存 MIME、大小、SHA-256 和相对路径，不保存 Base64 正文。
- 自有图片每张最多 20 MB、每次最多 16 个输出、每用户最多 200 MB。文件先写同目录临时文件，完成同步和哈希校验后再原子发布，避免并发读取半成品。
- 自有图片 GET 使用登录 Cookie 或 API Token 鉴权并再次校验用户所有权，不依赖媒体标签无法携带的 `New-Api-User` 头；响应使用 `private, no-store`，避免跨账号复用浏览器私有缓存。
- 图片归档失败只提示“生成成功、历史归档失败”，不会把已经成功并付费的模型请求标记为生成失败。

### 4. `/pricing` 模型详情中的参数治理

- 模型详情主标签由“概览 / 性能 / API”扩展为“概览 / 性能 / 模型参数 / API”。
- 所有访问者可查看公开模型目录中的输入/输出模态、上下文、输出上限、参数规模和端点摘要。
- 管理员进入“模型参数”后可直接使用既有“参数合同”和“候选路由”工作区，执行参数增删改、发布版本、证据维护、修订回滚和候选路由 CRUD。
- 普通用户和匿名访问者不会挂载管理员工作区，也不会请求 `AdminAuth` API。
- 管理工作区使用懒加载，避免增加公开详情首屏包体。

## 假设与边界

- CarLab Profile、Binding、有效合同和 Quote 仍是参数与采购价格真相源。
- 账单、消费日志和订阅/钱包流水仍是实际结算真相源；生成历史只保存产品体验快照。
- `model_routes` 仍是候选配置，尚未接入真实 Relay failover。本轮只让入口更容易找到，不改变线上路由。
- 上游 HTTP(S) 结果地址仍可能带有效期；本轮只把 Base64 图片转存到自有卷，不主动下载第三方 HTTP(S) 文件。
- Railway 持久卷是单服务、单区域文件存储，不是对象存储或 CDN；`new-api` 继续保持单副本。多副本或跨区域前必须迁移到 R2/S3 一类私有对象存储。
- CarLab API 当前 Railway 项目只有 `production` 环境，没有独立 Preview 服务。代码发布仍按既有功能分支部署到 `new-api`，真实付费验收采用单次、最低成本请求，禁止自动重试。

## 变更文件

- `model/playground_generation.go`、`model/playground_generation_asset.go`、`model/main.go`：生成历史、自有资产元数据、跨数据库 JSON 边界、用户级容量串行校验和 GORM 自动迁移。
- `service/playground_generation_asset.go`：Base64 图片探测、临时文件原子发布、鉴权文件解析和删除清理。
- `controller/playground_generation.go`、`router/relay-router.go`：Session 历史接口、图片上传接口，以及仅依赖 Cookie/Token 的私有媒体读取接口。
- `model/playground_generation_test.go`、`controller/playground_generation_test.go`：生命周期、所有权、过滤排序、URL/大小边界和 HTTP 契约测试。
- `web/default/src/features/playground/api.ts`、`types.ts`：生成历史前端合同与 API。
- `web/default/src/features/playground/components/media-playground.tsx`：Quote → 建记录 → 单次付费生成 → 更新历史，以及视频刷新恢复。
- `media-generation-history.tsx`、`media-preview-dialog.tsx`：历史列表、删除确认和大尺寸预览。
- `web/default/src/features/pricing/components/model-details.tsx`：模型参数主标签、公开摘要和管理员治理入口。
- `web/default/src/i18n/locales/en.json`、`zh.json`：本轮中英文文案。

## 云资源与配置

- Railway 项目：`carlab-api`
- Railway 服务：`new-api`
- Railway 数据库：PostgreSQL
- Railway 缓存：Redis
- Cloudflare 入口：`https://api.carlab.top`
- GitHub 分支：`WashedYirgacheffe/new-api:codex/oem-api-hub`
- 新增数据库表：`playground_generations`、`playground_generation_assets`
- 新增 Railway 持久卷：`new-api-volume`，ID `0ec90021-eeea-4e75-8ef2-bbe3a38cb625`，挂载到 `new-api:/data`，容量 5 GB。
- 新增 Railway 变量：`PLAYGROUND_ASSET_DIR=/data/playground-assets`。
- 持久卷挂载配置部署 `8a19711d-d219-4e68-8c4c-0735e521a459` 为 `SUCCESS`，运行的是功能发布前基线。
- 首次功能部署 `08cd793c-5188-496d-b41f-cc0c9c16eb45` 成功运行 `2903af0c`，随后被修复部署替换，Railway 标记为 `REMOVED`，不是构建失败。
- 当前 active deployment 为 `eb06bcab-59c6-4367-8c96-7833ffee3ff3`，对应 `0509c550`，状态为 `SUCCESS / RUNNING / Online`。
- 卷状态为 `READY`，Railway 统计已用约 4.23 MB；`https://api.carlab.top/api/status` 返回 `success: true`。
- 不修改渠道密钥、价格配置、DNS、Cloudflare 或计费算法；服务继续为 Southeast Asia 单副本。

### Railway 核验安全事件

- 一次 SSH 只读探针因 Railway CLI 参数转发异常，把完整运行时环境意外回显到本次内部工具执行日志。本文档和用户消息均未记录或复述任何秘密值，后续所有 SSH 和变量探针已立即停止。
- 受影响范围可能包括 GitHub、Railway、数据库、Redis、Session 和应用加密凭据。应按已暴露处理，轮换相关凭据并检查访问日志；轮换前不得把本次工具日志复制到工单、聊天或公开文档。
- 该探针没有成功创建文件，也没有修改持久业务数据。资产目录写入、原子发布和 Cookie 鉴权已由真实图片归档与重新读取成功间接验证；独立 hardlink 探针未获得可信结果。

## 验证证据

本地与自动化验证：

- `go test ./model ./service ./controller ./router -run PlaygroundGeneration -count=1` 通过，覆盖生命周期、所有权、Session Cookie 媒体读取、MySQL TEXT 边界、文件完整性和存储不可用时禁止误删历史。
- `bun test src/features/playground/api.test.ts` 通过，覆盖历史 GET/POST/PATCH/DELETE 路径和 HTTP(S) 地址过滤。
- `bun test src/features/playground/lib/media-contract.test.ts` 8 个用例通过，覆盖图片/视频分发、参数和真实失败状态解析。
- `bun run typecheck` 通过。
- Playground 本轮 TS/TSX 文件定向 oxlint 通过。
- `bun run build` 通过。
- `git diff --check` 通过。
- `go test ./model ./service ./controller ./router -count=1` 中 model、service、router 通过；controller 仅有基线已存在的 `TestListModelsTokenLimitIncludesTieredBillingModel` 失败，本轮定向 controller 用例通过。

线上图片付费验收：

- 模型 `deepwl/gpt-image-2-all`，参数 `1024x1024 / quality=low`；Quote 为 `40000 / $0.08`。
- generation ID 为 `eb017573fd3b464bb92c852dfabbe270`，生成成功；余额从 `$199.57` 变为 `$199.49`，消费日志渠道为 `#7 DeepWL Image`，净额 `$0.08`。
- 刷新后记录仍存在，私有图片可重新加载和放大，实测 `1254 x 1254`。请求尺寸与实际像素不同但比例一致，需如实保留为供应商行为证据。

线上视频付费验收：

- 模型 `deepwl/omni-fast`，界面参数 `4 秒 / 16:9 / 720p`；Quote 为 `750000 / $1.50`。
- generation ID 为 `080f15834f87430496e623f95dce3a9b`，公开 task ID 为 `task_223Lr70QNGc468ExhHUYDW0MkgN5S6R8`；上游约 118 秒完成。
- 余额从 `$199.49` 变为 `$197.99`。没有重复创建任务或重复扣费；修复部署后复用同一任务恢复成功。
- 刷新后历史仍为成功，放大播放器 `readyState=4`、无媒体错误、`1280 x 720`。再次验收时媒体时长仍为 `10.005` 秒。

线上模型详情验收：

- 正确入口是 `/pricing`：点击模型行或卡片打开详情抽屉，也可访问 `/pricing/{URL 编码后的模型名}/` 独立详情页。
- 当前线上 `deepwl/gpt-image-2-all` 详情已显示四个主标签。管理员进入“模型参数”后可看到 `image.generate` 合同、参数字段编辑器、添加/删除、发布版本、修订回滚、证据 CRUD 和“候选路由”。
- 用户此前截图只有三个标签，属于最终部署前页面或浏览器旧缓存；重新打开页面或强制刷新后即可看到新入口。
- 尚无 operation binding 的模型只显示空摘要，不能在该详情页直接建立第一个 binding；需先通过现有合同治理流程建立绑定。

视频轮询事故与修复：

- 第一次提交响应同时包含内部数字 `id=14` 和公开 `task_id`，前端原先优先使用 `id`，下一次请求因此错误访问 `/pg/videos/14` 并返回 400。
- `0509c550` 将任务 ID 选择顺序修正为 `task_id -> taskId -> id`，并加入同时含两种 ID 的回归测试。
- `eb06bcab-59c6-4367-8c96-7833ffee3ff3` 部署后，页面使用原 task ID 恢复同一已付费任务，没有再次调用生成接口。

## 本次采用的发布与付费验收护栏

1. 发布后先验证 `/api/status`、`/pricing`、Playground 目录、Quote 和历史列表，确认只读检查不会产生费用。
2. 选最低成本图片模型，只点击一次生成；记录模型、Quote、generation ID、request ID、生成结果和消费日志净额。
3. 刷新页面，确认历史仍存在并可放大，不再次调用生成接口。
4. 选最低成本、最短时长视频模型，只点击一次；保存 generation ID 和 task ID，等待原任务轮询完成。
5. 刷新页面验证视频任务可恢复，完成后核对视频地址、历史状态、放大播放和消费日志。
6. 任一请求出现 5xx、历史缺失、重复上游任务或余额不匹配时立即停止，不点击重试。

## 回滚

1. 当前运行目标是提交 `0509c550`、部署 `eb06bcab-59c6-4367-8c96-7833ffee3ff3`。整体功能回滚应回到基线 `5e2375de`；曾运行该基线且已挂卷的部署为 `8a19711d-d219-4e68-8c4c-0735e521a459`。
2. 不要把 `08cd793c-5188-496d-b41f-cc0c9c16eb45` 作为最终回滚目标，因为它包含已确认的视频 task ID 轮询缺陷。
3. 验证 `/api/status`、原 Playground Quote/生成、`/pricing` 三个旧标签和消费日志正常。
4. `playground_generations`、`playground_generation_assets` 和 `new-api-volume` 可暂时保留，旧二进制不会读取。若需要彻底清理，先导出记录和 `/data/playground-assets` 文件，再在维护窗口删除数据库表、变量和持久卷；禁止先删卷造成不可恢复的数据丢失。
5. 回滚不会更改已经发生的真实账单、上游任务或已生成资产；账务异常必须通过原消费日志和资金流水处理，不能修改历史表伪造退款。

## 剩余风险与后续

- **凭据轮换为最高优先级运维事项**：Railway SSH 异常回显可能已把完整运行时环境写入内部工具日志。完成 GitHub、Railway、数据库、Redis、Session 与应用加密凭据轮换并检查访问日志前，不应关闭此风险。
- Omni 官方文档明确使用字符串 `seconds`，范围 4 到 30 秒；本地合同和回归测试也确认请求会发出 `"seconds":"4"`。但本次实际 MP4 为 `10.005` 秒，仍需让 DeepWL 结合 task ID 核对上游参数消费；禁止通过再次付费生成盲测。
- 第三方 HTTP(S) 结果地址长期可用性依赖供应商；完整媒体归档仍需迁移到私有对象存储。
- 视频关页后由服务端 Task 继续执行，但生成历史终态仍在用户重新打开 Playground 后通过 task ID 同步；尚未增加后台定时同步器。
- 同步图片或尚未取得 task ID 的视频在浏览器断网/关页时只能保留“结果不确定”的 pending 状态；幂等键和再次付费拦截可降低重复扣费风险，但完整恢复仍需要服务端生成编排/outbox。
- 历史目前保留提示词与参数，尚未增加用户级保留期、批量清理或导出策略。
- 管理员硬删除用户时，现有全局账号删除流程尚未接入卷文件清理；在实现统一资产回收任务前，运维需先删除该用户的 Playground 历史或执行卷巡检。
- 非中英文语言当前依赖 i18n 英文 fallback；本轮没有顺带补齐项目既有的 170+ 个跨语言缺失键。
- 候选路由不等于真实低价 failover；接入前仍需完成错误分类、参数转换、幂等与最终目标结算设计。
- 参数发布后会立即更新服务端合同；已打开的 Playground 目录有 15 秒缓存，跨页面没有 WebSocket 推送，因此“实时同步”准确含义是下一次重新获取合同后生效。
