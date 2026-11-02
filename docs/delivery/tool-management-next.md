# 工具管理后续纵向切片

版本1.0.0；完整31项／P0–P7目标进行中。T193–T195已实现接入、准确重检与未知核对；T198补齐准确节点高级配置及具体原因入口；T199实现Doctor统一维护建议、原进度与跨配置准确定位；T200已实现配置换新的正式原子交接；T201补齐Claude SDK原生发现及桌面快捷连接；T202完成发现区渐进展示与键盘交互；T203贯通准确准备诊断、协议／节点交接及Codex原生配置生成。参见[维护建议验证](../quality/tool-doctor-maintenance-verification.md)与[换新合同](../../contracts/tool-profile-replacement.md)，不能把这些已有能力重新列为缺失CRUD。

## T200已实现：配置换新正式交接

沿原CreateToolProfile／matches与CommandToolProfile领域实现明确确认的原子换新：核对旧档案expected_version、当前项目与目标节点资格及新鲜完整配置；用户明确选择准确ID／版本的已有目标，或新建paused／v1目标。pause_original必须显式提供，可在同一事务中暂停原配置；复用保留目标原有状态，不自动选择匹配项或启用新配置。

正式关系保存准确旧／新档案快照、created／reused结果、原暂停意图、创建人及时间；关系快照总大小受2 MiB上限约束。只读outgoing／incoming历史按当前项目授权分页读取，保留不可变交接事实，不重新观察或冒称当前健康。关系不构成新的配置权威。

未知写入沿原账户、组织、项目、路由、正文和idempotency_key恢复原回执；不换新操作标识、不刷新版本后重复交接。CLI提供明确确认的tools replace与只读tools replacements，界面沿原写入恢复机制处理；不自动重试、改写历史hash／原计划、迁移任务或声称外部进程已经停止。原启用、暂停、撤销命令独立保留，revoke保留历史，不新增物理DELETE。

## T201已实现：Claude SDK简洁接入

第九项被动发现复用有界版本探测与原进程停止证明。用户只填写固定模型和凭据环境变量名称，桌面宿主核对Node／CLI／SDK资源和完整摘要；连接回显原名称而不回显值，刷新或回执丢失后可核对同一配置。等待本次启动后的准确库存，沿原matches／Create登记paused／v1，再明确启用。原配置调整继续T200交接；浏览器与远程节点的原手工入口保持可用。参见[需求](../product/claude-sdk-quick-connect.md)与[限定验证](../quality/claude-sdk-quick-connect-verification.md)。

## T202已实现：发现列表渐进展示

已发现、检测异常和当前选择保留主列表，其余未发现协议进入原生折叠；原行、依据、已有连接和配置入口完整保留。新选择跨组时保留该明确动作焦点，空主列表提供原节点入口，安装观察数字与真实可派发资格分开。桌面表单令牌覆盖引起的触达问题已按组件范围修正。[需求](../product/local-tools-discovery-experience.md)与[限定验证](../quality/local-tools-discovery-experience-verification.md)记录10项定向和代表界面边界。

## T203已实现：准确准备诊断和原生配置生成

Claude SDK／OpenCode SDK受信检查输出有限准备原因，当前权限／来源及原未知回执优先；凭据问题准确定位名称字段。原Quick、Workspace和远程指南保留九种协议的当前选择与准确节点，单调意图序号防止清空后旧请求复活。同一节点的新工具不被旧配置覆盖。Codex App Server专门表单与原Runner新文件模式已贯通，界面生成的七字段JSON经真实CLI形成固定原生协议配置。[需求](../product/tool-preparation-recovery.md)和[限定验证](../quality/tool-preparation-recovery-verification.md)区分实际配置、合成界面、厂商执行及完整验收。

## T204已实现：当前库存反馈与准确返回

