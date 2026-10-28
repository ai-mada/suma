# Tool profiles, fixed command workflows and structured execution

Status: current suma 1.0.0 contract, updated 2026-10-29 / T-QUALITY-119. Full product scope remains in docs/delivery/implementation-plan.md. Real HTTP/process evidence is recorded separately from vendor and production acceptance. Remote session resume, full terminal/input mediation and external bridge restart-query support remain separate requirements.

## T-P4-011 — Native diagnostic projection

`ToolEvent` adds optional `diagnostic:ToolDiagnostic`. The closed version1 object contains `schema_version:1`, one known native `message_key`, and an optional closed, exactly typed `message_params` object. It has no locale or rendered prose. The authority is the 46 definitions in `internal/localization/catalog_native.go`; generation emits the SDK directory, OpenAPI and TypeScript wire types from that contract. Only a `suma-jsonl-v1` error may carry a diagnostic object. Top-level `diagnostic:null` means absent metadata; inner `message_params:null` is invalid. Existing size/depth limits apply to the whole event.

Runner first proves the complete selected original message against its canonical template, then sanitizes the message and metadata and proves the cleaned pair again. Sanitization cannot create provenance for an originally mismatched message. Unknown keys, wrong/extra parameters, credential-bearing metadata or invalid selected Unicode drop native display metadata without manufacturing a different process/terminal/usage fact. The `display.locale` and `display.localized_message` supplied by the SDK are never trusted or persisted as platform evidence.

Formal tool events are stricter: duplicate, aliased/case-varied and unknown field names, invalid selected UTF-16 escapes and invalid integer lexemes are rejected. Project Runner HTTP retains raw `tool_event` JSON through current credential checks before strict decoding; pool Runner retains its existing raw path. This prevents an intermediate map remarshal from making lossy Unicode appear valid. Diagnostic rejection uses the controlled `dag.tool.diagnostic` message identity, preserving its raw error and error category.

Journal enqueues and pending readers deep-copy validated primitive parameters. All metadata is sanitized before journal/network and again before durable DAG event/outbox writes; the original typed request still determines operation idempotency. Current tenant/project, worker, attempt, dispatch, fence and pinned tool authority continue to apply. Diagnostic display cannot create a test, candidate, acceptance, budget debit or successful run.

Web display uses the current language only after exact canonical source proof; unavailable catalogs or unproven metadata show the original message. An independent disclosure retains the raw reason. Tool-list row identity binds the durable parent event identity and complete raw content; the scrolling40-row window must not transfer an open/focused raw reason to another event. This provenance is private UI state and is not serialized back into `ToolEvent` evidence.

The workflow editor requires a single allowlisted identifier for command/agent/test. Program arguments remain Runner configuration. Generic task/script nodes accept an optional custom-worker execution reference; the stock Runner does not gain support by editing that field. Imported malformed arrays remain intact until a human edits them. Compiler/capability matching is unchanged.

See [native diagnostic design](../docs/agents/native-diagnostics.md) and [local verification](../docs/quality/native-diagnostics-verification.md). Actual loopback429 transport validation is separate from authenticated vendor/model integration and software acceptance. Historical sections below retain their original scope.

## Ownership

- Platform agent: internal/toolcontract; internal/platform/tool_*.go; DAG tool validation/telemetry and capability matching; relevant tests. Root integrates HTTP/database wiring when requested. Do not edit runner/web or task_dispatch*.go.
- Runtime agent: internal/runner, cmd/suma-runner, packages/agent-runtime; runtime tests/docs. Do not edit DAG/platform/shared types.
- UI agent: apps/web; task dispatch and tool workspace, structured execution facts. Do not edit Go/generated contracts/root manifests.
- Root: task_dispatch*.go, shared contract, API generator, integration wiring, final docs, actual validation and commit. Freeze shared toolcontract types first and communicate paths before parallel source integration.

## Shared types (internal/toolcontract)

All JSON snake_case. No secrets, executable paths, raw args/environment values or external login files in inventory/read DTOs.

`Profile {command_id:string,adapter:string,protocol:string,profile_sha256:string,executable_sha256:string,tool_version:string,read_only:bool}`. IDs match `[A-Za-z0-9][A-Za-z0-9_.-]{0,99}`; both hashes canonical lower64 hex; version 1..256 UTF-8 bytes. Adapters command/suma/codex/opencode/harness/codebuddy; protocols text-v1/suma-jsonl-v1/codex-jsonl-v1/opencode-sdk-v1 with exact matching pairs. command/text supports generic allowlisted processes. No guessed vendor protocol. OpenCode uses only the [current pinned SDK contract](opencode-sdk.md), without a raw CLI JSONL compatibility parser.

`Observation {profile:Profile,available:bool,reason:string}`. reason is at most1000 UTF-8 bytes and nonempty when unavailable. An unavailable entry still names its configured command, but may lack unavailable executable/version hashes. Display declared/probed/available/execution/accepted distinctly. An installed binary/version/help is not authenticated model health or successful execution.

`Usage {input_tokens:int64,cached_input_tokens:int64,cache_write_input_tokens:int64,output_tokens:int64,reasoning_output_tokens:int64}` nonnegative optional fields. No money estimates.

