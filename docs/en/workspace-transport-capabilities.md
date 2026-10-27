# Current runner workspace capabilities

Status: bounded local implementation, T-P4-039 / 1.0.0. Owner: Control, Runner and workbench. [Contract](../../contracts/workspace-transport-capabilities.md), [architecture and SOP](../integrations/workspace-transport-capabilities.md), [verification](../quality/workspace-capability-verification.md).

Native tool approval and outbound file transport are separate facts. Current tool observations carry exact read and save protocol versions. Save requires read. Only a successfully probed native workspace configuration can advertise them; there is no administrator toggle. The approved profile and its digest do not change.

Control checks the current authenticated inventory, project and attempt scope before admission and content redelivery. Removing the declaration prevents new work without hiding an original command receipt or discarding an already performed effect. This is not remote binary attestation or connection-health verification.

The existing collapsed observation section displays raw protocol versions or “Not declared”. Import accepts public native/model/purpose capability fields, rejects private configuration and credential URLs, and submits only the chosen profile for approval. Server-side current inventory and exact deployment validation remain authoritative.

The existing local preview has been upgraded with consistent backups. Local evidence does not establish production, physical multi-host deployment, vendor model execution, complete IDE support or completion of the P0–P7 / requirements 1–22 platform scope.
