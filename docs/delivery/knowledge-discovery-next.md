# 知识领域后续交付

版本：1.0.0；状态：完整目标继续；更新：2026-11-02。T196交付已发布关键词发现，T197交付正式领域outbox、原始ACK、真实全文／向量融合召回、当前授权、准确码点定位、墓碑重启与明确预算重建。实现及限定证据见[报告](../quality/knowledge-projection-verification.md)，不能再将已实现语义投影列为尚无实现。

## 仍待完成

1. **存储与规模。** 实际OceanBase／PostgreSQL迁移事务、备份恢复、索引能力与容量验证；Lance每个目录单一拥有者不等于物理多机高可用。
2. **完整灾备。** 当前证据覆盖目录保留下重启及重建；目录整体丢失后，需正式快照／事件重灌、追平与完整校验，不将新空目录可启动当成恢复成功。
3. **商业计量。** 服务持久化重建预算已实现；租户归属的嵌入费用、并发模型配额及正式账单另行交付。
4. **外部专业记忆同步。** 定义连接授权、远端稳定身份、同步方向、版本冲突、删除传播和原操作恢复；单向索引不是双向同步。
5. **模型与质量。** 准确任务引用不证明模型已应用记忆，真实模型、后续代码、同提交独立测试及人工验收分别取证；不重复未变关键词或技能物化检查。
6. **召回覆盖。** 当前按有界provider候选再过滤知识种类，保留candidate范围事实；完整语料召回、离线协作、跨平台安装及生产规模继续验收。

原31项／P0–P7及58限定／34待办／92基线保持，见[实施计划](implementation-plan.md)。优先恢复工具接入易用性：准确节点高级配置、可操作Doctor下一步与公开草稿保留，不重复建设已有的登记、重检和未知核对链路。

*English: T196 keyword discovery and T197 typed publication projection, authorized native hybrid retrieval, exact locators and bounded generation rebuild are scoped implementations. External bidirectional synchronization, complete lost-directory recovery, physical multi-host ownership, tenant-attributed embedding billing and verified model use remain separate deliverables.*
