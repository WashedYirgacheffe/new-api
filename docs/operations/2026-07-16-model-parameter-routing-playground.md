# 模型参数、路由与 Playground 维护记录

- 日期：2026-07-16
- 状态：本地实现与发布门禁完成，待提交、部署和云端验收
- 分支：`codex/oem-api-hub`
- 基线提交：`eacf9b91e92de8981994d2315d78df190968b9d1`

## 范围

本轮把 CarLab 的模型参数从展示性文本升级为可校验、可修订、可追溯的有效合同；增加参数证据与模型路由的管理 API；扩展 Playground，使文字、OpenAI Images、Gemini 原生图片和异步视频共用 CarLab 的模型目录、合同与报价边界。核心 DeepWL 合同同时收紧到已记录的 GPT Image 2 和 Omni 参数范围。

## 假设与边界

- CarLab 继续作为模型 Profile、Binding、有效参数合同和采购报价的唯一真相源；应用端只能消费已发布合同。
- 参数证据区分文档、Demo、人工记录和实际测试。保存证据不等于对应参数已经完成付费实测或生产发布。
- `model_routes` 当前是管理与兼容性校验数据层。`enabled=true` 只表示配置可被读取，不表示 Relay 已执行失败切换。
- 路由写入时保守比较输入/素材 schema、请求合同、参数默认值/强制值、dispatch/poll 路径、端点类型、执行模式和响应合同，并检查图循环；输入和素材采用方向性能力包含（目标必须覆盖源的合法范围），协议字段仍精确匹配。读取关系时会重新计算兼容状态。按 operation 建立数据库锁，避免空图并发创建 A→B/B→A。
- 路由策略名 `lowest_effective_cost_failover` 是目标语义；当前目标顺序仍由管理员保存的 `priority`、`tie_breaker` 决定，不能宣称已按实时价格自动排序。
- 本轮没有执行会产生上游费用的真实 Playground 请求，没有修改生产渠道、价格、密钥、DNS 或环境变量。
- API Key、Token、Cookie、数据库口令和媒体签名地址不得进入源码、文档或 Git。

## 已完成改动

- Binding override 增加 `schema_mode=replace`，支持模型别名覆盖继承参数，并对字段类型、枚举、默认值、数值范围、UI 映射、素材映射和请求适配器执行服务端校验。
- Quote 先应用默认值与强制值，再校验并计算参数倍率，避免页面参数与实际报价合同分叉。
- Relay 在价格估算前校验必填字段和原始未声明参数，并把同一份 effective parameters 写回文本、OpenAI Images、Gemini 原生图片和异步视频请求；报价、预扣与上游派发不再各自解析参数。
- 显式 `max_completion_tokens` 会参与合同校验和计费，但不会再同时注入默认 `max_tokens`，避免向上游发送互斥字段。
- 公网 URL 素材通过固定 SSRF 防护客户端执行 HEAD + 有限 Range 内容探测，核对 MIME、总大小和内容指纹；`transport=url` 拒绝 data URI/multipart，无法可信验证的角色或总时长合同关闭失败。
- 启动 Seed 只创建缺失 Binding，并保留精确的已知旧版本迁移；管理员修改过的 profile/version/overrides/enabled 不会在重启时被默认值覆盖。
- object/array 输入 schema 的保存、运行参数校验和路由兼容比较递归到嵌套 properties/items、required、枚举与范围。
- 素材 MIME 在运行校验和路由兼容中支持 `image/*`、`video/*` 与 `*/*` 的方向性包含，目标合同必须覆盖源合同的全部合法类型。
- Binding 增加合同 hash/version、乐观锁、修订历史和回滚；保存与回滚的过期 hash 均返回 HTTP 409。
- Binding 删除同样要求 `expected_contract_hash` 或 `If-Match`，过期删除返回 HTTP 409。
- 新增参数证据 CRUD：`GET/POST /api/model-profiles/evidence`、`DELETE /api/model-profiles/evidence/:id`。
- 初始化幂等写入五个核心 DeepWL 模型的三层证据（官方文档、Duoyuanx Demo、TapLater 发布合同），并为 GPT 三模型记录公开价证据；公开价不等于运行路由或实际结算。
- 新增 Binding 修订 API：`GET /api/model-profiles/bindings/revisions`、`POST /api/model-profiles/bindings/rollback`。
- 新增模型路由 CRUD、输入/输出关系查询、CAS、稳定优先级、循环检测和基础合同兼容校验：`/api/model-routes`。
- 初始化幂等创建一个禁用的 GPT Image 别名管理候选组（`gpt-image-2-all` → `gpt-image-2-c` → `gpt-image-2`）；它仅用于关系展示，不进入 Relay 兜底。
- 新增 Session Playground 目录、报价、OpenAI Images、Gemini generateContent、异步视频提交与轮询路由；网关 `group` 查询参数不会转发给上游。
- Playground 的报价、图片/视频生成和视频轮询统一发送 `X-CarLab-Operation`；视频成功任务同时规范化返回 `video_url`，失败原因不会再被误判为媒体地址。
- 修正 Relay 重试选路路径：保留 Playground 规范化路径，同时排除普通请求 query，避免 Advanced Custom 首次选路与重试选路不一致。
- 参数 Binding、证据、修订、模型路由和 Playground 的异步读取按模型、operation 与运行代次隔离；快速切换模型或卸载页面后，迟到响应不能覆盖新上下文。
- 参数证据 Seed 只创建缺失记录，命中既有记录时保留管理员设置的验证状态、验证时间和备注。
- GPT Image 2 与 C 固定为 Demo 记录的 13 档尺寸、`low/medium/high` 和 `n=1`；All 固定为三档 1K 尺寸、同三档质量和 `n=1`。
- Omni 与 Omni V2V 固定为 `4/6/8/10` 秒和 `720p`；Omni 支持最多 5 张参考图，V2V 要求 1 个不超过 15MB 的参考视频并可附加最多 5 张参考图。

