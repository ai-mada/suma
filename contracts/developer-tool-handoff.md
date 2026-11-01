# 开发工具交接与远端接入

T-DEVELOPER-TOOL-HANDOFF-182；suma1.0.0；原31项／P0–P7完整范围保持。两条独立入口汇入原工具档案、唯一Go派发与质量链，不增加身份源、调度器或历史兼容别名。

## A. 讨论内容交给正式开发工具

原TaskDiscussionDevelopmentCreateRequest增加可选`codex_execution_selection`，`model_execution_selection`改为可选。双选明确拒绝；不是强制二选一：原dispatcher决定原生模型、Codex目录选择及固定配置各自的必需／禁止规则。请求不开放任意knowledge；依据仍来自原回复／消息的精确来源、版本、摘要和知识引用。

去掉讨论交接独有的native-only限制，完整复用`buildTaskDispatchSnapshotTx`及普通派发目前支持的结构化可写开发入口：suma、codex、harness、codebuddy、固定Qoder ACP、Claude、OpenCode SDK、Octop。Cursor等仅检测入口不因此获得执行能力。实际支持仍由原工作流编译器与不可变Profile决定，不靠前端名称或新白名单授予。

Codex采用原真实目录选择、固定绑定、配置摘要、模型／参数与技能协议。其用量只观察，enforce策略继续由原门禁明确拒绝，不宣称厂商硬预算上限。固定模型工具不伪造平台或Codex选择。原审核、工作树、候选、同提交独立测试、正式Review和后续基线交接不改。

来源知识必须在模型／预算、上下文和execution_skills固定之前进入原构建链，并按原当前授权与准确pins规则复核；不能先冻结计划再追加技能而漏绑定。消费者若复用普通Codex回执校验，只能把已验证的source.message与context一致pins投影给该校验，不接受客户端任意knowledge。原pending创建／启动优先，完整body/key恢复，不以另选工具或换key替代。

前端复用现有工具／模型选择组件，显示当前确实支持的开发Profile，固定配置不显示错误模型字段。版本／来源／计划／目录变化保留草稿并重新核对；开始仍是原明确操作。中文英文完整，不触碰受保护TaskConversationKnowledge两源。

## B. Runner自动准备与单次上报

新增`suma-runner --prepare-tools PATH [--local-tool ID]`，复用原八项DiscoverInstalledTools固定探测策略，只采用实际带有Command、Installed且LaunchSupported的结果生成原DecodeCommands配置。配置型Codex App Server、Qoder、OpenCode及Cursor不得因已安装而生成伪可运行配置；显示原配置必需／不支持等事实。

文件必须新建，父目录须已存在，禁止覆盖或跟随链接到未知目标；权限与原本机配置规则一致，不写平台或厂商秘密。不产生空配置成功：没有可生成项不创建文件，返回明确未准备结果。stdout采用`suma-runner-tools-preparation-v1`，包含prepared、file、command_count及remaining中的adapter_id／reason_code。生成不是库存新鲜、模型登录或可派发证明。该模式不读平台执行凭据、不建立控制连接或领取任务。空`--prepare-tools=`和显式`--report-tools=false`均在执行入口前拒绝，不能误入普通Runner。

新增`suma-runner --report-tools`，复用原--control／--project／--worker／--commands／--state、已登记SUMA_RUNNER_TOKEN、journal锁和完整inventory恢复／上报／ACK逻辑，完成后退出，绝不进入Worker.Run／Claim／Heartbeat任务。`--allow-exec`只在该模式授权原固定程序／协议／工作区探测；未给时只能上报真实disabled事实，不伪健康。四项自动准备的可写命令还需要真实--repository、完整--base-commit和准确--workspace；界面以已填仓库根作为工作区，不另添字段。stdout采用`suma-runner-tool-report-v1`，保留project_id、worker_id、operation_id、observed_at、recovered及原items，不携带执行凭据。

存在未确认inventory时先恢复同operation/body并返回原ACK，不在同一次单步操作另发新清单；先锁定日志和核对原scope，此时当前commands文件损坏或移走不阻止原ACK核对。提交成功但回执丢失保留原journal，下一次单步核对准确原操作。新清单才读取当前配置、运行原完整探测、持久化后上报。不得用新operation掩盖未知原结果，不清其他待处理日志。

InventoryReport必有scope_sha256，绑定准确控制端点（保留服务路径）、项目和节点；普通上报、单次上报、Client发送及只读侧车核对共用此约束。凭据不进入摘要，换凭据不等于换作用域。换项目／服务／节点必须使用相应独立状态目录；原摘要缺失或损坏时保留原件并拒绝，不回填、不迁移、不自动清空，也不把旧未知操作当作可安全续期。

浏览器通过原inventory、准确Profile匹配、暂停登记与明确启用复用结果，无新API或远程执行权限。原接入指南增加简洁PowerShell／Bash命令与可达控制面地址、准确Worker／项目；命令只包含公开定位信息和凭据环境变量引用，不包含token值。回环地址明确仅本机可用，远程机器需要可达安全地址。配置准备、探测上报、连接执行和启用分别操作，不自动安装程序或运行复制内容。

## 并行与验收

后端负责讨论快照与源pins；Runtime负责两个Runner模式；Web负责原讨论表单及指南；Root负责共享合同、生成类型和集成。只跑新增与实际受影响定向检查，一次全类型／国际化及代表界面，复用未变SDK和既有候选／独测证据。真实本机文件／HTTP／Runner、受控协议、合成UI和厂商生产分别记录；远程物理多机、更多工具、硬预算、安装及完整平台继续交付。
