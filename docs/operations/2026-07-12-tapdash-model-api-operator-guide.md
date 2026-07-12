# TapDash 模型 API 操作指引实施记录

## Scope

为 TapDash 的 AI 服务板块补齐面向管理员的完整操作路径和页面内说明，覆盖：

- API 渠道、网关模型、能力模板、应用模型、模型集合与 OEM 的固定五步流程。
- 模型供应商、API 渠道商、运行渠道、Profile、合同、应用模型和模型集合的术语边界。
- 合同采用、字段映射、素材规则、分辨率/时长倍率、Quote、真实烟测和生产发布门禁。
- CarLab 路由组采购倍率与超级种子本地用户分组零售倍率的区别。
- 首次接入、已有模型变更、紧急下线、OEM 分配、故障排查和回滚。

## Assumptions

- CarLab API 继续作为渠道、网关模型、路由、价格、合同和 Quote 的权威系统。
- TapDash 继续作为超级种子应用模型验证、发布和 OEM 分配层。
- TapLater 继续只消费动态目录与结构化合同，不新增模型级字段或供应商价格硬编码。
- 不修改现有生产模型、合同、渠道、价格、OEM 数据或凭据。
- 按操作方要求不运行本地测试，使用 Vercel 云端构建与生产浏览器验收。

## Changed Files

- Superseed `CarLab/TapDash/docs/MODEL_API_WORKFLOW.md`：完整管理员手册，包含系统边界、逐页操作、合同 JSON 示例、计费、OEM、排障、回滚与完成标准。
- Superseed `CarLab/TapDash/src/views/ai/ModelApiGuideView.vue`：新增 Dash 内可直接访问的“模型 API 操作指南”。
- Superseed `CarLab/TapDash/src/features/ai-service/guide.ts` 和 `AiServiceWorkflow.vue`：统一五步流程、当前页面用途、下一步条件和风险提示。
- Superseed `AiFieldLabel.vue` 和 `AiFilterField.vue`：为筛选器、表头和表单增加可见标签与问号说明。
- Superseed `ChannelsView.vue`、`GatewayModelsView.vue`、`CapabilityProfilesView.vue`、`AppModelsView.vue` 和 `ModelCollectionsView.vue`：接入统一流程并补齐页面级说明。
- Superseed `ModelContractLabDrawer.vue`：解释 Input/UI/Material Schema、Field Map、Coercions、请求端点、参数倍率、Quote、实际金额和验证记录。
- Superseed `AppModelsView.vue`：新增模型固定为 `Draft + 停用`，不再展示后端必然拒绝的创建时 Production/立即启用选项。
- Superseed `AdminLayout.vue` 和路由：AI 服务菜单新增“操作指南”，AI 服务默认入口指向指南。
- Superseed `CarLab/BugBoard/data/progress.json`：记录本轮实施和生产验收。

## Cloud Resources

- Vercel project: `superseed-dash`
- Final production deployment: `HCwrHBEPT81eSYuxkFAQh4STXYaJ`
- Final production URL: `superseed-dash-66vgc1ote-washedyirgacheffe-4517s-projects.vercel.app`
- Production alias: `superseed-dash.vercel.app`
- Superseed branch: `codex/ai-service-migration`
- Main implementation commit: `9c55c03`
- Mobile containment fix: `fd908c3`

## Configuration Names

- No production environment variable value was changed.
- Existing `ADMIN_PASSWORD_ADMIN` was used through the normal login page for browser acceptance; its value was never printed into source, documentation, Git, or final reports.
- A temporary Vercel environment file was created under `/tmp` for acceptance and deleted immediately after use.
- Existing CarLab service and administrator tokens were not changed.

## Verification Evidence

- Vercel production build ran `vue-tsc && vite build` successfully for deployment `HCwrHBEPT81eSYuxkFAQh4STXYaJ`.
- Production route `/ai-service/guide` renders the new menu entry, authority boundaries, five-step workflow, terminology, publish gates, change scenarios, action effects, billing and rollback guidance.
- Clicking workflow step `04 应用模型` navigates to `/ai-service/app-models`; no console warnings or errors were reported.
- The application-model page renders the current-step explanation, TapLater visibility gate, visible filter labels, field-help controls and clarified `合同与核价` / `真实烟测` actions.
- Opening `Gemini 3.5 Flash · 合同实验室` shows application and gateway contract versions/hashes, fixed-contract warning, tab descriptions and field-level help without changing data.
- Opening `新增应用模型` shows the `Draft + 停用` rule and disabled initial stage/status fields; no Production or immediate-enable creation path remains.
- At `390x844`, the final guide has no document, main-container or guide-content horizontal overflow. The five-step workflow intentionally remains a local horizontally scrollable control.
- The in-app browser screenshot command repeatedly timed out and the Chrome extension backend was unavailable. Standalone Playwright was attempted only as fallback, but its bundled browser executable was absent; no browser dependency was installed. DOM, layout metrics, interaction state and console logs provide the acceptance evidence.

## Rollback

1. Promote TapDash deployment `3JZ5A9ksBaso59ZoRrcXT2DE66yq` if the new guide or shared annotation components cause a production regression.
2. Revert Superseed commits `fd908c3` and `9c55c03` together to remove the guide route, workflow component and page annotations.
3. The rollback does not require CarLab API, Supabase, channel, model, contract, pricing or OEM data changes because this operation did not mutate those resources.
4. Delete only the new guide documentation after code rollback if it no longer matches the deployed interface.

## Remaining Risks Or Permissions

- TapDash remains a desktop-first administration interface. The guide content now contains correctly on narrow viewports, but the existing fixed-width global sidebar still leaves limited working width on small phones.
- Browser acceptance intentionally avoided smoke tests, channel tests, contract saves, publish switches and OEM writes because those actions mutate production data or incur upstream charges.
- The complete guide must be updated whenever backend publish gates, supported contract widgets, billing semantics or OEM route precedence changes.
- Existing large Vite chunks still emit the prior Rollup size warning; this operation does not change the shared application bundle architecture.
