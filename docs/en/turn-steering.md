# Send a saved message to the original running turn

Status: implementation in progress for suma 1.0.0; requirement: T-TURN-STEERING-188; owners: integration, control plane, Runner and Web. Sources: [domain contract](../../contracts/turn-steering.md), [product requirements](../product/turn-steering.md), [architecture and AI SOP](../architecture/turn-steering.md), [interaction design](../design/turn-steering.md). Implementation, verification and human acceptance are separate facts; this document does not claim a passing verification result.

## User journey and scope

A user saves a human message in the existing task discussion, opens its explicit steering action, checks the complete original text and exact live target, then separately sends it. Posting an ordinary comment does not send it to an agent. A new supplement requires another saved message; the steering panel cannot rewrite the original body.

The current scope is native Codex text `turn/steer`. It uses the original process, thread and active turn. It creates no new turn, resumes no old thread, and replaces neither the original plan nor the acceptance baseline. The original model, budget, workspace, tool permissions, knowledge and skills retain their own controls. Stable text steering does not require the experimental T187 question feature and adds no desktop switch.

The server requires a current browser project writer sending their own saved human message to the same task's exact original execution before formal acceptance. API, Runner and assistant messages cannot impersonate that author. Runner advertises a live session only after exact original configuration and `turn/start` acknowledgements, before stop or closure. A running node, an old run ID or an active session record alone proves no current sending authority. Current machine identity, complete Assignment, inventory, Grant, lease/fence, pause and budget checks still apply.

Each session accepts at most 32 instructions, with one instruction per saved message. Text follows the original discussion producer's shared validation: valid UTF-8, at most 32,000 bytes and 8,000 Unicode characters, nonblank and without NUL. All other characters are preserved through JSON without truncation. This is not a path for secrets or credentials.

## Independent delivery facts

| Fact | Meaning |
|---|---|
| Saved | The platform committed the instruction and original target binding |
| Observed | The original Runner observed it |
| Write started | The native write began; issuing a new write is unsafe |
| Fully written | The original protocol frame was completely written |
| Native accepted | The exact request received acceptance for the same turn |
| Native rejected | The original request received an explicit rejection |
| Unknown | Write or acknowledgement cannot be established; reconcile the original operation |
| Closed, not delivered | The original turn closed before dispatch; no acceptance is inferred |

Native acceptance does not establish model compliance. Execution completion, a sealed code candidate, independent tests against that exact commit, human acceptance and deployment retain their existing separate evidence and decisions.

## Architecture and contracts

`codexcontract` owns fixed native parameters, source binding, deterministic digests and safe evidence. `steeringcontract` owns the session, original message, instruction and browser request. The control plane owns current authority, transactional CAS, storage, receipts and Result validation. Runner reuses the original app-server, Journal, single stdin writer, ordinary/Pool clients and Worker lifecycle. Web reuses task discussion and existing scope/recovery guards. No second dispatcher or model layer is introduced.

`CodexSteeringSource` fixes `codex-turn-steer-v1`, original thread/turn IDs and binding SHA-256. A session fixes project/task/run/node/attempt/fence/worker/dispatch/profile, source, version and expiry. Its state is active or closed; send is the only possible allowed action. Current authority is computed independently of that state. Instructions retain the original message ID, sequence, author, full body and SHA-256.

Browser context is read with `GET /tasks/{id}/execution-steering?project_id&message_id`; explicit creation uses `POST /task-execution-steering`. The browser sends only project/task/message/session IDs, expected task/session versions and an idempotency key. The server resolves the authoritative body and native address. Closed context reasons are `not_own_message`, `task_accepted`, `no_live_turn` and `current_execution_unavailable`; private records are not exposed.

Ordinary and Pool machines use their existing complete Assignment proof with `/runner/steering/…` or `/pool-runner/steering/…`: sessions, poll, observations and close. Open returns `SteeringOpenResult`: a session without a reason, or HTTP 200 with only `reason=not_task_execution` when current machine identity and full scope are valid but the original workflow is not managed task development. That normal no-session outcome advertises no steering capability and preserves the ordinary workflow. Authorization, database or unknown HTTP errors must not be silently treated as unsupported. Close returns a session, Poll returns its instructions, and Observation returns the instruction. Successful operations return HTTP 200; generated schemas reject unknown fields. Results remain on the original Result path. Current schema declarations, uniqueness and no-write preflight apply without historical shape backfills or data resets.

