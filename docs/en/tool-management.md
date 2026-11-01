# Tool management: connection and maintenance

Status: scoped implementation and verification complete. suma1.0.0, T-TOOL-MANAGEMENT-193. Owner: integration agent. See the [contract](../../contracts/tool-management.md), [PRD](../product/tool-management.md), [architecture and AI SOP](../architecture/tool-management.md), [design](../design/tool-management.md) and [verification](../quality/tool-management-verification.md). The original31-requirement/P0–P7 objective remains active.

One **Connect tool** entry coordinates Quick detection, Advanced configuration and Doctor guidance. They retain the current actor, organization, project and exact target through scoped public drafts. Drafts do not contain credentials, private process paths, arguments or stale inventory presented as current detection. Existing exact matches can be opened rather than registered again.

Native local scanning, authorized node-state reads, inventory reports, valid configuration, dispatch eligibility and model health are separate facts. A browser refresh is not a native scan, installation or successful repair. Missing fresh information points to the selected target; it does not silently switch workers. SDK readiness cannot be inferred from another CLI or a generic desktop connection.

The original list GET accepts `state`, `query`, `limit` and `cursor`. Default `active` includes enabled and paused profiles. Revoked history is explicit. Literal Unicode search covers public name, command ID, worker ID, adapter and protocol only, with a120-byte UTF-8 input limit. Current authorization precedes domain normalization; cursors bind actor, organization, project and normalized filters. Changing scope or filters clears old pagination and rejects late results.

Configuration changes prepare and report a new immutable configuration on the target machine, then reuse an exact profile or register it paused before explicit enablement. **Pause** uses the original pause action. **Remove connection** permanently revokes future eligibility while preserving history and evidence. There is no physical DELETE, configuration PATCH, new API path or business table.

Optional diagnostic impact covers this project's running, paused and cancelling runs only: at most200 runs,2000 nodes and five public reference samples. It matches exact profile ID and SHA through the original binding. `complete` applies only to this scope; bounds produce `partial`, unavailable sources produce `unknown`. Observed counts are not global totals. Pending nodes may honestly have zero attempt/fence and an empty worker. Draft plans, historical runs and unknown external processes are excluded.

Revocation does not prove existing processes stopped. A diagnostic read does not grant cancellation or repair authority. Mutations retain current permissions, expected versions, stable operation IDs and original receipts. Unknown effects require reconciliation of the original operation, preserving drafts rather than blindly issuing new requests.

Chinese and English share concise states, keyboard behavior and narrow-screen flows. Source implementation, focused checks, actual HTTP, representative interface validation and production acceptance remain distinct evidence. This stage does not declare the full platform complete.
