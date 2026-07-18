# NodyHub 视频协议路由维护记录

- 日期：2026-07-18
- 状态：首轮生产探测完成，本地纠正代码与定向测试完成，未重新部署
- 操作目录：`/Volumes/CODE/Code_SYS/CarLabAPI`
- 代码分支：`codex/oem-api-hub`

## 范围

本轮让协议类型仍为 OpenAI 的 NodyHub 渠道根据 `channel_provider=nodyhub` 使用其视频任务协议：普通视频生成提交到 `POST /v2/videos/generations`，后台任务轮询使用 `GET /v2/videos/generations/{task_id}`。NodyHub JSON 出站体中的合法 `seconds` 整数字符串转换为 JSON number，非法整数字符串直接拒绝；轮询响应按其裸 Task 形状解析大写状态、百分比字符串进度、失败原因和成功结果 URL。下游 CarLab API 路由继续保持 `/v1/videos`，其他 OpenAI 渠道、原生 Sora 渠道和 Remix 路径继续使用原有 `/v1/videos` 协议、字段类型与响应解析。

本轮不改模型计费、任务结算、响应解析、视频内容代理、渠道数据库配置或任何上游密钥。

## 假设

- NodyHub 渠道已使用规范化后的渠道商代码 `nodyhub`；代码同时使用大小写不敏感的精确比较，不接受前缀或模糊匹配。
- NodyHub 轮询返回裸 Task，使用 `task_id`、`SUBMITTED` / `QUEUED` / `IN_PROGRESS` / `SUCCESS` / `FAILURE`、字符串 `progress`、`fail_reason`、`result_url` / `video_url` 与 `data`；该结构仅在 NodyHub 渠道分支解析。
- NodyHub 轮询端点从 URL 获取任务 ID，生产探测证明该路径要求 GET 且不要求请求体。
- Remix 协议不在本轮 NodyHub 接入范围，保持原有 `/v1/videos/{task_id}/remix` 行为。

## 变更文件

- `constant/context_key.go`：新增渠道商请求上下文键。
- `middleware/distributor.go`：选中渠道后将 `channel_provider` 放入请求上下文。
- `relay/common/relay_info.go`：将渠道商传入 `ChannelMeta`，供任务适配器使用。
- `relay/channel/task/sora/adaptor.go`：仅为 NodyHub 切换视频提交/轮询路径、规范化 JSON `seconds` 字段并解析裸 Task 轮询响应。
- `relay/channel/task/sora/nodyhub_adaptor_test.go`：覆盖 NodyHub、其他 OpenAI 与 Sora 的提交/轮询协议、`seconds` 类型和响应解析合同。
- `service/task_polling.go`、`service/task_polling_test.go`：后台轮询从持久化渠道恢复渠道商并覆盖该行为。
- `AGENTS.md`、本文档：登记操作范围、验证与回滚边界。

## 云资源与配置

- Railway 项目：`carlab-api`
- Railway 服务：`new-api`
- 公网入口：`https://api.carlab.top`
- 运行配置：渠道字段 `channel_provider=nodyhub`
- 本轮没有修改 Railway、Cloudflare、数据库渠道、环境变量、Token 或 Secret，也没有执行部署。

## 验证证据

首轮生产探测得到以下纠正证据，记录中不保存密钥或任务 ID：

- `POST /v2/videos/generations` 已命中 NodyHub，但字符串 `seconds` 被上游拒绝，错误明确要求整数类型。
- 对同一上游任务直接调用 `GET /v2/videos/generations/{task_id}` 返回 HTTP 200。
- 对同一路径使用 POST 返回 `Invalid URL`，因此轮询方法纠正为 GET。
- GET 响应为裸 Task，而不是现有 Sora 轮询结构；真实失败样本使用大写 `FAILURE`、字符串 `100%` 和 `fail_reason`。回归 fixture 已匿名化，不包含真实任务 ID 或内容。

执行：

```bash
go test -count=1 ./constant ./middleware ./relay/common ./relay ./relay/channel/task/sora ./service
git diff --check
```

结果：所有受影响包均通过，差异检查无报错。适配器测试验证 NodyHub 使用 `POST /v2/videos/generations` 和 `GET /v2/videos/generations/{task_id}`，合法 `seconds` 整数字符串转换为 JSON number、非法字符串返回错误，并兼容五种大写任务状态、字符串进度、`fail_reason` 及成功结果 URL；其他 OpenAI/Sora 渠道继续使用 `POST /v1/videos`、`GET /v1/videos/{task_id}`、字符串 `seconds` 和原响应解析。服务测试继续验证后台轮询初始化时收到 `nodyhub` 渠道商。

本轮不会把工作区中其他开发者的既有改动纳入、回退或重排。

## 回滚

1. 在生产止损时先停用 NodyHub 渠道，阻止新视频任务提交。
2. 回退本轮渠道商上下文传递与 Sora 适配器分支，重新部署上一成功的 Railway 版本。
3. 回滚后普通 OpenAI/Sora 视频协议将统一恢复为提交 `POST /v1/videos`、轮询 `GET /v1/videos/{task_id}`，出站 `seconds` 恢复原字段类型。
4. 已创建的公开任务 ID 与上游任务 ID 分离机制不受本轮影响；必要时保留旧版本服务完成存量任务轮询后再切换。

## 剩余风险与权限

- 首轮生产探测已确认提交路径和 GET 轮询路径，但纠正后的 JSON number 出站与完整任务生命周期尚未重新部署验收。
- 首轮生产样本为失败任务；成功任务的 `result_url` / `video_url` 解析已有匿名 fixture，但仍需用最低成本成功任务验证实际字段位置与媒体可访问性。
- 视频内容代理目前仍按现有 OpenAI/Sora 规则处理；发布前必须验证成功任务能返回或代理可访问的视频 URL。
- 生产验收应使用单次最低成本请求，禁止自动重试，并在确认提交、轮询、任务落库和计费日志一致后再保持渠道启用。
