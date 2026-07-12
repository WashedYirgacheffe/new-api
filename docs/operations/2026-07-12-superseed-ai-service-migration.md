# Superseed AI 服务中心与统一生成链路实施记录

## 范围

- CarLab API 目录搜索增加 `model_type` 服务端过滤。
- Token 模型目录增加去重后的已发布 Profile 契约清单。
- TapDash 增加 API 渠道、网关模型、能力模板和应用模型管理页。
- Supabase 增加网关快照、应用模型、模型集合和统一生成账本。
- TapLater 增加动态模型目录、Schema 控件、Profile 校验和 Generation BFF。
- 文字、图片、视频保留独立功能开关，默认不切换现有生产调用。

## 假设与边界

- CarLab API 是渠道、网关模型、Profile、路由和 Quote 的权威系统。
- Supabase 只维护超级种子的应用发布、模型集合和本地生成账本。
- TapLater 不解析上游自然语言文档，也不接收客户端提交的 CarLab 路由组。
- 当前基础 Profile 只描述最低公共能力；模型特有参数仍需逐批建立并发布新版本。
- 本轮遵循云端优先约束，没有运行本地 Go 测试。

## 代码变更

### CarLab API

- `controller/model_meta.go`
- `controller/model_meta_test.go`
- `model/model_meta.go`
- `controller/model_catalog.go`

### TapDash

- `CarLab/TapDash/api/_carlab.js`
- `CarLab/TapDash/api/ai-channels.js`
- `CarLab/TapDash/api/ai-models.js`
- `CarLab/TapDash/api/ai-profiles.js`
- `CarLab/TapDash/api/ai-app-models.js`
- `CarLab/TapDash/src/features/ai-service/`
- `CarLab/TapDash/src/views/ai/`
- `CarLab/TapDash/src/types/ai-service.ts`
- `CarLab/TapDash/supabase/migrations/20260712134500_ai_service_center.sql`

### TapLater

- `CarLab/TapLater/packages/taplater/api/model-catalog.js`
- `CarLab/TapLater/packages/taplater/api/generations.js`
- `CarLab/TapLater/packages/taplater/api/generations/[id].js`
- `CarLab/TapLater/packages/taplater/api/_carlabGateway.js`
- `CarLab/TapLater/packages/taplater/api/_generationBilling.js`
- `CarLab/TapLater/packages/taplater/api/_generationDispatch.js`
- `CarLab/TapLater/packages/taplater/src/features/canvas/model-contract/`
- `CarLab/TapLater/packages/taplater/src/stores/modelCatalogStore.ts`
- `CarLab/TapLater/packages/taplater/src/services/generationClient.ts`
- `CarLab/TapLater/packages/taplater/supabase/migrations/20260712143000_generation_gateway.sql`

## 云端资源与配置

- Railway 项目：`carlab-api`
- Railway 服务：`new-api`
- CarLab 公网入口：`https://api.carlab.top`
- Vercel 项目：`superseed-dash`
- TapDash 生产域名：`https://superseed-dash.vercel.app`
- Vercel 项目：`superseed`
- Supabase 项目：`SUPERSEED`，ref `ctlirbtjzychneuaruci`
- 本机钥匙串：`carlab-api-tapdash-admin-token`
- 本机钥匙串：`carlab-api-superseed-token`
- 本机钥匙串：`superseed-dash-super-admin-password`

配置名称：

- `CARLAB_API_BASE`
- `CARLAB_ADMIN_ACCESS_TOKEN`
- `CARLAB_ADMIN_USER_ID`
- `CARLAB_SERVICE_TOKEN`
- `MODEL_CATALOG_ENABLED`
- `GENERATION_BFF_TEXT`
- `GENERATION_BFF_IMAGE`
- `GENERATION_BFF_VIDEO`
- `VITE_GENERATION_BFF_TEXT`
- `VITE_GENERATION_BFF_IMAGE`
- `VITE_GENERATION_BFF_VIDEO`

## 数据库迁移

- `20260712134500_ai_service_center`：通过 Supabase Management API 原子执行并登记迁移历史。
- `20260712143000_generation_gateway`：通过 Supabase Management API 原子执行并登记迁移历史。
- 原文件名 `20260712_ai_service_center.sql` 与远端既有 `20260712` 版本冲突，已改为唯一时间戳。

## 验证证据

- Railway 部署 `7907da7d-9568-4a20-90d4-d53978d41224` 验证 `model_type=video` 返回 227 个视频模型。
- TapDash 部署 `dpl_4ePuoe5LQ6rCzPFAdxJYGcRBdSUc` 通过 Vue TypeScript 和 Vite 构建。
- TapDash 渠道接口返回 8 个渠道。
- TapDash 网关模型接口按 `video` 返回 227 个模型。
- TapDash Profile 接口返回 6 个已发布基础 Profile。
- TapDash 目录同步写入 1056 个网关快照，定价版本为 `a42d372ccf0b5dd13ecf71203521f9d2`。
- Generation BFF 开关保持关闭，避免在应用模型尚未正式发布和逐模型验价前切流。

## 回滚

1. 将六个 Generation BFF 前后端开关全部设为 `false`，恢复旧供应商调用与旧节点积分流程。
2. 将 `MODEL_CATALOG_ENABLED` 设为 `false`，TapLater 自动回退硬编码模型目录。
3. 在 Vercel Dashboard 将 `superseed-dash` 或 `superseed` Promote 到上一成功部署。
4. 在 Railway 将 `new-api` 回滚到上一成功部署。
5. 数据表为加法迁移，回滚代码时可保留；如必须删除，先导出 `app_models` 和 `generation_runs` 后再手工执行反向 SQL。

## 剩余风险与权限

- 基础图片和视频 Profile 仍是最低公共契约，尚不能代表每个上游模型的全部参数和素材约束。
- BFF 的真实结算对账、OSS 服务端镜像和异步任务 Cron 接管仍需云端 E2E 验收后才能开启。
- OEM 所有权、集合过滤、CarLab 路由组映射和对账视图需要在应用模型稳定后继续落地。
- TapLater 生产项目通过 Git 自动部署，不能用未提交工作区直接发布。
