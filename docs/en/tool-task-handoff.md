# Exact tool-to-task handoff

Version: 1.0.0. Stage: T-TOOL-TASK-HANDOFF-213. Status: implemented; final integration evidence belongs in the [quality report](../quality/tool-task-handoff-verification.md). Owners: workspace, task dispatch and integration. References: [contract](../../contracts/tool-task-handoff.md), [product](../product/tool-task-handoff.md), [architecture and AI SOP](../architecture/tool-task-handoff.md), [interaction design](../design/tool-task-handoff.md). The full 31 requirements and P0–P7 remain active.

## User flow

1. Open the original tool details and review the exact configuration. “Use for task” is available for an enabled, writable, task-protocol tool with a real available observation.
2. Choose a task using the existing authorized, bounded project list and search. Continue rereads the exact task and tool rather than trusting a list row.
3. Enter the original task-dispatch dialog, choose a published workflow and Agent node, then explicitly check and adopt the carried tool.
4. Configure model, parameters, budget and knowledge through the existing components. Create the formal preview, then separately confirm Start.
5. Continue through the existing execution, same-commit independent testing, human review, issue feedback and baseline handoff gates.

The picker and dispatch form use one Modal at a time. Closing returns to the exact original tool and its entry focus. A created formal plan uses the original refreshable task/plan route.

## Exact intent and current authority

The in-memory pin contains only `project_id`, `id`, `version`, `worker_id`, `profile_sha256` and `name`. It contains no model, budget, credential or full configuration and never enters the URL or browser storage. Adoption reads the exact tool through the current dispatch-options endpoint. The same task, workflow node and project execution route must accept that tool. The browser checks the returned identity, version, worker and profile digest against the pin. An exact tool outside the first directory page remains selectable.

Current actor, organization, project, API caller, task version, workflow version and Agent node scope all asynchronous results. Monotonic ownership rejects A→B→A callbacks. Task records follow the current `RecordItem` contract, including its legitimate empty values.

The parent tracks whether the handoff still owns the selected tool. Rechecking removes only the exact handoff-owned A and its model binding before reading again. Failure cannot leave that old A previewable. An explicit manual or candidate selection B keeps its ownership; an old A callback cannot replace or clear it. The adopted label follows the actual parent selection.

Tool observation, workflow incompatibility and route mismatch have separate messages before adoption. Preview and Start independently recheck authority, budget and the fixed execution context. An uncertain original write retains its path, body, operation key and caller; recovery takes precedence over handoff actions.

## Implementation and experience

`ToolWorkspace` owns the entry, single-dialog transition and return focus. `ToolTaskHandoffDialog` reuses the original task collection and exact reads. `TaskDispatchToolHandoff` provides an explicit read/adopt action; `TaskDispatchDialog` retains its existing profile scope, model binding and immutable plan flow. No new backend endpoint, table, scheduler or model configuration authority is added.

The UI uses a small progressive card, paired Chinese/English messages, 44-pixel action targets, native keyboard controls, visible focus and a single-column narrow layout. Loading, changed configuration, denied access, unknown reads and pending write recovery remain distinct states.

## Verification boundary

Focused checks cover off-page exact profiles, current selection ownership, manual changes, source ABA and the original pending preview. The representative real integration is scoped to HTTP/SQLite task selection and formal preview, with no run, Worker or model execution planned for this stage. Unchanged execution and quality-chain evidence is reused. Actual outcomes, screenshots and remaining platform scope are recorded in the final quality report; this page does not predeclare an integration pass.
