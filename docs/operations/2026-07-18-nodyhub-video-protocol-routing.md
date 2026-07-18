# NodyHub 视频协议路由维护记录

- 日期：2026-07-18
- 状态：生产部署与文字、图片、视频代表性验收完成
- 操作目录：`/Volumes/CODE/Code_SYS/CarLabAPI`
- 代码分支：`codex/oem-api-hub`

## 范围

本轮让协议类型仍为 OpenAI 的 NodyHub 渠道根据 `channel_provider=nodyhub` 使用其视频任务协议：普通视频生成提交到 `POST /v2/videos/generations`，后台任务轮询使用 `GET /v2/videos/generations/{task_id}`。NodyHub JSON 出站体中的合法 `seconds` 整数字符串转换为 JSON number，非法整数字符串直接拒绝；轮询响应按其裸 Task 形状解析大写状态、百分比字符串进度、失败原因和成功结果 URL。内容代理在 NodyHub 历史任务只保存自身代理地址时，从已持久化的任务响应恢复直接媒体 URL。下游 CarLab API 路由继续保持 `/v1/videos`，其他 OpenAI 渠道、原生 Sora 渠道和 Remix 路径继续使用原有 `/v1/videos` 协议、字段类型与响应解析。

本轮不改模型计费、任务结算、视频内容代理、渠道数据库配置或任何上游密钥。

## 假设

- NodyHub 渠道已使用规范化后的渠道商代码 `nodyhub`；代码同时使用大小写不敏感的精确比较，不接受前缀或模糊匹配。
- NodyHub 轮询返回裸 Task，使用 `task_id`、`SUBMITTED` / `QUEUED` / `IN_PROGRESS` / `SUCCESS` / `FAILURE`、字符串 `progress`、`fail_reason`、可选顶层 `result_url` / `video_url`，且真实成功样本的媒体地址位于 `data.result.videos[].url[]`；该结构仅在 NodyHub 渠道分支解析。
- NodyHub 轮询端点从 URL 获取任务 ID，生产探测证明该路径要求 GET 且不要求请求体。
- Remix 协议不在本轮 NodyHub 接入范围，保持原有 `/v1/videos/{task_id}/remix` 行为。

## 变更文件

- `constant/context_key.go`：新增渠道商请求上下文键。
- `middleware/distributor.go`：选中渠道后将 `channel_provider` 放入请求上下文。
- `relay/common/relay_info.go`：将渠道商传入 `ChannelMeta`，供任务适配器使用。
- `relay/channel/task/sora/adaptor.go`：仅为 NodyHub 切换视频提交/轮询路径、规范化 JSON `seconds` 字段并解析裸 Task 轮询响应。
- `relay/channel/task/sora/nodyhub_adaptor_test.go`：覆盖 NodyHub、其他 OpenAI 与 Sora 的提交/轮询协议、`seconds` 类型和响应解析合同。
- `controller/video_proxy.go`、`controller/video_proxy_nodyhub_test.go`：仅为 NodyHub 从已存直接结果或任务响应恢复媒体 URL，继续复用现有 SSRF 保护代理。
- `service/task_polling.go`、`service/task_polling_test.go`：后台轮询从持久化渠道恢复渠道商并覆盖该行为。
- `AGENTS.md`、本文档：登记操作范围、验证与回滚边界。

## 云资源与配置

- Railway 项目：`carlab-api`
- Railway 服务：`new-api`
- Railway 成功部署：`39474155-56b5-4b6a-91dc-abc5a4c95820`
- 部署代码提交：`494c1236`
- 公网入口：`https://api.carlab.top`
- 运行配置：渠道字段 `channel_provider=nodyhub`
- 本轮没有修改 Railway 环境变量、Cloudflare、数据库渠道、Token 或 Secret；部署从目标提交的干净 worktree 上传。

## 验证证据

首轮生产探测得到以下纠正证据，记录中不保存密钥或任务 ID：

- `POST /v2/videos/generations` 已命中 NodyHub，但字符串 `seconds` 被上游拒绝，错误明确要求整数类型。
- 对同一上游任务直接调用 `GET /v2/videos/generations/{task_id}` 返回 HTTP 200。
- 对同一路径使用 POST 返回 `Invalid URL`，因此轮询方法纠正为 GET。
- GET 响应为裸 Task，而不是现有 Sora 轮询结构；真实失败样本使用大写 `FAILURE`、字符串 `100%` 和 `fail_reason`。回归 fixture 已匿名化，不包含真实任务 ID 或内容。
- 真实成功样本使用大写 `SUCCESS`，媒体地址位于 `data.result.videos[0].url[0]`；该地址的 Range GET 返回 HTTP 206 和 `video/mp4`，而未纠正版本的 CarLab `/v1/videos/{public_task_id}/content` 返回 HTTP 502，因为解析器没有把嵌套媒体地址写入任务结果 URL。
- 最终部署后，同一成功任务保持 `SUCCESS / 100%`，CarLab `/v1/videos/{public_task_id}/content` 返回 HTTP 200、`video/mp4` 和 390832 字节；无需再次创建付费任务。
- 最终视频请求只记录一笔 600000 quota 的 gold 消费且没有退款；两条协议纠正阶段失败任务均有等额类型 6 退款日志，不需要人工补偿。
- 本轮目录代表性烟测还包括 `nodyhub/gpt-4o-mini` 文字和 `nodyhub/gpt-image-1` 图片，均命中 channel 3 / gold 并成功返回结果。

执行：

```bash
go test -count=1 ./constant ./middleware ./relay/common ./relay ./relay/channel/task/sora ./service
go test -count=1 ./controller -run '^TestGetNodyHubVideoURL'
git diff --check
```

结果：适配器、服务与 NodyHub 内容代理定向测试通过，差异检查无报错。测试验证 NodyHub 使用 `POST /v2/videos/generations` 和 `GET /v2/videos/generations/{task_id}`，合法 `seconds` 整数字符串转换为 JSON number、非法字符串返回错误，并兼容五种大写任务状态、字符串进度、`fail_reason`、顶层成功 URL 及 `data.result.videos[].url[]` 嵌套 URL 数组；其他 OpenAI/Sora 渠道继续使用 `POST /v1/videos`、`GET /v1/videos/{task_id}`、字符串 `seconds` 和原响应解析。服务测试继续验证后台轮询初始化时收到 `nodyhub` 渠道商，内容代理测试覆盖已存直接 URL、历史自身代理 URL 回放和缺失 URL 失败。

本轮不会把工作区中其他开发者的既有改动纳入、回退或重排。

## 回滚

1. 在生产止损时先停用 NodyHub 渠道，阻止新视频任务提交。
2. 从提交 `5190b1f5` 创建干净 worktree 并重新部署，或回退本轮三个视频修复提交后重新部署。
3. 回滚后普通 OpenAI/Sora 视频协议将统一恢复为提交 `POST /v1/videos`、轮询 `GET /v1/videos/{task_id}`，出站 `seconds` 恢复原字段类型。
4. 已创建的公开任务 ID 与上游任务 ID 分离机制不受本轮影响；必要时保留旧版本服务完成存量任务轮询后再切换。

## 剩余风险与权限

- NodyHub 的公开端点目录把任务查询标为 POST，但生产实证要求 GET；来源 metadata 保留原声明，运行适配以实证为准。
- 本轮只验证一个最低成本 Veo 模型；其他 NodyHub 私有视频协议仍需在各自发布前独立验证，不能由目录标签推断可执行性。
- 上游媒体 URL 有过期时间；CarLab 内容代理按现有策略实时读取 URL，没有新增长期媒体归档。
