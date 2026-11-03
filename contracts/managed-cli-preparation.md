# 已安装CLI的远程准备与准确配置

T-MANAGED-CLI-PREPARATION-225，速码1.0.0，主集成代理。基线 `1872774afc1b79c67fdac0b0b7a871f1d0261b23`。前阶段为已核实并推送的progress，完整31项／P0–P7、58limited／34pending／92保持active。本合同扩展[原维护准备合同](managed-tool-preparation.md)，复用同一域表、API、权限、操作回执、维护owner、不可变generation、关联库存事务和原Quick；不是新调度器或安装服务。

## 当前完整合同

自动adapter总计六项：原Qoder ACP、Cursor CLI，以及codex-jsonl、codebuddy-cli、dsh-multica、claude-code-cli。两SDK及Codex App Server需要各自明确资源／provider合同，仍从原手工入口接入，不从检测到native推导SDK可用。

Configuration仍为且仅为两个必填字符串 model、credential_env_ref。ACP两项沿原严格模型／凭据校验。Claude CLI必须用户明确固定model、credential_env_ref必须空，复用原Claude模型token校验、claudeArguments(model)与完整claude_execution，不沿用扫描预设默认模型。其他三项两个字符串必须都空，明确采用准确已检测本机命令，本次不覆盖其模型或账号；不忽略用户提交的非空model/ref。

原独立allow-tool-preparation默认false且必须installed discovery；maintenance-only与任务AllowExec分开。能力只声明实现实际可处理的adapter，准确安装仍由原scan结果和私有来源核对。ACP资格仍installed=true/launch_supported=false/acp_configuration_required；新四项需installed=true/launch_supported=true/workspace_required、合法版本和完整SHA。Claude还需原受支持版本2.1.314规则。未知工具、SDK、版本／来源不完整拒绝，不自动填路径或降级。

| 目录adapter | Profile adapter/protocol | 平台准备配置 |
|---|---|---|
| codex-jsonl | codex / codex-jsonl-v1 | 两字符串空，原命令 |
| codebuddy-cli | codebuddy / codebuddy-jsonl-v1 | 两字符串空，原命令 |
| dsh-multica | harness / dsh-multica-jsonl-v1 | 两字符串空，原命令 |
| claude-code-cli | claude / claude-jsonl-v1 | 明确model，原CLI登录，不收凭据ref |

## 机器来源与后续稳定性

扫描的私有不可变回执保存原固定CommandSpec、准确原生文件／Node host及必需安装依赖pins；浏览器没有路径、命令、env值或资源树输入。准备仅从原scan取得来源，不调用会重新扫描PATH的PrepareToolCommands，也不猜argv。Claude仅用原builder替换固定模型。writer沿原有根目录独占新文件，生成新命令ID，不覆盖既有配置。

原生程序核完整字节SHA。JS安装必须由原resolver准确包／模块根，复用有界rooted遍历核完整受限可加载安装树及必要hoisted／sibling解析范围；Node host单独pin。链接、外部或超限依赖缺乏准确证明时明确回原手工指南，不宣称entry hash是完整闭包。pins进入生成CommandSpec及其正式profile摘要，后续Describe／Execute／恢复静态核对，安装依赖漂移不能沿旧Profile继续。工作区内容、CLI账号和用户插件不被宣称已打包为安装闭包。

Cursor旧接线检查发现inspectCursorPackage第二返回值是Node路径，不能与scan Version相比较。当前实现按其真实返回语义核准确Node路径和package闭包，版本来自原真实扫描，不通过猜值修复。

## 正式完成与恢复

沿原typed intent、持久started、不可变文件／接纳receipt／head、内存snapshot和每scope claim门；新generic intent与ACP intent互斥、按准确Operation.Adapter判定。文件存在和SHA一致不等原接纳回执。重启、断连、prepared或库存ACK丢失恢复原操作／body/key，不重复writer或探测；未知保持原占用和核对。

准备后的元数据观察仅原版本和协议能力；maintenance-only可得到完整但available=false的Profile，不打开执行或调用模型。generic observer使用原CLI校验与完整能力，前三项保持原plain Profile，不添加额外workspace/model/terminal/SDK能力；Claude完整能力与明确model相等。原generation receipt可接受六类准确Profile；control额外核parent/new SHA、commandID、准确native版本／SHA、原stop proof/source以及本次配置。原关联库存tc.EqualProfile完整相等并同事务completed，ACK不续旧库存TTL。

Quick采用准确完整Profile与已完成preparation pin，可保存paused；enable独立授权，实际Preview／Start还需当前available和预算。不同动作不自动合并。准备不安装、登录、初始化厂商协议或运行模型，也不证明后续开发、独测和人工验收。

## 交互和并行交付

原扫描行按准确能力显示准备入口；同一个panel/helper决定三种模式：ACP模型+凭据、Claude仅模型、其他原生配置仅说明+确认。未知adapter拒绝，不能展示错误模型输入。原pending／unknown恢复优先，保A→B→A来源代次和回调保护。中英文使用原组件／tokens，移动纵向排列，不新增复杂向导。现有普通／Pool指南开关复用，无新权限或菜单。

Root负责本合同、shared validators/type枚举、严格schema/生成和最终集成／文档／Git。Runner代理负责私有source与CommandSpec pins、原writer/observer/generation/恢复/launch薄接线及Cursor修复。Control代理负责preparationReceiptMatches与原权限／同Tx／HTTP针对测试。UI代理负责原panel/reader/cap model/RemoteScan/双语/模型共用predicate和定向测试。新增共享文件先报Root。

必要验证聚焦：六类资格和闭形配置、准确tuple/modelcap、source/native/Node依赖漂移、已接纳head恢复与旧ACK、原HTTP严格边界及库存同Tx。重用T224未变互斥／Scope授权证据，不重跑无关全套。选择代表CLI进行原HTTP/SQLite/Worker/版本进程→generation→paused实跑；仅owned fixture不冒称厂商。CN/EN代表模式与原Quick操作分开于真实执行记录。SDK完整资源、真实厂商／模型、多机Pool、其他数据库／系统／安装／规模／生产及全平台仍继续。
