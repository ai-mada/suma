# Workspace files and record queries

Implemented contract, 2026-10-23. HTTP wiring belongs to the platform API; these helpers enforce persistence and query boundaries.

## File saves

`POST /workspace/file` requires `project_id`, a stable 8–128 byte `idempotency_key`, `path`, `expected_hash`, and UTF-8 `content` no larger than 1 MiB. Authorization is checked on the server. The caller must retain the entire original request when retrying after a timeout or lost response.

`Service.MigrateWorkspace` creates `suma_workspace_file_operations` during startup migration. GET requests perform no DDL or other database writes. Each journal row stores its tenant, project, operation ID, actor-bound request hash, canonical root, relative path, original and resulting hashes, intended content, original response, state and creation time. OceanBase uses MEDIUMTEXT for content/response to accommodate the editor limit and JSON escaping.

A no-op write locks the existing project row before reading operations or files. On SQLite it obtains the database writer lock; PostgreSQL and OceanBase lock the project row. Existence is checked separately because MySQL can report zero changed rows for a successful no-op update.

1. Check the durable operation. A completed operation returns its exact stored result, even if the file has since changed. Reusing its ID with different content returns `idempotency_conflict`.
2. Reconcile prepared operations under the project lock before accepting another save. If the file has its original hash, finish the intended replacement. If it already has the resulting hash, finalize the operation. A third hash returns `workspace_reconciliation_required` and leaves the external content untouched.
3. Persist and commit the new preparation before changing the filesystem. Write a sibling temporary file, preserve permissions, sync its content, recheck the original hash and path, then atomically rename it.
4. Commit the completed journal state. A crash or lost connection on either side of rename is recoverable by resending the same request. A later save or binding change also reconciles outstanding preparations before proceeding.

Preparation and completion use separate database transactions because an ordinary SQL transaction cannot atomically commit a filesystem rename. This is a recovery protocol, not a distributed filesystem transaction. Retain completed journal rows for as long as retry replay is promised; deleting receipts breaks that promise. Windows has no portable directory fsync through Go's `os.File.Sync`; sudden host power loss can still require explicit reconciliation. Writers that bypass this service, including other projects bound to the same physical tree, do not participate in the project lock.

## Workspace jail

The server must explicitly set `SUMA_WORKSPACE_ROOT`. Bindings resolve to canonical directories inside that root. File access rejects traversal, absolute paths, alternate data streams, trailing dots/spaces, non-regular files, and symlinks in every relative component. Prepared operations retain their original canonical root and reject a root whose link resolution has changed. Known credential directories and files, environment files, private-key extensions, service caches and internal save files are excluded.

This is a filesystem path policy, not a content-based secret scanner. Privileged local processes can alter directories between path validation and OS operations; deployments must control local write permissions. Hard-link aliases and arbitrary secrets stored under ordinary source filenames are not classified by this deny list.

## Record pagination

`ListPage(ctx, principal, project, kind, limit, cursor)` checks the table allow list and project permission, counts all scoped records in SQL, and returns at most `limit` records. The default is 100 and the maximum is 200. Ordering is `updated_at DESC, id ASC`; the next page uses a keyset predicate over that pair. Cursors bind the tenant, project and record kind and cannot transfer to another list.

The returned total is the full scoped count, not page length. An empty next cursor marks the end. Ordinary transactions keep SQLite count/page reads consistent within a request; PostgreSQL/OceanBase default isolation can observe intervening commits. Pages are live reads, so edits between requests can move a record across the cursor boundary; the cursor does not promise an immutable multi-request snapshot.

## Keyword search

`SearchKeywords` filters the full authorized SQL sources for requirements, tasks, issues and documents. It matches lowercase literal substrings in titles, descriptions and serialized document/record data, including `data.content`. `%`, `_` and the chosen escape character `!` are escaped before LIKE matching. SQL parameters carry all user data; table identifiers come from a fixed server-owned list.

The response includes at most 50 results, the true matching `total`, `limit`, `truncated`, source revision/locator and explicit keyword coverage. The result limit never limits the records searched. Search remains keyword search; semantic indexing and source revision checks are handled separately by the knowledge adapter. Case conversion follows the selected database's LOWER behavior; SQLite does not provide full Unicode case folding by default.

## Verification

Regression coverage in `internal/platform/workspace_test.go` exercises lost acknowledgements and changed request rejection, two independent SQLite connections racing distinct saves, prepared operations before/after filesystem commit, ambiguous external edits, read-only GETs with the journal table removed, traversal/secrets/symlink denial, 1005-record keyset traversal, literal wildcard search and tenant/project isolation. Tests keep process TEMP/TMP, GOTMPDIR and GOCACHE under `.cache/`. SQLite execution is recorded in the integration verification report; PostgreSQL/OceanBase deployment behavior and real host power-loss recovery remain unverified.
