# Application release rollout contract — 1.0.0

Stage T-ROLLOUT-125. Authoritative shared DTOs: `internal/platform/application_release_rollout_types.go`; kernel binding: `internal/dag/bound_subworkflows.go`. This extends the original [release-set contract](application-release-sets.md) without introducing group approval or another deployment protocol.

## Scope and API

All routes are under `/api/v1/application-release-sets/{set_id}/rollouts`. The set and plan group are immutable, project scoped, and limited to 16 members. Dependency cycles block automated ordering. Start and lifecycle commands require the current browser principal and release authority; machine credentials cannot authorize a rollout or approve a plan. Current read permission is required for reads.

| Method / suffix | Purpose |
| --- | --- |
| `POST /preview` | Pure current start preview |
| `POST /` | Explicit start with exact preview digest |
| `GET /` | Bounded rollout list |
| `GET /{id}` | Immutable identity and current parent/member facts |
| `POST /{id}/resume-preview` | Pure current recovery preview |
| `POST /{id}/commands` | Explicit pause, resume or cancel |

Start preview keys: `project_id`, `plan_group_id`, `expected_set_sha256`, `expected_plan_group_sha256`, `reason`. Start adds `expected_preview_sha256`, `idempotency_key`.

Resume preview keys: `project_id`, `expected_run_version`, `expected_rollout_sha256`. Command keys: `project_id`, `expected_run_version`, `expected_rollout_sha256`, `action`, `expected_preview_sha256`, `reason`, `idempotency_key`. Actions are `pause`, `resume`, `cancel`; a fresh preview digest is required for resume and an empty digest is used for pause/cancel. Exact canonical shapes, duplicates, unknown fields and nulls follow the current strict request decoder. GET includes one project ID and uses bounded cursor pagination for the collection.

## Protected identities

The immutable rollout binds exact set/group digests and each original plan's ID, version, revision and digest. Each member records its original reserved child run/node, parent node, service binding and dependency application IDs. A binding string copied into another workflow conveys no authority.

All original plans must already have current explicit approvals. Start revalidates actor, approvers, source/quality, plan and environment bases; it atomically creates the parent and protected relationships. Only eligible members acquire their original child run, deployment operation and environment reservation when the existing scheduler actually starts them. Unstarted downstream members have none of those execution effects.

The ordinary original plan, authorization ticket, effect grant, provider protocol, artifact and health contracts remain authoritative. Dependency success requires matching current original child success and deployment effect/health facts. Provider acknowledgement alone is insufficient.

## Lifecycle and recovery

Pause stops new member starts while current children drain. Failure, an unknown outcome or a failed current gate pauses further progression. Pause does not mean cancellation, confirmed process stop or rollback.

Resolve unknown outcomes through the original deployment reconciliation API. Keep the original operation and the original unknown result; do not redispatch to obtain a clean receipt. Reconciliation does not implicitly resume the parent. Resume requires a fresh current preview, revalidates unfinished plans and completed facts, and preserves healthy original children without replaying deployment effects. It may finish an already healthy but paused parent.

Known failures cannot use a generic blind retry. A corrected recovery requires a new reviewed and approved plan. Cancellation retains the original actual-stop and effect semantics; it does not erase applied facts. Coordinated rollback, cross-environment atomic execution, arbitrary mid-run plan replacement and traffic canaries are outside this contract.

Stable operation keys and expected versions protect commands; current authorization precedes receipt recovery. Current read views expose parent state, pause reason, original per-member run/operation/effect/health facts and bounded blockers, without source bytes or provider credentials.
