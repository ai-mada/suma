# Suma implementation contract — 2026-10-23

This is the integration contract for the initial executable implementation. The full target remains tracked in docs/delivery.

## Ownership

- Core agent owns internal/dag, internal/store, and their tests/migrations. It may update go.mod/go.sum for required drivers.
- Runtime agent owns internal/runner, cmd/suma-runner, packages/agent-runtime, apps/desktop and their documentation/tests. Do not alter DAG or HTTP service source.
- UI agent owns apps/web and packages/ui. Root owns root manifests, API, business records, shared contracts, deployment, documentation, and integration.

## Shared wire conventions

JSON uses snake_case. IDs are strings, times are RFC3339 UTC strings, versions are positive integers. Errors are {error:{code,message,details?}}. Lists are {items:[],total:number}. Mutations use {expected_version?:number,idempotency_key:string,...fields}. Authentication is an HttpOnly same-origin session cookie or Bearer token. API base /api/v1. Tenant comes from authenticated session; project_id is mandatory on project-scoped routes. Secret tokens never occur in read responses.

## DAG Go contract

Package internal/store exports Open(driver,dsn string)(*sql.DB,error), Dialect(driver string) string. Drivers: sqlite (default), oceanbase, postgres. Runtime unsupported/unverified capabilities must be explicit.

Package internal/dag exports New(db *sql.DB, dialect string)(*Engine,error). Definitions: Workflow{ID,Name,Version,Nodes,Edges}; Node{ID,Name,Type,Command []string,DependsOn []string,Join string,Quorum int,Condition?,Branches?,TimeoutSeconds int,MaxRetries int,Idempotent bool,Resources?,Config?}; Edge{From,To,Condition?}. Use JSON snake_case. Compiler should accept DependsOn and edges, deduplicate and validate.

Engine methods (context.Context first): Publish(ctx,tenantID,projectID string,w Workflow)(Workflow,error); Workflows(ctx,tenantID,projectID string)([]Workflow,error); CreateRun(ctx,tenantID,projectID,workflowID,operationID string)(Run,error); Runs(ctx,tenantID,projectID string)([]Run,error); GetRun(ctx,tenantID,projectID,runID string)(Run,error); Events(ctx,tenantID,projectID,runID string,after int64)([]Event,error); Command(ctx,tenantID,projectID,runID,action,operationID string,expectedVersion int,payload map[string]any)(Run,error); Claim(ctx,tenantID,projectID,workerID string,capabilities []string)(*Assignment,error); Heartbeat(ctx,tenantID,projectID string,a Assignment)error; Complete(ctx,tenantID,projectID string,result Result)error; Tick(ctx)error. Compiler exported Compile(w Workflow)error.

Run JSON: id,tenant_id,project_id,workflow_id,workflow_name,workflow_version,version,state,created_at,updated_at,nodes:NodeRun[]. NodeRun JSON: id,name,type,state,reason,attempt,fence,worker_id,command,output,error,started_at,completed_at. Assignment JSON: run_id,node_id,attempt,fence,worker_id,command,type,timeout_seconds,idempotent,config. Result JSON: run_id,node_id,attempt,fence,worker_id,exit_code,output,error,operation_id,stopped. Event JSON: sequence,run_id,node_id,type,at,data. Error exports may use typed error or recognizable error values; root maps to HTTP statuses.

Long-standing statuses: pending,ready,running,waiting,reconciling,succeeded,failed,skipped,cancelled. Approval action payload node_id,decision,reason. Retry payload node_id; reconcile payload node_id,outcome(not_started|succeeded|failed),reason; pause/resume/cancel are run-level. Claim returns nil if no eligible work. Leases use DB-time semantics; default short testable values. Root handles authentication and RBAC before Engine methods.

## HTTP / UI contract

GET /auth/session -> {user,organization,projects}; POST /auth/setup {name,email,password,organization,project} -> session; POST /auth/login {email,password}; POST /auth/logout.

GET /projects -> list; POST /projects {name,description,idempotency_key}. GET /overview?project_id -> {project,counts,activity,capabilities}. Entities endpoints /requirements,/tasks,/members,/issues,/documents,/releases: GET ?project_id, POST/PATCH {project_id,...}; records {id,project_id,version,title,name?,description,status,priority,owner_id,requirement_id,depends_on,acceptance_criteria,created_at,updated_at,...}. Task status cannot be accepted without successful linked run + test evidence + explicit review. PATCH /tasks/{id} actions assign/update only; POST /tasks/{id}/review for acceptance. POST /requirements/{id}/decompose creates selected task drafts atomically.

GET/POST /workflows?project_id -> lists/published Workflow. POST /workflows/validate -> {valid,errors}; POST /runs {project_id,workflow_id,task_id?,idempotency_key}; GET /runs?project_id; GET /runs/{id}?project_id; GET /runs/{id}/events?project_id&after; POST /runs/{id}/commands {project_id,action,expected_version,idempotency_key,payload}. UI should read declared nodes/edges; never fabricate linear edges when edges are absent. Mutations await success, preserve draft on errors, and re-fetch current version after 409.

POST /runner/claim {project_id,worker_id,capabilities} -> {assignment:null|Assignment}; POST /runner/heartbeat {project_id,assignment}; POST /runner/results {project_id,result}. Runner uses Bearer token provided by user, root documents setup token creation. GET /capabilities?project_id reports configured/available/verified separately. POST /search {project_id,query,types} -> {items:[{id,type,title,snippet,source_ref,revision,locator}],mode,index_watermark}.

UI navigation overview,requirements,development,quality,operations,knowledge,settings, with development subviews tasks,runs,workflows,ide. Light/dark themes, Chinese copy, URL-persistent project/view/run/node. Initial setup must be functional. API responses, not hardcoded demonstration numbers, own all counts and state.
