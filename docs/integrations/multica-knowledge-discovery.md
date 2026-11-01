# Multica 技能发现参考与速码实现对应

状态：本阶段参考与实现边界；负责人：平台集成；T-KNOWLEDGE-DISCOVERY-196。产品和接口以[速码需求](../product/knowledge-discovery.md)及[领域合同](../../contracts/knowledge-discovery.md)为准。

本轮核对本地 `D:\codes\github\multica` 提交 `5a2cc891ac338eed1919aa11a1eef5957cd82cec`，工作区干净，并读取[官方项目说明](https://github.com/multica-ai/multica)。当前README将解决问题沉淀为可复用技能、统一团队工作和运行交接作为产品能力。

| 参考源与技术思路 | 速码本阶段采用 | 实际边界 |
| --- | --- | --- |
| [skill.sql](https://github.com/multica-ai/multica/blob/5a2cc891ac338eed1919aa11a1eef5957cd82cec/server/pkg/db/queries/skill.sql)：技能列表只取摘要，正文及文件按需读取 | 发现结果只传准确引用、发布摘要和片段；原历史修订承载完整正文与来源 | 速码沿既有不可变发布与来源资格，保留草稿／发布分离 |
| 同文件：工作区SQL范围限制及批量加载避免热路径逐文件读取 | 当前组织／项目权限与有界候选读取；CLI、Agent和Web共享同一领域服务 | 项目授权、来源复核及派发门禁继续由速码原控制面负责 |
| [skills-page.tsx](https://github.com/multica-ai/multica/blob/5a2cc891ac338eed1919aa11a1eef5957cd82cec/packages/views/skills/components/skills-page.tsx)：集中列表、筛选、详情及当前权限动作 | 沿现有知识空间与任务选择区提供共享发现／预览，保留少量清晰入口 | 中英文、当前调用方隔离、准确版本选择与有界继续查找按速码合同实现 |

本阶段复用速码现有 Go／SQL／React／Radix／i18next、知识修订、任务上下文、CLI与应用工具能力，未增加运行依赖或复制上述Multica源码。参考仓库的[完整许可](https://github.com/multica-ai/multica/blob/5a2cc891ac338eed1919aa11a1eef5957cd82cec/LICENSE)同时包含Apache文本及附加条件；其商业、托管与品牌部分须按原条件处理，不能简化为仅Apache许可。

关键词发现、原文预览、明确任务采用、技能目录物化与模型实际使用分别记录证据。完整Multica能力映射、语义技能检索、分布式目录同步及全平台目标继续按[实施计划](../delivery/implementation-plan.md)推进。
