# 超级种子模型目录治理收尾验收

## 范围

本轮是超级种子模型目录治理部署后的文档收尾与云端验收，不新增业务代码，不修改超级种子项目，不迁移任何调用路径。目标是把当前 Railway 版本、前端主题、后台入口、公开模型集合和渠道商筛选证据固定下来，供后续逐模型调试与 OEM 配置使用。

## 结论与纠正

- 当前活跃 Railway 部署为 `2a4dea53-81ed-4552-b5b5-f4a8ff97f228`，状态为 `SUCCESS`；筛选版本部署 `d9533046-4f05-41ab-b4b6-e0f14b4fd55e` 已被后续部署替换并显示为 `REMOVED`，不应继续把它当作线上当前版本。
- “未知供应商”是模型元数据缺少 `vendor_id`，不等于 API 渠道商未知；渠道商来自 `channels.channel_provider`。
- `/login` 能返回单页应用壳不代表它是登录路由；新前端规范登录入口是 `/sign-in`。
- ID 2 是服务账号，ID 3 是普通个人账号；ID 3 不是管理员。密码采用不可逆哈希，不能从数据库反查明文；Root 密码仅可从本机钥匙串读取，不能写入文档、聊天、Git 或日志。
- `/v1/models` 只公布已配置显式计费的模型。当前 4 个模型少于 28 个可路由目录，是价格配置边界，不是模型导入失败。

## 变更文件

- `docs/operations/2026-07-11-superseed-catalog-governance.md`：修正部署状态、登录入口、公开定价数量、主题配置和回滚说明。
- `docs/operations/2026-07-11-superseed-catalog-closeout.md`：记录本轮收尾、验证证据和后续操作顺序。
- `AGENTS.md`：增加本记录的索引和对应资源映射。

## 云资源与配置

| 资源 | 当前事实 |
| --- | --- |
| Railway 项目 `carlab-api` / 服务 `new-api` | Southeast Asia，活跃部署 `2a4dea53-81ed-4552-b5b5-f4a8ff97f228`，状态 `SUCCESS` |
| 公开域名 `https://api.carlab.top` | 通过 Cloudflare 边缘访问 CarLab API |
| `theme.frontend` | `default` |
| 渠道目录 | DMXAPI 16、Nodyhub 20、SiliconFlow 2、VolcEngine 视频 2；VolcEngine 图片渠道手动禁用 |
| 公开显式计费目录 | `gpt-4o-mini`、`gpt-image-1`、`sora-2`、`sora-2-pro` |

主题切换前备份：

- `~/Library/Application Support/CarLabAPI/backups/2026-07-11-pre-default-frontend-theme.json`
- SHA-256：`9b3e109fc71ebec8a758d813785745ac1e1e7b69536a1e478c0b24301bffd314`
- 内容为 `theme.frontend=classic`，且 `contains_secrets=false`

## 验证证据

### 接口

- `GET /api/status` 返回 `theme=default`。
- `GET /api/pricing` 返回 4 条公开价格记录，且每条都具有显式价格、倍率或有效分层计费配置。
- 无令牌访问 `GET /v1/models` 返回 HTTP 401。
- 使用已存在的业务令牌访问 `GET /v1/models` 返回 4 个模型，集合与 `/api/pricing` 完全一致。
- 使用同一业务令牌调用 `POST /v1/chat/completions`，模型 `gpt-4o-mini` 返回 HTTP 200，验证文本为 `carlab-final-ok`。

### 管理后台

- `https://api.carlab.top/models/metadata` 显示“模型供应商”和“API 渠道商”两个独立筛选控件。
- “API 渠道商”选项显示 `dmxapi (16)`、`nodyhub (20)`、`siliconflow (2)`、`volcengine (2)`。
- 选择 `dmxapi (16)` 后，URL 写入 `channelProvider=["dmxapi"]` 的编码查询参数，表格总计为 16，16 行均包含 `dmxapi · DMXAPI Production`。
- 管理员侧栏可见“渠道”“模型”“用户”“系统设置”等管理入口，说明当前登录态具备 Root 管理权限。

## 后续操作顺序

1. 使用 Root 打开 `https://api.carlab.top/sign-in`；密码只从本机钥匙串读取：`security find-generic-password -a root -s carlab-api-railway-admin -w`。
2. 在“渠道”页按渠道商分组，逐模型执行可用性测试，先记录协议、状态码、耗时和上游错误，再确认原始成本价。
3. 成功且完成核价后，才在“系统设置 → 模型”写入全局价格；同一模型来自多个成本渠道时，不得静默覆盖为某一家价格。
4. 在“系统设置 → 计费”配置 OEM 下游分组倍率，并用独立下游账号和令牌验证预扣、结算与日志。
5. 全部确认后，再按文本、图片、视频分别迁移超级种子调用；本轮不修改超级种子代码。

## 回滚与剩余风险

- 本轮新提交只包含文档。文档回滚可直接恢复上一提交；云端代码回滚应从 Git 分支重新部署 `c46b2b5f` 或更早版本，不应依赖已显示 `REMOVED` 的旧 Railway 部署 ID。
- 当前公开目录只有 4 个已计费模型，不能据此宣称 28 个可路由模型全部已核价或可售。
- DMXAPI 与 Nodyhub 存在同名模型，New API 全局模型价格不能同时表达两家不同成本；后续必须选择主成本渠道、拆分模型别名，或建立渠道级成本账本。
- DeepWL/FOX、Wuyinkeji 等私有或非标准协议仍需单独适配；未完成协议验证前，不得把目录状态标记为已上线。
- Root 尚未完成两步验证配置；应在正式 OEM 分发前开启 2FA 并离线保存恢复码。
