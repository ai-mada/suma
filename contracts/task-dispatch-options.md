# Current tool recommendations / 当前工具推荐

Status: T-P4-034 / 1.0.0 implementation contract. This is part of the complete platform scope, not a claim that intelligent model routing or the complete IDE is delivered.

## Observable behavior

After selecting a fixed workflow version and its development Agent node, a task can read current compatible tools. Selecting a recommendation fills the existing tool selector only. The existing explicit preview freezes task/design/baseline/knowledge, tool version/hash, workflow and route; Start reauthorizes and dispatches. Reading or adopting a recommendation never creates a plan/run, consumes model tokens, reserves budget, enables a tool or changes a model.

先选择固定流程与开发节点，再读取当前可用工具。采用推荐只填写现有工具选择；准备执行预览和启动仍是独立明确操作。推荐不创造权限，不改变模型、配额或执行状态。

## Read contract

`GET /tasks/{id}/dispatch-options`

Required query: `project_id`, `expected_task_version`, `workflow_id`, `workflow_version`, `agent_node_id`.
Optional: `limit` (canonical decimal 1..200, default50), `before` (original actor/project/task/workflow/node/search-bound cursor, maximum4096 bytes), and query (up to 120 UTF-8 bytes, with nonempty trimmed text). Exact alternative: tool_profile_id selects one tool in the same project and cannot be combined with limit, before, or query. Unknown/repeated/empty query parameters, noncanonical versions or extra scope are rejected. Task and workflow versions must still match.

`TaskDispatchOptionsRequest`: those fields plus server-selected `task_id`.

`TaskDispatchOptions`:

- project_id, task_id, task_version, workflow_id, workflow_version, agent_node_id;
- optional query (the submitted spelling when a text filter was applied), observed_at, context (`TaskContext`), execution_route (`ProjectExecutionRoute`);
- items (`TaskDispatchToolOption[]`), total, next_cursor, recommended_tool_profile_id.

`TaskDispatchToolOption`:

- tool: current `ToolProfile`, with its immutable profile hash/version and real observation;
- eligible: current observation/workflow/route compatibility, never an execution grant;
- reason_code: `eligible | tool_unavailable | workflow_incompatible | route_incompatible`;
- reason and optional reason_diagnostic: current raw diagnostic and existing typed display hint;
- model_state: `fixed_by_tool`; cost_state: `unknown`;
- eligible_basis: `current_observation_and_compatibility`.

Read the complete selected task context, including protected design and code baseline. Additional knowledge references are frozen/rechecked in the existing explicit preview. Tool availability must reuse authenticated real inventory, profile activation and current private/Pool execution identity; workflow compatibility must reuse the actual dispatch builder and DAG compiler. Declared capability is insufficient.

The current project read accepts a browser session or an API credential bound to that project. Responses use `Cache-Control: no-store`. All returned records use current organization/project authorization. The read must not invoke quota reservation or attempt accounting, insert dispatch rows, or alter any workflow/template. Errors in a candidate can exclude that candidate; database/authority failures must not be silently converted into a healthy recommendation.

## Filtered candidates

Search uses the authorized project tool directory's case-insensitive substring match on configuration name, command ID, worker ID, adapter, or protocol. It filters before pagination, live observations, and compatibility checks. Total counts matched configurations in the active directory, including paused tools. The first eligible tool in the returned page remains page-local. Paging and retry use the canonical trimmed, case-folded search bound to the original cursor. The response echoes the original spelling for client scope checks.

## Exact tool handoff

Exact mode uses the same authorized task, published workflow, project route, current tool observation and compiler within one read transaction. It returns the existing envelope with one item, total 1 and no continuation cursor. A missing or out-of-scope tool returns 404. A paused or revoked tool can be returned for diagnosis. The browser checks the exact ID, version, worker and profile digest against the carried tool and only adopts an eligible match. A newer profile or another route never substitutes for the selected tool.

## Ranking and model boundary

Stable ID order and the first eligible item **on the current page** are the complete first policy. Show page scope and next_cursor; do not imply a global optimum. No fabricated score, quality, latency, free slot or zero cost. No natural-language task keyword heuristic pretending to be expert selection.

Current writable CLI/native profiles do not provide independently governed model capability/price metadata. Therefore model selection stays fixed by the tool, cost remains unknown, and model parameters cannot be edited under the same profile hash. Future model-capability, pricing/rate and quality policy needs versioned inputs and a separate reviewed execution contract.

## UI and CLI

Use a compact optional recommendation section in the existing task dispatch dialog, paired zh-CN/en-US labels, loading/error/empty/no-permission states and explicit pagination. Validate returned task/workflow/node/version scope before rendering. Clear candidate choice when its input scope changes; keep pending writes and their original caller/path/body/key intact. Adoption never submits a write.

Page navigation preserves the server's exact continuation token in memory for the active task and workflow basis. Refresh and retry read the requested page. Previous and First issue new authorized reads using the original cursor or the empty first-page boundary. Loading and failed reads remove the page's selectable options until that page is confirmed. The current cursor and any visited cursor in its forward chain cannot be offered as a next page.

Each compatible row offers an explicit selection action. The first compatible row keeps the page recommendation label. Incompatible rows remain available with their current reason in a collapsed current-page group. In the existing handoff, an explicit recovery action can open the same compatible options panel without changing the selected task, workflow or node. A choice from any page supplies the exact profile ID and version to the existing dispatch selector. The manual inventory selector identifies inventory readiness and points to the compatibility check. Both direct and discussion reads verify that the returned execution route belongs to the currently selected organization.

The direct preview receipt must match the original project, task and task version, workflow and workflow version, Agent node, tool profile and tool version before acknowledging the pending write. Protocol-specific bindings are checked afterwards. A receipt with missing or mismatched identity keeps the original request and operation ID for recovery. Start remains an explicit, separately authorized operation.

The preview receipt also validates the frozen task context, ordered knowledge pins, selected Agent binding and plan state. Reopening a plan uses the current authorized read for the exact route plan and task. A failed read hides the previous facts and Start. A successful retry requires fresh confirmation. Historical rows without a public route may be read, with Start disabled. Start receipt recovery checks the organization and reserved managed run against the original plan after its transition to started.

兼容项均可直接选用，当前页首个兼容项保留推荐标识。原预览回执需匹配项目、任务、流程、开发节点和工具版本。回执不匹配时保留原请求与操作标识，供用户核对恢复。

CLI: `suma --project PROJECT tasks dispatch-options TASK --expected-version N --workflow FLOW --workflow-version N --node NODE [--limit N] [--before CURSOR]`. It uses the same current read contract and does not start work.

## Verification and ownership

Domain agent owns the new service/types and necessary pure-builder extraction. Root owns HTTP/CLI/schema/Go diagnostics. UI agent owns TaskDispatchDialog and dedicated recommendation components/catalogs. IDE save-receipt integrity is independent; it shares no public source file with recommendations.

Verify current authority, stale observations/versions, cross-scope cursors, unsupported tools and workflows, private/Pool route mismatch, immutable templates and zero new plans/runs/usage writes. Use a real public HTTP seam and response-shaped CNEN component checks. Reuse unchanged Runner/SDK/merge/test/acceptance evidence; do not replay model or formal tests to prove a read-only selector.
