# Inspect the Runner's local execution journal

[中文](../agents/runner-journal-inspection.md) · [Exact contract — Chinese original](../../contracts/runner-journal-inspection.md)

Delivery task: T-DELIVERY-112. Product version: 1.0.0. This local command shows recorded results, progress and artifact checkpoints while preserving the original execution state.

```text
suma-runner --inspect-state --state .cache/runner --language en-US
```

For a pool runner, select one actual slot:

```text
suma-runner --inspect-state --state .cache/runner/pool/slots/0 --language en-US
```

No Runner token, model account, control-plane URL or tool configuration is required. Inspection does not initialize a directory, take the runtime write lock, change a result, acknowledge delivery or restart a task. Do not combine it with execution options.

Read `status` first. `available` means the local journal was read; `missing` means it does not exist. `invalid`, `unreadable` and `over_limit` indicate invalid content, an unsafe or inaccessible input, and the 16 MiB read limit. A failed read does not emit private payloads or a partial summary.

`counts` summarizes locally pending results, unknown outcomes, unacknowledged progress, inbox entries without results, and artifact or candidate checkpoints. `items` lists at most 100 entries. `complete:false` explicitly marks an incomplete item list. An opaque entry reference distinguishes local records; it is not an operation key or authorization token.

Only `journal.json` is covered. Claim checkpoints, workspace sidecars, current server acknowledgements, leases, actual rejection reasons and runner health are not inspected. An `acked` flag is a local record; a pending item does not prove server rejection. Output excludes log text, commands, configuration, paths, credentials and execution payloads.

Keep the original private state when outcomes or delivery are uncertain, then reconcile the original operation through the existing authorized workflow. Inspection cannot drop events, set ACK flags, clear state or rerun work. The current delivery record states the actual verification scope.
