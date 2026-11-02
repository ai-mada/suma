# Multi-project Pool configuration preparation

Version 1.0.0. Stage T209; scoped evidence is recorded in the [verification report](../quality/pool-config-preparation-verification.md).

Open **Prepare project configuration** on an exact active Pool Runner. Select the target shell and a new absolute output file, then add project rows with tenant/project IDs and existing local workspaces. Reference an existing private command file when tools are needed; an empty reference only prepares the workspace. Repository, full commit, worktrees and optional model/external configuration files are advanced fields.

The browser keeps only an in-memory draft and generates a preparation command. It does not upload configuration contents, credentials or project data, and copying does not execute it. On the target machine, `suma-runner --prepare-pool-config NEWFILE` reads bounded private stdin and existing local files, validates the original Pool configuration, and exclusively creates a new file. Existing files are never overwritten. The safe report contains identity, counts and the final byte digest, without private paths or contents.

Preparation does not authenticate a machine, create a project Grant, enable tools or start a Worker. Continue through the original paired identity, current session/Grant checks and explicit tool inspection/enablement. Changed execution configuration needs a new state directory; unresolved operations must be reconciled through the original workflow.

When a row switches to a different tenant/project, its workspace and six other private local references are cleared and the old preparation command becomes invalid. Reselecting the same scope or switching only to manual entry preserves its inputs. Other rows are unaffected. This also applies when tenant/project IDs are edited manually.

See the [closed contract](../../contracts/pool-config-preparation.md), [architecture and maintenance SOP](../architecture/pool-config-preparation.md), and [interaction design](../design/pool-config-preparation.md). The complete 31-requirement/P0–P7 platform remains in progress.