Current human or machine authority is checked before recovering a receipt. An exact already-committed receipt is then recovered before fresh version, liveness or expiry checks. This preserves one committed operation without reviving expired credentials. Fresh polls and writes for an active session still require current original execution authority. After session closure, Poll checks current machine identity and the complete original scope, then permits a read-only final list of all instructions, at most 32, without the fresh new-work gate. This only reconciles instructions around Close and completes Result evidence; it cannot authorize a native write. Started delivery facts are preserved as facts when later eligibility changes.

## Native safety and recovery

The only native parameters are `threadId`, `expectedTurnId`, one text input containing the exact saved body, and `clientUserMessageId=instruction.id`. Shared builders produce the deterministic JSON digest; model or sandbox overrides are forbidden. The client message ID is correlation data, not a vendor idempotency guarantee.

Steering acknowledgements have their own pending map and use the original monotonic client RequestID. They cannot overwrite startup waiting IDs or be confused with server-originated approval/question IDs. A response must match the typed request ID and exact original turn. Observations accumulate a monotonic sequence, parameter digest, observed/write-started/fully-written/native-accepted/native-rejected/turn-closed facts and a fixed safe failure code.

Persist intent before writing, recheck current platform authority, then use the original single writer. Complete writes and native acknowledgements are independent. Early acknowledgements are staged safely until callbacks merge; a close before a fresh write prevents sending, while late facts from a started write remain recorded. Unknown writes or lost acknowledgements never trigger another native send, a new key, a new turn or a thread resume. Only original HTTP requests and observations are replayed.

Ordinary and Pool recovery uploads original observations and session closure before Result replay. A successful Result must match every registered instruction, persisted observation and closed session. Unknown prevents success; rejection or closure before dispatch cannot masquerade as acceptance. Safe execution evidence contains source, instruction IDs, message hashes and observations, with no message body copy.

## Interaction, implementation SOP and evidence

Use one explicit action beside the original discussion message and one on-demand panel. Do not poll each row. Preview full text and the exact run/node/attempt; require explicit target selection and a separate send. Keep technical IDs in expandable details while delivery facts and recovery remain visible. The active record never substitutes for current `can_send` and allowed actions.

Persist pending intent before the network call under user/org/project/task scope, retaining the old session, message, execution tuple, body and key. A new attempt cannot hide it. Unknown outcomes retain that intent for exact recovery; never clear it on storage failure or a protected refusal. Scope changes and late responses use the existing epoch guard, revoked access clears private content, and discussion/reply/development/draft actions aggregate their busy state. Chinese and English labels share the same behavior; original text is not automatically translated. The panel supports 390px layout, wrapping, visible keyboard focus and explicit errors.

The development SOP is to read current contracts and source, assign module ownership, fix native schema/version provenance, and cover current authorization/CAS/exact receipts before expanding behavior. Reuse suma's existing deep modules. The local Multica interaction reference is commit `5a2cc891ac338eed1919aa11a1eef5957cd82cec`; external Web references are recorded separately. Do not copy restricted code or import native timeout retries/new-turn fallbacks. Exercise closure races, early ACK, partial write, wrong-turn ACK, unknown recovery, revocation and both ordinary/Pool paths. Run new and directly affected checks, reuse unchanged successful evidence, and consolidate final type/language/component checks. Record original failures, exact source hashes and environment; keep caches, logs, databases, screenshots, private configuration and binaries outside the committed delivery.

Focused protocol fixtures, real CLI with an explicitly owned controlled model, authorized HTTP/SQLite, synthetic production-component UI and human acceptance require separate evidence. They do not establish real vendor execution or full authenticated App acceptance. The original 31 requirements and P0–P7 remain active: 58 limited, 34 pending, 92 total. Physical multi-host, other databases, cross-platform installation, production and the prior T171 explicit GUI-stop gap remain subject to their own delivery and verification.
