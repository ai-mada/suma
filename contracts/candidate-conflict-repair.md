# Governed conflict repair and ordered source recovery

Stage T-P4-032; suma 1.0.0. Frozen implementation contract, 2026-10-27.
This extends the complete P0–P7 platform and the accepted031 composition path.
Implementation, local execution, physical multi-host and production acceptance
remain separate. The original failed plan never becomes an accepted repair.

## Observable delivery and task ownership

1. Read an exact known conflicted composition from its current authorized
   durable plan/run/node/attempt/fence/worker. Expose the original literal paths,
   ordered sources and rich host Git evidence separately from current eligibility.
2. Explicitly select a published workflow, root agent producer and administrator
   opted-in native tool. Provide a repair task title, description and acceptance
   criteria. `CreateCandidateCompositionRepair` atomically creates one unstarted
   task and one new ready composition plan; it does not execute development.
   The UI action explicitly says **创建修复任务并预览 / Create repair task and preview**.
3. A separate acknowledged Start creates the new managed DAG run through the
   existing Go scheduler. The new plan stores its actual initiating credential,
   exact task/source/tool/workflow versions, route and immutable ConflictBasis.
4. The trusted native workspace reimports only the exact frozen authorized
   bundles. It recreates the actual conflict and exposes bounded file tools plus
   host-owned merge status/continue tools. All2–16 ordered sources are processed.
5. Each continuation explicitly acknowledges every active conflicted path and
   its current hash or deliberate absence. Host Git records a real resolved
   prefix commit, processes the next fixed source, and pauses for any later
   conflict. No prefix result is sealed, accepted, transferred or called complete.
6. Only after every source has been processed and current development checks
   pass may the host seal the final candidate. The existing same-run
   `candidate_from` node independently tests its exact commit. Original browser
   Review decides the new repair task's acceptance.
7. Only that genuinely accepted **new plan** may enter the existing governed
   target integration or protected downstream code-baseline binding. The
   original conflicted plan, failure, tests and source records stay unchanged.

Creation reuses an existing repair link for the same original conflict identity
rather than creating duplicate tasks for another operation key. The protected
link uses immutable composition/assignment/result/evidence identity, excluding
mutable current task and run versions. The whole basis digest remains the fresh
compare-and-swap requirement; a task edit cannot hide an existing repair. Exact operation
replay restores the original result under current authorization. A known failed
repair uses governed run recovery; an unknown outcome must first reconcile the
original operation. Neither observation timeouts nor source drift justify a new
model session, silent retry or replacement task.

Managed composition runs expose the existing scoped pause, resume, cancel,
retry and reconcile commands. New execution rechecks current source, creator,
route and fixed tool authority. A retry requires a known failed node whose
published definition is idempotent; it retains the failed attempt and advances
the attempt/fence. Already executed descendants and unknown outcomes retain the
ordinary DAG recovery restrictions. Stopping or reconciling existing work does
not require granting a new source execution. Deployment runs keep their own
governed command path.

## Domain and API

- `GET /candidate-compositions/{id}/conflict?project_id=...`
- `POST /candidate-compositions/{id}/repairs`
- Existing composition Get/List/Start/commands and task Review remain the same
  shared flow. A new repair plan uses `strategy: conflict_repair` and its non-null
  `conflict_basis`; an ordinary plan uses `strategy: merge_commit` without that
  basis. Clients validate both together rather than treating all plans as clean
  merges.
- POST requires exact project, expected_composition_version,
  expected_conflict_sha256, repair{title,description,priority?,owner_id?,
  acceptance_criteria}, workflow_id/version, producer_node_id,
  tool_profile_id/expected_tool_version and stable idempotency_key.
- The repair task has a title of at most200 characters, description of
  at most16,000 characters and1–16 nonblank acceptance criteria, each at most
  1,000 characters. The server validates actual current member ownership.
  Omitted priority defaults to `medium` before receipt hashing; explicit invalid
  priorities and null optional strings are rejected. An omitted owner follows
  the current account only when it uniquely identifies an active project human
  resource, otherwise the task remains unassigned.
- Context returns project_id, composition_id/version, basis?, repair_plan_id,
  can_create and blockers. Recorded conflict facts never imply current permission.
- New plan additionally records tool_profile_id/version/sha256. Existing
  CompositionID-based CodeBaselineBinding refers to this new plan after its own
  acceptance; no ambiguous union or relabelled failed original is introduced.

`CandidateCompositionConflictBasis` fixes composition/task/run IDs and versions,
node/attempt/fence/worker, complete assignment, exact stored result SHA256,
canonical typed composition evidence SHA256, the evidence itself, and the whole
basis SHA256 (computed with its own SHA field cleared). The request's expected
conflict SHA is that whole basis identity.

Source acceptance, task context, current actual credentials, tool observation and
route are checked at create/start/fresh code access and again for new approval or
handoff. Genuine late completion remains an immutable fact even when a current
source or credential changes. All private and Pool identities are retained;
repair permission never becomes general Git, shell, network or Review authority.
A reserved repair task cannot be dispatched through generic task/expert paths
without its protected prepared repair input.

## Native execution protocol

`internal/candidatecompositioncontract` owns
`CandidateConflictRepairProtocol = suma.candidate-conflict-repair.v1` and
`CandidateConflictRepairExecution`:
protocol, plan_id/version, project_id, composition_id/version,
original_assignment, original_evidence_sha256, composition (the original frozen
composition execution), conflict (its typed rich evidence), instructions,
fixed_timestamp and operation_id. OriginalEvidenceSHA256 is the canonical typed
original composition evidence digest; the full original result digest remains
separate in ConflictBasis.

