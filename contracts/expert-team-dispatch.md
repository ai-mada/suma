# Project expert configurations and coordinated task dispatch

Stage: T-P4-027. Product: suma 1.0.0. Frozen integration contract, 2026-10-26.
This implements versioned project role bindings and atomic dispatch of independent
tasks. Full autonomous expert collaboration, cross-task synthesis, candidate review,
target integration and distributed acceptance remain in the platform plan.

## Domain and authority

- Reuse organization `Team` / `TeamMember`; do not duplicate people, resources or grants.
- One configuration per `(tenant, project, team)`, version 0 means unconfigured.
  Persist every saved/published/retired version immutably. Current states are
  `draft`, `published`, `retired`; retired is terminal. A saved change becomes a
  draft and invalidates old unstarted plans; publication is an explicit decision.
- Project writers save drafts; current project approvers publish/retire. Drafts, previews and dispatch accept current scoped project API writers as well as browser users.
  Publication/retirement require browser approval. Scoped API credentials may read. Team membership or
  an expert label never grants project, tool, machine or pool authority.
- 1–8 unique roles, each binds one active project `agent` or `tool` resource that
  is an active member of this team, one fixed published workflow/development
  node, an approved current tool profile and at most 8 reviewed knowledge refs.
  Human responsibility and formal review remain with existing task owners and
  approvers. An agent node ID is not a resource ID.
- Each role freezes resource/team/membership versions, workflow version,
  tool version/SHA and knowledge references. Reuse actual dispatch eligibility,
  supported structured adapters, isolated coding, independent candidate testing,
  design context and current route/grant checks; do not invent health.
- Configuration is an execution template, not a runtime credential. After start,
  a run keeps its fixed workflow and route; current Runner/profile/grant authority
  still applies. Later template edits affect new plans, not received facts.

## JSON types (Go exported names; snake_case wire names)

`ExpertTeamRoleRequest`: `key`, `title`, `resource_id`,
`expected_resource_version`, `workflow_id`, `workflow_version`, `agent_node_id`,
`tool_profile_id`, `expected_tool_version`, `knowledge` (ProjectKnowledgeReference[]).
Key is `[a-z][a-z0-9_-]{0,39}`; title 1–120 UTF-8 bytes. No arbitrary instructions,
commands, worker, routing, credential or approval may be supplied here.

`ExpertTeamRole`: `key`, `title`, `resource_id`, `resource_version`,
`resource_kind`, `resource_name`, `membership_id`, `membership_version`,
`workflow_id`, `workflow_version`, `agent_node_id`, `tool_profile_id`,
`tool_profile_version`, `tool_profile_sha256`, `tool_name`, `knowledge`.

`ExpertTeamConfiguration`: `project_id`, `team_id`, `team_name`, `team_version`,
`version`, `state`, `roles`, `created_at`, `updated_at`, `valid`, `blockers`
(string[]), `can_edit`, `can_publish`, `can_retire`.
Revision reads return the original snapshot with actions cleared; current
eligibility is not a rewrite of history.

`ExpertTeamSaveRequest`: `project_id`, `expected_version`, `roles`, `idempotency_key`.
`ExpertTeamCommandRequest`: `project_id`, `expected_version`, `action`
(`publish` | `retire`), `reason`, `idempotency_key`.
`ExpertTeamConfigurationPage`: `items`, `total`, `next_cursor`.

`ExpertTeamPlanItemRequest`: `task_id`, `expected_task_version`, `role_key`.
`ExpertTeamPlanCreateRequest`: `project_id`, `team_id`,
`expected_configuration_version`, `items`, `idempotency_key`.
`ExpertTeamPlanItem`: `role_key`, `role_title`, `resource_id`, `resource_name`,
`plan` (TaskDispatchPlan), `run_state` (empty before start; actual run state after).
`ExpertTeamPlan`: `id`, `project_id`, `team_id`, `team_name`, `version`, `state`
(`ready` | `started`), `configuration_version`, `configuration` (fixed snapshot),
`items`, `created_at`, `expires_at`, `valid`, `blockers`, `can_start`.
`ExpertTeamPlanStartRequest`: `project_id`, `expected_version`,
`acknowledge_dispatch` (must be true), `idempotency_key`.

## API and service boundary

