# Unified tool confirmations and native Codex approvals

T-CODEX-HUMAN-REQUESTS-174 · suma 1.0.0 · 2026-10-31. Owners: permission domain, Runner, native protocol, workspace and integration. See the [contract](../../contracts/codex-human-requests.md), [architecture and SOP](../architecture/codex-human-requests.md), [interaction design](../design/codex-human-requests.md) and [scoped evidence](../quality/codex-human-requests-verification.md).

ACP and Codex now use the same platform permission domain, current identity, transactional receipts and original Runner/Pool authority. Native request types remain explicit. Codex approvals preserve exact typed RPC IDs, current thread/turn/item, source digests and worktree-relative visible effects. Distinct requests for the same item remain distinct.

The fixed Codex approval policy is `on-request` and belongs to the execution configuration digest. The supported responses are one-time `accept`, `decline` and `cancel`. Session grants, persistent write roots, network or rule amendments, incomplete effects and unsupported extensions cannot be approved. A one-time decision does not prove OS isolation or grant business acceptance.

Human waiting and persistence run outside the native protocol mutex. One stdin writer sends each exact response. The Runner durably records write intent before replying and acknowledges a complete write only after it actually succeeds. Cancellation blocks replies that have not started; partial writes and crashes remain unknown and are not replayed.

Human decisions, Runner observations, reply progress, native request cleanup and item results are separate facts. `serverRequest/resolved` does not mean the operation succeeded. Observations use the original assignment, machine scope, lease, fence, source and idempotency key. Repeated commands return their saved receipt snapshot. The observation outbox is replayed before results; permanent authorization failures retain evidence for reconciliation instead of pretending to synchronize.

The compact production panel exposes the immediate effects and permitted actions first. Technical details are collapsible. Historical attempts are read-only, background refresh preserves focus, and unknown submissions retain the exact original body/key. Chinese and English use paired domain catalogs; the catalog mechanism remains extensible.

This is one current 1.0.0 initial schema. The renamed source-digest columns are checked before any DDL; mismatched existing databases are rejected without migration, reset or backfill. Existing user data is preserved.

Focused checks, current installed CLI schema export, controlled protocol execution, representative UI and real vendor/model/production acceptance have separate evidence boundaries. The complete original 31 requirements, P0–P7 and 58 limited / 34 pending / 92 total scope remain active.
