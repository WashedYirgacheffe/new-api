# Superseed 分站控制面迁移

- 日期：2026-07-22
- 状态：已部署并完成无付费线上验收
- 代码分支：`codex/oem-api-hub`
- 代码提交：`7feaa6df`、`78fea327`、报价限流修复待部署提交

## 范围

本轮把 Superseed 分站认领、分站资料 CRUD 和分站模型启用控制面放到 `api.carlab.top`。CarLabAPI 新增介于普通用户与平台管理员之间的分站管理员角色；TapLater 通过按域名授权的服务端目录读取模型集合，TapDash 不再承担这些运营入口。

生成模型的元数据、Profile、Binding、路由和价格仍由 CarLabAPI 既有模型合同治理。本轮不修改供应商协议，不执行付费图片或视频生成。

## 假设与权限边界

- 分站管理员角色由平台管理员在用户管理页显式授予或撤销，认领动作不修改用户角色。
- 分站管理员只能读取可认领站点，并管理自己已认领的站点模型；平台管理员可维护全部分站。
- 域名目录只接受实际路由组与站点 `route_group` 一致的业务 Token；Token 和认领凭据均保存在受管环境或数据库中，不写入本文档。
- 默认 Superseed 分站只在首次创建分站表时初始化；删除或迁移站点后，服务重启不会重新创建。认领密码优先读取 `SUBSITE_DEFAULT_CLAIM_PASSWORD`，未配置时随机生成且不回显，平台管理员需在交付分站管理员前显式设置。
- 4 个视频运行时变体只参与服务端派发，不能作为独立分站模型启用。

### 凭据边界

- CarLabAPI 管理控制台使用用户管理 `access_token`：请求必须同时带 `Authorization: Bearer <access_token>` 和对应数字 `New-Api-User`。该令牌必须存在于当前实例用户表；仅有一串外部管理令牌不能通过管理鉴权。
- TapLater/目录读取使用的是 CarLab 业务 Token（`model.Token`），只用于 `/api/user/models/catalog` 和按域名目录，不具备用户管理权限。
- 两类令牌不能互换。若管理接口返回 401，应在 `api.carlab.top` 用户管理中为目标管理员重新生成 access token，并把数字用户 ID 与令牌分别配置到受管环境；不要把令牌写入文档、前端变量或提交记录。

## 代码与数据变更

- `common/constants.go`、`middleware/auth.go`、`controller/user.go`：新增角色 5、即时角色校验和受限的普通用户/分站管理员角色切换。
- `model/subsite.go`、`model/main.go`：新增 `subsites`、`subsite_admins`、`subsite_models`，并初始化 Superseed 站点和 17 个 canonical 模型。
- `controller/subsite.go`、`router/api-router.go`：新增认领、平台 CRUD、模型集合和按域名 Token 目录接口；错误使用真实 HTTP 4xx/5xx 和稳定 code。
- `router/api-router.go`：`/api/user/models/quote` 保留业务 Token 鉴权，但移除 IP 级 `CriticalRateLimit`。该接口由 TapLater Vercel Functions 代表用户请求，Vercel 共享出口 IP 会把 20 次/20 分钟的 CriticalRateLimit 放大成 Preview 级 429；全局 API 限流和生成提交的权威计费校验仍保留。
- `web/default/src/features/subsites/`、`web/default/src/routes/_authenticated/subsites/`：新增认领、模型双栏筛选保存和管理员分站页面。
- `web/default/src/features/users/`、侧栏、角色及 i18n 文件：用户管理支持角色 5，并按角色隐藏平台控制面。
- Superseed 对应分支：TapLater 改读 `/api/subsites/by-domain/:domain/catalog`，TapDash 隐藏重复入口，Generation BFF 保留结构化错误和失败日志。

## API 契约

- `GET /api/subsites/claimable`
- `POST /api/subsites/claim`
- `GET/POST /api/subsites`
- `PUT/DELETE /api/subsites/:code`
- `GET/PUT /api/subsites/:code/models`
- `GET /api/subsites/by-domain/:domain/catalog`

