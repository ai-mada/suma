# Native Codex quick connection

T-CODEX-QUICK-CONNECT-175 · suma 1.0.0 · 2026-10-31. See the [contract](../../contracts/codex-quick-connect.md), [product requirements](../product/codex-quick-connect.md), [architecture and SOP](../architecture/codex-quick-connect.md), [interaction design](../design/codex-quick-connect.md) and [scoped verification](../quality/codex-quick-connect-verification.md).

The existing native Codex execution now has a direct managed-connection path: detect the installation, explicitly configure the provider, read the real catalogue, connect the original Runner, register the new inventory in a paused state, then explicitly enable it. Users no longer need to write a commands file for this path. Task model and reasoning selection still use the existing formal catalogue.

The desktop main process owns installation paths and credential values. The renderer supplies only scoped intentions, nonsecret provider settings and exact installation/probe references. Private CLI input is bounded and closed; catalogue discovery runs before service/database initialization and cannot accept a development goal, worker credential or arbitrary command.

The current configuration digest includes version3 and `on-request`. A catalogue must match its exact configuration, and connected status requires inventory observed after the current Runner started. Configuration, catalogue availability, connection, registration, enablement, execution and human acceptance remain separate facts.

Unknown operations retain their original body/key. Reconnecting does not replace unresolved configuration. An explicitly stopped node can adopt a newly confirmed configuration while preserving its original machine registration identity. Secrets are not copied from user login files or written into the renderer or journal.

Chinese and English share the extensible catalog mechanism. The compact connection card explains missing runtimes, unsupported launchers, incomplete configuration and reconciliation with a clear next step. It does not install tools, relax isolation or silently enable dispatch.

Scoped source, real local probes, controlled interfaces, vendor/model use and production acceptance have separate evidence boundaries. The complete original31/P0–P7 platform scope remains active.
