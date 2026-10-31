# OpenCode SDK quick connection

T-OPENCODE-QUICK-CONNECT-177 · suma 1.0.0. Status: scoped source, actual SDK resources and representative synthetic components verified; real native/model integration remains open. See the [contract](../../contracts/opencode-quick-connect.md), [existing SDK execution contract](../../contracts/opencode-sdk.md), [product flow](../product/opencode-quick-connect.md), [architecture and SOP](../architecture/opencode-quick-connect.md), [interaction design](../design/opencode-quick-connect.md) and [verification](../quality/opencode-quick-connect-verification.md).

This stage connects the existing `opencode-sdk` / `opencode-sdk-v1` runtime to the original tool onboarding flow. The SDK and native server remain pinned to **1.19.4**. It adds no dispatcher, model service, or database.

In the desktop app, inspect the installation, explicitly enter the provider ID, fixed model, service URL, and optional credential environment reference, then connect. The main process resolves the native executable, trusted SDK assets, and target Node. Missing or unsupported resources prevent new Worker issuance. A browser provides desktop or server-Runner instructions rather than claiming it can scan the browser's machine.

The renderer cannot provide executable paths, Node, scripts, manifests, arbitrary arguments or environment values, or raw credentials. The private configuration uses the existing `OpenCodeConfigurationDigest`: policy version1, SDK version1.19.4, and the complete original provider structure with Go JSON encoding. Native bytes, Node bytes, manifest bytes, and the full SDK/plugin payload are verified separately.

The original authenticated project/workspace registration, encrypted journal, Worker, and stable operation body/key remain authoritative. An uncertain registration is reconciled through that original operation. Reconnection preserves its configuration and identity. A different configuration requires a confirmed stop and no unresolved operation.

Connected status requires inventory observed after this Worker started. Inventory matches the **Node SHA**, runtime version `suma-opencode-runtime 1.0.0`, and exact `opencode_execution` capability. The connection's six required metadata fields are `model`, `tool_version`, `executable_sha256`, `configuration_sha256`, `provider_id`, and `endpoint_origin`; its version/SHA describe the native server. Provider/model/origin come from the saved configuration, never the current draft. Full URLs and secrets are not returned.

New tool profiles are registered paused and enabled through a separate explicit action. Tasks retain the existing `fixed_opencode` selection, isolated worktree, trusted host tools, candidate delivery, independent exact-commit tests, and human acceptance. There is no new model catalogue discovery or automatic model selection.

Packaging hooks use `resources.opencode_runtime` under `control/opencode-runtime`, alongside the control binary. They prepare and validate the existing manifest, SDK/tool/plugin assets, target Node22+, and its license. A missing target distribution fails; host binaries cannot stand in for another platform. Format checks alone do not demonstrate execution on that target.

Chinese/English states distinguish installation, configuration, reconciliation, connection, registration, and enablement. Usage remains **observe-only**: asset checks and configuration acknowledgements prove neither provider identity, model success, billing, nor enforced budgets. Integrated UI, real native/vendor execution, and complete packaging acceptance still require separate evidence. The original31/P0–P7 and58/34/92 platform scope remains active.