认领密码只写不读。模型保存会拒绝未知、不可路由、未定价、合同未就绪和仅运行时模型。域名目录只返回站点已启用且当前 Token 可路由的模型合同。

## 云资源与配置

| 资源 | 影响 |
| --- | --- |
| Railway `carlab-api/new-api` | deployment `1fe5b370-9a0b-4c0b-8340-8fa2182fac1a` 已成功，构建新后端和默认 React 控制台，启动时迁移三张分站表；首次 seed 可选读取 `SUBSITE_DEFAULT_CLAIM_PASSWORD` |
| Railway PostgreSQL | 保存分站、认领关系和启用模型；不保存明文认领密码 |
| Railway Redis | 用户角色和目录相关缓存按既有机制失效 |
| Cloudflare `api.carlab.top` | 继续代理 Railway 源站，无新增公开源站 |
| Vercel `superseed` Preview | TapLater 服务端读取 CarLabAPI 分站目录 |

TapLater 使用的配置名称为 `CARLAB_SUBSITE_DOMAIN`、`CARLAB_SUBSITE_ROUTE_GROUP`、`CARLAB_SERVICE_TOKEN` 或 `CARLAB_ROUTE_TOKENS_JSON`。CarLabAPI 可选使用 `SUBSITE_DEFAULT_CLAIM_PASSWORD` 完成首次 seed；未设置时使用不可回显的随机值。仓库不保存任何 Token 或认领密码。

## 验证证据

- CarLabAPI 分站 model、controller、middleware 聚焦 Go 测试通过；SQLite fixture 覆盖默认 seed、认领不改角色、真实 HTTP 错误和路由组隔离。
- 默认前端 `bun run typecheck`、定向 lint/format 和 `bun run build` 通过。
- TapLater 使用 Node 20.20.2 执行全量测试，331/331 通过；`vue-tsc --noEmit` 和 Vite build 通过。
- CarLabAPI `go test ./router` 通过，覆盖 `/api/user/models/quote` 路由保留 `TokenAuth` 与 `QuoteTokenModel`，且不再挂接 rate-limit handler。
- Railway deployment `1fe5b370-9a0b-4c0b-8340-8fa2182fac1a` 状态为 `SUCCESS`，服务为 `Online`；`GET https://api.carlab.top/api/status` 返回 HTTP 200。
- 未登录访问 `GET /api/subsites/claimable` 返回 HTTP 401 和 `error.code=subsite_auth_required`；携带无效 access token 与用户 ID 返回 HTTP 401 和 `error.code=subsite_access_token_invalid`。
- `https://api.carlab.top/` 和三个分站控制台 SPA 路径均返回 HTTP 200；TapLater Preview deployment `dpl_5ecDr89kYWoVQQ8y13AthxsHFXpd` 状态为 `READY`，对应 Superseed commit `74a3ddb`。
- 未执行任何付费生成请求。

## 回滚

1. Railway `new-api` 选择上一条成功 deployment 执行 Redeploy；PostgreSQL 和 Redis 不回滚或删除。
2. Superseed Preview 回到上一 Git commit；生产 `main` 不在本轮直接切换。
3. 需要逻辑回滚时先在 CarLabAPI 禁用目标分站，再回退 TapLater；三张新增表保留，不做破坏性删除。
4. 若目录授权异常，核对业务 Token 的实际路由组和站点 `route_group`，不得临时放宽跨组校验。

## 剩余风险与权限

- TapLater 对分站目录有最长 60 秒服务端缓存，后台禁用后的授权撤销不是瞬时生效，但不会跨路由组。
- 生产 TapLater 的本地 `app_models` 仍必须保留对应合同身份；它不再决定分站准入，但缺失时会阻断报价和派发。
- 未配置 seed 认领凭据时，默认分站只能由平台管理员先重设认领密码再交付；本文档不记录任何原始值。
- 本轮只做无付费合同与权限验收，真实图片/视频产出继续遵守独立付费验收流程。
