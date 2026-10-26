# Task responsibility handoff

Status: T-P4-022 is implemented and verified locally. Open a task and expand **Task handoff**; the panel is collapsed by default and loads only when opened.

Choose an eligible project person, optionally bind their active team, and add a note. The recipient must be a project human resource linked to a real account with current project write access. Team membership must be active member or lead. Resources and accounts use different identifiers; the server resolves them and rechecks every command.

The designated recipient can accept or return a pending proposal; its sender can withdraw it. Acceptance explicitly changes responsibility, advances the task version, moves it to In progress, and invalidates previous execution/acceptance eligibility and dependent results. Original runs, tests and reviews remain historical evidence. It does not run tools or approve delivery. Confirm the impact before accepting, then use the normal fixed dispatch, independent test and review workflow.

Task, requirement, execution/delivery basis or recipient/team changes prevent acceptance of an old proposal. Active or unknown execution must be reconciled first. Return/withdraw can close a stale proposal. Unknown write results recover the original operation and body; retries never silently create a new proposal.

[CLI commands](../api/cli-task-handoff.md) use the same API and identity, with explicit version, operation and acceptance acknowledgement. [Verification and remaining scope](../quality/task-handoff-verification.md) distinguish real local UI/SQLite behavior from CLI request fixtures and future distributed/production acceptance. Autonomous expert/agent teams and the full platform remain in progress.
