# Inspect workspace operations

Status: current1.0.0 workbench. Owners: execution and workbench. Requirements13/23. [Contract](../../contracts/workspace-operations.md), [verification](../quality/workspace-operations-verification.md), [中文](../development/workspace-operations.md).

1. Open the exact node and attempt under Development → Runs.
2. Expand Workspace operations within tool facts. The collapsed summary keeps the page compact; details show actual reads, changes, checks, merge operations and sealing.
3. Filter failures or unknown outcomes. Inspect the request/policy digests, generation and timestamps. Local workspace sequence and platform receipt sequence represent different orders.
4. Open original check details only when they still match that effect, generation, policy and result. An unavailable detail is never replaced with a later check sharing the same name.
5. Use the existing attempt history pages for earlier records. The current window, saved history watermark and local capture coverage are separate facts.

A completed check must also be assessed using its exit code, stopped flag and unknown outcome. Changes or sealing do not prove independent testing or human acceptance. Reusing an original receipt does not execute its effect again. Unknown effects require reconciliation; refreshing must not repeat them.

Missing capture metadata means not provided, rather than zero operations. A network failure or revoked permission can leave local records unacknowledged. Loaded history pages do not prove complete local capture. Switching projects, nodes or attempts clears the previous protected view.
