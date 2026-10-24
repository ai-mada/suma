# 发布控制面合同 v1

状态：2026-10-24 实施合同。与 release-execution.md 配套；功能须以实际测试和提供方观测验收。旧 runtime 交付产物保持冻结。

## 浏览器 API

基础 `/api/v1`，列表 `{items:[],total:N}`，首轮完整项目列表，不声称已分页。读取要求实时项目访问。除只读与 preview 外，写入必须 `idempotency_key`；更新必须精确整数 `expected_version`。危险写还要求当前未过期 browser 会话、实时项目 admin 或组织 owner/admin；reviewer 不能批准部署，API/Agent/共享 Runner 不能冒充 browser。

| 方法/路径 | 请求/响应 | 权限 |
|---|---|---|
| GET `/environments?project_id` | Environment 列表 | 项目读取 |
| GET `/environments/{id}?project_id` | Environment 详情 | 项目读取 |
| GET `/release-providers?project_id` | 非秘密 `{id,site_ids,kind,capability_state}` 选项列表；无配置则空 | 项目读取 |
| POST `/environments` | `{project_id,name,provider_id,site_id,health,idempotency_key}` → Environment | browser admin |
| PATCH `/environments/{id}` | `{project_id,expected_version,idempotency_key,name?,status?,health?}` → Environment；status active/archived，已有 provider/site 不可换绑 | browser admin |
| POST `/release-plans/preview` | PlanInput → `{preview_sha256,environment,candidate_id,action,health,eligibility,blockers,allowed_actions}` | browser developer/admin |
| POST `/release-plans` | PlanInput + `{preview_sha256,idempotency_key}` → ReleasePlan | browser developer/admin |
| GET `/release-plans?project_id` | ReleasePlan 列表 | 项目读取 |
| GET `/release-plans/{id}?project_id` | ReleasePlan，附实时 eligibility/blockers/allowed_actions | 项目读取 |
| POST `/release-plans/{id}/approve` | `{project_id,expected_version,idempotency_key,plan_revision,plan_sha256,decision:"approved"|"rejected"|"revoked",reason}` → ReleasePlan | browser admin |
| POST `/release-plans/{id}/execute` | `{project_id,expected_version,idempotency_key,plan_revision,plan_sha256}` → Deployment | browser admin |
| GET `/deployments?project_id` | Deployment 列表 | 项目读取 |
| GET `/deployments/{operation_id}?project_id` | Deployment 含最新结构化 observation、观测历史、eligibility/blockers/allowed_actions | 项目读取 |
| POST `/deployments/{operation_id}/reconcile` | `{project_id,expected_version,idempotency_key}` → Deployment；只读取已认证提供方持久观测核对，不能提交 outcome/健康结论 | browser admin |
| POST `/deployments/{operation_id}/rollback-preview` | `{project_id,expected_environment_generation}` → rollback PlanInput/可用历史候选；实际回退仍走新 preview/create/approve/execute | browser admin |

`PlanInput={project_id,candidate_id,environment_id,action:"deploy"|"rollback",expected_environment_generation,rollback_source_operation_id?}`。rollback 必须指定该环境历史已健康部署的 operation，其候选、制品和原证明由服务端核验。不可直接传 artifact/proof/source_ref 代替持久候选。计划内容不可编辑；需要修改则创建新计划，首轮每个新计划 `revision=1`。`version` 是可变审批/执行状态版本，与不可变 `revision/sha256` 分开。

Environment：`{id,project_id,version,name,status,provider_id,site_id,policy_version,health,generation,active_generation,active_artifact_id,active_artifact_sha256,barrier_operation_id,capability_state,created_at,updated_at}`。health 为 `{checks:[{path,expected_status,expected_file}],timeout_ms,attempts}`，固定站点内路径，不接受 URL、命令、端口、脚本或凭证。provider/site 必须在管理员服务器配置中登记，配置本身仅 configured，不表示已连接或验证。generation 是最高预约代数，active_generation 是已观测 active 代数，不可混用。环境归档保留身份、历史和 generation；活动屏障时不能改策略/归档。

ReleasePlan：`{id,project_id,version,state,revision,sha256,content,created_by,created_at,approval,execution_operation_id,eligibility,blockers,allowed_actions}`。content 使用 shared `ReleasePlanContent`：candidate_id、action、environment_id/provider_id/site_id/policy_version、expected_generation/generation、artifact_id/artifact_sha256、source_commit、build_attestation_id、package_test_attestation_id、accepted_task_basis_sha256、reserved_execution(run_id,node_id)、health(含实际文件 SHA)、rollback_source_operation_id。摘要是服务端规范化内容 SHA，不含临时票据或可变 decision。计划创建预留 run/node 身份；execute 同一事务创建唯一管理 DAG 并绑定，不先创建可被 claim 的未绑定 run。

state：`pending_approval,approved,rejected,revoked,executing,completed`。approval 保存 actor/reason/时间及批准的 plan hash；每次 approve、execute、ticket、consume、begin-effect 均重新核验候选当前依据、当前审批人与执行人权限、环境策略及代数。`eligibility` 为当前可用布尔，`blockers` 是 `{code,message}` 数组；读到旧 approved 不等于仍可执行。失效不会被普通刷新或旧幂等回执恢复。

