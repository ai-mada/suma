# 交互终端与讨论到自主开发：限定交付验证

状态：T-TERMINAL-129 限定实现与实际验证；产品 **1.0.0**；2026-10-29。负责人：控制面／DAG、Runner／智能体、工作台领域。关联需求9、12、17、22、23与原P0–P7／需求1–26。完整台账仍为 **58项有限实现、34项待办、92项总计**；本报告不表示平台整体或生产验收完成。

[精简证据摘要](interactive-development-summary.json)、[架构](../architecture/interactive-terminal.md)、[交互设计](../design/interactive-terminal.md)、[操作与AI SOP](../development/interactive-development.md)、[交接合同](../../contracts/task-discussion-development.md)、[公共API](../api/openapi.json)。

## 本轮实际交付

任务工作台增加按需展开的真实交互终端。明确选择已登记的专用Shell配置并授权后，沿原Go DAG、原Runner和独立attempt工作树运行；前端复用固定版本xterm，Windows复用go-pty／ConPTY及Job进程树管理。连接、输入接收、PTY送达、命令结果与实际停止分别表达。关闭页面不确认停止，恢复查看不另起Shell，未知输入不会自动重发。

已完成的任务讨论回复增加“从讨论转为开发”。用户编辑审阅说明和检查项，重新选择发布流程、可写原生工具、模型、参数及Token／费用预算，创建原TaskDispatchPlan预览，再单独明确启动保留运行。它复用当前模型网关、候选封存与传输、不同Worker独立测试、人工Review和问题回流，没有新增派发器或把助手文本直接喂给Shell。

共享合同生成681个公开模型、315条API路径；OpenAPI文件实际含682个Schema定义。公开终端入口为`/workspace/terminal-sessions`，Runner及Pool分别走原认证出站交换。模型参数沿当前128步／131072总输出能力，专门只读回复仍为1步／8192；终端不继承模型密钥或预算。

## 真实原生开发、独立测试与问题回流

`TestTaskDiscussionDevelopmentActualNativeAndIndependentQuality`在本机真实Go服务、SQLite、HTTP、两个独立仓库及真实Node原生SDK／MCP宿主上通过。使用自有确定性模型端点，**3次HTTP请求、2条模型attempt结算、30 Token、按运营方测试费率估算39微元CNY；商业模型调用0**。另2条普通命令用量记录不冒充模型账单。

1. 原基线运行生成真实失败JUnit；固定失败用例形成受保护缺陷，并通过唯一IssueIntake回流需求，重复操作恢复原需求。
2. 任务讨论保存原消息，原生只读回复完成；其确切助手出处形成可编辑开发计划，固定新的模型、参数、预算及执行依据。
3. 显式启动后，`native-developer129`在独立工作树通过宿主`workspace.write`修改`sum.cjs`并封存候选，真实进度经原执行事件回写。
4. 候选提交为`ed2f098210535c3a94fcf8dd7b260a75a527002c`；503字节Git bundle经实际传输导入另一仓库。`independent-verifier129`对相同SHA执行独立JUnit，原件摘要为`a8a63a0e7a996657c70208d547b1a591ea5422af94a85bcfdaae0a96a9b909b5`。
5. 独立测试完成前的正式验收被拒绝；完成后，由隔离夹具所有者账号显式调用原Review接口，任务accepted、正式质量passed。两个管理员工作目录保持原样。

这条新交接链路通过真实API启动，浏览器表单的预览／分开启动、错误恢复和刷新由受影响组件检查覆盖；**未宣称新开发计划由实际浏览器启动**。这是同一主机的独立仓库／执行节点，不是物理多机验收，也不是客户人员签收或商业模型效果证明。

原生SDK源码未变，复用T128已核验bundle：SHA256 `f61c8d1b0d7ec7e91fbdb8e7bbe87874ee0d09e65ee6b43724ba5cdf213579cb`；未重复构建SDK或调用商业模型。

## Windows交互终端与实际界面

底层`TestTerminalRuntimeActualInputReceiptResizeAndTreeStop`实测通过：中文输出、一次真实输入、窗口尺寸调整、Ctrl-C中断前台进程及后台后代最终停止。修复采用专用子进程bootstrap，在执行用户Shell前清理该子进程继承的忽略Ctrl-C状态；不修改常驻Runner的控制处理。Windows先暂停创建、纳入kill-on-close Job，再恢复执行。