## 变更文件

- `model/model_operation_contract.go`、`model/model_operation_profile.go`：合同校验、replace 模式、版本/CAS 和核心默认合同。
- `model/model_operation_governance.go`：Binding 修订、回滚和参数证据持久化。
- `model/model_route.go`：管理态路由组、目标、关系、兼容性、循环检测和 CAS。
- `controller/model_operation_profile.go`、`controller/model_route.go`、`controller/model_catalog.go`：管理 API 与 Quote 规范化。
- `controller/playground.go`、`controller/relay.go`、`middleware/distributor.go`、`relay/common/relay_info.go`、`router/*.go`：多模态 Playground 与规范化 Relay 路径。
- `model/main.go`：新增治理与路由表的 GORM 自动迁移。
- `model/*_test.go`、`controller/*_test.go`、`middleware/*_test.go`、`relay/common/*_test.go`：合同矩阵、治理、路由和 Playground 回归测试。
- `web/default/src/features/models/`、`web/default/src/features/playground/`：并行前端任务提供参数/证据/路由工作区与多模态 Playground；本维护记录不替代前端构建和浏览器验收。

## 云资源与配置

- 目标运行资源仍为 Railway 项目 `carlab-api` 的 `new-api`、PostgreSQL 和 Redis，以及 Cloudflare 入口 `https://api.carlab.top`。
- 新表由既有 GORM 迁移流程创建：`model_operation_binding_revisions`、`model_operation_parameter_evidences`、`model_route_groups`、`model_route_targets`、`model_route_operation_locks`。
- 本轮不新增环境变量。部署仍应由该仓库既有 Git/Railway 流程触发，不能在未提交工作区上宣称云端已更新。
- Superseed Preview 可消费 CarLab 目录与合同，但其 Vercel/Supabase 配置和验收属于 Superseed 仓库，不在本记录中伪报完成。

## 验证证据

