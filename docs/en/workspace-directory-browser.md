# Local workspace directory browser

Status: 1.0.0 / T-WORKSPACE-106, bounded local verification passed. See the [evidence](../quality/workspace-directory-verification.md). Owners: Control, workbench and integration. See the [wire contract](../../contracts/workspace-tree.md), [architecture](../architecture/workspace-directory-browser.md) and [full delivery plan](../delivery/implementation-plan.md).

The CLI exposes the same scoped read as `suma --project PROJECT workspace files [--directory src] [--limit 100] [--before CURSOR]`; it fetches only one page.

The IDE loads binding metadata first and reads a single directory on demand. It no longer recursively loads up to 1,000 files when opening the workspace. A compact root/breadcrumb row, parent action, directory/file buttons, loaded-entry filter and next-page action share the existing sidebar. Browsing and refreshing directories preserve the current editor draft and original save request; Git review keeps its existing tab.

Directory responses include relative names, entry types, an explicit page limit, a metadata snapshot and an optional next cursor. They do not expose the server's absolute root or invent a repository-wide file count. Filtering applies only to entries already loaded from the current directory. Platform labels and errors support Chinese and English; user paths and source text retain their original content.

Each read checks current project authority, the original binding, root-constrained paths and directory identity. Sensitive paths and links are excluded. Cursors bind the original caller, project, root, directory and metadata snapshot. A changed directory requires a fresh first page rather than merging two observations. Reads scan at most 10,000 raw entries in one directory and return at most 200 entries per page. This is a bounded directory observation, not an atomic snapshot of all file contents.

The implementation reuses existing frameworks and dependencies. The optional 12ui CLI is unavailable locally; no new design service, login or paid generation was used. Full remote Git, terminals, LSP/DAP, multi-host operation and the remaining P0–P7 platform acceptance remain separate deliverables.
