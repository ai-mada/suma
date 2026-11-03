# 真实CLI只读探测、准确诊断与快速接入

T-REAL-CLI-ONBOARDING-226，速码1.0.0，负责人：主集成代理。基线f716279e627295fcf8d742f29bd71a21e74d7454；前阶段T225为已核实59文件提交推送的progress。原31项／P0–P7及58limited／34pending／92继续active。本阶段沿原扫描、维护owner、停止证明、配置generation、库存事务和Quick；不安装新CLI、不创建第二调度器，不以受控fixture代替厂商证据。

## 目标与真实来源

使用已安装Codex原生程序，仅执行原版本／帮助元数据路径，贯通准确安装→准备→库存→暂停登记。候选位于官方Codex应用bin，368970544字节，静态SHA256 3097c909fb6d5cd63510d44aceb1d42df9f6e85722a3e447e477def1ecf60e16，嵌入签名主体OpenAI OpCo, LLC；未核在线证书链。实际CLI版本和帮助仍需受控实读，不复制该369MB程序、不读取现有登录、不发Prompt或模型请求，不自动enable或Start。

Claude SDK0.3.314的vendor manifest记录2.1.314及平台校验值，仅证明发布metadata；尚未定位对应已装真实native，不得以旧owned PE或manifest值冒称实际文件。官方当前CLI参数与版本要求已实读，保现有严格CLI策略；safe-mode／disableAllHooks不覆盖管理员hooks，文件工具限制不等Shell／OS隔离，预算observe_only不等计费硬帽。参考Multica本轮连接器锁定main 2115a513a375bfe2910b76725b617504faee4b25；仅metadata已核，新源和许可必须另行核实，不把旧24b54审核冒充新提交。新增ZCode等能力在原工具完整待办中记录。

## 单一只读探测生命周期

版本、帮助和固定协议探测沿原runProbeProcess／processController／collector。统一可信私有probe scope：无显式scope时创建受控HOME、USERPROFILE、APPDATA、LOCALAPPDATA、XDG配置／缓存、TEMP／TMP／TMPDIR、CODEX_HOME及working目录，不继承用户账号目录；不把任意spec.Env中的HOME当可信依据。

现有Codex／SDK／Cursor helper创建的私有目录通过内部context显式声明，原features配置和必要runtime工作目录保持，不再被collector blanket覆写；纯元数据配置不能借此扩大任务权限。保原begin／stop receipt与持久owner，不另建全局进程管理器。独立scope只在原停止／等待事实可靠后清理，未知停止保留目录／原核对，不删除可能仍活动的状态；collector-owned目录沿原Journal生命周期。

CLI版本只取有界stdout，stderr仍有界／净化但不能成为版本首行；帮助继续原固定协议允许流。版本／安全flag／协议验证不放宽，不按厂商stderr文本正则猜错误。实际未执行模型或登录不能声称验证了账号、模型健康或全部协议会话。

## 唯一诊断合同

在原ToolInstallationItem新增可选且非null的diagnostic，类型ToolInstallationProbeDiagnostic，恰好三个必填字符串stage、code、next_action。不存在表示没有可分类故障事实，不补造已通过；不是产品版本兼容分支。launch_supported=true、not_installed、workspace_required、configuration_required及acp_configuration_required不得同时含故障诊断。原reason_code保粗观察语义，只有resolve不能判定安装时新增scan_incomplete；不将权限错误冒为未安装。

阶段：resolve、source、version、help、protocol。固定码与动作如下，字段组合由共享校验器和同源Schema约束：

| code | stage | next_action |
|---|---|---|
| source_unavailable | resolve／source | guide |
| source_changed | source | scan |
| configuration_invalid | source | guide |
| permission_denied | 任一阶段，须真实errors.Is依据 | guide |
| version_unsupported | version | guide |
| launch_failed | version／help／protocol | guide |
| probe_failed | version／help／protocol | guide |
| probe_timeout | version／help／protocol | scan |
| output_limit_exceeded | version／help／protocol | guide |
| help_contract_mismatch | help | guide |
| protocol_mismatch | protocol | guide |
| stop_unverified | version／help／protocol | none |

低层typed error保存内部cause并可Unwrap，公开只生成固定三字段，无stderr、程序路径、argv、环境值或秘密。已知version/help/协议不符在原生产点显式赋码；非typed错误保持未分类而不regex猜测。原SourceSHA／ref／fence／nonce和完整原请求重放不改。诊断不能产生配置／安装／启用／执行资格。

## 简洁本地与远程交互

原扫描行按需显示准确阶段与固定原因，动作复用原指南或显式重检：远程重检只走原create([adapter_id])，本机重检走原scan(true)，标签区分。行级指南明确传目标adapter／草稿／Worker，不依赖先setState再读取旧adapter。失败行有诊断动作时不堆重复准备按钮；能力／来源／会话／unknown／pending／busy及原回执恢复优先。未知停止绝不能显示可重probe动作，即便来自历史结果；没有回调只展示建议。

本地和远程共用严格reader：缺失允许，显式null、别名、未知字段／枚举／错配动作均拒绝；原粗原因与真实诊断不相互覆盖。中文／英文共用组件和状态，错误不直接展示机器原文。原配置、模型、预算、独测和验收链保持，完整平台范围不缩小。

## 并行所有权与必要验证

Root：本合同、ii共享类型／校验、同源Schema和generated、当前文档／状态、最终集成Git。Runner：可信scope／原probe生命周期、stdout版本、typed failure、已知CLI与source mapping／原installationItems、相应必要测试。UI：单诊断model、remote/local严格reader及原行动作、Quick准确target callback、CNEN与定向测试。Control：原HTTP strict Item诊断入库／回执隐私与两范围必要测试、真实Codex原库Worker的受控计划／记录，不能擅自运行vendor。

复用T225未变的generation／source pins／权限／双边授权／异常恢复证据，不重跑完整套。必要检查覆盖枚举／严格presence／隐私／stdout警告、scope保真与未知停止保留、原回执及正确行target。真实Codex实跑限原版本+exec帮助；使用原HTTP／SQLite／Worker／暂停登记，当前源和native SHA前后核对、进程停止证据与隔离目录分别记录。代表中英文只验新诊断／动作；厂商模型、其他CLI、SDK资源、物理多机和完整OS／DB／安装／规模／生产仍按原31项／P0–P7继续。

## 构建资源

C盘满导致helper／审批初始化失败已通过可恢复迁移处理：仅本任务Go缓存常规包和3份旧suma-contracts缓存程序，逐文件SHA复制验证后迁至项目忽略目录，源码／验收证据不动，未知条目触发停止。新process-local GOCACHE读取.cache/go-cache-current.json指向D盘，不改用户／机器环境、不删除未知目录。完整迁移审据在.cache/real-cli-onboarding-226/go-cache-relocation.json。
