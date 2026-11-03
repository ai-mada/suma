# 协作入口的独立测试绑定

T-COLLABORATIVE-TEST-BINDINGS-223，速码1.0.0。基线d1effe53157427e6c01f981a87fe796776e770ed；完整31项／P0–P7及58limited／34pending／92总账保持active，T222为有精确领域负例和正常dev推送的progress。扩展原[独测合同](test-dispatch-bindings.md)，不重建调度、图、质量或人工验收权威。

## 同一派发合同与三个入口

- TaskDiscussionDevelopmentCreateRequest增加可选TestBindings []TaskDispatchTestSelection，json:test_bindings,omitempty、wire:nonnull；明确传到原TaskDispatchPreviewRequest。原reply/task/workflow/model/预算依据与body/key恢复保持，正式Plan.TestBindings仍冻结原六字段，读投影不补当前工具元数据。
- DispatchRecommendationAdoptRequest顶层增加同一可选slice；推荐模型输出Choice继续原闭形五字段，不添加test_bindings或赋予模型测试授权。Recommendation响应／持久记录增加AdoptedTestBindings []TaskDispatchTestBinding，json:adopted_test_bindings,omitempty，保存采用时正式Plan.TestBindings六字段；原推荐provenance逐项核对该快照与原Plan，不以今天目录补历史。分析Request／SourceSHA／ChoiceSHA语义不改；显式测试选择只用于明确采用。
- ExpertTeamPlanItemRequest增加TestBindings *[]TaskDispatchTestSelection，json:test_bindings,omitempty、wire:nonnull。create nil使用发布定义；未开始revision nil保留原已固定测试选择，非nil空数组[]明确恢复发布定义，非nil非空明确替换。pointer让omitempty仍区分nil与[]，原操作摘要／同key不同body冲突必须验证。null一律在HTTP拒绝，不当作省略。已开始项原规则不允许修订。

团队保留只在原task与相同role/workflow/version/agent/developer profile身份下生效；来源上下文变化且旧绑定非空时，要求显式选择或明确恢复默认，不跨上下文静默继承。任务版本／代码基线更新可随明确revision重新审阅并保留同一配置，仍重验当前准确工具和路线。read/source重建从原Plan.TestBindings恢复三字段选择，再用原派发器核验；旧快照六字段与新核验一致，失效只阻止新动作／显示原因，不改原history。GET校验补原checkTaskDispatchTestBindingsRouteTx，现有can_start与记录事实分开。

三入口复用原freezeTaskDispatchTestBindingsTx、TaskDispatchPlan当前校验和原子Start／route；max64、准确静态candidate_from关联、唯一node、固定JUnit协议、非空不同Worker、Pool同Worker不同slot不算独立、当前权限与库存及原capability都保留。0新API路径、表、队列、dispatcher、生产依赖或发布图写入。

## 闭形与授权

Root复用validateTaskDispatchTestSelectionFields于讨论raw map（保持64KiB、重复键／Unicode／UseNumber）、推荐adopt顶层optional key和team create/revise的每个item。拒显式null、别名、未知／重复字段、非整数版本、超过64项和错误items形状。直接Go请求仍由原builder最终校验，HTTP presence规则不被typed nil擦除。

原命令当前权限、源版本、幂等key和预期版本保持。未知原写入继续核对同route/body/key，源目录或当前资格变化不得删除原pending；新操作不得复用旧确认。推荐实际采用前按真正选中的developer Worker复验独测，不把手工表单的独立性判定直接套给另一推荐。

## 简洁交互

复用TaskDispatchTestBindings及原ToolFacts，主操作不堆叠配置。讨论按reply/task/flow/agent/developer的准确上下文隔离；专家团队逐项按需展开，原冻结六字段可只读查看，保留／调整／恢复发布定义必须明确。共享一个workflow与工具目录，采用仍exactGET原profile/version/SHA，目录外已选项不丢失；已开始行只读。

推荐允许已完成选择的明确采用，仅限匹配当前测试选择所属flow/version/agent的行；不匹配行清楚禁用并引导原正式预览重配，不静默丢选。分析不新增测试授权。原creation/pending恢复保护按准确来源拆分，不能先删原guard再伪称支持。

中英文、44px、窄屏、键盘和单调epoch/ABA／迟到响应保持。所有冻结事实来自原保存Plan，和实时工具资格／执行／测试报告／人工Review分别显示。

## 所有权与必要验证

Root owns本合同、三个DTO/Recommendation快照字段、三个HTTP ingress、共享冻结事实导出、OpenAPI生成及生成文件、文档／状态／整合／Git。后端代理 owns三领域的原builder传播、持久／provenance一致性、team source/revise/read校验及focused tests。团队UI代理 owns TeamTaskDispatchDialog/ExpertTeamPlanPanel/轻聚合ExpertTestBindings及模型和tests；不需要给共享picker注入假ToolProfile，可读原冻结事实、显式模式保留或调整。讨论／推荐UI代理 owns DiscussionDevelopmentPanel/model、DispatchRecommendations/model及TaskDispatch薄props/guards接线和tests；共享选择器API不变，若确需扩展先报Root冻结，不互改其他代理文件。

验证聚焦三入口冻结传播、null/整数/闭形、nil/[]原回执区别、保留／清空／跨上下文与started保护、真正developer同Worker拒、准确route/漂移读不改历史、pending原body/key、UI源ABA。每域少量新top加直接旧回归，Root统一Go/类型/语言/合同；只续失败，不跑无关SDK/全Go/完整旧旅程。

实际用一个隔离DB／三task／共享workflow：讨论与推荐只预览采用并读六字段；团队创建／修订／activate／tick后只执行一次真实开发+另Worker JUnit，复用现有绝对Node/元数据/SHA，禁复制85MB工具、重播T222基线Review链或付费模型。推荐需真实完成原只读协议，以自有确定性提供方明确记录，不直接伪造完成记录。成熟现有Playwright Core/Electron隔离验证一组入口状态，所有manual mutation有准确view头和具体领域错误；测试actor不称客户验收。源码/证据/D状态及系统tmp.cache编译缓存按原规则隔离，不改用户全局环境。

## 当前工具自动核对（最新请求）

ToolWorkspace复用原live开关传autoRefresh，仅当前选中工具可见时在报告到期且距上次读取至少15秒后重新GET。后台标签页等可见／focus再核对，失败／来源变化／撤权停止自动读，原显式恢复保留。暂停自动刷新不禁手工刷新；unknown写入／pending原保护仍挂起Doctor。不触发探测、启用、注册、修复或模型调用，无新接口、依赖或队列。
