# DeepWL pricing and group settlement audit

## Scope

This round audited the DeepWL prices exposed by CarLab API before OEM resale. It compared the public price API, the price-page presentation, DeepWL token-scoped settlement logs, CarLab consume logs, and the three active DeepWL channel keys. It did not change Superseed, TapDash, TapLater, user balances, DeepWL credentials, or downstream OEM group multipliers.

The machine-readable evidence is stored in `docs/catalog/deepwl-pricing-audit-2026-07-12.json`. No token, cookie, password, task ID, request ID, or raw credential is stored in either document.

## Assumptions and price authority

The sources are not equivalent:

1. DeepWL `/api/log/token` settlement records, or a balance delta around one request, are the final cost authority.
2. DeepWL `/api/pricing` is the best machine-readable catalog source for base ratios, fixed prices, supported groups, and group multipliers.
3. The DeepWL pricing page is a presentation of a selected group. It can show a lower or otherwise different group than the active key and is not independent settlement evidence.

CarLab downstream groups and DeepWL upstream groups are separate concepts. CarLab `default`/`vip` groups control customer routing and sale-price multipliers. DeepWL `default`, `gemini-anti`, `gemini-vertex`, and similar groups control upstream procurement routing and cost. Copying DeepWL group ratios into CarLab's downstream `GroupRatio` would mix procurement cost with OEM pricing and is therefore intentionally not performed.

## Why the two Gemini prices differ

DeepWL reports `gemini-3.5-flash` with `model_ratio=0.75`, `completion_ratio=6`, and the following known group multipliers:

| DeepWL group | Multiplier | Input USD / 1M | Output USD / 1M |
| --- | ---: | ---: | ---: |
| `default` | 1 | 1.50 | 9.00 |
| `gemini-anti` | 0.5 | 0.75 | 4.50 |
| `gemini` | 2 | 3.00 | 18.00 |
| `gemini-text` | 2 | 3.00 | 18.00 |
| `gemini-vertex` | 2.5 | 3.75 | 22.50 |

The screenshot price of `$0.75/$4.50` is the `gemini-anti` price. CarLab's `$1.50/$9.00` is the `default` price. All successful logs for the current DeepWL Text, Image, and Video keys recorded the actual group as `default`, so lowering CarLab to the screenshot's `gemini-anti` price would undercharge the current procurement route by 50%.

A fresh direct request confirmed the formula: `2` prompt tokens plus `5` completion tokens consumed `24 quota`, exactly `2 * 0.75 + 5 * 0.75 * 6`. An earlier CarLab-routed request with `2` prompt and `13` completion tokens consumed `60 quota` on both sides.

## Static catalog comparison

At capture time:

- CarLab exposed 67 `deepwl/*` namespaced models.
- DeepWL `/api/pricing` exposed 119 models.
- All 67 CarLab entries matched the corresponding DeepWL API values for quota type, model ratio, completion ratio, cache ratios, and fixed model price.
- No static pricing field mismatch was found among the 67 imported models.
- 52 DeepWL pricing entries were not yet present in CarLab and remain a separate catalog-onboarding gap.

This result only proves that CarLab copied the public API correctly. It does not prove that every DeepWL public price equals actual settlement.

## Public API limitations found

DeepWL `/api/pricing` does transmit `group_ratio`, `usable_group`, `auto_groups`, and each model's `enable_groups`. It is sufficient to build an upstream group price matrix for known groups.

It does not expose the active API token's group through `/api/usage/token/`. The actual group is visible in `/api/log/token`, which is why log evidence takes priority.

The model catalog references five group names without a corresponding public `group_ratio` entry, including private, case-variant, video, and dedicated groups. Fifteen of the 67 imported models reference at least one such group. Eleven imported models support a known group above `default`, with a maximum known multiplier of `2.5`. A future DeepWL token-group change can therefore create underpricing unless the actual log group is re-audited before the key is used for resale.

