# DMXAPI 人民币价目与模型生命周期核验

## 范围

本轮核验 DMXAPI 人民币价目页 `https://www.dmxapi.cn/rmb`，只修正 CarLab API 正式目录中的生命周期冲突并保存可审计的渠道成本证据。没有修改超级种子、TapDash、供应商凭据、渠道路由、New API 计费参数或 OEM 分组倍率。

## 假设与边界

- `/rmb` 为 DMXAPI 自定义动态价目页，不是 New API 标准定价接口。
- 页面标注的 DMXAPI 价格为人民币且含税 6%；这些值仅代表抓取时点的上游成本证据。
- 页面宣称当前启用 532 个模型，但公开主表只有 333 行，因此不能把该表当作完整可路由模型清单。
- 模型必须按完整路由 ID 精确匹配；系列名、前缀或描述相似不能自动生成别名或渠道绑定。
- 上游对 `kling-v3-video-generation` 的通知没有年份，并且该条目在 7 月 11 日仍可见，因此采用“弃用并禁止新迁移”，不执行物理删除。

## 发现与处理

- 标准 `GET /api/pricing` 返回 HTTP 403 和 `pricing is disabled`，不能作为 DMXAPI 的机器可读价格源。
- 动态价目页返回 HTTP 200，响应头版本为 `v2.6.8`；页面显示 532 个启用模型和 333 条公开价格记录。
- `kling-v3-video-generation` 明确提示“该模型将于6月30日下架，请尽快迁移至kling-v3”，与原目录中的正常启用定位冲突。
- 新增正式目录项 `kling-v3`，保留 `kling-v3-video-generation` 为 legacy 记录，并设置 `lifecycle_status=deprecated` 与 `replacement_model=kling-v3`。
- 云端新增模型元数据 ID 15（`kling-v3`，`status=1`），并将旧模型元数据 ID 14（`kling-v3-video-generation`）设为 `status=0`；没有自动配置旧名映射。
- `viduq2` 不能与 DMXAPI 的 `viduq2-pro`、`viduq2-ctv` 模糊合并，继续仅绑定 DeepWL，等待协议和能力核验。
- 其余 7 个已标记 DMXAPI 的目录 ID 没有在公开人民币价目表中精确出现；不据此删除渠道归属，但在视频迁移前必须使用有效凭据逐模型验证。

## 变更文件

- `docs/catalog/initial-model-catalog.json`：目录 schema 升级为 2，新增 `kling-v3`，将旧路由标为弃用并指向替代项。
- `docs/catalog/dmxapi-rmb-price-evidence.json`：保存来源元数据、Kling V3 精确价格证据、未匹配 ID 和 Vidu 命名冲突。
- `docs/architecture/model-catalog-oem-design.md`：增加生命周期字段、精确匹配规则和价格证据边界。
- `AGENTS.md`：增加本轮维护文档索引。

## 云资源与配置

| 资源或配置 | 本轮处理 |
| --- | --- |
| `api.carlab.top` / Railway `carlab-api/new-api` | 仅同步模型元数据；不部署新代码 |
| DMXAPI Production 渠道 | 不修改 Key、Base URL、优先级、权重、分组或模型权限 |
| `ModelPrice` / `ModelRatio` / `CompletionRatio` / Group Ratio | 不写入，不依据上游成本自动生成售价 |
| 超级种子业务令牌 | 只用于验证现有可见模型集合，不修改 |

## 验证证据

- `https://www.dmxapi.cn/rmb` 返回 HTTP 200；页面主表表头明确为“厂商原价”和“DMXAPI价格（含税6%）”。
- 精确读取到 `kling-v3` 与 `kling-v3-video-generation` 两行，以及旧模型的迁移通知。
- 云端 DMXAPI Production 渠道模型权限同时包含 `kling-v3` 与 `kling-v3-video-generation`。
- 新增的 `kling-v3` 元数据运行时反查到 DMXAPI 与 Nodyhub 两个绑定渠道；本轮只核验了 DMXAPI 公开价目，Nodyhub 绑定不作为正式价格或可用性证据。
- 云端查询确认 `kling-v3` 为 ID 15、`status=1`，旧 `kling-v3-video-generation` 为 ID 14、`status=0`。
- 同步前超级种子业务令牌的 `/v1/models` 不包含任何 Kling V3/V2.6 条目，证明本轮元数据修正不会直接扩大业务可调用范围。
- 同步后再次查询仍返回 154 个可见模型且没有 Kling V3/V2.6 条目，证明没有绕过售价和分组门槛。
- `docs/catalog/initial-model-catalog.json` 和 `docs/catalog/dmxapi-rmb-price-evidence.json` 均通过 `jq` 解析；目录模型 ID 无重复。

## 回滚

1. Git 回退本轮提交，恢复 schema 1 的 14 条初始目录。
2. 云端删除新增的 ID 15 `kling-v3` 模型元数据记录。
3. 将 ID 14 `kling-v3-video-generation` 恢复为 `status=1`。
4. 不需要回滚渠道、Token 或计费配置，因为本轮没有修改这些对象。

## 剩余风险与权限

- DMXAPI 公开价格页是动态数据，没有稳定公开 JSON 价格接口；后续需要定期复核，不能依赖当前快照长期计费。
- 公开页、具体模型文档和凭据化 `/v1/models` 的覆盖范围不同；正式迁移仍需同时满足可见性、协议、真实调用和售价配置四项验证。
- Nodyhub 当前也绑定 `kling-v3`，但尚未完成来源、协议、成本和真实调用核验，不能作为 DMXAPI 的自动故障转移依据。
- `kling-v3` 尚未配置 CarLab 对外售价和 OEM 分组倍率，本轮不会对业务 Token 开放。
- 本轮没有启动超级种子文本、图片或视频迁移，仍需用户逐阶段确认。
