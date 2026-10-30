# Quick tool connection

T-TOOLS-151, suma 1.0.0. **Add tool** reuses the existing authorized node inventory and tool-profile commands. See the [contract](../../contracts/tool-quick-connect.md) for exact boundaries; this guide is not acceptance evidence.

## Configure from a registered node

1. Open **Tool management → Execution tools → Add tool** and choose an implemented execution entrypoint.
2. The page reads the current project's private and Pool nodes. It selects the sole active node automatically; choose explicitly when several exist. An active registration does not prove that the node is online.
3. The page reads that node's real inventory. A sole matching available configuration and its suggested name are selected automatically. Choose among multiple configurations and edit the name as needed.
4. **Register configuration** creates a paused profile. Review the node and permissions, then use the separate **Enable tool** confirmation. Dispatch still revalidates current authority, fingerprints and fresh inventory.

Unreported, expired, unmatched and failed reads have explicit next steps. **Read again** reads the existing report; it does not run a remote probe or extend the original 60-second lifetime. **Advanced setup** retains local configuration instructions and the original strict inventory-import flow.

## Check this computer's installation

When no active project node is available, the desktop app can check Codex, CodeBuddy Code or DeepSeek Harness through **Check local installation**. A normal browser cannot inspect the user's computer through the server.

The desktop uses its fixed packaged control executable in a separate read-only mode. It checks bounded absolute PATH entries and known package metadata, then reuses fixed version, help and protocol probes. It does not install software, execute shell shims, log in, request models, start tasks or read business credentials. The renderer receives installation facts, with executable details available on demand; local command definitions are removed at the IPC boundary.

Finding an installation or a supported launch method does not connect a Runner, register a profile, enable dispatch or confirm supplier authentication. WorkBuddy account authorization remains separate from CodeBuddy CLI installation.

## Recovery and remaining work

Read failures may be retried. An uncertain registration or enable operation keeps its original scope, request, version and operation ID for receipt reconciliation; it must not silently create a second operation. Switching account, organization, project or tool suppresses late results.

Cancellation notifies the Go probe and waits for the child process to close, with bounded escalation. Unconfirmed termination retains the busy state and returns no installation facts.

Managed local Runner credentials, an encrypted local journal and automatic Runner startup are not implemented in this stage. The next step should keep the one-time credential in a private local component, record its original issuance operation before sending, and reconcile unknown receipts before any further issuance. The current public receipt cannot recover a lost token. The browser should display safe node facts and explicit approval, without copying secrets.
