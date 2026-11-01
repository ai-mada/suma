# Multica 工具重新检查采用记录

状态：限定实现与验证完成；suma1.0.0；T-TOOL-REINSPECTION-194。负责人：主集成代理。关联[重新检查合同](../../contracts/tool-reinspection.md)及[限定验证](../quality/tool-reinspection-verification.md)。原31项需求、P0–P7及完整平台目标继续。

## 参考基线

本次只读检查`D:\codes\github\multica`，本机main的HEAD为`5a2cc891ac338eed1919aa11a1eef5957cd82cec`。采用依据是当时工作树的实际文件字节；15份来源SHA及7条流程记录保存在本阶段忽略目录的`read-reference.json`。没有fetch、checkout或修改参考仓库，也未确认本地修改状态，不能宣称工作树干净、文件与HEAD完全相同或已获取远端最新代码。

该工作树`LICENSE`由Apache2.0全文及PartI附加条件共同组成，另有托管／嵌入商业分发、界面品牌和归属说明等条款，不能等同无附加条件的Apache2.0。本阶段没有复制其实现、界面或商业部分；仅提取可独立实现的流程与边界，保留固定来源和许可事实。

## 已核对的采用点

| 流程 | 实际来源与符号 | 速码采用边界 |
| --- | --- | --- |
| 发现与原生检查分层 | `agents_probe.go`的`probeAgentCLIs`、`cachedShellResolvedAgents`；`agents_refresh.go`的`agentDiscoveryLoop` | 区分路径发现、原生版本检查、库存上报与可派发资格；读取已有数据不能显示为一次新扫描 |
| 收敛与失败退避 | `convergeAgentRuntimes`、`nextConvergeBackoff`、`refreshAgentVersions` | 准确目标的并发请求合并与有界退避可以减少重复探测；失败不产生健康、已修复或已安装事实 |
| 稳定来源 | `daemon.go`的`resolveAgentEntryWithHeal`、`resolveAgentLaunchTarget` | 路径和版本成对观察，具体目标变化后重新核对；不能自动替换速码已审pin、固定配置或计划依据 |
| 连接确认 | `connect-remote-dialog.tsx`的`WorkspaceConnectDialog`、`handleDaemonRegister` | 先建立准确范围的新鲜初始基线，再用目标节点事实确认；通知只触发读取，不直接证明连接成功 |
| 远程请求生命周期 | `runtime_models.go`的`ModelListRequest`、`InitiateListModels`、`GetModelListRequest`、`ReportModelListResult` | 参考准确runtime／request关联、出站daemon领取与回写、晚到终态保护；这是模型目录示例，不是现成的工具库存重扫接口 |
| 本地生命周期边界 | `daemon-os.ts`的`daemonLifecycleUnreachable`及`daemon-manager.ts`的生命周期串行入口 | 原生动作前核对真实目标；普通读取或重新检查不能暗中重启、停用另一命名空间或中断活执行 |
| 移除与历史 | `runtime_teardown.go`的`TeardownRuntime`及工作区关联检查 | 借鉴影响说明与历史保护，继续速码原pause／revoke语义；不搬入物理删除、自动取消或把业务取消说成进程停止 |

## 不直接采用的行为

Multica路径发现包含带TTL和环境指纹的login-shell回退；缓存未命中时可能执行用户rc文件。速码不能把这种行为嵌入被动浏览器读取，也不能在明确指定路径失败后静默换到另一工具。

其当前daemon已具有周期发现与版本收敛实现；onboarding中的“必须重启才能重新探测”注释不能替代实际实现作为设计依据。重新检查不应默认重启节点或销毁原执行。

参考实现的最低版本自愈、自动注册、缓存返回及兼容分支，均不等于速码的精确来源复核、当前授权和明确启用。缓存结果应保留原观察时间，不能因一次重读刷新为新的原生检查。多副本下的请求生命周期需要一致存储，不能用进程内状态或通知代替正式事实。

本阶段不自动批准权限、改变模型或pin、安装工具、复制登录材料、停进程或清理Journal。未知副作用仍核对原请求和原回执，不用新操作盲重。

## 本阶段实现方向与验证

T194已限定实现持久且准确绑定worker的重新检查请求，由原inventory执行协程进行新的受控探测，并将结果与原库存原子关联。它继续使用速码现有控制面、执行节点身份和库存流程；具体接口、字段、状态及恢复规则以本阶段合同为准，本文不另立定义。

仅完成参考核对不代表该链路已经实现或验收。源码、定向检查、实际HTTP／节点行为、代表性界面与生产验收分别记录。当前参考阶段没有运行Multica daemon、厂商程序、模型、安装、测试或构建；后续真实验证统一进入本阶段报告，不重复已通过且输入未变的SDK或正式质量旅程。
