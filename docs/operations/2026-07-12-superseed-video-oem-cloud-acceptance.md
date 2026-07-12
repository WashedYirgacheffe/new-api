# Superseed 视频与 OEM 云端验收记录

## 范围

本轮完成统一 Generation BFF 的剩余云端验收闭环：修复 Vercel 异步状态路由、完成 DeepWL 视频真实调用、验证 OEM 模型集合与计费路由组，并清理全部临时测试数据。

## 假设与边界

- CarLab API 继续作为模型、Profile、Quote、New API 计费组和网关路由的权威系统。
- Supabase 继续作为应用模型、OEM 租户、模型集合、本地用户倍率与生成账本的权威系统。
- 验收只在 `codex/ai-service-migration` 对应的 Vercel Preview 进行，没有切换 TapLater Production 流量。
- 本轮遵循云端优先约束，没有运行本地 Go、Vue 或 Vite 测试套件。
- 所有密钥只从 Vercel 托管配置读取；文档、命令输出和 Git 中均不保存密钥值。

## 变更文件

### Superseed

- `CarLab/TapLater/packages/taplater/api/generation-status.js`
- `CarLab/TapLater/packages/taplater/src/services/generationClient.ts`
- `CarLab/TapLater/packages/taplater/vercel.json`
- `CarLab/TapLater/packages/taplater/docs/DEPLOYMENT.md`
- `CarLab/BugBoard/data/progress.json`

### CarLab API

- `docs/operations/2026-07-12-superseed-video-oem-cloud-acceptance.md`
- `docs/operations/2026-07-12-superseed-ai-service-rollout.md`
- `AGENTS.md`

## 云端资源

| 资源 | 验收版本 |
| --- | --- |
| CarLab API Railway 服务 `new-api` | branch `codex/oem-api-hub`, commit `6a90b2bb`, deployment `8c859844-8dd3-46e0-9b07-f32526b972c7` |
| CarLab API | `https://api.carlab.top` |
| Superseed Vercel Preview | commit `c5d0afa`, deployment `dpl_AxSD9VkkL2tAxtaB3s1xQsNqfFZ8` |
| Preview URL | `https://superseed-thh7ind4s-washedyirgacheffe-4517s-projects.vercel.app` |
| TapDash | `https://superseed-dash.vercel.app` |
| Supabase | project ref `ctlirbtjzychneuaruci` |

本轮读取但不记录值的配置名称：

- `CARLAB_API_BASE`
- `CARLAB_SERVICE_TOKEN`
- `CARLAB_ROUTE_TOKENS_JSON`
- `MODEL_CATALOG_ENABLED`
- `GENERATION_BFF_TEXT`
- `GENERATION_BFF_IMAGE`
- `GENERATION_BFF_VIDEO`
- `VITE_GENERATION_BFF_TEXT`
- `VITE_GENERATION_BFF_IMAGE`
- `VITE_GENERATION_BFF_VIDEO`
- `VITE_SUPABASE_URL`
- `VITE_SUPABASE_ANON_KEY`
- `SUPABASE_SERVICE_ROLE_KEY`

## 验收证据

### 异步状态路由

- 根因是 `/api/generations/[id]` 与 SPA catch-all rewrite 冲突，线上请求回落到 `app.html`。
- 新增根级 Function `/api/generation-status?id=<generation_run_id>`，复用原状态处理器。
- Preview 未登录请求返回 HTTP 401、`content-type: application/json` 和 `{"ok":false,"error":"unauthorized"}`，证明请求不再进入 SPA。

### 视频 Generation BFF

- 应用模型：`603793ba-a83f-41b4-96a7-5c9d3792f553`，网关模型 `deepwl/grok-video-3`。
- Profile：`video.generate.basic@3`，端点 `openai-video`，异步执行，时长固定为字符串 `"6"`。
- 真实任务 `d2acb42e-b842-4f22-901b-72772192b2db` 成功完成；上游任务 ID 为 `task_VQgKWS8jYm1PGAVUUJhl9NvVZb7MsRmz`。
- Quote `estimated_amount=0.4`，定价版本 `a42d372ccf0b5dd13ecf71203521f9d2`，普通用户倍率 1.8。
- `quoted_credits=11`、`reserved_credits=11`、`actual_credits=11`，余额从 100 变为 89，无退款流水。
- 重复提交返回同一 generation run，最终只写入一条 `ai_tasks`，`credit_cost=11`。
- 返回结果包含 `storage.deepwl.cn` 的 HTTPS 视频地址；文档不保存有时效签名参数的完整 URL。

### OEM 模型集合与路由组

- 实际本地默认用户组 ID 为 `normal`，映射目标为 CarLab 组 `default`；不要把本地组 ID 误写成 `default`。
- OEM 用户未分配模型集合时，`/api/model-catalog` 返回 0 个模型，Generation BFF 在创建 run 和扣费前返回 `oem_model_collection_not_available`。
- 分配临时集合后，目录只返回三个已发布应用模型：
  - 文字 `8542d6c5-ff2e-4146-9568-9111a1e23656`
  - 图片 `dce1c40e-7282-47e0-b101-d8d9fe3f169a`
  - 视频 `603793ba-a83f-41b4-96a7-5c9d3792f553`
- 真实文字任务 `eb9a7aee-edd2-4873-b4da-384dc78edcf1` 返回 `OK`，`expected_carlab_group` 与 `effective_carlab_group` 均为 `default`。
- 本地倍率 1.8，预扣和实际结算均为 1 积分，重复提交幂等且只有一条 `ai_tasks`。

### 低输出上限调查

- `text.chat.basic@3` 的默认 `max_tokens` 为 1024，烟测配置为 128。
- 直接调用 `deepwl/gemini-3.5-flash` 且设置 `max_tokens=8` 时，上游 HTTP 200、`finish_reason=length`，5 个 completion token 全部为 reasoning token，最终 `content` 为空。
- Generation BFF 将空内容判为 `invalid_text_generation_response` 并走退款路径是正确行为；正常烟测必须使用 Profile 的 128 或默认值，不能用极小 token 上限推断模型不可用。

### 清理

- 视频验收后，临时用户、`generation_runs`、`ai_tasks` 与积分流水均为 0 残留。
- OEM 验收后，临时用户、OEM 租户、模型集合、集合成员、用户组分配、路由映射、`generation_runs`、`ai_tasks` 与积分流水均为 0 残留。

## 回滚

1. 将 Preview 与 Production 的 `GENERATION_BFF_*` 和 `VITE_GENERATION_BFF_*` 全部设为 `false`。
2. 将 `MODEL_CATALOG_ENABLED` 设为 `false`，TapLater 回退旧硬编码目录。
3. 对 Superseed commit `c5d0afa` 执行非破坏性 `git revert` 并推送，让 Vercel Git 集成重新构建。
4. 如需回滚 CarLab Profile，回退 Railway 到 `6a90b2bb` 之前的成功 deployment。
5. OEM 表是加法迁移；优先禁用租户或解除集合，不直接删除生产账本。

## 剩余风险与权限

- TapLater Production 的文字、图片、视频 BFF 开关仍保持关闭，必须由业务方确认后按类型逐项开启。
- 当前 Profile 是已验证模型的最低公共能力，不代表所有渠道模型都支持同一组高级参数；新增应用模型仍需文档证据与真实烟测。
- DeepWL 返回的视频地址带时效签名，正式画布流程必须继续使用既有媒体持久化链路，不能把上游临时 URL 当永久资产。
- OEM 管理功能已具备集合、管理员与路由组基础能力；真实 OEM 入驻、配额报表、账单结算和管理员授权仍需按租户逐项配置。
