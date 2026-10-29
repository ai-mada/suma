# Project task view contract

Stage T-MULTICA-137; current initial product1.0.0. Domain owner: project planning. This adds a read projection and browser preferences, not another task database or executor.

## Authorized query

`GET /api/v1/task-workspace?project_id=...` returns `TaskWorkspacePage {items: Record[], total, next_cursor, as_of}`. Browser sessions and currently authorized project API credentials may read; Runner and agent-read credentials are refused. Every page revalidates current identity, tenant, project and credential authority inside the same read transaction as count and rows. Unknown or other-project owner IDs are exact filters, not member lookups.

Optional single-valued parameters: `q`, `status`, `owner_id`, `priority`, `sort`, `limit`, `cursor`. Unknown or duplicate parameters fail. Search is a literal substring of title, description or ID; `%`, `_` and the escape character have no wildcard meaning. Omitted status/owner/priority means unrestricted; explicitly empty exact filters fail. `owner_id=__unassigned__` means no assigned resource. Status and priority preserve exact bounded user/domain values rather than redefining the existing record lifecycle.

Sort is `updated_desc` (default), `created_desc`, `title_asc` or `priority_desc`. Priority order is urgent, high, medium, low, then unrecognized/missing priorities; a stable ID tie-breaker makes traversal deterministic for unchanged records. Title collation follows the supported database; this stage's evidence is SQLite, not a cross-database Unicode collation guarantee. Page size defaults to100 and cannot exceed100.

The cursor binds the current identity, tenant/project and normalized complete filter/sort. Reusing it under another scope fails. Count and rows are from one current read transaction, and `as_of` marks that read. Multiple pages are not a frozen snapshot: edits may move records between pages. Clients merge by ID, label loaded scope, and offer refresh instead of claiming an immutable full export.

No query changes ownership, status, run, test evidence, human acceptance, dispatch or deployment. Existing task writes and team plans retain versions, stable operation IDs and current command authorization.

## Views, grouping and restoration

List and board consume the same server query. Grouping by status, owner or priority changes presentation only, using the adapted Multica column algorithm identified in [provenance](../third_party/multica-legacy/PROVENANCE.md). Column counts are counts of loaded records; `total` is the complete authorized match count. Loading more retains existing rows. Failed refresh may show stale rows with a visible error; authorization loss clears protected data. A superseded request cannot install rows from an old scope.

The production task detail, discussion, edit, dependency view and controlled team dispatch remain authoritative. No drag transition or local preference can mark accepted or start an agent. Returning from a detail restores the task view's URL filters; known URL fields are validated, and project changes do not silently carry another project's owner.

When current counts or continuation prove an accumulated window inconsistent, the client clears that window and requires an explicit first-page refresh. It does not loop or display more loaded items than the current total. Current-scope authorization failures from task or form lookups also clear task detail and editing content; failures from a previous identity cannot clear a new scope. Task form prerequisites use a separate unfiltered project query and pagination, retaining selected dependency IDs even outside a loaded page.

Named views in this stage are explicitly **private preferences in this browser**. They store only query, status, owner ID, priority, order, layout and grouping, partitioned by current user, organization and project. They do not contain task titles, descriptions, run snapshots, credentials, authorization claims or server-side approvals. Persisted shape, size and count are bounded and validated; corrupt/unavailable storage is visible and does not prevent live task access. Switching tabs cannot silently overwrite active edits.

Saving, loading, overwriting and deleting a local view are explicit; deletion confirms the named target. Shared/server-persistent views, cross-device synchronization, custom fields, swimlanes, bulk business mutation and complete Multica parity remain in the original implementation plan. Browser persistence is never described as those capabilities.
