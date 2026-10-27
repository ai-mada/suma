# Repair plan revisions and task execution provenance

Stage T-P4-033; suma 1.0.0. Implementation contract, 2026-10-27.
Extends032 while preserving the complete P0-P7 objective. Explicit user-approved
implementation is autonomous; this contract does not claim deployed or verified
behavior. No vendor SDK/model, broad suite or unchanged032 journey replay is
required merely to develop this stage.

## Observable behavior

1. A repair task has one current prepared plan. Replacing an expired, disabled or
   unsuitable tool/workflow creates a **new plan ID**, keeping the same reserved
   business task, immutable original conflict identity and ordered source inputs.
2. The previous plan, its definition, run, attempts and execution payload remain
   unchanged. An immutable revision family records root/parent/ordinal, current
   author, reason, definition digest and any predecessor recovery basis. The
   existing conflict-to-repair reservation moves its current plan by CAS.
3. Revision creates a ready preview only. Its separate acknowledged Start creates
   its own managed run. No workflow/profile substitution, task cloning, model
   invocation or implicit start occurs during revision.
4. A ready unstarted plan may be revised. A failed plan may be replaced only when
   every attempted producer result is terminal known failure with an authenticated
   typed host startup proof before workspace/main process entry, no executed
   descendants, no candidate/transfer, no compensation and no unknown/live work.
   Missing output, absent evidence, error prose or an observation timeout do not
   certify startup failure. Unsupported cases keep original-operation recovery.
5. A failed predecessor remains the task's current run until replacement Start.
   Revision and Start both recheck task/run versions, complete attempt/result
   digest set, fixed source acceptance and current actor/profile/workflow/route.
   Start atomically records provenance and rebinds that same task.
6. Superseded plans cannot claim, access fresh code, start, retry or resume, even
   while the replacement is only a preview. Existing exact facts, receipts and
   appropriately authorized cancel/reconcile remain available independently of
   the current head. Old Start/revision receipt replay restores the original
   operation identity, never compiles or starts the new head.

Native model selection, frozen quote and budget handling for Create/revision
follow [repair model execution](repair-model-execution.md); Codex repair remains
unsupported under the dedicated native workspace lifecycle.

The initial revision is ordinal1; at most32 semantic revisions per family.
Refresh is still an expiry-only operation on the current ready plan. It cannot
alter its selected definition. Accepted candidates/plans cannot be revised.

## Domain and storage

Domain agent owns new revision eligibility/create/lineage helpers, current plan
read/start/source/head guards, repair DTOs and composition migration integration.

- `CandidateCompositionRepairRevisionRequest`: project_id, expected_version,
  expected_task_version, expected_conflict_sha256, workflow_id, workflow_version,
  producer_node_id, tool_profile_id, expected_tool_version,
  model_execution_selection?,
  expected_recovery_sha256?, reason, acknowledge:true, idempotency_key.
- `CandidateCompositionRepairRevisionContext`: project_id, plan_id,
  plan_version, task_version, conflict_basis?, recovery_basis?, can_revise,
  blockers. Add explicit eligibility mode if it improves safe consumer selection.
- `CandidateCompositionRepairRecoveryBasis`: plan/task/run IDs and versions,
  complete ordered attempted assignments and exact result/startup-proof digests,
  sha256 calculated with its own digest cleared.
- `CandidateCompositionPlan.repair_revision?`: safe server-derived lineage with
  root_plan_id, parent_plan_id, ordinal, current_head_plan_id, is_current,
  reason/created_at, definition_sha256, creator kind/principal identifier. Never
  expose tokens, secret material or token hashes, or infer creator from viewer.

Methods (Actor is a value):

`GetCandidateCompositionRepairRevisionContext(ctx, engine, actor, project, id)`
returns the context; `ReviseCandidateCompositionRepair(ctx, engine, actor,
project, id, request)` returns `CandidateCompositionPlan`.

New immutable table `suma_candidate_composition_repair_revisions` records each
plan's root, parent, ordinal, task, creation reason/time and immutable safe
lineage/definition/recovery payload. Use scoped uniqueness for root+ordinal and
one successor per parent. Initial032 records can be snapshotted as observed
initial definitions when their first explicit revision occurs; do not fabricate
historical authorship or original preparation timestamps.

Original `suma_candidate_compositions` and per-run execution snapshots retain
their existing role. Revision does not mutate the old plan/version/state. Only
the protected reservation current head changes, in the same transaction as the
new plan/lineage/outbox/receipt. Fresh conflict CAS uses current whole basis SHA;
its immutable conflict identity must equal the reserved original. Sources remain
the exact frozen accepted bindings. Newly selected authority is resolved from
the actual current command; no historical credential grants new permission.

## Shared host startup proof

Runtime/provenance agent owns a standalone `internal/executioncontract` module:
`StartupFailureProtocol = suma.execution-startup-failure.v1` and
`StartupFailureProof {protocol,stage,assignment,profile,purpose,
workspace_started,process_started}`. Stage is exactly `before_workspace`, purpose
is `candidate_conflict_repair`, both started flags are false. Assignment uses the
complete existing private/Pool `candidatecontract.AssignmentRef`; profile is the
exact typed fixed `toolcontract.Profile`.

