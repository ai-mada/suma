# Managed preparation for installed command-line tools

suma 1.0.0 · T-MANAGED-CLI-PREPARATION-225. See the [contract](../../contracts/managed-cli-preparation.md), [architecture](../architecture/managed-cli-preparation.md), [interaction design](../design/managed-cli-preparation.md), and [verification scope](../quality/managed-cli-preparation-verification.md).

The existing remote scan can continue into configuration preparation, original inventory acknowledgement and paused registration. It reuses the same maintenance owner, journal, permissions and task dispatcher.

- Qoder and Cursor ACP require a fixed model and the original credential-variable policy.
- Claude CLI requires an explicit fixed model. It retains the existing CLI identity; credential references are not accepted here.
- Codex JSONL, CodeBuddy and dsh retain the exact detected native configuration. Both configuration strings must be empty; this action does not override their model or account.

The continuous Runner must explicitly enable both installed discovery and `--allow-tool-preparation`. Preparation defaults to off and does not grant task execution. The browser supplies no executable path, shell command, install URL, dependency tree or secret value. The machine consumes its original private scan receipt, pins native files and the required Node installation scope, and rejects changed or unsupported sources.

Prepared configuration is not yet complete: the original associated inventory transaction must acknowledge the exact generation and profile. Lost acknowledgements recover the original operation and body. Unknown effects are reconciled rather than retried under a new identity. A complete but unavailable maintenance configuration may be saved paused. Enabling is separate; actual dispatch still checks current availability and budget.

SDK resource discovery and Codex App Server provider configuration remain separate work. Owned metadata-only fixtures do not prove a vendor login, protocol session or model request. Physical multi-host, operating-system/database/installers/scale/production acceptance and the original full-platform plan remain active.
