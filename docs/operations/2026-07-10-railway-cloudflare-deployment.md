# Railway 与 Cloudflare 首次部署维护记录

- 日期：2026-07-10
- 状态：已完成（核心链路上线；Zone 级 WAF、Access 与 Turnstile 待补授权）
- 操作目录：`/Volumes/CODE/Code_SYS/CarLabAPI`
- 代码分支：`codex/oem-api-hub`

## 目标与边界

本轮将 New API 作为独立的多供应商模型中转中心部署。应用、PostgreSQL 和 Redis 运行在 Railway；公网入口、TLS、基础边缘防护和后续 WAF 由 Cloudflare 承担。没有把 New API 放入超级种子仓库，也没有把 PostgreSQL 替换为 Cloudflare D1。

## 架构对应关系

| 层级 | 资源 | 用途 |
| --- | --- | --- |
| Cloudflare | `api.carlab.top` | 统一 API、模型目录、API 文档和管理界面的公网入口 |
| Cloudflare Workers | `carlab-api-edge` | 流式反向代理、源站鉴权、安装接口屏蔽、认证端点限流和统一安全响应头 |
| Railway | 项目 `carlab-api` | New API 运行资源的隔离边界 |
| Railway | 服务 `new-api` | Go 后端与 React 前端容器 |
| Railway | 服务 `Postgres` | 用户、渠道、计费、日志和系统配置的持久化存储 |
| Railway | 服务 `Redis` | 分布式缓存和运行时协调 |

Railway 项目 ID 为 `a2f94bb2-4f70-4a58-92fe-614e90ba2783`，应用服务 ID 为 `ca79a3e0-a075-4f1e-8d27-c8219414738c`。这些 ID 用于运维定位，不是认证凭据。

## 本轮代码变更

1. 新增 Scalar API Reference 页面及 `/docs` 路由，并在构建前从 `docs/openapi/relay.json` 同步公开 OpenAPI 文件。
2. 更新 `Dockerfile`，让默认前端镜像构建能读取公开 OpenAPI 文件。
3. 修正 `.dockerignore`，继续排除其他文档，仅允许 `docs/openapi/relay.json` 进入 Docker 构建上下文。
4. 新增 `railway.json`，固定 Dockerfile 构建器、`/api/status` 健康检查、300 秒超时和失败重启策略。
5. 新增 `cloudflare/edge-gateway/`，用于部署薄型 Cloudflare Worker 边缘代理。
6. 新增 `middleware/edge_origin_auth.go`，启用共享密钥后只允许 Cloudflare 访问源站业务路由，保留 `/api/status` 给 Railway 健康检查。
7. 新增本维护记录，并在根 `AGENTS.md` 建立每轮操作文档索引。

## Railway 操作记录

### 创建资源

- Railway CLI：`5.26.0`
- 工作区：`WashedYirgacheffe's Projects`
- 区域：`Southeast Asia`（新加坡）
- PostgreSQL 模板：PostgreSQL 18，5 GB 持久卷
- Redis 镜像：Redis 8.2.1，5 GB 持久卷
- 应用构建器：根目录 `Dockerfile`

### 环境变量

已设置以下变量名，文档不记录任何原始值：

- `SQL_DSN`：引用 Railway PostgreSQL 的 `DATABASE_URL`
- `REDIS_CONN_STRING`：引用 Railway Redis 的 `REDIS_URL`
- `SESSION_SECRET`：独立生成的 384 位随机值
- `CRYPTO_SECRET`：独立生成的 384 位随机值
- `EDGE_SHARED_SECRET`：与 Cloudflare `ORIGIN_AUTH_SECRET` 配对的 384 位随机源站鉴权值
- `SESSION_COOKIE_SECURE=true`
- `SESSION_COOKIE_TRUSTED_URL`
- `SQL_MAX_IDLE_CONNS=10`
- `SQL_MAX_OPEN_CONNS=50`
- `SQL_MAX_LIFETIME=300`
- `TZ=Asia/Shanghai`
- `GIN_MODE=release`
- `ERROR_LOG_ENABLED=true`
- `BATCH_UPDATE_ENABLED=true`
- `NODE_NAME=carlab-api-railway-1`

### 构建故障与修复

第一次构建因 `.dockerignore` 排除整个 `docs/`，导致 `COPY ./docs/openapi/relay.json` 找不到文件。修复方式是仅重新包含该 OpenAPI 文件，未把其他文档加入镜像上下文。最终生产部署 ID 为 `94d08b1f-22bb-4c0e-9eee-808005743350`，云端构建成功编译新增 Go 中间件并通过健康检查。

## 管理员初始化与凭据

应用首次启动时数据库中没有 Root 用户，安装状态为未完成。操作过程先保持服务无公网域名，再通过一次性 Railway 域名执行安装接口，创建成功后确认 `/api/setup` 返回已初始化。