Add `startup_failure?` to Runner and DAG results and copy it across transport.
Runner may emit it only on a failed dedicated repair dispatch before entering
`prepareWorkspace` or the main agent process. Once preparation begins, even a
preparation error cannot carry this proof. The proof does not grant execution or
change unknown/cancel/side-effect semantics. Mixed/invalid envelopes are refused.

Shared structural validation:
`ValidateStartupFailureProof(proof, expectedAssignment, expectedProfile, purpose)`.
Platform `validateStartupFailureResultTx(ctx,tx,tenant,project,result)` validates
present proofs; `CertifiedCandidateRepairStartupFailureTx(ctx,tx,tenant,project,
result) (bool,error)` is reused by revision/accounting. Missing proof returns
false. Compare against immutable execution/node/assignment/profile facts, not
current head or current enabled profile, so historical facts survive revocation.
Reject success, stop, unknown, workspace/tool/candidate/artifact/release or
integration effects and any contradictory prior model counters.

Only certified startup failure with no earlier usage may settle explicit zero
input/output tokens via existing accounting states, release unused reservations
and clear unresolved exactly once. Usage source is `host_startup_proof`, retaining
the exact original result/proof identity. Reuse settled/unpriced and existing rate
semantics; no new billing state, invented currency/rate or retroactive refund of
older evidence-free failures. Native eight-tool/repair-v1 SDK remains unchanged.

## Global task/run association

Runtime/provenance agent owns `task_run_provenance*.go`, focused checks and a
minimal `delivery.go` hook. A new `suma_task_run_provenance` registry keeps scoped
run-to-one-task uniqueness permanently, including historical runs after rebind.
Current task pointers and protected managed execution snapshots are checked too;
an ordinary task cannot attach a repair run or steal another task's run.

`bindTaskRunProvenanceTx(ctx,tx,auth,task Record,newRun string)` participates in
the same binding transaction, including exact no-op replay. New binding records
the actual bound task version/time. When an older current reference is observed
before rebind, preserve it with observed_at and binding_kind=observed; bound
version/time remain nullable unless an immutable source actually proves them.
Do not pretend current task version/time was its original binding version/time.

`TaskRunProvenance {task_id,run_id,task_version?,bound_at?,observed_at,
binding_kind,plan_id,current}`; `TaskRunProvenancePage` is bounded/paginated.
`ListTaskRunProvenance(ctx,actor,project,task,limit,before)` uses current scoped
read authority and project/task/actor-bound cursor. No GET populates history.
`MigrateTaskRunProvenance` is called from existing task-dispatch migration so
normal and fixture startup paths create the registry before BindRunTx.

## Public interfaces and UI ownership

Root owns HTTP, CLI, OpenAPI/TypeScript generation, localization integration,
callback-validator registration and record-provenance protection.

- GET `/candidate-compositions/{id}/repair-revision-context?project_id=...`
- POST `/candidate-compositions/{id}/repair-revisions`
- GET `/tasks/{id}/run-history?project_id=...&limit=...&before=...`
- CLI `candidate-compositions revision-context ID`, `revise-repair ID --file ...
  --confirm`; `tasks run-history TASK_ID` follows existing CLI conventions.

Reject unknown/duplicate/null fields, wrong project/path, noncanonical versions,
missing acknowledgement and forged source/assignment/history. Root protects new
lineage/history/startup keys in generic record/data writes. Errors are structured
and paired CN/EN. Product remains1.0.0; native conflict-repair protocol stays v1.

UI agent owns a folded `CandidateRepairRevisionPanel` on the repair child, pure
wire guards, paired catalogs and focused consumer checks; minimal composition
panel integration and a compact task execution-history view. Configuration loads
only when opened. Form has workflow, agent producer, native profile and reason;
route is current server fact, not arbitrary client input. Safe old/new details
show the reviewed change and exact previous/current plan links. Successful
revision opens its exact returned plan URL and clears Start acknowledgement.
Old links do not silently jump to latest. Unknown outcomes retain path/body/key.

UI also owns the necessary shared-write seam: validate response shape **before**
success clears durable pending input. Malformed success remains an uncertain
original operation; do not enable a new request. Write401/403 immediately hides
protected child facts. Coordinate shared API semantics with Root before editing.

## Verification and resource policy

Use focused domain concurrency/current-authority/history and typed-result
boundary checks, one real HTTP/Pool/CLI seam per changed contract and actual
response-shaped UI consumers. Do not repeat32's16-source Git/SDK loop or complete
business/model journey when bytes/semantics are unchanged.

The actual033 journey should reuse accepted A/X, preserve an original conflict,
revise a ready plan without task/run creation, record a genuine host startup
failure, explicitly revise to a healthy native tool, start the new run, test its
exact candidate independently, Review and bind an unstarted downstream task.
Prove both old and new task/run links, old callback/Start receipt recovery and
new-head-only admission. A controlled missing owned workspace root before
prepare can exercise startup failure; do not mutate shared source or infer it
from error text. Six bounded local protocol model responses suffice for the one
healthy execution; no vendor or repeated source development is needed.

Keep primary032, user browser tabs, existing source/runtime/evidence and all
credentials untouched until reviewed forward upgrade. Use ignored cache for
scratch. Commit necessary source/contracts/docs/compact evidence after stage
verification; no cache, DB, token, logs, screenshots, binaries or push.
