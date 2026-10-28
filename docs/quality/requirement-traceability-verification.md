# 需求验收追踪验证

状态：限定范围完成，全平台继续实施。日期：2026-10-28；负责人：主集成代理；任务：T-TRACE-113；产品：1.0.0。

[精简摘要](requirement-traceability-summary.json) · [读取合同](../../contracts/requirement-traceability.md) · [中文指南](../development/requirement-traceability.md)／[English](../en/requirement-traceability.md) · [两份规划采用](../integrations/shared-roadmap-adoption.md) · [当前台账](../delivery/implementation-plan.md)

## 交付行为

需求详情增加默认折叠的“验收追踪”，按需显示当前每条验收标准及其正式设计任务、采用后的真实任务、确切运行与测试节点、当前正式人工验收。任务及运行／节点／attempt通过现有导航下钻，不新建顶级页面、派发器、验收规则或平行业务状态。

新接口 `GET /api/v1/requirements/{id}/traceability?project_id=...` 使用当前 ProjectRead 授权与项目协调事务，仅允许浏览器／项目API凭据。标准身份限定为需求ID、正式版本与1起始索引，相同文字不会合并。映射来自不可变发布修订的Covers和受保护采用绑定；普通关联任务、同名或可编辑元数据不能构造覆盖。需求变化、设计退役或来源不可用时，仍保留当前标准原文，清空旧位置映射，不把旧运行成功归给新发布。

未采用、无运行、依据变化、来源缺失、SQL／上下文读取错误分别处理。正式验收沿用现有当前需求、设计、任务、依赖、补偿及精确运行证据核验；成功执行和测试节点成功都不自动变成人工验收。完整标准和发布任务各最多100项；节点与测试引用各最多20条，完整计数与列表完整性分开。没有逐用例模型或criterion级自动测试证据，不能称完整测试覆盖矩阵。

界面校验完整双向映射后开放引用，账号／组织／项目／需求切换隔离旧响应；401／403清除受保护事实并等待显式重试。独立审阅发现自动刷新父详情可能形成重复读取，已删除自动权限／版本通知。版本失配锁定事实动作，只有点击“刷新需求详情 / Refresh requirement”才请求父详情刷新。

## 来源与架构采用

第一份[分享规划](https://chatgpt.com/s/t_6ae19e1f3af481918d11132ccf5119fe)沿用已核验的64个可见编号及来源摘要；原文末尾在知识解释处中断。第二份[分享规划](https://chatgpt.com/s/t_6ae1a95216648191961a25087e713f3a)最初Markdown只有空白助手块，随后普通basic页面提取的原HTML包含正式fallbackMarkdown。仅用JSON解码恢复4537字符／10174字节正文，没有执行网页脚本或调用内部API。SHA256为 `038cb2784ece5c79a710b9c8eca7d083b4c63a9a5951f6163183d9fc2efbcb9e`。原文停止于参考平台表的不完整Octop行，未补造后续章节。

第26类统一智能输入／补全／配置／CLI驱动能力、S01–S12边界及八项验收要求已映射到当前领域，并补充[架构](../architecture/intelligent-operations.md)与[AI SOP](../delivery/ai-engineering-sop.md)。模型只提出结构化草稿，当前权限／CAS／原回执／实际后验核对及唯一Go调度器保留正式权威。`FND-INTENT-001`仍为待实现，不因设计文档完成而声称已交付通用智能操作或配置引擎。台账新增这一个待办，当前为58项有限实现、34项待办、92项总计，P0–P7及需求1–26保持完整范围。

## 定向检查

16个不同的受影响用例通过：平台4、合同1、后端词条1、Web模型4、Web组件6。Go初始4项明确因服务功能缺失失败；UI初始失败是新增模块尚未存在，属于缺功能证据，不当作既有行为回归。共享的运行摘要校验直接复用，未重复完整旧套件。权限／版本显式刷新修正后仅重跑2个受影响组件用例，不重复计入16项。

Go命令、退出码及输出摘要在精简摘要逐项登记，使用缓存Go1.26.10、离线依赖并串行执行。主检查为：

```text
go test ./internal/platform ./cmd/suma-contracts ./internal/localization -run TestRequirementTraceability -count=1 -json
pnpm --filter @suma/web test src/requirement-traceability-model.test.ts src/RequirementTraceabilityPanel.test.tsx
```

类型检查通过；4868个中英文键成对，项目自有生产硬编码0；合同生成和检查通过，489个模型、258条API路径。新读取边界及Schema的100／100上限另做定向核对，不运行无关全套、厂商或模型任务。原设计退役详情的一致性已追加断言：正式retire会清除发布指针，原生产helper保留，没有把未复现的推测改成新代码。

Web构建两次，分别49.07秒和52.66秒；第二次用于实质的显式刷新修正，最终构建成功。Control仅构建一次，Runner无构建、无运行。现有IDE分块体积警告保留。无新增依赖、业务表、兼容别名或旧数据回填。

## 独立真实HTTP核对

使用新SQLite目录、测试账号、独立Control和本轮Web构建，不修改用户运行目录或预览。实际发布两个任务分别覆盖两条标准，未采用时有两个提案、没有正式任务身份；正式预览与采用后获得两个受保护任务，均无run且未正式验收。随后显式修改需求为版本2和一条新标准，返回 `requirement_changed`、零旧映射；退役后返回 `retired`、保留当前一条标准、零映射。

无效查询在真实HTTP中分别返回确切中文和英文诊断，原业务正文保持原文。总共启动执行0、Review0、产品模型调用0、厂商调用0；没有以夹具声明替代真实智能编码或独立测试。本轮正式验收和旧运行失效分离由受影响领域用例验证，不重复授予实际验收。

本次自有Control已按stop.request正常关闭，holder退出0，私有结束记录确认child关闭，原PID不再存在。临时数据库、正文／原HTML、原始记录、构建与凭据继续保存在忽略的 `.cache/e2e/coverage-113/`；秘密没有进入公开摘要或Git。

## 浏览器缺口与交付边界

本轮Windows Computer Use在读取独立Chrome窗口时，因无法足够可靠地确定当前浏览器URL以执行安全策略而结束。随后停止所有界面输入，没有改用其他方式绕过。没有当前实际浏览器通过证据或截图，不把jsdom组件检查、HTTP或构建称为浏览器验收。临时空白Chrome窗口关闭未确认，用户原有标签页和活动预览未修改。

当前源码／合同清单共23个文件，摘要 `65eca1ff190718b9c2d609366a45a345bb2ca90c1a20091692653d77b6a1e8a0`，清单位于 `.cache/e2e/coverage-113/source-manifest.json`。Git仅包含必要源码、当前合同、设计指南、精简证据及状态；缓存、HTML、数据库、原始日志、凭据和二进制不提交。

完整TestPlan／TestCase、逐用例证据、应用版本集、智能输入／配置、实际厂商、多机、OceanBase／PostgreSQL集群、长时规模、跨平台安装与生产验收仍按原范围推进。该阶段不证明全平台功能完成。
