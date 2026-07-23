# RE 精确合同与画布材料输入

## 范围与假设

- 范围仅包含 RE Async；RE Chat 不接入、不启用，也不进入 TapLater。
- 以 RE 官方模型详情页 Playground 配置和同版本前端校验清单为参数证据；当前发布集合取价格目录中可发布且不处于延后计费的 88 个模型。
- 渠道、模式条件取官方可用并集，保留真人实测路径；比例、时长、分辨率、质量和数量等有限值仍严格使用官方枚举。
- CarLabAPI 是合同、分站与路由组权威；TapLater 的 Supabase `app_models` 仅保存稳定 UI/账本身份。

## 代码与数据

- `docs/catalog/reapi-async-contracts.json`：逐模型完整输入、UI、材料、请求映射、默认值、来源 URL 与内容哈希。
- `cmd/reapi-contract-snapshot`：重抓官方模型页和同版本静态清单，规整有限值选择控件并校验发布集合。
- 快照工具会读取 `web/node_modules/zod` 解析上游同版本校验器；干净工作区须先运行 `cd web && bun install`，再回到仓库根执行 `node cmd/reapi-contract-snapshot/main.mjs --check`。
- `cmd/reapi-onboard`：注册四类 `re-task` 异步 Profile，写入 88 个 `schema_mode=replace` Binding，并清理异步文本工具的遗留 Chat Binding。
- `model/model_operation_contract.go`、`controller/model_catalog.go`：注册 `re-task` 请求适配器与四类异步响应合同的可派发判定。
- `relay/channel/task/reapi/adaptor.go`：保留精确合同映射后的材料字段名，不再把所有图片输入强制改成 `image_urls`。
- Superseed TapLater：按合同渲染选择控件；材料只从画布连接进入；连接文字与节点提示词合并；RE 异步提交、轮询、结果与错误均走 Generation BFF。

## 云资源与配置

- Railway 项目 `carlab-api`、服务 `new-api`、Postgres：运行精确合同 onboarding，并保持 `REAPI_TASK_API_KEY` 仅为 Railway Secret。
- RE Async 渠道同时声明 `default,gold`，使 `cjzz.top` 的 gold 业务 Token 可以看到并路由 RE 模型。
- CarLabAPI 分站 `superseed`：在合同就绪后保留原 17 个模型并追加 88 个 RE Async 模型。
- Vercel 项目 `superseed` 对应 Preview：使用既有 `CARLAB_SERVICE_TOKEN`、`CARLAB_SUBSITE_DOMAIN`、`CARLAB_SUBSITE_ROUTE_GROUP` 和 Supabase 服务端配置，运行幂等 RE app-model 镜像同步。

## 验证

- 合同快照必须满足 88/88、无重复模型、与价格 publish-ready 集合完全一致，并为每个模型保留证据。
- CarLabAPI 定向测试覆盖合同解析、Binding 规范化、目录 dispatch-ready、精确材料字段透传和 onboarding 幂等性。
- TapLater 定向测试覆盖材料 URL 防绕过、RE 四类结果与错误、文字合并、分站白名单和动态控件；随后执行类型检查与生产构建。
- 云端只读验收核对分站 RE 启用数、`profile_ready` 数和画布目录；付费真实生成不作为本轮自动化验收前置条件。

本地收口结果：88 个发布合同与 8 个排除项完整匹配，输入 schema 递归 URI 字段为 0，144 个有限值字段均为枚举选择；62 个模型具有画布素材合同、20 个使用多槽位合同。Wan 2.7 的 `media[]` 按角色模板合并，`driving_audio` 与 `reference_voice` 按索引写入同一对象；RE 入站拒绝 `metadata` 参数展开、未知混合素材类型和缺失配对槽位。CarLabAPI 定向 Go 测试、合同 `--check`、TapLater 生成链路 209 项测试与 `vue-tsc --noEmit` 均通过。

## 回滚

1. CarLabAPI 渠道管理中先停用 `RE Async`，立即停止新任务路由。
2. 从 `superseed` 分站模型集合移除 `re/` 模型，保留原有 canonical 模型。
3. 回滚 CarLabAPI Git commit；旧 Binding 可通过 Binding revision 恢复，或重跑上一版 onboarding。
4. 回滚 Superseed Git commit；Supabase RE `app_models` 可改为 `enabled=false`、`rollout_stage=draft`，不删除既有生成账本引用。

## 剩余风险

- 官方文档中的 channel/mode 条件已保留为证据和宽松并集，仍需按实际业务优先级逐模型真人验证后再收紧。
- 画布已提供首帧、尾帧、参考图、参考视频、音频、蒙版等官方槽位选择；真人验收仍需逐模型确认常用默认槽位，但不能恢复 URL 文本框或用历史参数绕过连线材料。
- `music-video-1-0.srt_url` 是当前唯一未接入的文件素材候选；画布尚无通用文件节点，因此仅保留上游证据，不暴露文本输入。
