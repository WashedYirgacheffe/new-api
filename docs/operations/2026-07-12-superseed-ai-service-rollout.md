# Superseed AI Service Rollout

## Scope

This round continues the staged migration from hard-coded provider calls to CarLab API as the shared model, capability, quote, routing, and OEM control plane.

The implementation covers:

1. Strengthen the CarLab text, image, and video Profile contracts and Quote response.
2. Validate Profile parameters again inside the Generation BFF before billing or dispatch.
3. Publish and cloud-test a small text application-model set before image and video activation.
4. Add TapDash model-collection management, model smoke testing, and CarLab route-group selection.
5. Add OEM collection assignment and local billing-group mapping without replacing New API group pricing.
6. Keep production image and video feature flags disabled until their provider-specific contracts pass live calls.

## Assumptions

- CarLab API remains authoritative for gateway models, model providers, API channel providers, Profile versions, quotes, and route groups.
- Supabase remains authoritative for Superseed application models, model collections, OEM assignments, local user-group multipliers, and generation ledgers.
- A client may select only an `app_model_id`; it cannot choose or override a CarLab route group directly.
- The existing New API group-ratio system remains the upstream pricing mechanism. Superseed user-group multipliers remain the downstream retail multiplier.
- Verification is cloud-only. No local Go, Vue, or Vite test suite is run in this round.
- Secrets are read from managed cloud configuration or macOS Keychain and are never written to this document or repository.

## Implementation Plan

1. CarLab capability contracts
   - Update `model/model_operation_profile.go` with standard text controls and endpoint-correct image/video contracts.
   - Update `controller/model_catalog.go` so catalog bindings expose only compatible endpoint contracts and Quote returns route and retail-relevant metadata.
   - Verify through Railway deployment and token-scoped `/api/user/models/catalog`, `/profile`, and `/quote` requests.
2. Generation BFF
   - Update Superseed `api/_generationBilling.js` to merge binding/UI overrides, reject unknown or invalid parameters, and apply the local user multiplier to quoted credits.
   - Select CarLab service tokens server-side by expected route group through `CARLAB_ROUTE_TOKENS_JSON`; never accept a client-provided group or token.
   - Reserve text credits with a conservative UTF-8 input estimate and bounded output tokens, then refund the difference from actual provider usage when the pricing version and route group remain unchanged.
   - Update Superseed generation dispatch and polling to follow Profile endpoint and response contracts instead of assuming one media response shape.
   - Verify with a temporary Supabase user, temporary credits, idempotent duplicate submission, and cleanup.
3. TapDash operations
   - Add app-model smoke-test support to `api/ai-app-models.js` and `AppModelsView.vue`.
   - Add a `ModelCollectionsView.vue` route for collection members, OEM ownership, user-group visibility, and CarLab route-group mapping.
   - Verify on the `superseed-dash` production deployment.
4. OEM routing and billing
   - Add an additive Supabase migration for OEM tenants, tenant administrators, collection assignments, and user-group route mapping.
   - Filter `/api/model-catalog` by the authenticated user's assigned OEM collection when present.
   - Resolve CarLab route groups and their service tokens server-side, deny OEM users without an enabled collection, and preserve the existing New API group ratio plus Superseed retail multiplier chain.
5. Rollout order
   - Enable text only in branch Preview and complete live generation and refund checks.
   - Leave production text, image, and video flags disabled until explicit acceptance.
   - Prepare image and video contracts and async polling, then activate each category only after a provider-specific live call succeeds.

## Cloud Resources And Configuration

| Resource | Intended operation |
| --- | --- |
| Railway project `carlab-api`, service `new-api` | Deploy CarLab contract and Quote changes from `codex/oem-api-hub` |
| `https://api.carlab.top` | Verify token catalog, Profile, Quote, text generation, and route-group behavior |
| Supabase project `ctlirbtjzychneuaruci` | Apply additive OEM and generation-ledger migration; publish test application models |
| Vercel project `superseed` | Git-based Preview deployment from `codex/ai-service-migration`; no direct production deployment |
| Vercel project `superseed-dash` | Manual production deployment after TapDash changes |

Configuration names used in this round:

- `CARLAB_API_BASE`
- `CARLAB_SERVICE_TOKEN`
- `CARLAB_ROUTE_TOKENS_JSON`
- `MODEL_CATALOG_ENABLED`
- `GENERATION_BFF_TEXT`
- `GENERATION_BFF_IMAGE`
- `GENERATION_BFF_VIDEO`
- `VITE_GENERATION_BFF_TEXT`
- `VITE_GENERATION_BFF_IMAGE`
- `VITE_GENERATION_BFF_VIDEO`
- `VITE_SUPABASE_URL`
- `VITE_SUPABASE_ANON_KEY`
- `SUPABASE_SERVICE_ROLE_KEY`

## Verification Criteria

- Token catalog marks each binding with `dispatch_ready`; only endpoint-compatible bindings can be published as application models.
- Quote returns the effective CarLab route group, gateway group ratio, pricing version, estimated gateway amount, and explicit estimation limitations.
- `/api/model-catalog` returns authenticated application models and OEM-filtered collections.
- Generation BFF rejects stale Profile versions, invalid parameters, unassigned OEM models, and client route-group injection.
- Text generation creates one ledger entry, charges once, returns a normalized result, and remains idempotent.
- Text settlement never trusts client token counts and never performs an unbounded post-call debit; conservative reservation is reduced only by an idempotent refund.
- Failed dispatch or task completion produces one refund and an auditable generation status.
- OEM users without an enabled collection receive no models, and every configured CarLab route group must resolve to a Token whose actual group matches.
- TapDash can sync models, publish an application model, run a smoke test, and manage collection membership.
- Cloud deployments are healthy and all temporary users, tokens, credits, and test rows are removed unless explicitly retained as production configuration.

## Rollback

1. Disable all `GENERATION_BFF_*` and `VITE_GENERATION_BFF_*` flags.
2. Revert the Superseed branch commit and allow the Git-connected Preview deployment to rebuild.
3. Revert the CarLab commit and redeploy the previous Railway revision.
4. Disable affected application models or collections in Supabase; additive OEM tables can remain without affecting the legacy path.
5. Roll back TapDash using the previous Vercel deployment and promote it to production.

## Remaining Risks

- Provider-specific image and video parameter names cannot be declared production-ready from model names alone; each published Profile still requires upstream documentation and a live request.
- A pre-request Quote is an estimate for token-priced models. Final provider usage settlement must remain observable and may require a later reconciliation job.
- Existing imported model metadata contains classification errors. Application publication must stay allowlist-based until endpoint and provider evidence is corrected.
