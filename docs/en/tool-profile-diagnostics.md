# Automatic tool profile diagnostics

T-TOOL-DOCTOR-181 · suma 1.0.0. See the [domain contract](../../contracts/tool-profile-diagnostics.md), [product flow](../product/tool-profile-diagnostics.md), [architecture and AI SOP](../architecture/tool-profile-diagnostics.md), and [interaction design](../design/tool-profile-diagnostics.md). See the [scoped verification report](../quality/tool-profile-diagnostics-verification.md); full production acceptance remains pending. The full 31 requirements/P0–P7 target remains active.

Selecting an official tool profile automatically reads its current prerequisites. A compact card in the original details shows the first blocking reason and an existing next step; users can expand the four facts. It assigns no health score and performs no suggested action automatically.

## Four separate facts

| Fixed code | Meaning |
| --- | --- |
| `authorization` | Original profile state and the enabling user's current authority |
| `worker` | Existing private/Pool identity, credentials and eligibility; valid identity does not establish that a machine is online |
| `inventory` | Whether the exact worker has supplied a current, fresh inventory |
| `profile` | Original command, complete Profile equality and original Observation against fresh inventory |

Each status is exactly `pass`, `blocked` or `not_checked`. Missing, expired or future inventory blocks inventory and leaves profile `not_checked`; an old Observation cannot yield a pass. A profile that is not enabled can still expose independent worker and inventory facts.

Original `Tool.Available`, `Reason` and Observation remain the availability authority. Diagnostics share their collector and ordering rather than creating another ready flag or dispatch rule. Model login, budget, execution, tests and human acceptance remain separate facts.

## Current read contract

`GET /api/v1/tool-profiles/{id}/diagnostics?project_id=...` checks current `browser-project-read` and the exact resource ID before looking up the profile. API, Agent, Runner and Pool tokens do not gain this permission.

`ToolProfileDiagnostics {tool:ToolProfile,checked_at,expires_at,checks,next_action}` carries the complete original Tool. Checks contain exactly `authorization,worker,inventory,profile` in that order; each is `{code,status,reason,reason_diagnostic?,next_action}`. Display diagnostics carry identity only; fixed domain results determine actions, not keys or arbitrary reason text.

The read retains the original current-read transaction, database time and shared observation/activation/worker collector. It writes no business state or operation receipt and invokes no model, CLI or SDK. No scheduler, repair path or execution authority is added.

## Existing next steps

| Action | Existing destination |
| --- | --- |
| `review_profile` | Original versioned CAS enable confirmation for a paused profile or lost enabling authority; an enabled, non-revoked profile requires explicit review |
| `open_worker` | Original management entry for the exact worker, for identity, credential, Pool or inventory issues |
| `register_profile` | Original quick configuration flow for a revoked profile or changed complete configuration |
| `refresh` | Read current facts again when unknown or necessary |
| `none` | No additional action when all known prerequisites pass |

The overall action follows the first blocking check in the original availability order. Buttons use only parent-provided actions bound to the current exact source. Without an exact management entry, show the reason and refresh; do not invent a local probe, new worker or repair button.

## Display deadline and identity

`checked_at` uses one database instant. `expires_at` is at most 60 seconds later and cannot exceed fresh inventory's original deadline. This TTL limits display; it is not an authorization lease. The UI measures monotonically from request start, deducts network time, then hides actionable advice after expiry and offers refresh.

Discard late responses after the API caller, account, organization, project, tool ID or version changes. A returned version different from the current details requires refreshing those details. An old report cannot unlock original actions or restore an old identity.

An unresolved original write takes priority. Diagnostics cannot clear pending state, replace its original body/stable key or overwrite an unknown receipt. Subsequent writes retain current authorization, explicit confirmation, original `expected_version` CAS and original operation receipts, even while the report remains fresh.

## Read-only CLI and evidence

`suma --project PROJECT tools doctor --id PROFILE_ID` reuses the original login session, current project view and client. It reads the same GET and strictly checks scope, complete Tool, all four checks and their deadlines. It prints facts only, never runs suggestions or enables, renews, registers, dispatches or repairs anything.

The bilingual card uses existing controls, visible focus, keyboard access and narrow-screen layout. Loading, errors, expired reports and unchecked facts remain explicit; refresh is read-only. A failed read does not create an unknown write.

Focused authorization/fact/deadline/CLI and interaction checks, actual HTTP/SQLite evidence, synthetic UI evidence and real vendor execution are recorded separately by integration. This document claims no passing acceptance; full remote, multi-machine, database, installation and platform delivery continue within the original scope.
