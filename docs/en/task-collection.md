# Unified task workspace and bounded groups

Stage T-TASKS-137, initial product1.0.0. Owner: planning and integration. [Canonical contract](../../contracts/task-workspace.md), [design/SOP](../design/task-collection.md), [verification](../quality/task-collection-verification.md).

Development → Tasks has one entry with local named views, four server sorts, list/board layouts and status/owner/priority grouping. Advanced filters collapse on narrow screens; counts distinguish the complete authorized result from loaded rows and group headers.

`/task-workspace` is the sole row reader. `/task-collection/summary` shares its literal base filters and returns complete task/group counts plus a bounded group page (default20, maximum100). Users explicitly load more groups and rows. At most three reads run concurrently. Archived tasks are included; `all` and `__unassigned__` are literal values. Separate unassigned/priority_unset/status_unset flags identify genuinely empty data. Search preserves spaces and escapes SQL wildcards. Time ordering preserves nanoseconds without rewriting source records.

Each request rechecks current human/user-API authority in its read transaction; Runner/Agent credentials are refused. Cursors bind identity, project, full filters and page size. Multiple pages are live reads, not a frozen snapshot. Higher task versions win when columns overlap; equal-version conflicts and changed counts request a refresh. Each group batch retains its own observation time.

One column's temporary failure is independently retryable. Current-scope401/403/404 clears protected lists, detail, editors, selections and queued reads; a response from an old identity cannot clear a new scope. Only whitelisted view fields reach navigation, so changing filters cannot resurrect old detail/panel locators.

Named views remain private to this browser and current user/organization/project. Unavailable/corrupt storage and cross-tab changes have explicit recovery. Prerequisite selection reads an independent unfiltered task query. Original team dispatch, model budgets, DAG execution, fixed candidates, independent tests, human review and issue feedback keep their existing authority; presentation never starts or accepts work.

The narrow attributed historical Multica grouping adaptation remains in its reusable renderer; current restricted sources are behavior references. Shared views, cross-device sync, real database/deployment matrices, all tool integrations and production-scale acceptance remain in the full plan. This stage does not change the58 limited/34 pending/92 total ledger.

Current local view envelopes require `format: "suma-task-views-literal-v1"`. Unrecognized envelopes are retained unchanged and shown as invalid until explicit reset; no old owner meaning is inferred or migrated.
