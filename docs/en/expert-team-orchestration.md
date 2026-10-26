# Expert team dependency plans

Status: T-P4-029, suma 1.0.0, verified within a bounded local scope on 27 October 2026. The full platform remains in progress.

The existing team task dialog now supports a fixed dependency graph for 1–16 actual project tasks. Independent tasks are the all-root case of the same `ExpertTeamPlan` domain. The Go DAG remains the only execution dispatcher; the browser never loops over task creation or dispatch requests.

Select tasks from the loaded task list, choose a published expert configuration and assign a role to each task. Review the fixed task sources, dependencies, tools, knowledge and execution routes, then explicitly activate the plan. Preview creates no runs. Ready tasks can enter the existing queue; waiting successors occupy no execution slot.

A successor requires current formal acceptance of the exact predecessor run created by this plan. A successful run, green tests or an Agent summary does not accept a task. Open the original task for formal review. Progress distinguishes waiting dependencies, ready, queued, executing, awaiting review, accepted, blocked and stopped work; only actual dispatched runs receive run links.

- **Pause new dispatch** leaves existing runs active.
- **Resume with current authority** renews the activation using the actual current browser or scoped API credential. Every later dispatch rechecks authority and fixed source.
- **Stop unstarted work** is final for pending items. Existing runs retain their own stop and reconciliation controls.
- **Revise unstarted tasks** explicitly reads current task versions and published roles, creates an immutable source revision and preserves started source, role and run identities. A changed configuration requires explicitly including every unstarted item.

Keep the original operation key after an uncertain response. Read the latest version after a conflict, retain the draft and confirm again; do not create a replacement run. Restore the plan through its existing `expert-team:` URL. Source, hashes and revision history stay under disclosures; historical reads grant no current actions. The existing suma `team-plan` CLI uses the same scoped APIs and requires explicit confirmation for writes; it introduces no new identity or scheduling path.

The recorded local journey used a real Control process and two independent Runners on one Windows host, with owned Node CLI protocol fixtures. Two runs produced eight independent formal assertions and two explicit UI reviews. Pausing prevented successor dispatch even after root acceptance; a pending-only revision preserved the original root, and explicit resume completed revision R2. Representative Chinese/English, refresh, read-only history and conflict recovery were observed. No model or vendor calls were made.

This is business orchestration, not automatic Git candidate inheritance or merging. It does not establish real vendor CLI/model/account behavior, physical multi-host execution, production acceptance or the complete device/theme matrix. Governed repository integration remains a separate capability.

See the [frozen contract](../../contracts/expert-team-orchestration.md), [CLI contract](../../contracts/expert-team-orchestration-cli.md), [product scope — Chinese](../product/expert-team-orchestration.md), [architecture and operating SOP — Chinese](../architecture/expert-team-orchestration.md), [interaction design — Chinese](../design/expert-team-orchestration.md), [verification and remaining scope — Chinese](../quality/expert-team-orchestration-verification.md) and [compact verification summary](../quality/expert-team-orchestration-summary.json).
