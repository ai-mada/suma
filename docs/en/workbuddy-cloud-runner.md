# Governed cloud task execution and recovery

Suma 1.0.0 · T-WORKBUDDY-RUNNER-155 · Scoped Windows/SQLite verification passed. [Contract](../../contracts/workbuddy-cloud-runner.md), [architecture](../architecture/workbuddy-cloud-runner.md), [interaction and AI SOP](../design/workbuddy-cloud-runner.md), [evidence and limits](../quality/workbuddy-cloud-runner-verification.md).

1. Open the original workflow node and explicitly delegate one creation using your own current connection.
2. A qualified private or Pool Runner claims the immutable assignment. A connection or installed tool alone never grants execution authority.
3. The Runner persists the exact request and operation before calling the central creator. Prepared or unknown submissions never authorize a second provider task.
4. Following a lost response or restart, inspect the original submission and retain the same identity. A later observation does not acknowledge an earlier uncertain refresh.
5. Provider completion waits for delivery evidence. It does not prove that code was delivered, independently tested or accepted.

When the Runner disconnects, the original attempt waits for reconciliation. Local process resources and Pool slots can be released; external-effect reservations remain. Cancelling observation cannot prove the remote task stopped. Only a trusted, atomic check of an absent submission and unconsumed delegation may establish that no cloud task was created; retry still requires the original policy and current authority.

The interface presents one primary status and next action, with request and execution facts in collapsible details. Chinese and English share the same persisted Run/Node facts; language does not change operation identity or authorization.

Machine ACP authorization, cloud candidate import, independent tests, human acceptance and real-provider/production verification remain separate deliveries within the complete platform plan.
