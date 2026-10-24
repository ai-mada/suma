# 当前交付状态

日期：2026-10-24。状态：可运行开发版本，完整产品实施进行中。

## 当前可运行

| 能力 | 源码入口 | 已验证范围 |
|---|---|---|
| 初始化/会话/项目权限 | internal/platform/auth.go、http.go | 本机SQLite API与浏览器初始化 |
| 组织/团队/真实账号治理 | internal/platform/governance*.go | 单次邀请、最后所有者保护、授权墓碑、真实双连接竞争、浏览器组织切换与隔离 |
| 迭代/里程碑/计划/容量 | internal/platform/planning*.go、baselines.go | 双版本原子排期、全项目不可变快照、自然周估算分摊、撤权排除；浏览器真实创建/比较/完成 |
| 需求/任务/问题闭环 | internal/platform/records.go、delivery.go | 幂等、批次回滚、依赖、验收失效、回流 |
| 持久DAG | internal/dag | SQLite编译/并行/条件/等待/恢复/子流程/补偿测试 |
| 持久自动化与事件续传 | internal/automation、internal/platform/automation*.go、run_stream.go、AutomationWorkspace.tsx | 真实人工/cron双触发、固定修订与流程、时区/错过/重叠、撤权与事务竞争；Chrome完整终态、正常轮换与重启上下文保持 |
| Runner与候选交接 | internal/runner、cmd/suma-runner、internal/dag/candidate.go | Windows实际独立Git工作树、进展持久ACK、固定候选交接、原失败保留及真实测试 |
| 真实制品与不可变发布范围 | internal/platform/artifact_*.go、release_candidate_service.go | 真实上传字节、封存、build/package-test证明、固定C与当前已验收任务范围 |
| 发布、核对与独立回退 | internal/deployer、release_deploy*.go、ReleaseWorkspace.tsx | 真实浏览器5次操作A/B/A/B/A；健康404与实际活跃内容分开，独立批准、单调代数、问题去重回流 |
| 原生智能体 | packages/agent-runtime | 有界循环/工具协议；未连接生产模型 |
| 外部工具 | 工具目录与许可命令、HTTP桥接 | 实机只读探测/本地协议测试；未调用收费模型 |
| 知识服务 | packages/knowledge | 真实本地Lance全文/向量、版本隔离、墓碑 |
| 界面与编辑 | apps/web | 类型/构建/129项回归；真实治理/计划/候选/发布与自动化闭环，部分详情四种实际宽度、深浅主题、弹层Escape与回焦点 |
| Windows桌面 | apps/desktop、dist/desktop | 未签名NSIS/portable及目录构建；安装/GUI/升级仍分别验收 |
| CLI与部署配置 | cmd/suma、deploy、Dockerfile | Bubble Tea/游标分页测试；Compose校验，镜像/Helm未实跑 |
| Go四程序交付 | scripts/build-runtime.ps1、dist/runtime/builds | 六目标×控制面/Runner/CLI/Deployer，真实编译、构建信息与逐文件摘要；目标系统运行分别验收 |

## 当前完整性边界

完整目标与未完成项逐项列在implementation-plan.md。需要继续完成：分片增量调度/完整跨租户公平、实际合并与终端语言调试、工作制容量日历/实际工时/连续进度历史、组合规划、完整设计编辑、Webhook/Git/CI/监控签名输入与设备治理、生产CI/签名/远端和容器部署/灰度、授权智能故障修复、离线协作、办公/媒体/渠道、支付计费和跨平台正式交付。制品与Deployer首轮只支持本机静态站点；单个上游候选交接不包含多分支聚合和跨执行节点Git对象分发。

OceanBase/PostgreSQL驱动与DDL代码不等于真实集群验证；本地Lance测试不等于多模态OCR/转录全部完成；单元测试不等于目标1000执行槽位已通过。

## 使用入口

浏览器默认127.0.0.1:4310。首次创建管理员和项目；执行需要另建项目范围Runner凭证并配置本地命令许可。代码编辑需要服务端设置允许根目录，再绑定项目工作区。

验证使用独立`.cache/e2e`数据，保留用户正式`.suma`数据目录。交付产物使用dist，专业文档使用docs；秘密、运行数据库和临时日志均不提交。当前T-P4-002汇总：Go329个通过事件（含子测试和helper）、1个Windows目录链接场景权限不足跳过，vet通过；JS/TS包测试138项、类型与网页构建、120模型/103路由漂移检查通过。当前六目标四程序批次为20261024T023250966Z-97012787；[专项验收](../quality/automation-verification.md)保留实际范围和原失败证据，不能据此签收完整P0–P7。阶段功能完成后按用户要求commit；P5历史证据保持原切面，独立部署4测试是此前记录。
