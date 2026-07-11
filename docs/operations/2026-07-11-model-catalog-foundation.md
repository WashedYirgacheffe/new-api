# CarLab 模型目录与渠道商基础维护记录

- 日期：2026-07-11
- 状态：已完成
- 操作目录：`/Volumes/CODE/Code_SYS/CarLabAPI`
- 代码分支：`codex/oem-api-hub`

## 目标与边界

本轮在不修改 New API 分组倍率计费主链的前提下，明确区分模型供应商与 API 渠道商，并建立可追溯到上游公开文档或页面的正式模型目录基础。同步准备 OEM 数据隔离边界和 TapDash 后续适配说明。

本轮不修改超级种子模型调用代码，不迁移文本、图片或视频请求，不删除其现有供应商变量。超级种子分阶段迁移必须等待用户完成 CarLab API 测试并再次确认。

## 假设

- 模型供应商指模型知识产权或官方服务主体，例如 Google、OpenAI、ByteDance、Kuaishou。
- API 渠道商指提供聚合、转售或兼容 API 的平台，例如 DeepWL、Wuyinkeji、DMXAPI。
- New API 现有 `vendors` 继续承载模型供应商；渠道新增独立归属字段，不复用协议类型或运营标签。
- 商业计费继续使用 New API 的模型价格、分组倍率和额度规则，本轮只增加目录元数据。

## 上游公开来源

- DeepWL：`https://zx1.deepwl.net/api/pricing`、`https://doc.deepwl.cn/llms.txt`
- Wuyinkeji：`https://api.wuyinkeji.com/doc` 及具体 `/doc/{id}` 页面
- DMXAPI：`https://doc.dmxapi.cn/`、`https://doc.dmxapi.cn/model-list` 及具体模型文档页

## 代码与文档变更

- `model/channel.go` 新增 `channel_provider`，并在渠道校验、权限字段分类、搜索和前端创建/编辑表单中贯通。
- `model/model_meta.go` 新增模型展示名、模型类型和来源 URL；绑定渠道记录返回渠道商代码。
- 模型管理表显示模型供应商、模型类型、来源域名和带渠道商的绑定渠道；渠道管理表直接显示 API 渠道商。
- `docs/catalog/initial-model-catalog.json` 建立 14 条首批有来源目录记录，覆盖 DeepWL、Wuyinkeji、DMXAPI 文档中与超级种子相关的文本、图片和视频模型。
- `docs/architecture/model-catalog-oem-design.md` 固化术语、来源优先级、New API 分组倍率计费边界、OEM 对象映射和迁移门槛。
- 超级种子新增独立 TapDash 适配文档，本轮没有修改 TapDash 页面、Supabase schema 或 TapLater 调用代码。
- 通过系统设置将部署产品名更新为 `CarLab API`，保留 New API 与 QuantumNous 的开源归属和许可信息。

## 云资源

| 资源 | 本轮用途 |
| --- | --- |
| Railway `carlab-api/new-api` | 云端编译、数据库自动迁移和目录接口验证 |
| Cloudflare `api.carlab.top` | 控制台、模型目录和 API 公网验证 |
| Vercel `superseed-dash` | 本轮仅形成适配方案，不部署代码 |

## 验证证据

- 前端 `bun run typecheck` 通过；涉及的 TS/TSX 文件定向 oxlint 无错误；目录和全部 locale JSON 通过 `jq` 解析。
- 本机没有 Go 工具链，未运行本地 Go 测试或 `gofmt`。Railway Docker 云端构建成功完成 Go 编译、前端生产构建、PostgreSQL AutoMigrate、容器启动和 `/api/status` 健康检查。
- 第一条兼容字段部署 `55305e32-c954-47ef-8802-591bcab1e2d2` 为 `SUCCESS`；最终前端部署 `4b21b8e6-4a80-4e6a-8c42-b4eebe76bcdd` 为 `SUCCESS`。
- 现有 5 个渠道已补齐渠道商：`dmxapi` 1 个、`nodyhub` 1 个、`siliconflow` 1 个、`volcengine` 2 个。
- 模型目录包含 14 条首批记录，全部具有 `display_name`、`model_type`、`description`、模型供应商和 `source_url`。
- `gpt-4o-mini` 目录记录反查到 DMXAPI 与 Nodyhub 两个绑定渠道，并分别返回渠道商代码。
- `GET /api/status` 返回产品名 `CarLab API`，`self_use_mode_enabled=false`，证明没有为未定价模型放宽规则。
- 超级种子业务令牌请求 `GET /v1/models` 返回 HTTP 200 和 154 个当前可路由模型。
- 同一令牌真实调用 `POST /v1/chat/completions`，`gpt-4o-mini` 返回 HTTP 200 和预期内容 `catalog-ok`。
- 本轮没有调用 `ModelPrice`、`ModelRatio`、`CompletionRatio` 或 Group Ratio 的写接口，商业计费仍使用 New API 原生规则。

## 回滚

1. Railway 回滚到上一成功部署。
2. 新增数据库列为向后兼容的可空/默认字段，旧版本可忽略；如需物理删除，必须另行评估数据后执行迁移。
3. 目录元数据可通过管理 API 禁用或删除，不影响渠道 Key 与实际路由能力。
4. 控制台名称可通过系统设置恢复，不删除上游项目归属。

## 剩余风险与权限

- Wuyinkeji 与 DMXAPI 没有稳定公开 OpenAPI 目录，文档快照需要定期人工复核。
- 渠道商模型名称可能与模型原厂命名不同，正式迁移前必须建立别名映射并逐模型验证。
- OEM 租户级控制台、账单聚合和权限隔离不在本轮实现范围。
- DeepWL/FOX 和 TokenShen 当前凭据仍无效，Wuyinkeji 仍需私有协议适配器；目录记录不代表对应渠道已经可调用。
- 超级种子分阶段迁移仍处于等待用户确认状态。
