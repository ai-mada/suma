# OpenCode execution contract — suma 1.0.0

Status: current single implementation. Owners: execution, control, workbench.
Scope: fixed administrator-approved model for task/expert development. The full platform remains tracked in [the implementation plan](../docs/delivery/implementation-plan.md).

## One execution path

`opencode / opencode-sdk-v1 / opencode-sdk-jsonl` is the only OpenCode execution pair. There is no JSONL CLI compatibility parser, input alias, old configuration translation or automatic retry. Official SDK/plugin and server are pinned to **1.19.4**, commit `148842ef7cb1bc34192b3fff57d0f9721ab9c801`. Vendor protocol versions are independent of product 1.0.0.

Local `CommandSpec.opencode` contains absolute executable path, provider_id, model, base_url and optional API-key environment reference. One bundled Node entry and its complete asset manifest are required. Credentials, arguments, filesystem roots and transport configuration cannot come from a workflow or browser. See [the administrator example](../deploy/runner/opencode.example.json).

`OpenCodeExecutionCapability` is closed: sdk_version, server_version, provider_id, model, endpoint_origin, configuration_sha256, budget_mode=observe_only, identity_evidence=configuration_ack_only. Inventory, original task preview and producer `config.opencode_execution` retain equal facts. Publish/preview/start/current source/claim revalidate exact bindings; a change requires a new preview. A native/Codex selection cannot be attached to this fixed tool. Organization or project enforce policy blocks admission and atomic reservation; generic prices cannot make the execution priced or hard-capped.

## Trusted bridge and process ownership

The Runner creates the isolated attempt worktree, existing native-workspace MCP host and durable operation ledger. The SDK wrapper starts one private authenticated loopback server with a generated empty HOME/XDG configuration and a trusted six-tool module. Built-in execution/file tools, external plugins/skills, project configuration, implicit title/compaction model jobs and inherited accounts are disabled. Managed configuration is refused. Actual pinned plugin dependency files and their lock are packaged; no fake installed marker or per-attempt npm download is used.

Six custom tools list/read/write/edit/remove/check call the existing authorized host. Mutation identity includes attempt request, session, message, call and tool; calls serialize and never retry an unknown effect. Tool failure makes sealing fail through the original ledger. Model-provided paths, tool results and source remain untrusted data. This is a constrained application bridge, not an OS/network sandbox.

The private server stays in the Runner's process scope. Wrapper acknowledgement alone does not attest process containment. The Runner must complete authoritative process cleanup before accepting final stop evidence or sealing. Unconfirmed stop/EOF/disconnect becomes unknown reconciliation, not task success. Confirmed post-session cancellation additionally requires the original session abort ACK; confirmed pre-session stop does not invent an abort ACK.

## Frames and final facts

Runner stdin: one `{v:1,type:execute,request_id,goal,workspace_bridge}`; optional original `{v:1,type:cancel,request_id}`. Output is bounded JSONL with the same v/request_id: ready, started, progress and exactly one completed/failed/cancelled. The offline `--probe` checks asset/executable/configuration identity and version; it makes no model request and proves no account health.

Whole-turn completion requires the exact original session/user parent/final assistant/provider/model, completed timestamp, no assistant error, finish=stop and no pending or interrupted tool parts. HTTP200, step_finish, idle, EOF and abort ACK cannot stand in for that fact. Missing final counters are unreported; accepted counters are at most partial. Configuration ACK does not attest actual provider identity or billing.

`OpenCodeExecutionEvidence` records configuration digest, session/user/assistant IDs, requested/acknowledged provider and model, finish/completed_at, server_stopped and abort_acknowledged. Common DAG and current private/Pool callback validation enforce the same correlation and stop rules. Known startup failures may lack SDK evidence; a confirmed SDK stop cannot.

After completion the existing host seal, candidate transfer, independent exact-SHA test and human review remain separate. No automatic merge, acceptance, production deploy, foreign-session resume, input relay, planning/extraction/diagnosis or native conflict-repair capability is granted.

## Verification

The [stage report](../docs/quality/opencode-execution-verification.md) separates real SDK loopback tests, Runner protocol-child/MCP/Git checks, current control-plane tests, browser inspection and actual vendor execution. Only the explicitly recorded level is accepted.
