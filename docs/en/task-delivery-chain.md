# Follow delivery from the task dependency view

[中文](../development/task-delivery-chain.md) · [Exact contract — Chinese original](../../contracts/task-delivery-chain.md) · [Edit dependencies](task-dependencies.md)

Version: 1.0.0. Delivery task: T-DELIVERY-112. Select a task in **Development → Tasks → Task dependencies**, then expand **Delivery and quality** in its side panel. Reads do not dispatch tools, run tests, submit reviews or create issues.

Check the task version and execution basis first. A current basis means the bound run matches the current task inputs; it does not prove successful delivery. Changed inputs retain their original run for inspection without presenting it as current acceptance. Missing or unknown execution is shown explicitly.

When **WorkBuddy code delivery pending verification** appears, select **Open original WorkBuddy attempt** to inspect the exact run, node and attempt behind that status. Opening the link is read-only.

The test section reports test-node execution in the exact bound run. Successful nodes are not test-case coverage or approval eligibility. Open a test to inspect its run, node and an existing attempt; an unstarted node has no valid attempt. Complete child-workflow test conclusions require their original run evidence.

Formal acceptance is shown separately. Open task review to recheck current authority, task and requirement versions, predecessor acceptance, independent tests and exact code evidence. Explicit rejection preserves the review, creates an issue with authoritative review lineage and invalidates current and accepted dependent delivery eligibility. It does not rewrite successful execution as a failed run.

Team plans, issues and returned requirements cover only the selected task's **current bound run**. Unstarted plans and other historical runs are excluded. Each list shows at most 20 references and declares its actual total. Issue origins are execution failure or review rejection; editable issue metadata cannot manufacture these links. A formally adopted requirement opens directly. Otherwise, open the issue and explicitly adopt it through the existing workflow.

If delivery and graph versions differ, compare or refresh the graph while protecting unsaved edits. Account, organization, project and task changes isolate old requests and facts. Revoked access hides references until a new authorized read succeeds.

For team development, mark 1–16 unaccepted tasks in a complete dependency graph. Save dependency edits and resolve conflicts before opening the existing team-dispatch flow. Select a published team configuration, per-task roles and models, review the fixed version preview, then explicitly start. Predecessor acceptance still controls downstream dispatch. Execution, independent tests, formal review and issue intake remain distinct actions.

The panel exposes compact metadata; logs, commands and complete evidence remain behind their existing authorized detail views. Current verification does not imply full-platform, distributed-capacity or production acceptance.
