# Governed candidate composition and development baselines

Stage T-P4-031; suma 1.0.0. Implementation contract, 2026-10-27.
This extends the accepted full P0–P7 platform. Source, local protocol execution,
physical multi-host and production acceptance are recorded separately.

## Observable delivery

1. Select an explicit integration task, a fixed workflow version and producer
   node, and an ordered set of 2–16 distinct currently accepted task candidates.
   Resolve all original run/attempt/fence, sealed bytes, independent tests and
   current acceptance server-side. A branch name or supplied SHA is insufficient.
2. A read-only preview creates no run. Explicit start freezes the complete plan,
   route, original initiating credential, source order, administrator base,
   operation, Git strategy and commit time. Bind the integration task and managed
   DAG run in one transaction. The existing Go DAG owns dispatch.
3. A trusted Runner command producer imports the exact authorized bundles and
   uses Git's native three-way merge to compose them in its private attempt.
   Preserve parent history. A clean result is a new candidate, not an inherited
   test pass or acceptance. Source order and commit identity are fixed.
4. Existing `candidate_from` transfers this producer's exact candidate to an
   independent formal test node in the same run. It remains an immutable test
   contract. Task acceptance uses the existing explicit browser Review gate.
5. The accepted integration task can use the existing governed repository
   integration path. Its approved fast-forward result is the exact combined
   tested candidate. This stage does not weaken target CAS, fencing or receipts.
6. Explicitly bind the accepted composition to an unstarted development task.
   Freeze that binding in its next dispatch or in an explicit revision of an
   unstarted expert-plan item. The new workspace contains the combined code;
   it produces its own candidate, tests and acceptance facts.

Conflict is a real outcome with paths and source identities, never a clean
candidate with conflict markers. No `ours`/`theirs` shortcut, silent source drop,
automatic acceptance, target overwrite or guessed latest branch is permitted.
Conflict repair through the IDE/new fixed candidate retains its own reviewed
source and tests; a conflict report alone does not implement automatic repair.

## Domain and execution boundaries

Composition is a code-producing task execution, distinct from target repository
mutation. Reuse `RepositoryIntegrationBasis` to prove each accepted source and
`CandidateBinding` to identify sealed code. Keep `RepositoryIntegrationPlan` and
the administrator-managed target provider's existing responsibility unchanged.

- Domain agent A owns `internal/platform/candidate_composition*.go` except Root's
  HTTP and transport integration tests: DTOs, migration, source/current authority,
  preview/start/read, immutable execution binding and protected task baselines.
- Runtime agent C owns `internal/candidatecompositioncontract`, Runner command,
  strict multi-source/baseline import, workspace provenance and focused checks.
  It owns Runner wire/client changes. Root owns DAG and Control shared wires.
- UI agent owns the composition panel, model/hooks, CN/EN domain catalogs and
  product/architecture/design documentation. Reuse the current compact detail
  layout and progressive disclosures. No separate top-level navigation.
- Root owns HTTP, private/Pool assignment and result seams, task/expert dispatch
  baseline plumbing, generated contracts, controlled localization and integration.

New public DTOs use the `CandidateComposition` prefix; the protected baseline
uses the shared `CodeBaselineBinding`. Exact exported field names are frozen in
the owning Go type files before UI and API generation. JSON is snake_case.

## Domain semantics

A composition plan binds project, integration task/version, workflow/version,
producer node, ordered source references and authoritative source snapshots,
source digest, administrator base and fixed commit time, actual run and current
actions. Preview has an expected version, expiry and stable idempotency key.
Start requires explicit acknowledgement and revalidates all current authority,
sources, task, workflow and route before atomic managed-run/task binding.

A root static command or agent can be selected as the producer; test, conditional,
non-root or incompatible capability nodes remain ineligible.
Only the chosen producer is replaced by the internal `suma-compose` command;
the fixed independent quality nodes remain. Reject unsupported template shapes
instead of silently dropping nodes or weakening their gates. No paid model is
required to perform a deterministic Git composition.

Projection separates planned/queued/executing/failed/reconciling/awaiting review
and accepted. Success of the compose command or formal tests is not business
acceptance. Failed/unknown attempts retain their original run recovery actions;
there is no automatic new run or unknown-effect reexecution.

Task development baselines live in a protected domain table, not editable record
data. Binding requires a current accepted composition and an unstarted target
task, expected task/binding versions and an explicit stable operation. Increment
the task version to invalidate old previews. Replacing a pending baseline is an
explicit reviewed change. Never rewrite a started run, historical plan or source.

The downstream dispatch freezes the complete `CodeBaselineBinding` alongside
the task source. Expert-plan pending revisions use the same binding; they may
not silently select a newly available composition or latest target head.
The baseline's task/version, source acceptance and complete lineage are checked
again on actual dispatch and fresh code access.

