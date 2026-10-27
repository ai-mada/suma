# Outbound read-only workspace observations

Status: T-P4-037 / 1.0.0, bounded implementation, 2026-10-27. Control owns authorization and durable requests; Runner owns active native handles and outbound receipts. The [contract](../../contracts/workspace-observation.md) owns wire behavior and limits; the [architecture](../architecture/remote-workspace-broker.md) owns implementation structure. GUI and production acceptance remain separate.

An authenticated browser session or project API credential selects an exact, currently running native agent attempt. Control freezes the project, original credential reference, run, node, attempt, fence, worker and approved tool hash. The active Worker polls through its existing outbound HTTP connection, reads through its already opened `os.Root` workspace handle, and returns a bounded `list` or `read` result. Control never opens the Runner host path. No workspace MCP URL, loopback port, bearer or model secret reaches the browser.

The active host supplies a separate `workspace_policy_sha256` when claiming a request; it is not a renamed tool hash. Pool access additionally requires the original immutable dispatch, current machine credential, active Pool and the same currently approved bilateral grant ID and version. Permission to drain old execution facts after grant withdrawal does not authorize a fresh file read.

Requests live for 60 seconds and observation leases for 10 seconds. Read-only work may be leased again before expiry with a strictly higher observation fence. Old fences, changed policies and expired leases cannot overwrite results. Lost acknowledgements replay the original result and hash rather than reading changed files again. Observation failures never complete a DAG node, retry a model, change a task or create acceptance evidence.

The original credential must still have current project access when claiming, returning or viewing an observation. Another session for the same user cannot read the original request. Closure, attempt changes, revocation and expiry stop disclosure. Metadata polling repeats the checks without source text. Control expires separately stored source results in bounded batches; the private Runner recovery journal has its own retention and later-checkpoint pruning behavior.

Public routes under `/api/v1`:

| Route | Purpose |
| --- | --- |
| `GET /workspace/targets?project_id=…` | Current native targets within the bounded discovery window |
| `POST /workspace/observations` | Explicit scoped `list` or `read` request with a stable operation ID |
| `GET /workspace/observations/{id}?project_id=…` | Current authorization and bounded result |
| Same route with `metadata=true` | Current authorization without repeating source text |

Runner uses `POST /runner/workspace-observations/claim` and `/result`; Pool uses corresponding `/pool-runner/` routes with its original full assignment. Results use `observation_result`, distinct from DAG results. Machine acknowledgements contain request ID, lease fence and result hash, with no source text.

A fully authorized poll returning `lease:null` has no durable effect or operation receipt. Reusing that operation ID may claim work that arrives later; an empty response does not freeze future queue state. Actual leases, re-leases and result acknowledgements retain their exact idempotent recovery behavior.

Native path, exact-case, sensitive-file and sensitive-content checks remain active. Source files are limited to 1 MiB, returned text to 48 KiB, lists to 200 entries, and the serialized result structure to 64 KiB. Hashes describe original file bytes even for a returned line range. Discovery scans at most 201 running records and displays 50 eligible targets; the truncated flag matters. Pool dispatch lookup also has an explicit bounded scan and fails closed above it. These are implementation limits, not scale acceptance.

CLI uses the same API:

```text
suma --project PROJECT workspace targets
suma --project PROJECT workspace observe --file observation.json --wait
suma --project PROJECT workspace observation REQUEST_ID --wait
```

[Control tests](../../internal/platform/workspace_observation_test.go) cover authority, expiry, fencing, exact acknowledgement recovery, metadata and unchanged execution state. The [Worker integration entry](../../internal/platform/workspace_observation_runtime_test.go) uses a real native child, a separate attempt worktree and outbound HTTP with no model calls or formal task acceptance. Remote Git, writes, terminals, PTY, LSP/DAP, historical workspace recovery, physical multi-host/TLS deployment and production isolation remain separate work. The full platform goal remains active.
