# CarLab API 模型目录与 OEM 架构

## 1. 业务术语

CarLab API 必须分开管理以下三个概念：

| 概念 | 含义 | 示例 |
| --- | --- | --- |
| 模型供应商 | 模型原厂、知识产权或官方服务主体 | Google、OpenAI、ByteDance、Kuaishou |
| API 渠道商 | 提供聚合、转售、兼容协议或代充服务的平台 | DeepWL、Wuyinkeji、DMXAPI |
| 渠道实例 | 一枚凭据及其协议、分组、Base URL 和模型权限 | DeepWL Text、DeepWL Seedance、DMXAPI Production |

New API 现有 `vendors` 表继续表示模型供应商。`channels.channel_provider` 表示 API 渠道商代码，渠道 `name` 表示具体实例，渠道 `type` 只表示协议适配器类型。三者不得混用。

规范渠道商代码首批为：

- `deepwl`：包含超级种子历史命名中的 FOX 渠道。
- `wuyinkeji`：速创 API。
- `dmxapi`：DMXAPI。
- `siliconflow`：SiliconFlow。
- `nodyhub`：Nodyhub。
- `volcengine`：火山方舟直连。

## 2. 正式模型目录

正式目录的路由主键仍是 `model_name`，避免影响 New API 现有路由和计费。目录新增字段只承担产品展示和治理：

| 字段 | 用途 |
| --- | --- |
| `model_name` | New API 路由 ID，不因展示名称变化而改变 |
| `display_name` | CarLab API 面向管理员和 OEM 的名称 |
| `model_type` | `text`、`image`、`video`、`audio`、`embedding`、`rerank` 等 |
| `vendor_id` | 模型供应商 |
| `source_url` | 本条元数据的公开来源 |
| `description` | 来自公开文档并经压缩整理的能力说明 |
| `tags` | 能力标签，例如视觉、工具调用、图生图、音频生成 |
| `lifecycle_status` | 源目录治理状态；省略等同 `active`，还可为 `deprecated` 或 `retired` |
| `replacement_model` | 退役或弃用模型的明确替代路由 ID，不用于自动重写请求 |
| `endpoints` | New API 支持的端点类型 |
| `bound_channels` | 运行时反查的渠道实例及 `channel_provider` |

`display_name` 不作为请求参数，`source_url` 不参与计费，`channel_provider` 不参与路由选择。真正的路由仍由 abilities、优先级、权重、分组和渠道状态决定。当前数据库尚无独立生命周期列，云端将 `deprecated` 映射为模型元数据 `status=0`，替代说明保留在源目录；不得据此自动建立旧名到新名的模型映射。

## 3. 信息来源优先级

目录信息按以下优先级维护：

1. 模型原厂官方文档。
2. API 渠道商的具体模型文档。
3. API 渠道商的公开模型目录或定价页面。
4. 使用有效业务凭据获取的 `/v1/models`，仅证明可见性，不自动证明能力和价格。
5. 超级种子现有代码只能作为待核对线索，不能作为正式目录的最终来源。

首批渠道来源：

- DeepWL：`https://zx1.deepwl.net/api/pricing` 和 `https://doc.deepwl.cn/llms.txt`。
- Wuyinkeji：`https://api.wuyinkeji.com/doc` 及具体 `/doc/{id}` 页面。
- DMXAPI：`https://www.dmxapi.cn/rmb`、`https://doc.dmxapi.cn/model-list.html` 及具体模型文档。

每次更新记录抓取日期。文档与公开价格冲突时，目录能力以具体模型文档为准，成本核对以渠道商价格页为准。

DMXAPI 的 `/rmb` 是独立动态价目页，不能等同于 New API 标准 `/api/pricing`，也不能替代凭据化 `/v1/models` 的可用性核验。公开页快照与当前目录的精确 ID 对账记录在 `docs/catalog/dmxapi-rmb-price-evidence.json`；模糊包含、前缀相似或产品系列相同均不得自动视为同一模型。

## 4. 价格与倍率

本项目不新增第二套计费引擎：

- 上游页面价格只用于运营核对成本。
- 渠道价格证据文件只保存抓取时点的上游成本，不由程序自动写入 New API 价格配置。
- CarLab 对外计费继续使用 New API `ModelPrice`、`ModelRatio`、`CompletionRatio`、图片/音频倍率和 Group Ratio。
- OEM 差异价格使用不同用户分组和分组倍率实现。
- 未配置价格的模型继续拒绝调用，不开启全局自用模式绕过。
- 图片和视频动态参数仍使用 New API 现有按次价格及其他倍率机制。

## 5. OEM 对象映射

第一版不增加独立租户数据库，先用 New API 原生对象实现清晰隔离：

| OEM 对象 | New API 对象 |
| --- | --- |
| OEM 平台 | 普通用户账户 |
| OEM 管理员 | 该普通用户登录身份；不授予全局管理员 |
| 生产/测试环境 | 独立 Token |
| 套餐 | 用户分组、模型白名单和额度 |
| 差异售价 | 分组倍率 |
| 消费统计 | 用户、Token、模型维度日志 |

当一个 OEM 需要多个管理员或子项目时，再增加组织层。组织层必须在用户之上做授权，不能让 OEM 管理员读取全局渠道、供应商 Key 或其他 OEM 数据。

## 6. CarLab API 前端

面向用户的产品名称设置为 `CarLab API`，模型管理页面使用“模型供应商”和“API 渠道商”两个独立术语。模型目录需要展示模型类型、展示名、来源和绑定渠道商。

New API 与 QuantumNous 的开源归属、许可、代码标识和关于信息必须保留。CarLab API 是部署产品名称，不替换或删除上游项目身份。

## 7. 超级种子迁移门槛

以下条件全部满足后，才可向用户申请迁移确认：

1. 目标模型已存在正式目录记录和公开来源，且 `lifecycle_status` 为 `active` 或省略。
2. 至少一个渠道已通过真实低成本调用。
3. 模型价格和 OEM 分组倍率已配置。
4. 文本、图片或视频协议适配已通过对应测试。
5. TapDash 已能识别模型供应商和 API 渠道商。
6. 已有回滚开关，可恢复旧供应商代理。

迁移顺序继续采用文本、图片、视频三阶段；每一阶段都需要用户单独确认。
