# Task workspace and personal views

Open Development → Tasks in a project. Search title, description or ID across the complete currently authorized project. Combine business status, owner and priority; choose updated, created, title or priority order. List and board share the same server query. Grouping by status, owner or priority only changes presentation.

On mobile, expand Filters and views to edit conditions; collapse them to prioritize tasks. The button reports the active filter count. Bulk team arrangement appears after selection. Dependency candidates in the create/edit form have an independent unfiltered project query and pagination, so current list filters do not hide valid prerequisites.

Group counts describe loaded records; the footer separately reports the complete matching total. Load more continues the current query. Concurrent edits may move records between pages; refresh rereads current data instead of treating multiple pages as a frozen export.

Save a named view to retain filters, order, grouping and layout in this browser, partitioned by user, organization and project. Explicitly load, overwrite, save another copy or confirm deletion. Other-tab changes do not silently replace your active filters. Corrupt storage is preserved until an explicit reset. These are personal local preferences, not shared or cross-device views.

Task titles open the existing formal detail and discussion. Back restores query conditions. Selecting tasks or assigning an owner does not start execution or grant acceptance. Read failures offer retry; lost authority clears protected data. An owner outside the loaded member directory keeps its identifier rather than becoming unassigned.

AI and CLI clients should use the [authorized query contract](../../contracts/task-workspace.md), never extrapolate from the first100 tasks. Existing typed task/team commands retain current authorization, versions and stable operation IDs. See [source reuse and scope](../integrations/multica-reuse.md) and [verification](../quality/multica-task-views-verification.md).
