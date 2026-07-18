# NodyHub Catalog Source Metadata

## Scope

This operation adds lossless upstream metadata storage for model catalog imports and a controlled way to defer repeated pricing rebuilds during a large administrator-driven model refresh. It supports the Superseed NodyHub full-catalog replacement without changing relay protocols, billing formulas, or non-NodyHub routing.

## Assumptions

- NodyHub raw model objects are serialized by the caller and stored as text; CarLab does not interpret or execute arbitrary upstream metadata.
- `defer_refresh=true` is used only while the affected channel is disabled. The operator must finish with one complete model PUT without that query parameter after re-enabling the channel.
- Model CRUD remains administrator-only. This change does not add a public or token-scoped write surface.

## Changed Files

- `model/model_meta.go`: adds optional `source_metadata` TEXT storage and includes it in full model updates.
- `model/model_meta_test.go`: verifies insert, update, database, JSON, and channel-provider relationship behavior.
- `controller/model_meta.go`: honors `defer_refresh=true` on model create, update, and delete so a controlled bulk operation can rebuild pricing once at the end.

## Cloud Resources And Configuration

- Target: Railway project `carlab-api`, service `new-api`, production environment.
- Database: existing Railway PostgreSQL. The existing GORM `AutoMigrate(&Model{})` startup path adds the nullable TEXT column.
- New environment variables: none.
- Secret changes: none. NodyHub and CarLab administrator credentials remain outside Git and deployment payloads.

## Verification

- `go test ./model -count=1` passes, including the new source metadata round-trip test.
- `go test ./controller -count=1` currently has one pre-existing failure in `TestListModelsTokenLimitIncludesTieredBillingModel`; the source metadata and deferred-refresh changes do not touch that test path.
- `git diff --check` passes for the operation-owned files.
- Production deployment and API round-trip evidence will be appended after the clean-worktree Railway rollout.

## Rollback

1. Revert the operation commit and deploy the previous CarLab API revision from a clean worktree.
2. The nullable `source_metadata` column may remain; older code ignores it. Do not run destructive DDL during application rollback.
3. If a catalog refresh was in progress, keep NodyHub channel 3 disabled and restore its model metadata, channel configuration, pricing Options, and provider relations from the pre-refresh secret-free snapshot before re-enabling it.

## Remaining Risks And Permissions

- Model CRUD has no transaction-wide batch API. The external synchronizer must keep the channel disabled and automatically restore from its snapshot on any partial failure.
- Option updates are one key per request and are not atomic as a group.
- NodyHub private endpoint labels do not create new CarLab protocol adapters; runtime support still requires an existing CarLab route and representative smoke test.
- Production rollout requires Railway CLI access. The repository's unrelated uncommitted changes must not be included; deployment must use a clean worktree at the exact operation commit.