Deployment：`{operation_id,project_id,version,plan_id,plan_revision,plan_sha256,environment_id,provider_id,generation,artifact_id,artifact_sha256,run_id,node_id,state,effect_state,health_state,observation,observations,issue_id,created_at,updated_at,allowed_actions,blockers}`。初始 state=`queued`；后续来自真实提供方。effect=`not_started|applied|unknown`，health=`not_checked|passed|failed|unknown`，组合投影 `deployed_healthy|deployed_unhealthy|failed|cancelled|reconciling`。观察缺失、网络不明、停止请求或旧 attempt 不能自动成功。变更内容复用幂等 key 返回409；重放前必须复核当前权限。

## Runner/Provider API 与共享 DTO

Runner 请求携带管理员登记的专属 worker 发布凭证；只对真实 managed run/node/current attempt/fence/worker + 未停止有效租约发放授权。

| 路径 | DTO | 说明 |
|---|---|---|
| POST `/runner/release-authorizations` | ReleaseAuthorizationRequest → ReleaseAuthorization | 随机 ticket 120秒，只存 hash、不进 receipt/journal；重领撤销旧未消费 ticket |
| POST `/provider/deployment-authorizations/consume` | ConsumeDeploymentAuthorization → DeploymentAuthorizationGrant | 独立 provider token 固定 tenant/project/provider audience；caller_worker_id 来自 provider 验证的传输身份 |
| POST `/provider/deployment-authorizations/begin-effect` | BeginDeploymentEffect → DeploymentEffectGrant | 持久独占 effect grant；再次核验当前依据，grant 后屏障不得由租约到期释放 |
| POST `/provider/deployment-authorizations/inspect` | InspectDeploymentAuthorization → DeploymentAuthorizationGrant | 严格绑定的只读历史消费回执，无秘密；消费ACK丢失/重启时恢复，不能新增副作用 |
| POST `/provider/deployment-observations` | DeploymentObservation → Deployment | 非秘密结构化事实，只接受绑定 provider；单调版本、同版本不同内容409 |

共享结构在 `internal/releasecontract/deployment.go`。ReleaseExecution 放 Assignment 顶层，只有非秘密字段。Provider request SHA 由 `DeploymentRequestDigest` 对 protocol、完整 assignment 和 ReleaseExecution 规范编码，不含 ticket。Provider observation evidence SHA 由 `ObservationDigest` 对除 evidence_sha256 外的完整结构编码，必须匹配；成功须 applied、本 generation/artifact、每项健康 path/status/body hash 与计划相同、有 begin-effect grant，且 provider fence_generation 不小于 generation。effect unknown 保留环境屏障。已 effect grant 的 not_started 只有 provider 持久 fence ACK（fence_generation≥预约代数）才能释放。

提供方历史观测不可变，迟到历史可保存但不覆盖新 active。reconcile 不允许人手填 succeeded；读取 authenticated observation 后核对。原操作可恢复查询，取消/租约过期不清屏障。未知时 UI 只提供刷新/核对；回滚必须新的候选计划、审批、operation、generation。首轮无 arbitrary remote shell、URL 或自动回滚。

已合法 begin-effect 的实际终态不会因之后撤批或任务变化而被抹掉：观测/健康保留为历史事实，当前候选 eligibility/批准仍分别显示失效。Runner 原结果丢失、unknown 或租约过期时，管理员核对使用实际 Provider GET 记录，在 `CompleteManagedObservedBound` 事务中恢复 managed DAG；保留原结果于 `dag_managed_resolutions` 和事件，不虚构 Runner 的 Stopped。尚有有效租约则只核对观测，由当前 Runner 正常提交结果。未知观测不走成功恢复。

`allowed_actions` 为 string[]：环境列表 create，环境 edit/archive；计划 approve/reject/execute/revoke；部署 reconcile/rollback_preview（对应路径 `/rollback-preview`）。rollback-preview 返回 `{input:PlanInput,preview:ReleasePlanPreview}`。只读建议基于实时项目角色，写入仍要求当前真实 browser 会话。审批历史单独不可变保存并由详情 `approval_history` 返回；不能用当前 approval 对象覆盖过去的批准。

## 事务与接入点

- 全部危险读写锁序 organization → DAG scheduler → environment；实时成员/角色及 browser session 在 receipt 前复核。
- `MigrateReleaseDeployment(ctx)` 独立迁移。root 接 database.go/http.go。
- root 提供 `validateReleaseCandidateTx` 当前有效候选、`releaseWorkerTx` 专属 worker 登记验证。
- root 提供原子 managed DAG API：before 回调锁 org/重验权限，scheduler lock 后 bind 回调锁 environment、复核 candidate/审批/receipt、CAS 代数和屏障、落唯一 run/node 绑定；任何失败整体回滚。
- Provider 配置由管理员启动参数传 `Service.ReleaseProviders []ReleaseProviderConfig{ID,TenantID,ProjectID,TokenHash,SiteIDs,Origin,ReconcileToken}`，普通 environment 请求不能创建 provider 身份或修改 token。秘密只来自进程配置。`SUMA_RELEASE_PROVIDERS_JSON` 只含 ID/site_ids、token_env、固定回环 origin 与 reconcile_token_env 引用；原始秘密不写 JSON。
- CompleteBound 必须在同一 scheduler 事务核对当前 provider observation/evidence，不允许普通 Result、泛用 reconcile 或任意 external_workflow 绕过 managed binding。失败/unknown 原样保留并形成问题回流。
