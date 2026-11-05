# Application release sets — 1.0.0

Stage T-RELEASESET-124, requirement 25. Authoritative DTOs are `internal/platform/application_release_set_types.go`; original-plan provenance is `releasecontract.ApplicationReleaseSetRef`. Generated OpenAPI and TypeScript types use these contracts. No legacy field aliases or alternate execution path are defined.

## Immutable association and proof

Each member supplies `application_id`, `application_version_id`, `application_version_sha256`, `candidate_id`. The set stores exact artifacts, candidate snapshots, build/package-test proofs, original source-task bases and the candidate's own accepted build-task bases. These two task sets may differ and must not be rebound.

Maximum members: 16, unique application IDs, without reusing a candidate/artifact object. Every member's source scopes must have one governed target identity and one commit; unsupported multi-origin/multi-commit versions are blocked. Candidate source commit must match, and build/package-test proofs must identify the same artifact/manifest/source with different Workers.

Member-to-candidate selection is explicit and retains the review reason. A scoped build proof must match the exact application version and its registered single source scope. A candidate from a scoped build cannot be attached to a sibling application or another version, even with the same commit. A project-wide build without scoped proof can be manually associated and keeps its project-wide provenance. Current deployment transport uses the static-site protocol.

The selected versions' pinned dependencies must all be present with their exact version ID and SHA. Missing, conflicting or explicitly incompatible edges block registration. Unknown compatibility requires `acknowledge_unverified_compatibility`; acknowledgement never changes it into verified compatibility. Cycles are retained as strongly connected groups.

## Scoped build evidence

An administrator-bound `command` node can supply `config.application_build` with these exact string fields: `application_id`, `version_id`, `version_sha256`, `scope_id`, `scope_root`, `scope_tree_oid`, `source_commit`, and `receipt_sha256`. The empty root denotes the repository root. The selected registered version must have one source. Its snapshot, Git tree, source commit, and original integration receipt remain authoritative.

The project release Runner uses an approved build command profile. Control checks the binding before claim, again before new artifact upload, and at successful completion. The Runner verifies the actual checkout and Git tree, executes from the non-linked scope directory, and collects the configured relative output there. Its build proof records the binding. The original package-test attempt continues to verify the sealed artifact and manifest independently. CWD and tree evidence cover the selected source scope; external dependency access remains under administrator command policy. Use project release Runner credentials or an approved execution Pool with dedicated artifact-capable profiles. The Pool requires the current bilateral grant on each transfer and successful proof. [Pool artifact protocol](execution-pools.md#应用构建制品与独立包测试)

## Project-scoped API

All paths are under `/api/v1`. GET requires exactly one `project_id`; list uses `limit` (default20, maximum100) and optional returned `before`. Queries and bodies reject unknown fields, aliases, duplicates and required-null values through the current strict decoder. Read/preview follows current browser/project-API project authority; registration and preparation require the current browser's write authority. No machine credential substitutes for a human decision.

| Method / path | Result |
| --- | --- |
| `GET /application-release-sets` | Bounded summaries and cursor |
| `GET /application-release-sets/options` | Current application/version and candidate options with coverage |
| `POST /application-release-sets/preview` | Read-only set, preview SHA and blockers |
| `POST /application-release-sets` | Explicit immutable registration |
| `GET /application-release-sets/{id}` | Immutable set, current basis and bounded plan-group facts |
| `POST /application-release-sets/{id}/deployment-preview` | Per-member original release previews |
| `POST /application-release-sets/{id}/deployment-plans` | Atomically prepared original pending-approval plans and group |

Set preview keys are `project_id`, `label`, `reason`, `members`, `acknowledge_unverified_compatibility`. Registration adds `expected_preview_sha256`, `idempotency_key`. Arrays are explicit and use `[]`, not null.

Deployment preview keys are `project_id`, `expected_set_sha256`, `environments`, `reason`; every environment item contains `application_id`, `environment_id`, `expected_environment_generation`. Every set member must be mapped once; repeated environments or the same provider/site are rejected. Preparation adds `expected_preview_sha256`, `idempotency_key`.

## Transaction and original-plan binding

Preparation rechecks set/current member proofs and each original release preview inside one transaction. It creates original `ReleasePlan` records, the immutable group, protected plan bindings, activity and operation receipt together. It does not reserve environment generations, grant approval, create a run or deploy.

Each original plan content includes `application_release_set` with `release_set_id`, `release_set_sha256`, `application_id`, `application_version_id`, `application_version_sha256`. This reference enters the original plan SHA. Approval, execution, authorization and effect gates revalidate the current set/group/member binding through the original plan-basis check. A reference conveys no authority.

Repeated commands restore their original identity only after current authorization. Source/candidate/version, preview or environment drift is a conflict requiring a new review, not silent replacement. Stable operation recovery does not perform a second deployment.

## Facts and limitations

The immutable set has no invented group approval/execution state. `ApplicationReleaseSetPlanGroupView.Items` derives each original plan version/state, deployment operation/state, effect and health. Pending, partial, failed and unknown member facts remain distinct; one healthy member cannot make the group healthy.

History remains immutable when current eligibility changes. Rollback, cancellation, unknown-effect reconciliation and environment barriers remain original per-operation protocols. Dependency ordering is separately governed by the [ordered-rollout contract](application-release-rollouts.md), requiring original individual approvals and an explicit start. Cross-environment atomic rollout, coordinated rollback, generic container/package publication and automated builds covering multiple source scopes remain separate.
