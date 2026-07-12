# DeepWL 契约驱动模型管理设计记录

## 范围

本轮完成 CarLab API、TapDash 与 TapLater 的 DeepWL 契约驱动模型管理设计，覆盖模型发布开关、字段与素材合同、请求/返回映射、参数价格倍率、金银铜结算组、节点原生弹层和模型品牌展示。

## 假设与边界

- DeepWL 全量模型进入 Dash 草稿目录，只有经过真实验证的模型可以发布。
- CarLab API 是模型合同与参数价格倍率的唯一真相源。
- Dash 负责编辑、调试、对账和发布，不建立第二套规则。
- TapLater 只消费已发布合同，不显示渠道商文字。
- 本轮仅完成设计，没有修改运行代码、数据库或云端配置。

## 变更文件

- `docs/architecture/deepwl-contract-driven-model-control-design.md`
- `docs/operations/2026-07-12-deepwl-contract-design.md`
- `AGENTS.md`

## 云端资源与配置

本轮没有执行 Railway、Vercel、Supabase 或 Cloudflare 变更。

后续实施将涉及但不在本轮写入值的配置：

- New API `GroupRatio` 与用户组。
- `CARLAB_ROUTE_TOKENS_JSON`。
- `MODEL_CATALOG_ENABLED`。
- `GENERATION_BFF_TEXT`、`GENERATION_BFF_IMAGE`、`GENERATION_BFF_VIDEO`。
- 对应 `VITE_GENERATION_BFF_*` 前端开关。

## 设计验证

- 用户确认采用契约驱动方案。
- 用户确认 CarLab 结算组与 TapLater 本地倍率分层计算。
- 用户确认增加分辨率等模型参数倍率，并由 CarLab Quote 与实际 Relay 共用规则。
- 用户确认 TapLater 恢复原模型/筛选/数量弹层。
- 用户确认模型菜单保留品牌图标、模型名和描述，删除 DeepWL 等渠道商文字。
- 用户确认 Dash 全量草稿、合同调试、真实生成、价格对账和发布门禁工作流。

## 回滚

本轮仅新增设计文档。回滚时对本轮文档提交执行非破坏性 `git revert`，不涉及运行服务或数据回滚。

## 剩余风险与权限

- DeepWL 不一定提供所有模型的机器可读字段合同；缺失部分必须依据官方文档和真实调用验证，不能从模型名称推测。
- 参数倍率必须同时进入 Quote 与真实 Relay，否则会产生报价和实扣差异。
- CarLab 用户 `ID=2` 调整为金牌组及服务 Token 迁移属于生产配置变更，需要在实施阶段单独验证与记录。
- 全量 DeepWL 逐模型真实测试会产生上游费用，实施时应先按合同族测试并保留预算边界。
