# Remote workspace CAS saves

Status: T-P4-038 / 1.0.0, bounded implementation, 2026-10-27. Owners: Control for authorization and durable commands; Runner for native effects; workbench/CLI for drafts and explicit recovery. The [contract](../../contracts/workspace-write.md) and [architecture](../architecture/remote-workspace-write.md) are authoritative. GUI, physical multi-host/TLS and production acceptance are recorded separately.

Only an existing complete UTF-8 text file in one current native attempt can be overwritten. Both old and new text are limited to 48 KiB. Obtain the complete source through read-only `read` with `full_file:true`; paginated or truncated output is not an editable whole-file baseline. Fix the relative path, original byte SHA, desired content SHA, exact run/node/attempt/fence/worker/tool scope and active workspace policy SHA. No create, delete, force overwrite, host path, browser MCP/Bearer, terminal or Git write is exposed.

Create and execute claim both require the original credential's current project write permission, the current unaccepted task run/basis, and the approved native tool/current inventory. Pool execution additionally requires the original full assignment, current machine credential and the same current bilateral grant ID/version. `can_write` describes human/task eligibility, not host health. Expiration of the original read observation alone does not revoke a draft's CAS basis; every new save is reauthorized.

| Public API under `/api/v1` | Meaning |
| --- | --- |
| `POST /workspace/writes` | Create a fixed save command with exact content and a stable key |
| `GET /workspace/writes?project_id=…` | Original credential's latest 50 metadata items, exact total and truncation flag |
| `GET /workspace/writes/{id}?project_id=…` | Current project-read authorization and original command facts |
| `POST /workspace/writes/{id}/inspect` | Explicit inspection with expected version and a stable key |

Commands never return source content. A new login, even for the same user, cannot read an old credential's commands. Original credential/current project-read permission can retrieve effect metadata after the target closes. A lost create response can also be recovered with the same original key and request digest under current read permission after write permission is withdrawn; this only reads the existing receipt. New commands and first execution still require write permission. Workbench drafts remain in memory; the recent-save list restores command metadata, not source drafts. The intended interface uses simple editing, explicit saves/inspection and manual adoption after reviewing a conflict diff; this document does not claim GUI acceptance.

A request lasts 120 seconds. Execute permission lasts at most 10 seconds and is issued once with execution fence 1. Timeout or uncertain effects become `reconciling`; execution is never requeued. Private and Pool workers use outbound `/runner/workspace-writes/{claim,result}` and `/pool-runner/workspace-writes/{claim,result}`. Empty authorized `lease:null` polls are stateless and create no operation receipt; actual leases and ACKs retain exact recovery identities.

Inspection is explicit, uses its own claim ID and 10 second lease, and contains no content. It can only inspect the original native operation. A completed exact native receipt proves applied. A durable `not_started` barrier under the native write/seal mutex proves no effect and blocks a late execute. A missing/closed host or an uncertain prepared operation remains unknown; never reopen historical paths, infer success from matching file bytes, or repeat the write. A lost inspection response must reuse the same command ID, original key and original expected version before any new inspection action.

Current original machines may drain fixed late facts after target closure, caller revocation or Pool grant withdrawal. This does not grant new writes or content delivery. Applied facts cannot be downgraded by later unknown results. Authorization and the remote rename are not a distributed atomic transaction: an already issued short permit can still take effect before its deadline after revocation.

Control stores delivery content separately and clears it on timeout, unclaimed expiration or returned effect facts; operation receipts contain metadata only. Cleanup is bounded to 100 commands per sweep. Runner private journals/native ledgers have separate recovery lifecycles; logical cleanup does not mean simultaneous erasure of every disk copy.

Native CAS uses the existing ledger/mutex shared with model edits. A successful write increments generation and invalidates earlier native checks for sealing. Fixed execution plans, save receipts, independent tests of the exact candidate SHA, human acceptance and deployment remain separate facts. Saves do not invoke models or change task/DAG state.

CLI: `workspace save --file save.json --wait`, `workspace save-status ID --wait`, and `workspace inspect-save ID --expected-version N --operation KEY --wait`, with the usual project selection. Preserve the original request/key on transport failure. Review the latest complete source before adopting a new CAS baseline; no automatic force or retry is available.

Evidence: [Control/HTTP tests](../../internal/platform/workspace_write_test.go) plus existing observation regressions passed in 8.856 seconds; [Runner tests](../../internal/runner/workspace_write_test.go) cover native CAS, concurrent model writes, check invalidation, exact recovery and the no-effect barrier. Actual Worker integration and GUI evidence are separate. Remote Git writes, PTY/LSP/DAP, persistent cross-session drafts, full IDE capabilities, physical multi-host/TLS and production isolation remain pending.

Local UI and real worker evidence: [bounded verification](../quality/workspace-write-verification.md). Full platform and production acceptance remain in progress.
