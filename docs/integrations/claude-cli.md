# Claude Code CLI执行接入

T-CLAUDE-CLI-158，速码1.0.0。此入口让速码向已安装的Claude Code派发前台开发任务，与[Claude调用速码的MCP配置](developer-clients.md)及尚待实施的Agent SDK入口分开。完整行为以[执行合同](../../contracts/claude-cli.md)为准。

当前代码与定向协议／界面检查已接通；真实本机进程采用明确标记的协议夹具。核查本机PATH及常见安装位置未发现Claude，因此没有安装、登录或运行厂商模型，不宣称厂商任务验收通过。最终组合类型／构建与实际界面检查由本阶段整合记录说明。

## 接入原任务流程

1. 由本地管理员准备已审阅的Claude Code **2.1.314原生可执行文件**，使用该工具自身登录。速码不读取或复制登录文件，不支持npm／shell包装器；其他版本会明确保持不可用。
2. 在工具接入面板选择Claude Code CLI，先检测安装。桌面托管沿原明确Git目录绑定及私有凭据流程；手动节点可复制展开向导中的固定配置，再使用`--describe-tools`检查实际版本与协议。探测不调用模型。
3. 收到该节点本次真实库存后，核对完整模型、版本与执行范围，暂停登记档案；需要派发时再明确启用。已配置或已连接不代表具有当前任务资格。
4. 从任务创建原派发预览，核对任务／需求版本、固定工作流与工具档案后明确开始。正式Runner沿原授权、claim、心跳、事件和结果通道执行，不借浏览器直接启动模型。
5. 读取原运行的真实工作树和提交证据。只有新的已提交HEAD、clean状态和原候选传输证据才能形成候选；随后使用独立测试节点测试准确候选，最后由人员明确验收。成功文本不能替代这些步骤。

## 模型、工具与权限

模板固定完整模型ID`claude-sonnet-5`，来源为官方当前CLI示例；其在具体账户上的可用性仍需真实执行核验。管理员可在本地档案明确配置另一个完整`claude-*` ID，重新探测与登记；任务不能覆盖模型、命令、目录、MCP、权限或会话。init报告的模型必须与原固定ID一致。界面显示配置事实，不提供伪造的平台gateway选择。

固定`-p`、text stdin、verbose及stream-json输出；启用safe-mode、restricted、无会话持久化和内建文件目录读写保护。文件工具为Read／Edit／Write／Glob／Grep；Bash自动许可仅限Git状态、差异、暂存和提交规则，不授予通用Bash。项目扩展、MCP发现、后台／子代理、计划任务和此次进程自动更新均关闭，不自动批准权限请求，不恢复任意会话。

本地管理员仍须审阅这组权限。managed policy优先，safe-mode不能关闭所有受管hooks；内建文件保护不是Bash的操作系统隔离。用户自身认证上下文保留，但平台Runner凭据不会传入Claude。

CLI用量最多作为partial，缺失为unreported，崩溃后不会以零覆盖已有部分计数。客户端费用估算不是供应方账单；组织或项目启用enforce预算时拒绝此入口，不能用CLI参数冒称平台硬预算。

## 未知与停止

缺少整体结果、会话／模型变化、未完成工具、deferred／排队工作及未知输出都不能报告成功。权限拒绝保留原事实；不要改操作ID或自动重跑来掩盖不确定结果。

停止由原宿主进程控制与实际关闭证据确认。重启恢复原journal／outbox，不自动重发模型目标或按会话名猜测resume。Windows本机协议夹具的停止检查不等于真实Claude、其他系统或远端模型已验证停止；这些范围分别记录。

官方依据：[2.1.314发布](https://github.com/anthropics/claude-code/releases/tag/v2.1.314)、[CLI参数](https://code.claude.com/docs/en/cli-reference)、[程序化执行](https://code.claude.com/docs/en/headless)、[权限](https://code.claude.com/docs/en/permissions)、[环境变量](https://code.claude.com/docs/en/env-vars)、[用量边界](https://code.claude.com/docs/en/agent-sdk/cost-tracking)。本入口不宣称Claude SDK、任意续接、供应方计费、其他数据库、多机、安装包或生产验收已完成；完整58／34／92目标保留。
