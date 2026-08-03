# RE Seedance 联网搜索出站格式修复

## 范围

- 修复 `doubao-seedance-2.0-face` 与 `doubao-seedance-2.0-fast-face` 通过 RE 异步视频端点生成时，`tools` 被错误发送为 boolean 而返回 422 的问题。
- TapLater 和 CarLab `effective_contract` 继续把联网搜索呈现为 `tools:boolean` 开关；RE task adapter 在供应商边界转换为上游 wire 格式。
- `seedance-2.0-mini` 继续使用顶层 `web_search:boolean`，不转换为 `tools`。
- 本轮不修改模型参数枚举、价格、Binding、数据库 schema、环境变量或密钥，也不执行付费生成和页面验收。

## 假设与权威证据

- 具体模型 API reference `https://reapi.ai/docs/seedance-2-0` 优先于旧快照和通用说明。
- RE 当前 wire schema 要求 `tools` 为对象数组，已文档的唯一元素为 `{"type":"web_search"}`；页面序列化规则为开关开启时发送该数组，关闭时不发送 `tools`。
- Face/Fast Face 的 `nsfw_checker:boolean` 必须原样保留，包括显式 `false`；Mini 的 `web_search:boolean` 同样原样保留。

## 变更文件

- `relay/channel/task/reapi/adaptor.go`：仅对两个精确 Face 模型把 `tools:true` 转为 `[{"type":"web_search"}]`，把 `tools:false` 删除。
- `relay/channel/task/reapi/adaptor_test.go`：覆盖 Face 开启、Fast Face 关闭、`nsfw_checker:false` 保留和 Mini `web_search:true` 不被改写。
- `cmd/reapi-contract-snapshot/seedance_overrides.test.mjs`：锁定 Mini 的 `web_search:boolean` UI 合同，防止逻辑合同与 wire 合同再次混淆。
- `AGENTS.md`：登记本运维记录。

## 云资源与配置

- 目标资源为 GitHub `WashedYirgacheffe/CarLabAPI` 的 `main`、Railway 项目 `carlab-api` 的 `new-api` 服务，以及 Cloudflare 入口 `https://api.carlab.top`。
- 不修改 Railway PostgreSQL、Redis、Cloudflare、Vercel、Operation Binding revision、`REAPI_TASK_API_KEY` 或其他配置。
- TapLater 无运行代码变化；已有 P0-P2 Preview 继续发送合同定义的 boolean，由 CarLab 完成上游序列化。

## 验证证据

- 修复前定向测试复现：开启时实际值仍为 `true`，关闭时请求仍包含 `tools:false`。
- `go test ./relay/channel/task/reapi -count=1`
- `node --test cmd/reapi-contract-snapshot/seedance_overrides.test.mjs`
- `go test ./cmd/reapi-onboard -count=1`
- `git diff --check`
- `go test ./... -count=1` 已执行，本轮相关包均通过；全仓仍有两项既有基线失败：根包缺少未构建的 `web/classic/dist` 嵌入产物，以及 `controller.TestListModelsTokenLimitIncludesTieredBillingModel` 未发现其隔离测试模型。这两项已在此前 Seedance 运维记录中登记，不由本轮改动引入。
- 按用户要求不打开页面、不截图、不调用付费模型；因此本轮只证明静态请求 JSON 与文档一致，不把能力标记为真实调用已测。

## 发布状态

- 开发提交 `6a4c713e` 已推送到 `WashedYirgacheffe/new-api:codex/oem-api-hub`；生产没有合并已分叉的开发分支，而是从最新 `CarLabAPI/main` 精确 cherry-pick 为提交 `824d8e2d`，保留生产独有的 DeepWL/NodyHub 运行时改动。
- `824d8e2d` 已推送到 `WashedYirgacheffe/CarLabAPI:main`，由 GitHub 自动触发 Railway 部署 `7e150b91-1e03-4fc4-9e1e-718a5ae33f1a`；状态为 `SUCCESS`，镜像摘要为 `sha256:94b1a2f6935cc1fc17e46195bd68d7fe9644ce43aafebea31a95db00bda42adf`。
- Railway 容器完成 PostgreSQL 初始化检查后正常启动，部署健康检查 `GET /api/status` 返回 200；Cloudflare 入口 `https://api.carlab.top/api/status` 同样返回 HTTP 200。未使用本地 `railway up`，未提交视频生成任务。

## 回滚

1. 回退本轮 CarLab 提交并推送 `main`，由 GitHub/Railway 自动部署上一版。
2. 紧急止损可暂时在模型合同中隐藏 Face/Fast Face 的 `tools` 开关；无需回滚数据库或恢复 Binding revision。
3. TapLater 不含本轮供应商 wire 特判，无需回退或重新部署前端。

## 剩余风险

- 未执行真实付费生成，无法证明 RE 运行时在所有账号、区域和模型变体上与公开文档完全一致。
- 该转换只覆盖两个精确 Face 模型；标准、official 和 Mini 保持各自现有合同，不能把本规则泛化到其他模型。
- 上游若新增其他工具类型，必须先取得具体模型文档并扩展合同与测试，不能把任意 boolean 或对象宽松透传。
