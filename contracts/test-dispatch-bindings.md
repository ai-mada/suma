# 开发与独立测试的明确派发绑定

版本1.0.0；阶段T-TEST-DISPATCH-BINDINGS-216；负责人：控制面／Runner质量／工作台／主集成。完整31项／P0–P7继续；本合同不等同全部平台交付。

## 原计划内的明确选择

原TaskDispatchPreviewRequest增加可选test_bindings数组（最多64项；显式null拒绝），项为TaskDispatchTestSelection：node_id、tool_profile_id、expected_tool_version。只选择本次Agent关联的原静态test节点（candidate_from===agent_node_id）；拒重复、非test、其他来源及动态节点。省略保留原发布定义，不据此声称旧定义已固定独立测试工具或Worker。用户可以明确覆盖原测试command/绑定，但不改变节点身份、类型、依赖、条件、输入及发布图，不自动创建或发布流程。

TaskDispatchPlan可选test_bindings只记录本次明确覆盖的冻结TaskDispatchTestBinding：node_id、tool_profile_id、tool_profile_version、tool_profile_sha256、worker_id、command_id。计划保存精确SHA／版本／Worker／命令，GET不补读当前元数据冒充历史。原派生workflow和原发布flow保持各自身份。所有新请求与回执沿原操作key、版本、事务及原恢复机制。0新API路径、数据表或调度器。

每项在预览／计划当前验证／原子Start及最终运行route内复核原当前权限、准确profile版本／SHA／command、启用及新鲜库存、Worker身份与同一路由。只允许command/text-v1、可写且TestReportProtocol为原tr.Protocol（suma-junit-report-v1），不得兼任artifact_from。测试Worker必须非空且不同于所选开发Worker；Pool不同slot不能冒充不同Worker。只替换明确工具与command约束；其他额外能力不得借此绕过，最终仍原dag.Compile。配置资格不替代运行期同提交pin、报告与人工验收。

## 简洁交互与恢复

原派发开发工具后默认折叠“本次独立测试工具”，列关联节点与原发布绑定／未确认状态，明确选择覆盖。共享当前工具目录供候选展示，采用时以当前useApi准确读取profile、核项目／ID／版本及原profile格式；目录分页外所选项保留。不默认选首项、不默默替换。工具状态和原因显示真实观察，非执行健康保证。

选择归属当前账户／组织／API／项目／任务版本／流程版本／Agent／开发profile版本和SHA；切源清空，ABA与迟到读取不复活旧选择；原pending冻结原body和key，不重取版本生成另一写。只通过原正式预览提交明确test_bindings，冻结计划显示准确测试工具／Worker／SHA。显式覆盖存在或选择未确定时，原智能推荐不能静默创建忽略这些选择的计划；如推荐流程本阶段未扩展同合同，清楚禁用其新创建并指向正式预览，保留原已存在pending的核对／恢复与离开保护。中英文、键盘、44像素触达及窄屏。

## 代码验收独立性

现有validateCodeTestEvidence不仅核sameSHA／干净成功工作树，还必须对每个实际producer（agent/command）的当前候选找到准确配对的成功test：双方节点ID、Worker非空，Worker不同，当前attempt／fence正数，test来源candidate_from／candidate_attempt／candidate_fence／candidate_evidence_sha256精确等于该producer，测试base=head=producer head。两个节点的attempt数都1合法，数值不同不是独立性条件。相同SHA的另一producer或旧attempt不得借用该测试。

提取小纯来源匹配谓词由原严格DBpair与纯Run gate复用；纯Run没有Config及数据库pin，不替代原DAG Complete候选核对、原严格DBpair、报告和Review事务。code-bearing缺Worker／来源证据明确拒验收，不伪造补全；test-only非代码语义保留。旧测试夹具须真实使用第二Worker和原Candidate*，不填虚构成功证据以绕门禁。

## 本阶段验收

必要的服务层／HTTP与类型合同检查：非关联／重名／过期／同Worker／跨项目／路由／撤权／重复操作、原发布图不变、计划快照及Start复核；纯gate准确pair及正式Review同Worker拒绝。原UI只验受影响选择／pending／来源和冻结事实，中英文代表状态。真实本机可复用原HTTP/SQLite与两个Runner，以自有确定性Suma脚本真实Git提交和测试/JUnit贯通候选→另Worker同提交测试；明确不是厂商模型或自主推理，不重放无关SDK全套。所有正式人工门禁保持原权威，完整31/P0–P7及真实厂商／多机／数据库／安装／规模／生产继续。
