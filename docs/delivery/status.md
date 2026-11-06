# 1.0.0 当前交付状态

T-ADAPTER-DISCOVERY 工具目录默认浏览全部方向，支持按名称、入口和协议快速查找，并接回原配置流程。终端可用项目凭证查询同一目录。 [交互与命令](../design/adapter-connections.md#信息层级与异常恢复) · [验证](../quality/adapter-discovery-verification.md)

T-SCHEDULING-READ-RECOVERY 调度队列可重试原续页，撤权后清理旧记录与计数。尝试详情在恢复访问后沿原深链重新读取。 [交互](../design/scheduling-workspace.md#队列与尝试读取恢复) · [验证](../quality/scheduling-read-recovery-verification.md)

T-WORKBUDDY-CLI-PROGRESS 可用项目凭证在终端查询准确 WorkBuddy 节点的已保存进度。真实终端联调验证跨项目拒绝、撤权和零供应方调用。 [操作](../operations/task-dispatch-options.md#终端运行跟踪) · [验证](../quality/workbuddy-cli-progress-verification.md)

T-WORKBUDDY-TEAM-READS 团队成员可直接查看原云任务共享进度。原授权人核对私人账号时可先看进度，确认权限后再操作。 [交互](../design/workbuddy-cloud-task.md#更新记录与查询供应方) · [验证](../quality/workbuddy-cloud-status-verification.md#团队进度读取补充验证)

T-TOOL-QUICK-DESKTOP-251 添加工具可从准确暂停回执直达启用审阅，窄屏优先展示节点、版本、权限和确认操作。Windows 桌面已用真实 Codex CLI 跑通检测、选择、保存与启用。 [交互](../design/tool-quick-connect.md) · [验证](../quality/tool-quick-desktop-verification.md)

T-WORKBUDDY-DIRECT-AUTH 支持首次与重新授权直接打开 WorkBuddy。浏览器拦截时可打开原链接，超时或断开后定位原连接。真实供应方账号联调待部署配置后验收。 [交互](../design/workbuddy-connections.md) · [验证](../quality/workbuddy-direct-auth-verification.md)

T-CLI-RUN-WATCH 支持从终端跟随持久运行事件，从原序号续传，并查询当前项目队列及精确节点准入。真实联调验证连接轮换、事件去重和撤权退出。 [终端操作](../operations/task-dispatch-options.md#终端运行跟踪) · [验证](../quality/cli-run-watch-verification.md)

T-CLI-TASK-DISPATCH 从终端按当前项目授权读取工具选项、核对独立测试、预览、审阅及显式启动。重试使用原请求、版本和操作标识。 [终端操作](../operations/task-dispatch-options.md#终端任务派发) · [验证](../quality/cli-task-dispatch-verification.md)

T-TEST-DISPATCH-PREFLIGHT 在任务派发中选择独立测试工具时，先核对发布流程、开发档案、测试节点和当前执行路由。结果显示原配置版本与读取时间；条件变化时在同一行提供原因。 [交互](../design/test-dispatch-bindings.md#工作台与推荐) · [验证](../quality/test-dispatch-preflight-verification.md)

T-POOL-RUNNER-OCCUPANCY 从共享资源池的执行节点页查看已保留槽位和登记上限，区分执行中记录、待核对与未知占用。真实派发、保留和核对释放已定向验证。 [交互](../design/execution-pools.md#节点槽位占用) · [验证](../quality/pool-runner-occupancy-verification.md)

T-PORTFOLIO-CAPACITY 从项目管理按周对照已分摊任务工时与组织工作日历。可筛选已超配或待核对人员，展开任务依据，跳转原任务和有权工作日历。已验证跨周分摊、当前撤权和桌面／窄屏交互。 [交互](../design/project-portfolio.md#跨项目周负荷与容量) · [验证](../quality/project-portfolio-capacity-verification.md)

T-PACKAGE-REPAIR-RELEASE-255 可从已关闭包测试修复进入原候选表单，冻结本轮任务和制品证明。重开后保留候选历史并阻止旧候选继续正向发布。 [合同](../../contracts/release-control.md#修复交接到发布候选) · [验证](../quality/package-repair-release-verification.md)

T-PACKAGE-TEST-REPAIR-254 包测试问题沿原修复流程关单，核对原测试档案和应用源码范围，保留独立复验与原失败两份证据。 [合同](../../contracts/issue-repair.md) · [验证](../quality/package-test-repair-verification.md)

T-PACKAGE-TEST-ISSUE-253 当前独立包测试失败可从质量页登记正式问题，经原任务交付回流需求，按准确尝试去重。 [合同](../../contracts/test-plans.md) · [验证](../quality/package-test-issue-verification.md)

T-POOL-MIXED-QUALITY-252 已发布质量计划支持共享池同一运行的源码用例与独立包测试。质量页分别展示门禁和原件，正式验收重核两类证据。 [合同](../../contracts/test-plans.md) · [验证](../quality/pool-mixed-quality-verification.md)

T-POOL-APPLICATION-ARTIFACTS-251 共享池可构建、封存、下载并独立测试同一应用制品。当前授权变更时拒绝旧派发的传输回执和新的包测试证明。 [执行池合同](../../contracts/execution-pools.md#应用构建制品与独立包测试) · [验证](../quality/pool-application-artifacts-verification.md)

T-APPLICATION-SCOPED-BUILD-250 构建证明固定应用版本、Git 子树、提交与合并回执。制品、发布候选和集合可追溯该范围，预览拒绝同提交的串用候选。 [操作](../development/application-release-sets.md#从应用源码版本构建) · [验证](../quality/application-scoped-build-verification.md)

T-REPOSITORY-APPLICATION-HANDOFF-249 已确认的受管 Git 回执可直接交接到匹配应用的源码登记。预览、确认及登记保留同一提交、原回执和源码树。 [操作](../development/project-applications.md#从原合并回执登记源码版本) · [验证](../quality/repository-application-handoff-verification.md)

T-REPOSITORY-SOURCE-INTEGRATION-248 从当前正式验收直接打开仓库集成。已用同一 Git 候选完成独立测试、预演、批准、受管分支快进与原回执核对。 [验证](../quality/repository-source-integration-verification.md)

T-REPOSITORY-SOURCE-POOL-247 验证两个实际 Windows Pool Runner 进程分别导入候选与测试同一提交。Git 包读写核对当前授权版本，执行结束后槽位释放。 [部署](../deployment/repository-integration.md#在共享执行池交接仓库候选) · [验证](../quality/repository-source-pool-verification.md)

T-ORG-WORK-CALENDAR 从组织设置维护当前成员的周工作制与日期例外，并按周读取已保存的可用工时。已验证管理员编辑、本人只读、并发保存及原回执。 [交互](../design/organization-work-calendar.md) · [验证](../quality/organization-work-calendar-verification.md)

T-PORTFOLIO-SCHEDULE-OVERLAP 从项目组合按周核对同一成员在多个可读项目的排期重叠，展开确切日期和原任务。已验证周切换、任务返回与当前撤权隔离。 [交互](../design/project-portfolio.md#跨项目排期重叠) · [验证](../quality/project-schedule-overlap-verification.md)

T-REPOSITORY-SOURCE-WORKBENCH-246 任务工作台可核对基线、批准留存、选择不同 Worker、预览并确认启动。未知响应沿原操作恢复。 [交互](../design/repository-source-workbench.md) · [验证](../quality/repository-source-workbench-verification.md)

T-ORG-PROJECT-PORTFOLIO 在项目管理汇总当前组织内有权项目的验收、阻塞、活动风险和待完成里程碑。服务端搜索、分页和跨项目权限已验证。 [交互](../design/project-portfolio.md) · [验证](../quality/project-portfolio-verification.md)

T-REPOSITORY-SOURCE-DISPATCH-245 项目审批人可将留存仓库来源派发为受管候选，Runner 隔离封存原包，另一 Runner 独立测试同一提交，再沿新运行验收。 [合同](../../contracts/workbuddy-task-dispatch.md#派发已批准仓库来源的候选和独立测试) · [验证](../quality/repository-source-dispatch-verification.md)

T-TOOL-NODE-MAINTENANCE 工具管理按当前项目权限展示凭证维护。常用配置与检测直接可达，轮换和撤销集中处理。撤权后关闭旧写入确认，撤销后保留节点历史。 [交互](../design/tool-identity-management.md#节点操作与凭证维护) · [验证](../quality/tool-node-maintenance-verification.md)

T-REPOSITORY-SOURCE-INTAKE-244 项目审批人可按确切基线及提交保留 WorkBuddy 待交付仓库的 Git 包。审批回执绑定原运行、节点和尝试，Runner 严格导入已独立验证。 [合同](../../contracts/workbuddy-task-dispatch.md#批准并保留确切仓库来源) · [验证](../quality/repository-source-intake-verification.md)

T-REPOSITORY-BUNDLE-EXPORT-243 受管仓库提供方可从固定入站分支按管理员基线导出精确 Git 包，已在独立仓库验证并导入同一提交。 [部署](../deployment/repository-integration.md#启用登记分支-git-包导出) · [验证](../quality/repository-bundle-export-verification.md)

T-WORKBUDDY-REPOSITORY-SOURCE-242 在待交付尝试上按需核对项目已登记仓库的实时分支提交。读取前后检查任务、原执行和目标版本，候选与验收各自保留依据。 [合同](../../contracts/workbuddy-task-dispatch.md#核对已登记仓库的分支提交) · [验证](../quality/workbuddy-repository-source-verification.md)

T-WORKBUDDY-MANAGE-UX 按连接状态突出刷新、替换链接或只读核对，按需展开账号管理。重新授权可直接沿用明确列出的权限，刷新提示与实际连接状态一致。 [交互](../design/workbuddy-connections.md) · [验证](../quality/workbuddy-manage-ux-verification.md)

T-WORKBUDDY-LATE-DELIVERY-240 可从原运行刷新云任务，将可推进的原尝试更新为待交付并开放准确定位。原执行结果与验收事实保留。 [合同](../../contracts/workbuddy-task-dispatch.md#迟到的云端完成观察) · [验证](../quality/workbuddy-late-delivery-verification.md)

T-TOOL-QUICK-CHOICES-241 本机已确认的入口可直接选用，同名产品按协议类型区分。多份节点配置显示读写范围，单份已有配置直接展示可用操作。 [交互](../design/tool-quick-connect.md) · [验证](../quality/tool-quick-choices-verification.md)

T-WORKBUDDY-CONNECTION-UX 将首次连接简化为直接选权限、在 WorkBuddy 确认。焦点随步骤移动，授权核对和账号管理分区展示。真实 WorkBuddy 应用联调仍待配置后验收。 [交互](../design/workbuddy-connections.md) · [验证](../quality/workbuddy-connection-ux-verification.md)

T-TOOL-QUICK-REVIEW-240 添加工具支持首屏配置核对、回车保存及暂停回执焦点。检测详情可展开，缺失或过期时直接显示处理入口。 [交互](../design/tool-quick-connect.md) · [验证](../quality/tool-quick-review-verification.md)

T-WORKBUDDY-ACCOUNT-RECOVERY 支持在原连接重新生成待兑换链接，并在浏览器返回后核对同一授权代次。未启用时显示部署要求，Compose 和 Helm 可引用服务端秘密。真实 WorkBuddy 应用联调与部署验收待完成。 [交互与配置](../design/workbuddy-connections.md) · [验证](../quality/workbuddy-account-recovery-verification.md)

T-WORKBUDDY-LOCAL-CANDIDATE-239 支持从等待交付的正式 WorkBuddy 尝试继续本机候选，保留原云来源，按新运行封存、独立测试和验收。 [合同](../../contracts/terminal-recovery-candidates.md#从原-workbuddy-尝试继续本机开发) · [验证](../quality/workbuddy-local-candidate-verification.md)

T-TOOL-QUICK-START-239 简化添加工具。桌面端对唯一确认可启动的入口预填选择，自动接续当前项目的唯一节点和可用配置。检测结果可展开，登记仍需明确确认并保存为暂停。 [交互](../design/tool-quick-connect.md) · [验证](../quality/tool-quick-start-verification.md)

T-MULTICA-INBOX-BATCH 支持当前项目及组织范围内已选通知的批量处理。逐条核对权限和版本后整批写入，并保存原操作回执。 [交互](../design/notification-inbox.md#批量处理已选通知) · [验证](../quality/inbox-batch-verification.md)

T-MULTICA-RUN-FAILURE-INBOX 正式任务运行失败时通知投影时有权的关注者和人员负责人。项目和组织收件箱均可打开原运行，保留原问题与回执。 [交互](../design/notification-inbox.md#执行失败的处理入口) · [验证](../quality/run-failure-inbox-verification.md)

T-MULTICA-ORGANIZATION-INBOX 在开发收件箱汇总组织内当前可读项目的个人通知。支持准确来源定位、原项目已读／归档，以及权限变化后的安全重读。 [交互](../design/notification-inbox.md#组织内跨项目处理) · [验证](../quality/organization-inbox-verification.md)

T-MULTICA-SHARED-VIEW-RECOVERY 支持刷新后核对原视图写入。同标签页恢复只在明确操作后发送原请求，重新读取最新目录并保留当前筛选。 [交互设计](../design/shared-task-views.md#保存中断与页面恢复) · [验证](../quality/shared-view-recovery-verification.md)

T-MULTICA-SHARED-VIEW-LIVE 支持共享任务视图的在线版本核对。另一设备修改时保留当前筛选，显式应用新版本。断网、归档和撤权分别处理。 [交互设计](../design/shared-task-views.md#在线版本核对与显式应用) · [验证](../quality/shared-view-live-verification.md)

T-WORKBUDDY-DELIVERY-TRACE-238 在交付与质量面板直接定位当前任务绑定的 WorkBuddy 待交付尝试，覆盖通用列表之外的节点。 [合同](../../contracts/task-delivery-chain.md) · [验证](../quality/workbuddy-delivery-trace-verification.md)

T-MULTICA-TASK-BATCH-CLI 在原速码命令行提供任务优先级和计划状态的预览与提交。使用同一项目权限与原操作回执。 [CLI 合同](../api/cli-task-batch-changes.md) · [验证](../quality/task-batch-cli-verification.md)

T-WORKBUDDY-HISTORY-ARTIFACTS-237 可在原授权会话的已保存历史中回看 WorkBuddy 产物类型和变更动作，沿现有私有记录读取。 [合同](../../contracts/workbuddy-cloud-collaboration.md) · [验证](../quality/workbuddy-history-artifacts-verification.md)

T-MULTICA-TASK-PLANNING-STATUS 支持列表和看板批量调整计划状态。逐项核对原运行及验收资格，符合条件时整批更新并可恢复原回执。 [合同](../../contracts/task-planning-status-changes.md) · [验证](../quality/task-planning-status-changes-verification.md)

T-TASK-DISPATCH-ENVELOPE-236 核对派发预览的固定上下文、工具和知识引用。打开历史预览时按当前任务与计划读取，异常读取隐藏启动入口。 [合同](../../contracts/task-dispatch-options.md) · [验证](../quality/task-dispatch-envelope-verification.md)

T-MULTICA-TASK-PRIORITY 支持任务列表和看板批量调整优先级。逐项预览、整批核对版本并保留既有验收证据，支持刷新后恢复原回执。 [合同](../../contracts/task-priority-changes.md) · [验证](../quality/task-priority-changes-verification.md)

T-TASK-DISPATCH-CHOICE-235 支持从兼容结果直接选用任意兼容配置，并在确认预览时校验原任务、流程、节点和工具版本。 [合同](../../contracts/task-dispatch-options.md) · [验证](../quality/task-dispatch-choice-verification.md)

T-CANDIDATE-TRANSFER-RECOVERY 在工具管理中核对手动登记节点的未封存 Git 包。原尝试确认失败或取消后，授权操作者可结束传递并保留原证据，再单独核对凭证轮换。 [合同](../../contracts/manual-worker-candidate-recovery.md) · [验证记录](../quality/candidate-transfer-recovery-verification.md)

T-CURSOR-EDITOR-234 在 Windows 本机工作区提供“在 Cursor 中打开”，沿当前项目与原 Git 目录绑定发出编辑器启动请求。 [合同](../../contracts/desktop-workspace.md#windows-原工作区在-cursor-中打开) · [验证](../quality/cursor-editor-launch-verification.md)

T-ARTIFACT-UPLOAD-RECOVERY 为手动节点增加未封存发布制品核对。以精确原尝试和围栏为依据结束上传，保留证据，凭证轮换由用户另行确认。 [合同](../../contracts/manual-worker-artifact-upload-recovery.md) · [验证记录](../quality/artifact-upload-recovery-verification.md)

T-POOL-DIAGNOSTIC-GUIDE-233 在执行节点行加入本机命令与离线槽位预览。摘要只在当前浏览器会话中处理，按异常槽位继续核对原执行。 [交互](../design/execution-pools.md#节点本机诊断) · [验证](../quality/pool-diagnostic-guide-verification.md)

T-MANUAL-RUNNER-ROTATION 支持工具管理中的手动登记节点凭证轮换，核对当前授权、原版本及未决执行后沿原节点签发新凭证。 [合同](../../contracts/manual-runner-rotation.md) · [验证记录](../quality/manual-runner-rotation-verification.md)

T-POOL-INSPECTION-232 新增本机共享执行池总览，按槽位查看日志计数、未知结果和原领取阶段，并沿单槽位命令定位原尝试。 [诊断指南](../agents/runner-journal-inspection.md) · [阶段验证](../quality/pool-state-inspection-verification.md)

T-POOL-SHUTDOWN-231 将共享池退出结果绑定到原槽位、库存管理和状态锁收尾。遇到原执行未知时保留原派发并输出槽位诊断。 [值班手册](../agents/execution-pool-runner.md#停止与核对) · [阶段验证](../quality/pool-shutdown-verification.md)

T-MULTICA-BOARD-SELECTION 将列表和看板的多选与批量操作统一，支持在跨泳道选择后分配负责人、调整迭代或安排团队。已验证触屏滚动和正式事务。 [交互设计](../design/task-board-selection.md) · [验证记录](../quality/task-board-selection-verification.md)

T-DESKTOP-STOP-RECOVERY-230 允许桌面重开后核对原停止操作。匹配同一节点、操作与退出证据后恢复手动连接入口，继续保留原任务记录。 [阶段验证](../quality/desktop-stop-recovery-verification.md)

T-RUNNER-STOP-229 将本机停止、原任务结果和控制面回执分开确认。桌面仅在收到本次子进程的匹配回执并核实正常退出后确认停止，重连沿用原节点和持久请求。 [阶段验证](../quality/runner-stop-receipt-verification.md)

T-MULTICA-ITERATION-BATCH 在任务列表支持逐项预览和批量调整迭代，按当前任务、排期和目标版本原子提交，支持冲突刷新和原回执恢复。 [交互设计](../design/task-iteration-assignments.md) · [验证记录](../quality/task-iteration-assignments-verification.md)

T-CURSOR-RECOVERY-228 为 Cursor 前台停止增加有界等待和原尝试核对。普通节点与共享执行池已验证丢失回执后的同请求恢复，运行界面支持明确记录当前尝试结论。 [验证记录](../quality/cursor-foreground-recovery-verification.md)

T-WORKBUDDY-ARTIFACT-TIMELINE 已在原云回合显示产物类型、变更动作和有限预览。 [合同](../../contracts/workbuddy-cloud-turns.md) · [验证](../quality/workbuddy-artifact-timeline-verification.md)

T-CODEX-DELIVERY-227 已贯通 Windows 真实 Codex 的固定模型与用量提醒、持久进度、候选封存、同提交独测和隔离复核。Linux 可选 PID 命名空间已通过交叉编译，实机待验。 [接入指南](../integrations/codex-candidate-delivery.md) · [验证记录](../quality/codex-candidate-delivery-verification.md)

T-TOOL-IDENTITY-MANAGEMENT：[节点与账号管理设计](../design/tool-identity-management.md)／[验证记录](../quality/tool-identity-management-verification.md)。节点凭证筛选、按节点配置、撤销历史，以及 WorkBuddy 权限差异与断开历史已通过代表验证。

T-REAL-CLI-ONBOARDING-226：[产品](../product/real-cli-onboarding.md)／[架构与接入](../integrations/real-cli-onboarding.md)／[交互](../design/real-cli-onboarding.md)／[限定验证](../quality/real-cli-onboarding-verification.md)。真实 Codex CLI 版本／帮助、原准备和库存确认已贯通至暂停登记；三字段诊断提供准确指南与明确重检，未知停止保留现场。私有探测配置保真，中英文完整，完整31项／P0–P7继续。

T-MANAGED-CLI-PREPARATION-225：[已安装CLI快速接入](../product/managed-cli-preparation.md)／[架构与AI SOP](../architecture/managed-cli-preparation.md)／[简洁交互](../design/managed-cli-preparation.md)／[限定验证](../quality/managed-cli-preparation-verification.md)。新增Codex JSONL、CodeBuddy、dsh和Claude CLI，当前六种自动工厂复用原准备／库存／暂停登记；准确来源进入摘要并在执行前复核。两代隔离配置实跑及中英文代表界面已验，厂商／模型和完整31项／P0–P7继续。

T-MANAGED-TOOL-PREPARATION-224：[远程工具快速准备与接入](../product/managed-tool-preparation.md)／[架构与AI SOP](../architecture/managed-tool-preparation.md)／[简洁交互](../design/managed-tool-preparation.md)／[限定验证](../quality/managed-tool-preparation-verification.md)。准确扫描后明确模型与机器写入许可，原Runner接纳新配置并确认库存，再保存暂停；同一原操作恢复两种丢失回执。中英文代表组件已核验；当前自动准备仅Qoder／Cursor，真实厂商和完整31项／P0–P7继续。

T-COLLABORATIVE-TEST-BINDINGS-223：[协作测试配置与工具自动核对](../product/collaborative-test-bindings.md)／[架构与AI SOP](../architecture/collaborative-test-bindings.md)／[交互](../design/collaborative-test-bindings.md)／[限定验证](../quality/collaborative-test-bindings-verification.md)。讨论、推荐采用及团队计划保留准确独测配置；工具按原live开关可见到期自动核对，后台刷新保留原请求，失败明确重试。真实同提交开发／独测和中英文代表组件分别核验；完整31项／P0–P7继续。

T-ACCEPTED-TASK-BASELINE-222：[已验收任务交给后续开发](../product/accepted-task-baseline.md)／[架构与AI SOP](../architecture/accepted-task-baseline.md)／[交互](../design/accepted-task-baseline.md)／[限定验证](../quality/accepted-task-baseline-verification.md)。普通智能体候选经准确独测与Review后，可明确采用为未开始任务的基线，沿原broker和独立工作树继续开发；不继承旧验收、不自动开始。完整31项／P0–P7继续。

T-QODER-TOOL-PREPARATION-221：[Qoder配置准备与AI SOP](../product/qoder-tool-preparation.md)／[交互设计](../design/qoder-tool-preparation.md)／[限定验证](../quality/qoder-tool-preparation-verification.md)。明确模型和本机信息沿原ACP准备器创建新文件，再采用准确普通字段或共享本机引用；手工JSON按需展开，来源切换可靠隔离。准备不运行Qoder或模型，真实发现／登记／启用分别核验。完整31项／P0–P7继续。

T-LOCAL-TOOL-PREPARATION-220：[通用CLI准备与AI SOP](../product/local-tool-preparation.md)／[交互设计](../design/local-tool-preparation.md)／[限定验证](../quality/local-tool-preparation-verification.md)。四种通用CLI使用Shell与全新文件两字段，复用原发现／准备命令；手工JSON按需展开，明确采用路径接续普通字段或共享引用。复制、被动探测、文件准备、登记和模型执行分别记录。完整31项／P0–P7继续。

T-SHARED-TOOL-ONBOARDING-219：[共享工具接入与 AI SOP](../product/shared-tool-onboarding.md)／[交互设计](../design/shared-tool-onboarding.md)／[限定验证](../quality/shared-tool-onboarding-verification.md)。共享安装检测后的准备使用准确Pool身份，保留工具、节点和项目返回原Quick；新接入不制造旧档案。明确授权失效撤下旧表单，到期保留输入；跨租户离线配置与项目读取权分离。完整31项／P0–P7继续。

T-TEAM-WORKSTATION-RECOVERY-218：[共享节点恢复与 AI SOP](../product/team-workstation-recovery.md)／[交互设计](../design/team-workstation-recovery.md)／[限定验证](../quality/team-workstation-recovery-verification.md)。工具诊断可进入准确资源池、节点和项目配置，再返回原工具换新；核对到期保留输入，来源失效阻止迟到配置。桌面默认留下，实际关闭后才进入原收尾。中英文代表界面及原生窗口已限定验证；真实远程修复、完整31项／P0–P7继续。

T-TOOL-RECOVERY-NAVIGATION-217：[工具恢复交接与AI SOP](../product/tool-recovery-navigation.md)／[交互](../design/tool-recovery-navigation.md)／[限定验证](../quality/tool-recovery-navigation-verification.md)。常用操作前置，Doctor→准确普通节点→原连接准备／上报／Quick贯通；来源ABA、截止、共享边界与原未知保护保持。5项定向、类型0／9774对词条、4组代表合成界面通过；1次构建，失败与宿主退出分开记录。真实远程修复及完整31／P0–P7继续。

T-TEST-DISPATCH-BINDINGS-216：[明确开发／独立测试工具与AI SOP](../product/test-dispatch-bindings.md)／[交互](../design/test-dispatch-bindings.md)／[限定验证](../quality/test-dispatch-bindings-verification.md)。原派发按需选择并冻结另一Worker测试工具，准确来源及同提交质量门贯通；真实Git／双Worker／JUnit及夹具Review通过，SQLite取消泄漏沿官方稳定驱动修复。25项主要定向及新版存储／关键平台检查通过；全包曾超时，未声称整包或完整31／P0–P7完成。

T-COMMAND-COMPOSITION-215：[已有命令配置组合与AI SOP](../product/command-composition.md)／[交互](../design/command-composition.md)／[限定验证](../quality/command-composition-verification.md)。跨文件checks沿原完整校验生成新配置，明确采用输出路径接回原连接指南；13项定向、9739对词条及实际PowerShell组合／拒覆盖通过。完整类型首次仅新mock3诊断，修正后5文件语义0；完整31／P0–P7继续。

T-TASK-DELIVERY-NAVIGATION-214：[交付准确导航与AI SOP](../product/task-delivery-navigation.md)／[交互](../design/task-delivery-navigation.md)／[限定验证](../quality/task-delivery-navigation-verification.md)。团队、问题、回流需求及原Review入口贯通；10项定向、类型0／9715对词条和一次双语代表组件通过。验收定位准确消费、异步Run选择及原恢复保留，完整31／P0–P7继续。

T-TOOL-TASK-HANDOFF-213：[准确工具到任务](../product/tool-task-handoff.md)／[架构与AI SOP](../architecture/tool-task-handoff.md)／[交互](../design/tool-task-handoff.md)／[限定验证](../quality/tool-task-handoff-verification.md)。工具详情→当前任务重读→原派发准确采用→固定预览贯通；9项定向、类型0／9708对词条、最终真实HTTP／SQLite与双语代表界面通过。六次实际尝试及前五次失败均保留；未启动任务／模型，完整31／P0–P7继续。

T-POOL-CONNECTION-212：[机器连接与准确项目工具](../product/pool-connection.md)／[架构与AI SOP](../architecture/pool-connection.md)／[交互](../design/pool-connection.md)／[限定验证](../quality/pool-connection-verification.md)。单次原身份检查、独立事实读取和准确工具交接贯通；13项定向、类型0／9675对词条、真实检查及原库Worker只读执行通过。完整31／P0–P7继续。

T-POOL-PREPARATION-DIAGNOSTICS-211：[本机诊断与定位](../product/pool-preparation-diagnostics.md)／[限定验证](../quality/pool-preparation-diagnostics-verification.md)。七字段安全报告、准确输入关联和双语静态建议贯通；9项定向、类型0／9618对词条、真实CLI与只读诊断区通过。未运行模型或业务任务，原31／P0–P7继续。

T-POOL-CONFIG-SCOPE-210：[项目切换隔离验证](../quality/pool-config-scope-verification.md)。修复新项目沿用旧本机配置的风险；1项RED→GREEN、窄类型0和5断点只读界面通过，0重复Go／CLI／业务。

T-POOL-CONFIG-PREPARATION-209：[多项目配置准备](../product/pool-config-preparation.md)／[限定验证](../quality/pool-config-preparation-verification.md)。准确节点与已加载授权选择、9字段本机引用、全新原Pool配置和错误定位贯通；9项定向、类型0／9594对词条、真实PowerShell→CLI及双语代表界面通过。未启动任务或启用工具，完整31／P0–P7继续。

T-POOL-RUNNER-PAIRING-208：[Pool机器配对](../product/pool-runner-pairing.md)／[架构与AI SOP](../architecture/pool-runner-pairing.md)／[交互](../design/pool-runner-pairing.md)／[English](../en/pool-runner-pairing.md)／[限定验证](../quality/pool-runner-pairing-verification.md)。独立系统凭据、原子Pool登记与原ACK恢复贯通；26个不同检查、类型／9548对词条、实际Windows凭据1创建1清理与三ACK、双语工作台及只读简化提案限定通过。无项目Grant／业务／模型，完整31／P0–P7继续。

T-OPENCODE-PREPARATION-207：[OpenCode快捷准备](../product/opencode-tool-preparation.md)／[架构与AI SOP](../architecture/opencode-tool-preparation.md)／[交互](../design/opencode-tool-preparation.md)／[English](../en/opencode-tool-preparation.md)／[限定验证](../quality/opencode-tool-preparation-verification.md)。九字段静态新文件准备、原准确节点指南与配置回接贯通；14个不同定向检查、完整类型0／9476对词条及真实PowerShell→Go文件创建／拒绝覆盖、双语代表界面限定通过。未运行供应方／模型，完整31／P0–P7继续。

T-TOOL-MANAGEMENT-EXPERIENCE-206：[工具管理交互](../design/tool-management-experience.md)／[English](../en/tool-management-experience.md)／[限定验证](../quality/tool-management-experience-verification.md)。首屏单一添加与列表前置、更多接入按需展开、原准确节点指南和焦点交接贯通；6个不同检查（3新／2直接旧／1复用）、完整Web类型0、9471对词条及3组代表整页合成界面限定通过。真实厂商与完整31／P0–P7继续。

T-REMOTE-TOOL-SCAN-205：[远程节点工具检测](../product/remote-tool-installation-scan.md)／[架构与AI SOP](../architecture/remote-tool-installation-scan.md)／[交互](../design/remote-tool-installation-scan.md)／[English](../en/remote-tool-installation-scan.md)／[限定验证](../quality/remote-tool-installation-scan-verification.md)。固定目录、明确机器允许、原维护owner与持久停止证明贯通，空commands检测后回准确Guide；29个不同检查、真实双ACK恢复／2原生树／0任务及最终双语只读视觉限定通过。完整31／P0–P7继续。

T-TOOL-REPORT-FEEDBACK-204：[工具上报反馈](../product/tool-report-feedback.md)／[架构与AI SOP](../architecture/tool-report-feedback.md)／[交互](../design/tool-report-feedback.md)／[English](../en/tool-report-feedback.md)／[限定验证](../quality/tool-report-feedback-verification.md)。当前清单、限时等候及准确Quick返回贯通，旧ACK／时钟回退／过期与撤权分别处理；18个不同定向检查、9430对词条、真实HTTP／SQLite和两组双语代表界面限定通过。完整31／P0–P7继续。

T-TOOL-PREPARATION-203：[工具准备与恢复](../product/tool-preparation-recovery.md)／[架构与AI SOP](../architecture/tool-preparation-recovery.md)／[交互](../design/tool-preparation-recovery.md)／[English](../en/tool-preparation-recovery.md)／[限定验证](../quality/tool-preparation-recovery-verification.md)。两SDK准确准备原因、当前权限与原回执优先、九协议／同节点指南及原生Codex配置生成贯通；29个不同定向检查、双语代表组件与一次真实配置生成限定通过。完整31／P0–P7继续。

T-TOOL-DISCOVERY-202：[本机工具发现](../product/local-tools-discovery-experience.md)／[架构与AI SOP](../architecture/local-tools-discovery-experience.md)／[交互](../design/local-tools-discovery-experience.md)／[English](../en/local-tools-discovery-experience.md)／[限定验证](../quality/local-tools-discovery-experience-verification.md)。已发现、检测异常和当前选择保持首屏，其余工具键盘展开；准确原入口、44px触达及跨组焦点贯通。10个不同定向检查和两组双语代表组件验证通过，完整31／P0–P7继续。

T-CLAUDE-SDK-CONNECT-201：[Claude SDK快捷接入](../product/claude-sdk-quick-connect.md)／[架构与AI SOP](../architecture/claude-sdk-quick-connect.md)／[交互](../design/claude-sdk-quick-connect.md)／[English](../en/claude-sdk-quick-connect.md)／[限定验证](../quality/claude-sdk-quick-connect-verification.md)。原生自动发现、两个公开字段、受信资源、原节点恢复与新鲜库存接回暂停登记；38个不同定向检查、受控真实HTTP／Worker链和双语代表界面通过。真实厂商模型、完整桌面／跨平台与原31／P0–P7继续。

T-TOOL-REPLACEMENT-200：[配置换新](../product/tool-profile-replacement.md)／[架构与AI SOP](../architecture/tool-profile-replacement.md)／[数据](../data/tool-profile-replacement.md)／[交互](../design/tool-profile-replacement.md)／[限定验证](../quality/tool-profile-replacement-verification.md)。一次确认完成准确旧→新原子交接、可选暂停、原回执恢复与双向历史；21不同定向、真实22HTTP及双语代表界面通过，完整31／P0–P7继续。

T-TOOL-DOCTOR-199：[合同](../../contracts/tool-doctor-maintenance.md)／[PRD](../product/tool-doctor-maintenance.md)／[架构与AI SOP](../architecture/tool-doctor-maintenance.md)／[限定验证](../quality/tool-doctor-maintenance-verification.md)。准确维护建议、原检查／核对进度、跨配置定位与CLI双语表格贯通；29不同定向、真实22HTTP及代表界面限定通过，完整31／P0–P7继续。

T-TOOL-EXPERIENCE-198：[工具管理需求](../product/tool-management.md)／[交互](../design/tool-management.md)／[限定验证](../quality/tool-experience-verification.md)。准确节点贯通高级配置与原命令，公开原始原因配准确配置入口，中英文及窄屏交互限定通过；原登记／重检／核对／启用门保持，完整31／P0–P7继续。

T-KNOWLEDGE-PROJECTION-197：[合同](../../contracts/knowledge-projection.md)／[PRD](../product/knowledge-projection.md)／[架构与AI SOP](../architecture/knowledge-projection.md)／[限定验证](../quality/knowledge-projection-verification.md)。正式发布投影、墓碑、真实混合检索与预算重建贯通；原文定位和当前授权保持，真实数据库／原生索引重启链及双语代表界面限定通过，完整31／P0–P7继续。

T-KNOWLEDGE-DISCOVERY-196：[合同](../../contracts/knowledge-discovery.md)／[PRD](../product/knowledge-discovery.md)／[架构与AI SOP](../architecture/knowledge-discovery.md)／[限定验证](../quality/knowledge-discovery-verification.md)。已发布记忆、SPEC与技能的关键词发现、准确预览和原任务明确引用贯通；有界分页、草稿隔离与当前资格保持，完整31／P0–P7继续。

T-TOOL-RECOVERY-195：[合同](../../contracts/tool-recovery.md)／[PRD](../product/tool-recovery.md)／[架构与AI SOP](../architecture/tool-recovery.md)／[限定验证](../quality/tool-recovery-verification.md)。真实维护能力、原停止证明、一次有限核对授权与持久ACK释放未知占用贯通；关闭保原未知事实、不重probe。30个不同定向、Windows父子进程／三次Worker及双语代表界面限定通过，完整31／P0–P7继续。

T-TOOL-REINSPECTION-194：[合同](../../contracts/tool-reinspection.md)／[PRD](../product/tool-reinspection.md)／[架构与AI SOP](../architecture/tool-reinspection.md)／[限定验证](../quality/tool-reinspection-verification.md)。真实准确节点检查请求、新报告原子闭合及原回执恢复贯通；Pool满槽位可重检，单scope错误隔离。21个不同定向检查及受控实际链／3组双语界面限定通过，原31／P0–P7／58／34／92继续。

T-TOOL-MANAGEMENT-193：[合同](../../contracts/tool-management.md)／[PRD](../product/tool-management.md)／[架构与AI SOP](../architecture/tool-management.md)／[限定验证](../quality/tool-management-verification.md)。简洁连接向导、当前／历史筛选、准确目标诊断、公开草稿及原生命周期命令贯通；活动影响明确完整／部分／未知，撤销不伪称进程停止。62个不同定向检查及真实HTTP／SQLite／CLI、双语代表界面限定通过，完整31／P0–P7／58／34／92继续。

T-CLAUDE-SDK-192：[合同](../../contracts/claude-sdk.md)／[PRD](../product/claude-sdk.md)／[架构与AI SOP](../architecture/claude-sdk.md)／[限定验证](../quality/claude-sdk-verification.md)。官方SDK原生一次许可、提问、固定配置、原Runner及同提交质量贯通；CLI／SDK／MCP、配置与真实厂商、送达与验收分别记录。下一优先简化工具新增／调整／移除与自动检测修复，原31／P0–P7／58／34／92继续。

T-CURSOR-SETUP-191：[合同](../../contracts/cursor-setup.md)／[PRD](../product/cursor-setup.md)／[架构与AI SOP](../architecture/cursor-setup.md)／[限定验证](../quality/cursor-setup-verification.md)。高级／浏览器／独立节点沿唯一Cursor表单生成原Go受核配置，复用原库存、暂停登记和明确启用；成功RPC后受控宿主停止与真实退出值分别记录，未知门不松。真实厂商账号／模型、Unix未覆盖约束及完整31／P0–P7／58／34／92继续。

T-CURSOR-INPUTS-190：[合同](../../contracts/cursor-inputs.md)／[产品](../product/cursor-inputs.md)／[架构与 AI SOP](../architecture/cursor-inputs.md)／[限定验证](../quality/cursor-inputs-verification.md)。Cursor原问题／多选／完整计划明确决定贯通原输入、普通／Pool、typed响应和送达事实；原生Codex关闭记录保持准确投影。新受控完整链实际贯通HTTP、SDK、Journal、候选、同提交独测及验收；真实账号／模型与完整31／P0–P7／58／34／92继续。

T-CURSOR-ACP-189：[合同](../../contracts/cursor-acp-execution.md)／[架构与 AI SOP](../architecture/cursor-acp-execution.md)／[限定验证](../quality/cursor-acp-execution-verification.md)。Cursor 采用成熟 ACP SDK 接入受核 Node 包、明确凭据引用、准确模型配置、原任务／讨论／专家团队和普通／Pool 权限链；代码候选、同提交独立测试与验收沿原门禁。编辑器观察、包完整性、配置、执行和质量事实独立，未知效果不盲目重发。真实厂商账号／模型、多机与完整31／P0–P7／58／34／92继续交付。

T-TURN-STEERING-188：[合同](../../contracts/turn-steering.md)／[限定验证](../quality/turn-steering-verification.md)。本人已保存消息明确送同一活Codex回合，普通／Pool当前资格、原意图与单writer、native ACK、closed尾项及Result准确闭合；无效RPC error误判P2已修。31不同定向／24Go子例、类型与8803对词条、一次构建／3合成GUI及一次真实CLI＋自有模型通过。完整31／P0–P7／58／34／92、持久会话／其他工具／多机与生产继续。

T-TOOL-INPUTS-187：[合同](../../contracts/tool-inputs.md)／[限定验证](../quality/tool-inputs-verification.md)。显式本地实验性能力、普通／Pool问答、原Journal单writer、原scope恢复和中英文节点交互接通；独立窄审修复回执在lease／terminal漂移后的ACK恢复。43不同定向／30Go子例、类型与8761对词条、一次构建／3合成GUI及一条真实CLI＋自有模型问答通过；真实商业模型、多机与完整平台继续。

T-DEVELOPMENT-CONTINUATION-186：[合同](../../contracts/task-development-continuation.md)／[限定验证](../quality/task-development-continuation-verification.md)。同任务终态封存代码接续到新讨论计划、原派发、M→N/B与准确N独测/Review；祖先验收撤回fresh门已修复。26不同定向/27Go子例、最终类型及8710对词条、一次组件构建/5合成交互通过。Codex旧thread恢复、物理多机/其他数据库/生产与完整31/P0–P7/58/34/92继续。

T-RUNNER-INBOX-185：[合同](../../contracts/runner-inbox.md)／[限定验证](../quality/runner-inbox-verification.md)。实时项目配对收件箱、准确目标交接、公开ID复用身份与独立持续启动草稿贯通；32不同定向／39Go子例、一次类型及8683对词条、一次组件构建／5代表合成交互通过。当前Grant、在线、库存、启用与验收仍分别表达，完整31／P0–P7／58／34／92与实际多机／生产继续。

T-RUNNER-PAIRING-184：[合同](../../contracts/runner-pairing.md)、[产品](../product/runner-pairing.md)、[架构与AI SOP](../architecture/runner-pairing.md)、[交互](../design/runner-pairing.md)、[报告](../quality/runner-pairing-verification.md)／[摘要](../quality/runner-pairing-summary.json)。项目十分钟待办→机器系统库原凭据／摘要提议→浏览器当前CAS明确批准→原Grant核验后prepared；复用原Worker、24小时Grant及原report/显式Worker，不Claim或启用。修复无效public请求先拿writer、终端漏指纹和identity指南缺当前范围；原窗口准确接续工具配置，双语完整。41不同顶层（36新5旧）／41Go子例通过；一次合同生成，首新test5型错及最终scope输入后TSC／8617目录／组件收口通过。Windows专有系统库和真实HTTP集成同1例修后复验，两entry确认清理；6合成GUI／2图Root复核／两原生退出0。最后确定错误刷新仅复验原单例及2文件语义0，既有GUI分支不重；OpenAPI语义仅11项新增。其他OS／物理多机／厂商／安装／规模及原31／P0–P7／58／34／92继续。

T-WIP-REVIEW-183：[报告](../quality/remaining-code-review-verification.md)／[摘要](../quality/remaining-code-review-summary.json)。原保留代码已按桌面、控制服务／Store及知识界面并行检查，修复共同初始化先写后拒绝及SQLite大小写变体、打包临时目录与双语失败、知识ID显示回归；SDK字段与原IPC固定预期核对。15个不同顶层／9Go子例通过，未重全套、SDK、安装或GUI。必要原文档同步，两份重复历史本地审计原样保留不入Git；完整31／P0–P7／58／34／92继续。

T-DEVELOPER-TOOL-HANDOFF-182：[合同](../../contracts/developer-tool-handoff.md)、[产品](../product/developer-tool-handoff.md)、[架构与AI SOP](../architecture/developer-tool-handoff.md)、[交互](../design/developer-tool-handoff.md)、[报告](../quality/developer-tool-handoff-verification.md)／[摘要](../quality/developer-tool-handoff-summary.json)。讨论交接复用八类当前结构化可写派发，源知识与模型技能先固定再明确Start；固定工具不强制平台选择。Runner新增新文件自动准备和单次原inventory／ACK恢复，准确control路径／项目／节点scope贯穿normal／single／Client／侧车，不Claim。原指南增加目标Shell、仓库工作区与完整提交命令，默认折叠、仅复制。39不同定向（27新12旧）、35Go子例、一次全类型与8559对词条通过；一次合同生成、5合成GUI交互／2图Root复核／一次正常原生退出。组件因工作区源变更刷新一次，辅助HTML复制EEXIST经同摘要核验完成，不重建SDK／业务程序。原31／P0–P7／58／34／92、多机／厂商／跨库／安装／规模和171显式停止GUI缺口继续交付。

T-TOOL-DOCTOR-181：[合同](../../contracts/tool-profile-diagnostics.md)、[产品](../product/tool-profile-diagnostics.md)、[架构与AI SOP](../architecture/tool-profile-diagnostics.md)、[交互](../design/tool-profile-diagnostics.md)及[报告](../quality/tool-profile-diagnostics-verification.md)／[摘要](../quality/tool-profile-diagnostics-summary.json)。原可派发collector与四事实共用，独立显示当前授权／身份／库存／配置，单调期限与原pending隔离；只导航原CAS审核、准确节点或配置，不自动操作。通用可选读取视图断言及CLI GET贯通。27不同顶层（22新5旧）、41Go子例、一次全类型和8520对词条通过；3次明确输入变更的合同生成，零业务／SDK构建。实际会话Go／SQLite1成功GET+2视图拒绝；1组件构建，首脚本定位失误后仅修driver续剩5项，共6检查／2图Root复核／2原生退出0。原31／P0–P7／58／34／92、其他副作用、多机／跨库／安装／生产及171显式GUI停止缺口保持交付。

T-TOOL-PROFILE-REUSE-180：[合同](../../contracts/tool-profile-reuse.md)、[产品](../product/tool-profile-reuse.md)、[架构与AI SOP](../architecture/tool-profile-reuse.md)、[交互](../design/tool-profile-reuse.md)及[限定报告](../quality/tool-profile-reuse-verification.md)／[摘要](../quality/tool-profile-reuse-summary.json)。完整当前scope/Worker/Profile非分页查询、显式已有档案复用、原Create回执优先及事务防重、CLI只读同合同贯通。22不同顶层（18新／4旧）、5Go子例、一次类型检查及8493对词条通过；一次合同生成、零业务／SDK构建。真实Go／SQLite登录会话HTTP与双请求竞争已验；1组件构建／6功能GUI检查，截图范围定位及仅截图重取保留，总5GUI退出0，最终中英图Root复核。真实厂商、完整桌面、多机／跨库／安装／生产及原31／P0–P7／58／34／92和171显式停止缺口继续交付。

T-DESKTOP-RESULT-RECOVERY-179：[合同](../../contracts/desktop-result-recovery.md)、[需求](../product/desktop-result-recovery.md)、[架构与AI SOP](../architecture/desktop-result-recovery.md)、[交互](../design/desktop-result-recovery.md)及[限定验证](../quality/desktop-result-recovery-verification.md)／[摘要](../quality/desktop-result-recovery-summary.json)。当前管理员与原HMAC核对原历史回执，60秒签名事实凭证只授权本地一次原子ACK；旧机器凭据仍拒绝执行，原计划／未知项／sidecar保留，续期与明确连接独立。27不同定向、21Go子例、一次全类型及8472对词条通过；1Go构建／3真实离线CLI观察，1组件构建／1GUI启动／6代表检查及两图Root复核、退出0。首CLI驱动语言参数错误原件保留，仅修驱动复用构建。未提交副作用及完整桌面、多机、跨平台、数据库和生产验收继续交付；原31／P0–P7／58／34／92及171显式GUI停止缺口保留。

T-DESKTOP-CREDENTIAL-178：[合同](../../contracts/desktop-runner-renewal.md)、[需求](../product/desktop-runner-renewal.md)、[架构与AI SOP](../architecture/desktop-runner-renewal.md)、[交互](../design/desktop-runner-renewal.md)及[限定验证](../quality/desktop-runner-renewal-verification.md)／[摘要](../quality/desktop-runner-renewal-summary.json)。原Worker和配置续期、加密pending先行、HMAC／当前资格／回执／CAS及本机和服务端排空、原renew未知恢复贯通，ACK仅prepared；两期限／版本与中文英文说明分开。30不同定向、18Go子例、一次全类型及8442对词条通过；1Go构建／3真实只读CLI观察，1组件构建／4中文原用例复用＋1英文续验，两图已复核、两次退出0。原键盘断言、日志碰撞和late cloud门禁修正留证。过期未结效果恢复票据、完整桌面／多机／跨平台／生产和31／P0–P7／58／34／92继续交付，171显式GUI停止缺口保留。

T-OPENCODE-QUICK-CONNECT-177：[合同](../../contracts/opencode-quick-connect.md)、[需求](../product/opencode-quick-connect.md)、[架构与AI SOP](../architecture/opencode-quick-connect.md)、[交互](../design/opencode-quick-connect.md)及[限定验证](../quality/opencode-quick-connect-verification.md)／[摘要](../quality/opencode-quick-connect-summary.json)。原SDK执行接到第八项原生检测、四字段配置、受信Node／SDK资产、原托管Worker与准确新库存／暂停登记／明确启用。40不同定向、7Go子例、一次全类型与8421对中英词条通过；一次当前Go编译／真实被动CLI未发现OpenCode，一次真实SDK构建验证3621资产及宿主资源解析。2组件构建／3GUI尝试保留原夹具和关闭故障，最终5项通过、两图已复核、正常退出0；真实厂商模型与完整安装仍待验。原31／P0–P7／58／34／92及171显式GUI停止缺口保留。

T-TOOL-ONBOARDING-176：[合同](../../contracts/tool-onboarding-scan.md)、[需求](../product/tool-onboarding.md)、[架构与AI SOP](../architecture/tool-onboarding.md)、[交互](../design/tool-onboarding.md)及[限定验证](../quality/tool-onboarding-verification.md)／[摘要](../quality/tool-onboarding-summary.json)。一次自动扫描七个接入项，复用原Go并行目录，60秒同范围观察缓存／合并等待／当前授权；高级登记保留原来源与过期预检，原未知请求恢复及手工暂停登记独立。27不同定向、一次全类型及两组精确修订语义、8381对中英词条通过；0Go编译，1次真实CLI约1.218秒，1次生产组件构建／5代表检查及两图已复核。完整桌面旅程、真实模型／多机／跨平台／生产及31／P0–P7／58／34／92继续交付，171显式GUI停止缺口保留。

T-CODEX-QUICK-CONNECT-175：[合同](../../contracts/codex-quick-connect.md)、[需求](../product/codex-quick-connect.md)、[架构与AI SOP](../architecture/codex-quick-connect.md)、[交互](../design/codex-quick-connect.md)及[限定验证](../quality/codex-quick-connect-verification.md)／[摘要](../quality/codex-quick-connect-summary.json)。Codex原生能力接入自动检测、私有提供方配置、真实目录、原托管节点／新库存／暂停登记／明确启用；未知原操作及停止后明确换配置均保持来源。38不同定向、5Go子例、一次全类型与8346对词条通过；1Go编译／2真实CLI命令得到9模型目录且Go/JS摘要相同，0浏览器工程／UI构建。未证明完整桌面点击、真实账号模型／计费／OS隔离、跨平台／多机／生产；完整31／P0–P7／58／34／92和171显式GUI停止缺口继续交付。

T-CODEX-HUMAN-REQUESTS-174：[合同](../../contracts/codex-human-requests.md)、[架构与AI SOP](../architecture/codex-human-requests.md)、[交互](../design/codex-human-requests.md)及[限定验证](../quality/codex-human-requests-verification.md)／[摘要](../quality/codex-human-requests-summary.json)。原权限领域显式区分ACP／Codex，准确一次决定、异步写入与持久观察沿普通／Pool当前资格和原回执贯通；65个不同定向检查、54个Go子例与9项真实HTTP／SQLite／原Worker／受控CLI／生产组件检查通过；两次验证器启动前问题原件保留，最后复用二进制及UI完成，无额外编译。两张中英局部图已查看，真实厂商／模型及正式验收不外推。完整31／P0–P7／58／34／92及原171显式GUI停止缺口保留。

T-CODEX-SKILLS-173：[合同](../../contracts/codex-native-skills.md)、[架构与AI SOP](../architecture/codex-native-skills.md)、[交互](../design/codex-native-skills.md)及[验证](../quality/codex-native-skills-verification.md)／[摘要](../quality/codex-native-skills-summary.json)。固定发布技能沿原任务／专家团队计划、当前普通／Pool资格与受保护Assignment物化到独立HOME，准确目录后显式调用，五种执行事实分别展示；移除缺包的历史兼容放行。31Go／16Web不同顶层通过，Go子例70过／4链接权限跳过；一次全类型4处新夹具诊断修正后3文件语义0，其余17源未变，8208对词条通过。本机当前CLI一次initialize／skills-list目录验证0模型、正常退出；一次组件构建及4项中英／390／键盘检查通过，合成数据不冒充真实模型。真实技能使用、完整Codex能力、物理多机／跨库／安装与31／P0–P7／58／34／92继续交付，原171显式UI停止缺口保留。

T-DELIVERY-KNOWLEDGE-172：[合同](../../contracts/delivery-knowledge.md)、[架构与AI SOP](../architecture/delivery-knowledge.md)、[交互](../design/delivery-knowledge.md)及[验证](../quality/delivery-knowledge-verification.md)／[摘要](../quality/delivery-knowledge-summary.json)。正式验收来源固定Review、运行、测试及候选依据，复用知识提案、人工草稿、发布、目录及后续上下文；保留原操作恢复和来源漂移保护。23Go／20Web不同顶层、7单列子例、类型及8181对词条通过。一次Go／UI构建及6项真实授权HTTP／生产组件只读检查通过，17GET／0POST／17表不变、正常退出及两中英代表局部图留证；后补两项英文数量文案不重跑业务。真实模型、下一Codex执行、物理多机／其他数据库、完整App和31／P0–P7／58／34／92继续交付，171显式UI停止缺口保留。

T-ACP-QUICK-CONNECT-171：[合同](../../contracts/acp-quick-connect.md)、[架构与AI SOP](../architecture/acp-quick-connect.md)、[交互](../design/acp-quick-connect.md)及[限定证据](../quality/acp-quick-connect-verification.md)。明确原生检测、无Prompt实际模型、原托管连接和准确新库存／暂停登记／独立启用已接通；49定向顶层、8137对语言、6关键实际检查与两代表局部图留证。三个驱动1保留：初始busy产品问题已修，空数组与最后窄屏导航为夹具问题；显式UI停止未验，真实app-close Worker停止0。Go4／UI2／probe2，0Prompt／vendor，原18产物与21项其他工作受保护。真实厂商、完整旅程及31／P0–P7／58／34／92继续交付。

T-ACP-RUNTIME-170：[合同](../../contracts/acp-cli-execution.md)、[架构与AI SOP](../architecture/acp-cli-execution.md)、[交互](../design/acp-cli-execution.md)、[接入](../integrations/acp-cli-execution.md)及[证据](../quality/acp-runtime-verification.md)。成熟ACP stdio／固定Qoder策略、真实模型配置ACK、private／Pool一次权限和当前授权、原Worker／候选／独立质量已接通。36Go／24Web不同顶层、全Web类型、8087对语言及8项真实同机Server／SQLite／双Worker／受控CLI／生产组件通过；3项P2及原夹具失败留证，未重复旧绿。1正式Run／1候选、不同Worker同提交JUnit1/1，task仍in_progress且待人工验收；2Go／1组件构建、正常排空、私有快照、两代表视口。真实Qoder／账号／模型、OS沙箱、强制预算、包装器／发现、物理多机及完整31／P0–P7／58／34／92仍待交付或独立验收。

T-WORKBUDDY-CLOUD-TURNS-169：[合同](../../contracts/workbuddy-cloud-turns.md)、[架构与AI SOP](../architecture/workbuddy-cloud-turns.md)、[交互](../design/workbuddy-cloud-turns.md)及[限定证据](../quality/workbuddy-cloud-turns-verification.md)。原云任务内明确新输入、SDK持续回复、准确once权限决定、取消和失联核对已接通；持久命令收件由原lease runtime消费。28个Go／14个Web不同顶层、七项同机双服务HTTP／SQLite／SDK业务和一项独立只读键盘／390px视觉通过；两个夹具失败及第八原旅程partial保留。两次Go编译、一次前端构建复用，493文档／385路径检查通过，原21项工作受保护。回合结束不替代可信代码候选、同提交独立测试及人工验收；真实供应方、物理多机及完整31／P0–P7／58／34／92继续交付。

T-WORKBUDDY-TASK-DISPATCH-168：[合同](../../contracts/workbuddy-task-dispatch.md)、[架构与AI SOP](../architecture/workbuddy-task-dispatch.md)、[交互](../design/workbuddy-task-dispatch.md)及[阶段验证](../quality/workbuddy-task-dispatch-verification.md)。任务详情直接选择本人WorkBuddy账号、固定上下文预览并明确派发，复用原DAG／唯一Run／委派／Worker及原节点工作区；未知回执沿原body/key核对。35个不同顶层、一次类型／7934对语言及8项真实HTTP／SQLite／私有Worker／生产组件通过；首次启动提交后截断JSON，原GUI恢复后仍只创建1个云任务。1Go宿主／1SDK构建、正常关闭与私有快照保留；两原图为滚动局部视口，时分展示小修单列验证，不重复8项业务。供应方completed仍等待可信交付；云候选／独立测试／验收、真实供应方及完整31／P0–P7／58／34／92保持待交付或待验。

T-NOTIFICATION-PREFERENCES-167：[合同](../../contracts/project-notification-preferences.md)、[架构与SOP](../architecture/project-notification-preferences.md)、[交互](../design/project-notification-preferences.md)及[限定证据](../quality/project-notification-preferences-verification.md)。本人当前项目可静音关注讨论，精准提及和其他来源不受影响；原body/key、CAS/no-op与当前值重读分开。14Go／38Web、7875对词条及8项真实HTTP／SQLite业务通过；旧样式夹具缺失被明确修正，保留业务结果后另以快照副本／0POST／15表摘要不变完成两图视觉。业务与视觉各1次必要宿主／SDK构建，各自门禁与首次端口失败留证；无完整App／其他数据库／规模／生产声明，完整31／P0–P7／58／34／92不变。

T-CURSOR-CONNECTION-166：[接入合同](../../contracts/cursor-connection.md)、[架构](../architecture/cursor-connection.md)、[交互](../design/cursor-connection.md)及[验证记录](../quality/cursor-connection-verification.md)。快捷接入区分Cursor编辑器、Agent候选与派发资格，桌面只检查当前PATH固定文件；被动发现与指引已限定核验，真实Cursor执行适配仍planned。不得把文件存在、静态官方包或受控界面当作厂商执行、模型预算、代码候选或生产验收。完整31／P0–P7／58限定＋34待交付＝92项不变。

T-WORKBUDDY-HISTORY-SHARE-165：[明确共享合同](../../contracts/workbuddy-cloud-history-sharing.md)、[架构](../architecture/workbuddy-cloud-history-sharing.md)、[交互](../design/workbuddy-cloud-history-sharing.md)及[限定证据](../quality/workbuddy-cloud-history-sharing-verification.md)。本人选终止观察、同项目目标和精确预览后生成human讨论副本，当前任务读者能读副本而不继承私有会话权限；来源、消息、关注通知与原回执同事务，原操作沿元数据恢复。Go25不同（13新／12旧）、Web41、最新类型及7823对语言通过；两项早期UI缺陷和迟后分页选择P2均已精准修复并独立复核。原11项按修复前输入在第二隔离环境完成；迟后分页修复另用1条当前组件／55条合成GET场景核实，未重跑原11。四次同body／key运输只产生1消息／1来源／1通知，最后正常HTTP201及准确详情200清除journal；viewer私有读取404，原运行／节点／任务依据不变。首次准备失败、测试拦截status0、原驱动及续验退出1和仅自有调试器CLI强停均保留，关键服务正常关闭、快照留存。真实厂商与完整协作／部署矩阵仍待验，完整31／P0–P7／58／34／92不变。

T-WORKBUDDY-CLOUD-COLLAB-164：[原会话协作合同](../../contracts/workbuddy-cloud-collaboration.md)、[架构与AI SOP](../architecture/workbuddy-cloud-collaboration.md)及[限定证据](../quality/workbuddy-cloud-collaboration-verification.md)。已有准确原任务上，明确一次ACP加载形成本人私有有界快照，丢响应以同body／key恢复同一记录，本地分页／普通刷新不再Attach；覆盖始终未核实。Go平台14、闭合合同14、公共同源Schema1及Web33项分别通过，最终类型／7777对语言通过。真实App／授权HTTP／SQLite的13项检查使用受控SDK，55条按50＋5读取，原Run仍running、节点仍waiting且attempt／fence未变；中英代表图已复核。原驱动与首捕获退出1、同窗口最终续验0分别保留；源码未改、未另起加载。真实厂商、完整输入／权限／停止协作、多机与生产仍待验，58／34／92不变。

T-WORKBUDDY-CLOUD-UI-163：[状态与交互合同](../../contracts/workbuddy-cloud-status-ui.md)及[限定证据](../quality/workbuddy-cloud-status-verification.md)。供应方原状态／观察／安全失败提示移到详情外，平台暂停独立显示，原关键状态优先级与权限动作不变。29项不同Web（2新增）、一次类型及7723对语言检查通过；3组真实组件可见性／键盘、中文1366与英文390代表图已复核。整页截图疑点经一次只读测量确认无产品溢出，原图保留、三组未重跑；窗口／进程正常退出。合成组件不代表完整App或供应方验收，58／34／92不变。

T-OPERATION-OWNERSHIP-162：[工具操作恢复合同](../../contracts/tool-operation-recovery.md)与[限定证据](../quality/tool-operation-ownership-verification.md)。发送／校验／清理核对当前调用和原记录，删除失败保留pending；同步ticket防重复及旧finally，三个中英错误词条。53项不同Web、类型及7722对语言检查通过，3个实际浏览器组件组通过；驱动关闭退出1、原生关闭及进程不存在分别留证，无完整App／厂商调用。保护以删除前同步核对为界，最终消费确认、跨窗口持久和其他journal仍待交付，完整58／34／92不变。

T-TASK-COLUMNS-160：[任务显示列](../design/task-columns.md)与[验收报告](../quality/task-columns-verification.md)限定完成。五种数据列按需显示／排序，标题固定，可只保留标题；本机、URL、个人／项目视图使用同一偏好，显式非法值拒绝，原回执摘要不变。后端CLI及schema6个新增／5个原回归、Web112项（9新增）、最终类型和7719语言键通过；一次前后端构建，18项实际检查与中英代表图已复核。列变化不额外查询任务、不改选择／版本；未知保存同body/key恢复同一第1版视图。原数据保留，其他数据库、多机、离线及生产待验，58／34／92不变。

T-CLAUDE-FORMAL-159：[正式CLI交付与问题回流](../quality/claude-formal-verification.md)及[摘要](../quality/claude-formal-summary.json)限定验收完成。一个原Run沿双本机Worker、真实Git候选和独立Node／JUnit通过，Review退回后任务阻塞并关联Issue，Run仍成功；四个前端入口误比原工作流身份的问题已统一修复。6项受影响Web检查、独立类型／7705语言键和一次UI构建通过；准备、HTTP、GUI与恢复分别计数。恢复依赖测试宿主重载原body／key，不代表跨窗口自动持久；真实供应方／物理多机／其他系统及生产仍待验，完整58／34／92不变。

T-SKILL-COMPARE-157：[固定技能修订比较](../design/skill-revision-compare.md)及[限定验证](../quality/skill-revision-compare-verification.md)。默认收起，两个准确版本对照、纯文本后备与明确采用当前版本沿原草稿审核；刷新／重开不回显旧正文，差异模型释放不影响IDE。复用本地Monaco，27项不同Web（15新增）及最终类型通过；7准备断言、15实际功能交互、2最终只读布局分开记录，中英1366／390已目视。真实界面发现的销毁异常及窄屏双栏已修复，13次正式写分为seed12与撤权1，无采纳重写或模型调用。内建SDK命令语言、其他数据库、多机及生产继续待验，58／34／92不变。

T-CLAUDE-CLI-158：[Claude CLI执行接入](../integrations/claude-cli.md)、[架构](../architecture/claude-cli-execution.md)及[限定验证](../quality/claude-cli-verification.md)。固定完整模型、实际版本／帮助门禁、私有／Pool派发、原工作树及代码候选已接线；权限拒绝、未知结果和未完成工具不能冒充成功，组织／项目强制预算均拒绝此观测能力。15个不同新增Go顶层、Node1／Web4、最终类型和7705对文案通过；控制面、Runner、组合Web分别构建成功。本机原生协议夹具执行及同提交独立Node测试通过；GUI、正式Worker.Run／HTTP与真实Claude供应方调用尚未验，完整58／34／92不变。

T-MULTICA-ITERATION-156：[正式迭代任务与共享泳道](../quality/task-iterations-verification.md)及[摘要](../quality/task-iterations-summary.json)限定验收完成。列表／看板使用正式排期迭代，支持准确筛选、未关联迭代、迭代分组与跨账号在线共享视图；独立任务／排期版本避免迟到结果错误换列，撤权清除受保护内容，筛选默认收起。新增Go7项、Web46项包含于相关定向／集成结果，最终类型与7673语言键通过；8项HTTP准备与11项双账号实际交互分列，中文1366／英文390深色已目视。首轮旧夹具／驱动失败保留，仅复验受影响项；保存视图不改任务或启动执行。源码来源与强制停止方式据实记录，物理多设备、离线同步及生产仍未验，完整58／34／92不变。

T-WORKBUDDY-RUNNER-155：[正式云执行与恢复限定验收](../quality/workbuddy-cloud-runner-verification.md)及[安全摘要](../quality/workbuddy-cloud-runner-summary.json)。私有／Pool正式领取固定原委派与attempt，中央一次创建、机器只读lookup、原生Worker持久恢复及保护性waiting已接通；供应方completed不等于交付或验收。工具目录直达原账号面板，保持草稿与权限；实际目视发现的通用waiting误称审批已修正。44个新增Go顶层、11个不同Web用例（10新增）、完整类型／7665语言键及合同检查通过；17项真实本机Worker-HTTP／GUI业务检查和3项只读视觉复核分开记录，每旅程仅一次供应方POST。首轮夹具失败与v2新环境均如实保留；服务／子进程正常收尾，持久数据保留。真实供应方、机器ACP、云候选、独立测试／正式验收、多机及生产继续完整目标，58／34／92不变。源码`22c3b9c`已推dev，验收与视觉修正随本记录提交。

T-WORKBUDDY-CLOUD-152：[本阶段WorkBuddy云任务工作区](../quality/workbuddy-cloud-execution-verification.md)。原运行节点内的本人委派、准确来源和供应方任务核对已接通；控制面以原attempt机器资格提供持久一次创建，未知作用保留原操作。源码、定向检查、隔离构建及真实界面各自范围以专项报告为准，完整自动DAG领取／Execute、ACP协作、可信制品、独立测试和正式验收继续交付。 完整58／34／92不变。


T-MULTICA-FOLLOW-154：[本阶段当前人员任务关注](../quality/task-subscriptions-verification.md)。任务讨论旁用一个关注按钮管理个人偏好，正式讨论与最终assistant消息沿原事务通知当前有权关注者；与精准提及去重，取消后保留历史通知。37个不同Go领域顶层、59项Web及16项真实账号／HTTP／GUI检查限定通过，中英1366／390图片已检查；真实assistant GUI、其他数据库、多机、规模与生产仍待验。 完整58／34／92不变。


最新T-MULTICA-DESKTOP-153：[桌面本机连接](../product/desktop-managed-runner.md)沿现有Go Worker／DAG完成OS加密journal、HMAC摘要发行、固定control私有stdin、启动后新水位库存及151暂停登记／独立启用。Windows Electron35.7.5软件离屏／真实DPAPI的17项实际GUI／HTTP／Worker检查通过，中英1366／390图片已目视；唯一节点／原发行操作保持，五次注册传输同正文／摘要／操作，五个正式Go子进程均exit0／close，0任务／运行／模型调用。首次GUI拒绝小数秒库存后仅修复Web解析器，沿原journal续验，见[报告](../quality/desktop-managed-runner-verification.md)及[摘要](../quality/desktop-managed-runner-summary.json)。本条限定覆盖151的托管待办，旧记录保留；24小时续权与原节点处置交互、真实供应方、真实OS选择器、强制崩溃、macOS／Linux、安装包、多机、规模及生产继续待办或待验，完整58／34／92不变。

并行T-WORKBUDDY-ACP-149已限定完成[WorkBuddy准确Session访问与ACP底层](../architecture/workbuddy-cloud-submission.md)：固定供应方端点，成熟ACP SDK v0.13.5类型／编解码，SSE和HTTP独立读写，明确握手／load能力、准确历史、关联prompt、权限回执及取消。56个不同Go顶层／163子场景通过，18个交付文件及8份日志摘要核对一致；见[报告](../quality/workbuddy-acp-verification.md)、[摘要](../quality/workbuddy-acp-summary.json)及[SDK许可清单](workbuddy-acp-dependencies.json)。没有公共派发入口或UI接线，仍不把云工具登记为可派发；原DAG当前权限／机器／attempt绑定及持久一次派发留待T-WORKBUDDY-CLOUD-152，真实供应方和生产未验。工具快速接入和本机托管Runner仍按原优先级推进，58／34／92不变。

最近交付T-TOOLS-151：[工具快速接入](../product/tool-quick-connect.md)以选择工具为入口，唯一已授权节点和新鲜匹配配置自动采用，名称自动建议，暂停登记后原地引导明确启用，高级配置与原回执恢复保留。桌面复用同一control程序新增受限安装检测，支持Codex／CodeBuddy／dsh，不暴露命令权或凭据。14个不同Go顶层通过、1符号链接场景跳过，Web55、原生15及实际GUI／HTTP／IPC15项通过；中英1366／390图片已目视。真实Runner上报使用明确合成探测CLI，未开发／请求模型；见[报告](../quality/tool-quick-connect-verification.md)／[摘要](../quality/tool-quick-connect-summary.json)。本机凭据托管、Runner自动启动及已有未上报身份的简化连接仍待交付；58／34／92不变。

最近交付T-MULTICA-BULK-150：[批量负责人分配](../product/task-owner-changes.md)将最多16个明确任务、当前真人资源、准确版本、预览摘要与影响确认放在同一流程。复用普通任务更新的同事务核心；全部预验后采用，未变项不空更新，通知与依赖失效原子保存，未知结果只核对原正文／操作ID。24个Go顶层和24个不同Web用例通过，23项实际本机断言与5项最终只读核对分开记录，最终中文1366／英文390深色图片已目视。实际GUI刷新恢复受测试宿主限制，已按原API恢复且无重复；见[合同](../../contracts/task-owner-changes.md)、[报告](../quality/task-owner-changes-verification.md)与[摘要](../quality/task-owner-changes-summary.json)。不等于全部M07或平台验收，完整58／34／92不变。

最近交付T-HANDOFF-INBOX-148：[交接请求](../product/task-handoff-inbox.md)在原事务通知原接收人，私有反向绑定后从个人收件箱准确定位；接收责任与正式Review分开。限定检查及16项实际本机GUI／HTTP通过，一次明确退回由exact GET核对v2／returned，通知仍未读；两次截图timeout，视觉图片待验。见[合同](../../contracts/task-handoff-inbox.md)、[报告](../quality/task-handoff-inbox-verification.md)及[摘要](../quality/task-handoff-inbox-summary.json)。完整58 limited／34 pending／92总项不变。

当前T-WORKBUDDY-CONNECTIONS-147：[账号授权](../product/workbuddy-connections.md)复用成熟x/oauth2、当前项目本人／原浏览器nonce核验、在库加密及grant／token代次，未知兑换／刷新不重复供应方POST。Web默认收起；桌面四ID窄IPC打开同Session、无preload／Node的隔离子窗，返回聚焦且仅在无草稿／未确认写入时GET原连接，不reload或自动认定已连接。既有检查、原生协议样例及最终候选18项实际本机GUI／HTTP见[连接报告](../quality/workbuddy-connections-verification.md)、[摘要](../quality/workbuddy-connections-summary.json)和[窗口证据](../quality/workbuddy-desktop-oauth-verification.md)。最终候选以合成应用配置阻断供应方外网，失败不冒充已连接且只读恢复不重复POST；视觉图片、真实供应方授权、云派发／ACP、多机及生产待验，完整58／34／92不变。

T-CONTEXT-145已限定实现[讨论固定资料](../product/task-conversation-knowledge.md)：明确选择最多16个已发布知识的准确ID／修订／SHA，按需读取准确历史，历史可读不授予新消费资格；助手仅从真实completed backing Reply继承依赖，批准正文在模型Source中固定一次，并沿原开发预览与Start重验当前授权／资格。选择器默认收起，失效pin不静默升级，提供中英交互；见[合同](../../contracts/task-conversation-knowledge.md)、[架构](../architecture/task-conversation-knowledge.md)及[交互与AI SOP](../design/task-conversation-knowledge.md)。

[阶段证据](../quality/task-conversation-knowledge-verification.md)及[摘要](../quality/task-conversation-knowledge-summary.json)记录125个Go顶层、148个子场景通过，另1项跳过；Web36项、类型／7312对语言键／生成合同／隔离构建及真实OS Node子进程两轮本机HTTP模型协议链路通过。实际GUI仅5项通过，后续4组因工具热更新后技能缺失仍待验；不代表完整GUI、商业模型或平台验收。完整P0–P7、需求1–26及58 limited／34 pending／92总项不变。

T-COLLAB-144人员提及、团队准确执行跟进与审核责任已完成限定整合和36项真实GUI验证；双角色保存、准确资产／历史、草稿保护及原决定边界均通过，独立接口复核v4仍未发布且新请求pending。见[整合报告](../quality/collaboration-integration-verification.md)和[浏览器证据](../quality/collaboration-browser-summary.json)。源码63bcc39已推dev，最终证据独立交付；全包Go未完整及全平台58／34／92范围继续保留。

T-REVIEW-142已接通验收责任请求→同事务个人收件→原Review准确决定／问题回流；修复测试计划developer／reviewer操作遗漏，任务结论统一accepted／rejected。93项独立Go、58项Web、完整类型／7292语言键／合同／隔离构建及12项实际三账号Runner-HTTP-CLI检查通过，见[需求](../product/task-review-requests.md)、[架构](../architecture/task-review-requests.md)、[交互与AI SOP](../design/task-review-requests.md)和[限定报告](../quality/task-review-requests-verification.md)。binding_current不代表质量门禁通过；最终GUI状态以T-COLLAB-144整合报告为准；完整58／34／92不变。

T-WORKBUDDY-143已验证云任务协议与持久单次提交底层，27项去重Go通过，见[合同](../../contracts/workbuddy-cloud-submission.md)、[架构](../architecture/workbuddy-cloud-submission.md)与[专项证据](../quality/workbuddy-cloud-submission-verification.md)。此阶段没有UI变更；正式OAuth、ACP、当前资格／预算、云候选和真实厂商接入仍待实施。内部模块尚未开放为可派发工具，其他阶段分别验收，以各自当前记录为准；完整平台范围保留。

T-TEAM-EXECUTION-141增强原团队计划的准确步骤与等待原因：历史attempt不再冒充当前执行，完整状态分类与最多24项节点明细分开，原run／node／attempt入口和读取时间可回查。8项后端、24项前端、类型／7228双语键／生成合同及隔离构建通过；见[设计](../design/expert-team-execution.md)和[专项证据](../quality/expert-team-execution-verification.md)。实际浏览器已通过，本阶段实现及剩余GUI按T-COLLAB-144记录；58／34／92完整范围保留。

T-MENTIONS-140接通明确人员讨论→同事务个人通知→准确原消息定位，并提供discussion CLI。46项独立Go、56项Web、完整类型／7191对语言键、合同及隔离构建通过；两个本机账号完成12项实际接口／CLI检查。见[需求](../product/task-conversation-mentions.md)、[架构与来源](../architecture/task-conversation-mentions.md)、[交互与AI SOP](../design/task-conversation-mentions.md)、[限定报告](../quality/task-conversation-mentions-verification.md)。本域实际GUI与剩余项目见T-COLLAB-144整合报告；完整58／34／92不变。

T-ADAPTERS-139已限定验证当前授权的20入口统一工具接入目录及已有配置接续：执行／客户端方向、源码能力和限制分开，默认收起并保留选择。后端9项、Web43项独立用例（受影响11项复验包含在内）、类型／7156语言键、生成合同及隔离构建通过；用户重新授权后完成25项实际GUI检查，覆盖中英、键盘、桌面／390px与明暗代表性状态，修复固定页头遮挡移动详情标题。阶段scoped_verified，本记录覆盖此前139待验状态；厂商执行与其他在途页面仍单独验收。见[合同](../../contracts/adapter-catalog.md)、[设计](../design/adapter-connections.md)和[当前证据](../quality/adapter-connections-verification.md)。WorkBuddy云执行仍planned且与CodeBuddy分开，Harness按具体协议／版本登记；未新增厂商适配、真实安装、多机或生产验收，58／34／92不变。

T-INBOX-138限定实现当前人员任务分配收件箱：原事务真实来源、规范接收人、当前权限／来源水合、序号分页、本人已读与归档、原操作恢复，以及同源Suma CLI。见[合同](../../contracts/notification-inbox.md)、[产品要求](../product/notification-inbox.md)、[架构](../architecture/notification-inbox.md)、[交互与AI SOP](../design/notification-inbox.md)及[实际验证](../quality/notification-inbox-verification.md)。讨论提及、审批／告警多源路由、订阅偏好与通道outbox仍待交付，58／34／92不变。

T-VIEWS-138已限定交付服务端个人／项目共享任务视图、在线跨设备读取、版本冲突比较、原操作重试与归档墓碑，并保留本机原件。9项Go领域、3项CLI、43项Web回归以及24项实际浏览器、7项真实CLI检查通过，见[证据](../quality/shared-task-views-verification.md)与[合同](../../contracts/shared-task-views.md)。同人双浏览器不代表物理多机；离线同步、实时协作、工具扩展与全平台目标继续待办，58／34／92不变。以下早期阶段的“共享视图待办”由本条限定范围推进。

T-TASKS-137将任务工作台整合为同一正式入口：四种排序、本机命名视图、完整范围统计、三类有界分组与列分页；统一当前权限、字面筛选、纳秒排序、跨列版本去重及异常恢复。见[合同](../../contracts/task-workspace.md)、[交互与AI SOP](../design/task-collection.md)和[专项证据](../quality/task-collection-verification.md)。原有团队派发、独立测试、验收及问题回流权威保留；共享视图、其他数据库实测、规模和全平台仍待交付，58／34／92不变。

T-MULTICA-137推进Multica M06／M07任务工作台：全项目搜索／筛选／稳定分页、列表看板同源分组、本机个人命名视图、跨标签提示与详情返回。少量改编可追溯历史分组算法，当前受限源码功能独立实现；见[复用清单](../integrations/multica-reuse.md)、[合同](../../contracts/task-workspace.md)与[限定证据](../quality/multica-task-views-verification.md)。共享保存视图、跨端同步、完整批量编辑、新工具真实接入与全平台生产验收仍待办，58／34／92不变。

T-MULTICA-136基于已核对上游／本地一致SHA139b13a，独立实现个人任务完整范围查询与命令面板，复用既有任务／工作区／权限和Radix等组件；[参考采用](../integrations/multica-adoption.md)、[生产合同](../../contracts/my-work.md)、[交互](../design/multica-collaboration.md)及[限定证据](../quality/multica-collaboration-verification.md)。当前Multica许可附加托管／品牌等条件，本阶段未复制其代码／样式。全项目服务端分组分页、正式通知收件箱、审核分配及完整平台继续待办，58／34／92不变。

T-WORKLOG-135推进原P3人员协作：开发→计划→工时及同源Go CLI，当前人员身份、任务版本、明确代办、更正／作废不可变历史、组织跨项目每日1440分钟事务上限与原操作恢复。见[合同](../../contracts/worklogs.md)、[架构](../architecture/worklogs.md)、[限定报告](../quality/worklogs-verification.md)和[摘要](../quality/worklogs-summary.json)。登记是人员声明，非自动测量、审批、工资或机器计费。完整58／34／92不变，真实GUI／其他数据库、HR／组织容量与全平台仍待交付。

T-DESIGN-135继续深化[架构主规格](../suma-architecture-design.html#s21)及[统一体验](../design/unified-experience.md)：十二个可操作工作区，新增模型四通道／Harness接续、团队分工／负载、事件规则／未知核对、缺陷修复周期、配置继承与离线三方比较。主规格按来源分开当前事实、目标和待验；[阶段验证](../quality/design-workflows-verification.md)仅证明设计与本地合成样稿。无新增生产集成，58／34／92不变。

本轮 T-PLATFORM-DESIGN-134 完成全平台设计细化与可点击样稿：[统一体验](../design/unified-experience.md)、[编排架构](../architecture/unified-capability-orchestration.md)、[工具／Harness矩阵](../integrations/development-tool-matrix.md)、[样稿](../design/prototypes/enterprise-workbench.html)。覆盖11项业务闭环、八个演示工作区、配置继承、调度背压和部署开放门槛；[验证报告](../quality/platform-design-verification.md)只证明设计样稿交互，不代表生产页面、新适配器或真实服务集成完成。源码实现基线仍为下述各专项，完整58／34／92保持不变。

更新：2026-10-30。产品统一开发基线为 **1.0.0**；完整 P0–P7 与扩展需求 1–26 继续实施，生产集成尚未完成。[基线与范围](../product/v1-baseline.md)确定产品合同，[实施计划](implementation-plan.md)保留完整待办，[文档索引](../README.md)集中各领域设计、合同和证据。

当前T-CALENDAR-134推进原资源排期：版本化七日工作制及日期例外、按完整任务区间可用小时分摊、未知／0与未完整排期分开，正式智能规划绑定完整日历。中英编辑器与原回执／当前资格连通；[报告](../quality/resource-calendars-verification.md)和[摘要](../quality/resource-calendars-summary.json)记录SQLite／HTTP、DOM及类型／语言／构建证据。真实浏览器及组织计划容量／HR／生产仍待交付；人员明确登记由T-WORKLOG-135单独交付，58／34／92不变。

前序T-EXPERIENCE-133深化[企业研发工作台需求与架构](../product/enterprise-workbench.md)：保留全量交付与分页关注，按需展开准确分类和原运行依据诊断；新增四类客户端可信无密钥配置导出及其他工具手动指南。8项Go、67项Web、类型／6673键中英／生产构建及合同检查通过，GUI实际渲染仍待验。见[当前报告](../quality/enterprise-experience-verification.md)／[摘要](../quality/enterprise-experience-summary.json)，完整58／34／92不变。

当前 T-EXPERIENCE-131：首页项目全量交付统计及当前权限下的分页读取、导航视觉基础、IDE渐进展开代码完成，本阶段定向检查及中英／明暗／桌面／390px隔离浏览器验证通过；完整前端1236项通过，全量Go仍有已记录失败。[设计](../design/unified-experience.md)、[权威计划深化分组](implementation-plan.md#统一体验与全平台深化)、[阶段报告](../quality/unified-experience-verification.md)及[摘要](../quality/unified-experience-summary.json)分别记录范围和证据。完整文档编辑、排期容量、智能自动化策略、多端离线与正式交付仍待办，原58／34／92状态及既有阶段证据保持不变。


前序T-BASELINE-131将已验收基线真正用于后续终端开发：原管理员锚点B、采用提交M、新候选N分别保存，当前认证导入、明确候选授权、独立同提交JUnit与正式Review闭环，上游退回阻止新的验收。16项Go专项、9项Web、类型／语言／构建及真实Windows双Worker旅程通过；GUI当前未验。见[合同](../../contracts/terminal-baseline-development.md)、[报告](../quality/terminal-baseline-development-verification.md)和[摘要](../quality/terminal-baseline-development-summary.json)，58／34／92不变。

前序T130推进[终端原执行核对与代码候选](../development/interactive-development.md)：原会话停止证明、未知输入逐项确认及按失败关闭，与明确授权的候选预览、单独启动、同SHA独立Worker测试、人工Review和问题回流分别保留。已验收精确候选可显式绑定另一未启动任务的基线，不自动派发。实现与边界见[合同](../../contracts/terminal-recovery-candidates.md)、[架构](../architecture/interactive-terminal.md)、[交互](../design/terminal-recovery.md)、[当前专项报告](../quality/terminal-recovery-candidates-verification.md)和[摘要](../quality/terminal-recovery-candidates-summary.json)；检查数量、实际恢复故障、界面与本机链路以该报告为准。

本次真实PTY修改沿固定命令检查、候选封存与独立克隆测试继续原质量链路，不新增模型调用；提前Review拒绝、明确采用／拒绝与唯一问题回流分别记录。恢复仍要求原持久停止证明，不把失联、人工确认未知或历史读取当作进程已停／执行成功。完整恢复故障覆盖、Unix containment、物理多机、跨平台安装和生产验收继续待办，58／34／92不变。T129原终端与讨论开发交接的[历史报告](../quality/interactive-development-verification.md)／[摘要](../quality/interactive-development-summary.json)保留原来源及失败审计。

前序T-REPLY-128推进任务讨论到真实多轮智能体回复：明确模型、单次参数、近期消息范围和 Token／费用预算，沿原 Go 模型网关、唯一 DAG 与私有／Pool Runner 执行，并以准确回执追加助手消息。见[架构](../architecture/task-conversation-replies.md)、[合同](../../contracts/task-conversation-replies.md)、[开发与AI SOP](../development/task-conversation-replies.md)、[当前验证](../quality/task-conversation-replies-verification.md)和[摘要](../quality/task-conversation-replies-summary.json)。真实Go／Node原生CLI、本地受控提供方及网关HTTP已完成三轮／六条持久消息，第三轮由实际界面明确选择模型、输出512、近期20条、Token40000及CNY0.04费用上限后创建；实际中英文1366／390、刷新六条记录及原执行／用量已按报告限定验证。只读回复不会执行终端命令、修改任务开发运行或授予验收权。

T-REPLY-128限定结果：固定模型／预算的真实多轮、原完成回执提交后丢失恢复，以及任务详情→工作台→讨论→追问→配置→真实回复和SSE运行中／已保存状态均有实际证据；六条记录刷新后保留，原执行及7输入／3输出Token可查看。自有标签、视口及隔离服务已显式关闭／恢复。持久任务讨论的发送、刷新及中英文视觉缺口由本次T128组合旅程补验；T127历史档案不改。后续完整供应商、多机、安装矩阵及生产验收仍待交付。交互式[PTY终端](../architecture/interactive-terminal.md)、语言／调试服务、自主代码开发及完整 P0–P7／需求1–26继续交付，58／34／92不变。

前序T-COLLAB-127继续原生任务协作：准确人／API作者、任务／需求／运行依据、事务内序号、纯文本消息、原回执恢复和双语工作台输入。见[合同](../../contracts/task-conversations.md)、[开发与AI SOP](../development/task-conversations.md)、[报告](../quality/task-conversations-verification.md)和[摘要](../quality/task-conversations-summary.json)。实际HTTP回执丢失恢复及接口／组件检查已通过；该阶段浏览器连接中断；持久讨论的当前视觉补验见T128实际组合旅程，原阶段档案保留。消息保存本身不启动智能体、执行或验收。真实持续模型回复及[PTY会话](../architecture/interactive-terminal.md)继续完整交付，58／34／92不变。

前序T-WORKBENCH-126推进原生IDE任务上下文：同一当前授权快照中的任务、需求／采用设计、准确运行、计划用例和正式验收，配套可折叠双语界面及原证据链接。见[合同](../../contracts/workspace-task-context.md)、[AI SOP](../development/workspace-task-context.md)、[完整工作台交互](../design/reference-workbench.md)、[116项参考产品操作与缺口](../product/reference-platform-parity.md)、[报告](../quality/workspace-task-context-verification.md)和[摘要](../quality/workspace-task-context-summary.json)。当前授权HTTP、真实历史复读和中英文1366／390界面已验证，范围以摘要为准；选择任务不绑定代码目录或授予执行／验收权。完整会话、PTY、办公及全平台目标保留，58／34／92不变。

前序T-ROLLOUT-125在准确应用版本、独立制品候选与原计划组上推进受保护的依赖有序发布：原计划分别批准后明确启动，暂停排空，未知／失败暂停，原操作核对及新预览后明确恢复，已健康子项不重做。[架构](../architecture/application-release-rollouts.md)、[操作与AI SOP](../development/application-release-rollouts.md)、[合同](../../contracts/application-release-rollouts.md)、[English](../en/application-release-rollouts.md)、[限定验收报告](../quality/application-release-rollouts-verification.md)和[摘要](../quality/application-release-rollouts-summary.json)。真实本地源码／独立测试／原提供方部署、暂停排空与回执丢失核对恢复，以及中英文390只读界面已验证，范围详见报告；多来源构建、灰度流量、协调回退与完整平台继续待办，58／34／92不变。

T-ROUTING-122继续原P2／P4与需求13、14、21、26：真实有界候选、统一Go网关的一次只读模型建议、可编辑人工采用及原计划明确启动，复用现有独立质量链路。设计及边界见[架构](../architecture/dispatch-recommendations.md)和[报告](../quality/dispatch-recommendations-verification.md)，58／34／92不变。

当前能力按实现范围汇总。下表中的验证均受对应报告的源码、环境和实际场景限制；不会因本次版本统一继承为新代码全部通过。执行成功、独立测试、人工验收与生产发布是不同事实。

完整台账为 **58 项限定实现、34 项待实现，共 92 项**。`FND-INTENT-001`仍待完整交付；T-INTENT-114推进统一智能输入到需求／设计／知识草稿计划、明确预览及同库原子采用，不增加整项完成。T-TRACE-113需求验收追踪及历史T-DELIVERY-112仍以各自报告的源码／环境和台账为准，局部交付不表示平台完成。

T-QUALITY-115在原P5范围内细化自动化测试计划及逐例证据，不新增整项完成或改变上述统计。当前实现边界见[合同](../../contracts/test-plans.md)与[架构](../architecture/quality-evidence.md)；实际双Runner／HTTP／Node报告旅程及定向检查见[阶段报告](../quality/test-plans-verification.md)和[摘要](../quality/test-plans-summary.json)；真实界面仍未验，不能据本阶段推断全平台或生产验收完成。

T-MODEL-118收敛原生编码与 Octop 的共同模型选择、Go HTTP／SSE 网关、逐次剩余输出许可和持久原操作回执。当前实现、实际 SDK／本机端点、独立测试、取消、账务及中英文界面范围统一见[本轮报告](../quality/model-gateway-verification.md)与[摘要](../quality/model-gateway-summary.json)，不沿用历史 T-OCTOP-117 结果替代当前检查，也不改变58／34／92台账。

T-QUALITY-119继续原P5范围：问题的受保护修复周期、原失败用例回归、固定产码候选与不同Worker独立测试、当前正式Review后关单，以及保留旧证据的重开。复用原Go DAG与`command/text-v1`固定档案，不增加派发或审批权威。简洁中英文问题面板及实际检查范围见[当前报告](../quality/issue-repair-verification.md)和[摘要](../quality/issue-repair-summary.json)，不代表完整Issue／Bug全生命周期或生产验收完成。

T-WORKSPACE-120继续原P2／P4与需求13、23：Go宿主工作区调用、原效果回放、拒绝、固定检查及父封存，经现有私有／Pool事件与确切attempt历史展示；本地采集、平台确认和当前页分别表达，默认收起、中英文完整。当前实现与实际范围见[报告](../quality/workspace-operations-verification.md)、[摘要](../quality/workspace-operations-summary.json)和[架构](../architecture/workspace-operations.md)。该阶段不改变58／34／92台账。

T-PROJECT-121继续原P3／P5与需求7、10、12、13：正式项目风险登记、真实建议采用、负责人和顺序评估、任务关联、固定复核及关闭／重开。代码缓解保留当前正式Review与精确候选独立测试，子流程沿受保护关系有界核对；复杂来源不足明确阻止。概览入口及简洁中英文面板、实际双Worker代码链路与界面范围见[报告](../quality/project-risks-verification.md)、[摘要](../quality/project-risks-summary.json)、[PRD](../product/project-risks.md)和[架构](../architecture/project-risks.md)，不代表跨项目portfolio、外部告警或全平台验收完成，58／34／92不变。

四份分享规划均已取得当前可见正文；新增实施蓝图与深度设计的可见范围、截断及采用见同一映射。第一份64个编号、第二份需求26及S01–S12见[逐域采用与缺口映射](../integrations/shared-roadmap-adoption.md)。两份末尾均不完整，未见内容保持未知。它们是设计来源，不作为源码或完成证明；当前草稿计划仅推进需求26的一部分，通用配置与全场景多端旅程仍待实现。

## 当前实现与证据

| 领域 | 当前实现 | 验证入口与明确边界 |
| --- | --- | --- |
| Octop 项目MCP与Harness执行 | 项目专用MCP及17项共享目录；公开Harness1.0.5／ACP0.12.1共用统一ModelBinding与Go网关，真实SSE、六工具、独立工作树、检查／封存及同SHA独立JUnit；中英文配置与可展开执行事实 | [接入](../integrations/octop.md)、[执行架构](../architecture/octop-execution.md)、[当前报告](../quality/model-gateway-verification.md)、[摘要](../quality/model-gateway-summary.json)分别记录官方SDK、本地模型协议、取消树停止与界面结果；完整Octop CLI／GUI、厂商账户、多主机、跨平台及生产仍待验 |
| 身份、组织与治理 | 初始化、登录、会话、组织／项目隔离、真实账号、团队、邀请、角色、撤权及当前凭证授权 | [当前权限](../quality/current-authority-verification.md)、[平台验证](../quality/verification.md)；完整自定义角色／菜单、企业 SSO、OAuth／设备治理及跨数据库验收待办 |
| 项目计划与进度 | 迭代、里程碑、版本化排期、完整项目不可变基线、当前比较、工作日历加权估算与真实业务指标 | [产品与治理](../product/governance.md)、[验证](../quality/verification.md)；计划估算不等于实际工时；当前工作制／日期例外见[日历报告](../quality/resource-calendars-verification.md)，外部HR／请假审批、连续历史、预测和portfolio待办 |
| 业务任务依赖编辑与参考源码整合 | 当前项目完整任务图、图／表共享草稿、最多 100 项原子版本 CAS、全图校验、显式冲突处理及原操作回执恢复；修改撤销旧验收资格并保留执行快照 | [中文操作](../development/task-dependencies.md)／[English](../en/task-dependencies.md)、[合同](../../contracts/task-dependencies.md)、[源码适配清单](../integrations/referenced-conversation-integration.md)、[本阶段报告](../quality/task-dependencies-verification.md)；按实际参考源码重构当前 1.0.0，不是旧 Git 历史 cherry-pick；最多 1000 个任务完整图，实际 API 与界面复验结果以报告为准，分布式／大图性能／生产验收未由本阶段推断 |
| 任务交付与质量链路 | 依赖图所选任务按需读取确切运行、完整节点／测试计数和每类 20 条正式引用；团队预览／派发、证据、人工验收、退回问题与专用来源映射的需求草稿连通 | [操作](../development/task-delivery-chain.md)／[English](../en/task-delivery-chain.md)、[合同](../../contracts/task-delivery-chain.md)、[限定报告](../quality/task-delivery-chain-verification.md)；本机自有 DSH 协议命令实际提交代码，另一 Runner 对同 SHA 执行 3 项断言，界面退回形成 1 个问题并回流 1 个需求草稿；下游 B 保持未启动并等待正式验收，非厂商智能编码效果、物理多机或生产验收 |
| 自动化计划与逐例质量门禁 | 每任务单一计划、不可变草稿／发布修订、任务依据与需求精确版本；原Go DAG同运行绑定与固定候选独立测试、受限原JUnit采集、当前attempt逐例投影、Review重核及明确failed/error用例问题回流 | [中文指南](../development/test-plans.md)／[English](../en/test-plans.md)、[合同](../../contracts/test-plans.md)、[架构](../architecture/quality-evidence.md)、[阶段报告](../quality/test-plans-verification.md)、[摘要](../quality/test-plans-summary.json)保留对应阶段事实。没有人工passed或事后倒签；带计划运行不支持map／子流程／外部流程，完整Issue／TestCase管理和手工／应用／发布测试仍待交付；当前限定修复关单与重开见下一行 |
| 受保护问题修复与回归关闭 | `open/in_progress/closed`业务状态；同事务唯一需求回流与每轮新普通任务，明确绑定当前发布计划及非空回归用例；原测试缺陷必须保留原case和selector；所选报告对应固定产码候选、原attempt／fence／摘要和不同Worker独立测试；当前Review后由有权人员关单，重开保留旧关闭证据并开始新周期 | [中文指南](../development/issue-repair.md)／[English](../en/issue-repair.md)、[合同](../../contracts/issue-repair.md)、[架构](../architecture/issue-repair.md)、[当前报告](../quality/issue-repair-verification.md)、[摘要](../quality/issue-repair-summary.json)。简洁问题面板提供当前事实、阻塞和有界历史；`command/text-v1`沿固定档案、当前库存及原结果合同执行，通用记录不能改生命周期或伪造关系。测试独立报绿不能代替修复来源，复杂未证明候选来源继续阻止；实际结果以报告为准，完整分诊、缺陷治理、应用／发布回归和多机生产范围仍待交付 |
| Runner 本地执行日志诊断 | `--inspect-state` 有界读取单个 journal，白名单计数及最多 100 条引用，缺失／损坏／不可读／超限分别表达，不创建目录、不读取凭据、不改 ACK 或重跑 | [中文指南](../agents/runner-journal-inspection.md)／[English](../en/runner-journal-inspection.md)、[合同](../../contracts/runner-journal-inspection.md)、[本阶段报告](../quality/task-delivery-chain-verification.md)；仅本地执行日志，非完整设备健康、服务器回执、持久隔离原因或自动恢复能力 |
| 需求与设计 | 版本化产品／体验／技术设计、可审阅任务提案、明确发布及原子采用、固定执行与验收依据 | [设计采用](../quality/requirement-design-verification.md)、[依据展示](../quality/task-design-context-verification.md)；完整 AI 需求与设计编辑、可运行 UED 原型待办 |
| 需求验收追踪 | 当前正式需求版本／条目序号 → 已发布提案 → 受保护采用任务 → 确切运行、当前测试节点和正式验收读取 | [中文指南](../development/requirement-traceability.md)／[English](../en/requirement-traceability.md)、[合同](../../contracts/requirement-traceability.md)、[限定报告](../quality/requirement-traceability-verification.md)；需求变更不沿用旧序号映射，执行／测试／验收分别展示，实际验证以报告为准；完整TestPlan／TestCase、跨版本标准身份及应用版本／发布矩阵待办 |
| 全平台智能输入与自动配置 | T-INTENT-114自然语言目标／选定安全来源，经唯一Go DAG一次只读模型生成最多8步需求／设计／知识草稿计划；可编辑提案、只读预览、本次明确确认、真实同库原子采用及原操作恢复 | [中文指南](../development/intelligent-operations.md)／[English](../en/intelligent-operations.md)、[合同](../../contracts/intelligent-operations.md)、[架构与验收SOP](../architecture/intelligent-operations.md)、[限定报告](../quality/intelligent-operations-verification.md)；只采用同库本地草稿，不发布／启动执行或改配置，实际验证以报告为准。FND-INTENT-001未勾选，全场景意图／补全、持久自动策略、配置变更／实际验证与多端完整旅程待办 |
| 知识、记忆、规范与技能 | 项目资产版本、审阅发布、固定任务上下文、组织共享目录、授权检索、跨项目独立草稿采用、模型辅助提案及发布溯源 | [项目资产](../quality/project-knowledge-verification.md)、[共享技能](../quality/skill-catalog-verification.md)、[模型提炼](../quality/knowledge-intelligence-verification.md)；授权向量索引、外部记忆、可执行插件与完整专家策略待办 |
| 智能规划与失败分诊 | 固定来源的单次模型规划、人工修订／采用、持久站内风险；真实失败证据形成不可变观察与人工修复需求 | [规划](../quality/intelligent-planning-verification.md)、[分诊](../quality/failure-diagnosis-verification.md)；本地自有协议证据，非厂商效果或长期自主恢复验收 |
| DAG 与共享执行池 | 条件／并行／汇合／map／子流程／人审／等待／补偿、增量前沿、attempt／fence／回执、双边授权及分层加权准入 | [增量调度](../quality/scheduling-verification.md)、[平台门禁](../quality/scheduling-platform-guards.md)、[共享池](../quality/execution-pool-verification.md)；同机多进程不等于物理分布式、完整公平／吞吐或长期容量验收 |
| 工具派发与应用工具 | 固定工具版本、真实库存、当前资格与路由推荐、明确采用后预览／启动；官方 MCP SDK 项目读取和受控草稿 | [工具执行](../quality/tool-execution-verification.md)、[工具推荐](../quality/task-dispatch-options-verification.md)、[项目 MCP](../quality/project-mcp-verification.md)、[只读诊断](../quality/developer-connection036-verification.md)；探测、复制配置和单次诊断不等于完整客户端接通 |
| 原生与外部 CLI 执行 | 原生受限工具、独立 Git 工作树、固定开发检查、机器封存及另一 Runner 同 SHA 正式测试；Harness／DSH、CodeBuddy 的限定协议与 Node 入口 | [原生编码](../quality/native-coding-verification.md)、[团队 CLI](../quality/team-cli-verification.md)；本地协议夹具与真实厂商任务分开，其他工具、会话恢复及 OS 隔离待办 |
| OpenCode 当前固定 SDK 执行 | 官方SDK／server1.19.4、固定普通／专家配置；当前轮MCP连接、真实六工具宿主、操作账本／封存、Go合成停止证明与observe-only门禁；简洁运行卡及测试下钻 | [设计与SOP](../architecture/opencode-execution.md)、[真实限定验证](../quality/opencode-actual-execution-verification.md)：官方Windows CLI／SDK配自有确定性模型协议、两Worker、候选交接与精确SHA独立测试；商业模型效果／账单、物理多机、Unix运行、会话恢复及生产待办 |
| Codex 独立模型执行 | 真实目录选择模型／推理强度、固定单任务计划、安装版 app-server 私有工作树编码、持久原进度／结果与部分用量、明确取消门禁 | [Codex 限定验证](../quality/codex-app-server-verification.md)；自有 Responses 端点实际编码观测 88 Token、费用未定价，配置 ACK 不证明提供方模型身份；observe-only 提醒不等于硬预算；专家逐项选择与修订已有[跨入口证据](../quality/model-selection-workflows-verification.md)；Codex活动工作区已接通[只读broker](../quality/codex-workspace-observation-verification.md)；协调保存、冲突修复及完整远程IDE待办 |
| 模型、预算与用量 | 原生／Octop共用部署、参数、attempt额度、冻结运营方报价、私有／Pool原子预留与Go请求账本；按剩余总输出许可发送、同step同正文只回原缓存，未知不重复外发；模型报价入口与简洁双语事实；专家／修复仍沿明确用途门禁 | [统一网关报告](../quality/model-gateway-verification.md)、[摘要](../quality/model-gateway-summary.json)、[共同合同](../../contracts/model-execution.md)；已知计数按固定费率估算，缺失／partial不记零、未知保留hold，费用不等于供应商账单；完整报价目录、跨工具速率、厂商成本与全主体额度仍待验 |
| 专家团队与任务交接 | 版本化专家职责、固定工具／流程／知识、原子多任务派发、验收依赖推进、暂停／恢复／停止、未开始项修订及明确人员交接 | [团队派发](../quality/expert-team-dispatch-verification.md)、[依赖编排](../quality/expert-team-orchestration-verification.md)、[任务交接](../quality/task-handoff-verification.md)；运行完成不自动验收，完整自主专家协同与跨组织远端待办 |
| 候选、修复与仓库集成 | 机器封存 bundle、跨 Runner 导入、固定多源 Git 合成、精确结果独立测试、明确下游基线；冲突独立修复任务、受控重试／修订；固定提供方审批后 CAS 快进 | [候选交接](../quality/candidate-transfer-verification.md)、[多源合成](../quality/candidate-composition-verification.md)、[冲突修复](../quality/candidate-conflict-repair-verification.md)、[修订](../quality/candidate-repair-revisions-verification.md)、[仓库集成](../quality/repository-integration-verification.md)；本地裸仓库／同机 Runner 证据，完整合并策略、动态来源继承、物理远端与未知效果恢复待办 |
| 工作区与 IDE 基础 | 桌面项目范围原生选择／明确绑定及持久根证明；本地 Monaco 目录快照分页、范围筛选、草稿／CAS 保存、只读 Git index／worktree 差异；原生／Codex活动工作区出站 list／read；原生一次保存许可、原回执及显式 inspect、手工差异合并 | [桌面目录与真实文件链路](../quality/desktop-workspace-verification.md)：HTTP保存／回执／184表重启已验证，原生GUI因运行时启动失败仍待验；[目录与CLI](../quality/workspace-directory-verification.md)、[Git](../quality/workspace-git-verification.md)、[执行读取](../quality/workspace-observation-verification.md)、[执行保存](../quality/workspace-write-verification.md)、[传输能力](../quality/workspace-capability-verification.md)、[Codex读取](../quality/codex-workspace-observation-verification.md)；Codex只读，执行保存仅已有 UTF-8 文本且新旧均 ≤48KiB，创建／删除、大文件、Git写、LSP／DAP和完整IDE待办；独立attempt终端、原执行核对及候选交付当前限定范围见[T130](../development/interactive-development.md) |
| 自动化与事件 | 持久人工／cron 触发、时区／错过／重叠策略、固定来源、签名 GitHub／Suma 事件、去重交付、当前授权 SSE 与断线续传 | [自动化](../quality/automation-verification.md)、[事件入口](../quality/ingress-verification.md)；本机真实 Git／协议事件不替代公网厂商、生产 CI 身份或消息通道验收 |
| 质量、制品与发布 | 独立测试证据、人工验收、失败去重回流；真实字节上传封存、包测试、固定范围、本机静态站点批准／发布／核对／独立回退 | [验收矩阵](../quality/acceptance.md)、[发布控制](../deployment/release-control.md)、[发布证据](../architecture/release-evidence.md)；生产 CI、签名、远端／容器部署、灰度与生产健康待办 |
| 国际化与交互 | 成对中文／英文目录、持久账户／组织偏好、结构化诊断、原文保留及代表性键盘／刷新旅程 | [语言偏好](../quality/locale-preferences-verification.md)、[自有前端](../quality/remaining-localization-verification.md)、[后端诊断](../quality/backend-diagnostics-verification.md)、[运行诊断](../quality/native-diagnostics-verification.md)、[显示资格](../quality/platform-display-verification.md)；目录键数不代表完整平台、库内控件、通知、安装器及所有语言表面完成 |
| 存储与客户端交付 | SQLite、本地文件／Lance 与 OceanBase／PostgreSQL 驱动路径；CLI／TUI、Web构建、六目标桌面打包入口／原生CI配置、实际Windows未签名安装／便携包及控制程序六架构核验 | [数据设计](../data/transactional-store.md)、[构建](../deployment/build-reproducibility.md)、[运行包](../deployment/runtime-bundles.md)、[桌面](../deployment/desktop.md)、[当前跨平台打包验证](../quality/desktop-packaging-verification.md)；真实分布式数据库、S3／多模态全链路、安装／签名／目标系统运行、离线协作、移动 App 与 SaaS 商业化待办 |

## 1.0.0 初始化与验证边界

应用统一为单一 1.0.0 新库初始化，当前唯一数据合同为 `suma-initial-quality-v1`，不保留历史库升级、旧字段别名或旧协议适配分支。包括旧网关声明在内的不匹配合同、混合组件或未知合同在初始化写入前拒绝；现有 `.suma/`、原凭据、业务原件与专项验收档案保留，不迁移、回填、清空或重写。新实例另选独立空目录／数据库，检查范围见[本轮报告](../quality/issue-repair-verification.md)及[摘要](../quality/issue-repair-summary.json)。产品版本、协议版本、业务修订和数据合同分别管理。

既有主预览曾以保留 188 表与原会话的前向交付取得证据，准确过程留在 [Codex 专项报告](../quality/codex-app-server-verification.md)。该事实不作为本次新库初始化变更的迁移承诺，也不表示当前代码已经重新部署。当前变更的检查结果以 [验证入口](../quality/verification.md)为准。

OceanBase／PostgreSQL 的驱动与 DDL 不等于实集群验收，本地检索不等于完整 OCR／转录，交叉编译不等于安装或运行，单元测试不等于目标执行槽位容量已通过。厂商模型、正式业务负责人签收与生产验收均需独立证据。

## 后续交付与使用

项目执行记录现提供独立的“团队执行”入口，无需先选任务即可查看正式分页计划、验收进度和阻塞，并复用原详情／任务／运行链接；见[团队操作指南](../development/team-execution.md)与[限定证据](../quality/team-execution-verification.md)。优先继续补齐智能选择与模型授权／报价、完整 IDE、知识／技能同步、真正多机与厂商任务闭环；身份、存储、协作、渠道、移动端、支付、安装和规模仍按完整台账实施，不以局部适配缩减范围。

浏览器默认 `127.0.0.1:4310`。执行需另建项目范围 Runner 凭证并配置许可命令；浏览器文件编辑需明确服务端允许根目录和项目绑定。当前本地读取／保存已绑定实际目录身份，工作区变化保留草稿并明确采用新依据，普通与桌面绑定均按原操作回放；[实际HTTP、重启和中英文浏览器验证](../quality/workspace-file-context-verification.md)通过。桌面原生窗口／选择器旅程仍待验。首次初始化与部署配置见 [服务器说明](../deployment/server.md)。

验证数据使用独立 `.cache/e2e/`，保留用户正式运行目录。正式产物进入 `dist/`，专业文档进入 `docs/`；秘密、数据库、原始日志、截图、依赖缓存和本地二进制不提交。历史阶段过程由 Git 和专项报告追溯，本页只维护当前能力。

## 初始化结构

当前源码以单一1.0.0及`suma-initial-quality-v1`新库结构建立，不维护旧队列／资源镜像、旧内核分代或凭证／设计索引回填。当前新库、重启及不匹配声明拒绝的实际检查见[T-QUALITY-119报告](../quality/issue-repair-verification.md)；[初始化设计](../architecture/initial-version.md)、[原初始化验证](../quality/initial-version-verification.md)和[文件上下文验证](../quality/workspace-file-context-verification.md)保留对应旧阶段的表数和环境，不推断为当前全量通过。既有用户数据库保持原样。
