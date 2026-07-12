# Superseed Dash 与模型能力契约调查记录

- 日期：2026-07-12
- 状态：调查与方案设计完成，未开始代码迁移
- 操作目录：`/Volumes/CODE/Code_SYS/CarLabAPI`、`/Volumes/CODE/Code_SYS/超级种子`
- CarLab API 分支：`codex/oem-api-hub`

## 范围

本轮只进行功能架构调查，目标是确定 TapDash 如何统一管理 CarLab API 渠道和模型，以及 TapLater 如何从模型硬编码迁移到结构化能力契约。没有修改超级种子运行代码、Supabase 数据库、Railway、Cloudflare 或 Vercel 配置，也没有切换任何生产调用。

长期设计写入 `docs/architecture/superseed-dash-capability-contract-design.md`。

## 假设

- CarLab API 继续是供应商渠道、路由、协议适配和网关计费的权威系统。
- TapDash 继续使用 Vue + Element Plus，不复制 New API React 页面源码。
- TapLater 继续负责画布交互、超级种子积分、工作区和 OSS，但不继续管理上游 Provider。
- 本轮按用户要求优先分析功能落地，没有展开身份打通、权限隔离和凭据保护设计。
- 超级种子实际迁移仍需在 CarLab API Profile、Quote 和模型测试能力完成后分阶段确认。

## 调查证据

### TapDash

- `CarLab/TapDash/api/admin-models.js` 保存独立硬编码模型目录。
- `CarLab/TapDash/src/views/content/ModelsView.vue` 仅有本地筛选、单条开关和 JSON 导入。
- `CarLab/TapDash/api/content.js` 逐模型执行导入。
- `StatusView.vue`、`health-probe.js`、`health-status.js` 分别维护硬编码 Provider。
- `FallbackConfigView.vue` 再次硬编码图片模型列表，只支持图片兜底。
- `UserGroupsView.vue` 管理本地积分倍率，但没有 CarLab 计费与路由组绑定。

### TapLater

- `_models.ts` 维护模型、Provider、清晰度、比例和能力。
- Image/Video/Text Node 根据模型 ID 和 Provider 计算控件、兼容性和请求参数。
- `materialValidation.ts` 仅对少量模型维护素材规格。
- `creditsStore.ts` 维护模型成本和参数化价格。
- `generateImage.ts`、`generateVideo.ts` 和服务端 `api/images/generate.js` 分别执行 Provider 路由、请求转换、结果解析和轮询。
- `TapCanvas.vue` 的节点清洗只保留固定字段，未来动态参数需要新增通用 `parameters`。

### CarLab API

- Channel 模块已经支持分页、筛选、批量启停、标签、模型抓取、余额、测试、复制和 Multi-Key。
- Channel 配置已经包含模型映射、group、优先级、权重、参数覆盖和请求头覆盖。
- Model 模块已经区分 Model Provider、API Channel Provider、模型类型、来源、标签和端点。
- Relay Adapter 与 Task Adapter 已覆盖标准同步请求、异步提交/轮询、结果归一化和动态计费钩子。
- Billing Expression 和 Other Ratios 可以读取请求参数参与计费。
- `system_tasks` 可以扩展为持久化模型批量测试任务。
- OpenAPI 目前是端点级参考，缺少每个模型的能力枚举、素材规则和 UI 信息。

## 设计结论

- 新增“模型能力契约”，作为文档、TapDash Profile 编辑器和 TapLater 动态控件的共同来源。
- 新增“渠道协议适配”层，将请求模板、轮询和响应提取留在 CarLab API。
- 使用标准协议、声明式适配和代码适配三档策略，不追求所有协议纯配置化。
- TapDash 新增 AI 服务中心，拆分 API 渠道、网关模型、能力模板、应用模型、测试中心和生成记录。
- TapLater 增加动态 Profile Store、控件注册表、统一 Generation BFF 和节点通用参数对象。
- TapLater 价格展示改为 CarLab Quote，实际积分扣费和退款仍由超级种子服务端完成。
- 迁移顺序继续采用基础契约、Dash、动态控件、文本、图片、视频和 OEM 的分阶段方式。

## 变更文件

- `docs/architecture/superseed-dash-capability-contract-design.md`
- `docs/operations/2026-07-12-superseed-dash-capability-audit.md`
- `AGENTS.md` Operations Index

没有修改 Go、React、Vue、SQL、部署配置或云端数据。

## 云资源与配置

| 资源 | 本轮状态 |
| --- | --- |
| Railway `carlab-api` | 未修改、未部署 |
| Cloudflare `api.carlab.top` | 未修改 |
| Vercel `superseed` | 未修改 |
| Vercel `superseed-dash` | 未修改 |
| Supabase | 未执行迁移或数据写入 |
| API Key / 管理员凭据 | 未读取、未输出、未写入文档 |

## 验证

- 对 TapDash 路由、导航、模型、监控、兜底、生成记录、用户分组和 API 客户端进行了只读代码检查。
- 对 TapLater 模型目录、节点控件、素材校验、积分、Provider 路由、图片服务端编排、任务表和画布节点清洗进行了只读检查。
- 对 CarLab API Channel/Model 前端、管理 API、模型元数据、参数覆盖、OpenAPI、Relay/Task Adapter、Billing Expression 和 System Task 进行了只读检查。
- 确认当前 OpenAPI 不能独立表达模型级枚举与素材条件，方案采用 OpenAPI + JSON Schema + UI/Material Schema。
- 本轮没有运行本地测试或构建，因为没有代码行为变更。

## 回滚

本轮只新增文档和索引。如需回滚，删除两份新增文档并移除 `AGENTS.md` 对应索引行即可；不涉及数据库或云资源回滚。

## 剩余风险与待确认事项

- 模型能力契约的数据库结构和 API 仍需实现并验证跨 SQLite、MySQL、PostgreSQL 兼容。
- Quote API 需要复用真实计费路径，不能建立第三套价格计算。
- Wuyinkeji 和部分 DeepWL 私有接口需要逐个判断声明式适配或 Go Adapter。
- 旧画布字段迁移、Profile 版本固定和复制/粘贴兼容是 TapLater 切换前的阻断项。
- TapDash 到 CarLab API 的管理员身份和权限边界按本轮范围暂未设计。
