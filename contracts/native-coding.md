# 原生智能体编码、工作区工具与候选封存合同

状态：T-P4-020 已实现并完成限定本地验证；主集成代理冻结共享合同，三个域代理并行实现。关联完整P0–P7：P2原生智能体与IDE、P4固定工具及恢复、P5独立测试与验收。020预览使用验收后的隔离构建，019程序、网页和备份保留。

## 目标、边界和分工

原生智能体在每次attempt独立Git工作树内实际读取、创建、替换和删除文本文件，运行管理员固定开发检查。宿主在唯一成功终态及所有进程静止之后形成真实候选提交，复用现有bundle、独立测试、进展回写和人工验收。平台Agent凭证仍只提供四项项目读取，不授予控制面主工作区写权限。

本阶段只操作管理员明确信任仓库的隔离工作树。固定命令会执行仓库代码，工作树与Job不是完整文件系统／网络沙箱；容器和低权限身份仍是独立交付范围。不得把开发自测当正式DAG测试，不把本地提交当目标分支合并，不以模型completed替代已知进程、候选及证据。

Root拥有execute.go、tools.go、CommandSpec／共享类型、profile有效摘要与检查启动、生成合同、UI及交付集成。Workspace代理独占`native_workspace.go`、`native_workspace_mcp.go`、`native_workspace_files.go`、`native_workspace_operations.go`及对应必要测试；Seal代理独占`native_workspace_seal.go`及测试；TS代理独占agent-runtime实现、测试、bundle工具与依赖锁定。共享`native_workspace_contract.go`由Root维护。不要复制第二套调度或绕过原Journal。

## 本机配置和摘要

CommandSpec新增可选`native_workspace:{checks:string[],runtime_manifest:string}`。只允许非read_only的`suma/suma-jsonl-v1`、`input:agent-jsonl`，原生JS入口必须是受控单文件ESM bundle；`runtime_manifest`是管理员绝对路径，构建产生版本1清单，绑定bundle SHA、Node外置导入及依赖版本。禁止非Node外置导入。由现有Node可执行SHA＋bundle／manifest实际字节＋policy版本＋每个固定check的规范配置与实际可执行SHA组成有效profile摘要。引用check变化使库存／派发不匹配；检查只允许command/text-v1，不能嵌套Agent、外部流程、制品或原生工具。

未配置policy的旧profile行为保留。完整JS模块／SDK由单文件bundle封闭；不能仅凭入口文件名声称固定了外部模块。配置探测仅版本／帮助，不启动模型或创建工作树。profile中可公开`workspace_tools_version:attempt-workspace-v1`，只表示目录能力声明，不表示已执行。

## attempt-local MCP

复用已固定官方Go SDK v1.8.0，绑定`127.0.0.1:0`，短期随机独立Bearer，scope为当前完整attempt、policy及执行context。只允许固定端点Host，不接受Origin、Cookie、重定向或远程URL。SDK无状态JSON、有界输入／输出，private缓存TTL0。每个调用复核活跃context；关闭后拒绝新操作并取消、排空在途检查。

Go宿主经原生stdin增加`workspace_bridge:{version:1,endpoint,token,policy_sha256,request_id}`。该数据只能由宿主构造，覆盖静态同名配置，不能来自workflow config；令牌加入Runner已知秘密过滤。不进入模型历史、stdout或平台证据。独立`SUMA_AGENT_TOKEN`只用于项目读取。TS采用已核实并固定版本的官方v2 MCP client；无auth provider／OAuth自动回退，fetch固定端点及Bearer、禁redirect、绑定AbortSignal和大小限制。初始化核验服务身份及精确六项目录，未知工具拒绝。

MCP工具名为`suma_workspace_list/read/write/edit/remove/check`；TS映射到模型可见canonical `workspace.list/read/write/edit/remove/check`，再复用确定provider别名与反向映射。项目和工作区目录合并后仍受64工具总限约束，重复名称拒绝。每个call使用隐藏`_meta["suma/operation_id"]`，值由request_id／step／provider call ID确定摘要，模型不可覆盖；同key同输入恢复，异输入冲突。

## 工具和确定语义