`Event {schema_version:1,command_id:string,profile_sha256:string,protocol:string,kind:string,session_id?:string,item_id?:string,message?:string,status?:string,usage?:Usage}`. Kinds session_started/turn_started/activity_started/activity_updated/activity_completed/message/usage/error/turn_completed/turn_failed/waiting_input. IDs max128 safe printable characters; message max8192 sanitized UTF-8 bytes, status max64, overall event max16KiB. Validate bounded depth/unknown fields, reject invalid identities/schema. External facts never change DAG state, grant permissions, satisfy tests, approve or consume budget automatically. Waiting-input is an observation while the process/slot remains occupied; no unsupported reply/resume UI.

Progress request adds optional `tool_event:Event` alongside existing assignment/operation_id/message. At least message or tool_event required. Receipt binds the entire original payload before sanitization; current assignment/worker/attempt/fence validation and durable event/outbox semantics remain. Store event type `tool_progress`, data `{operation_id,attempt,fence,worker_id,tool_event}`. Result adds optional `tool_evidence:Evidence {profile:Profile,session_id?:string,terminal:string,event_count:int,usage?:Usage}`; terminal completed/failed/unknown. Persist current-attempt evidence in NodeRun; validate profile matches pinned node before accepting. Never inherit previous attempts' evidence. All known-secret and generic credential redaction occurs before journal/network.

## Administrator profiles and live observations

Use existing bound release-worker registration/credential for stable execution identity (profiles may be empty, deployment=false). Do not create a second generic credential authority. Human management uses current project write+approve permissions under organization lock; read current project authorization. Reject runner/agent/API scopes on human profile management.

`ToolProfile {id,project_id,worker_id,name,version,state,profile:Profile,enabled_by,created_at,updated_at,observation?:Observation,last_seen_at?,available:bool,reason:string}`. State paused/enabled/revoked. Immutable Profile definition; changed configuration needs a new registration. Create paused. enable/pause/revoke use CAS and stable operation receipts; revoke is permanent. Current enabling actor's live write+approve permissions are rechecked before dispatch.

- GET/POST `/tool-profiles?project_id` -> `{items,total,next_cursor}` / ToolProfile. Create `{project_id,worker_id,name,profile,idempotency_key}`.
- GET `/tool-profiles/{id}?project_id` -> ToolProfile.
- POST `/tool-profiles/{id}/commands` `{project_id,action:enable|pause|revoke,expected_version,idempotency_key}` -> ToolProfile.
- POST `/runner/tool-inventory` `{project_id,worker_id,operation_id,items:Observation[]}` -> `{acknowledged:true,observed_at}`. Runner bearer must match registered worker and current project authority; max128 items, strict validation. Repeated exact operations recover original receipt, changed body conflicts. Database time, report expiry60sec. Replayed old report must not refresh expiry. No historical reports asserted as currently online.

Each fresh Runner report is computed from locally allowlisted installed commands with execution enabled, known protocol and read-only version/help probe (timeout5sec/output32KiB). Local `--describe-tools` mode requires no platform token and emits these nonsecret observations. It must not invoke models or modify system config. Profile SHA binds canonical administrator command config (including private configured values only via digest), adapter/protocol and resolved executable SHA; does not claim complete transitive script/module supply-chain attestation. Revalidate executable/config hash immediately before launch; changed/missing/disabled configurations refuse execution. Inventory marks missing/unsupported entries unavailable. Probe once startup then refresh on bounded interval/config or executable drift, not every2sec model launch.

Worker claims command-scoped capabilities `command:<command_id>` only for available local allowlist entries; no unsupported node should consume an attempt. DAG command/agent/test nodes require exactly one command and a matching command capability (plus any explicitly declared capability), including dynamic maps and compensation. Empty capabilities never mean unrestricted. Existing internal tests must declare actual matching capabilities; do not add a permissive production bypass to preserve tests.

Tool-bound agent node config contains `tool_profile_id` and `tool_profile_sha256`. Platform eligibility checks registered worker identity, enabled immutable profile/current enabling authority, exact command/adapter/protocol/hash and matching fresh observation before acquiring attempt/resources. Nonmatching candidates remain ready and do not consume attempts; independent eligible candidates can continue. Compiler rejects partial/malformed tool binding and non-agent bindings. Assignment retains the fixed config. Current read models must explain unavailable reasons without reporting configured as healthy.

## Immutable task dispatch plans (root)

- POST `/task-dispatch-plans` `{project_id,task_id,expected_task_version,workflow_id,workflow_version,agent_node_id,tool_profile_id,expected_tool_version,idempotency_key}`.
- GET `/task-dispatch-plans/{id}?project_id`.
- POST `/task-dispatch-plans/{id}/start` `{project_id,expected_version,idempotency_key}` -> Run.

`TaskDispatchPlan {id,project_id,version,state,task_id,task_version,requirement_id?,requirement_version?,workflow_id,workflow_version,agent_node_id,tool_profile_id,tool_profile_version,tool_profile_sha256,run_id,created_at,expires_at,context:TaskContext,workflow:Workflow,valid:bool,reason:string}`.

