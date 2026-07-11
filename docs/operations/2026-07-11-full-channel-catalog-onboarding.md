# 全渠道模型目录接入维护记录

- 日期：2026-07-11
- 状态：已完成
- 操作目录：`/Volumes/CODE/Code_SYS/CarLabAPI`
- 代码分支：`codex/oem-api-hub`
- 公网入口：`https://api.carlab.top`

## 目标与范围

本轮将“仅保留超级种子当前使用的 65 个模型”调整为“接入可通过标准模型目录核验的渠道商全量模型”。超级种子的 65 个模型继续作为无前缀兼容目录，不再作为 CarLab API 的全局白名单。新增目录统一使用渠道命名空间，避免同名模型在不同渠道上产生路由和成本冲突：

- `deepwl/<upstream-model>`
- `dmxapi/<upstream-model>`
- `nodyhub/<upstream-model>`
- `siliconflow/<upstream-model>`

`model_provider` 表示模型原厂，`channel_provider` 表示 API 渠道商。DeepWL 与 FOX 是同一渠道体系，统一使用 `deepwl` 代码。无法从上游资料确认原厂的私有别名使用 `Owner not disclosed by <channel>`，不再显示笼统“未知供应商”。

本轮没有修改超级种子仓库、TapLater、TapDash 或 Vercel 环境，也没有开始超级种子的分阶段迁移。

## 关键假设与安全边界

- “全量接入”包含后台目录、渠道归属、运行渠道 ability 和可核验价格，不等于把无价格模型免费开放给下游。
- New API 仍使用原生模型价格与分组倍率。没有公开或可安全转换价格的模型保留在管理目录中，但不进入业务 Token 的 `/v1/models`。
- 目录渠道关系和真实运行渠道关系分开保存。目录归属不会创建 ability，也不会让已停用渠道继续显示为可绑定渠道。
- Wuyinkeji 使用私有异步协议，尚不能作为标准 New API 渠道完整路由；TokenShen 旧凭据返回 401；VolcEngine `/models` 返回 404。这三类不伪装成已完成的全量标准目录。
- 不在仓库、维护文档或聊天中保存供应商密钥、管理员密码、业务 Token、Cookie 或 Cloudflare 源站密钥。

## 代码改动

- `model/model_meta.go`：新增 `ModelChannelProvider` 关联表；模型创建、更新和删除与目录渠道关系使用同一事务；渠道商筛选合并目录归属和已启用运行渠道；已停用渠道不再出现在绑定渠道、筛选和计数中。
- `model/main.go`：在完整迁移和快速迁移中创建关联表，保持 SQLite、MySQL 和 PostgreSQL 兼容。
- `controller/model_meta.go`：列表与详情批量补充目录渠道商；规则模型合并目录渠道和真实绑定渠道；删除模型时同步清理关联记录。
- `controller/model_meta_test.go`、`controller/model_list_test.go`：覆盖目录关系不创建路由 ability、停用渠道不产生状态漂移、渠道代码归一化和不存在模型更新不留下孤立关系。
- `web/default/src/features/models/`：模型表新增独立的“API 渠道商”列；筛选绑定目录渠道字段；模型编辑抽屉支持维护多个渠道商代码。
- `docs/catalog/superseed-model-allowlist.json`：明确该文件只描述 65 个超级种子兼容模型，并补齐其原厂、类型、渠道归属和 DeepWL 别名映射。
- `docs/catalog/full-channel-catalog-evidence.json`：固定全量目录数量、来源、可调用边界、运行渠道和排序摘要哈希。

## 云端目录结果

| 范围 | 模型数 | 已核价并对业务 Token 可见 | 暂不开放原因 |
| --- | ---: | ---: | --- |
| DeepWL / FOX | 67 | 67 | 无 |
| DMXAPI | 640 | 255 | 385 条缺少可安全转换的明确价格 |
| Nodyhub | 726 | 726 | 无 |
| SiliconFlow | 91 | 0 | 上游模型目录未提供可机读公开价格 |
| 四渠道命名空间合计 | 1524 | 1048 | 476 条受计费门禁保护 |
| 超级种子及其他无前缀兼容模型 | 65 | 8 | 仅保留已验证兼容入口 |
| CarLab API 管理目录总计 | 1589 | 1056 | 管理目录和业务目录职责不同 |

目录完整性检查结果：1589 条模型均有非零 `vendor_id`，均有至少一个目录渠道商；模型类型为文本 1063、图片 212、视频 227、音频 45、Embedding 26、Rerank 16。

后台筛选计数会合并命名空间目录和无前缀兼容记录，因此显示 DeepWL 90、DMXAPI 658、Nodyhub 748、SiliconFlow 95。该计数不是重复导入：同一兼容模型可同时属于多个渠道商。

## 类型规范化

上游目录存在字段自相矛盾的情况。例如 DeepWL 将描述和标签都明确为视频的 `veo_3_1`、`veo_3_1-fast` 标为 `audio`；Nodyhub 将 6 个 Runway 路由和 `MiniMax-Hailuo-02` 标为 `text`。本轮只修正 11 个有明确名称、描述或标签证据的条目为 `video`，不对语义不确定的私有别名猜测分类。

规范化前快照：

- `~/Library/Application Support/CarLabAPI/backups/2026-07-11-pre-model-type-normalization.json`
- SHA-256：`c9efce46c81f200dcee7e281963596e9b572b76310b1de6fdba621d36d364692`
- `contains_secrets=false`

