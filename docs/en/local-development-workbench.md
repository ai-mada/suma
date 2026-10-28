# Local development workbench

Status: **1.0.0 / T-DESKTOP-107, in progress**, 2026-10-28. Requirement 23 covers a complete local professional workbench. The full P0–P7 plan and requirements 1–25 remain active. See the [main architecture and SOP](../architecture/local-development-workbench.md), [native directory contract](../../contracts/desktop-workspace.md), [desktop startup guide](../deployment/desktop.md) and [delivery plan](../delivery/implementation-plan.md).

Electron now implements a single-purpose native directory picker alongside its existing local-service lifecycle. The shared IDE asks the user to confirm the selected folder before submitting a project binding. The signed selection belongs to the original user, organization, project, path and operation. Control independently checks current authority, the binding version and the exact acknowledgement. Selection alone grants no role, execution authority or acceptance.

**The actual Electron launch currently fails:** its GPU child exits with `0xC0000135`, a DLL-loading failure status whose missing dependency has not yet been identified. The real native-picker-to-IDE journey remains unverified. Passing Node and Web checks does not establish desktop startup, installer, signing or production acceptance.

## Current capability and remaining requirement 23 scope

| Area | Current implementation | Remaining work or acceptance |
| --- | --- | --- |
| Desktop lifecycle | One Electron instance starts a private loopback Go service, waits for health, and confirms child exit during shutdown | Resolve the current launch failure; verify real startup, shutdown and restart; complete diagnostics, recovery and explicit remote connections |
| Native project folders | Sender-checked native selection, explicit confirmation, scoped signed grant, binding CAS and exact receipt checks; current root identity checks are shared with workspace operations | Real Chinese/English choose, cancel, bind, lost-receipt and restart journeys; target-system directory and permission acceptance |
| Project workspaces | Existing projects, task/attempt isolation, baseline references and on-demand directory paging | Recent workspaces, complete multi-root management, richer branch/task context and device synchronization |
| Editor and Git | Monaco text editing, draft protection, hash-checked saves, Files/Changes views and read-only comparisons | Multi-tab/split editing, complete file operations, global search and full built-in editor localization |
| Terminal, language and debugging | Bundled Monaco language workers and existing command execution nodes | Actual permission-scoped PTY, LSP, DAP, previews and port access; workers and command nodes do not provide these services |
| Tools, models and skills | Existing bounded native/Codex/OpenCode execution and approved capability contracts reuse the platform's one scheduler | Complete discovery, installation/configuration, model/skill choice, interactive input, durable sessions and provider/billing acceptance |
| Intelligent development | Controlled planning, expert dispatch, isolated worktrees, candidate handoff and existing repair/baseline flows | Complete persistent autonomous workflows, feedback revisions and cross-tool context consistency |
| Quality and delivery | Existing exact-code independent tests, review and acceptance records remain separate from execution outcomes | Complete merge policy, production CI/deployment, operational feedback and remote recovery |
| Extensions, knowledge and synchronization | Versioned assets, authorized retrieval and bounded MCP integrations | Executable plugin isolation, complete knowledge/memory indexing, A2A coverage and offline/device synchronization |
| Distribution | Current package scripts declare Windows NSIS/portable, macOS DMG/ZIP and Linux AppImage/deb targets | Current 1.0.0 artifact validation, installation/update/uninstall, code signing/notarization and actual OS coverage |

## Local data and interaction boundaries

The desktop launcher pins SQLite to `<Electron userData>/data/suma.db` and keeps its private signing key in the same dedicated application-data area. Inherited database, workspace-root and server-address settings cannot silently redirect this local mode to shared infrastructure. Packaged resources supply the control program and Web assets. Development-only overrides require all three absolute paths—user data, control program and Web assets—inside the repository's `.cache`; packaged mode ignores them.

The renderer retains sandboxing and context isolation. The preload bridge exposes platform/version and directory selection, without general IPC, filesystem, command, cookie or key access. A selected project folder stays in its original location. Current workspace reads and writes must retain project authority and the root's recorded identity; a broken native proof cannot fall back to a broader root. Directory availability says nothing about tool or model health.

An unconfirmed binding retains its exact original caller, request and operation in bounded application memory. The user explicitly checks that operation; the UI never automatically submits it again on returning to a view. Navigation is guarded and closing or refreshing prompts the user, but recovery is not promised after a page or process restart. Account, organization, project and API changes hide the old selection and reject late results. A definite conflict requires an explicit metadata refresh and a new selection; a new expired selection must be chosen again. An original successful receipt remains immutable. Before showing a current binding, the IDE reads current metadata separately; a different binding is reported explicitly. If that read fails, it retains the acknowledged operation and offers a metadata refresh without another POST. Unsaved or unconfirmed file changes disable rebinding.

Platform text, native dialog labels and known server diagnostics support Chinese and English. User names, paths, source, tool output and unknown system diagnostics retain their original text. Browsers keep the existing server-configured-root binding flow and do not claim native-picker availability.

## Verification boundary

The local stage receipts under `.cache/e2e/desktop-107` distinguish Node/VM checks, Web behavior and the failed Electron launch. Node checks cover the signing key, signed scope, sender/frame/origin, cancellation/concurrency and local configuration. The Web receipt records 10 affected cases, 2 subsequent focused checks and successful final type checking. These checks do not validate a real native dialog or complete requirement 23. The [current delivery status](../delivery/status.md) remains the source for integrated progress and open acceptance work.