- `gofmt` 已应用于本轮后端 Go 文件，`git diff --check` 通过。
- `go test ./model ./service ./relay/helper ./relay ./middleware ./relay/common ./router -count=1` 通过；核心合同矩阵逐项覆盖 `deepwl/gpt-image-2`、`deepwl/gpt-image-2-c`、`deepwl/gpt-image-2-all`、`deepwl/omni-fast` 和 `deepwl/omni-fast-v2v`。
- `go test ./relay/helper ./service ./relay ./middleware ./relay/common -count=1` 通过；合同参数写回、素材 URL SSRF/Range/内容指纹、角色/时长关闭失败和异步视频请求缓存均有回归覆盖。
- `go test -race` 定向运行本轮 `service` 素材探测与 `relay/helper` 合同测试通过。直接对两个包运行全部 race 测试仍会命中未修改的 logger 全局状态与旧 task polling/stream scanner 测试竞态，不属于本轮改动。
- 路由方向性输入/素材/请求合同失配、合同变更后的实时失配、operation 锁、禁用候选组、Binding 删除 CAS、证据归属不可变和证据 seed 幂等测试均通过。
- `go test ./middleware ./relay/common -count=1` 通过；`go test ./router -count=1` 通过。
- controller 除仓库既有 `TestListModelsTokenLimitIncludesTieredBillingModel` 外全部通过；该失败可在未修改的测试与实现上单独复现，测试 fixture 没有创建可路由 Ability，和本轮合同/Playground 改动无关。
- `web/default` 的五个相关测试文件逐文件执行为 40/40，通过 TypeScript typecheck、所有本轮 TS/TSX 文件定向 oxlint/oxfmt 和 Rsbuild 生产构建（5.87 秒）；中英文静态键与新增动态素材校验键均无缺失。一次性 `bun test` 只发现一个文件，不能作为本轮全量前端证据。
- 全仓 oxlint 与全仓 format check 仍会命中未修改文件的既有规则债务；本轮没有借机格式化或修改订阅、钱包、系统设置等无关模块。
- `go test ./... -count=1` 还被缺失的 `web/classic/dist` 阻塞根包 `go:embed`；本轮没有伪造或提交前端构建产物。除根包与上述 controller 基线失败外，命令输出中的其他包全部通过。
- `go vet ./...` 同样先被缺失的 `web/classic/dist` 阻塞。本轮涉及的 `controller`、`middleware`、`model`、`service`、`relay/helper`、`relay/common`、`router` vet 通过。
- 尚未执行 Railway 部署、生产数据库迁移、真实付费调用或浏览器验收，因此不存在对应云端成功证据。

## 回滚

1. 回退本轮后端、前端和运维文档提交，并通过既有 Git/Railway 流程重新部署上一成功版本。
2. 验证旧 `/api/model-profiles`、Token 模型目录、Quote 和文字 Playground 仍可用。
3. 新增数据表可暂时保留，不会被旧二进制读取；需要清理时先导出参数证据、Binding 修订和路由配置，再由数据库管理员在维护窗口删除，禁止应用启动时自动破坏性回滚。
4. 已发布 Binding 如需数据级回退，优先使用修订回滚与当前 contract hash；不要直接修改生产表绕过 CAS。
5. 路由尚未进入真实 Relay，关闭或删除路由组不会影响现有渠道重试逻辑。

## 剩余风险与权限

- 真实别名兜底尚未实现。必须先完成参数/素材变换、失败分类、幂等边界、预扣与最终目标结算，才能把管理路由接入 Relay。
- 公网 URL 素材会在 Relay 提交前执行严格 HEAD/Range 与内容指纹探测，但源站仍可能在探测后、上游拉取前替换内容；彻底消除该 TOCTOU 风险需要 CarLab 托管素材并向上游发送不可变对象地址。
- `roles` 只有类型化 `{url, role}` 输入才能验证；`max_total_duration` 在没有服务端媒体解析时一律关闭失败。启用时长限制前必须增加可信容器解析或托管转码链路，不能依赖客户端声明。
- 参数证据更新和删除目前只有行锁与归属保护，没有 `expected_updated_time`/hash CAS；并发管理员可能发生后写覆盖前写或删除刚更新记录。前端应避免并行写入，后续需为 Evidence CRUD 增加版本令牌和 HTTP 409 冲突语义。
- 证据 seed 已写入代码并在有表环境幂等执行；尚未执行生产数据库迁移，因此不能把本地记录当作云端完成证据。
- 素材数量和 15MB 限制属于有效合同，实际上传/转发仍需每个消费端和适配器执行同等校验，不能只依赖 UI。
- 本地测试使用 SQLite；新增表和事务通过 GORM 保持跨库写法，但仍需在 Railway PostgreSQL Preview/部署阶段验证迁移、索引和 CAS 并发行为。
- 多模态 Playground 尚未执行真实 DeepWL 图片/视频请求；生产验收必须使用隔离低额度账号、单次调用并核对 Quote、消费日志、轮询与结果解析，禁止自动重试。
- `web/classic/dist` 缺失与既有 controller/vet 告警需要独立修复；本轮为避免扩面没有修改这些无关基线问题。