The assignment field is `candidate_conflict_repair`, mutually exclusive with
normal composition, accepted code baseline, same-run candidate import, repository
integration and release modes. Producer type is agent. `CodeInputRequest` uses
input_kind `conflict_repair`, binding_id equal to the new repair plan ID, and a
member transfer ID. A raw workflow config cannot grant cross-run code access.

Rich original conflict evidence includes conflict_tree, conflict_stages of
{path,mode,object,stage}, and conflict_sha256 in addition to the original fixed
source order, prefix history and conflict source index. Incomplete older reports
remain readable but are explicitly ineligible; do not synthesize or rewrite them
as if they recorded the missing facts.

`NativeWorkspacePolicy.conflict_repair` is an explicit local administrator opt-in.
Only that probed native profile advertises ToolExecutionProfile's
`conflict_repair_protocol`. Ordinary commands, vendor CLIs and existing six-tool
native sessions do not silently gain this capability. Repair sessions have the
exact authenticated eight-tool catalog, including:

- `suma_workspace_merge_status({})`
- `suma_workspace_merge_continue({expected_step,expected_generation,
  expected_state_sha256,resolutions:[{path,expected_sha256}|{path,
  expected_absent:true}]})`

The operation ID uses the existing authenticated MCP operation metadata. All
paths remain under the private rooted attempt workspace; host Git state and the
journal remain inaccessible as agent-editable authority.

## Git, sealing and recovery invariants

Use mature hermetic Git `merge-tree --write-tree` and its actual tree/higher-stage
records. Do not introduce external merge drivers, hooks, rerere, guessed branch
heads, ours/theirs shortcuts or silently omitted later sources. A private
host-owned Git repository/worktree retains all frozen source objects. Every
materialization, resolution, prefix commit and next-source transition is tied to
an original durable operation, step, generation and state digest.

Continuations acknowledge the complete current conflict path set, reject stale
hashes/absences and unresolved markers, invalidate previous development checks,
and record actual ordered merge parents. All sources must be ancestors of the
final result; this supplements actual processing of their content and cannot
replace it. Final output closure is independently importable from the original
administrator anchor B. No temporary seed or unresolved prefix is a candidate.

Pause/cancel/unknown retain the existing DAG attempt and fencing semantics. A
lost continuation ACK recovers the same operation and prefix without repeating
model work. A replay with altered input is rejected. A failed compensation or
uncertain write remains visible; source validity changes block new use rather
than erasing recorded execution facts.

## Interaction and language

Reuse the current task/composition panel and design tokens. Default-fold the
repair section; folded state performs zero reads. Show one short next action,
exact source/run links, editable repair goal, fixed tool/workflow preview and a
separate Start confirmation. New task creation, running, sealed candidate,
independent tests, awaiting Review and accepted are separate facts.

URLs identify exact plan/task; no invented latest ID. Current read failure or403
hides protected facts and disables writes;409 preserves draft and requires a
fresh read plus renewed acknowledgement. Scope changes abort old requests.
Unknown writes retain exact original path/body/key. Only an opened active plan
refreshes. CN/EN system labels are paired; user text, paths, code and original
technical evidence remain literal. Configuration UI creates a local admin
configuration example; it never pretends to mutate a Runner's private files.

## Meaningful acceptance

- Exact HTTP/CAS/idempotency, wrong-scope/fence/source/creator and protected-task
  rejection; atomic creation and same-conflict task reuse; immutable old failure.
- Native realGit first conflict plus a later clean source and later conflict,
  exact stage materialization, full source ancestry/content and independent
  final bundle import/test. Test stale/omitted resolutions, marker rejection,
  remaining-source/check-stale seal denial and original-operation ACK recovery.
- Necessary private/Pool claim, code bytes and result boundaries consume the
  shared typed envelope; current-source invalidity never deletes late facts.
- Representative actual UI creates/starts a repair, observes independent tests,
  reviews its own exact result and explicitly hands off the accepted baseline.
  CNEN, refresh, keyboard and original request recovery use the same facts.

Reuse unchanged031 evidence and dependencies. Run only affected checks and one
final isolated build, keeping primary031 unchanged until the new journey is
closed and artifacts are reviewed. Vendor models/physical multi-host/production
and the full remaining platform stay explicit future acceptance scopes.

## Parallel ownership

Domain agent: composition repair/types/storage/source/acceptance/task guards and
focused domain checks. Runtime agent: shared repair contract, Runner/native
workspace/MCP bridge/SDK and realGit checks. UI agent: compact repair panel,
passive facts, local configuration example, CNEN catalogs and four design docs.
Root: HTTP/private/Pool/DAG/capability field, schema generation, CLI/localization,
shared entry integration, final meaningful actual journey, documentation,
preview update and exact necessary-file local commit. No push is authorized.

## Ready preview expiry and known-failure retry

An explicit repair-only `refresh` command uses the original CommandRequest
(expected_version, reason, stable key). It revalidates the unchanged task, exact
conflict, sources, native profile and route, then only advances plan version and
expiry. It creates no task or run, does not change definitions, and never renews
a normal or already-started composition. Saved operation replay returns the
original refresh version even after Start. The UI does not auto-renew previews.

Derived repair execution keeps MaxRetries=0 and the administrator template's
Idempotent choice. Known failed runs may use existing explicit DAG retry only
when that policy permits; uncertain outcomes still reconcile their original
operation. Automatic repair recovery and reassignment of tool/workflow/plan
definitions remain separate incomplete enhancements in the full platform ledger.
