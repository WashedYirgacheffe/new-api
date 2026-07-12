# 模型能力契约阶段 0 实施记录

- 日期：2026-07-12
- 仓库：`/Volumes/CODE/Code_SYS/CarLabAPI`
- 分支：`codex/oem-api-hub`
- 范围：CarLab API 能力契约、Token 模型类型权限、Token 范围目录与计费报价

## 范围

本轮按 Superseed Dash 与 TapLater 能力契约方案实施阶段 0，只修改 CarLab API。没有修改超级种子、TapDash、TapLater、Supabase、Vercel 或 Cloudflare 路由，也没有切换超级种子的生产 API 调用。

阶段 0 包含：

1. 可版本化的模型 Operation Profile、Profile Version 和 Model Binding 三表。
2. JSON Schema、UI Schema、Material Schema、响应契约和 Smoke Test 的结构化存储与管理接口。
3. Token 的模型类型权限，并与模型白名单、实际路由组和计费可用性取交集。
4. Token 范围模型目录、单模型 Profile 读取和 Quote 接口。
5. 文本、图片、视频、音频、Embedding、Rerank 六类保守通用 Profile，以及对现有启用模型的首次绑定。
6. CarLab API 默认前端中的模型类型权限选择和列表展示。

## 假设与边界

- `group` 继续只承担计费和路由，不复用为模型类型分类。
- 旧 Token 默认关闭模型类型限制，部署后保持兼容。
- 通用 Profile 只声明当前目录共同具备的 OpenAI 兼容最低调用能力，不声明尚未逐模型核验的清晰度、比例、时长或素材能力。
- Quote 复用 New API 当前真实计费 Helper 与有效 group 倍率。固定价格返回基础按次价格；Token 价格返回预扣估算。
- 尚未接入已验证 Task Adapter 的操作，不伪造参数倍率；响应会明确 `parameter_adjustments_applied=false` 和估算假设。
- Profile JSON 使用跨 SQLite、MySQL、PostgreSQL 兼容的 `TEXT` 字段，由 Go 解析和校验。

## 接口

### 管理员接口

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| `GET` | `/api/model-profiles/` | 分页读取 Profile 及版本 |
| `GET` | `/api/model-profiles/:profile_key?version=1` | 读取指定或最新版本 |
| `POST` | `/api/model-profiles/` | 创建 Profile 版本或更新 Draft |
| `GET` | `/api/model-profiles/bindings?model=...` | 读取模型绑定 |
| `POST` | `/api/model-profiles/bindings` | 创建或更新模型 Operation 绑定 |
| `DELETE` | `/api/model-profiles/bindings?model=...&operation=...` | 删除模型绑定 |

已离开 Draft 的 Profile Version 不允许原位修改；需要调整时必须发布新版本并重新绑定。

### 业务 Token 接口

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| `GET` | `/api/user/models/catalog` | 按当前 Token 的类型、模型白名单、路由组和价格过滤目录 |
| `GET` | `/api/user/models/profile?model=...&operation=...` | 读取 Token 有权访问模型的 Profile 与 Override |
| `POST` | `/api/user/models/quote` | 使用真实计费 Helper 获取 group 生效后的报价或预扣估算 |

目录返回模型供应商、API 渠道商、模型类型、端点、计费模式、价格版本、Profile Binding、可路由组和价格就绪状态。

## Token 权限规则

Token 新增：

- `model_type_limits_enabled`
- `model_type_limits`

支持的模型类型为 `text`、`image`、`video`、`audio`、`embedding`、`rerank`。运行时有效模型范围为：

```text
模型类型权限
∩ 指定模型白名单
∩ Token 实际计费/路由组可用模型
∩ 已配置价格或允许未定价模型的用户策略
```

该规则同时应用于 `/v1/models`、Token 范围目录和实际 Distributor 路由；异步任务查询会在仅启用类型限制时回填原始模型，避免轮询绕过或误拒绝。

## 变更文件

- 数据与迁移：`model/model_operation_profile.go`、`model/main.go`
- Token 权限：`model/token.go`、`controller/token.go`、`model/pricing.go`
- 认证与路由：`constant/context_key.go`、`middleware/auth.go`、`middleware/distributor.go`、`controller/model.go`
- Profile、目录与 Quote：`controller/model_operation_profile.go`、`controller/model_catalog.go`、`router/api-router.go`
- 计费边界：`relay/helper/valid_request.go`
- CarLab API 前端：`web/default/src/features/keys/`、`web/default/src/i18n/locales/`
- 架构依据：`docs/architecture/superseed-dash-capability-contract-design.md`

## 云资源与配置

| 资源 | 本轮操作 |
| --- | --- |
| Railway 项目 `carlab-api` / 服务 `new-api` | 待本分支推送后部署并记录 Deployment ID |
| PostgreSQL | 应用启动时通过 GORM AutoMigrate 新增三张 Profile 表和两个 Token 字段 |
| Redis | Token 缓存对象自动包含新增字段，无独立数据迁移 |
| `https://api.carlab.top` | 待 Railway 健康检查成功后验收新接口 |
| Cloudflare | 不修改 DNS、Worker 或路由 |
| 超级种子 / Vercel / Supabase | 不修改 |

本轮未读取、输出或写入 API Key、管理员密码、Cookie、数据库连接串或其他秘密。

## 验证证据

- Go 文件使用 Go 1.25.1 `gofmt` 完成格式化。
- 变更通过 `git diff --check`。
- 受影响的默认前端 TypeScript/TSX 文件通过 `oxfmt`。
- 受影响的默认前端文件通过 `oxlint`，只剩两个改动前已存在的 `parseInt` / `parseFloat` 建议级 warning，没有 lint error。
- 用户明确要求不运行本地 Go 测试；Go 编译、前端生产构建、PostgreSQL AutoMigrate、启动和接口验收由 Railway 云端完成。

## 回滚

1. Git 回滚本轮提交并重新执行 Railway 部署。
2. 旧应用版本不会读取新增 Profile 表或 Token 类型字段；新增表和列可保留，不影响旧版本运行。
3. 如需彻底清理，在确认没有下游使用后再单独删除 `model_operation_profiles`、`model_operation_profile_versions`、`model_operation_bindings`，不要在应用回滚时直接执行破坏性 DDL。
4. 若仅 Quote 或目录异常，可先回滚 API 路由提交，不需要变更现有 Channel、Ability、价格或 Token Key。

## 剩余风险与后续

- 当前通用 Profile 是保守最低能力，不代表 1589 个模型的完整参数能力；下一阶段需按渠道文档发布专用版本和 Model Override。
- Quote 尚未调用具体 Task Adapter 的 `EstimateBilling`，因此视频时长、分辨率等提交后倍率仍明确标记为未应用。
- TapDash 的 Profile 编辑器、批量绑定、批量测试中心和发布任务尚未实施。
- TapLater 仍使用原有硬编码模型与参数，本轮没有迁移生产调用。
- OEM 管理员身份隔离、租户级可见性和分组映射属于后续 OEM 阶段。
