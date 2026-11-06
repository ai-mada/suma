# 远程工具受限准备、执行节点接纳与准确登记

速码1.0.0当前维护准备合同，负责人：主集成代理。T225已将[四种原生CLI](managed-cli-preparation.md)并入同一当前实现；原T224质量报告保留当时两ACP的证据范围。原T224建设基线为c3ef1cc9204c433372a971ac0a09f2173ed840c2；当前T225基于已核实并推送的1872774afc1b79c67fdac0b0b7a871f1d0261b23继续；完整31项／P0–P7与58limited／34pending／92总账继续active。本阶段接通机器执行通道，不把新文件、安装、版本、配置、当前库存、启用、模型执行或质量验收混为同一事实。

## 用户目的与现有原路径

浏览器在准确扫描结果上选择工具，按实际配置模式填写必要字段，明确授权本次机器配置写入；受限Runner准备并持久接纳新命令代次，上报准确库存，返回原Quick选择／暂停登记，独立启用。复用原维护owner／Journal／普通与Pool出站身份／事务回执／原typed ACP准备器／库存和派发器。不会下载、安装、运行任意shell、收取环境值、扩展任务AllowExec或代用户启用。

九项目录保留现有手工能力。自动准备支持 qoder-acp、cursor-cli、codex-jsonl、codebuddy-cli、dsh-multica、claude-code-cli。Codex App Server 在节点具备下文的准确本机配置时额外可用。能力由本机真实来源及明确 opt-in 声明。Qoder 允许空凭据引用，Cursor 要求引用，Claude CLI 固定模型且引用为空。Codex App Server 及其他三种原生配置的两个字段均为空。两种 SDK 继续使用现有配置入口。

### Codex App Server 换新

节点需允许执行，并在原项目范围的基础命令文件中提供符合固定协议的 Codex App Server 命令。节点发布最多 8 个经筛选的本机命令 ID，按名称稳定排序。没有可选项或数量超限时进入原手工指南。派生代次不作为模板。候选中有多项时必须明确选择，单项可直接确认。浏览器固定 `template_id`、原扫描和代次。接口只有在当前恰有一个候选时才接受省略，控制面在正式操作中固定所选 ID。

执行器按当前基础配置的准确 ID 复用本机供应方、凭据引用和沙箱策略，使用新扫描固定的程序路径及字节摘要。来源变更或缺少模型目录观察时保留原代次，处理原操作结果。公开候选只含命令 ID，秘密值与本机配置正文留在节点。任务模型选择与正式执行另行确认。已有已领取回执沿原意图恢复。缺少候选列表时可核对已保存记录，新的 Codex 准备须等待节点上报明确候选。

## 独立允许与准确来源

持续Runner／Pool增加本机--allow-tool-preparation，默认false，必须同时明确允许原installed-discovery。maintenance-only仍不领取业务；任务执行仍须原AllowExec与当前平台授权。原ToolMaintenanceCapability/Status新增managed_preparation{protocol:suma-managed-tool-preparation-v1,adapter_ids,generation_sha256}，绑定原instance与准确scope，不从scan/inspect权限推导写许可。

Create冻结当前完成扫描ID／版本、准确Item及原Scan Reference／SourceSHA／停止回执、当前expected_generation_sha256、配置与操作者。普通节点保原登记人和项目tool_manage；Pool保原机器owner、当前双边grant与消费项目route，不把池管理权变项目写权。浏览器不能传exe、路径、argv、bundle、manifest、目的文件或secret值。

原扫描增加只在机器Journal保存的不可变原始观察回执，绑定原ref/source与公开结果，并保存准确原生路径、完整本机依赖pins、版本及SHA。prepare仅消费该准确私有回执，重新静态核对文件身份与字节；缺失则明确rescan_required，不猜PATH、不复用同名新安装、不重跑原扫描。配置来源与原生文件变化在产生副作用前拒绝。

## 持久状态与两阶段完成

唯一公共合同internal/toolpreparationcontract；唯一域表suma_tool_preparations，原suma_operations持久操作和幂等回执复用。初始1.0.0 schema声明先完整precheck再创建，不回填历史、不重置用户数据。

状态pending、claimed、awaiting_inventory、completed、failed、unknown、closed_unknown、expired、superseded。pending300秒、claim60秒；claimed租约失联转unknown而非重派。unknown与awaiting_inventory保维护占用；与原inspection、scan、未确认inventory互斥。原持续维护owner轮询，不增加业务dispatcher或任意远程执行服务。

