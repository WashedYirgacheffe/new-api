# 核心模型生产闭环维护记录

- 日期：2026-07-15
- 状态：CarLab 生产就绪，Superseed Preview 验收通过，主站生产切流待确认
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

## 已完成改动

- 修正目标模型元数据端点与 Profile 端点不一致造成的 `dispatch_ready=false`。
- 为 Banana 合同增加分辨率参数倍率，并确保报价、同步调用和消费日志使用同一规则。
- 在 CarLab 生产环境执行三款获批模型的单次真实调用，核对服务 Token 分组、报价和消费日志。
- 扩展 TapDash 生成记录，显示网关模型、计费分组、上游预计金额、零售倍率、预计积分、预扣积分、实际积分和退款状态。
- 保留 TapLater 旧版节点与弹出式参数交互，只由动态合同替换模型和参数来源。

## 变更文件

- `model/model_operation_profile.go`：核心模型端点、Profile、Binding、Banana 分辨率倍率和 Omni 路径。
- `model/model_operation_profile_test.go`：核心合同迁移和分辨率倍率测试。
- `relay/helper/model_contract_pricing.go`：合同参数倍率进入实际采购计费。
- `relay/helper/model_contract_pricing_test.go`：图片分辨率计费回归测试。
- Superseed `CarLab/TapLater/packages/taplater/api/_generationDispatch.js`：兼容 Gemini 文本型 Base64 图片回收。
- Superseed `CarLab/TapDash/api/content.js` 与 `src/views/content/GenerationsView.vue`：生成运行和计费链路中文展示。

## 云资源与配置

- Railway 项目 `carlab-api`，服务 `new-api`、PostgreSQL、Redis。
- Cloudflare 生产入口 `https://api.carlab.top`。
- Vercel 项目 `superseed-dash` 与 `superseed`；主站生产切流必须在本轮门禁全部通过后按文字、图片、视频分阶段执行。
- Supabase 项目保存应用模型、生成运行、任务和积分流水。

## 验证证据

- Railway 部署 `a265c4d6-819d-4fc3-bce2-35d85f25ffa6` 在线，CarLab 目录同步 74 个模型；核心成功模型已进入 `超级种子核心模型` 集合。
- Flash 3.5 的 Superseed 端到端运行成功，CarLab `gold` 消费日志实际扣除 501 quota，即 0.001002 元；超级种子普通用户倍率 `1.8`，实扣 1 积分。
- nanoBanana2 的 1K 报价为 0.25 元，分辨率倍率为 `×1`；成功运行回收 1 张 JPEG Base64 图片，超级种子实扣 7 积分。
- nanoBanana2 首次 BFF 调用因 DeepWL 返回 `parts[].text` 纯 Base64 而解析失败；CarLab 已产生 0.25 元采购成本，超级种子预扣 7 积分完整退款。修复文本图片解析后复测成功。
- Omni Fast 的 4 秒、720p、16:9 报价和消费日志均为 750000 quota，即 1.50 元；异步任务提交、六次轮询和视频 URL 回收成功，超级种子实扣 41 积分。
- 隔离验收账号从 200 积分降至 151，净消耗 `1 + 7 + 41 = 49`；三条成功运行和对应 `ai_tasks` 均已写入生产 Supabase。
- Superseed 付费验收 Preview `https://superseed-6wk5nfekw-washedyirgacheffe-4517s-projects.vercel.app` 通过；提交 `cd4be2c` 的 Git Preview `https://superseed-lqw8omrzh-washedyirgacheffe-4517s-projects.vercel.app` 和固定分支别名均为 Ready，6 个核心模型全部可用，品牌图标存在且不显示 DeepWL 渠道标签。文字、图片、视频开关和动态目录生效，主站生产开关没有变更。
- nanoBananaPRO 两次真实请求均由上游返回 429 且净扣费为 0，继续保持 Draft 和停用；nanoBanana 仍会出现空结果但上游扣 0.10 元，不进入本轮新增生产范围。

## 回滚

1. 在 TapDash 关闭受影响应用模型并保持主站 Generation BFF 生产开关关闭。
2. 回退 CarLab API 与 Superseed 本轮提交，重新发布上一成功版本。
3. 恢复目标模型原端点元数据、Binding 和参数倍率，确认旧模型不被错误上架。
4. 验证 `/api/status`、Token 模型目录、用量日志和既有文字/图片/视频代表模型仍正常。

## 剩余风险与权限

- DeepWL 页面价格只能作为参考；若没有可关联请求 ID 的上游账单接口，仍需人工核对余额变化或账单明细。
- DeepWL 未承诺 `Idempotency-Key` 的完全去重语义，真实调用失败后不得自动重放。
- Gemini 同步图片响应的 JSON 体没有请求编号，Superseed 成功运行中的 `carlab_request_id` 暂为空；CarLab 消费日志仍保留对应请求编号，后续可在网关客户端补采响应头。
- TapDash 生产页面需要有效超管会话。本机钥匙串密码与当前 Vercel 生产密码不一致，本轮没有在模型验收任务中擅自轮换后台凭据。
- TapLater 主站仍保持 Generation BFF 生产开关关闭；应等待并行画布状态机集成，再按文字、图片、视频分类型切流。
