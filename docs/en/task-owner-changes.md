# Bulk task owner changes

T-MULTICA-BULK-150, initial suma product 1.0.0. Status: contract frozen, implementation in progress. Owners: task domain, frontend interaction and integration, divided by the [contract](../../contracts/task-owner-changes.md). This page defines behavior rather than certifying tests, GUI or delivery acceptance.

The existing task list provides explicit selection of up to 16 tasks; ordinary updates own assignment notifications and dependency invalidation. This slice combines them into one protected command, keeping team arrangement, execution, independent testing and formal Review separate.

## Explicit user journey

1. Select 1–16 loaded tasks in the current project; a filter does not implicitly select all matching tasks.
2. Choose one current canonical human resource from the server picker, with a real user binding and project access. Literal search and explicit pages do not infer eligibility from a partial local member array.
3. Read the actual per-task preview: old owner, current version and whether the owner changes, plus real changed/unchanged counts.
4. Acknowledge the impact and explicitly adopt the same selection, target resource version and preview digest. The digest is neither a credential nor approval.
5. Inspect the original result and every selected task's final version/status. A failed refresh is separate from a saved command.

The caller needs current human browser/project-API write authority. A target with viewer access may own a task, but assignment grants no new permissions. Preview is read-only business behavior: it creates no receipt, activity, notification, task mutation or tool call.

## No-op owners and real impact

A changed task follows ordinary Update semantics: in_progress, cleared verified_run_id and the original assignment notification. A task already owned by the target has no empty owner write or assignment notification. If all owners match, the preview cannot be adopted.

An unchanged-owner task may still be a descendant of another selected task that changes. For example, if A precedes B and only A needs a new owner, B can lose delivery eligibility under the original dependency rules. owner_changed=false does not promise an unchanged final version or status. Results must re-read all selections after invalidation.

Accepted descendants may be invalidated and completed iterations/milestones may reopen. Show the true selected-item preview and a generic impact warning, not invented descendant counts or a frozen graph claim. Existing run, code, test and Review history remains; no dispatch or acceptance is triggered automatically.

## Atomic server contract

The current [ordinary update](../../internal/platform/records.go) supplies the existing semantics. The contract requires single and bulk commands to share its transaction core, not loop over a self-committing Service.Update or duplicate its implementation.

GET /api/v1/task-owner-targets reads current canonical human targets in this project. POST /api/v1/task-owner-changes/preview returns the exact target, per-item basis, change counts and preview_sha256. POST /api/v1/task-owner-changes accepts the closed typed selection, target version, digest, explicit impact acknowledgement and original operation ID.

Current authority and project coordination precede receipt recovery. Without a receipt, validate every selected task version, target identity/resource version and recalculated preview before any writes. Duplicate/cross-project tasks, extra fields and invalid exact integers fail; original number lexemes are not rounded into valid CAS versions.

Apply only actual owner changes through the shared core and original notifications. Call InvalidateDependentsTx(includeRoots=false) once for the changed roots after all selected versions were checked. This avoids invalidating a selected successor before its own version check and creating a false conflict. Re-read every selection before returning final versions/statuses. Any conflict, notification or persistence failure rolls back the whole transaction.

## Recovery and interaction states

The actual path is [Records(tasks)→TaskRecordsScope](../../apps/web/src/App.tsx)→[TaskViews.selectionActions](../../apps/web/src/TaskViews.tsx)→TaskOwnerChanges. The recovery component stays mounted; TaskCollection is embedded with controls=null, rather than the generic App actions slot. Zero selection disables a new preview, not original recovery or storage-error feedback. Scope follows user, organization and project.

Unknown results retain the original caller, JSON, versions, digest and operation ID. Recover the immutable original receipt under current authorization; the same ID with another body is rejected. Recovery cannot depend on the current page retaining selections or rebuild its body from new directory data. No new write starts while the original remains unconfirmed.

A 409 keeps selection and target for explicit re-preview rather than silently adopting new versions. Missing/loading/error states cannot enable submission from stale facts. Scope changes or 401/403/404 clear protected preview/target data and isolate late caller responses. A confirmed result followed by a failed refresh must not create a new adoption.

Parent 401/403/404 synchronously masks private data, aborts reads and retires late write callbacks; this does not undo a possible server effect. While blocked, the user may explicitly GET task-owner-targets with project_id and limit=1 using the original-view headers. Only success notifies TaskRecordsScope to clear denial and refresh reads. It selects no person and grants no role; the original write still requires explicit Restore, never automatic replay.

Chinese/English, keyboard navigation, 44px interaction targets and a 390px layout are design requirements. Preserve names, titles and versions, using existing Radix focus/navigation protection and design tokens. Selection, picker, preview, impact acknowledgement, conflict and recovery each need clear feedback beyond color; actual GUI evidence remains separate.

## CLI flow

```text
suma --project PROJECT task-owners targets --query "Name"
suma --project PROJECT task-owners preview --file preview.json
suma --project PROJECT task-owners apply --file original-apply.json
```

POST project scope is body-only, checked or filled from the global project. Preserve the original task array, order and exact versions; apply requires the exact preview_sha256, acknowledge_invalidation:true and original idempotency_key. Unknown outcomes keep that file and key unchanged.

## AI SOP, reference and evidence

AI may explain preview facts and general impact. It must not expand selection implicitly, treat the digest as authorization or submit without explicit user authorization. Reconcile an unknown original operation before a new decision; target/version changes require explicit re-preview.

The [recorded Multica priority map](../integrations/multica-priority-map.md) informs behavior. This slice adds no copied Multica source, styles or brand assets and does not claim direct source reuse. Full P0–P7, requirements 1–26 and 58 limited / 34 pending / 92 total remain unchanged.

[Product](../product/task-owner-changes.md) · [Architecture](../architecture/task-owner-changes.md) · [Interaction and AI SOP](../design/task-owner-changes.md). Future [verification](../quality/task-owner-changes-verification.md) and [summary](../quality/task-owner-changes-summary.json) are pending integration evidence, not asserted results.
