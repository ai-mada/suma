# Expert team dependency orchestration

Stage: T-P4-029; suma 1.0.0. Extends the existing ExpertTeamPlan domain and
expert-team-plans API. Independent tasks are the all-root case of this same
business delivery graph. The Go DAG remains the only execution dispatcher.
No duplicate delivery-plan product, workflow engine or compatibility shim.

## Observable behavior and responsibility

1. Select 1–16 actual project tasks and fixed roles from one published expert
   configuration. The server freezes task/requirement/design/knowledge, role,
   resource, tool, workflow, project route and actual task dependencies. Compile
   this business graph: unique tasks, no cycle/self-reference/foreign reference;
   selected predecessors may be unaccepted, external predecessors must already
   have current formal acceptance. Do not expand scope implicitly.
2. Preview creates no DAG run or paid execution. Explicit acknowledged start
   activates the exact revision. Ready roots can be bound atomically immediately;
   waiting tasks occupy no machine slot. A persistent bounded controller later
   advances eligible tasks through the existing managed-run transaction.
3. A selected predecessor must be formally accepted against the exact run created
   by this plan, with current task/requirement/design/test evidence. Run success,
   exit 0, green tests or an Agent summary do not grant business acceptance.
   Existing `BindRunTx` and `Review` guards remain authoritative.
4. Display each task's waiting dependencies, ready/queued, executing, awaiting
   review, accepted, blocked or stopped state, current run/reason and explicit
   counts. Unknown effects, failed or cancelled runs do not create a new run or
   auto-retry. Existing run recovery remains on its original run/attempt.
5. Freeze task inputs for unstarted work separately from legal dispatch/review
   status changes. Drift in task body/assignment, requirement/design, dependency
   graph, configuration, resource, knowledge, tool or route blocks that unstarted
   work. Already-started definitions and received facts remain immutable.
6. Pause stops new dispatch and drains existing runs. Resume explicitly renews
   the current activation and rechecks all pending bases. Stop is terminal for
   unstarted items; it does not claim running processes stopped. Individual runs
   retain their actual cancel/reconcile controls and stop receipts.
7. An explicit revision of not-yet-started items creates an immutable new source
   revision and graph digest. Never replace a started item's role/source/run or
   retroactively approve changed evidence. Keep reviewable revision history.
8. This is business orchestration. Accepted upstream facts can be inspected;
   this change does not silently switch Git base or claim to combine separate
   candidates. Governed code integration and newly tested merge results remain
   distinct capabilities, with their existing authorization contracts.

## Reuse and persistence

Retain current expert configuration and plan heads/items. Introduce immutable
`suma_expert_team_plan_revisions` and a private
`suma_expert_team_plan_activations` control record if needed for durable due time,
generation and actual creator credential reference. Keep these concerns out of
public snapshot JSON. No token, serialized Actor or secret in storage/public UI.

The source builder must support a non-persisting snapshot. Existing task previews
expire in 30 minutes and cannot be precreated as authoritative future dispatch.
At actual eligibility build a fresh task dispatch plan, preserving the reviewed
semantic source and fixed identifiers, then bind task/run/item in one database
transaction. A reserved identifier is not a running task or public run link.

`CreateManagedRunsBound` freezes workflows before its before hook. Prepare the
immutable candidate outside this transaction, re-read and compare its complete
source inside the current-authority before hook, then insert the task plan and
bind/commit atomically. Do not mutate the precompiled workflow in a hook, nest
Engine transactions, or call Create and Start as two background HTTP operations.

Start/resume store the exact actual browser/API credential reference and subject.
The controller may restore only that persisted original reference from current
credential rows and current plan activation, retaining the original kind/scope;
every dispatch reauthorizes with database time, expiry/revocation, current account,
organization/project membership and role, route/grant and all source bases.
Do not synthesize a browser from a Principal, repurpose generic automation proof,
or broaden unrelated command purposes. Keep any restored Actor private to this
domain; no public token/hash endpoint. Credential expiry/revocation blocks new
dispatch until explicit resume by a current authorized browser/API actor.

Factual run/review projection is independent of dispatch authority: later logout
or template changes cannot erase received facts. System projection has no power
to create a run, accept a task, change an owner or grant access.

## Shared public contract

Keep current create/get/start endpoints and role bindings; extend the current
`ExpertTeamPlan` / item read model rather than adding a parallel type system.
The owning domain agent freezes exact exported DTO names/fields before UI writes.
The model must include source revision, graph digest, activation generation,
actual dependency IDs/facts, item state/blockers, actual dispatch/run references,
count-based progress and current allowed actions. Preserve technical facts under
folded disclosures. Historical revision reads expose no current actions.

Add typed command and immutable revision endpoints:

- `POST /expert-team-plans/{id}/commands`: exact `project_id`,
  `expected_version`, `action` (`pause`, `resume`, `stop`, `revise`),
  `reason`, explicit acknowledgement, stable `idempotency_key`; revision inputs
  explicitly identify only unstarted changes while preserving the started set.
- `GET /expert-team-plans/{id}/revisions?project_id&limit&before`
- `GET /expert-team-plans/{id}/revisions/{revision}?project_id`
- `GET /expert-team-plans?project_id&limit&before` for visible project history.

Strict input/duplicate JSON/exact numeric and current account/org checks match
existing typed APIs. API project writers may prepare/start/manage; publication of
expert configurations and formal task acceptance retain their browser gates.
All writes recover the original operation before creating a new effect, while
still checking current authority. Commands affect the original plan only.

`Service.TickExpertTeamPlans(ctx, engine)` scans a bounded persistent due order;
one plan cannot starve others. Limit per-round plans and newly created runs.
No-op checks advance only controller timestamps, not business versions/history.
Every genuine transition uses expected version/generation, stable run/operation
identifiers and same-transaction binding. Multiple controllers must be safe.
Run it in a maintenance-aware bounded background worker, independently from the
DAG lease/ready tick, without network/model/process work in SQL transactions.

## UI and actual verification

Enhance the existing team dispatch dialog/list entry. Show a topological list
with dependency chips, frozen role and explicit per-task readiness; after start,
use a compact progress summary and actual tasks/runs. Pause/resume/stop/revise have
precise labels. Stop explicitly says active runs continue until separately
stopped. Show source changes, approval waits and unknown states as actionable
facts. Defaults stay simple; source/hash/history fold away.

Use the existing extensible CN/EN domain catalogs, explicit accessible labels,
stable project/plan URL, scope-isolated original-request recovery, focus handling
and literal user/tool/source content. Only the opened active plan may refresh on
a bounded timer; no browser dispatch loop or whole-project polling.

Verify affected graph/authority/CAS/rollback/replay, predecessor success without
acceptance, wrong accepted run, source drift, pause/resume/stop, lost start ACK,
concurrent tick and fairness. Reuse mature Core/Runner/Git evidence where inputs
did not change. Perform one actual small dependent-team UI journey with existing
owned protocol fixtures; no paid model or duplicated full suites. Preserve the
028 primary preview/data until the final reviewed artifact pair is ready.
