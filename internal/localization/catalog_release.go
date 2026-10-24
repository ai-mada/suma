package localization

// Product-owned release messages describe fixed evidence and current authority.
// These exact sources are display-only: plan digests, human approval reasons,
// provider evidence, credentials and durable receipts are never translated.
var releaseCatalog = []Definition{
	// internal/platform/artifact_http.go:74
	{Key: "release.artifact.download_assignment_required", Texts: map[Locale]string{ZhCN: "下载需携带当前测试执行身份", EnUS: "Downloads must include the current package-test execution identity."}},
	// internal/platform/artifact_proofs.go:33
	{Key: "release.proof.unsuccessful_execution", Texts: map[Locale]string{ZhCN: "停止或结果未知的执行不能形成成功制品证明", EnUS: "Stopped executions or executions with an unknown outcome cannot produce successful artifact proof."}},
	// internal/platform/artifact_proofs.go:40
	{Key: "release.proof.fixed_source_required", Texts: map[Locale]string{ZhCN: "制品证明必须来自同一固定源码，构建及包测试不得改变代码", EnUS: "Artifact proofs must come from the same fixed source revision; builds and package tests must not change the code."}},
	// internal/platform/artifact_proofs.go:51
	{Key: "release.proof.command_profile_mismatch", Texts: map[Locale]string{ZhCN: "命令与登记的制品配置不符", EnUS: "The command does not match the registered artifact configuration."}},
	// internal/platform/artifact_proofs.go:59
	{Key: "release.proof.sealed_artifact_receipt_mismatch", Texts: map[Locale]string{ZhCN: "实际封存制品与执行回执不一致", EnUS: "The actual sealed artifact does not match the execution receipt."}},
	// internal/platform/artifact_proofs.go:64
	{Key: "release.proof.explicit_build_required", Texts: map[Locale]string{ZhCN: "构建证明需来自明确构建命令", EnUS: "Build proof must come from an explicit build command."}},
	// internal/platform/artifact_proofs.go:75
	{Key: "release.proof.build_identity_mismatch", Texts: map[Locale]string{ZhCN: "制品不属于本次构建身份或固定配置", EnUS: "The artifact does not belong to this build identity or fixed configuration."}},
	// internal/platform/artifact_proofs.go:79
	{Key: "release.proof.fixed_test_command_required", Texts: map[Locale]string{ZhCN: "包测试必须使用管理员固定的测试命令", EnUS: "Package tests must use the test command fixed by an administrator."}},
	// internal/platform/artifact_proofs.go:83
	{Key: "release.proof.frozen_test_input_missing", Texts: map[Locale]string{ZhCN: "缺少本次测试冻结的制品输入", EnUS: "The artifact input frozen for this package test is missing."}},
	// internal/platform/artifact_proofs.go:87
	{Key: "release.proof.test_input_mismatch", Texts: map[Locale]string{ZhCN: "测试制品、来源代码与冻结输入不一致", EnUS: "The tested artifact and source code do not match the frozen input."}},
	// internal/platform/artifact_proofs.go:91
	{Key: "release.proof.original_build_missing", Texts: map[Locale]string{ZhCN: "原构建证明不存在", EnUS: "The original build proof does not exist."}},
	// internal/platform/artifact_proofs.go:97
	{Key: "release.proof.kind_unknown", Texts: map[Locale]string{ZhCN: "未知制品证明种类", EnUS: "The artifact proof kind is unknown."}},
	// internal/platform/artifact_proofs.go:124
	{Key: "release.proof.current_attempt_changed", Texts: map[Locale]string{ZhCN: "构建或包测试当前尝试已变化", EnUS: "The current build or package-test attempt has changed."}},
	// internal/platform/artifact_proofs.go:128
	{Key: "release.proof.final_receipt_missing", Texts: map[Locale]string{ZhCN: "缺少可信执行的最终制品回执", EnUS: "The final artifact receipt from trusted execution is missing."}},
	// internal/platform/artifact_proofs.go:132
	{Key: "release.proof.final_receipt_mismatch", Texts: map[Locale]string{ZhCN: "制品证明与最终回执不一致", EnUS: "The artifact proof does not match the final receipt."}},
	// internal/platform/artifact_proofs.go:160
	{Key: "release.proof.explicit_build_ancestor_required", Texts: map[Locale]string{ZhCN: "制品输入仅能取自本次测试的明确构建祖先", EnUS: "Artifact input may only come from the explicit build ancestor of this package test."}},
	// internal/platform/artifact_proofs.go:163
	{Key: "release.proof.unique_test_profile_required", Texts: map[Locale]string{ZhCN: "包测试需要唯一的管理员测试配置", EnUS: "A package test requires exactly one administrator test configuration."}},
	// internal/platform/artifact_proofs.go:188
	{Key: "release.proof.workflow_invalid", Texts: map[Locale]string{ZhCN: "流程定义无效", EnUS: "The workflow definition is invalid."}},
	// internal/platform/artifact_proofs.go:210
	{Key: "release.proof.static_build_ancestor_required", Texts: map[Locale]string{ZhCN: "制品来源必须是流程的静态构建祖先", EnUS: "The artifact source must be a static build ancestor in the workflow."}},
	// internal/platform/artifact_proofs.go:214
	{Key: "release.proof.upstream_build_unavailable", Texts: map[Locale]string{ZhCN: "上游尚无可用构建证明", EnUS: "No usable upstream build proof is available yet."}},
	// internal/platform/artifact_proofs.go:228
	{Key: "release.proof.test_input_already_fixed", Texts: map[Locale]string{ZhCN: "测试已固定制品，请恢复原解析操作", EnUS: "The package test already has a fixed artifact. Resume the original resolution operation."}},
	// internal/platform/artifact_storage.go:58
	{Key: "release.artifact.assignment_incomplete", Texts: map[Locale]string{ZhCN: "执行身份不完整", EnUS: "The execution identity is incomplete."}},
	// internal/platform/artifact_storage.go:75
	{Key: "release.artifact.lease_changed", Texts: map[Locale]string{ZhCN: "执行租约已变化，不能新增制品或证明", EnUS: "The execution lease has changed. No new artifacts or proofs may be added."}},
	// internal/platform/artifact_storage.go:109
	{Key: "release.artifact.registered_build_command_required", Texts: map[Locale]string{ZhCN: "制品构建需绑定管理员登记命令", EnUS: "The artifact build must be bound to a command registered by an administrator."}},
	// internal/platform/artifact_storage.go:120
	{Key: "release.artifact.retention_quota_exceeded", Texts: map[Locale]string{ZhCN: "当前项目制品保留配额为 1 GiB；需要管理员扩展存储策略", EnUS: "The current project artifact retention quota is 1 GiB; an administrator must extend the storage policy."}},
	// internal/platform/artifact_storage.go:127
	{Key: "release.artifact.upload_already_started", Texts: map[Locale]string{ZhCN: "本次构建已有上传会话，请恢复原操作", EnUS: "This build already has an upload session. Resume the original operation."}},
	// internal/platform/artifact_storage.go:161
	{Key: "release.artifact.storage_locator_invalid", Texts: map[Locale]string{ZhCN: "制品存储定位或长度无效", EnUS: "The artifact storage locator or length is invalid."}},
	// internal/platform/artifact_storage.go:170
	{Key: "release.artifact.file_missing_or_changed", Texts: map[Locale]string{ZhCN: "制品文件缺失或已变化", EnUS: "The artifact file is missing or has changed."}},
	// internal/platform/artifact_storage.go:179
	{Key: "release.artifact.file_identity_changed", Texts: map[Locale]string{ZhCN: "制品文件身份已变化", EnUS: "The artifact file identity has changed."}},
	// internal/platform/artifact_storage.go:187
	{Key: "release.artifact.file_length_changed", Texts: map[Locale]string{ZhCN: "制品文件长度已变化", EnUS: "The artifact file length has changed."}},
	// internal/platform/artifact_storage.go:191
	{Key: "release.artifact.content_digest_mismatch", Texts: map[Locale]string{ZhCN: "制品真实内容摘要不匹配", EnUS: "The digest of the actual artifact content does not match."}},
	// internal/platform/artifact_storage.go:198
	{Key: "release.artifact.upload_identity_invalid", Texts: map[Locale]string{ZhCN: "制品摘要、长度或操作标识无效", EnUS: "The artifact digest, length, or operation ID is invalid."}},
	// internal/platform/artifact_storage.go:232
	{Key: "release.artifact.received_bytes_mismatch", Texts: map[Locale]string{ZhCN: "收到的实际文件长度或 SHA-256 不匹配", EnUS: "The received file's actual length or SHA-256 does not match."}},
	// internal/platform/artifact_storage.go:259
	{Key: "release.artifact.file_outside_fixed_manifest", Texts: map[Locale]string{ZhCN: "文件不属于已固定的制品清单", EnUS: "The file does not belong to the fixed artifact manifest."}},
	// internal/platform/artifact_storage.go:285
	{Key: "release.artifact.sealed_upload_immutable", Texts: map[Locale]string{ZhCN: "上传已封存，不能新增文件", EnUS: "The upload is sealed. No new files may be added."}},
	// internal/platform/artifact_storage.go:367
	{Key: "release.artifact.already_sealed", Texts: map[Locale]string{ZhCN: "已封存，请恢复原封存操作", EnUS: "The upload is already sealed. Resume the original sealing operation."}},
	// internal/platform/artifact_storage.go:376
	{Key: "release.artifact.file_incomplete", Texts: map[Locale]string{ZhCN: "文件尚未完整收到", EnUS: "The file has not been received in full."}},
	// internal/platform/artifact_storage.go:379
	{Key: "release.artifact.manifest_length_mismatch", Texts: map[Locale]string{ZhCN: "文件长度与清单不一致", EnUS: "The file length does not match the manifest."}},
	// internal/platform/artifact_storage.go:411
	{Key: "release.artifact.sealed_manifest_changed", Texts: map[Locale]string{ZhCN: "封存清单已变化", EnUS: "The sealed manifest has changed."}},
	// internal/platform/artifact_storage.go:417
	{Key: "release.artifact.incomplete", Texts: map[Locale]string{ZhCN: "制品不完整", EnUS: "The artifact is incomplete."}},
	// internal/platform/artifact_storage.go:427
	{Key: "release.artifact.file_digest_invalid", Texts: map[Locale]string{ZhCN: "文件摘要无效", EnUS: "The file digest is invalid."}},
	// internal/platform/artifact_workers.go:79
	{Key: "release.worker.id_invalid", Texts: map[Locale]string{ZhCN: "执行节点标识应为 1–100 位字母、数字、点、短横线或下划线", EnUS: "The worker ID must contain 1 to 100 letters, digits, dots, hyphens, or underscores."}},
	// internal/platform/artifact_workers.go:84
	{Key: "release.worker.profiles_invalid", Texts: map[Locale]string{ZhCN: "发布命令配置无效", EnUS: "The release command configuration is invalid."}},
	// internal/platform/artifact_workers.go:89
	{Key: "release.worker.profile_identity_invalid", Texts: map[Locale]string{ZhCN: "命令需唯一标识、种类及真实配置摘要", EnUS: "Each command requires a unique ID, a kind, and the digest of its actual configuration."}},
	// internal/platform/artifact_workers.go:119
	{Key: "release.worker.identity_already_registered", Texts: map[Locale]string{ZhCN: "执行节点标识已有登记；请使用不同标识，不能覆盖历史凭证", EnUS: "The worker ID is already registered. Use a different ID; historical credentials cannot be overwritten."}},
	// internal/platform/artifact_workers.go:164
	{Key: "release.worker.explicit_revocation_version_required", Texts: map[Locale]string{ZhCN: "发布凭证仅能以明确版本撤销", EnUS: "Release credentials may only be revoked with an explicit version."}},
	// internal/platform/artifact_workers.go:190
	{Key: "release.worker.credential_version_changed", Texts: map[Locale]string{ZhCN: "发布执行凭证已变化，请刷新", EnUS: "The release execution credential has changed. Refresh it."}},
	// internal/platform/release_candidate_service.go:36
	{Key: "release.candidate.actual_delivery_required", Texts: map[Locale]string{ZhCN: "候选范围必须由本次构建和包测试所在运行实际交付并验收", EnUS: "The candidate scope must have been actually delivered by the run containing this build and package test, with human acceptance for that same run."}},
	// internal/platform/release_candidate_service.go:41
	{Key: "release.candidate.human_acceptance_missing", Texts: map[Locale]string{ZhCN: "缺少对应版本的正式人工验收", EnUS: "Formal human acceptance for the corresponding version is missing."}},
	// internal/platform/release_candidate_service.go:71
	{Key: "release.candidate.proof_basis_mismatch", Texts: map[Locale]string{ZhCN: "候选构建和包测试依据不一致", EnUS: "The candidate build and package-test bases do not match."}},
	// internal/platform/release_candidate_service.go:77
	{Key: "release.candidate.accepted_scope_missing", Texts: map[Locale]string{ZhCN: "候选缺少已验收交付范围", EnUS: "The candidate has no accepted delivery scope."}},
	// internal/platform/release_candidate_service.go:85
	{Key: "release.candidate.acceptance_basis_changed", Texts: map[Locale]string{ZhCN: "需求、任务、执行或人工验收依据已变化，需要重新建立候选", EnUS: "The requirement, task, execution, or human acceptance basis has changed. Create a new candidate."}},
	// internal/platform/release_candidate_service.go:89
	{Key: "release.candidate.scope_digest_mismatch", Texts: map[Locale]string{ZhCN: "候选范围摘要不一致", EnUS: "The candidate scope digest does not match."}},
	// internal/platform/release_candidate_service.go:113
	{Key: "release.candidate.name_required", Texts: map[Locale]string{ZhCN: "请填写 1–200 字节的候选名称", EnUS: "Enter a candidate name of 1 to 200 bytes."}},
	// internal/platform/release_candidate_service.go:117
	{Key: "release.candidate.accepted_tasks_required", Texts: map[Locale]string{ZhCN: "请选择 1–100 个已验收任务作为交付范围", EnUS: "Select 1 to 100 accepted tasks as the delivery scope."}},
	// internal/platform/release_candidate_service.go:150
	{Key: "release.candidate.sealed_artifact_proofs_required", Texts: map[Locale]string{ZhCN: "请选择同一已封存制品及其真实构建、包测试证明", EnUS: "Select the same sealed artifact and its actual build and package-test proofs."}},
	// internal/platform/release_candidate_service.go:160
	{Key: "release.candidate.task_version_invalid", Texts: map[Locale]string{ZhCN: "任务版本格式无效", EnUS: "The task version format is invalid."}},
	// internal/platform/release_candidate_service.go:167
	{Key: "release.candidate.duplicate_task", Texts: map[Locale]string{ZhCN: "交付范围有重复任务", EnUS: "The delivery scope contains duplicate tasks."}},
	// internal/platform/release_candidate_service.go:175
	{Key: "release.candidate.task_version_changed", Texts: map[Locale]string{ZhCN: "任务版本已变化，请刷新候选范围", EnUS: "The task version has changed. Refresh the candidate scope."}},
	// internal/platform/release_candidate_service.go:242
	{Key: "release.pagination.limit_invalid", Texts: map[Locale]string{ZhCN: "limit 应为 1–100", EnUS: "limit must be between 1 and 100."}},
	// internal/platform/release_candidate_service.go:248
	{Key: "release.pagination.object_invalid", Texts: map[Locale]string{ZhCN: "分页对象无效", EnUS: "The pagination object is invalid."}},
	// internal/platform/release_candidate_service.go:254
	{Key: "release.pagination.cursor_scope_invalid", Texts: map[Locale]string{ZhCN: "分页游标无效或不属于当前项目", EnUS: "The pagination cursor is invalid or does not belong to the current project."}},
	// internal/platform/release_deploy.go:148
	{Key: "release.request.format_invalid", Texts: map[Locale]string{ZhCN: "发布请求格式错误", EnUS: "The release request format is invalid."}},
	// internal/platform/release_deploy.go:164
	{Key: "release.provider.not_registered", Texts: map[Locale]string{ZhCN: "该项目尚未登记此发布提供方", EnUS: "This release provider has not been registered for the project."}},
	// internal/platform/release_deploy.go:176
	{Key: "release.provider.site_registration_changed", Texts: map[Locale]string{ZhCN: "该站点已不在管理员当前发布登记中", EnUS: "The site is no longer in the administrator's current release registration."}},
	// internal/platform/release_deploy.go:296
	{Key: "release.record.version_changed", Texts: map[Locale]string{ZhCN: "发布记录已变化，请刷新", EnUS: "The release record has changed. Refresh it."}},
	// internal/platform/release_deploy.go:424
	{Key: "release.environment.registration_required", Texts: map[Locale]string{ZhCN: "环境必须提供名称和管理员登记的提供方/站点", EnUS: "The environment requires a name and an administrator-registered provider and site."}},
	// internal/platform/release_deploy.go:437
	{Key: "release.environment.site_not_registered", Texts: map[Locale]string{ZhCN: "站点未登记", EnUS: "The site is not registered."}},
	// internal/platform/release_deploy.go:456
	{Key: "release.environment.site_identity_exists", Texts: map[Locale]string{ZhCN: "站点已绑定环境，归档也不复用身份", EnUS: "The site is already bound to an environment. Archiving does not allow its identity to be reused."}},
	// internal/platform/release_deploy.go:469
	{Key: "release.environment.version_changed", Texts: map[Locale]string{ZhCN: "环境版本已变化", EnUS: "The environment version has changed."}},
	// internal/platform/release_deploy.go:472
	{Key: "release.environment.operation_requires_reconciliation", Texts: map[Locale]string{ZhCN: "环境仍有占用操作，须先核对", EnUS: "The environment is still occupied by an operation. Reconcile it first."}},
	// internal/platform/release_deploy.go:475
	{Key: "release.environment.binding_immutable", Texts: map[Locale]string{ZhCN: "已创建环境不能换绑提供方或站点", EnUS: "An existing environment cannot be rebound to another provider or site."}},
	// internal/platform/release_deploy.go:492
	{Key: "release.environment.name_or_status_invalid", Texts: map[Locale]string{ZhCN: "环境名称或状态无效", EnUS: "The environment name or status is invalid."}},
	// internal/platform/release_deploy.go:541
	{Key: "release.plan.version_changed", Texts: map[Locale]string{ZhCN: "计划版本或内容已变化，请刷新后重新审核", EnUS: "The plan version or content has changed. Refresh and review it again."}},
	// internal/platform/release_deploy_authorization.go:54
	{Key: "release.authorization.approval_invalidated", Texts: map[Locale]string{ZhCN: "此操作的固定审批已失效", EnUS: "The fixed approval for this operation is no longer valid."}},
	// internal/platform/release_deploy_authorization.go:69
	{Key: "release.authorization.operation_or_assignment_invalid", Texts: map[Locale]string{ZhCN: "部署操作或执行身份无效", EnUS: "The deployment operation or execution identity is invalid."}},
	// internal/platform/release_deploy_authorization.go:94
	{Key: "release.authorization.original_attempt_requires_reconciliation", Texts: map[Locale]string{ZhCN: "此操作已冻结在原执行尝试，必须核对原操作", EnUS: "This operation is frozen to its original execution attempt. Reconcile the original operation."}},
	// internal/platform/release_deploy_authorization.go:105
	{Key: "release.authorization.provider_already_accepted", Texts: map[Locale]string{ZhCN: "提供方已受理原操作，请按 operation_id 查询，不重领副作用", EnUS: "The provider has accepted the original operation. Query it by operation_id; do not request authorization for its side effects again."}},
	// internal/platform/release_deploy_authorization.go:199
	{Key: "release.authorization.ticket_expired", Texts: map[Locale]string{ZhCN: "发布票据已过期", EnUS: "The release ticket has expired."}},
	// internal/platform/release_deploy_authorization.go:243
	{Key: "release.authorization.effect_generation_mismatch", Texts: map[Locale]string{ZhCN: "副作用代数不匹配", EnUS: "The side-effect generation does not match."}},
	// internal/platform/release_deploy_authorization.go:265
	{Key: "release.authorization.inspect_binding_invalid", Texts: map[Locale]string{ZhCN: "授权查询绑定无效", EnUS: "The authorization query binding is invalid."}},
	// internal/platform/release_deploy_http.go:90
	{Key: "release.request.json_depth_exceeded", Texts: map[Locale]string{ZhCN: "请求 JSON 层级超过128", EnUS: "The request JSON nesting depth exceeds 128."}},
	// internal/platform/release_deploy_http.go:110
	{Key: "release.request.json_duplicate_fields", Texts: map[Locale]string{ZhCN: "请求包含重复 JSON 字段", EnUS: "The request contains duplicate JSON fields."}},
	// internal/platform/release_deploy_http.go:128
	{Key: "release.request.json_structure_invalid", Texts: map[Locale]string{ZhCN: "JSON 结构无效", EnUS: "The JSON structure is invalid."}},
	// internal/platform/release_deploy_http.go:132
	{Key: "release.request.json_invalid", Texts: map[Locale]string{ZhCN: "请求 JSON 无效、层级超限或存在重复字段", EnUS: "The request JSON is invalid, exceeds the nesting-depth limit, or contains duplicate fields."}},
	// internal/platform/release_deploy_http.go:142
	{Key: "release.provider.request_unreadable", Texts: map[Locale]string{ZhCN: "提供方请求不可读或超限", EnUS: "The provider request is unreadable or exceeds the size limit."}},
	// internal/platform/release_deploy_http.go:150
	{Key: "release.provider.request_fields_invalid", Texts: map[Locale]string{ZhCN: "提供方请求存在未知字段或错误类型", EnUS: "The provider request contains unknown fields or incorrect types."}},
	// internal/platform/release_deploy_observations.go:28
	{Key: "release.observation.operation_binding_mismatch", Texts: map[Locale]string{ZhCN: "提供方观测与固定操作不匹配", EnUS: "The provider observation does not match the fixed operation."}},
	// internal/platform/release_deploy_observations.go:32
	{Key: "release.observation.digest_mismatch", Texts: map[Locale]string{ZhCN: "提供方观测摘要不匹配", EnUS: "The provider observation digest does not match."}},
	// internal/platform/release_deploy_observations.go:37
	{Key: "release.observation.operation_state_invalid", Texts: map[Locale]string{ZhCN: "提供方操作状态无效", EnUS: "The provider operation state is invalid."}},
	// internal/platform/release_deploy_observations.go:42
	{Key: "release.observation.effect_state_required", Texts: map[Locale]string{ZhCN: "必须显式提供实际副作用状态", EnUS: "The actual side-effect state must be provided explicitly."}},
	// internal/platform/release_deploy_observations.go:47
	{Key: "release.observation.health_state_required", Texts: map[Locale]string{ZhCN: "必须显式提供实际健康状态", EnUS: "The actual health state must be provided explicitly."}},
	// internal/platform/release_deploy_observations.go:50
	{Key: "release.observation.provider_error_too_long", Texts: map[Locale]string{ZhCN: "提供方错误信息过长", EnUS: "The provider error message is too long."}},
	// internal/platform/release_deploy_observations.go:54
	{Key: "release.observation.active_artifact_mismatch", Texts: map[Locale]string{ZhCN: "实际 active 指针不匹配本代制品", EnUS: "The actual active pointer does not match this generation's artifact."}},
	// internal/platform/release_deploy_observations.go:58
	{Key: "release.observation.not_started_active_conflict", Texts: map[Locale]string{ZhCN: "未开始观测与实际 active 指针矛盾", EnUS: "The not-started observation contradicts the actual active pointer."}},
	// internal/platform/release_deploy_observations.go:62
	{Key: "release.observation.health_artifact_mismatch", Texts: map[Locale]string{ZhCN: "健康观测不属于本代实际制品", EnUS: "The health observation does not belong to this generation's actual artifact."}},
	// internal/platform/release_deploy_observations.go:67
	{Key: "release.observation.health_time_or_duplicate_invalid", Texts: map[Locale]string{ZhCN: "健康检查重复或缺少实际时间", EnUS: "A health check is duplicated or lacks an actual observation time."}},
	// internal/platform/release_deploy_observations.go:78
	{Key: "release.observation.health_policy_mismatch", Texts: map[Locale]string{ZhCN: "实际 HTTP 内容或状态不匹配固定健康策略", EnUS: "The actual HTTP content or status does not match the fixed health policy."}},
	// internal/platform/release_deploy_observations.go:84
	{Key: "release.observation.failed_health_active_required", Texts: map[Locale]string{ZhCN: "失败健康观测也须绑定实际 active 制品", EnUS: "A failed health observation must also be bound to the actual active artifact."}},
	// internal/platform/release_deploy_observations.go:88
	{Key: "release.observation.failed_health_incomplete", Texts: map[Locale]string{ZhCN: "失败健康观测不完整", EnUS: "The failed health observation is incomplete."}},
	// internal/platform/release_deploy_observations.go:97
	{Key: "release.observation.failed_health_policy_mismatch", Texts: map[Locale]string{ZhCN: "失败健康观测不属于固定策略", EnUS: "The failed health observation does not belong to the fixed policy."}},
	// internal/platform/release_deploy_observations.go:102
	{Key: "release.observation.success_unproven", Texts: map[Locale]string{ZhCN: "部署成功需要实际激活、本代 HTTP 健康与持久 fencing ACK", EnUS: "Deployment success requires actual activation, HTTP health for this generation, and a durable fencing ACK."}},
	// internal/platform/release_deploy_observations.go:105
	{Key: "release.observation.cancel_unproven", Texts: map[Locale]string{ZhCN: "已发生或未知的副作用不能标为取消成功", EnUS: "Side effects that have occurred or whose outcome is unknown cannot be marked as successfully cancelled."}},
	// internal/platform/release_deploy_observations.go:174
	{Key: "release.observation.version_payload_conflict", Texts: map[Locale]string{ZhCN: "同观测版本已保存不同内容", EnUS: "Different content has already been saved for the same observation version."}},
	// internal/platform/release_deploy_observations.go:189
	{Key: "release.observation.terminal_immutable", Texts: map[Locale]string{ZhCN: "终态观测不可覆盖", EnUS: "A terminal observation cannot be overwritten."}},
	// internal/platform/release_deploy_observations.go:387
	{Key: "release.deployment.version_changed", Texts: map[Locale]string{ZhCN: "部署版本已变化", EnUS: "The deployment version has changed."}},
	// internal/platform/release_deploy_observations.go:415
	{Key: "release.deployment.reconciliation_scheduler_required", Texts: map[Locale]string{ZhCN: "核对管理执行需要发布调度器", EnUS: "Reconciling managed execution requires the release scheduler."}},
	// internal/platform/release_deploy_observations.go:444, internal/platform/release_deploy_observations.go:488
	{Key: "release.deployment.changed_during_reconciliation", Texts: map[Locale]string{ZhCN: "核对期间部署记录已变化，请刷新", EnUS: "The deployment record changed during reconciliation. Refresh it."}},
	// internal/platform/release_deploy_observations.go:457
	{Key: "release.deployment.original_assignment_missing", Texts: map[Locale]string{ZhCN: "缺少原执行身份", EnUS: "The original execution identity is missing."}},
	// internal/platform/release_deploy_observations.go:491
	{Key: "release.deployment.original_assignment_changed", Texts: map[Locale]string{ZhCN: "原执行身份已变化", EnUS: "The original execution identity has changed."}},
	// internal/platform/release_deploy_observations.go:514
	{Key: "release.provider.reconciliation_connection_missing", Texts: map[Locale]string{ZhCN: "未配置固定提供方核对连接", EnUS: "The fixed provider reconciliation connection is not configured."}},
	// internal/platform/release_deploy_observations.go:518
	{Key: "release.provider.reconciliation_loopback_required", Texts: map[Locale]string{ZhCN: "首轮核对仅允许管理员固定回环提供方", EnUS: "The initial reconciliation implementation only allows an administrator-fixed loopback provider."}},
	// internal/platform/release_deploy_observations.go:529
	{Key: "release.provider.unreachable_operation_occupied", Texts: map[Locale]string{ZhCN: "提供方不可达，原操作保持占用", EnUS: "The provider is unreachable. The environment remains reserved for the original operation."}},
	// internal/platform/release_deploy_observations.go:533
	{Key: "release.provider.original_operation_unavailable", Texts: map[Locale]string{ZhCN: "提供方尚未返回可核对的原操作记录", EnUS: "The provider has not yet returned a record of the original operation that can be reconciled."}},
	// internal/platform/release_deploy_observations.go:537
	{Key: "release.provider.observation_unreadable", Texts: map[Locale]string{ZhCN: "提供方观测超限或不可读", EnUS: "The provider observation exceeds the size limit or is unreadable."}},
	// internal/platform/release_deploy_observations.go:542
	{Key: "release.provider.observation_unstructured", Texts: map[Locale]string{ZhCN: "提供方返回非结构化观测", EnUS: "The provider returned an unstructured observation."}},
	// internal/platform/release_deploy_observations.go:603
	{Key: "release.deployment.provider_success_missing", Texts: map[Locale]string{ZhCN: "发布结果缺少真实提供方成功证据", EnUS: "The release result lacks actual provider evidence of success."}},
	// internal/platform/release_deploy_observations.go:620
	{Key: "release.deployment.original_execution_evidence_missing", Texts: map[Locale]string{ZhCN: "缺少原操作的完整执行证据", EnUS: "Complete execution evidence for the original operation is missing."}},
	// internal/platform/release_deploy_observations.go:624
	{Key: "release.deployment.current_observation_required", Texts: map[Locale]string{ZhCN: "发布结果不能代替当前持久提供方观测", EnUS: "The release result cannot replace the current durable provider observation."}},
	// internal/platform/release_deploy_observations.go:634
	{Key: "release.deployment.known_fenced_terminal_required", Texts: map[Locale]string{ZhCN: "提供方尚未证明已知且持久fencing的终态", EnUS: "The provider has not yet proved a known terminal state with durable fencing."}},
	// internal/platform/release_deploy_observations.go:637
	{Key: "release.deployment.unknown_or_failed_not_success", Texts: map[Locale]string{ZhCN: "未知或失败不能提升为成功", EnUS: "An unknown or failed outcome cannot be promoted to success."}},
	// internal/platform/release_deploy_observations.go:640
	{Key: "release.deployment.failed_reconciliation_mismatch", Texts: map[Locale]string{ZhCN: "失败核对与提供方事实不一致", EnUS: "The failure reconciliation does not match the provider facts."}},
	// internal/platform/release_deploy_plans.go:39
	{Key: "release.plan.input_invalid", Texts: map[Locale]string{ZhCN: "候选、环境和发布动作无效", EnUS: "The candidate, environment, or release action is invalid."}},
	// internal/platform/release_deploy_plans.go:49
	{Key: "release.environment.archived", Texts: map[Locale]string{ZhCN: "环境已归档", EnUS: "The environment is archived."}},
	// internal/platform/release_deploy_plans.go:52
	{Key: "release.environment.original_operation_barrier", Texts: map[Locale]string{ZhCN: "环境仍被原操作占用，必须先核对", EnUS: "The environment is still occupied by the original operation. Reconcile it first."}},
	// internal/platform/release_deploy_plans.go:55
	{Key: "release.environment.generation_changed", Texts: map[Locale]string{ZhCN: "环境代数已变化", EnUS: "The environment generation has changed."}},
	// internal/platform/release_deploy_plans.go:58
	{Key: "release.environment.generation_limit", Texts: map[Locale]string{ZhCN: "环境代数已到限制", EnUS: "The environment generation has reached its limit."}},
	// internal/platform/release_deploy_plans.go:69
	{Key: "release.plan.rollback_successful_history_required", Texts: map[Locale]string{ZhCN: "回退必须选择历史成功操作", EnUS: "A rollback must select a historically successful operation."}},
	// internal/platform/release_deploy_plans.go:80
	{Key: "release.plan.rollback_basis_invalid", Texts: map[Locale]string{ZhCN: "回退目标必须是同环境真实健康部署的原候选", EnUS: "The rollback target must be the original candidate of an actual healthy deployment in the same environment."}},
	// internal/platform/release_deploy_plans.go:83
	{Key: "release.plan.deploy_with_rollback_source", Texts: map[Locale]string{ZhCN: "普通部署不能传回退来源", EnUS: "An ordinary deployment must not include a rollback source."}},
	// internal/platform/release_deploy_plans.go:101
	{Key: "release.plan.health_file_missing", Texts: map[Locale]string{ZhCN: "健康策略要求的文件不在实际封存制品中", EnUS: "A file required by the health policy is absent from the actual sealed artifact."}},
	// internal/platform/release_deploy_plans.go:142
	{Key: "release.plan.preview_digest_required", Texts: map[Locale]string{ZhCN: "必须确认最新计划预览摘要", EnUS: "Confirm the digest of the latest plan preview."}},
	// internal/platform/release_deploy_plans.go:169
	{Key: "release.plan.preview_basis_changed", Texts: map[Locale]string{ZhCN: "预览依据已变化，请重新预览", EnUS: "The preview basis has changed. Preview the plan again."}},
	// internal/platform/release_deploy_plans.go:196
	{Key: "release.plan.digest_corrupt", Texts: map[Locale]string{ZhCN: "计划摘要损坏", EnUS: "The plan digest is corrupted."}},
	// internal/platform/release_deploy_plans.go:203
	{Key: "release.plan.environment_policy_or_binding_changed", Texts: map[Locale]string{ZhCN: "环境策略或绑定已变化", EnUS: "The environment policy or binding has changed."}},
	// internal/platform/release_deploy_plans.go:207
	{Key: "release.plan.environment_generation_or_occupancy_changed", Texts: map[Locale]string{ZhCN: "环境代数或占用已变化", EnUS: "The environment generation or occupancy has changed."}},
	// internal/platform/release_deploy_plans.go:211
	{Key: "release.plan.environment_barrier_mismatch", Texts: map[Locale]string{ZhCN: "环境屏障不属于此操作", EnUS: "The environment barrier does not belong to this operation."}},
	// internal/platform/release_deploy_plans.go:222
	{Key: "release.plan.candidate_basis_changed", Texts: map[Locale]string{ZhCN: "候选发布依据已变化", EnUS: "The candidate release basis has changed."}},
	// internal/platform/release_deploy_plans.go:268
	{Key: "release.plan.approval_invalid", Texts: map[Locale]string{ZhCN: "审批结论或理由无效", EnUS: "The approval decision or reason is invalid."}},
	// internal/platform/release_deploy_plans.go:298
	{Key: "release.plan.dispatched_approval_immutable", Texts: map[Locale]string{ZhCN: "已派发计划仅能撤销后续授权，不能覆盖历史审批", EnUS: "For a dispatched plan, only subsequent authorization may be revoked; historical approval cannot be overwritten."}},
	// internal/platform/release_deploy_plans.go:306
	{Key: "release.plan.no_approval_to_revoke", Texts: map[Locale]string{ZhCN: "计划尚无可撤销审批", EnUS: "The plan has no approval that can be revoked yet."}},
	// internal/platform/release_deploy_plans.go:353
	{Key: "release.plan.scheduler_missing", Texts: map[Locale]string{ZhCN: "发布调度器未配置", EnUS: "The release scheduler is not configured."}},
	// internal/platform/release_deploy_plans.go:387
	{Key: "release.plan.execution_receipt_mismatch", Texts: map[Locale]string{ZhCN: "执行回执不属于此计划", EnUS: "The execution receipt does not belong to this plan."}},
	// internal/platform/release_deploy_plans.go:396
	{Key: "release.plan.already_dispatched", Texts: map[Locale]string{ZhCN: "计划已派发，请读取已有部署", EnUS: "The plan has already been dispatched. Read the existing deployment."}},
	// internal/platform/release_deploy_plans.go:401
	{Key: "release.plan.managed_execution_receipt_mismatch", Texts: map[Locale]string{ZhCN: "回执与管理执行状态不一致", EnUS: "The receipt does not match the managed execution state."}},
	// internal/platform/release_deploy_plans.go:408
	{Key: "release.plan.fixed_approval_required", Texts: map[Locale]string{ZhCN: "此固定计划尚未批准", EnUS: "This fixed plan has not been approved yet."}},
}
