# Current tool recommendations / 当前工具推荐

Status: T-P4-034 / 1.0.0 implementation contract. This is part of the complete platform scope, not a claim that intelligent model routing or the complete IDE is delivered.

## Observable behavior

After selecting a fixed workflow version and its development Agent node, a task can read current compatible tools. Selecting a recommendation fills the existing tool selector only. The existing explicit preview freezes task/design/baseline/knowledge, tool version/hash, workflow and route; Start reauthorizes and dispatches. Reading or adopting a recommendation never creates a plan/run, consumes model tokens, reserves budget, enables a tool or changes a model.

先选择固定流程与开发节点，再读取当前可用工具。采用推荐只填写现有工具选择；准备执行预览和启动仍是独立明确操作。推荐不创造权限，不改变模型、配额或执行状态。

## Read contract

`GET /tasks/{id}/dispatch-options`

Required query: `project_id`, `expected_task_version`, `workflow_id`, `workflow_version`, `agent_node_id`.
Optional: `limit` (canonical decimal 1..200, default50), `before` (original actor/project/task/workflow/node-bound cursor, maximum4096 bytes). Unknown/repeated/empty query parameters, noncanonical versions or extra scope are rejected. Task and workflow versions must still match.

`TaskDispatchOptionsRequest`: those fields plus server-selected `task_id`.

`TaskDispatchOptions`:

- project_id, task_id, task_version, workflow_id, workflow_version, agent_node_id;
- observed_at, context (`TaskContext`), execution_route (`ProjectExecutionRoute`);
- items (`TaskDispatchToolOption[]`), total, next_cursor, recommended_tool_profile_id.

`TaskDispatchToolOption`:

- tool: current `ToolProfile`, with its immutable profile hash/version and real observation;
- eligible: current observation/workflow/route compatibility, never an execution grant;
- reason_code: `eligible | tool_unavailable | workflow_incompatible | route_incompatible`;
- reason and optional reason_diagnostic: current raw diagnostic and existing typed display hint;
- model_state: `fixed_by_tool`; cost_state: `unknown`;
- eligible_basis: `current_observation_and_compatibility`.

Read the complete selected task context, including protected design and code baseline. Additional knowledge references are frozen/rechecked in the existing explicit preview. Tool availability must reuse authenticated real inventory, profile activation and current private/Pool execution identity; workflow compatibility must reuse the actual dispatch builder and DAG compiler. Declared capability is insufficient.

All returned records use current organization/project authorization. The read must not invoke quota reservation or attempt accounting, insert dispatch rows, or alter any workflow/template. Errors in a candidate can exclude that candidate; database/authority failures must not be silently converted into a healthy recommendation.

## Ranking and model boundary

Stable ID order and the first eligible item **on the current page** are the complete first policy. Show page scope and next_cursor; do not imply a global optimum. No fabricated score, quality, latency, free slot or zero cost. No natural-language task keyword heuristic pretending to be expert selection.

Current writable CLI/native profiles do not provide independently governed model capability/price metadata. Therefore model selection stays fixed by the tool, cost remains unknown, and model parameters cannot be edited under the same profile hash. Future model-capability, pricing/rate and quality policy needs versioned inputs and a separate reviewed execution contract.

## UI and CLI

Use a compact optional recommendation section in the existing task dispatch dialog, paired zh-CN/en-US labels, loading/error/empty/no-permission states and explicit pagination. Validate returned task/workflow/node/version scope before rendering. Clear candidate choice when its input scope changes; keep pending writes and their original caller/path/body/key intact. Adoption never submits a write.

CLI: `suma --project PROJECT tasks dispatch-options TASK --expected-version N --workflow FLOW --workflow-version N --node NODE [--limit N] [--before CURSOR]`. It uses the same current read contract and does not start work.

## Verification and ownership

Domain agent owns the new service/types and necessary pure-builder extraction. Root owns HTTP/CLI/schema/Go diagnostics. UI agent owns TaskDispatchDialog and dedicated recommendation components/catalogs. IDE save-receipt integrity is independent; it shares no public source file with recommendations.

Verify current authority, stale observations/versions, cross-scope cursors, unsupported tools and workflows, private/Pool route mismatch, immutable templates and zero new plans/runs/usage writes. Use a real public HTTP seam and response-shaped CNEN component checks. Reuse unchanged Runner/SDK/merge/test/acceptance evidence; do not replay model or formal tests to prove a read-only selector.