原指南明确检查或限时等待当前项目／准确节点／准确协议库存，再返回原Quick重新核对，独立登记与启用保持。时间严格晚于读取和原观察两个边界，原ACK、回退、过期、取消和权限失效分别处理；未完整生成命令也可检查既有库存。[需求](../product/tool-report-feedback.md)及[限定验证](../quality/tool-report-feedback-verification.md)记录18个不同检查、真实HTTP／SQLite、两组代表界面与原失败。当前库存不是operation／文件因果证明。

## T205已实现：远程已安装工具检测

原持续Runner／Pool明确允许固定目录检测，空commands也可发现安装；原维护owner与Journal、当前授权、双向互斥及真实停止证明保持。仅维护模式不接业务，丢ACK恢复原body／key，来源变化和历史审计分开；准确工具／节点回原Guide。[需求](../product/remote-tool-installation-scan.md)与[限定验证](../quality/remote-tool-installation-scan-verification.md)记录真实受控native2树、0任务及双语视觉，不冒称实际供应方。

## T206已实现：简洁首屏与准确接入

首屏单一添加、配置列表前置、更多接入首次按需挂载并保留草稿，原云授权独立生命周期保持；准确Guide／节点／预设和弹窗焦点交接沿原合同。双语代表整页合成界面与定向检查见[交互设计](../design/tool-management-experience.md)和[限定验证](../quality/tool-management-experience-verification.md)。原三份布局增量已纳入本阶段；不冒称新厂商执行。

## T207已实现：OpenCode SDK新文件快捷准备

模型和公共连接优先、机器信息按需填写；固定九字段私有输入沿原静态资产核对及独占writer生成新配置，准确三字段草稿接回原Guide／库存反馈。真实生产页面命令经PowerShell／Go创建文件且第二次拒绝覆盖，详见[产品](../product/opencode-tool-preparation.md)及[限定验证](../quality/opencode-tool-preparation-verification.md)。实际版本／模型／完整正式登记启用仍按原链单独验收。

## T208已实现：Pool执行机器安全配对

原节点页主配对、系统凭据库与完整指纹核对、同事务原Pool身份登记及原操作恢复贯通，项目配对不扩权。明确Pool身份启动沿原配置和当前机器认证，原普通／Pool调度保持。实际Windows系统库／HTTP／SQLite／工作台及最终只读简洁提案见[产品](../product/pool-runner-pairing.md)与[限定验证](../quality/pool-runner-pairing-verification.md)。

## T209已实现：多项目私有配置准备

原准确节点行选择当前已加载的批准项目或手工范围，只填写机器文件引用；Shell闭形输入沿原配置校验和新文件防护，校验自动展开／定位字段。真实双项目生成／拒覆盖和双语界面见[产品](../product/pool-config-preparation.md)及[限定验证](../quality/pool-config-preparation-verification.md)。不自动补授权、启用工具或运行任务。

## 下一步：安全诊断与真实执行接续

继续本机准备的逐scope／字段安全原因与修复指引、生成配置沿原配对身份的实际启动／库存／派发和同提交质量接续、OpenCode真实环境及WorkBuddy可信代码交付；不重建已完成的配对／配置准备／检测／Doctor／换新。

继续把目标机器的原准备结果与具体资源检查接回当前项目／节点的可追溯事实，降低手工复制和重复配置；优先复用原配对、工具报告、Doctor、私有凭据及操作回执。准备建议、明确授权、机器实际执行、库存和派发资格分别保存；未知效果先核对。真实工具／模型、多机执行、分布式资源及完整部署验收按原能力矩阵推进，不将本阶段公开草稿和配置生成视为全部远程自动执行。

## 继续交付与验收边界

工具能力与模型、安装／探测、启用／派发、业务／质量仍分别取证。完整31项／P0–P7目标继续；真实厂商账号与模型、物理多机、OceanBase／PostgreSQL真实数据库矩阵、完整规模、跨平台及生产验收仍按原矩阵推进，T200–T203不代表全平台完成。成熟SDK、原权限与回执优先，不重复建设已交付入口。