| 工具 | 参数 | 正式行为 |
| --- | --- | --- |
| list | directory可省略、limit、cursor | 单目录分页，最多200项；明确next_cursor，过滤保护目录，拒绝非法或过期cursor；不静默冒充全量 |
| read | path、start_line、max_lines | 有界UTF-8文本、全文件SHA／bytes、所读行及next_line／complete；超长单行不能静默截断 |
| write | path、content、expected_sha256或expected_absent:true二选一 | 替换需当前SHA，新建需明确不存在；可创建许可父目录；接受后持久prepared，再写/fsync/根锚定原子替换，最后completed |
| edit | path、expected_sha256、edits | 1–32项old_text/new_text；old_text非空且在原文件唯一，匹配区间互不重叠；同时应用，复用单次CAS受理，避免大文件重复全文与参数限额 |
| remove | path、expected_sha256 | 仅删除一个明确regular文件；持久保存旧SHA及受理，不递归删目录；同操作恢复，当前第三种内容进入核对 |
| check | check_id | 管理员固定命令，工作目录固定Source；不得提供argv/env/cwd/executable/timeout。单个检查总时限60秒，保留实际退出／停止／未知事实和有界脱敏输出 |

路径使用Go1.26 `os.Root`锚定操作，再拒绝敏感文件、Git元数据、链接／reparse、ADS、保留设备名、尾点／空格及大小写别名；禁止绝对路径和`.`／`..`。固定文本文件上限1MiB、每结果64KiB；默认读取200行。输入严格封闭schema、整数词法、重复键/null/UTF-8与数量边界。错误不回显内容、绝对路径或秘密。

文件修改、删除和检查串行。每次变更递增持久generation，检查绑定generation及工作区源内容摘要；检查不能隐式修改源文件。配置的所有check必须在最终内容上已知成功，方可封存；空checks明确表示未做开发自测。检查结果只作开发反馈，独立DAG test仍必须测试封存后的同一SHA。

回执位于Source外，绑定完整attempt、policy和规范请求SHA。先受理再效果，回执未知时核对文件旧／新SHA；第三种内容、检查进程无法证明停止或已有unknown操作阻断继续写、重复执行与封存。Runner重启保持已有普通执行outcome_unknown，不自动重跑模型。这里的操作核对不擅自恢复整个未知attempt。

## 宿主候选封存

仅在原生进程唯一成功终态、退出0、进程树已知静止、MCP已关闭且所有必要检查当前成功后执行。禁止模型操作`.git`或提供Git参数。Native工具的最新文件效果清单必须覆盖所有源码变更，当前文件SHA与已接受结果一致；检查产生未受理的源变更则阻断。

使用成熟Git plumbing及隔离index：以固定原HEAD读树，对受理文件以`hash-object --no-filters`和cacheinfo更新；删除只移除批准path。禁外部diff、textconv、hooks／filters和任意Git配置。确定保存初始HEAD、tree、机器作者／日期、operation与候选commit，再使用HEAD旧值CAS与独立操作receipt ref核对。原HEAD变化、第三种ref、丢失原始依据都进入核对，不强制reset、不自动重跑模型。机器提交不能冒用人类作者。

无净变更返回`no_change`，不造新提交。完成后记录`NativeWorkspaceSeal`，再复用原capture、candidate pending、bundle上传与ACK。源码未改变的结果复用和多源合并仍按全平台待办独立推进，不能把未传包记成已传包。

## 证据与验收

`NativeWorkspaceEvidence`保存policy、generation、路径和SHA效果、开发检查摘要及seal；无内容／令牌。前端只渲染经固定版本／类型校验的摘要，原SHA、路径和历史事实保留，可按需展开；未知、未检查、未封存分别显示。中英共用目录，可扩展语言，不递归翻译原文。

必要纵向：真实工作树列举／读取→CAS修复及新建／删除→固定检查失败／修复／通过→机器封存→跨独立工作树正式测试同SHA→现有任务人工评审；模型边界可使用明确标注的本地协议fixture，实际文件、检查、Git、bundle和数据库均需真实执行。另检查路径逃逸、同key冲突／丢ACK、过期SHA、取消及未知副作用、scope结束、源变动、检查摘要闭包和bundle／Go SDK协商。复用未变更018／019证据，不重复无关全量或设备矩阵；共同依赖发生变化时做一次必要集成门禁。真实厂商模型、生产、容器沙箱与完整平台仍单独验收。
