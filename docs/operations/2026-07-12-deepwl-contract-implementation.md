# DeepWL Contract Implementation

## Scope

Implement the approved contract-driven model control path for DeepWL-backed models:

- Version and fingerprint model bindings.
- Validate structured model overrides.
- Apply contract parameter multipliers consistently in Quote and relay billing.
- Expose effective contracts to Superseed Dash and TapLater.
- Roll out the Superseed `gold`, `silver`, and `bronze` settlement groups after cloud verification.
- Reconcile pre-consume estimates with final text usage so TapLater credits settle against the same quota calculation as CarLab relay logs.
- Publish the first verified DeepWL text, image, and video application models through TapDash.

## Assumptions

- CarLab API remains the source of truth for runtime fields, request/response contracts, and parameter pricing.
- Existing New API model prices remain the source of truth for base prices.
- Existing provider adaptors remain authoritative for provider-specific post-submit adjustments.
- No credential values are stored in source code, documentation, command output, or Git history.

## Changed Files

- `model/model_operation_contract.go` and `model/model_operation_contract_test.go`: normalize, version, hash, and validate effective operation contracts and parameter-pricing rules.
- `model/model_operation_profile.go`, `controller/model_operation_profile.go`, and `controller/model_catalog.go`: expose versioned contracts, request/response mappings, brand metadata, routing groups, Profile, catalog, and Quote responses.
- `relay/helper/model_contract_pricing.go`, `controller/relay.go`, `relay/relay_task.go`, and `service/text_quota.go`: apply the same contract parameter multipliers to Quote, synchronous relay, asynchronous task billing, and usage-based settlement.
- `service/text_quota_test.go`: protect usage-based settlement from the conservative pre-consume token floor while retaining contract multipliers such as resolution.
- Superseed `CarLab/TapDash/api/ai-app-models.js`: pin contract fingerprints, force unverified auto-pins back to Draft, dispatch contract-shaped smoke tests, poll asynchronous tasks, and compare Quote with CarLab consume logs.
- Superseed `CarLab/TapLater/packages/taplater/api/_generationBilling.js`: send final upstream usage to the settlement Quote so reserved credits can be reconciled with actual cost.
- Superseed model-node and contract files from branch `codex/ai-service-migration`: render native schema controls and model-brand icons without exposing channel-provider labels.

## Cloud Resources

- Railway project: `carlab-api`
- Railway service: `new-api`
- Railway deployment: `a1ac0436-f80f-442e-bd9d-a29445f2a4e4` (`SUCCESS`)
- Public gateway: `api.carlab.top`
- TapDash production deployment: `3JZ5A9ksBaso59ZoRrcXT2DE66yq`, aliased to `superseed-dash.vercel.app`
- Superseed Vercel project: production and branch-specific Preview variables prepared for Git deployment from `codex/ai-service-migration`
- Supabase project: `ctlirbtjzychneuaruci`, storing application-model pins and contract verification evidence

## Configuration Names

- New API `GroupRatio`: existing groups preserved; `gold=1`, `silver=1.2`, and `bronze=1.5` added.
- CarLab user ID `2` (`superseed-service`) and token `superseed-production`: assigned to `gold`.
- DeepWL Text, Image, and Video channels: existing `default` route retained and `gold,silver,bronze` added.
- `CARLAB_SERVICE_TOKEN`, `CARLAB_ADMIN_ACCESS_TOKEN`, and `CARLAB_ADMIN_USER_ID` refreshed in TapDash Production and Preview without exposing their values.
- `CARLAB_SERVICE_TOKEN`, `CARLAB_API_BASE`, `MODEL_CATALOG_ENABLED`, `GENERATION_BFF_TEXT`, `GENERATION_BFF_IMAGE`, `GENERATION_BFF_VIDEO`, and matching `VITE_GENERATION_BFF_*` values prepared for Superseed Production and branch Preview.
- Existing DeepWL channel credentials stored only in cloud secret stores

## Verification Evidence

- Token-scoped catalog reports `token_group=gold`; all three target models route only through `gold` for the service token while the DeepWL channels remain available to the configured downstream groups.
- Catalog pricing version advanced to `3323acfc6c6c609ea47531e95cda90bc` after usage-based settlement semantics changed.
- `deepwl/gemini-3.5-flash`: contract v1 `3a705809...`, real text response succeeded; settlement Quote and consume log both charged `0.000854` in `gold`.
- `deepwl/gpt-image-2-all`: contract v1 `3e096716...`, real image response succeeded; Quote and consume log both charged `0.08` in `gold`.
- `deepwl/grok-video-3`: contract v2 `ea1121b2...`, the verified `6 seconds + 720P` request completed through asynchronous polling; Quote and consume log both charged `0.4` in `gold`.
- TapDash records `smoke=success`, `fields=success`, and `pricing=success` for all three current contract hashes before setting each application model to `production + enabled`.
- Railway and TapDash cloud builds completed successfully. Per operator instruction, no local test suite was run; correctness evidence comes from cloud compilation, health checks, and real upstream calls.

## Rollback

1. Disable the affected application model in TapDash; this immediately removes it from the production TapLater catalog.
2. Set the Superseed `GENERATION_BFF_*` and `VITE_GENERATION_BFF_*` variables back to `false`, then use the Git deployment workflow to rebuild Preview or Production.
3. Roll Railway back from deployment `a1ac0436-f80f-442e-bd9d-a29445f2a4e4` to `fe6affd1-23d1-4f99-9e45-0d1d963ed8de` if the settlement Quote endpoint regresses.
4. Restore user ID `2`, token `superseed-production`, and application models to `default` only if the `gold` route must be abandoned; keep DeepWL channel group additions unless routing itself is implicated.
5. Promote the previous TapDash production deployment from the Vercel dashboard if the contract laboratory or publish gate regresses.

## Remaining Risks Or Permissions

- DeepWL public pricing can differ from actual upstream settlement; production publishing remains gated on real-call reconciliation.
- The first image contract intentionally exposes only verified text-to-image input; unsupported image references, quality, and resolution choices remain hidden rather than guessed.
- The first video contract intentionally exposes only `6 seconds + 720P`; other duration or resolution multipliers must be verified with real upstream charges before a new contract version is published.
- Other DeepWL models and every DMXAPI, Wuyinkeji, Nodyhub, and SiliconFlow model remain outside this production migration until their request fields and pricing are verified separately.
- Production Superseed code is not manually deployed from this operation. TapLater follows the repository Git workflow; the current feature branch is used for Preview acceptance before any main-branch promotion.
