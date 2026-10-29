# 开发工作台任务上下文与 AI SOP

状态：1.0.0，T-WORKBENCH-126。负责人：工作台、需求设计、质量与平台领域。关联需求9–12、21、23、25、26；[合同](../../contracts/workspace-task-context.md)、[完整交互规格](../design/reference-workbench.md)、[验证报告](../quality/workspace-task-context-verification.md)。

## 领域边界

原生IDE中的任务选择是阅读上下文，不能改变项目文件目录、工作树、执行者或任务责任人。目录绑定继续使用原工作区授权；派发、质量计划、独立测试、人工Review和发布继续由各自领域命令及门禁负责。一个任务状态、一次运行结果、计划用例结果和正式验收是四类事实，必须分别展示。

`WorkspaceTaskContext`在一个当前项目读取事务中组合原任务版本、需求版本、受保护设计采用关系、正式负责人、准确关联运行和有界交付引用，以及已发布测试计划用例摘要。使用原`taskDeliveryTx`和质量领域查询，禁止另做可以产生不同验收结果的规则副本。缺少证据不能推断成功；设计依据变化显示过期，权限失败返回权限错误，数据库失败不能降级成空数据。

选择器使用服务端字面标题搜索，每页20项，最大50项；只在打开选择器、明确搜索或翻页时读取。游标绑定租户、项目和搜索，使用更新时间／ID稳定排序。当前选择独立按准确任务ID读取，因此不要求它出现在当前页。刷新通过URL恢复任务；当前页面保留代码编辑器，不因选择任务而重新挂载编辑器或丢失草稿。

## 交互与维护

默认显示紧凑任务条；详情按需展开任务说明、验收条件、需求及设计。执行区使用真实状态与原证据入口，计划用例摘要明确不证明独立Worker测试。业务、执行、质量和人工验收不能合成一个“全部完成”的绿色标签。原任务、需求、运行和测试链接沿用未保存文件离开保护。

中文和英文由独立领域字典提供，复用当前语言注册与回退机制。不得在组件中比较翻译文本、翻译用户任务内容、把未知协议枚举伪装为已知本地化状态。390宽度使用单列与可换行按钮；折叠区使用原生可访问控件；状态、加载、失败、无权限和重试应可由键盘操作。

请求所有者由项目、会话／当前权限上下文及准确任务组成。切换、退出或权限失败立即清除旧上下文；取消请求并核对请求代次，迟到响应不能复活上一个任务或权限下的内容。搜索草稿与已提交搜索分开，不在每次按键时重复请求。无需定时轮询；用户刷新读取当前事实。

## AI开发操作规程

1. 确认项目、所选任务及实际目录。任务上下文不等于该目录已经绑定任务，不自行建立或替换工作树。
2. 阅读任务说明、验收条目、当前需求和已采用设计；发现设计过期时回到原设计流程，不自动覆盖设计或采用新版本。
3. 查看原执行和独立测试证据，确认其版本及候选来源。计划用例通过不授予验收权，旧执行成功不证明新任务版本完成。
4. 需要开发时通过原任务派发／专家计划提交，保留预算、模型、执行能力和人工门禁；工作台读取接口不能触发命令。
5. 开发完成后沿原独立测试、正式Review和问题回流链路推进。离开编辑器先处理当前文件草稿；不以切换任务绕过保护。
6. 修改该模块先冻结合同，再分别修改领域组合、生成合同和界面。仅运行受影响的权限／快照／请求竞态／交互检查；实际历史证据复读可复用已固定源码与制品，不重复模型调用或部署。

## English operating summary

The native IDE task selector provides a read context. It does not bind a directory, switch a worktree, dispatch a run, grant independent-test authority, or approve a delivery. The API composes existing task, requirement, adopted-design, exact-run, published-plan and formal-review facts in one currently authorized transaction. A stale design, unknown execution or missing evidence must remain explicit.

Search is submitted explicitly and paginated on the server. The exact selected task is fetched separately, so refresh and selection outside the current page work. Abort and generation checks discard obsolete responses; authorization failures clear previous content. Original evidence links and task changes use the editor's unsaved-work guard. Chinese and English labels share the existing locale system, while user content and raw evidence retain their original text.

Use the original dispatch, independent-test and formal-review workflows to change business state. Current scope does not establish full terminal, chat, office or reference-platform parity; their complete requirements remain in the [product parity matrix](../product/reference-platform-parity.md).
