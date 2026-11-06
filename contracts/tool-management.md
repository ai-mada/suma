# 工具管理：快速接入与可解释维护

T-TOOL-MANAGEMENT-193；速码1.0.0。原31／P0–P7／58限定／34待交付／92完整目标继续；接续已提交T192，优先落实用户最新的简单、快速、自动检测、配置调整与移除要求。

## 一个协调入口

工作台保留一个“连接工具”主入口。自动检测、选择配置、原节点上报／暂停登记及明确启用共用原身份、公开草稿和当前scope，不再从Doctor跳转空白表单；高级配置收起，返回时不重复填写。已有配置优先查原精确matches并提供“查看已有接入”，不重复登记。

公开接入草稿仅保存工具方向／adapter／protocol、目标worker、命令ID、名称及已声明的非秘密字段，按actor／组织／项目／caller绑定并通过nonce避免迟到覆盖。不得把凭据值、私有路径／argv或旧库存保存成“已检测”。当前目标缺少fresh上报时明确指向该目标，不回退到另一worker。

本机扫描、读取节点上报、配置可解析、可派发、模型健康及执行结果分别表达。SDK不因原CLI扫描或generic桌面连接而就绪；浏览器不能探测未授权本地文件。原本机扫描／已授权节点status-rescan、原describe/report及控制面读取按实际能力接续，不制造扫描成功或自动安装。

## 当前列表接口

`GET /tool-profiles?project_id=…&state=…&query=…&worker_id=…&limit=…&cursor=…` 支持状态、文本和节点联合筛选。`state` 为 active／enabled／paused／revoked／all，默认 active（enabled＋paused）。`query` 为可选、最多 120 UTF-8 字节的公开字段检索，范围为名称、命令、节点、适配器和协议。

可选 `worker_id` 使用与固定工具档案相同的节点 ID 格式，按原始大小写精确匹配。显式空值、重复参数和无效 ID 会被拒绝。过滤范围为当前项目的已登记档案，未知节点返回空列表，撤销历史由 `state` 控制。原授权、排序及游标机制继续生效，游标还绑定精确节点条件。省略 `worker_id` 时沿用原无节点游标。

界面默认“当前接入”，按需查看“已移除记录”。搜索、状态和节点变化清空旧分页，当前项目或身份变化隔离迟到结果。CLI 使用 `tools list --worker ID` 调用同一接口，保留结构化输出及原权限。

## 配置与生命周期

原POST创建、精确matches、GET详情／diagnostics以及enable／pause／revoke为唯一权威。调整运行配置沿原目标本机准备→真实describe/report→精确复用或新paused档案→明确启用；不原位编辑历史摘要或隐式改变已发布计划。

“停用”使用原pause；“移除接入”使用原永久revoke，从当前列表撤下，历史运行及证据保留。不得假装存在物理DELETE或配置PATCH。原撤销并不保证已有外部进程停止；必要时给出原运行入口及真实影响说明，不自动重发停止、清空Journal或掩盖未知效果。SDK及其他配置变更仍受当前角色、CAS、操作ID、库存与策略约束。

## 检测与修复

诊断保留原授权、worker、库存和profile四个前提，按准确目标给出一个最合适的下一步。刷新读取不能标成原生扫描。修复以当前已授权原操作和明确确认驱动：重读／本机重扫／打开精确节点配置／打开已有profile复核启用，失败保留草稿。未知外部效果先核对原操作，不能自动创建新请求或改模型、密钥、权限、分支。

原diagnostics新增可选非空impact，由当前有权限项目的活动运行索引及同一读取事务生成，不查询所有历史任务。它是说明，不是执行或撤销授权。

ToolProfileImpact字段：scope固定active_project_runs、observed_at、coverage（complete／partial／unknown）、runs_examined、nodes_examined、observed_reference_count、items、items_truncated、reason（空／run_limit／node_limit／source_unavailable）。最多检查200个活动run／2000个节点，按原dag.ToolBinding匹配准确profile ID和SHA，仅统计非终态node；items最多5项，字段为run_id、node_id、run_state、node_state、worker_id、attempt、fence。

complete只覆盖当前项目活动run，不包含草稿、未启动计划或历史未知外部进程；超界partial，实际无法解析的来源unknown。observed计数不是总量，未覆盖不能显示为0；授权或数据库错误仍返回原错误，不伪装诊断成功。摘要默认一句话，必要运行入口与覆盖详情展开查看。

当前真实活动run_state闭集为running／paused／cancelling；可列出的非终态node_state为pending／ready／running／waiting／reconciling。节点尚未执行时attempt／fence可为0，worker_id可为空；不可因此伪造已领取或执行者。终态节点只计入检查数量，不列作活动引用。

## 交互与交付

常用动作集中在列表／详情操作菜单，主状态一句话概括，送达／hash／原生协议等依据默认折叠；必要解释贴近实际决定。中英文、390px、键盘、空态、加载、失联、冲突和恢复沿原组件。保持已有页面的位置、搜索和选中目标，不堆叠第二工具中心或重复大卡。

Root负责公共合同、Schema、文档、集成及Git；Control负责原列表／诊断与当前授权；CLI负责原管理命令；工作台负责唯一协调入口与公开草稿。只验证新增及直接受影响边界，代表性界面与一条实际HTTP旅程；T192 SDK、原正式质量链和已绿历史不重复。每阶段源、设计和紧凑证据正常提交dev，缓存／凭据／私有配置不提交。

## T-TOOL-EXPERIENCE-198：准确配置目标

Quick当前选择的工具及worker是高级引导的目标；显式切换以最新选择为准，不能沿最初preferredWorker或项目最近配对节点覆盖。选择与公开配置请求同时绑定当前actor、组织、项目、API caller及代次；变更后旧目标、旧请求与迟到回调失效。

只有批准配对记录的worker与准确目标相同时才带入配对身份；不同节点保留准确worker，使用既有手工identity入口，不能把另一节点的pairing、control URL或身份锁入命令。配置生成仍是公开草稿，不派发、不安装、不启用。

不可用配置显示原库存合同中有界、非空的公开原因，按普通文本渲染；节点原文与中英文界面提示分开，不能伪造不存在的诊断本地化身份。配置入口携带当前工具和worker。原因文本只能展示，不能转为可执行命令或自动修复授权。原登记、启用、重检、核对、暂停、撤销合同均保持；移除保留历史与原运行证据。