1. 先持久claim nonce；ACK丢失恢复同nonce。创建原key/body/route重放不依赖今天目录。
2. 核本机opt-in／scope／私有scan／完整原生pins／base generation；持久started和确定typed command intent。命令ID由准确操作派生，稳定且不覆盖原命令。
3. 原static builder／DecodeCommands／ValidateToolConfiguration产生全新私有配置；fsync/close，保存不可变generation与真正acceptance receipt，文件SHA、配置SHA与generationSHA分别保留。存在文件或hash相等本身不证明原操作完成。
4. 构建完整immutable Exec snapshot，原DescribeTools收集完整Profile和真实原生停止证明；失败不切head，保旧代次。
5. 原Journal锁内先持久新head及原待报正文，再原子publish snapshot，关闭此scope新claim门；旧attempt沿自己的旧snapshot继续。
6. /results的prepared只冻结generation receipt并进入awaiting_inventory。原ToolInventoryRequest增加可选Preparation绑定；原ReportToolInventory／Pool同事务核完整Profile、当前identity／instance／source和已保存prepared receipt，写准确库存并CAS completed，原ACK回显绑定+completed。原inventory操作ID/body持久，重放回原ACK不续库存TTL。
7. 本机核原ACK并持久确认后才开该scope新claim门。浏览器再当前重读准确库存，原matches／CreateToolProfile形成paused配置；enable单独确认。

post-POST断网不能回滚可能已被服务端接纳的代次；只恢复原结果／inventory正文。重启在原Journal锁内核scope/base Commands/head/manifest/receipt/pins后恢复同一immutable generation，再恢复pending ACK。started缺真实acceptance receipt保持unknown占用，不重writer、probe或换key。closed_unknown仅凭原可信停止／原ref证据记录未知，不声称回滚配置或提供派发资格。缺可靠证明时保原隔离与明确节点处置。

## 统一快照与并发边界

Worker.currentExecSnapshot用于runAssignment、DescribeTools、Info能力等所有读取，不原地改map。Pool按tenant/project隔离head与claim门，所有slot共享该scope snapshot；兄弟scope不因单项失败停用。base代次摘要绑定原Commands及本机必要资源，不把AllowExec／maintenance-only切换误作新工具内容。任务执行权限仍每次核验；在途assignment冻结原snapshot。原维护互斥与原未ACK业务／制品Journal保持，不清账。

## 共享类型与接口冻结

Protocol=suma-managed-tool-preparation-v1；StopProtocol=suma-managed-tool-preparation-stop-v1。公共long名ManagedToolPreparation系列；Go短名只用于实现方便，不是旧产品兼容。

- Capability{protocol,adapter_ids,generation_sha256}。Configuration{model,credential_env_ref}包含 `model`、`credential_env_ref`，仅 Codex App Server 可附 `template_id`。各 adapter 按当前 ConfigurationMode闭形校验，不忽略不适用的模型或凭据输入。
- CreateRequest{project_id,worker_id,installation_scan_id,expected_scan_version,adapter_id,configuration,expected_generation_sha256,acknowledge_configuration_write:true,idempotency_key}。
- Reference{preparation_id,claim_id,fence}；ClaimRequest{project_id,worker_id,claim_id}；PoolClaimRequest{tenant_id,project_id,runner_id,claim_id}；Claim{preparation:Operation|null}。
- Operation保存ID/project/worker/version/state/adapter/config、InstanceID、原scan及准确Item／ref／SourceSHA、ExpectedGenerationSHA、RequestedBy/timestamps、claim/fence与本操作SourceSHA；可选Receipt、ResultCode、InventoryOperationID／ObservedAt及只读SourceChanged。可变state/version/result与claim/lease字段不进入稳定SourceSHA；claim nonce/fence由准确Reference单独固定核对。
- GenerationReceipt{parent_generation_sha256,generation_sha256,command_id,artifact_sha256,local_receipt_sha256,profile,stop_proof}。Profile仍原toolcontract.Profile；ACP配置摘要从原Profile读取，不将artifactSHA当配置SHA。
- StopProof{protocol,preparation:Reference,source_sha256,receipt_sha256,probe_returned,all_native_trees_stopped,native_process_count,stopped_process_count,observed_at}；0计数必须真实且HTTP必填。
- ResultRequest{project_id,worker_id,preparation:Reference,source_sha256,idempotency_key,outcome:prepared|failed|unknown|closed_unknown,failure_code,receipt?,stop_proof?}；Pool相同机器范围换tenant/project/runner。
- InventoryBinding{reference:Reference,source_sha256,generation_sha256,local_receipt_sha256}。原ToolInventoryRequest／Receipt增加optional Preparation *InventoryBinding；Receipt增加PreparationState string。normal／Pool库存两路径对称。
- 浏览器GET/POST /tool-preparations及GET/{id}?project_id。Page{project_id,worker_id,as_of,maintenance,items,next_cursor?}沿原分页。机器normal／Pool各/tool-preparations/{claim,results}。

