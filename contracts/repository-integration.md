# Governed repository integration

Stage T-P4-028, suma 1.0.0. Frozen integration contract, 2026-10-26.
The full platform remains in scope. This vertical path integrates an accepted,
independently tested candidate into an administrator-managed bare repository.
It does not claim remote Git hosting, arbitrary merge/squash, physical multi-host
or production acceptance. Existing source checkouts and the suma branch are untouched.

## Architecture and ownership

- A owns `internal/platform/repository_integration*.go` (except Root HTTP/tests),
  provider configuration loading, current authorization, source selection,
  plans/approvals/generation/barrier, tickets, provider facts and necessary tests.
- Runtime owns `internal/repositoryintegrationcontract`, `internal/repositoryprovider`,
  `cmd/suma-repository-provider`, Runner bridge and necessary wire/Journal/Pool/Core
  seams. No new general scheduler, tool adapter or model purpose.
- UI owns `RepositoryIntegrationPanel.tsx/.css/.test.tsx`, domain models/hooks and
  CN/EN. Root owns task entry, HTTP, main, generated schemas, design and integration.
- Reuse the mature release provider bridge: `external_workflow`, dedicated typed
  `Assignment.RepositoryIntegration` and `Result.IntegrationEvidence`. Runner makes
  authenticated outbound requests through `External[provider_id]`; no target path,
  shell command or provider secret comes from a user/model. Keep release evidence separate.

## Local provider and Git rules

Independent Go provider CLI: `--config FILE --state DIR --addr LOOPBACK --language`.
Admin config fixes provider/tenant/project, control origin, three distinct process
secret names (control callback, Runner transport, control inspect/reconcile), fixed
absolute Git executable, and targets `{alias,bare_repository,ref}`. Paths stay local.
Control `SUMA_REPOSITORY_PROVIDERS_JSON` fixes audience/id/aliases/origin plus callback
token-env and reconcile-token-env references. All tokens >=32 bytes; never expose
or persist authorization tickets, tokens, or local repository paths in public data.

Only regular, nonlinked, administrator-owned bare repos and configured
`refs/heads/*` are supported. Reject duplicate physical repo/ref aliases, shallow,
replace/alternates, symbolic target refs, unsafe object format or configuration drift.
Use sanitized Git environment, disabled hooks/global/system config and external
diff/textconv; no checkout/reset/push or arbitrary network fetch. Import only the
hash/size/header/pack-verified sealed candidate bundle authorized by Control.

Prepare imports immutable objects, verifies the expected target and ancestry and
returns actual file changes + unified diff. It never changes the target ref.
Maximum bundle64MiB, changed files1000, diff512KiB. Exceeding limits or unsupported
binary review is explicitly blocked; do not approve a silently truncated diff.
The fast-forward result is exactly the already-tested candidate, not a synthetic
untested commit. Divergence requires a newly developed/tested candidate.

Apply atomically CASes target ref, creates this operation's immutable receipt ref,
and advances a target-level Git fencing ref with `git update-ref --stdin` transaction.
Do not use partial `--batch-updates`. Receipt blob binds full request SHA, operation,
target/ref/config SHA, before/result, bundle/prepared SHA, target generation and grant.
`target == candidate` alone is never proof this operation applied. Later advancement
does not erase a valid original receipt. Generation is target-wide, not DAG Fence.

Write durable nonsecret intent/checkpoint before effects. Restart/timeout/cancel
queries the same operation; never blindly apply again. A missing receipt remains
unknown unless an explicitly authorized reconciliation atomically fences the old
request and creates a not-applied receipt. This must race safely with an orphaned
Git transaction: the positive or negative receipt wins, never both. GET inspect is
read-only; negative fencing uses explicit POST with a current reconciliation grant.

## Shared protocol (Runtime writes exact Go types first)

Protocol `suma.repository-integration.v1`. All exported types are uniquely prefixed
`Repository...`; reuse `candidatecontract.CandidateBinding` and assignment identity.

- `RepositoryExecution`: protocol, tenant_id, project_id, provider_id, target_id,
  target_alias, target_ref, target_config_sha256, plan_id, plan_version, phase
  (`prepare`|`apply`), operation_id, source (CandidateBinding), expected_target_head,
  expected_fence_oid (empty for absent), expected_git_generation, generation,
  prepared_sha256 (empty only for prepare). No filesystem path or secret.
