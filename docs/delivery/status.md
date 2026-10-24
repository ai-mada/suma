# 当前交付状态

## 当前 T-P4-005：增量调度与可解释执行准入

在`1e9d77b`基线上完成持久增量推进、就绪/唤醒索引、丢ACK领取恢复、项目准入策略预览与确认、队列阻塞和持久attempt历史，以及用量已报告零/未知的区分。最终Go606通过事件、0失败/跳过、vet通过；前端、合同和当前交付资产见[阶段验收](../quality/scheduling-verification.md)及[机器摘要](../quality/scheduling-summary.json)。实际界面槽位0→2、双Runner真实并行、独立测试、人工批准后5节点/3尝试/22事件，累计额度3；旧78表逻辑摘要保持，新24程序当前源码一致。Git只交付必要源码/设计及精简验证，原始档案留本地。跨组织共享池、多项目执行身份、分层加权公平、模型网关及完整P0–P7继续实施。

## T-P4-004 历史：固定工具派发与完整任务依据

工具档案、实时观察、精确命令资格、固定任务/需求/流程/工具预览和原子启动、typed执行事实、独立Agent授权已交付。实际协议夹具完成隔离开发、独立2项测试、人工验收、失败去重回流和两次真实停止；它不代表厂商模型效果。最终Go512个通过事件、0失败/跳过，vet通过；JS212项、类型和生产构建通过，合同147模型/117路径/344引用。新24程序批次`20261024T051829203Z-df0ca9b5`已构建与独立核对，Windows实际激活复验；4runs/8nodes/5attempts，原三次运行/35事件及验收回流保持。工具最终paused v5。响应式完整节点列表、桌面依赖图、精确需求导航与焦点/主题部分验证完成。[阶段验收](../quality/tool-execution-verification.md)、[当前运行包](../deployment/tool-stop-runtime-bundles.md)。完整P0–P7、厂商真实账户、集群、安装和规模目标继续实施。


日期：2026-10-24。状态：可运行开发版本，完整产品实施进行中。

## 当前可运行

| 能力 | 源码入口 | 已验证范围 |
|---|---|---|
| 初始化/会话/项目权限 | internal/platform/auth.go、http.go | 本机SQLite API与浏览器初始化 |
| 组织/团队/真实账号治理 | internal/platform/governance*.go | 单次邀请、最后所有者保护、授权墓碑、真实双连接竞争、浏览器组织切换与隔离 |
| 迭代/里程碑/计划/容量 | internal/platform/planning*.go、baselines.go | 双版本原子排期、全项目不可变快照、自然周估算分摊、撤权排除；浏览器真实创建/比较/完成 |
| 需求/任务/问题闭环 | internal/platform/records.go、delivery.go | 幂等、批次回滚、依赖、验收失效、回流 |
| 持久DAG | internal/dag | SQLite编译/条件/等待/恢复/子流程/补偿，以及增量前沿/索引/资源CAS/重试窗口/真实历史迁移；见当前阶段摘要 |
| 持久自动化与事件续传 | internal/automation、internal/platform/automation*.go、run_stream.go、AutomationWorkspace.tsx | 真实人工/cron双触发、固定修订与流程、时区/错过/重叠、撤权与事务竞争；Chrome完整终态、正常轮换与重启上下文保持 |
| 签名事件入口与交付追踪 | internal/platform/automation_ingress*.go、AutomationIngress.tsx、internal/cli/webhook.go | GitHub原生/Suma v1协议、本机真实Git标签/CI/监控事件触发、固定来源修订、当前授权、回执去重；重启不重复执行，来源元数据不注入执行命令 |
| Runner与候选交接 | internal/runner、cmd/suma-runner、internal/dag/candidate.go | Windows实际独立Git工作树、进展持久ACK、固定候选交接、原失败保留及真实测试 |
| 真实制品与不可变发布范围 | internal/platform/artifact_*.go、release_candidate_service.go | 真实上传字节、封存、build/package-test证明、固定C与当前已验收任务范围 |
| 发布、核对与独立回退 | internal/deployer、release_deploy*.go、ReleaseWorkspace.tsx | 真实浏览器5次操作A/B/A/B/A；健康404与实际活跃内容分开，独立批准、单调代数、问题去重回流 |
| 原生智能体 | packages/agent-runtime | 有界循环/工具协议；未连接生产模型 |
| 外部工具 | 工具目录与许可命令、HTTP桥接 | 实机只读探测/本地协议测试；未调用收费模型 |
| 知识服务 | packages/knowledge | 真实本地Lance全文/向量、版本隔离、墓碑 |
| 界面与编辑 | apps/web | 最终web244项、类型及生产构建通过；真实治理/计划/候选/发布、自动化与本阶段准入闭环；局部四宽度/主题、精确attempt和图键盘选择见当前验收；完整体验矩阵继续实施 |
| Windows桌面 | apps/desktop、dist/desktop | 未签名NSIS/portable及目录构建；安装/GUI/升级仍分别验收 |
| CLI与部署配置 | cmd/suma、deploy、Dockerfile | Bubble Tea/游标分页与签名交付测试；部署8项测试及Compose组合/配置校验，镜像/Helm未实跑 |
| Go四程序交付 | scripts/build-runtime.ps1、dist/runtime/builds | 六目标×控制面/Runner/CLI/Deployer，真实编译、构建信息与逐文件摘要；目标系统运行分别验收 |

## 当前完整性边界

完整目标与未完成项逐项列在implementation-plan.md。需要继续完成：分片增量调度/完整跨租户公平、实际合并与终端语言调试、工作制容量日历/实际工时/连续进度历史、组合规划、完整设计编辑、厂商Git/CI/监控账户与公网交付验收、完整OAuth及设备治理、生产CI/签名/远端和容器部署/灰度、授权智能故障修复、离线协作、办公/媒体/渠道、支付计费和跨平台正式交付。签名接收、本机协议事件与真实下游执行不等于真实厂商接入或外部CI身份证明。制品与Deployer首轮只支持本机静态站点；单个上游候选交接不包含多分支聚合和跨执行节点Git对象分发。

OceanBase/PostgreSQL驱动与DDL代码不等于真实集群验证；本地Lance测试不等于多模态OCR/转录全部完成；单元测试不等于目标1000执行槽位已通过。

## 使用入口

浏览器默认127.0.0.1:4310。首次创建管理员和项目；执行需要另建项目范围Runner凭证并配置本地命令许可。代码编辑需要服务端设置允许根目录，再绑定项目工作区。

验证使用独立`.cache/e2e`数据，保留用户正式`.suma`数据目录。交付产物使用dist，专业文档使用docs；秘密、运行数据库和临时日志均不提交。T-P4-003历史汇总：Go385个通过事件（含子测试和helper），0失败、0跳过，9个测试包及4个只编译命令包，vet通过；JS/TS包测试173项、部署8项、类型检查与网页构建通过，最终文案后另复验12项前端消费者；合同131模型/110路由/315个Schema引用。18次本机签名/协议请求形成7条交付、3次真实成功运行、9个节点、6份派生证明和3个实际制品；Windows交付控制面与CLI重启后回执去重，原始执行仓库保持干净。该历史阶段六目标四程序批次为 `20261024T033010763Z-94b39f3b`；[事件入口专项验收](../quality/ingress-verification.md)披露具体范围。此前T-P4-002的329个Go通过事件、1个权限不足跳过、138项JS测试及原始证据仍保存在[自动化专项验收](../quality/automation-verification.md)，不作为本轮跳过项。阶段功能完成后按用户要求commit；P5及P4-002历史证据保持原切面，完整P0–P7尚未签收。