TaskContext freezes task title/description/acceptance and current linked requirement title/description/acceptance/version in the same authorized transaction. Plan expires after30min database time, readable history remains. Copy exact published workflow and replace only chosen agent's command/profile binding and goal. Goal combines original node instructions with explicitly delimited untrusted task/requirement context; bounded32000UTF8 bytes, no truncation. No code/branch interpolation. Reject wrong node, invalid workflow, unsafe/missing test candidate prerequisites or oversized context explicitly. Preview shows full fixed context, agent/profile/worker, tests and human acceptance boundary. All other node definitions stay frozen.

Start uses existing CreateManagedRunBound atomically for frozen derived workflow, reserved run ID, current auth/profile/observation check, current task+requirement versions, BindRunTx and plan started fact. Old preview cannot start after task/requirement/profile/auth changes or expiry. Same operation replays same run without rebinding changed task; authorization precedes replay. Production policies, test evidence and human acceptance remain existing gates. Failed real attempts continue through FailureIssue and existing deduplicated requirement intake. Task resource owner remains separate from machine execution identity.

## Protocol/runtime requirements

Separate stdout JSONL parser from bounded/redacted stderr. Reject oversize line/depth; fragmented UTF-8 and multiple events supported; unknown vendor event variants are bounded and do not become invented facts. Only allowlisted metadata/text/status/token counts; never persist chain-of-thought, raw tool arguments, URLs for follow-up, arbitrary payload objects or native credential output. No evaluation of external text.

For structured protocols success requires process exit0 AND one consistent supported terminal success; a vendor terminal failure cannot be overridden by exit0. Missing/conflicting/malformed terminal or process/control uncertainty must retain explicit failure/unknown evidence, never fabricate success. Actual process tree stop acknowledgment remains authoritative for cancellation; existing independent worktree/candidate capture/test evidence rules remain. Session ID is read-only provenance, never an automatic resume command. Automatic session recovery/retry after uncertain side effects is forbidden.

A confirmed process-tree stop (`stopped:true,outcome_unknown:false`) may carry `tool_evidence.terminal:"unknown"` when cancellation prevented a protocol terminal. Accept that exact fixed-profile receipt: a cancelling run can become cancelled, and a run without a cancellation request records failure rather than success, including a zero OS exit during a stop race. Preserve the unknown protocol fact; do not manufacture a completed tool terminal. Without a confirmed stop, unknown tool evidence still requires `outcome_unknown:true` and reconciliation.

Confirmed stop classification applies to generic commands and bound tools alike, including zero OS exit with absent, failed, completed or unknown protocol evidence. Outside cancellation a confirmed stop records failure; during cancellation it records cancellation. Startup may stop before protocol evidence exists, so missing evidence is accepted only as a non-success stop fact. Supplied evidence must still match the exact pinned profile. `outcome_unknown:true` always retains reconciliation, even when the process has stopped; no stopped receipt activates success-dependent nodes or satisfies a test. Normal completion must leave `stopped:false` and satisfy the usual terminal/candidate evidence checks.

Native agent CLI uses SUMA_AGENT_TOKEN for `/agent/tools`; Runner token remains runner-only and must not be forwarded as agent authorization. Fix docs and add actual platform HTTP scope tests. Native tool reads are authorized by separate agent scope/current project membership; model output cannot widen tool catalog. CLI probe/structured fixture verification and authenticated vendor execution are separately recorded.

## UI and verification

Use existing compact local workbench. Settings show execution workers/profile configuration, observed version/protocol, online expiry and actual availability reasons; no false percent/uptime. Task details -> preview fixed plan -> explicit start -> real run. Preserve unknown operation identity and CAS drafts, current project/user isolation and URL state. Workflow editor selects actual profiles, retains keyboard/JSON equivalent. Run panel shows typed facts/session and terminal status separately from DAG/business/test/acceptance. Partial narrow/wide/light/dark and Escape/focus must be tested in browser.

Tests include missing tool/disabled exec/wrong capability keeping ready without attempt, pinned digest drift, stale/replayed inventory, another worker/token/project, revocation, plan version/content changes, duplicate start/lost ACK, structured malformed/oversize/split output, success/failure conflicts, cancellation/unknown, credential redaction and actual agent-vs-runner scope. Actual journey must send task context into a real process, execute isolated candidate and independent test, human acceptance, plus a failed process and deduplicated issue/requirement. Deterministic fixture must be labelled as fixture; real CLI probe and any model execution separately disclosed.

## Fixed command workflows

An explicit `command` node may bind the complete `tool_profile_id` and
`tool_profile_sha256` pair. Its profile must be `command/text-v1`; structured
agents cannot acquire command-node semantics. Both private and shared-pool
claims and callbacks recheck the current authorized profile and exact
fingerprint. Agent development still uses its reviewed task dispatch plan.
A deterministic command workflow is not an AI model call.

Issue regression closure additionally requires the scheduler-pinned candidate,
the original producer attempt/fence/evidence, and a different test Worker. A
test command's self-reported source hash alone is insufficient.
