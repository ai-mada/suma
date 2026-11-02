# 已验收普通候选的基线来源

版本1.0.0 · T-ACCEPTED-TASK-BASELINE-222，状态：实现；[合同](../../contracts/accepted-task-baseline.md)／[证据](../quality/accepted-task-baseline-verification.md)。

## 单一来源和当前权威

延续CodeBaselineBinding及原基线／执行表，增加source_task_id与source_task_version两个可比较标量。原source保存准确run/node/attempt/fence/worker、代码／证据／bundle摘要；accepted_basis_sha256由既有RepositoryIntegrationBasis绑定当前正式Review、需求依赖及所有独立测试证明；binding_sha256覆盖全部字段。

普通任务、合成计划、终端计划三个来源严格互斥。原POST仅新增source_task_id、expected_source_task_version、source_node_id，不接用户提供的完整Source或验收结论。HTTP按字段存在性判断来源，拒绝部分、null、混合和非整数版本；OpenAPI与类型从同一Go领域模型生成。

## 当前验收与执行

acceptedTaskCandidateBasisTx复用repositoryBasisReadTx：当前accepted task／verified run／Review及需求依赖→当前封存transfer→原candidateSourceTx→同SHA、完整来源tuple及不同Worker测试。再读取准确当前producer spec/result，只允许普通agent及完全一致的attempt/fence/Worker。options新增can_bind_baseline作为同事务只读资格；不误用合成can_create，也不改变合成至少两个来源的规则。

验收递归walk沿原深度／数量／循环保护；普通来源memo包含任务版本和完整Source，防止同任务多producer互借依据。绑定和目标版本在原事务中双CAS、保存活动与原回执；重放先在当前权限下核对原操作，不以新版本伪造同意。

派发／预览／开始／取bundle／正式验收复用原保护入口，来源每次重新验证。Runner仍使用同一typed Binding和原代码broker，在独立Git工作树核验bundle、锚点及准确提交；没有新的下载通道、租约、表、执行器或调度器。

## B、M、N与质量边界

B是管理员仓库锚点；M是明确采用的已验收候选；N是新任务产物。工作区从M启动，完整bundle仍保留B到M/N的祖先关系；N必须由不同Worker测试同一SHA并重新Review。来源退回阻止新动作，不抹除原完成／用量／停止事实，也不使失效来源恢复验收资格。

当前实现不等于真实厂商、跨库、多机和完整平台验收；实际Git、测试夹具Review与代表界面分别记录。
