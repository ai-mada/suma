# Tool reinspection: exact target and a new report

Status: scoped implementation and verification complete. suma 1.0.0, T-TOOL-REINSPECTION-194. Owner: integration agent. See the [contract](../../contracts/tool-reinspection.md), [PRD](../product/tool-reinspection.md), [architecture and AI SOP](../architecture/tool-reinspection.md), [interaction design](../design/tool-reinspection.md), [reference adoption](../integrations/multica-tool-reinspection.md) and [verification](../quality/tool-reinspection-verification.md).

Reinspection is a persisted, authorized request for the exact registered tool and execution worker. Reading connection status or an older fresh inventory is a separate operation and cannot complete the new request. A local desktop status callback must not replace a selected remote worker; changing the target requires an explicit user action.

The original outbound Worker consumes the request through its existing inventory owner, recovers any older pending report first, and performs a new probe of its registered configuration. Its durable journal binds the original claim and inventory operation. The control plane persists the associated inventory and request result in one transaction. Lost responses recover the original operations rather than creating new requests or replaying unknown effects.

Current management permission, profile version and complete configuration, worker identity and applicable shared-pool authority remain required. Requests accept no arbitrary executable, script, credential, installation or privilege change. Inspection completion does not mean configuration matching, model health, task success or acceptance. Existing enable, pause, revoke and process-control workflows retain their own authority.

The interface presents the exact target, current request and one next action. Waiting and unknown results remain explicit, without invented progress or silent retries. Empty filtered pages describe the filter scope rather than claiming that no tools have ever been connected. Chinese, English, narrow layouts, keyboard use and authorization changes share the existing components.

Focused behavior checks, actual HTTP/SQLite/Worker/Journal execution and representative interface verification are recorded separately. Real vendor accounts, physical multiple hosts, database environments and production acceptance require their own evidence. The original 31 requirements and P0–P7 platform objective remain active.
