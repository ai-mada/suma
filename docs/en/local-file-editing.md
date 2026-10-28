# Local file editing and saving

[中文](../development/local-file-editing.md) · [Fields, transactions and recovery contract](../../contracts/workspace-file-context.md)

This guide covers the current 1.0.0 local text editing stage. Open Development → IDE, select the local workspace, and open an existing text file in the project's bound folder. Desktop users can explicitly select and bind a project folder; ordinary browsers use a binding inside the server's allowed root. The complete IDE, multiple workspaces, file creation/deletion, terminals, LSP and debugging remain in scope for further work.

## Confirm where the file came from

The directory panel shows the current project workspace. Each file read verifies its project, workspace name, actual root identity and content. The editor retains that basis with the draft. A “Your draft is from…” notice identifies the workspace of the original read, which may differ from the current directory panel. Matching folder names, relative paths and bytes do not establish the same workspace binding.

Choose Save or press Ctrl/Cmd+S after editing. Saving checks both the original file version and its workspace context. It never silently overwrites another operation's changes.

## Handle the two kinds of conflict

| Notice | Meaning and action |
| --- | --- |
| File version conflict | The workspace is unchanged, but another operation changed the file. Your draft remains in the editor. Choose “Keep draft and update version basis”, manually review and merge, then explicitly save. “Load latest content” replaces the editor draft with the current content already read. |
| Workspace changed or needs verification | The draft's workspace changed, or the original save was recovered without verifying the current workspace. Choose “Read current file”, inspect its origin and “Compare current content”. If this is the workspace where you want to apply the draft, choose “Use current workspace and keep draft”, merge manually, then save. “Load latest content” replaces the old editor draft. |

Reading the current file does not write it. Adopting a new basis does not save immediately. A failed read leaves the draft unchanged. Matching folder names, metadata refreshes or repeated save attempts cannot transfer an old draft to a new workspace.

When the service confirms that the original operation was not accepted, it retains a rejection receipt. A late copy of that request cannot write even if the original directory returns. After adopting the current workspace, the next explicit save uses a new operation ID.

## When the save result is unconfirmed

A connection failure, invalid acknowledgement or reconciliation notice does not prove that no write happened. Choose “Recover previous save” to check the original operation. The page retains its exact request, submitted content and original caller context. Recovery does not include later edits or automatically start another save.

An original successful acknowledgement describes that operation; it does not establish the current workspace. After recovery, read the current file and explicitly choose a new basis. If reconciliation is required, inspect the original operation, original content and submitted content. Follow the external-editor instructions only after confirming that this is still the original directory. A replaced root or an operation from another session needs its original record; writing into a new directory cannot resolve that uncertainty.

## How long drafts are retained

Unsaved drafts and pending requests use bounded memory in the current page session, isolated by account, organization and project. Switching identity does not expose or adopt another identity's draft. Returning to the original context can restore a record still in memory, but older records may be evicted. File contents and pending requests are not written to persistent browser storage. Recovery after refresh, closing the page or restarting the app is not guaranteed.

Navigation guards and unsaved-content prompts are not a durable backup. Keep your own copy of any draft that must survive the session. A successful file save does not establish passing tests, task acceptance or deployment.