- `RepositoryRequest`: execution plus assignment (CandidateAssignmentRef) and
  authorization_ticket. Stable request digest excludes ticket, LeaseUntil and
  DispatchState; all other assignment/audience/input fields are bound.
- `RepositoryTargetObservation`: alias, ref, config_sha256, head_commit,
  object_format, fence_oid, generation, available, reason, observed_at.
- `RepositoryFileChange`: path, old_path(optional), status, old_oid, new_oid,
  binary. `RepositoryPreview`: target_head, candidate_head, files, diff,
  diff_sha256, source_bundle_sha256, config_sha256, prepared_sha256,
  fast_forward, blocked_reason. Hash canonical content, no self-hash recursion.
- `RepositoryObservation`: protocol, operation_id, request_sha256, phase, state
  (`preparing`|`prepared`|`applying`|`applied`|`not_applied`|`blocked`|`unknown`),
  version, execution, preview(optional), receipt_oid, receipt_sha256,
  effect_grant_id, reason, observed_at, observation_sha256. Known application
  requires complete validated Git receipt; invalid or conflicting facts fail closed.
- `RepositoryAuthorizationRequest`: project_id, assignment, operation_id.
  `RepositoryAuthorization`: authorization_id, authorization_ticket,
  expires_at, operation_id, request_sha256.
- Consume/inspect/begin-effect/reconcile grant request/response types follow
  release's explicit ticket binding, with dedicated Repository type names.
  Bundle download requires authorization_id + operation/request SHA + assignment.

Provider endpoints: authenticated GET `/targets/{alias}`, POST `/operations`,
GET `/operations/{id}`, POST `/operations/{id}/reconcile` (current one-use grant).
Control callback prefix `/repository-provider`: POST `consume`,
`inspect-authorization`, `begin-effect`, `bundle`, `observations`,
`consume-reconciliation`. Neither transport token nor history grants new effects.
Runner authorization path: `/runner/repository-integrations/authorize`; Pool path
`/pool-runner/repository-integrations/authorize` retains complete machine/slot/grant scope.

Provider posts authenticated observations to Control before Runner completion;
normal result validation matches that persisted fact, never trusts Runner text.
Control reconcile independently reads the fixed provider origin and validates the
same request/receipt; no external IO inside SQL transactions.

### Read-only export from a registered delivery branch

`POST /source-exports` is an optional provider-to-service operation. It uses a dedicated full-object export secret and a source alias with a configured `delivery_base_alias`. Both aliases resolve to distinct direct branches in the same administrator-managed bare repository. The source target's configuration digest binds its designated baseline alias and configuration. Request fields are closed and required: `protocol` (`suma.repository-source-export.v1`), `tenant_id`, `project_id`, `provider_id`, `target_alias`, `target_ref`, `target_config_sha256`, `expected_head`, `expected_fence_oid`, `expected_generation`, `expected_base`. For a source with no fence, send an empty `expected_fence_oid` and zero `expected_generation`.

The provider verifies both refs, source configuration, Git object format and generations, and requires the exact baseline to be a strict ancestor of the requested head. It builds a Git bundle with one exact prerequisite and one advertised `HEAD`, checks the pack envelope and Git bundle prerequisites, and reads both refs again before returning any bytes. Maximum bundle size is 64 MiB. Success returns `application/x-git-bundle`, `Content-Length`, `X-Suma-Bundle-SHA256` and `X-Suma-Source-Request-SHA256`. The request digest is SHA-256 of the validated typed request in canonical contract encoding. The body contains only the bundle. Rejections use JSON and omit bundle metadata. A concurrent export returns 429. This operation does not change source refs, import objects or create an integration record.

Export credentials stay with trusted services. The receiving system must check the expected request and bytes, import into an isolated repository with the exact prerequisite and verify the objects and resulting commit. Code candidate admission, a separate Runner's same-commit test and human acceptance require their own task-scoped evidence. The provider response establishes the configured Git source, with external agent authorship tracked independently.

## Domain and public API (A writes DTOs first)

`RepositoryIntegrationTarget`: id, project_id, version, name, provider_id,
target_alias, ref, config_sha256, status (`active`|`paused`|`retired`), generation,
active_operation_id, observation(optional), can_manage, blockers.
Binding provider/alias/ref/config is immutable; updates change name/status only.
Provider/config drift blocks new work; do not reinterpret old operations.