## 备份与云资源

| 资源 | 状态与用途 |
| --- | --- |
| Railway 项目 `carlab-api` | 项目 ID `a2f94bb2-4f70-4a58-92fe-614e90ba2783` |
| Railway 服务 `new-api` | 服务 ID `ca79a3e0-a075-4f1e-8d27-c8219414738c` |
| Railway 最终验证部署 | `0575a0eb-10ac-4ed3-b61c-1428b23e1579`，状态 `SUCCESS`，镜像摘要 `sha256:bf85b62864740bef3696a753d87bafa07ba10da521e04d6e2e4cc9447843a859` |
| GitHub 分支 | `WashedYirgacheffe/new-api` 的 `codex/oem-api-hub`，运行代码提交 `058132f1` |
| Cloudflare `api.carlab.top` | 继续作为统一入口；本轮未修改 Worker 或源站密钥 |
| macOS 钥匙串 | `carlab-api-railway-admin`、`carlab-api-superseed-token`，只用于本机验收 |

全量导入前快照：

- `~/Library/Application Support/CarLabAPI/backups/2026-07-11-pre-full-channel-catalog-import.json`
- SHA-256：`fa92f32fad0d8ee5fa09f9e2ce38ac2f694f65355ecba079706fac292c0af148`
- 大小：50195 字节
- 内容：8 个渠道的无密钥配置、65 条导入前模型元数据、供应商和相关选项；文件声明 `contains_secrets=false`，渠道对象不含 Key 字段。

## 云端验证

- 管理接口按 100 条分页读取 16 页，共返回 1589 条，跨页无遗漏。
- 四个命名空间数量为 DeepWL 67、DMXAPI 640、Nodyhub 726、SiliconFlow 91；全目录排序摘要见 `docs/catalog/full-channel-catalog-evidence.json`。
- 后台实际打开 `https://api.carlab.top/models/metadata`，显示“模型供应商”“API 渠道商”“绑定渠道”三个独立维度，总计 1589 条、80 页。
- 后台选择 DMXAPI 后 URL 写入 `channelProvider=["dmxapi"]`，筛选显示 `dmxapi (658)`、总计 658、33 页；行内同时显示 `dmxapi/<model>`、目录渠道 `dmxapi` 和运行绑定 `dmxapi · DMXAPI Production`。
- 业务 Token 请求 `/v1/models` 返回 1056 条，其中 DeepWL 67、DMXAPI 255、Nodyhub 726、SiliconFlow 0、无前缀兼容模型 8。
- `dmxapi/MiniMax-M2.1`、`nodyhub/gpt-4o-mini`、`deepwl/gpt-4o-mini` 的 `/v1/chat/completions` 均返回 HTTP 200。
- `deepwl/gpt-image-2` 的 `/v1/images/generations` 返回 HTTP 200 和 1 张图片。
- 历史兼容条目 `kling-v2-6-text2video` 已补为 DMXAPI 目录归属，但没有伪造运行 ability。
- 5 个变更前端文件通过 oxfmt、定向 oxlint 和 TypeScript `tsgo -b`；JSON 与 Git diff 检查通过。
- 按用户要求不运行本地 Go 测试；Railway Docker 云端完成 Go 编译、前端生产构建、数据库迁移和健康检查，最终部署状态为 `SUCCESS`。
- 最终部署后 `/api/status` 返回 HTTP 200 和产品名 `CarLab API`；再次调用 `dmxapi/MiniMax-M2.1` 返回 HTTP 200 和有效 choice。

## 回滚

1. 紧急止损时先在 CarLab API 后台停用受影响渠道，阻止新请求进入上游。
2. 校验全量导入前快照 SHA-256，使用管理 API 恢复 8 个渠道的 `models`、`status`、映射和 65 条原模型元数据；快照不含密钥，不会覆盖钥匙串或 Railway Secret。
3. 删除本轮 1524 条命名空间模型及其目录渠道关系，并恢复快照中的供应商和计费选项。
4. 如只回滚类型规范化，按独立快照恢复 11 条模型的原 `model_type`。
5. 代码可回到 `a3fa5bdb`；新增关联表可暂留，旧版本会忽略。确认数据库目录恢复后再从该提交重新部署 Railway。
6. 回滚后重新验证 `/api/status`、管理目录总数、`/v1/models` 和至少一个文本模型；不要通过删除 Cloudflare Worker 绕过源站鉴权。

## 剩余风险与后续门槛

- 全量目录接入不等于逐模型验收。本轮只抽测 3 个文本模型和 1 个图片模型，其余模型仍需按端点类型、请求参数、响应格式和计费单位分批验证。
- DMXAPI 的 385 条和 SiliconFlow 的 91 条仍缺少可核验价格。填价前保持不可见是计费保护，不是目录缺失。
- 上游模型目录会新增、改名或下架。应定期重新抓取并比较排序摘要，任何差异先进入审核，不直接覆盖生产目录。
- 同一原厂模型在不同渠道的成本不同，必须继续使用渠道命名空间；不能把其中一个渠道价格静默写成无前缀模型的全局成本。
- Wuyinkeji 需要独立异步协议适配器后才能完整纳入统一调用；TokenShen 需要有效凭据；VolcEngine 只能维护可验证 endpoint，不能根据不存在的 `/models` 响应声称全量。
- 超级种子代码迁移仍等待用户完成 CarLab API 测试并明确确认。OEM 用户、Token、分组倍率和账单隔离应在模型验收后单独实施。
