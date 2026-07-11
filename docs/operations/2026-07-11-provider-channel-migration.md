# 现有供应商渠道迁移维护记录

- 日期：2026-07-11
- 状态：已完成（标准兼容渠道已上线；私有协议与失效凭据未强行接入）
- 操作目录：`/Volumes/CODE/Code_SYS/CarLabAPI`
- 代码分支：`codex/oem-api-hub`

## 目标与边界

本轮把超级种子生产环境中现有且协议兼容的供应商凭据迁移到 CarLab New API，由 `https://api.carlab.top` 提供统一模型入口。DeepWL 与 FOX 视为同一上游体系，按现有凭据用途拆分为独立渠道，避免不同上游分组或余额规则互相污染。

用户明确要求继续使用现有凭据，本轮不执行密钥轮换。实际探测发现，被删除泄密文档中的旧 SiliconFlow、DMXAPI 与 Nodyhub 凭据均已被各自上游返回 HTTP 401，不能继续承担生产流量；旧 Wuyinkeji 凭据仍有业务响应，但属于私有协议。为保证统一入口可用，本轮标准渠道采用 Vercel Production 中仍有效的现行凭据。任何原始密钥、管理员密码、会话 Cookie 和业务令牌均不写入仓库或本文档。

私有异步协议不伪装成 OpenAI 兼容渠道：Wuyinkeji `/api/async/*`、DeepWL 特有素材与部分视频路径继续保留原调用链，等待后续适配器或协议重构。本轮优先保证标准 OpenAI、Gemini、图片及 New API 已支持的供应商链路可验证使用。

## 配置来源与渠道规划

配置只从 Vercel 项目 `superseed` 的 Production 环境临时读取到 `/tmp`。涉及的环境变量名包括：

- `FOX_KEY`
- `FOX_TEXT_KEY`
- `FOX_IMAGE_KEY`
- `FOX_SEEDANCE_KEY`
- `SILICONFLOW_KEY`
- `DMXAPI_KEY`
- `NODYHUB_KEY`
- `TOKENSHEN_KEY`
- `VOLC_KEY`
- `WUYINKEJI_KEY`

计划中的 DeepWL/FOX 渠道均使用 `https://zx1.deepwl.net` 作为 Base URL，不附加 `/v1`。每枚 FOX 凭据建立独立渠道，并只开放该凭据上游实际返回或项目已使用的模型。

## 变更范围

- New API 数据库中的渠道配置和模型能力记录。
- New API 中供超级种子使用的独立业务令牌。
- macOS 钥匙串服务 `carlab-api-superseed-token`，用于本机运维读取完整业务令牌。
- Vercel 项目 `superseed` 的 `NEWAPI_BASE_URL` 和 `NEWAPI_KEY` Production 环境变量。
- 本维护记录与根 `AGENTS.md` Operations Index。
- 超级种子仓库删除泄密文档，并更新 BugBoard 进度记录。

不修改超级种子现有供应商调用代码，因此本轮不会立即停用旧的逐供应商环境变量。统一入口验证成功后，再以独立代码变更逐个切换调用路径。

## 云资源

| 资源 | 本轮用途 |
| --- | --- |
| Cloudflare Worker `carlab-api-edge` | 统一入口、源站鉴权与流式转发 |
| Railway 项目 `carlab-api` / 服务 `new-api` | 渠道配置、路由、配额、日志和业务令牌 |
| Vercel 项目 `superseed` | 读取现有供应商变量，并写入统一入口变量 |

## 验证证据

- New API 数据库新增 5 个渠道：SiliconFlow、DMXAPI、Nodyhub、VolcEngine 图片/文本、VolcEngine Seedance 视频。
- 上游模型目录探测结果：SiliconFlow 91 个、DMXAPI 640 个、Nodyhub 725 个；火山渠道按超级种子当前使用范围显式开放 1 个 Seedream 与 2 个 Seedance 模型。
- DMXAPI 与 Nodyhub 的 New API 内建渠道测试均以 `gpt-4o-mini` 真实调用成功。SiliconFlow 连接与模型目录正常，但所选新模型未配置 New API 价格，因此网关按规则拒绝调用；未开启全局自用模式绕过定价。
- 创建普通用户 `superseed-service`（普通角色）和独立令牌 `superseed-production`。令牌归属于业务用户，不归属于 Root；完整值只保存在 macOS 钥匙串服务 `carlab-api-superseed-token`。
- 使用业务令牌请求 `GET https://api.carlab.top/v1/models` 返回 HTTP 200，可见 154 个当前可路由且符合网关规则的模型，其中包含 `gpt-4o-mini`。
- 使用同一业务令牌请求 `POST https://api.carlab.top/v1/chat/completions`，`gpt-4o-mini` 返回 HTTP 200，响应内容为预期的 `carlab-ok`。
- Vercel 项目 `superseed` 的 Production 环境已写入 `NEWAPI_BASE_URL=https://api.carlab.top` 与 Sensitive 变量 `NEWAPI_KEY`。本轮没有使用 `vercel deploy`，后续代码切换由 Git 集成部署。
- FOX/DeepWL 四枚现行凭据与 TokenShen 凭据均被其上游 `/v1/models` 返回 HTTP 401，因此未创建伪可用渠道。FOX 与 DeepWL 的同源关系已确认，恢复有效凭据后应继续按 Text、Image/Gemini、Seedance、Default 四个分组拆分。
- 验证输出只记录渠道名、HTTP 状态、模型名与响应类型，没有记录原始凭据或完整模型响应。

## 回滚

1. 在 New API 管理后台禁用或删除本轮新增渠道。
2. 删除本轮创建的超级种子业务令牌，并从本机钥匙串删除 `carlab-api-superseed-token`。
3. 从 Vercel Production 环境删除 `NEWAPI_BASE_URL` 与 `NEWAPI_KEY`；现有逐供应商变量及当前代码路径不受影响。
4. 如需恢复误删文档，只能从确认不含有效凭据的净化版本重建，禁止恢复原始泄密文件。

## 剩余风险与权限

- 现有供应商密钥曾出现在仓库文档中。用户选择继续使用意味着接受凭据可能已泄露的剩余风险；应持续监控异常消费、来源 IP 和余额变化。
- New API 的模型倍率与供应商真实成本需要运营侧复核。当前只保证已定价模型可通过业务令牌调用；OEM 分发前必须完成价格、分组、配额和告警策略，不能开启全局自用模式代替定价。
- `superseed-service` 当前配置有限用户额度并签发无限令牌；网关仍受用户总额度约束。正式 OEM 分发时应按租户创建独立用户/令牌，并配置可审计额度，不共享此内部令牌。
- Vercel 已具备统一入口变量，但超级种子代码仍使用现有逐供应商代理。切换调用路径需要单独代码变更与回归，不能仅靠环境变量自动完成。
- 私有异步视频、素材和 Wuyinkeji 协议尚未统一，不能仅靠渠道配置实现完整迁移。