- GET `/expert-teams?project_id&limit&before`: current project configurations,
  keyset pagination (default20, max100). GET `/expert-teams/{team_id}?project_id`
  returns current configuration; an authorized existing team may return version0.
- POST `/expert-teams/{team_id}`: full draft save.
- POST `/expert-teams/{team_id}/commands`: publish/retire.
- GET `/expert-teams/{team_id}/revisions/{version}?project_id`: immutable snapshot.
- POST `/expert-team-plans`: create a fixed preview, no run or task changes.
- GET `/expert-team-plans/{id}?project_id`: current-authorized original plan plus
  eligibility and actual per-item run states; supports refresh/share.
- POST `/expert-team-plans/{id}/start`: explicit atomic start, returns ExpertTeamPlan.

Service methods (context first): `MigrateExpertTeams`, `ListExpertTeams(actor,
project,limit,before)`, `GetExpertTeam(actor,project,team,revision int)` (0=current),
`SaveExpertTeam(engine,actor,project,team,request)`, `CommandExpertTeam(engine,
actor,project,team,request)`, `CreateExpertTeamPlan(engine,actor,project,request)`,
`GetExpertTeamPlan(engine,actor,project,id)`, `StartExpertTeamPlan(engine,actor,
project,id,request)`. Typed requests and current authorization are mandatory.

## Transaction and execution semantics

- A batch contains 1–16 distinct tasks in one project, with roles selected from
  one published configuration. Each task gets its own TaskDispatchPlan and run.
  Existing accepted predecessor rules still apply; selecting an unaccepted
  predecessor in the same batch does not bypass business acceptance.
- Preview creation uses a same-transaction internal task-plan builder (refactor
  existing implementation rather than a second generator). All preview rows,
  fixed route/role bases and operation receipt commit together. Failure rolls back.
- Start rechecks current credential/tenant/project authority, configuration/version,
  team/resource/membership eligibility, every task/requirement/design/knowledge,
  profile and route before creating work. Then all managed workflows, independent
  run roots, original task bindings, plan states and operation receipts commit in
  one database transaction. No network/process/model work occurs inside it.
- A ready batch-owned task plan cannot be independently started using the old
  single-task endpoint. No partial dispatch, browser start loop or new scheduler.
- Existing DAG queue/fairness/resources/usage own actual parallelism. Batch size
  is not a promise of simultaneous starts. Pending waits consume no machine slot.
- Retries/replays use stable operation IDs and current authority, returning the
  exact original runs. A late/ambiguous response must recover the original plan,
  never create another paid execution. Tests, acceptance, release and issue return
  retain existing per-task contracts.

## Core bulk interface (Core agent ownership)

`dag.ManagedRunRequest { RunID string; Workflow Workflow }` (JSON tags allowed).
`Engine.CreateManagedRunsBound(ctx,tenant,project,operationID string,
items []ManagedRunRequest,before func(*sql.Tx)error,
bind func(*sql.Tx,string)error,commit func(*sql.Tx,[]Run)error)([]Run,error)`.

1–16 unique reserved IDs; normalize/compile all workflows; total top-level node
count max10000. Required hooks. One `writeBefore`, one current before hook, per-run
bind, one final commit hook. Stable ordered request fingerprint and existing
operation table store original run IDs. Replay performs before authorization,
loads original runs and skips bind/commit. Independent roots must not inherit
`transaction.familyRoot` from another item. All failures, events, ready nodes,
routes and domain side effects roll back. Existing single-run API stays intact.

## UI and necessary verification

Team detail: collapsed project expert configuration; compact role editor with
resource, fixed workflow/node, tool and reviewed knowledge. Simple statuses;
technical hashes and route facts fold away. Save draft, review/publish, current
blocked reasons, conflict rebase preserving inputs and original-operation recovery.

Task list: selection limited to currently loaded real tasks (1–16); one dialog
selects a published team configuration and per-task role, shows fixed preview,
then explicitly starts it. Preserve per-task run links, current state, quality
and independent human acceptance. URL restores team/plan; no background all-project
poll. All controls and errors use existing extensible CN/EN catalogs.

Verify affected authority/version/role isolation, dependency rejection, transaction
rollback, concurrent/replayed start, direct-start denial, private/Pool run routes,
independent root families and original task/test gates. Add focused UI recovery
checks and representative actual CN/EN batch journey. No paid model call or
unrelated full suite; source/runtime/UI/production evidence remain distinct.
