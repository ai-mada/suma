# Workspace Git review and native execution limits

Status: T-P4-035 / suma 1.0.0. Owner: platform integration. Focused checks and the isolated real Git/Monaco journey have passed; final disabled-workspace readback and primary preservation have also passed, as recorded in the [verification report](../quality/workspace-git-verification.md). The complete P0–P7 platform and requirements 1–22 remain active.

The project workspace now has Files and Changes views. Changing views preserves the editing model and unsaved draft. The read-only Monaco comparison shows HEAD→index for Staged and index→disk for Working tree. Changes include additions, deletions, renames and untracked files. A staged deletion followed by a new file at the same path retains both facts in one record. Restricted paths remain undisclosed; binary files do not expose source bytes.

In the isolated ordinary repository, actual UI review confirmed staged1→2 and working2→3 while draft9 remained unsaved. Returning to the same editor retained draft9. One native Monaco Ctrl+S saved it, and refreshed Git showed2→9. Chinese/English, tab keyboard navigation, browser refresh and the disabled edit action for a deleted file were verified. The source repository, fixture HEAD and index remained unchanged; the only intended disk write was the owned counter file.

Git reads use the existing current project authority, fixed command arguments, bounded time/output and restricted filesystem access. They do not stage, commit, merge, run hooks, fetch remotes, dispatch development or call a model. Current disk observations remain separate from sealed candidates, independent tests and human acceptance.

The Runner now carries validated `max_steps` and `max_output_tokens` into native SDK stdin (`max_tokens`). Output defaults explicitly to4096. Ordinary steps are1–16, default8; already authorized typed repair steps are1–128, default64. The repair stream validator matches its1024-token per-call proof. Invalid values fail before workspace/MCP/main-process startup. Focused actual-child stdin and stream checks passed; this does not make writable models independently selectable or priced.

CLI reads use the same API: `suma --project PROJECT workspace git`, or `workspace diff --path src/file.ts --source index`. The [contract](../../contracts/workspace-git.md), [product](../product/workspace-git.md), [architecture](../architecture/workspace-git.md), [AI SOP](../operations/workspace-git.md) and [model-binding design](../architecture/035-model-selection-design.md) describe exact boundaries.

Linked worktrees with external shared metadata require an explicit project repository grant. Remote Runner workspaces, complete PTY/LSP/DAP, durable drafts, localized built-in editor menus, real providers and distributed/production acceptance remain subsequent work.
