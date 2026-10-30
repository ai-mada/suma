# Task review responsibility requests

T-REVIEW-142 adds an explicit responsible human for one exact task delivery. It continues the existing P3/P5 and Multica M27 scope. Implementation checks and final browser observations are recorded separately; this document does not claim production or complete platform acceptance.

Choose one current canonical human with organization owner/admin or project admin/reviewer authority. Creating a request requires current project write access. Self-requests are allowed. The selected person receives responsibility without receiving new permission; any other currently authorized reviewer keeps their existing right to accept or reject through the original task Review command.

A request fixes the task version, run ID/version, task content basis, requester and reviewer identities/names, and note. A succeeded exact run is required to locate the delivery, but does not establish full quality eligibility. The original Review still checks independent tests, code evidence and current task, requirement, dependency and other quality bases. DAG approval, ownership transfer, test success and formal acceptance remain separate facts.

Each task has at most one pending request. Changes in binding or reviewer eligibility remain visible without modifying history on read. A currently authorized requester or administrator may explicitly cancel the request and create another. Cancellation changes neither the task nor a formal decision. A matching real Review closes the exact request atomically and exposes the actual reviewer and original decision; rejection preserves the original issue feedback. Replaying an old Review cannot close a later request.

The private inbox notification is produced in the same transaction as the request and its operation receipt. Notification failure rolls everything back, including its sequence. Reading or opening a request does not mark a notification read. Cancelled and completed requests retain their private events; broken source relations hide navigation rather than inventing a task or decision.

## CLI

Put global project and captured identity flags before the command. Lists return one page only; continue explicitly using the original cursor.

```text
suma --project PROJECT review-requests list --task TASK --limit 20
suma --project PROJECT review-requests targets --task TASK --query "reviewer" --limit 20
suma --project PROJECT review-requests get REQUEST
suma --project PROJECT review-requests create --file request.json
suma --project PROJECT review-requests cancel REQUEST --file cancel.json
```

Create file:

```json
{
  "project_id": "PROJECT",
  "task_id": "TASK",
  "expected_task_version": 7,
  "run_id": "EXACT_RUN",
  "expected_run_version": 12,
  "reviewer_resource_id": "CANONICAL_HUMAN_RESOURCE",
  "note": "Please review this exact delivery against the original requirements.",
  "idempotency_key": "original-review-request-operation"
}
```

Cancel file:

```json
{
  "project_id": "PROJECT",
  "expected_version": 1,
  "action": "cancel",
  "reason": "This responsibility request is being explicitly withdrawn.",
  "idempotency_key": "original-cancellation-operation"
}
```

`--file -` reads stdin. Both writes send the original object without adding URL queries or an operation key. On an unconfirmed response, repeat the same original path, file and caller; browser commands retain the original `--expected-user` and `--expected-organization`. The CLI never refreshes the identity, substitutes new versions, retries automatically, starts execution or performs formal acceptance. The existing `inbox` commands manage private read/archive state.

The exact HTTP and immutable/current field definitions are in the [contract](../../contracts/task-review-requests.md). Backend, CLI and UI share these domain facts. The current Multica license has additional restrictions: only workflow semantics informed this independent implementation; no current restricted source or assets were copied and no dependency was added.

All original Review entrypoints first honor unsaved-content protection. An unsent responsibility draft remains available below the page; it cannot trap the user behind a modal that refuses to close. Clearing the draft restores the original Review flow without changing its authority or evidence requirements.

Editing an existing test plan follows its current server `allowed_actions`; creation uses the session's effective project role. Unloaded task requirements remain unknown and cannot be treated as an empty acceptance list. Saving the same plan refreshes it in place. A newly created plan requests navigation only after refresh while the original caller and selection still match; an uncertain write retains its original operation.
