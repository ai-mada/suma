# 远程工作区单次 CAS 写命令

状态：T-P4-038／1.0.0，有界实现，2026-10-27。负责人：Control／Runner；公共字段和限额以[合同](../../contracts/workspace-write.md)为准，操作见[AI SOP](../operations/remote-workspace-write.md)。本地GUI与真实联合已取得[限定证据](../quality/workspace-write-verification.md)；物理多机／TLS、生产和完整远程IDE继续实施。

## 结构与许可点

写入是独立 `workspace-write-v1` 命令，未扩张只读 observation 的权限。工作台／CLI 从完整 `full_file` 观察形成草稿，以原文件字节 SHA 提交完整新 UTF-8 正文；Control 固定命令，Worker 从既有出站 HTTP 领取，使用当前进程内注册的 Native 句柄和原生 CAS 账本执行。浏览器没有宿主路径、MCP 端口或 Bearer，Control 不打开远端文件。

| 模块 | 责任 |
| --- | --- |
| `internal/workspacewritecontract` | 封闭 DTO、原请求／结果摘要及准确效果校验 |
| `internal/platform/workspace_write*` | 新写授权、持久命令、一次许可、显式核对、事实排空和无正文查询 |
| `internal/runner/workspace_write*` | 原句柄 CAS、同锁无效果屏障、准确 journal 恢复 |
| `cmd/suma-control` | 既有后台周期内每轮最多100项到期转换／正文清除 |
| 工作台／CLI | 内存草稿、明确保存／核对、冲突人工采用和 metadata 恢复 |

`suma_workspace_writes`保存不可变 scope／policy／摘要、原非秘密凭据引用、原 assignment、版本／状态、一次 execute 身份、各 inspect 身份与结果摘要。正文独立在 delivery 列，不进入 JSON metadata 或 `suma_operations`回执；按 caller、领取 scope 和到期建立索引。GET／列表统一处理期限，列表仅原凭据最近50项，准确 COUNT 不表示全局权限库存。

当前身份先按既有全序合并账户、组织和凭据锁，再核对准确 DAG／原派发及命令 CAS。Create 和 execute 均要求原 caller 当前项目写权限、未接受的当前任务 run／basis、当前已批准 Native 工具。私有 Runner 核对当前机器和固定 run 路由；Pool 额外核对 active Pool、原准确机器凭据和当前双边 grant ID／版本。`CanWrite`推导人和任务资格，不探测宿主健康。

控制面数据库时间决定120秒请求 TTL 和最多10秒 execute 许可。Runner 在同一 Native 锁内、准备和 rename 前再查原活动 scope／policy、取消和许可期限。两个节点间不存在原子撤权事务：已发许可到期前可能完成原授权效果；文档和 UI 不把撤权响应描述为远端即时停止保证。

## 未知效果与恢复

execute 只发一次，固定 fence=1；claim响应丢失仅恢复相同有效许可，不产生第二份许可。超时进入 reconciling，清 delivery 正文，不重排 execute。创建POST回执丢失后，原凭据和原key／请求摘要可凭当前项目读取权恢复已存在metadata；该分支不授execute，新建和首次execute仍需写权限。空队列仍完整授权，但 `lease:null`不落操作记录；非空 lease 的回执只含 metadata，恢复正文只允许原许可仍有效且当前新写授权仍通过。

Runner先持久固定领取身份，处理开始前移除 journal 中的 delivery 正文，再执行原 `remote-write:{id}`。已开始但结果未知的恢复只核对原账本，不能再次 rename；prepared远程操作不会在启动恢复时被自动补写。ACK丢失重发原结果，后续文件变化不改变旧准确回执。

核对由原 caller 当前项目读取权下的显式请求发起，命令版本 CAS 创建新 inspection ID；同 key／原 version 的丢响应恢复先查原操作。inspect 独立短租约，无正文，只查询原操作。活动句柄发现原操作尚未开始时，必须在与模型写入和封存相同的锁内落 `not_started`终态屏障，防止晚 execute 越过“无效果”判断；无句柄或无法证明时保留 unknown，不重开历史文件系统。

Complete 是固定事实排空入口，仍要求当前原机器凭据和原完整 assignment／claim／scope／policy／request 摘要。允许原 caller 撤权、目标关闭或 Pool grant 撤回后的准确旧事实落库，不授予新写或正文权限。applied只接受准确原生效果；后续 unknown不降级已确认效果。公开 GET／列表仍要求原调用凭据当前项目读取权，目标消失不抹去已发生的 metadata。

## 文件一致性与质量边界

原生 write 与模型 write 共用锁、CAS、账本和 generation；两个相同基线的并发写只有一个获胜。现有完整文本≤48KiB，新文本同限额；共享相对路径校验不能替代 os.Root、路径身份、精确大小写及敏感内容检查。

generation 增加使此前原生 checks失效，封存前需对当前准确版本重新通过既有 check门禁。工作流／计划、保存成功、正式候选测试和人工验收分别记录；写命令不改变 DAG、任务验收或模型用量。已固定计划不能继续充当变化后文件的检查证据。

Control正文清除不等于Runner私有journal、原生恢复账本、浏览器内存和磁盘旧页同步擦除。命令metadata保留效果身份，最近列表不恢复源码草稿。持久跨会话草稿、远程Git写、PTY／LSP／DAP及生产隔离不随本阶段开放。

## 当前证据

[Control用例](../../internal/platform/workspace_write_test.go)覆盖原caller／viewer、任务basis变化、一次许可／到期、显式核对恢复、晚回执、Pool撤回及无正文列表；与037观察回归合并定向通过，8.856秒。[Runner用例](../../internal/runner/workspace_write_test.go)覆盖同锁CAS、旧checks失效、not_started屏障和journal准确恢复。真实Worker联合和GUI证据由阶段验证记录分别给出，不从单元通过推导多机或生产完成。