## Actual settlement exceptions

### `gpt-image-2`

DeepWL `/api/pricing` declares a fixed `$0.15` price, but two real settlement logs used dynamic token billing with `model_ratio=4`, `completion_ratio=5`, and `model_price=-1`:

- `23` prompt + `229` completion consumed `4,672 quota`; CarLab charged `75,000`.
- `17` prompt + `2,058` completion consumed `41,228 quota`; CarLab charged `75,000`.

The public API is therefore not the settlement authority for this model. CarLab is conservatively overcharging the two observed requests, not losing money. The fixed price remains unchanged for now because switching to dynamic billing without a verified no-usage fallback could reduce a successful image request to a near-zero charge when an upstream response omits token usage.

### `grok-video-3`

A six-second request proved that DeepWL charges this model per request:

- Before the fix, DeepWL charged `200,000 quota` while CarLab multiplied the fixed price by six seconds and charged `1,200,000 quota`.
- The root cause was the generic OpenAI/Sora task adapter applying its `seconds` multiplier to a DeepWL model whose upstream price is already per request.

The Railway `new-api` service now sets:

```text
TASK_PRICE_PATCH=fox-grok-video-3,deepwl/grok-video-3
```

This uses New API's existing per-call task-price path for both the compatibility alias and namespaced model. After deployment, a fresh six-second CarLab request charged `200,000 quota`, and the matching DeepWL settlement log also charged `200,000 quota` in `default` group.

## Changed files and cloud resources

- Added `docs/catalog/deepwl-pricing-audit-2026-07-12.json`.
- Added this operation record.
- Documented the existing `TASK_PRICE_PATCH` runtime setting in `.env.example` without storing the production value.
- Updated the root `AGENTS.md` Operations Index.
- Updated Railway project `carlab-api`, service `new-api`, variable `TASK_PRICE_PATCH`.
- Railway deployment `8fce2d72-981b-490c-877a-23d80de71f03` completed successfully.

No source-code billing implementation, model ratio, completion ratio, cache ratio, fixed price, DeepWL key, or CarLab downstream group multiplier changed in this round.

## Verification evidence

- Railway deployment status: `SUCCESS`; application instance: `RUNNING`.
- The running container reports both verified `grok-video-3` model IDs in `TASK_PRICE_PATCH`.
- `https://api.carlab.top/api/status` remains healthy.
- The 67-model static comparison returned zero pricing mismatches.
- All three DeepWL keys' successful settlement logs use `default` group.
- Fresh `gemini-3.5-flash` settlement matched the `default` formula.
- Fresh six-second `fox-grok-video-3` settlement matched at `200,000 quota` on CarLab and DeepWL.

## Rollback

The previous Railway state had no `TASK_PRICE_PATCH` value. To restore it:

```bash
railway variable delete TASK_PRICE_PATCH --service new-api --json
```

Wait for the resulting deployment to reach `SUCCESS`, then confirm the running container no longer exposes the variable. This rollback restores the old seconds-multiplied behavior and should only be used if DeepWL changes `grok-video-3` back to duration-based settlement.

Documentation rollback is a normal Git revert of this operation record, its evidence JSON, and the Operations Index row.

## Remaining risks and next gate

- Actual settlement has not been sampled for all 67 imported DeepWL models. Static API equality is not enough for OEM release.
- The 52 newly visible DeepWL models are not yet imported into CarLab.
- `gpt-image-2` needs a dedicated usage-aware rule with a conservative fallback before CarLab can promise exact pass-through cost.
- DeepWL's public group data is incomplete for five referenced groups, and token group membership is not exposed by the token-usage endpoint.
- Any replacement or regrouping of a DeepWL key must trigger a low-cost settlement audit before the channel is enabled for downstream resale.
- Upstream procurement groups should be stored and displayed in a separate cost ledger. They must not reuse CarLab's downstream billing-group fields.
