# Superseed 报价与派发路由组一致性

- 日期：2026-07-22
- 状态：待 Railway 与 Vercel Preview 部署后进行最低成本真实文本验收
- 代码分支：`codex/oem-api-hub`、`codex/canvas-runtime-p0-p2`

## 范围

TapLater 的分站目录和报价已确认超级种子使用 `gold` 路由组，但原先的标准 `/v1` 生成请求没有带回该选择，CarLabAPI 按业务 Token 的 `auto` 组重新选渠道并返回模型不可用。该变更让 Generation BFF 以受信任 header 传递合同记录的路由组，并由 CarLabAPI 在正常 Token relay 中校验和锁定它。

## 假设与权限边界

- `expectedCarLabGroup` 只能由 TapLater 服务端的分站目录、报价校验和持久化 run 恢复链路产生；浏览器请求的 `group` 和 `parameters.group` 继续被拒绝。
- `X-CarLab-Route-Group` 仅允许具体组名，TapLater 缺失或得到 `auto` 时拒绝提交，不回退到自动派发。
- 固定分组 Token 只能请求自身组；无固定组或 `auto` Token 仍必须通过所属用户可用组校验。读取后立即剥离 header，不会转发给渠道上游。

## 代码变更

- `middleware/distributor.go`：标准 `/v1`/`/v1beta` relay 读取并校验 `X-CarLab-Route-Group`，在渠道选择前同步更新 `UsingGroup` 和 `TokenGroup`。
- `middleware/distributor_playground_test.go`：覆盖 header 优先于请求体 `group` 且不保留在请求 header 中。
- Superseed `api/_generationDispatch.js`：文字、图片、视频提交使用持久化的合同路由组；当前视频轮询也携带同一 header，旧 run 缺失该字段时仍可按任务 ID 读取状态。

## 云资源与配置

| 资源 | 影响 |
| --- | --- |
| Railway `carlab-api/new-api` | 部署后 `api.carlab.top` 标准 Token relay 接受受校验的内部路由组 header |
| Vercel `superseed` Preview | 使用既有 `CARLAB_SERVICE_TOKEN`、`CARLAB_ROUTE_TOKENS_JSON`、`CARLAB_SUBSITE_DOMAIN` 与 `CARLAB_SUBSITE_ROUTE_GROUP`；不增加明文凭据 |
| Supabase generation runs | 已保存的 `expected_carlab_group` 作为异步恢复的可信来源 |

## 验证证据

- CarLabAPI middleware 聚焦 Go 测试通过，确认标准 `/v1` 路径采用 header 并且 header 已删除。
- TapLater `generation-dispatch` 和 `generation-orchestrator` 专项 37/37 通过，覆盖文字、Gemini 图片、异步视频提交/轮询和缺失组的提交前失败。
- 未执行付费图片或视频生成。部署后只执行一次 1-credit 文本生成，确认 CarLab 真实日志使用 `gold`，再清理临时验证用户和旧的未知提交 hold。

## 回滚

1. Railway 回退到上一成功 deployment 可恢复原有 Token relay 行为。
2. Vercel Preview 回退 TapLater commit 可停止发送新 header。
3. 对已标记 `submission_unknown` 且已确认上游未提交的 run，使用已有 reconciliation 授权动作释放 hold；不得自动重发。

## 剩余风险与权限

- 实际渠道仍可能因上游余额或临时不可用失败，但不再因 `auto` 与报价组不一致造成错误选路。
- `gold` 必须继续存在于超级种子业务 Token 所属用户的可用组内；若运营调整组权限，CarLabAPI 会返回受控 403 而非越权路由。
- 本轮不改变分站认领、模型集合或 TapDash 权限：这些控制面仍只在 `api.carlab.top`。
