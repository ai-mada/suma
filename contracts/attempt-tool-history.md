# T-P4-012 — Durable tool facts for an exact execution attempt

Status: frozen implementation contract, 2026-10-25. Owner: control/DAG, workbench and integration. Base `79dd848ba253b23516c17e9e5ea275f04f5aa05f`. Implements the accepted persistent attempt/log/evidence journey; the full P0–P7 and all-surface internationalization scope remain active.

## Authority and evidence

The durable `dag_attempts` row joined to the authorized run defines run/node/attempt/fence/worker identity. Never use the current NodeRun or an unverified nested Result as the historical identity. A saved runner-shaped Result may be promoted only if its run/node/attempt/fence/worker match that row; mismatched saved receipts fail explicitly without rewriting the original. Manual reconciliation, child workflows and independently managed observations retain their existing distinct source semantics.

Return original persisted `Event` envelopes for `tool_progress` only when their original parent data proves the exact attempt/fence/worker. Require exact case-sensitive attribution keys, positive canonical safe-integer JSON lexemes and exact worker string; do not coerce fractions, exponents, nulls or strings, or accept duplicate/aliased keys. Selected identity strings must not acquire provenance through invalid-Unicode replacement. Missing or malformed/unprovable parent attribution is not repaired or assigned to the latest attempt. A proven parent with unsupported/missing inner tool metadata remains available as raw evidence; display validation cannot delete that row or translate arbitrary tool/model content. No raw event, tool result, usage or business state is rewritten by reading.

`Event.raw_data` is globally optional and populated only by this new reader with the original stored data text. It is required on every item in this page. `Event.data` remains the compatible parsed projection; raw_data independently preserves nested duplicate keys, escape spelling and numeric lexemes that a map projection cannot represent. The raw text must be valid UTF-8 and is never repaired, translated or written back. Other run/SSE event responses omit the new optional field and retain their existing JSON. Both representations count toward read byte budgets.

Go and JavaScript decode escaped unpaired UTF-16 differently in unknown inner content. Client compatibility comparison uses an ephemeral Go-compatible string-token projection before JSON parsing, preserving source-order collisions of normalized keys; original raw_data and selected parent proof remain unchanged. A repaired original tool frame cannot gain typed/native diagnostic identity from its compatible replacement: retain that row as raw-only. Valid pairs, literal U+FFFD and leading U+FEFF in valid strings keep their meanings. This is compatibility interpretation of a projection, never evidence repair.

## Public read contract

`GET /api/v1/runs/{id}/nodes/{node}/attempts/{attempt}/tool-events?project_id=...&limit=...&cursor=...`

Only human sessions and currently project-bound API scope may read. Current tenant/project/member/session expiry/token scope are checked in the same transaction as event selection, using the organization governance guard shared with persistent run-event reads. Agent/Runner or foreign-project scopes cannot read. The existing governance-lock coordination counter may advance; no scheduler/global event lock, run/attempt/event/outbox/operation, resource/budget debit, issue/intake or execution is created. No network wait occurs within the transaction.

The strict query accepts exactly one nonempty project_id and optional single limit/cursor; unknown, duplicate, bad encoding, explicitly empty or oversized values reject. attempt is a canonical positive decimal safe integer. limit defaults50, accepts1..100. Missing authorized run/node/attempt is404, expired/revoked session401, lost grant or wrong credential scope403, invalid query/cursor400. Infrastructure or corrupt selected result errors never masquerade as successful empty history. All new controlled diagnostics have paired zh-CN/en-US catalog definitions and preserve raw cause/category.

Go `AttemptToolEventPage` and generated wire type:

```text
schema_version: 1
project_id, run_id, node_id: string
attempt: positive safe integer
fence: nonnegative safe integer
worker_id: string
items: Event[]
watermark, scanned_through: nonnegative safe integer
has_more: boolean
page_cursor: string
next_cursor?: string
```

`watermark` is the last candidate tool_progress sequence for this node when the first page begins (0 for none). `scanned_through` is the last candidate examined, not a matched-event count. `page_cursor` binds the requested start position to this saved window: the initial response creates a position0 token, later responses echo their validated request token. Re-reading or navigating back to the first page therefore retains the same watermark, including an originally empty window. `has_more` exactly matches presence of next_cursor, which must differ from page_cursor. There is no invented total, percentage or ETA. First/next pages contain increasing unique sequences within the same watermark and exact identity; an empty page with continuation is valid. Only exhaustion establishes that all safely attributable facts in that saved window have been examined. A no-cursor refresh starts a new window; paging never silently changes it.

## Bounded portable query and cursor

Use one reusable `AttemptToolEventsPageTx` DAG query accepting an existing SQL transaction, with a non-Tx wrapper for direct callers/tests. Read the exact attempt metadata without loading full run graphs or result blobs. Seek candidates by tenant/project/run/node/type/sequence; add `dag_events_node_type_sequence` on `(tenant_id,project_id,run_id,node_id,event_type,sequence)` through the existing definition-checking `store.EnsureIndex`, preserving the existing run index and all rows. Startup DDL is synchronous and may block on a large store; no claim of online migration or actual distributed DDL acceptance.