严格闭形、重复／别名／null／非整数／越界输入拒绝，三字段独立事实的未知恢复不被当前读覆盖。通用字段／Schema同源生成；不新增开放任意JSON选项。

## 简洁界面与准确登记pin

原RemoteToolScan结果处按实际cap按需展开model/envNAME与一次明确配置写确认。复用原扫描、准备、库存反馈及Quick，不堆新导航。按准备／接纳／库存／暂停登记分别表达，缺能力回原指南，失败明确原因与原核对入口。旧截止／撤权／SourceABA与原操作pending继续保护；中英文及原设计系统沿用。

ToolReportFeedbackSelection的profileSHA256/observedAt不再丢失；通过ephemeral pin传回原Quick，只有同工具／节点／完整ProfileSHA及准确当前新鲜观察吻合才登记。相同command_id换模型或SHA必须明确重新选择；未知已提交的原body/key仍优先核对，不用新目录重造。观察pin不声称某次复制文件被执行；本阶段机器generation/operation/inventory回执另行证明。

## 并行所有权与必要验收

Root：本合同、唯一共享DTO／校验器、ToolMaintenance sharedcap/status、platform ToolInventory types、数据库初始化及主HTTP hook、OpenAPI/generated、原生本地化／设计文档／最终集成与Git。控制代理：platform新tool_preparation*.go/store/HTTP/tests、维护busy交叉及normal/Pool inventory同Tx关联。执行代理：Runner/CLI私有scanreceipt／typedfactory／journal／generation／所有snapshot读取和客户端，原维护cap clone；交互代理：原扫描新panel/model、Quick准确pin、Workspace/原Guide薄接线、CNEN与focused tests。共享文件变更先报Root。

验证按必要边界：权限／opt-in／旧scan／source漂移；同scope维护排他与兄弟scope；same nonce/key/fence/result/inventory ACK丢失不重写或probe；崩溃前后generation恢复；在途旧snapshot；真实owned原生版本进程→新命令接纳→当前库存→HTTP暂停登记；至少一组CN/EN原生产组件代表状态。0真实厂商登录／模型／安装默认验收，不重播T223独测链或无关全量SDK套。真实供应方、余下三种自动准备provider、多机／跨系统／其他数据库／规模和生产按完整矩阵继续，完整目标不缩小。

补充冻结：Claim总带由当前机器认证生成的tenant_id/project_id/worker_id，preparation可null；无prepare cap只返回认证scope，不领取pending。仅已有managed head的恢复可用此只读身份结果核租户，默认无state且无opt-in不额外调用。Operation准确字段以internal/toolpreparationcontract/types.go为准，含TenantID及InstallationScanID/InstallationScanVersion/InstallationReference/InstallationSourceSHA256/InstallationItem/InstallationStopProof。Quick pin允许当前fresh observedAt晚于或等于原选择且完整SHA不变；不凭浏览器补续期或凭新timestamp改变配置选择。

来源资格补充：原ACP安装观察的launch_supported=false／acp_configuration_required是需要准备的准确状态，不改写为已可启动。当前 CanPrepareItem 对 ACP 两项只接受此状态。Codex App Server 只接受 `launch_supported=false` 且 `reason_code=configuration_required` 的准确原生扫描。其他四项需要 `launch_supported=true` 且 `reason_code=workspace_required`。均需完整版本／nativeSHA及原合法Item，Claude还需原受支持版本；version_unsupported等禁止。SourceSHA只摘要不可变输入，claim/fence/lease单独按Reference核对，首次领取不改变用户确认的输入来源。

维护调度的空回执与未确认请求独立处理。原领取返回显式空结果时，执行器按同一作用域完成原领取。当安装扫描或检查已占用维护槽，准备领取也在当前授权事务保存空回执，原领取重放仍为空。执行器可继续处理已有维护请求，下一次新领取使用新标识。页面创建和修改的互斥仍按原授权、版本与回执核对。

配置接纳与可执行性分开：maintenance-only不扩大AllowExec，新准备项可为Available=false但必须完整Profile；关联库存不强求available。原CreateToolProfile允许保存paused，Quick仅真实completed preparationID来源的准确当前配置可显示明确“保存暂停配置”，保持实际不可执行原因；普通反馈仍原available门。该pin需重新读取准确准备操作与完整Profile匹配，不能用id当权限；原enable／Start门保留。
