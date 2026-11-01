# 已发布知识发现限定验证

状态：scoped_verified；负责人：平台集成；suma1.0.0；T-KNOWLEDGE-DISCOVERY-196；2026-11-02。完整平台目标继续。

[需求](../product/knowledge-discovery.md) · [合同](../../contracts/knowledge-discovery.md) · [架构与AI SOP](../architecture/knowledge-discovery.md) · [交互](../design/knowledge-discovery.md) · [English](../en/knowledge-discovery.md) · [参考对应](../integrations/multica-knowledge-discovery.md) · [证据与原件SHA摘要](knowledge-discovery-summary.json)。

## 本阶段实现

- 已发布memory／SPEC／skill独立关键词发现；标题、说明与正文来自准确发布，草稿目录保持原语义。
- 当前项目、发布SHA及来源资格复核；最多200候选／20命中／48KiB响应，有界空页与完整到尾分别表达。
- 资料、任务与讨论共用准确预览及明确选择；已选引用独立于搜索页，原任务预览／Start再次核验。
- 原CLI knowledge search和只读Agent／MCP工具复用同一发现领域；准确引用、双语表格和原权限错误保持。
- 修复HTTP格式错误遮蔽当前项目撤权、规划白名单漏新只读工具及界面权限恢复依据不完整的问题。

## 必要验证

27个不同顶层定向检查，18新增、9直接受影响；Go子用例6。控制面6、CLI／Agent消费者4、Root合同2、实际旅程1、Web14。初始控制面命令中有两个独立top已pass，另一个fixture错误期望400；如实保留该command exit1，并复用未变相关输入的通过观察，最终只复验受影响组。该记录不代表全包或全套通过。

本机真实HTTP listener与磁盘SQLite两次各19请求：三类发布→新草稿隔离→准确引用分页→原历史预览→明确引用进入原任务上下文→重新发布后旧Start409且无新业务效果，原历史保留。首实际旅程通过后，审阅发现HTTP筛选先于当前项目授权；真实撤权／非法参数RED后修为同事务先授权再解析，独立final实际旅程再次通过。首／final源与SQLite原件均保留。工具可用是明确的自有元数据夹具，0厂商／模型／原生工具／Runner调用或客户验收，不重复既有开发质量全链。

首次完整Web类型检查12条集成诊断，修正后6个直接受影响源语法／语义0；原完整检查exit2如实保留，未重完整类型检查；中英文9178键配对、生产硬编码0；一次隔离组件构建，2组代表交互及2张图已视检。界面合成API与真实后端证据独立。新HTTP路径1、Schema3、只读应用／MCP工具1、索引1、业务表0。类型首代与工具目录落盘后必要第二代均成功；首次一致性检查只发现新增工具目录漂移，最终检查通过，原失败保留。

Agent原规划top证明新发现工具会被原四项白名单过滤，精确增一名后通过。CLI严格读取复用原required presence，文本UTF8边界、source_count0–8、双expected-view及无GET body有定向证明。界面权限恢复、准确引用、分页、迟返回及预览当前资格由原消费者与共享组件覆盖，不以目录或成功读取推导模型已采用。

## 范围与后续

- 真实实际旅程为本机HTTP／SQLite发布到原任务预览与变更后启动门禁；工具可用状态是既有自有元数据夹具，厂商、模型、Runner和原生工具调用0。
- 界面为真实产品组件与隔离合成API数据的代表交互；完整跨平台、物理多机、数据库矩阵、规模与生产验收分别推进。
- 关键词发现、准确预览、明确采用、技能目录物化及模型实际使用是独立事实；LanceDB／OceanBase语义投影、事务outbox及外部记忆同步尚需后续实现。
- 完整原31项／P0–P7／58limited／34pending／92基线保持active；当前计划checkbox数不作为缩小目标的依据。

下一完整纵向交付见[知识语义投影与外部同步](../delivery/knowledge-discovery-next.md)，继续沿原领域、事务outbox和发布／退役状态实现。