实际浏览器旅程创建且只创建一个终端，原Worker领取、独立Git工作树启动真实ConPTY。通过原生键盘输入`echo terminal-ui-129>terminal-note.txt`，文件原件确实存在、内容正确，父仓库未变。中英文1366／390、调整尺寸、刷新恢复原会话和持久输出均实际观察；显式停止后持久记录确认进程树停止。终端不生成正式候选、独立测试或人工验收。

交换记录保留1712次请求、1次原操作重放，以及一次“输出已提交但HTTP确认丢失”恢复；39个输入回执、16个输出块均含重放，不可作为独立命令数或性能指标。运行使用HTTP轮询，没有WebSocket或吞吐验收结论。

### 原始夹具失败与只读复核

原`TestWorkspaceTerminalActualBrowserAndOriginalExchange`退出1，原始`result.public.json`保持failed。其最后断言直接比较Create返回记录与Get返回记录；Get会补派生展示字段`Name=Title`，Create原回执不含该字段。这不代表持久任务被改变，也不能把原退出码改写为通过。

复核以数据库内不可变的原Create操作回执为基准，**只读比较全部17个持久任务列，逐列完全相同**；原库和原结果摘要均保留。新增最小`TestWorkspaceTerminalTaskPersistenceAcrossCreateAndGet`通过，验证持久字段及该显示差异；实际终端夹具现比较同种读取和完整SQL快照。没有为此重复执行整条终端旅程。

最终前端又做一次`TestWorkspaceTerminalRetainedBrowserReadback`：先逐字节复制原已停止会话数据库，用当前服务和最终Web构建回看。只有普通测试账号登录可写副本认证状态，业务写入全部拒绝。中英文、原停止状态、输出、当前会话选项及390布局实际确认。**133张业务／执行表前后摘要完全相同，原库字节未变，拒绝写请求0，新增Worker／调度Tick／PTY／模型调用均0**。该独立回看检查退出0；不覆盖或抹除前述原退出1。

窄屏文档宽375、视口390，无整页横向溢出；终端内的横向浏览独立。最终当前构建截图保存在摘要列出的本机证据目录。原生xterm输入框为库控制的零宽辅助元素，使用真实键盘操作验证，没有修改DOM制造输入成功。

## 必要检查与资源控制

- 14项互不重复的顶层Go检查通过，逐项名称和原命令摘要见JSON。测试次数／重跑不累加为新检查。原终端浏览器夹具失败单列，不计入通过。
- Web行为检查11项：原回复4项、终端2项、模型参数3项、开发计划1项、终端库存导入1项。库存用例其余29项跳过不计通过。修改输入回执、启动回调后只复验受影响检查。
- 最终Web完整类型检查、隔离生产构建、生成合同一致性通过；双语目录6348项配对，生产源码硬编码0。模块新增终端56对、讨论开发36对，加载方式支持后续语言扩展。
- Go使用1.26.10、CGO关闭，固定缓存位于项目`.cache`；Web依赖使用现有锁文件。新增成熟依赖与准确MIT原件见[增量依赖清单](../delivery/interactive-terminal-dependencies.json)及[第三方说明](../delivery/third-party.md)。
- 没有运行无关全套、重建未变SDK、改变活动用户预览或安装版。前端行为修正后的多次构建各有独立输出目录与收据，最终使用`web-verified`，不把旧构建视为当前通过。

初次上游间接依赖缺少校验、初次Ctrl-C未真正中断、调度SQL遗漏新节点类型、夹具目录未处于各自仓库缓存、测试代码未用导入、组件测试类型错误等原失败收据均保留。对应修正后只运行受影响路径；详见摘要的失败与修正记录，不隐藏失败或据重复回放虚增执行数。

## 未完成边界与下一步

当前为Windows真实PTY、创建者固定epoch1、有限输出持久回放及只读文件观察。Unix尚无完整后代containment证据，Probe不宣称可用；多人控制接管、WebSocket、完整租约失联后的效果核对、终端修改转正式候选、LSP／DAP、原生三系统安装和SaaS执行隔离仍待实现。Pool接口源代码已覆盖，新的终端物理多主机旅程未验。

原始终端输入正文仅保存在需要投递的受保护存储，回执后清理；日志只记录必要身份和摘要。Git只收源码、锁文件、合同、设计、许可原件及本报告／精简JSON；数据库、输入／输出日志、截图、受控账号、bundle、程序和缓存留在忽略目录。保留其他并发工作的改动，阶段提交不表示整个工作区干净。

下一阶段优先补全失联效果核对和明确授权的代码候选交接，再推进Unix containment及三系统真实安装。完整原目标、OceanBase实集群、规模、商业供应商、生产审批和发布验收继续按台账实施。
