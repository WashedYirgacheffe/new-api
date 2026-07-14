# 核心模型生产闭环维护记录

- 日期：2026-07-15
- 状态：实施中
- CarLab API 分支：`codex/oem-api-hub`
- Superseed 分支：`codex/ai-service-migration`

## 范围

本轮按文字、图片、视频顺序完善超级种子常用 DeepWL 模型的生产闭环：修复 GPT Image 2 与 Omni 合同门禁，迁移超级种子现有 Banana 分辨率倍率，按上游文档补齐能力合同，真实调用 `nanoBananaPRO`、`nanoBanana2` 与 `Omni Fast`，并让 TapDash 生成记录展示完整的简体中文计费链路。

## 假设与边界

- CarLab API 继续作为网关模型、能力合同、采购价格、路由组和上游消费日志的唯一真相源。
- TapDash 只管理应用采用、站点模型集合、用户积分和生成账本，不重新开放模型技术合同编辑。
- 前端只展示预计积分；预扣、最终结算和退款只能由 Generation BFF 服务端执行。
- 模型能力以上游 API 文档为准落地，不要求每个参数组合逐项付费验证；用户批准的三款模型各执行一次真实调用，且不自动重试。
- Banana 分辨率沿用超级种子生产倍率：PRO 为 `1K×1、2K×1.25、4K×1.5`，Banana2 为 `1K×1、2K×1.2、4K×1.5`；CarLab 当前上游基础价仍作为计价基数。
- API Key、Token、Cookie、渠道凭据和真实媒体签名地址不写入源码、文档、Git 或命令输出。

## 计划改动

- 修正目标模型元数据端点与 Profile 端点不一致造成的 `dispatch_ready=false`。
- 为 Banana 合同增加分辨率参数倍率，并确保报价、同步调用和消费日志使用同一规则。
- 在 CarLab 生产环境执行三款获批模型的单次真实调用，核对服务 Token 分组、报价和消费日志。
- 扩展 TapDash 生成记录，显示网关模型、计费分组、上游预计金额、零售倍率、预计积分、预扣积分、实际积分和退款状态。
- 保留 TapLater 旧版节点与弹出式参数交互，只由动态合同替换模型和参数来源。

## 云资源与配置

- Railway 项目 `carlab-api`，服务 `new-api`、PostgreSQL、Redis。
- Cloudflare 生产入口 `https://api.carlab.top`。
- Vercel 项目 `superseed-dash` 与 `superseed`；主站生产切流必须在本轮门禁全部通过后按文字、图片、视频分阶段执行。
- Supabase 项目保存应用模型、生成运行、任务和积分流水。

## 验证证据

- 待补：定向测试、构建、线上目录与报价。
- 待补：三款真实调用和消费日志对账。
- 待补：TapDash 账务记录与 Preview 验收。

## 回滚

1. 在 TapDash 关闭受影响应用模型并保持主站 Generation BFF 生产开关关闭。
2. 回退 CarLab API 与 Superseed 本轮提交，重新发布上一成功版本。
3. 恢复目标模型原端点元数据、Binding 和参数倍率，确认旧模型不被错误上架。
4. 验证 `/api/status`、Token 模型目录、用量日志和既有文字/图片/视频代表模型仍正常。

## 剩余风险与权限

- DeepWL 页面价格只能作为参考；若没有可关联请求 ID 的上游账单接口，仍需人工核对余额变化或账单明细。
- DeepWL 未承诺 `Idempotency-Key` 的完全去重语义，真实调用失败后不得自动重放。
- TapLater 主站当前仍运行旧生产代码；本轮先完成 Preview 和计费门禁，不在证据不足时直接切换全部生产流量。
