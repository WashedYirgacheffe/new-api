# 模型合同 I/O 交互维护记录

- 日期：2026-07-21
- 状态：本地验证完成，浏览器验收受本机环境阻塞
- 分支：`codex/oem-api-hub`
- 目标入口：`https://api.carlab.top/models/contracts`

## 范围

本轮修复模型合同页在固定布局中无法滚动的问题，为全量网关模型增加远程联想选择，并在模型详情展示模型类型、可接受上游输入、输出类型与下游连接规则。对应的 TapLater 变更会使用同一份 effective contract 判断文本和媒体连接。

## 假设与边界

- CarLabAPI 继续作为 Profile、Binding、有效合同和模型元信息的唯一真相源。
- 输入能力由 `input_schema` 与 `material_schema` 派生；输出由 operation 与 `model_type` 派生。
- 不保存上下游模型 ID 列表，不把网关候选路由解释为画布连线。
- 不新增数据库字段，不修改 `ModelOperationEffectiveContract`，因此不改变合同 hash。
- 本轮不修改渠道、价格、密钥、环境变量或生产数据，不执行付费生成。
- 工作区已有 Omni 合同与验收证据改动属于其他任务，本轮不覆盖、不提交。

## 计划变更

- 为合同页根容器增加唯一纵向滚动边界。
- 基于 `/api/models/search` 增加防抖远程模型选择器，合同详情与 Binding 共用当前模型。
- 从模型元信息和选中 Binding 派生 I/O 能力摘要。
- 增加纯逻辑回归测试，并执行 TypeScript、lint、format 与生产构建检查。

## 云资源与配置

- 目标运行资源仍为 Railway 项目 `carlab-api` 的 `new-api`、PostgreSQL、Redis，以及 Cloudflare 入口 `api.carlab.top`。
- 不新增或修改 Railway、Cloudflare、GitHub、DNS、数据库和环境变量配置。

## 验证证据

- `bun test web/default/src/features/models/lib/model-contract-capabilities.test.ts`：通过，6/6。
- `bun run typecheck`：通过。
- `bun run lint -- --fix` / `bunx oxlint`：通过。
- `bun run format` / `bunx oxfmt`：通过。
- `bun run build`：通过。
- `git diff --check`：通过。
- 浏览器交互验收：未完成。本机 Browser 插件返回 `No browser is available`；Playwright fallback 缺 Chromium 二进制，未安装新的浏览器依赖，因此未完成 `/models/contracts` 页面滚动、模型搜索、键盘选择和 I/O 摘要截图验收。
- 云资源与配置：无变更；未修改 Railway、Cloudflare、GitHub、DNS、数据库和环境变量配置。

## 回滚

回退本轮 CarLabAPI 前端与运维文档提交并通过既有 Git/Railway 流程重新部署。没有数据库或配置回滚步骤。

## 剩余风险与权限

- I/O 摘要为现有合同字段的确定性派生，不替代服务端运行时 schema 校验。
- 自定义精确模型 ID 若不存在元信息，页面仍可尝试读取 Binding；无法从元信息或合同派生时才显示未知。
- 生产部署与付费模型验收需要单独发布窗口和授权，本轮本地实现不宣称已上线。