- 用户名：`root`
- 密码：192 位随机值，未写入仓库、命令输出或本文档
- 本机钥匙串服务名：`carlab-api-railway-admin`

本机读取密码：

```bash
security find-generic-password -a root -s carlab-api-railway-admin -w
```

首次登录后应立即启用两步验证。需要轮换密码时从 Web 管理界面完成，并同步更新或删除上述钥匙串条目。

## 验证证据

- Railway 应用、PostgreSQL、Redis 均为 `SUCCESS`，各自运行 1 个实例。
- `/api/status` 返回 HTTP 200，并通过 Railway 部署健康检查。
- 启动日志确认使用 PostgreSQL、Redis 已启用、数据库迁移完成、安装状态已持久化。
- 使用钥匙串密码真实登录成功，用户角色为 Root（`role=100`）。
- 会话 Cookie 包含 `HttpOnly`、`Secure` 和 `SameSite=Strict`。
- Railway 生产构建同时完成默认前端、经典前端和 Go 二进制编译。
- `https://api.carlab.top/docs`、`/pricing`、`/openapi/relay.json` 和 `/api/status` 均返回 HTTP 200。
- Cloudflare 入口的 `/api/setup` 返回 HTTP 404，未认证 `/v1/models` 返回 HTTP 401。
- Railway 随机域名的 `/docs` 与 `/api/setup` 均返回 HTTP 404，证明无法绕过 Cloudflare；仅 `/api/status` 返回 HTTP 200。
- Cloudflare 入口完成真实 Root 登录与退出验证。
- 按用户要求未运行本地 Go 测试；正确性以 Railway 云端 Go 编译、容器启动、健康检查和线上行为验证为准。

## Cloudflare 当前状态

- Wrangler 已通过 OAuth 登录 Cloudflare 账户。
- 可用 Zone：`carlab.top`，Zone ID 为 `da290ccc8a739d328741e231b92c0327`。
- Worker `carlab-api-edge` 已部署到 Custom Domain `api.carlab.top`，最终版本 ID 为 `d5e40f35-cab0-4e81-89e7-85bcf1bbc584`。
- `workers.dev` 与 Preview URL 均已关闭，避免产生额外公开入口。
- `ORIGIN_URL` 与 `ORIGIN_AUTH_SECRET` 均以 Worker Secret 保存，不写入代码或配置文件。
- Worker 对登录、注册、2FA、Passkey 登录和密码重置执行每 IP 每路径 20 次/分钟限流。
- Worker 对邮件验证码和找回密码邮件执行每 IP 每路径 5 次/分钟限流。
- `/v1/*` 未套上述边缘限流，避免 OEM 服务端共享出口被误伤；模型令牌与配额仍由 New API 管理。
- 当前 OAuth 不具备 DNS 读取/编辑和 Zone WAF 编辑权限，因此暂时不能创建 Zone 级自定义 WAF 或 Access 策略。
- 已发起包含 `challenge-widgets.write` 的 Wrangler OAuth 刷新，但浏览器未在超时前确认；Turnstile 尚未创建。

## 版本控制

- GitHub fork：`https://github.com/WashedYirgacheffe/new-api`
- `origin` 指向个人 fork，`upstream` 保持指向 `QuantumNous/new-api`。
- 本轮代码分支为 `codex/oem-api-hub`。

## 回滚

### Railway 应用

1. Railway Dashboard 打开项目 `carlab-api`。
2. 在 `new-api` 的 Deployments 中选择上一条成功部署。
3. 执行 Redeploy；数据库和 Redis 持久卷不会随应用回滚删除。

### Cloudflare Edge

```bash
cd /Volumes/CODE/Code_SYS/CarLabAPI/cloudflare/edge-gateway
npx wrangler rollback
```

源站鉴权启用后不能直接删除 Worker，否则业务路由会全部返回 404。紧急绕过时必须先在 Railway 删除或清空 `EDGE_SHARED_SECRET` 并等待新部署健康，再删除 `api.carlab.top` 的 Worker Custom Domain。恢复 Cloudflare 时按相反顺序执行：先部署会注入密钥的 Worker，再恢复 Railway 源站密钥。

## 剩余风险与后续动作

1. 为 Cloudflare Token 增加仅限 `carlab.top` 的 `Zone DNS Edit`、`Zone WAF Edit` 权限，并创建 Zone 级托管 WAF 规则。
2. 重新执行 `npx wrangler login` 并在浏览器完成授权，创建 Turnstile 后接入 New API 登录、注册与找回密码流程。
3. 创建独立管理域名并通过 Cloudflare Access 限制运营后台；公开模型 API 不能套 Access。
4. 在 Root 账号启用两步验证，并配置数据库备份与恢复演练。
5. 全仓格式检查仍命中 11 个本轮未修改的既有文件；本轮未扩大范围处理。
6. 后续接入模型供应商时，只在 New API 中保存供应商密钥，超级种子仅持有 New API 发放的渠道 Token。