`RepositoryIntegrationBasis`: task_id/task_version, run_id, source_node_id,
source (CandidateBinding), accepted_task_basis (ReleaseTaskBasis used as a code
acceptance snapshot, not as a release artifact), basis_sha256.

`RepositoryIntegrationPlan`: id, project_id, version, task_id, target_id, state
(`preparing`|`ready`|`approved`|`rejected`|`applying`|`integrated`|`blocked`|`reconciling`),
basis, target, prepare_run_id, apply_run_id, preview(optional), prepared_sha256,
approval(optional), operation_id, observation(optional), created_at, expires_at,
can_approve, can_execute, can_reconcile, blockers.
Approval freezes approver/reason/plan version/prepared SHA and exact current basis.
GET history never substitutes current content for immutable evidence.

- GET `/repository-integrations/options?project_id&task_id`: current accepted
  source choices, configured targets/providers and can_create/can_manage/blockers.
- GET/POST `/repository-integration-targets?project_id`: project list / approved
  binding create. PATCH `/{id}`: explicit expected_version/name/status/key.
- GET `/repository-integrations/plans?project_id&task_id&limit&before` (20/max100).
- POST `/repository-integrations/plans`: project_id, task_id,
  expected_task_version, source_node_id, target_id, expected_target_version,
  idempotency_key. Creates immutable basis and managed prepare run atomically.
- GET `/repository-integrations/plans/{id}?project_id`: original plan/current facts.
- POST `/{id}/commands`: project_id, expected_version, action
  (`approve`|`reject`|`execute`|`reconcile`), reason, acknowledge (explicit true for
  approval/execution/reconciliation), idempotency_key. Returns current plan.

Draft preparation: current project writer (browser/scoped API); target binding,
approval/rejection: browser project approver. Execute/reconcile: current project
approver (browser/scoped API), with prior current valid browser approval for apply.
No Agent/Runner credential can make governance decisions.

Before prepare/download, recheck accepted task/review/requirements/dependencies,
source success/test same SHA/sealed bundle/current authorization. No fake old lease.
Execute binds unique run/node, operation, target generation + exclusive barrier,
approval and receipt in one SQL transaction. Before-effect consume/arm rechecks the
current worker/lease/route/grants and human basis, then persists the exact effect grant.
Revocation before arm stops new effects. An already granted effect may complete;
later revocation cannot erase genuine terminal facts. Unknown keeps the barrier.
Reconciliation never selects a human-supplied outcome. Only known provider proof
can release barrier; DAG managed recovery still rejects a current live lease.

## UI, verification and delivery

Task detail entry: `RepositoryIntegrationPanel {project,taskID,session,refresh,
planID?,onPlan:(id:string)=>void,onRefresh:()=>void}`. Root restores URL panel
`repository-integration:<id>` and does not add a new top-level workspace.
Show accepted source, actual target, prepare progress; fixed file list/diff;
separate reason+ack approval and execution; actual result/run links; current
conflict reasons and original-operation reconciliation. Hashes/history fold away.
Source text remains literal. All own labels/status/errors are extensible CN/EN.

Use existing Go/Git/HTTP/Radix/editor facilities, no new dependencies. Verify only
changed authority, exact evidence, SQL rollback/idempotency, Git atomic races,
source/target drift, lost ACK/orphan fencing and representative CN/EN actual flow.
Reuse a closed accepted fixture/bundle and isolated bare target, no model call.
Preserve original preview/data. Commit necessary source/design/compact evidence;
no raw DB, secret, binary, log or screenshot in Git. No push.

Git transaction source: https://git-scm.com/docs/git-update-ref (observed with
installed Git 2.56.0.windows.3); immutable operation objects follow existing native
seal patterns. Full P0–P7, other integration strategies and remote/production
acceptance remain in the implementation ledger.


### Missing-provider-operation recovery

`RepositoryReconcileRequest.request?` is the Control-persisted original
`RepositoryNonSecretRequest`. When the provider never received the initial POST,
this tuple establishes an unknown non-secret intent only. A current exact
reconciliation grant is still required for the Git negative fencing transaction.
It cannot create a new apply. Existing operations must match the same request
digest. Read-only inspection never consumes a grant or changes refs.
