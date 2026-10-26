# Team CLI runtime integration

Status: T-P4-021 is implemented locally, with focused native-process and Node-entry checks passing. Actual Node task execution, the final bilingual UI and the forward update of the existing preview have passed local verification. Vendor models, WorkBuddy and full team handoff still require actual acceptance. See the [contract](../../contracts/team-cli-runtime.md), [Chinese product specification](../product/team-cli-runtime.md), and [integration / AI SOP](../integrations/team-cli-runtime.md).

Suma now normalizes CodeBuddy Code stream-json and the DeepSeek Harness Multica v1 profile through the existing Go Runner, DAG, shared pools, private worktrees, durable journal, candidate transfer, independent tests and explicit task review. The tool settings guide provides secret-free local templates. A template, installed binary or successful probe is not an executed or accepted task.

Use the [configuration example](../../deploy/runner/team-cli.example.json) on a trusted execution node. Install and authenticate the CLI through its own normal flow, pin the repository and full base commit, probe it, then import the actual inventory and enable the profile. Harness additionally requires a compatible preinstalled Multica runtime profile. Suma neither installs it automatically nor forwards its platform Runner credential.

The example keeps three native-executable profiles. CodeBuddy and Harness also support direct Node entry mode: set `executable` to the installed `node` or `node.exe`, and prefix the existing fixed `args` with the actual absolute `.js`, `.mjs` or `.cjs` CLI entry. The entry must be a regular file, not a symbolic link, and at most 16 MiB. Suma does not invoke `.cmd` / `.bat` wrappers or a shell, discover npm entry locations, or install packages. Version, help and profile probes use the same entry prefix.

For example, replace the entry below with an existing local file before using this Harness fragment; it is not a runnable installation path:

```json
{
  "executable": "node",
  "args": ["D:/REPLACE_WITH_ACTUAL_INSTALL/cli.mjs", "--profile", "multica", "--stdio"]
}
```

The profile binds the Node executable SHA, entry file content SHA and complete normalized command configuration. It does not attest the entire imported module dependency closure. Tasks and models cannot choose the entry, working directory or flags. Only CodeBuddy and Harness expose the native / Node choice in the bilingual, collapsed-by-default setup guide; an empty or relative Node entry disables template copying.

The Harness probe is a single versioned stdout JSON frame; stderr is bounded separately. Execution uses a host-owned request identity and working directory, duplex stdin, a unique whole-run result, bounded cancellation and known process-tree shutdown. CodeBuddy keeps its local permissions; pending permission requests and background activity cannot count as success. No resume or permission-bypass flags are silently added.

The current evidence uses real local process fixtures and isolated Git worktrees, not vendor model calls. WorkBuddy is a separate product with its own connector integration; CodeBuddy support does not prove WorkBuddy support. Full team handoff, repository integration, automatic knowledge / memory / skill refinement, SSO / A2A, installation and distributed capacity acceptance remain part of the active platform objective.
