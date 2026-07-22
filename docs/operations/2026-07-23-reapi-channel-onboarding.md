# RE 全量模型渠道接入

## 范围与假设

- 将 `https://reapi.ai/models` 当前公开模型接入 CarLabAPI，渠道商代码为 `re`，管理端名称为 `RE`。
- RE 使用两套互不兼容的 API 与凭据：聊天模型走 `https://api.reapi.ai/v1/chat/completions`，异步图片、视频、音频及文本工具走 `https://reapi.ai/api/v1`，并轮询 `GET /tasks/{id}`。
- 目录快照固定 104 个 `re/<upstream-model-id>`：8 个聊天模型与 96 个异步模型。上游目录和 OpenAPI 均可能变化，后续更新必须重新审查快照。

## 变更

- 新增 `ChannelTypeReAPI`（显示为 `RE`）和 RE 异步任务适配器。
- 新增统一入口：`POST /v1/re/generations` 与 `GET /v1/re/tasks/:task_id`；任务查询仅允许所属用户读取，响应仅暴露本地 `task_*` ID，不泄露上游任务 ID。
- 新增 `docs/catalog/reapi-model-catalog.json`，保留来源、模型类型、上游 ID、协议、端点、模型页元数据、状态与完整性摘要。
- 新增 `cmd/reapi-onboard`：对目录做完整性和运行时适配器一致性校验；有两类密钥时才会创建或更新 `RE Chat`、`RE Async`。两个渠道和全部模型都默认手动停用/目录停用，等待价格与实调验收。
- 管理端渠道类型和定价端点类型增加 `RE` / `re-task`。

## 云资源与配置

- 目标服务：Railway `new-api`（CarLabAPI）。
- 运行初始化命令前须安全配置以下 Railway Secret，两个变量分别用于两个上游 API，严禁复用或写入仓库：
  - `REAPI_CHAT_API_KEY`
  - `REAPI_TASK_API_KEY`
- 初始化示例：`go run ./cmd/reapi-onboard`。可先运行 `go run ./cmd/reapi-onboard --dry-run` 只校验目录，不写数据库。

## 验证证据

- `go test ./relay/channel/task/reapi -count=1`：覆盖端点选择、模型映射保护、图片输入转换、Bearer 鉴权、任务 ID 隔离、状态映射、深层结果 URL 和失败原因。
- `go test ./controller -run TestRelayReAPITaskFetchScopesAndRedactsTaskID -count=1`：覆盖任务所属用户校验、非 RE 任务拒绝和上游 ID 脱敏。
- `go test ./router -run TestRelayRouterRegistersRETaskRoutes -count=1`：覆盖两个 RE 路由注册。
- `go run ./cmd/reapi-onboard --dry-run`：校验 104 条目录和运行时端点映射，不触碰数据库。

## 回滚

1. 在 Railway 将 `RE Chat` 和 `RE Async` 保持或改为手动停用。
2. 回滚本次 Git 提交，重新部署 CarLabAPI。
3. 如需移除目录记录，先导出 `models`、`model_channel_providers`、`channels` 和 `abilities` 中渠道商为 `re` 的数据，再按变更窗口删除；不得影响其他渠道商记录。

## 剩余风险与权限

- 当前未获得两类 RE 正式密钥，故本次仅完成代码、目录和初始化接入；不等于已真实调用、已定价或已开放。
- `RelayTaskSubmit` 的既有通用任务路径默认按 `video.generate` 准备操作合同。RE 的图片、音频与文本工具在逐模型开放前，应分别补齐并验收对应的操作合同与对外价格。
- 获取密钥后，需在 Railway 运行初始化并至少实测 1 个聊天、1 个图片、1 个视频、1 个音频/工具模型；确认结算、轮询与价格后再逐步启用渠道能力。
