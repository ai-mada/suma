# Transactional store

`internal/store.Open(driver, dsn)` opens modernc SQLite by default, the MySQL driver for OceanBase (`oceanbase` or `mysql`), or pgx for PostgreSQL (`postgres`, `postgresql`, `pgx`). Unknown drivers fail explicitly. SQLite enables foreign key checking, WAL and a 10-second busy timeout and limits each handle to one open connection. PostgreSQL query placeholders are rewritten to positional parameters. OceanBase uses MySQL-compatible SQL and `LONGTEXT` payload columns.

SQLite is exercised by the local test suite, including two independent connections competing for one node. OceanBase and PostgreSQL are implementation paths requiring real-instance verification; driver availability does not imply deployment or SQL compatibility has been verified against those servers.

## Tables owned by the DAG package

| Table | Durable fact |
| --- | --- |
| `dag_workflows` | Tenant/project scoped immutable workflow versions and compiled graph JSON |
| `dag_runs` | Pinned workflow version, run state and optimistic version |
| `dag_nodes` | Node state, immutable node spec, attempt, fence, worker, lease deadline, timer, dynamic parent and output |
| `dag_attempts` | Individual execution attempts, ownership and reported result evidence |
| `dag_events` | Globally ordered events with tenant/project/run/node identity |
| `dag_ready` | Ready nodes and durable insertion order |
| `dag_run_fairness` | Last committed claim sequence per run |
| `dag_operations` | Scoped operation keys and request fingerprints |
| `dag_resource_limits` | Configured project capacities and cumulative budget consumed |
| `dag_resource_ledger` | Per-node concurrent resource reservations |
| `dag_subruns` | Unique parent-node-attempt to child run binding and parent pause ownership |
| `dag_compensations` | Source run/version/state, explicit reason, compensation run and last reported compensation state |
| `dag_compensation_effects` | Fixed successful source attempt/fence references for generated inverse nodes |
| `dag_lock` | Cross-process scheduler write lock and event sequence allocator |

`dag.New` creates the initial schema with idempotent DDL. It can reopen an existing schema from this implementation without clearing data. This initial version does not implement a general migration framework for incompatible historical schemas. Workflow versions preserve execution semantics independently of source changes; application/schema upgrades still need a reviewed migration and backup plan.

## Transaction protocol

1. Begin a SQL transaction and lock the singleton scheduler row with an update.
2. Sample current database epoch seconds.
3. Validate tenant/project identity, operation fingerprint, optimistic run version or assignment ownership as applicable.
4. Perform state changes, attempt updates, resource admission/release and queue operations.
5. Recompute joins, conditional skips, waits, map expansion and run finality.
6. Append sequenced events and commit all facts together.

A crash before commit exposes none of the transition. A committed claim remains leased until a matching result, heartbeat extension, lease expiry or reconciliation. Expiry changes ownership confidence to unknown; it does not establish that an external operation stopped. The engine retains resources and rejects unproven late success. Explicit stop proof from the same attempt/fence is accepted while reconciling; an operator reconciliation is separately authorized by the service and recorded with its reason.

Subworkflow children are created and linked atomically before parent readiness becomes visible. A child's terminal commit also advances its waiting parent, recursively through nested runs. Pause ownership is recorded so a parent resume does not override an independent child pause. Parent cancellation/deadline expiry records stop requests and waits for actual descendant termination or reconciled evidence. Compensation similarly commits the generated immutable workflow, run, exact source attempts and audit event together; it preserves the source run's historical outcome and forbids reusing that source for retry after compensation begins. The durable compensation relationship identifies generated workflows as internal: they cannot be publicly rerun, republished or nested as subworkflows. `CommandWithHook` includes service-owned acceptance invalidation in that same transaction, before the operation receipt is committed, and does not repeat the hook on receipt replay.

Idempotency keys are unique inside a tenant/project and operation scope. Their fingerprints cover publication, creation target/version/binding identity, command action/payload, or complete result. Publication receipts retain the exact immutable revision. `CreateRunBound` commits a service-owned task binding within the same scheduler transaction, so failed prerequisite validation cannot expose claimable work. Reusing a key with a different request returns conflict. Event sequence allocation is transactional, so rollback does not leave an event referring to an uncommitted state. Run reads use the same lock to return a consistent run and node snapshot. List queries close their row cursor before reading nested records, which avoids exhausting SQLite's single connection.

## Operational consequences

The singleton scheduler lock creates a serial mutation path across projects and tenants. This is an explicit initial correctness tradeoff; claims and `Tick` scan eligible work and are not load-tested at production scale. Large histories require query/index and retention work before a high-throughput deployment. No event cleanup or archival is performed automatically.

`Tick` must run periodically to reconcile expired leases and advance timers. Backups need a transactionally consistent database snapshot (including SQLite WAL when applicable). Worker journal storage is a separate recovery source and must be retained alongside server evidence. SQL transactions cannot roll back an external model invocation, process side effect, filesystem operation, or remote API call. Those destinations need their own idempotency keys and reconciliation proofs.

DSNs and database credentials are provided at runtime and are not embedded in records or source. Tenant/project filters protect all public engine reads and writes; HTTP authentication and project RBAC are the service's responsibility. A shared SQL database is not a worker filesystem sandbox or a substitute for database access controls.

## Business projections and document visibility

`Service.Migrate` installs `suma_index_outbox`, `suma_index_failures` and `suma_failure_projection` at startup, using `MigrateProjections` for the delivery ledgers. Periodic projection loops execute data reads/writes and perform no runtime schema creation. `IndexStatus` reads per-project pending/indexed/failed counts without changing activity or schema.

Document description plus optional body is limited to 1 MiB of UTF-8 bytes, including the joining newline. Creating or updating a document commits an immutable revision event with the canonical source mutation. `archived` and `deleted` sources are excluded from both keyword fallback and canonical validation of remote search hits. These visibility states produce a higher-revision `delete` tombstone instead of an upsert. Tombstone requests carry only source scope, revision and action, not the removed title/body. A deliberate restoration to active creates a newer upsert revision. The knowledge service's monotonic revision/tombstone rules reject late same-or-older upserts, while the control service independently rejects stale or hidden-source hits.

Index projection processes unrelated events even when one source fails. Permanent provider 4xx responses (except retryable 408/429) and malformed persisted payloads become `failed` with a scoped audit record. Transient transport/server/rate-limit errors remain pending. Failed deliveries are visible through `IndexStatus`; correcting and requeuing a permanent failed event remains an administrative operation, not an automatic claim of successful indexing.

Failure-to-issue projection consumes persistent `dag_events` `run_failed` facts, including approval, timer/subworkflow and other failures without a runner result request. Issue creation uses the existing per-run operation receipt, then acknowledges each failure event in `suma_failure_projection`. A crash between the two commits safely replays the same issue before writing the event receipt. Historical failure events remain projectable even if the run has since been retried. Issue deduplication is per run; the projection ledger preserves delivery evidence per event.
