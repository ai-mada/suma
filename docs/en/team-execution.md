# Review team execution and delivery progress

[中文](../development/team-execution.md) · [Workbench contract](../../contracts/team-execution-workbench.md) · [Team orchestration and recovery](../../contracts/expert-team-orchestration.md)

In the current project, open Development → Execution history → Team execution. You can browse the project's expert-team plans without first selecting tasks. The page reuses existing plans and their details. Opening, refreshing or paging the list does not start work.

## Open the plan that needs attention

Each plan shows its team, current state and original task-progress counts. Waiting, executing, awaiting review, accepted, blocked and stopped remain separate. Open a plan with blockers or work awaiting review to inspect its tasks, dependencies, actual runs and existing actions.

Use **Needs attention** for blocked plans or plans with a changed basis, and **Awaiting review** for plans with a task awaiting human review. Both filters apply to loaded plans. The team action on a plan row reads that team's exact results, and the team filter can be cleared independently.

| Fact shown | Meaning |
| --- | --- |
| Preview / ready to start | The plan and its execution basis are prepared; no task or model execution is implied. Starting still needs explicit confirmation in the detail view. |
| Waiting for dependencies or blocked | Inspect the stated reason, such as predecessor acceptance, changed source or authorization. Refreshing cannot bypass a gate. |
| Run succeeded / awaiting review | A result exists. Formal acceptance still depends on tests bound to the fixed version and an authorized decision. |
| Paused / stopped | Pausing prevents new dispatch. Stopping a plan does not establish that its running processes exited. Inspect the original run's stop receipt or reconciliation action. |

Start, pause, resume, stop, revision, testing and acceptance retain their existing conditions. Recover an uncertain command using its original operation. Creating another plan or starting again does not reconcile the original result.

## Return to the team plan from a run

Expand **Team assignment** in the execution detail for a Runner or WorkBuddy run. The section shows the team, assigned role and current plan revision when a binding is confirmed. **Open team plan** opens the original plan so you can review related tasks, dependencies and acceptance. Browser Back returns to the run. An unbound run, a denied read and a temporary read failure have distinct states.

Use `suma --project PROJECT team-plan list --run RUN_ID` for the same lookup in the terminal. Add `--team TEAM_ID` to require a matching team. The lookup returns at most one item.

## Total and loaded scope

The list reads at most 10 plans per request. The server total is the visible plan count for the current scope at the last successful read. The loaded count is the number of distinct plans the page currently holds. Load the next page only when needed. Per-plan progress is not a project-wide or organization-wide aggregate, machine health, available slots or proven multi-host capacity.

After a temporary read failure, displayed records may be the previous snapshot. Use the read-retry action and confirm the state before acting. Expired permission or a change of account, organization or project does not carry old records or cursors into the new scope.

## Return and share

Opening a plan retains the view and plan ID in the address. Refreshing or sharing the link performs current authorization and reads that plan again. Closing details returns to the same team-execution list. Refreshing the list reads it again; retaining every loaded page or scroll position is not guaranteed.

The current address uses `view=development&sub=runs&layout=team&panel=expert-team:<planID>`, with the original `project` parameter. A URL grants no project or team permission. An unreadable plan is not silently replaced by another plan.

The shareable team filter adds `execution_team=<teamID>`. In the terminal, use `suma --project PROJECT team-plan list --team TEAM_ID --limit 20`. Keep the same team and pass the original `--before` cursor to continue.

This guide covers the current 1.0.0 team-execution entry. Physical multi-host operation, external Multica service interoperability, complete vendor CLI support, the full IDE and production capacity require separate implementation and acceptance.