Each request examines at most1000 candidate rows; SQL returns at most1001 for lookahead. Iterate rows without accumulating all raw candidates. Stop at result limit or response-byte budget, advancing only through candidates actually examined/accepted, never beyond an unreturned matching row. One serialized event cannot exceed2MiB; the serialized returned-event array budget is4MiB. An oversized selected event fails explicitly; a later event that does not fit the remaining page resumes on the next page. These are read budgets, not truncation or repair. Later matching events remain reachable even after empty intermediate pages. Work across all pages can still scan other attempts of the same node; normalized indexed projection/backfill and large-scale latency remain separate implementation/acceptance work.

The opaque cursor is version1, maximum4096 encoded bytes, strict unpadded base64url JSON with separate tenant/project/run/node/attempt/fence/worker/last-scanned/watermark fields. Require safe integer literals and exact durable identity; reject duplicate/unknown fields, scope substitution, last-scanned outside0<=position<=watermark, or nonzero markers not present in the exact node/tool_progress stream. Verify a nonzero watermark marker too. Zero position represents the start; zero watermark represents a saved empty window and does not move forward when facts are appended. A cursor conveys a read position, never permission or cryptographic integrity; authorization repeats on every request. It survives process restart and later event appends without silently resetting.

## Workbench interaction

Add an inline selected-attempt history panel below the existing exact `AttemptFacts`. Use generated page/Event types and the existing attempt deep link; share/refresh restores project/run/node/attempt, not opaque local paging credentials. Keep one bounded page in memory, with bounded previous-position navigation (maximum100 local cursor entries; beyond that retain Next and explicit restart). Expose first/latest read, previous/next, loading, retained stale page, access loss, missing data, empty intermediate page and exhausted window distinctly. Read failure retries the requested position; locale/catalog changes only redisplay. Authorization loss clears sensitive page data and traversal rather than leaving an apparently current result.

Validate response schema/project/run/node/attempt/fence/worker, sequence ordering/uniqueness/window, bounds, continuation consistency and exact parent attribution before accepting a page. A different account/organization/project/run/node/attempt, refresh or newer request invalidates older responses, including errors. Never merge pages from different windows or borrow the latest NodeRun evidence. A selected historical attempt is a fixed read even while that attempt is currently running. Existing current live facts remain explicitly labelled as the current attempt and retain their40-row cap.

Extract/reuse event-row rendering and native message projection instead of duplicating catalogs or canonical proof. Durable pages render every returned row, including >40 and raw-only legacy inner payloads. Show persistent sequence and original timestamp, current-locale date, typed display when recognized, original raw_data as safely rendered text and independent raw Event disclosure. Validate that parsing raw_data agrees semantically with data without modifying either source; the original text remains the evidence when parsing loses lexical details. Changed/remounted event content cannot inherit another row's open/focused evidence. Native details/buttons, restrained polite status and aria-busy, readable wrapped identifiers/JSON,44px narrow actions and retained existing light/dark tokens. Replacing a page moves displaced focus to the page heading/control; same-page/locale redisplay preserves surviving focus and disclosures. Closing selected detail returns to history.

## Verification and implementation ownership

Control implementer owns internal/dag query/types/index/tests, internal/platform dedicated HTTP/read-authority extraction/tests and new localization definitions. UI implementer owns apps/web new hook/panel/shared rows, exact selection wiring, bilingual keys and DOM/race tests. Root owns this contract, generator/OpenAPI/wire types, actual service/browser integration, design/quality/memory and final necessary-only local commit. Fixture implementer writes ignored stage012 scripts only. Freeze shared interfaces before parallel source edits; no dependencies, installations, paid model, outbound uploads or Git mutations by subagents.

Meaningful backend RED→GREEN: real Progress across retries, >128 earlier events/>40 facts, exact all-page union, >1000 foreign-attempt candidates and valid later fact, empty continuation, append after watermark/restart, strict raw attribution and cursor/query boundaries, raw-only nested metadata, byte budget, portable index definition/seek/restart/unchanged data, no run/result/event/resource mutation. Current HTTP authorization must be exercised with real sessions/scopes and two-connection revocation/SQL-order evidence; SSE and legacy attempt/result-source behavior regressions included.

Meaningful UI RED→GREEN: actual selected-attempt consumer and URL/back/reload, >40 rows all visible in a page, page/refresh/selection/unmount races, access loss versus transient retained-page state, no locale-triggered GET/POST, frozen window, raw-only rows and strict scope refusal, stable disclosure/focus. Root runs appropriate complete suites/types/contracts/build, independent source-scoped reviews and fixes, then actual native HTTP429 with two idempotent attempts and small-limit real HTTP paging. A separately labelled structured-protocol fixture may exercise >40/default-page overflow through the UI; it must not be described as native SDK/vendor model execution. Actual reads must preserve execution/event/model counts and deduplicated feedback. Preserve all source cuts, failures and raw receipts under ignored `.cache/`; commit only necessary implementation/design/contracts and compact evidence. Other OS install, vendor integration, database distribution, all-surface language, scale and full platform acceptance remain unproven.
