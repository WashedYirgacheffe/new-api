# Grok 视频模型渠道合同修正

## 范围

本轮修正 CarLab 模型合同中 DeepWL 与 NodyHub 的 Grok 视频参数，解决 TapLater 切换到 NodyHub 次渠道后仍只显示通用 `6 秒` 参数的问题。变更只涉及默认 Profile、Binding 迁移、endpoint 类型补齐和回归测试；不修改渠道密钥、价格、生产数据库或部署配置。

覆盖模型：

- `deepwl/grok-video-3`
- `deepwl/grok-1.5-video-6s`，以 `10s`、`15s` 模型作为运行时派发身份
- `nodyhub/grok-video-3`
- `nodyhub/grok-imagine-1.5-video`

## 权威资料与假设

- NodyHub 的权威协议为用户提供的 `NODYHUB_API_DOC (1).md` 第 5 章。`grok-video-3` 支持 `6/10/15/20/25/30` 秒、七种 `ratio`、固定 `720P` 和最多 7 张 `images`；`grok-imagine-1.5-video` 支持连续 `6-30` 秒、五种 `size`、`480p/720p` 和最多 7 张 `image_urls`。
- CarLab 对外继续使用 `POST /v1/videos` 和 `GET /v1/videos/{task_id}`。NodyHub 渠道适配器内部再转换为上游 `/v2/videos/generations`，不能把上游路径暴露为 CarLab Profile 路径。
- DeepWL 价格页确认 Grok 1.5 与 Grok 3 均存在独立的 `6s/10s/15s` 模型身份，但具体模型详情页错误展示了通用 Chat API 参数，不能证明完整视频字段枚举。
- DeepWL 的五种比例、固定 `720P` 和最多 6 张参考图按用户确认的 TapLater 发布边界落地，不标记为 DeepWL 详情页已经完整证实的合同。若上游后续提供精确视频文档，应新建合同版本复核，而不是静默覆盖。

## 变更文件

- `model/model_operation_profile.go`
  - 为四个渠道模型建立互不污染的专用 Profile。
  - NodyHub Grok Video 3 使用六档时长、七档比例、固定 `720P` 和 7 张 `images`。
  - NodyHub Grok Imagine 1.5 使用 `6-30` 滑杆、五档比例、`480p/720p` 和 7 张 `image_urls`。
  - DeepWL 两个主模型使用 `6/10/15` 模式派发到精确上游模型身份。
  - 将旧 `video.generate.basic` 默认 Binding 迁移到专用 Profile；管理员自定义或禁用 Binding 继续保持不动。
  - 为主模型及 DeepWL 运行时模型补齐 `openai-video` endpoint。
- `model/model_operation_profile_test.go`
  - 锁定四个模型的参数、素材、路径、模式派发和渠道隔离。
  - 覆盖 NodyHub 非法时长、比例、清晰度拒绝。
  - 覆盖旧通用 Binding 迁移，验证其他视频模型仍使用通用 Profile。
- `model/subsite.go`、`model/subsite_test.go`
  - 将 DeepWL Grok 1.5 的 15 秒派发身份登记为运行时隐藏变体，防止被子站独立上架。
- `relay/channel/task/sora/nodyhub_adaptor_test.go`
  - 覆盖两款 NodyHub Grok 模型的完整出站字段，确认 `duration`、比例、清晰度和参考图数组不会在协议转换时丢失。
- `AGENTS.md`
  - 增加本记录到 Operations Index。

## 云资源与配置

- CarLab API：Railway `carlab-api/new-api`，生产域名 `https://api.carlab.top`。
- TapLater：Vercel `superseed` Preview，合同由 CarLab token-scoped catalog 提供。
- 本轮未修改 Railway 变量、Vercel 变量、渠道配置、模型价格或数据库数据，也未执行生产部署。

## 验证证据

已执行：

```bash
go test ./model -run 'Grok|CoreDefaultModelOperationContractMatrix|SeedDefaultModelOperationProfiles' -count=1
go test ./model -count=1
go test ./relay/helper ./relay/channel/task/sora -count=1
git diff --check
```

结果全部通过。部署后仍需直接在 TapLater Preview 切换 DeepWL/NodyHub 渠道核对参数控件；本轮不启动本地前端页面。

## 回滚

1. 回退本轮 Git 提交并重新部署 CarLab API。
2. 若默认 Binding 已在运行环境迁移，使用对应 `ModelOperationBindingRevision` 恢复旧 Profile、版本、Overrides 和 Enabled 状态。
3. TapLater 无需回退静态参数代码；旧 CarLab 合同恢复后，前端会重新显示旧合同。

## 剩余风险与权限边界

- DeepWL 详情页目前无法作为比例、分辨率和素材上限的完整权威证据；本轮值是明确的产品发布边界，仍需后续上游文档或付费烟测确认。
- 本轮未发起付费视频任务，未验证四个模型在当前渠道密钥下的真实生成结果。
- Preview 验收需要先让 CarLab 部署包含新 Profile，并完成默认 Binding 刷新；只部署 TapLater 不会改变参数。
- 本轮不自动扩展到 NodyHub 文档中的其他图片或视频模型，避免在没有逐模型验收时扩大影响范围。
