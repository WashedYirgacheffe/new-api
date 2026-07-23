# RE Async 生产准备补齐

## 范围与假设

- 修复 RE 异步模型此前统一按 `video.generate` 准备合同的问题，使图片、视频、音频和文本工具分别选择对应操作类型。
- RE 上游 OpenAPI 明确允许模型自定义字段；在逐模型专属合同尚未发布时，CarLabAPI 应保留顶层参数和 `metadata` 展平参数，不应套用其他协议的同步合同。
- 本轮收到的正式密钥仅对 `https://reapi.ai/api/v1` 有效；对 `https://api.reapi.ai` 返回未授权，因此不得复用为 Chat 密钥。

## 变更文件

- `relay/channel/task/reapi/adaptor.go`：允许无 `prompt` 的音频/工具任务，保留 RE 自定义顶层参数，覆盖上游模型映射并清除 CarLab 路由字段。
- `relay/relay_task.go`、`relay/helper/model_contract_pricing.go`：按 RE 模型类型选择操作，并仅应用端点类型为 `re-task` 的专属合同。
- `controller/relay.go`：任务查询持续返回 `re/<model>` 公共模型名，不回退成上游裸模型 ID。
- `cmd/reapi-onboard`：允许 Chat 与 Async 按已存在的密钥独立初始化；仍保持渠道手动停用、模型目录停用。
- 对应单元测试覆盖无提示词音频请求、自定义字段透传、操作类型映射和单密钥初始化。

## 云资源与配置

- Railway 项目：`carlab-api`，服务：`new-api`，环境：`production`。
- 已安全设置 `REAPI_TASK_API_KEY`，设置时跳过即时部署，等待 Git `main` 构建包含本轮代码。
- `REAPI_CHAT_API_KEY` 仍不存在；未创建伪 Chat 渠道，未把 Async 密钥写入 Chat 配置。
- 未把任何密钥值写入 Git、Markdown、前端变量或命令输出记录。

## 验证证据

- 使用不存在的任务 ID 对 RE Async 做无费用鉴权探测，返回 `404 task not found`，确认凭据有效且未创建任务。
- 使用同一凭据读取 RE Chat 模型目录返回 `401`，确认两套凭据不可复用。
- `go test ./relay/channel/task/reapi ./relay ./router ./service ./cmd/reapi-onboard -count=1` 通过。
- `go test ./relay/common -count=1` 与 RE 任务归属查询测试通过。
- `go run ./cmd/reapi-onboard --dry-run` 确认 104 条目录与运行时映射一致。

## 回滚

1. 保持或恢复 `RE Async` 渠道为手动停用，不启用任何 `re/*` 模型。
2. 回滚本轮 Git 提交并由 Railway 从 `main` 重新部署。
3. 如需撤销凭据配置，从 Railway 删除 `REAPI_TASK_API_KEY`；不得在日志或工单中粘贴其值。
4. 如需移除初始化数据，先备份 `re` 渠道、模型、供应商和能力映射，只删除 `channel_provider = re` 的记录。

## 剩余风险与权限

- 尚未获得 `REAPI_CHAT_API_KEY`，因此 Chat 仍未初始化和验收。
- 本轮不发起付费生成；图片、视频、音频/工具仍需在明确价格和成本上限后逐类实测。
- 104 个模型与 RE Async 渠道继续默认停用；初始化成功不等于已对用户开放。