## Shared Runner protocol

Protocol `suma.candidate-composition.v1`:

- `CandidateCompositionExecution`: protocol, plan_id, plan_version, project_id,
  base_commit, ordered sources (`CandidateBinding`), sources_sha256,
  fixed_timestamp and operation_id.
- `CodeBaselineBinding`: protocol, id, project_id, task_id, task_version,
  composition_id, composition_version, accepted_basis_sha256, source
  (`CandidateBinding`) and binding_sha256. It is a development input, not a
  replacement for scheduler candidate-test fields.
- `CodeInputRequest`: project_id, actual `CandidateAssignmentRef`, input_kind
  (`composition` or `code_baseline`), binding_id and transfer_id.

The authenticated assignment carries optional `candidate_composition` or
`code_baseline` fields. A caller-supplied workflow config or record data never
grants either capability. Control resolves them only from the durable exact
run/node binding; mixed or stale input modes are rejected.

POST `/runner/code-inputs/bundle` and `/pool-runner/code-inputs/bundle` return
the exact binary bundle after current authorization. The entire Pool machine,
slot, grant, credential, dispatch, attempt and fence scope is retained. A machine
does not become a browser or gain source access from knowing a transfer ID.

Every fresh code read checks the live consumer lease and frozen input membership,
current initiating delegation, source candidate and current accepted task basis.
Download only frozen sources; do not broaden the old same-run transfer resolver.
Keep external IO outside SQL transactions and recheck authorization around blob
reads. Received genuine results remain facts after later source drift; drift
prevents new approval, baseline adoption or fresh code authorization.

## Git and byte provenance

Use mature Git `merge-tree --write-tree` and `commit-tree`, not a handwritten
patch merger. The installed executable/version and hermetic execution policy
are part of the actual validation evidence. Disable hooks, signing, inherited
config, external attributes, custom merge drivers, filters, textconv and network
helpers. A clean isolated object repository or an explicitly validated config
allowlist is required; clearing global config alone does not disable local drivers.

Validate each sealed source's digest/size/header/pack and permitted base lineage.
`CandidateBinding.BaseCommit` records the selected workspace base. Its optional
`bundle_base_commit` records the full bundle prerequisite; when absent the two
are identical. A durable development binding is required when they differ.
Keep the administrator anchor B distinct from selected development input M.
The composition output bundle contains the complete object closure B..result,
so a different machine with only B can import all combined ancestry.
An inherited development workspace starts at M, and reports M as selected base
while preserving B as administrator base and its exact baseline digest.
Its next bundle must remain importable from B, with inherited provenance explicit.

Persist intent before effects. Stable parent order, author, committer, timestamp
and message ensure one operation cannot silently produce another commit.
Seal bytes once; retry upload/acknowledgement using the same manifest and Journal.
Cancellation, lost receipts or process uncertainty use the original attempt's
reconciliation, not a guessed new composition. Bound sources (16), each source
bundle (64 MiB), output bundle (64 MiB), command duration and output explicitly.

## Public interfaces and UI

The domain exposes options, preview/create, get/list, explicit start and
read-only current progress, plus protected target-task baseline read/bind.
All writes carry expected versions and stable operation keys. Strict duplicate
JSON, exact integers, single project scope and existing current identity rules
apply. Human acceptance and repository mutation retain existing gates.

The panel shows current source tasks and fixed code revisions, preview and start,
actual composition/testing/review states, original conflict/unknown facts, then
the existing target integration entry and explicit downstream baseline binding.
Technical hashes and original provenance are folded. Keep editable drafts on
409, recover only the original operation, abort stale project requests, preserve
selection on refresh and render loaded machine diagnostics in CN/EN without
translating source code, human content or immutable evidence.

## Necessary verification

- Real divergent A/B commits with a shared B compose into M containing both;
  a separate Runner tests exactly M; existing acceptance and target CAS operate
  on M. A downstream C starts with both changes and preserves baseline proof.
- Conflict emits accurate source/path facts, no candidate seal or target write.
- Current source/task drift, expiry/revocation, wrong project/Pool/attempt/fence,
  arbitrary input IDs, stale baseline and record-data spoofing are rejected.
- Lost upload/ack retains one sealed candidate; failed/unknown results are not
  converted to acceptance or rerun automatically. Immutable facts remain visible.
- Final affected Go/TS checks, generated schema consistency and representative
  CN/EN UI readback. Reuse unchanged evidence; no broad suite/model repetition.

Git mechanism references: [merge-tree](https://git-scm.com/docs/git-merge-tree),
[commit-tree](https://git-scm.com/docs/git-commit-tree). Vendor accounts, remote
physical hosts, full database/capacity/device matrices and production acceptance
remain separately recorded requirements of the full platform.
