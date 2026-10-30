# 正式云执行器的运行事实

T-WORKBUDDY-RUNNER-155，速码1.0.0。状态：合同冻结、实现与定向检查中，正式Runner／实际界面联合验收待完成。以[正式Runner合同](../../contracts/workbuddy-cloud-runner.md)和[152单次提交合同](../../contracts/workbuddy-cloud-execution.md)为准，不新增调度或供应方创建权威。

## 原运行内的下一步

用户在准确运行节点核对固定请求后明确委派。委派仍不启动或恢复运行；原Run启动／恢复、正式claim、Worker执行和回执分别保存真实事实。连接仍为connected不表示当前委派、owner、grant、机器或预算资格有效。

原节点云面板默认只显示一个主状态、当前下一步和折叠详情。详情保存原请求、摘要、委派期限、一次额度、准确task_id、供应方状态、观察时间、平台原因及原attempt／worker。供应方状态不着平台成功色，用量和费用继续显示未报告，不填0。

| 当前事实 | 主状态与下一步 |
| --- | --- |
| 未委派 | 匹配本人固定连接、明确期限和确认后委派 |
| 已委派但未领取 | 等待执行资格；查看原调度条件，不自动启动Run |
| 原Run已暂停 | 使用原运行控制恢复；云卡片不另造Resume命令 |
| 正式attempt已领取，尚无中央提交 | 已领取，等待提交事实；不能声称供应方正在创建 |
| prepared／unknown | 原创建结果待核对；读取已保存记录，不重发Create |
| task_known | 保留准确ID；本人可明确查询原任务 |
| cloud.delivery_required | 等待可信平台交付证据；供应方completed不解锁下游 |
| cloud.runner_disconnected | 原执行器失联；恢复正式Runner的原检查点，不建新attempt／任务 |
| cloud.stop_unconfirmed | 远端停止未确认；取消运行不证明云作用已停止 |
| cloud.not_created且无中央提交 | 可信resolver确认未创建云任务；查看原执行依据，满足原幂等与预算策略才允许明确重试 |

cloud.remote_unknown、waiting_input、interrupted、authorization_changed和observation_unconfirmed分别解释供应方结果未知、等待输入、原执行中断、资格变化及原观察未确认。本阶段不通过等待输入状态开放追加prompt或权限决定。未识别的调度阻塞键保留为详情中的原安全原因，不猜为健康或可派发。

## 事实来源与当前范围

[Workflow](../../apps/web/src/Workflow.tsx)只对准确不可变定义中的external_workflow／workbuddy-cloud／workbuddy-cloud/v1节点挂载工作区，向[Workspace](../../apps/web/src/WorkBuddyCloudTaskWorkspace.tsx)透传父级真实runVersion、runState、nodeState、reason、attempt、fence和worker。当前云读取与父快照版本一致时才组合该执行投影；版本不同保留可核实中央提交，不把旧父状态装入新记录。

[模型投影](../../apps/web/src/workbuddy-cloud-task-model.ts)只解释已有事实，没有新持久状态机。[Panel](../../apps/web/src/WorkBuddyCloudTaskPanel.tsx)复用React、Lucide、原生details、统一中英词条和既有容器样式。继承原运行共享刷新及合并读取，不为卡片增加计时轮询。

NodeSchedulingFacts仍负责当前准入与准确attempt历史；ToolExecutionFacts只消费真实工具证据。没有Tool／Workspace／Artifact／Test证据就不伪造终态或候选。云等待仍是既有DAG waiting，在本面板明确表达等待云端事实，避免误写成等待审批。

精确云节点隐藏手填“确认实际结果”入口；普通云失败也不提供通用重试。只有failed、cloud.not_created且原节点Idempotent允许时，才显示原重试入口，预算与次数等仍由原服务端策略重验。服务端同时拒绝用通用success或换attempt绕过未知；界面隐藏不是安全边界。

## 两种原操作不能混用

浏览器的delegate／revoke／observe使用原useWriteAction保存原body／key，未知结果锁住新写入，明确核对原操作。重新读取平台记录只观察，不确认未知ACK，也不调用供应方。

机器Create／Lookup／Refresh由正式Worker及其私有journal保存原claim、Assignment、binding、body和operation。浏览器既不能构造机器Create，也不能用新operation代替原恢复。失联提示留在原节点，准确attempt历史提供定位；恢复正式执行器后只能继续同一检查点。

task_known之后可明确观察准确原ID。供应方创建未知且没有ID时，不能按名称查找、替换连接或重复创建。撤销与取消都不宣称远端停止；enforce硬预算仍因云端不支持而阻止新外发，不从连接状态推断例外。

## 中英、窄屏与验证

主状态与风险提示使用成对中英词条；准确ID和供应方状态保持原文。详情默认收起，可用Tab、Enter和Space操作；原生焦点及至少44px按钮沿152样式，窄侧栏与390px单列呈现，长ID和原因可换行。确认范围改变撤掉确认项，当前身份／主来源拒绝移除受保护内容。

新增定向回归覆盖领取后未提交、prepared不冒充创建、已知task不推断成功、等待交付／失联／取消分离、一个主操作和折叠详情、机器未知与浏览器恢复隔离，以及中文可访问详情。本轮不重复152／154既有整组，也不先运行Vite构建。

后续实际旅程必须使用正式Worker.Run及正常HTTP领取，不用152专用夹具直接Create代替；真实一次创建、原响应丢失后同body恢复、供应方unknown不重建、completed不绿、下游未启动与取消未假停各自留证。联合构建及CN1366／EN390图片与键盘验收由主集成协调后记录；目前不预填通过。完整58项限定实现／34项待交付／92项总数不变，ACP、可信候选、独立测试、真实厂商、多机、其他数据库及生产继续原计划。
