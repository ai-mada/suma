# Task handoff requests in the personal inbox

T-HANDOFF-INBOX-148, initial suma product 1.0.0. Status: implementation and focused checks; final Git delivery and GUI acceptance are not complete. Owners: task collaboration, notifications, frontend interaction and integration, divided by the [contract](../../contracts/task-handoff-inbox.md). This document describes behavior, not production acceptance.

See the [Chinese product scope](../product/task-handoff-inbox.md), [architecture](../architecture/task-handoff-inbox.md) and [interaction and AI SOP](../design/task-handoff-inbox.md). Notifications, responsibility, execution and formal Task Review remain separate facts.

## Requests and actual assignment

| Action | Recorded fact | Boundary |
| --- | --- | --- |
| Send a handoff | A pending request and personal request notification | No ownership transfer, run or delivery acceptance |
| Accept responsibility | The original recipient explicitly takes ownership; the task returns to in progress and emits a separate assignment event | This is not formal Task Review |
| Return or withdraw | A terminal request and decision reason | Ownership does not change; the request notification remains |
| Open a request | A current-authorized exact read | No automatic read mark, acceptance, dispatch or approval |

The original transaction recovers the original operation first. A new handoff validates its task version, basis, recipient and optional team, then commits the handoff, pending slot, request notification, activity and receipt together. Notification failure rolls back all writes, including its sequence change. Recovery does not duplicate the notification.

Acceptance retains the original actor, current write authority, exact basis and version checks. The recipient explicitly acknowledges ownership change and invalidation of verified delivery and downstream results. Prior runs, tests, reviews and history remain. The separate task_assigned event describes the actual assignment; it does not replace or mark the earlier request notification read.

## Source integrity and private binding

The notification uses the original handoff ID as its source_event_key, with its kind checked independently. Recipient user/resource and source task version come from the fixed handoff, not the task's current owner.

A private notification_id is stored in the existing handoff payload and retained through subsequent commands. It is absent from public TaskHandoff, public metadata and command receipts. This stage adds no DDL, backfill or second event authority. Old handoffs without a notification retain that history.

The inbox exposes source_available, an exact handoff_id and current task only when all of these agree:

- The readable task, notification and formal handoff belong to the same tenant, project and task.
- Row and payload agree on identity, version and pending/accepted/returned/withdrawn state.
- Recipient user/resource and original task version match the notification's fixed fields.
- The payload's reverse notification ID matches this exact notification.

Missing, corrupt or mismatched sources retain the private event and its read/archive lifecycle, but hide task hydration and navigation. Database failures return errors rather than pretending the source is unavailable. A request notification does not infer a current assignment.

## Exact navigation and reads

The inbox opens development/tasks with record=taskID and panel=task-handoff:handoffID, respecting existing unsaved-change protection. Normal task details keep the handoff area collapsed; an explicit locator opens it, reads the exact request and focuses its card.

GET /api/v1/task-handoffs/{id}?project_id=P&task_id=T requires exactly those two single, nonempty canonical ID parameters. It rejects unknown/duplicate queries, wrong-task or unauthorized access and machine identities. Row/payload consistency and current task scope are checked before the existing basis/action projection. It neither scans history or recipients nor falls back to latest.

The located request is independent of the bounded history page. Requests outside the latest 20 records require no repeated pagination and do not alter page counts. Terminal history remains readable under current authority without restoring old actions. GET never accepts responsibility or updates notification state.

## Drafts, versions and recovery

Proposal and decision fields are independent parts of one human/organization/project/task-scoped draft. Proposal fields contain the recipient, optional team, the user's note and task version. Decision fields contain the exact request, chosen action, reason and handoff version. Protected source history is not cached in the draft.

Language changes, failed reads, refreshes and locator changes preserve unsent input. Confirmed receipts clear only the matching draft portion; another portion or subsequently edited input survives. Cancelling a decision preserves the proposal.

Adopting the current task version and adopting the current handoff version are explicit actions. Refresh does not silently replace either basis. Acceptance acknowledgement is transient and resets after version adoption. Return and withdrawal require the original role and a reason; they do not make stale delivery current.

A decision requires a successful exact authorized read. If that read fails or is absent, an old history-page record cannot supply decision authority. Even after a successful exact read, current versions, allowed actions, basis and impact acknowledgement still gate submission. Numeric writes retain original number lexemes for strict decoding; fractional versions are not rounded into valid CAS values.

Unknown writes retain the original caller, scope, version, operation ID and JSON. Recovery uses that original command and receipt rather than changing recipients or adopting a new version. Competing edits remain locked until the original write is confirmed. After confirmation, a failed refresh is reported separately from a saved command and does not justify sending it again.

## Current caller and interaction

Changing the caller or identity/task scope rebuilds reads and isolates late responses. A current 401/403/404 hides protected facts and executable controls and clears acceptance acknowledgement. Old-caller responses cannot restore them. Reopen task details after access is restored; private user drafts remain separate from protected sources.

Loading, empty history, no eligible recipients, read errors, permission changes, stale basis, damaged drafts, storage failures and unknown writes have distinct feedback. Damaged local drafts preserve their original data for explicit discard; storage failure asks the user to keep the page open and blocks new submissions.

Chinese/English labels preserve original names, notes and basis values. Buttons, selectors and basis disclosures have at least 44px height. The located card has visible keyboard focus and scroll spacing for fixed headers. Long text wraps on narrow screens, with existing reduced-motion behavior. Actual keyboard, narrow-screen and touch behavior needs separate browser evidence.

## AI SOP, implementation and evidence

AI may explain exact evidence, responsibility impact and current blockers. It must not treat a notification as acceptance or submit a decision without explicit user authorization. The original recipient and impact acknowledgement cannot be inferred from historical ownership, unread state or a model's suggestion. Development, testing and formal Review continue through their original controls.

Implementation: [handoff commands](../../internal/platform/task_handoff.go), [private source and exact read](../../internal/platform/task_handoff_inbox.go), [notification projection](../../internal/platform/notification_inbox.go), [handoff panel](../../apps/web/src/TaskHandoffPanel.tsx) and [inbox](../../apps/web/src/NotificationInbox.tsx).

The [verification report](../quality/task-handoff-inbox-verification.md) and [machine summary](../quality/task-handoff-inbox-summary.json) own final metrics and distinguish focused checks, actual HTTP/CLI, GUI and Git delivery. This document makes no provider, physical multihost, other-database or production claim.

Full P0–P7, requirements 1–26 and 58 limited / 34 pending / 92 total remain active in the [implementation plan](../delivery/implementation-plan.md).
