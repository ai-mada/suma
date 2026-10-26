# 多工具运行时集成与 AI SOP

状态：T-P4-021 功能本地已实现，原生进程及Node入口定向验证通过；Node实际任务、最终中英界面及原预览前向更新均已本地验证。厂商模型、WorkBuddy客户端及完整团队交接仍待实际验收。产品与授权边界见[PRD](../product/team-cli-runtime.md)、[共享合同](../../contracts/team-cli-runtime.md)。

## 参考与设计

Multica固定提交为[a47a1ced0c71bdb113a43facce4675442acb9001](https://github.com/multica-ai/multica/tree/a47a1ced0c71bdb113a43facce4675442acb9001)，原本地工作树18fa9f7保持。参考其机器runtime注册、团队分工、固定工作区、交付失败保留与持久重传思路，复用速码Go DAG/PoolWorker/Journal及当前权限，未复制参考项目的旁路权限、自动批准或软reset修复逻辑。

CodeBuddy依据[CLI参考](https://www.codebuddy.ai/docs/cli/cli-reference)、[Headless](https://www.codebuddy.ai/docs/cli/headless)和Multica固定源码。Harness依据[DSH集成](https://github.com/multica-ai/multica/blob/a47a1ced0c71bdb113a43facce4675442acb9001/server/pkg/agent/dsh.go)及其实际profile协议。WorkBuddy依据[官方连接器说明](https://www.codebuddy.cn/docs/workbuddy/From-Beginner-to-Expert-Guide/Function-Description/Connector)，单独跟踪，不和CodeBuddy混称。

## 管理员配置

完整无秘密示例见[命令配置](../../deploy/runner/team-cli.example.json)，保留Codex、CodeBuddy和Harness三个原生可执行profile。在执行节点安装CLI并按其正常流程登录，保存为commands.local.json。使用`suma-runner --commands commands.local.json --allow-exec --repository REPOSITORY --base-commit COMMIT --describe-tools`，替换实际路径和完整commit；共享池使用本机pool配置的命令目录。可执行文件、入口、完整基准及私有worktree均由管理员固定，业务任务不能提供或覆盖。

Windows若只有npm生成的`.cmd`，请使用Node入口模式，不调用`cmd.exe`或其它shell。将相应CodeBuddy／Harness条目的`executable`设为已安装的`node`或`node.exe`，把实际绝对`.js`／`.mjs`／`.cjs`入口加到`args`首位，原固定参数顺序不变。例如以下Harness片段中的入口必须先替换为本机真实文件；不能原样导入：

```json
{
  "executable": "node",
  "args": ["D:/REPLACE_WITH_ACTUAL_INSTALL/cli.mjs", "--profile", "multica", "--stdio"]
}
```

CodeBuddy同样在原`-p --input-format text --output-format stream-json --verbose`前加入入口。入口须为非链接普通文件且≤16MiB；相对路径或Node前置执行参数会被拒绝。profile绑定Node可执行文件SHA、入口内容SHA和完整规范化配置，不证明入口导入的整个模块依赖闭包。Suma不猜测npm包位置、不安装包；版本、帮助与profile探测均直调同一入口。网页仅对这两种CLI提供原生／Node选择，入口为空或相对路径时不允许复制，向导默认折叠并保留中英文。

DSH实际探测为`--profile multica --probe`，要求单stdout JSON的v/runtime/type/protocol_version/plugin_version；stderr独立有界，合法诊断不污染JSON。只读探测不发送execute、不启动模型或安装profile。真正执行采用保持开启的stdin，宿主发送固定execute，合法result后关闭；取消发送同request_id的cancel，1.5秒协议宽限后仍通过现有进程树控制确认。

CodeBuddy采用固定print/text输入、stream-json输出与verbose；管理员可明确配置default或acceptEdits及固定工具许可，不支持bypassPermissions、-y、resume或任务追加参数。禁后台任务的设置在配置摘要前规范化并在子进程再次强制；任何后台、权限等待或控制请求都会阻止成功。外部CLI使用自身认证，平台Runner凭证不进入子进程环境。

## 模块与执行语义

`tool_dsh.go`和`tool_codebuddy.go`只归一化有限公共事实；`tool_dsh_input.go`负责固定双向输入，`tool_external_cli.go`负责配置与实际probe。原`toolStream`负责重复键、UTF-8、行／事件限额、秘密过滤、持久ACK和整体证据。源码与工具参数、工具完整返回、私有推理等不会被新decoder隐式投放到公开事件；可显示的原始结果仍有界脱敏。

`Execute`显式拥有stdin copy和pipe，避免子进程无result退出时Wait被阻塞Reader拖住；所有收尾路径关闭并等待copy结束。协议result、进程退出、后代停止分别核对。未上报用量不同于0，局部小计不能当总额。新协议仍从固定工具资格进入现有任务及共享池，未增第二派发中心。

## AI开发操作顺序

1. 固定任务、验收条件、需求／知识修订、流程及工具版本。
2. 读取真实当前工具观察，按项目／机器／池权限预览派发；不把模板或installed标记视为可用。
3. 创建独立attempt工作树，发送一次固定目标；保存进展、用量、结果和候选来源。
4. 取消或失联时核对原attempt和原进程效果；不得自动换会话或重跑未知操作。
5. 独立测试精确候选SHA，由有权限人员明确评审。需要交付目标分支时执行独立的固定结果、测试、审批与CAS合同。
6. 输出的知识／技能／记忆先形成有来源的修订提案，经审阅发布后才进入团队后续上下文；不直接同步私人CLI会话数据库。

只运行发生变化的必要协议、配置、授权及代表性交互检查；重用未变依据。厂商模型、WorkBuddy客户端、完整权限沙箱和完整平台另行验收。
