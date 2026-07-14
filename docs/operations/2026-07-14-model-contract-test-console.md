# CarLab 模型合同测试台维护记录

- 日期：2026-07-14
- 状态：代码验证完成，待 Railway 发布
- 分支：`codex/oem-api-hub`
- 公网入口：`https://api.carlab.top`

## 范围

本轮修复 CarLab API 合同管理入口不直观的问题，并在模型合同页面增加 Token 作用域的 Profile、Quote 与 Gemini 图片真实调用验证台，使管理员无需回到 TapDash 或命令行即可核对模型合同、有效分组、预计金额和真实输出。

## 假设与边界

- CarLab API 继续作为 Profile、Binding、采购路由和计价合同的唯一真相源。
- 测试 Token 只保存在浏览器组件内存和本机 Keychain，不写入源码、数据库、文档、构建变量或日志。
- 测试台第一阶段只执行当前已验证的同步 Gemini 图片协议；其他协议仍可读取 Profile 与 Quote，但不伪装成已支持真实调用。
- 公开 Pricing 页面只作参考；Quote 与真实消费日志用于生产价格验收。

## 已完成改动

- 在管理员侧栏增加清晰的“模型合同与测试”入口，并纳入现有模型权限开关。
- 优化合同页面标题与说明，明确模型 ID、Profile、Binding、Quote 和真实调用的操作顺序。
- 增加不持久化 Token 的 Profile/Quote 测试表单和 Gemini 图片结果摘要。
- 使用专用 CarLab API Token 验证 `nanoBananaPRO` 与 `nanoBanana2` 的合同和报价；真实生成由管理员在页面中单次触发，避免重复采购。

## 管理员操作步骤

1. 登录 `https://api.carlab.top` 管理后台。
2. 在左侧“管理”区域打开“模型合同与测试”；也可直接访问 `/models/contracts`。
3. 在“测试 API 密钥”输入框粘贴本次测试 Key。Key 只保存在当前页面内存中，刷新后清除。
4. 输入精确网关模型 ID：
   - `nanoBananaPRO`：`deepwl/gemini-3-pro-image`
   - `nanoBanana2`：`deepwl/gemini-3.1-flash-image-preview`
5. 保持操作为 `image.generate`，在报价参数中填写需要核价的比例、分辨率等结构化参数。
6. 点击“加载合同与报价”，核对调度状态、请求适配器、实际计费分组、分组倍率、基础价格、预计额度和预计金额。该操作不生成媒体。
7. 需要验证真实输出时，只点击一次“执行 Gemini 图片真实测试”。按钮不会自动重试；调用会产生实际上游费用。
8. 打开“用量日志”核对该次调用的实际结算，再与 Quote 和上游账单对账。

密钥不会预填到源码、数据库、浏览器存储或 Railway 环境变量。这样可以避免管理前端包、页面会话或部署配置泄漏可消费额度的 Token。

## 云资源与配置

- Railway 项目 `carlab-api`，服务 `new-api`、PostgreSQL、Redis。
- Cloudflare 入口 `api.carlab.top`；本轮不修改 DNS、Worker 或源站密钥。
- 不新增环境变量；测试 Token 不进入 Railway 或 GitHub 配置。

## 验证证据

- `bun run typecheck`、涉及文件定向 `oxlint`、`oxfmt --check` 和 `git diff --check` 均通过。
- `bun run build` 通过，Rsbuild 生产产物构建成功。
- `deepwl/gemini-3-pro-image`：Profile `image.generate.gemini-native@1`，`dispatch_ready=true`，当前 Key 生效分组 `default`、倍率 `1`、基础价与预计金额均为 `0.6`。
- `deepwl/gemini-3.1-flash-image-preview`：Profile `image.generate.gemini-native@1`，`dispatch_ready=true`，当前 Key 生效分组 `default`、倍率 `1`、基础价与预计金额均为 `0.25`。
- 按管理员自行在测试台触发真实生成的操作边界，本轮未执行两款高价模型的实际生成，因此没有新增媒体消费或结算日志。
- 待补：Railway deployment、生产路由和界面验收。

## 回滚

1. 回退本轮前端与文档提交。
2. 从回退提交重新部署 Railway `new-api`。
3. 验证原 `/models/metadata`、`/models/contracts`、Profile 与 Quote API 仍正常。

## 剩余风险与权限

- 浏览器插件当前初始化失败，错误为 `Cannot redefine property: process`；完成代码后需用可用的浏览器会话或用户侧生产页面补充交互验收。
- DeepWL 未公开承诺 `Idempotency-Key` 去重语义，真实测试不得用自动重试推断 exactly-once。
- 当前测试 Key 的有效分组是 `default`，不是 Superseed 专属金牌组；页面展示的是 Token 的实际分组结果，不会替管理员隐式改组。
