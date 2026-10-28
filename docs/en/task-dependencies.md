# Edit task dependencies

[中文](../development/task-dependencies.md) · [API and consistency contract](../../contracts/task-dependencies.md) · [Reference integration record](../integrations/referenced-conversation-integration.md)

Open [Delivery and quality](task-delivery-chain.md) for a selected task, or mark tasks in a complete graph to enter the existing team preview. Dependency edits, actual execution, independent tests and formal acceptance remain separate actions.

Version: 1.0.0. Delivery task: T-INTEGRATION-111. Business dependencies describe delivery order and predecessor acceptance. Editing them does not start execution; use the existing workflow, development tool, independent test and review actions for delivery.

## View and edit one draft

Open **Development → Tasks → Task dependencies** in the current project. The dependency view uses `view=development&sub=tasks&layout=dependencies`. Select a task in the graph or table, then add or remove predecessors. An arrow runs from a predecessor to its successor: if B depends on A, A's delivery precedes B.

The graph and table share the same draft. Edits remain local until you save; discarding edits leaves the server unchanged. Names, states and dependencies come from actual project records. Editing does not create sample tasks or simulated progress.

The editor reads a complete graph of up to 1000 tasks. Larger projects show the actual total and the limit, with a route back to the ordinary task list. They do not expose an editable partial graph. One save can change up to 100 tasks.

## Confirm the current result after saving

Each changed task carries its original version. The server validates the full current project graph and rejects cycles, self references, duplicate predecessors and cross-project dependencies. Keeping the same predecessor set, including merely changing its order, does not count as an edit.

A committed change advances each modified task's version by one, sets it to In progress, removes its previous acceptance eligibility and invalidates accepted dependent results under the existing task rules. Original executions, tests and reviews remain traceable. Use the normal delivery process to establish that the current version meets its acceptance criteria.

The operation acknowledgement is followed by a separate read of the current graph. If another edit has already occurred, review the latest state. If this read fails, the editor keeps the draft and the fact that the original operation was confirmed. An old acknowledgement is not evidence of the current graph; do not generate a new operation for an uncertain save.

## Concurrent changes and recovery

A version conflict keeps your draft. Reading the latest graph is a comparison step and does not replace your edits. Explicitly choose **Use latest versions and keep local changes** to apply only your dirty task relationships to the latest graph, preserving other people's changes elsewhere. Alternatively, choose **Use latest graph and discard draft**. Review the impact before saving again.

An uncertain network result locks the original fields while the same account, project, request body and operation key recover the original operation. After confirmation, read the graph again. Switching accounts, organizations or projects cannot transfer this pending operation to the new context. Revoked access must be resolved before further handling.

Leaving the page, switching projects or opening task details protects unsaved edits. A table provides the same dependency editing actions as the graph, including on narrow screens and with a keyboard. Interface labels and server diagnostics are available in Chinese and English. User-authored titles and content keep their original language.

## Connect to the delivery workflow

Changing business dependencies does not change an execution DAG, merge code, approve prior tests or start an automated fix. Use [task handoff](task-handoff.md) for responsibility changes and [team execution](team-execution.md) for actual execution progress. Formal acceptance requires current-version execution evidence. Reconcile unknown side effects through their original operation before considering another execution.
