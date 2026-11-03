# Managed remote tool preparation

Suma 1.0.0 · T-MANAGED-TOOL-PREPARATION-224. The [current contract](../../contracts/managed-tool-preparation.md) defines the wire format; the [architecture and recovery guide](../architecture/managed-tool-preparation.md) describes ownership and persistence. The full 31-requirement / P0–P7 programme remains active.

## What is supported

Automatic preparation currently supports **Qoder ACP** (`qoder-acp`) and **Cursor CLI** (`cursor-cli`) through the existing typed ACP factory. The other seven catalogue entries retain their existing manual setup. A catalogue entry does not imply an automatic factory.

The feature uses the original continuous normal Runner or Pool maintenance channel. It does not install or update software, run arbitrary shell commands, log in, receive credential values, initialize ACP or call a model.

## Enable the local capability

Keep the original paired identity, control address, local configuration and Journal. Add both explicit local options to the appropriate continuous startup command:

```text
--allow-installed-tool-discovery --allow-tool-preparation
```

`--allow-tool-preparation` defaults to `false` and requires installed discovery. Use `--maintenance-only` when the node should perform maintenance without claiming development tasks. This feature does not enable `--allow-exec`; one-off `--report-tools` does not declare preparation capability.

For Pool deployments, keep the original Pool identity/configuration and project grants. Each tenant/project scope has its own durable head and claim gate, shared by all slots for that scope. Pool management permission does not grant project tool-management rights. Local state must remain available across restarts.

## Connect an installed tool

1. Select the exact node and run the original installed-tool scan. The node retains the private executable and dependency pins; the browser receives only public metadata.
2. For a supported, configuration-required result, choose a fixed model, enter the credential environment-variable **name** where needed, and explicitly confirm this configuration write. Qoder permits an empty reference; Cursor retains its required separate credential-reference rules.
3. Wait for preparation **and** inventory confirmation. The Runner creates a new immutable command generation; `prepared` is not `completed`.
4. Return to Quick Connect with the exact tool, worker and Profile SHA. A current read must still match that selection. Save the original tool profile as **paused**; enabling it remains a separate, authorized action.

A maintenance-only node may report a complete profile with `Available=false`. Saving a paused configuration does not make that node executable, available or authorized for dispatch. The original enable and task-start checks remain authoritative.

## API scope

| Caller | Original channel extension | Purpose |
| --- | --- | --- |
| Browser | `POST /tool-preparations` | Explicit preparation request with scan/version, expected generation, model/reference, write acknowledgement and stable operation key |
| Browser | `GET /tool-preparations` and `GET /tool-preparations/{id}?project_id=…` | Read current scope-qualified status; preserve the original operation when its outcome is unknown |
| Normal machine | `/runner/tool-preparations/claim` and `/results` | Consume and reconcile through the existing authenticated maintenance owner |
| Pool machine | `/pool-runner/tool-preparations/claim` and `/results` | The same protocol with the original tenant/project/runner scope |
| Inventory | Original tool-inventory operation with `Preparation` | Atomically persist the exact generation/profile and complete the preparation; echo the exact binding in the ACK |

Protocol: `suma-managed-tool-preparation-v1`. Browser configuration contains exactly `model` and `credential_env_ref`; it cannot supply an executable, path, argument list, output file or secret value. Current identity, project permissions, Pool grants, source and generation are checked on the server.

## Recovery and operational limits

Keep the original Journal and state directory. Lost responses replay the same nonce, request body, key and fence. They do not rerun preparation or native probing. Do not replace a key, overwrite a generated file or remove state to force progress.

Restart recovery verifies the canonical authenticated scope using the head's original claim, then checks the base digest, immutable manifest, local acceptance receipt, artifact bytes, every managed executable SHA and Cursor package closure before publishing a snapshot. Only generated ACP deltas are persisted; original manual environment values are not duplicated into the new state. In-flight attempts keep their old immutable snapshot.

An unknown effect stays blocked without trustworthy evidence. `closed_unknown` records a reconciled stop, not a rollback, successful preparation or dispatch permission. Recovered inventory acknowledgements retain their original timestamp; a separate current observation restores freshness. Use the original reconciliation or node isolation/re-pairing path when evidence is unavailable.

## Verified boundary

One controlled Windows journey used real loopback HTTP, SQLite and the embedded original ordinary Worker: installed scan → protected static ACP preparation → durable generation → atomic inventory confirmation → one paused profile through the original HTTP API. A committed preparation response and a committed inventory response were each lost once; both recovered with the original body. Two owned native processes ran only `--version`, three immutable artifacts matched, and no enable, business dispatch, ACP initialization, vendor account or model call occurred.

The reused 4 KB version-only executable was an owned fixture, **not a Qoder vendor installation or protocol acceptance**. Actual Pool machines, multiple machines, other operating systems/databases, the other seven automatic factories, vendor execution and scale acceptance remain follow-up work. This stage does not complete the whole platform.
