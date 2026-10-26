# 团队多工具执行协议合同

状态：T-P4-021 功能本地已实现，协议、原生进程及Node入口定向验证通过；Node实际任务、最终中英界面及原预览前向更新均已本地验证。厂商模型、WorkBuddy及完整团队交接仍待实际验收。完整候选审查／受管仓库三策略合并、知识／技能／记忆自动萃取与同步保持后续完整目标。

## 参考与模块边界

Multica固定a47a1ced0c71bdb113a43facce4675442acb9001，只读取已锁定源码，不改其本地工作树。实际参考为CodeBuddy Code与DeepSeek Harness；WorkBuddy为独立产品，未核实其公开协议前不能冒称接入。Go标准JSON、进程控制、原ToolContract、DAG、PoolWorker、Journal、隔离工作树、候选上传、测试及人工验收复用，无第二套派发器。

## 固定协议

新增adapter／protocol配对：`harness`／`dsh-multica-jsonl-v1`、`codebuddy`／`codebuddy-jsonl-v1`。command_id、配置／实际可执行SHA、CLI版本仍进入原固定profile。工具注册／探测、可用库存、实际任务、正式测试、人工验收分别记录；现有普通项目及共享资源池沿用同一授权与派发路由。

两种CLI支持原生可执行文件，或由管理员固定的Node入口：`executable`为`node`／`node.exe`，`args`首项为实际绝对`.js`／`.mjs`／`.cjs`文件，后接原固定CLI参数。入口必须是非符号链接的普通文件，大小不超过16MiB；相对路径、Node的`-e`／`--require`前置参数及其它解释器不能绕过校验。所有版本、帮助及profile探测使用同一入口前缀。Node可执行文件SHA、入口文件内容SHA及完整规范化命令配置共同绑定profile；这不证明整个模块依赖闭包。任务或模型不能选择入口，不运行`.cmd`／`.bat`或shell包装器，不自动查找npm入口或安装包。

DSH使用已安装的Multica runtime profile，固定argv为`--profile multica --stdio`，input为`dsh-jsonl`。宿主构造execute帧，绑定完整attempt导出的request_id、实际隔离cwd及固定goal，MCP列表为空；任务不能提供argv、cwd、profile、resume、凭据或模型配置。`newDshInput(requestID,cwd,goal string)(*dshInput,error)`实现io.ReadCloser和Cancel()error，保持stdin直至唯一合法result或宿主关闭。取消只发送原request_id的cancel；短暂协议宽限后仍由既有进程树停止确认。真实ready/version字段以官方profile的固定协议为准。

CodeBuddy使用固定`-p --input-format text --output-format stream-json --verbose`，input为goal；具体支持参数须由实际help及官方文档确认。保留工具自身既有认证，不读取登录文件；不自动加入bypass-permissions，不自动批准control_request，不替用户删除或覆盖MCP配置。等待输入／后台任务缺少已知停止证据时不可报告成功，本阶段不擅自恢复会话。

## 事件和结果

两种decoder复用toolStream的严格有界JSON、UTF-8、重复键和64KiB行限额、脱敏、有界原始输出、持久ToolEvent及净usage。模型文本或退出0不足以代表成功。要求已知协议就绪／会话、同一request/session、唯一整体终态、进程退出及进程树静止同时成立；重复或终态后事件、错误身份、非法计数／未知关键帧失败关闭。未上报usage不同于明确0，不重复累加subtotal和整体总额。

DSH decoder方法`(*toolStream).dsh(map[string]any)`，状态`dshState *dshStreamState`；CodeBuddy为`codebuddy(map[string]any)`及`codebuddyState *codebuddyStreamState`。Root提供`onTerminal func()`，仅唯一合法整体terminal后触发，关闭待写stdin；错误／取消由宿主清理，无盲目重跑。

## 配置、界面与验证

本地可信命令配置决定能力；任务、模型和普通浏览器不能注入命令。网页沿用紧凑工具列表／详情，默认折叠接入向导并提供成对中英文。仅CodeBuddy和Harness显示原生／Node入口选择；Node入口为空或不是绝对路径时禁止复制模板，服务端仍校验真实文件及固定参数。无秘密模板明确安装、profile依赖及验证范围；配置或帮助不代表已执行成功。

仅针对新协议、取消、身份／终态／usage和配置隔离做必要定向；复用上一阶段未变DAG／候选／权限证据。真实CLI未安装则明确记录；协议fixture不能升级为厂商模型验收。功能联调后一次相关集成检查，修复只复验受影响切面。最终只提交必要源码、规范和精简证据。
