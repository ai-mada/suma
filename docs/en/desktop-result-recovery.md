# Original result receipt recovery

T-DESKTOP-RESULT-RECOVERY-179 · suma 1.0.0. See the [frozen contract](../../contracts/desktop-result-recovery.md), [product flow](../product/desktop-result-recovery.md), [architecture](../architecture/desktop-result-recovery.md) and [interaction design](../design/desktop-result-recovery.md). Targeted platform SQLite/HTTP checks have passed. Desktop, private CLI and UI integration claims require their final evidence.

This feature covers an exact case: the server committed an original Runner result, its acknowledgement (ACK) was lost, and the machine credential subsequently expired. A current browser administrator can reconcile that immutable receipt and restore the existing local ACK. The expired token remains invalid. Recovery neither repeats execution nor calls a model, submits a new result, renews credentials or reconnects automatically.

## Explicit recovery flow

The user chooses recovery on the original desktop connection after its Runner process has closed. The main process privately prepares and encrypts the original plan and operation ID, queries the committed receipts, saves the signed proof, and invokes offline apply. Apply verifies the original journal snapshot, identities and digests under the original lock, clones the document, then makes one atomic save. Only exact matched entries may change from `acked:false` to `acked:true`; it does not open the normal replay journal or call Ack repeatedly.

The original plan can recognize an ACK saved before its response was lost. Any other journal change blocks application. A fresh proof may be requested for the same plan under current authorization when its old proof expires. Missing receipts never authorize new callbacks or execution.

## Private contract and authority

The existing loopback/HMAC bridge exposes private `POST /desktop-runners/recovery/receipts`. Its closed request is `{scope,recovery_operation_id,plan}`; HTTP 200 returns `Proof`. Input is bounded to 256KiB and each plan to 32 items. Shared types and digest/signature helpers live in `internal/desktoprecoverycontract`.

Current browser administrator authority is checked before historical data. The original holder, registration receipt, Worker credential digest, instance, workspace binding and base commit must match. The HMAC binds the ten Scope fields, recovery operation ID, Go-encoded plan digest, nonce and expiry. The old token is never used to authorize this request, and receives no Claim, heartbeat or write permission.

The server reads the exact tenant/project/`result:<run_id>`/operation receipt and verifies its historical node, attempt, Worker and fence. A later current head does not replace that fact. Fence is a canonical positive int64 decimal string. `body_sha256` binds the local Callback bytes; `result_sha256` reproduces the server's original typed `dag.Result` fingerprint, rather than hashing the Runner envelope. The full journal snapshot preserves JSON numbers and normalizes only the target ACK flags.

| Receipt status | Meaning |
| --- | --- |
| `matched` | The exact original result was committed; the corresponding local ACK may be restored |
| `missing` | No exact committed receipt exists; preserve the local result |
| `mismatch` | The stored operation fingerprint or result identity differs; keep recovery blocked |
| `source_changed` | The original connection or historical execution source differs; reconcile that source first |

Proof binds Scope, plan digest and ordered item statuses. Its independent HMAC domain uses the workspace private key; expiry is 60 seconds after the database authority check. A proof attests facts and grants no execution authority. The server does not Complete results or commit business/audit changes during lookup.

## User-visible states and limits

The original `manageLocalTool` flow adds explicit `recover` and read-only `recovery-status`. The recovery card displays counts and `prepared/checking/unknown/complete/partial/blocked`, with a concrete next step. While work is active or uncertain, parent switching and new registration stay blocked, but the original status action remains available. The renderer receives only the safe summary, never the plan, proof, paths, result bodies or keys.

Recovery completion means the plan's local ACKs are restored. Credential renewal and an explicit connection remain separate actions, with paused registration and enablement unchanged. An uncertain renewal must be reconciled through its original operation first. Other sidecar journals are untouched; unresolved cloud, workspace, release or permission effects still block the existing drain checks. An original `unknown` result remains unknown even after its transport ACK is restored.

Uncommitted side effects, the original 31/P0–P7 goals and complete platform acceptance remain outside this slice. Each actual CLI and representative UI claim requires its own scoped evidence.
