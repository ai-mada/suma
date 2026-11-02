# Claude SDK desktop quick connection

Status: T-CLAUDE-SDK-CONNECT-201: scoped implementation and verification complete; see the [evidence report](../quality/claude-sdk-quick-connect-verification.md). Suma 1.0.0. The full 31-requirement/P0–P7 platform goal remains active. See the [contract](../../contracts/claude-sdk-quick-connect.md), [requirements](../product/claude-sdk-quick-connect.md), [architecture and AI SOP](../architecture/claude-sdk-quick-connect.md), and [design](../design/claude-sdk-quick-connect.md).

Reuse the existing desktop connection flow: passive native CLI detection, explicit fixed model and credential environment reference, trusted SDK resources, the original managed Worker, fresh exact inventory, paused registration and separate activation. CLI presence does not establish SDK assets, model authentication or task success.

The main process owns private paths and the complete configuration digest. Renderer input contains only the detected native digest, model and environment name. Node and native CLI hashes remain distinct. SDK 0.3.314 and reviewed CLI 2.1.314 retain the original observe-only budget and SDK-init model evidence contract.

The trusted local connection returns the original environment variable name, never its value. Only complete connected facts matching the current model, environment name, native digest and scope establish proof. This supports original recovery after a lost acknowledgment or reload; matching model and CLI alone cannot infer a changed environment reference. Partial starting facts remain progress. Unknown operations retain original recovery without a new setup. Changed drafts and scope invalidate old proof. No automatic install, login, activation or process-stop claim is added.

Packaging keeps Node and its original license outside the closed SDK host asset directory. Native CLI comes from an existing user installation. Actual provider accounts, other operating systems, physical multihost, scale, complete installers and production acceptance remain separate requirements.
