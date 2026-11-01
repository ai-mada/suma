# 原执行提问与计划确认：限定验证

T-CURSOR-INPUTS-190；suma1.0.0；2026-11-01。状态：本阶段限定通过，完整目标保持active。

[产品](../product/cursor-inputs.md) · [合同](../../contracts/cursor-inputs.md) · [架构与AI SOP](../architecture/cursor-inputs.md) · [交互](../design/cursor-inputs.md) · [English](../en/cursor-inputs.md) · [摘要与证据](cursor-inputs-summary.json)

## 实现

- 复用原ToolInput领域，通过独占Codex／Cursor来源与专用答案接通原提问、多选和完整计划确认；不新增表、HTTP路径、派发器或命令批准。
- 原生source绑定准确session、原Prompt与server typed ID、method、配置及完整规范化payload摘要；正常typed响应绑定原参数，拒绝不触RPCerror fallback，不接任意planUri。
- 原ACP SDK0.13.5保持不变；仅在既有严格帧入口接两确切方法，共用原singlewriter、live ID／32总量上限及持久写前意图。
- 原权限、普通／Pool机器、CAS、当前预算／租约／暂停、原回执优先及断线恢复复用；不重答未知请求或重建原生会话。
- 通用送达与两真实native协议明确投影；Codex SQL／操作回执／Journal保持原turn_closed编码，Cursor执行关闭不伪造resolved或接受ACK。
- 修复写前持久IO期间关闭后仍可能写出的P2，最终状态门保留unknown意图；新输入slice及观察深拷贝，避免调用者修改已保存证据。
- 原面板按协议显示无预选单／多选、原文计划、接受／拒绝／取消；资料详情折叠、完整原意图恢复、中英文与窄屏／键盘接通；修复字段重排误清草稿及Schema capability enum。
- 新的本地受控完整链实际通过原Browser HTTP答复、SDK、Worker、Journal、Git候选、不同Worker同提交Node/JUnit和隔离账号Review，原工作区草稿保留。

## 实际验证

51个不同顶层检查（22新、29个直接受影响旧例），38个Go子例通过；平台5／Runtime16／新formal1／Web20／桌面1／Root8，不把重试或子例重复计数。全类型与8828对中英词条、占位符、复数、markup和0生产硬编码通过。原失败和辅助诊断分别保留，未冒充全部命令首遍成功。

新完整链只运行一次：同一Prompt的numeric request多选→原Browser答案API→string request计划正常typed rejected→原一次权限→真实候选 `1130f42647184aaf9811a2665b27c035d6989b60`→不同Worker真实Node/JUnit，test base/head同SHA→隔离邀请账号Review accepted。两输入证据都写入并关闭、无fabricated resolved；原Journal结果／进度pending=0，4个owned PID均消失，管理员原草稿保留。原189没有输入的同类旅程未重复。

一次隔离组件构建，一次原生GUI首遍3项通过：原单／多选IDs与保存／送达分开；完整计划无默认明确接受；英文390px暗色原tuple/body/key优先恢复后再查看新attempt。两张视口已由Root查看，pageerror=0，原生exit=0；没有覆盖活动预览。

生成合同增加15个闭合Schema、0新路径／表，原8个输入接口描述更新。独立窄审发现List.protocol漏Cursor枚举已修，原新cgen top单独复验并按实际生成器变更再生成；无无故全局重跑。原native Codex关闭字段、操作请求hash、回执与Journal编码均保持明确协议投影，不静默吞字段。

## 边界与继续交付

- 本轮未启动真实Cursor或Codex CLI、供应方账号或模型。固定Cursor2026.10.22-27dcf5f的189包／版本／握手证据及本轮只读模块事实复用，不冒充认证后模型或完整vendor扩展验收。
- 完整链的开发宿主是经过生产包门核验的自有Go原生fixture，不是Cursor或Node厂商包。SDK／HTTP／SQLite／Git及独立Node/JUnit是真实现；Review账号是隔离夹具，不是客户签署。
- 接受计划可能让原工具在本attempt私有HOME保存计划文档；该原生artifact不成为正式候选／需求／DAG或验收基线。后续命令权限、独立测试及人工验收仍各自检查。
- Cursor只有真实写入与原Prompt／执行关闭事实，没有Codex请求resolved或通用native接受ACK；完整写入不能保证模型遵从。未知保留原记录、不盲目重答。
- GUI使用生产Panel／Card与明确合成父级、账号、HTTP、scope及回执，3代表场景／2视口Root查看、一次构建、一次原生宿主正常退出；不是完整登录App／真实供应方验收。
- 初始完整类型及8828对词条检查通过；随后只因独立审阅发现List.protocol漏cu枚举，修生成Schema并重原新cgen top／第二次生成，TypeScript形状与Web产品源未变，没有重复整套类型或业务。
- 原失败、fixture／import／union诊断与package内真实通过结果分别保存；只重直接失败场景。未重复SDK安装／构建、版本探测、189无交互formal、全套或安装矩阵。
- 等待仍占原活进程及原资源／deadline，不声称释放执行槽位或恢复旧session。非桌面Runner配置引导、认证后生命周期、完整CLI／团队／知识／质量／运维、多机／跨库／跨平台安装／规模及原31／P0–P7／58／34／92继续。

## 原件与复现

忽略目录 `.cache/cursor-inputs-190/` 保留每条实际命令、进程内TEMP／TMP／离线Go缓存、原失败、source／artifact清单及哈希、SQLite、工作树、图片和构建；摘要记录关键证据SHA256。代码中的新case验证真正的并发、权限、摘要、未知写、深拷贝及恢复，不写镜像实现测试。完整链需显式opt-in，见 `internal/platform/cursor_acp_runtime_test.go`；不把受控进程当厂商包。

完整平台31项、P0–P7及58限定／34待交付／92总账仍active，不以本片或单项绿色关闭整体目标。
