# NodyHub 视频协议路由维护记录

- 日期：2026-07-18
- 状态：本地代码与定向测试完成，待生产部署验证
- 操作目录：`/Volumes/CODE/Code_SYS/CarLabAPI`
- 代码分支：`codex/oem-api-hub`

## 范围

本轮让协议类型仍为 OpenAI 的 NodyHub 渠道根据 `channel_provider=nodyhub` 使用其视频任务协议：普通视频生成提交到 `POST /v2/videos/generations`，后台任务轮询使用 `POST /v2/videos/generations/{task_id}`。下游 CarLab API 路由继续保持 `/v1/videos`，其他 OpenAI 渠道、原生 Sora 渠道和 Remix 路径继续使用原有 `/v1/videos` 协议。

本轮不改模型计费、任务结算、响应解析、视频内容代理、渠道数据库配置或任何上游密钥。

## 假设

- NodyHub 渠道已使用规范化后的渠道商代码 `nodyhub`；代码同时使用大小写不敏感的精确比较，不接受前缀或模糊匹配。
- NodyHub 提交和轮询响应继续兼容现有 Sora `id` / `task_id`、`status`、`progress` 与 `error` 结构。
- NodyHub 轮询端点从 URL 获取任务 ID，不要求额外请求体；POST 请求声明 `Content-Type: application/json`。
- Remix 协议不在本轮 NodyHub 接入范围，保持原有 `/v1/videos/{task_id}/remix` 行为。

## 变更文件

- `constant/context_key.go`：新增渠道商请求上下文键。
- `middleware/distributor.go`：选中渠道后将 `channel_provider` 放入请求上下文。
- `relay/common/relay_info.go`：将渠道商传入 `ChannelMeta`，供任务适配器使用。
- `relay/channel/task/sora/adaptor.go`：仅为 NodyHub 切换视频提交和轮询方法及路径。
- `relay/channel/task/sora/nodyhub_adaptor_test.go`：覆盖 NodyHub、其他 OpenAI 与 Sora 的提交/轮询协议合同。
- `service/task_polling.go`、`service/task_polling_test.go`：后台轮询从持久化渠道恢复渠道商并覆盖该行为。
- `AGENTS.md`、本文档：登记操作范围、验证与回滚边界。

## 云资源与配置

- Railway 项目：`carlab-api`
- Railway 服务：`new-api`
- 公网入口：`https://api.carlab.top`
- 运行配置：渠道字段 `channel_provider=nodyhub`
- 本轮没有修改 Railway、Cloudflare、数据库渠道、环境变量、Token 或 Secret，也没有执行部署。

## 验证证据

执行：

```bash
go test -count=1 ./constant ./middleware ./relay/common ./relay ./relay/channel/task/sora ./service
git diff --check
```

结果：所有受影响包均通过，差异检查无报错。适配器测试验证 NodyHub 使用 `POST /v2/videos/generations` 和 `POST /v2/videos/generations/{task_id}`，其他 OpenAI/Sora 渠道继续使用 `POST /v1/videos` 和 `GET /v1/videos/{task_id}`；服务测试验证后台轮询初始化时收到 `nodyhub` 渠道商。

本轮不会把工作区中其他开发者的既有改动纳入、回退或重排。

## 回滚

1. 在生产止损时先停用 NodyHub 渠道，阻止新视频任务提交。
2. 回退本轮渠道商上下文传递与 Sora 适配器分支，重新部署上一成功的 Railway 版本。
3. 回滚后普通 OpenAI/Sora 视频协议将统一恢复为提交 `POST /v1/videos`、轮询 `GET /v1/videos/{task_id}`。
4. 已创建的公开任务 ID 与上游任务 ID 分离机制不受本轮影响；必要时保留旧版本服务完成存量任务轮询后再切换。

## 剩余风险与权限

- 本轮只验证 HTTP 方法、路径、鉴权头和渠道商传播，没有使用生产密钥执行付费视频任务。
- NodyHub 若返回与现有 Sora 结构不同的成功状态、结果 URL 或错误字段，仍需单独扩展 `DoResponse` / `ParseTaskResult` 并补真实响应回归样本。
- 视频内容代理目前仍按现有 OpenAI/Sora 规则处理；发布前必须验证成功任务能返回或代理可访问的视频 URL。
- 生产验收应使用单次最低成本请求，禁止自动重试，并在确认提交、轮询、任务落库和计费日志一致后再保持渠道启用。
