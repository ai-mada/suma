# WorkBuddy 授权连接限定验证

T-WORKBUDDY-CONNECTIONS-147；2026-10-30，Windows、Go 1.26.10、CGO关闭。直接修改当前项目代码，沿[领域合同](../../contracts/workbuddy-connections.md)、[架构](../architecture/workbuddy-connections.md)及[唯一实施计划](../delivery/implementation-plan.md)。本阶段交付平台授权连接与接入页面；供应方云执行、ACP、预算及候选交付继续独立推进。

## 实现与证据

| 范围 | 实际结果 |
| --- | --- |
| OAuth协议边界 | `go test ./internal/workbuddy -count=1 -v`通过，15个顶层，其中7个新增OAuth用例、8个既有任务协议用例；0.384秒。实际本机HTTP验证授权码、显式刷新、准确路由和单次请求，不调用厂商服务 |
| 平台最终集成 | `go test ./internal/platform -run '^TestWorkBuddyConnections' -count=1 -timeout=90s -v`通过，24个顶层、29个子场景，16.701秒；最终共享capacity helper输入只执行此一次统一检查 |
| Web接入交互 | 26项独立WorkBuddy组件检查通过：19项既有结果及7项新增native行为。末轮只复验3项受影响既有用例（能力缺失指南、准确查询的等待／失权及分页外新连接），不重复计数。先前3个原组件文件29项受影响回归按原范围复用 |
| 类型与语言 | native接线完成后完整noEmit通过；当前合并工作树7402对语言键、生产硬编码0。最终独立候选同样noEmit为0、7402对语言键／硬编码0；不复用无关工作区构建 |
| 桌面接线 | 15项独立窗口边界检查通过；主进程／preload新增3项行为检查先红后绿，2项受影响workspace入口回归通过，248.055ms；语法检查通过。普通弹窗仍deny、生命周期清理与既有打包在途改动分开 |
| 接口与控制程序 | 生成器及检查通过，最终候选754模型、353路径；控制程序配置用例1个顶层／4个场景通过。回调是writeOnly字段与互斥code／access_denied合同；独立落地页GET不虚列query参数 |
| 隔离构建 | 独立候选控制程序与Web均exit0，Vite报告1m 14s；输出到147独立目录，未覆盖145／148活动预览 |
| 实际浏览器／桌面 | 按可用computer-use技能完成真实Electron本机协议窗口、同Session、准确回调／草稿保留和清理；见[原生窗口报告](workbuddy-desktop-oauth-verification.md)。最终候选production preload／controller与Go／Web完成18项本机交互／HTTP，含明确scope、4ID IPC、父页面保留、英文及390px DOM边界；模块fixture与外部请求阻断均不代表厂商成功授权 |

平台24项为13项命令、8项存储、3项真实HTTP／私有读取检查。测试使用真实当前Actor、SQLite、原Server.Handler、Cookie／原视角和本机协议端点；未用伪Actor替代授权路径。集成记录为`.cache/workbuddy-147/platform-integration.log`及exit文件；OAuth为同目录`package-tests.log`；Web紧凑证据为`.cache/workbuddy-147-ui/verification-summary.json`。安全可提交摘要见[JSON](workbuddy-connections-summary.json)，私有配置、令牌、数据库和缓存不提交。

## 已观察到的行为

- 原所有者、原浏览器session、当前项目资格与state期限共同生效。更换会话、撤权、跨所有者读取、跨范围游标、过期或重复回调均不能取得新的兑换许可；代码仅以摘要参与原请求核对。
- 首次无Cookie的回调GET仅返回隔离页面；CSP、no-store、no-referrer有效，交付的脚本在同源session／POST前清除查询。实际HTTP验证Cookie和原视角缺失被拒、同一回调仅发送一次兑换。此项没有模拟浏览器执行结论。
- 连接密文AAD绑定租户、项目、所有者、应用、外部主体、scope及代次；修改绑定、错误密钥和未知初始结构均被拒绝。已有未登记结构在写前失败，不重置原数据。
- 单次外发准备和结果接纳分别检查当前资格及版本。并发刷新、响应丢失、保存失败、断开后的迟到响应均不能自动重发或恢复已撤销资格。供应方未返回新refresh token时明确保留旧值，身份或scope变化进入重新授权。
- 配置缺失、scope收紧、密钥不可用时仍保留当前有权所有者的本地断开。断开清除本地密文并作废旧fence，不声称远端撤销或停止。
- 真实CAS可落到2147483647。重新授权先预留3次版本及对应代次，兑换／刷新先预留2次，断开1次；不足时旧密文、连接、操作均不变且外发次数为0。允许动作直接复用命令容量规则。
- 中文1200字符及2000个Unicode字符／8000字节断开理由按合同接纳，越界拒绝；数值小数、重复／未知字段、非规范分页及换身份后的私有URL受控处理。

## 修复与复用

定向红绿回归实际覆盖了配置收紧仍显示刷新、合法最终版本被错误拒绝、公开动作与可执行容量漂移、Start清除旧凭据后没有完整兑换余量，以及Web列表先失败而准确连接随后拒权时残留私有入口。仅复验受影响用例，最后在共享helper冻结后统一检查24项平台用例。HTTP密文／诊断格式追加断言复验原用例，不新增计数。早期缺实现编译失败、夹具缺原DAG表或命令参数错误分别记录，不当作产品行为红证据。

Go进程使用本仓库`.cache/toolchains/go1.26.10/go/bin/go.exe`；`GOTOOLCHAIN=local`、`GOWORK=off`、`GOENV=off`、`GOPROXY=off`、`GOMODCACHE=.cache/go-mod-runtime`。最终平台集成的`GOCACHE=.cache/workbuddy-147/go-cache`、`GOTMPDIR/TEMP/TMP=.cache/workbuddy-147/http/tmp`仅作用于该进程。未重复全平台后端或无关套件，未把CGO关闭的并发检查称为race验证。

## 剩余验收

需要真实登记的WorkBuddy应用验证官方回调、用户授权与刷新；147中英及390px代表性交互已验；完整明暗、键盘／设备与图片级视觉仍待验。ACP进度／输入／权限请求、当前授权任务网关、硬预算、云产物与固定候选、独立测试、多物理机与其他数据库竞争继续在原计划中；本阶段未将WorkBuddy注册为可派发云工具，没有声明厂商、分布式、生产或完整平台验收。

集成审阅发现原桌面容器拒绝新窗口，`target=_blank`不能完成授权跳转。已接通主进程、preload及原接入组件的四ID按钮，未确认写入／表单／打开中不在focus时读取，返回后只核对准确原连接；并发focus合并，迟到结果与当前身份隔离。新增回归还发现原checkCurrent未核对返回connection ID，已修复，不把另一条同项目连接误报成功。真实Electron本机协议证据及最后可选错误说明分支分别记录；最终匹配构建及18项实际页面／native入口检查已记录，完整视觉、厂商及设备范围继续待验。该模块不导出Cookie／state／code，原生样例没有访问厂商，不证明正式应用授权和完整桌面打包验收。

## 最终独立候选与实际接线

以已推送148提交`b1bfd36`归档，只叠加147正向清单和两个最小入口补丁；未包含桌面打包／启动78码、TaskConversationKnowledge在途改动或149。冻结来源`8d2354f06604f0f47219d9cfb9849d0825b74c8aeab3a51c78cfe8d40705170f`，控制程序`cd6182e557eadfb414a730719d8d9e0923091248b40cc7a8f1fa13eab3b420c7`，Web入口`c5e439468231f334dbd8c5521b6f4bb2d56ea5e0d06d031d8addf73fbb85fe3e`。精确目录与42个覆盖文件见本机source-snapshot；生成接口、类型、语言、构建及5项候选桥接复验分别有退出码，已有用例不重复增加计数。

18项实际检查使用已有合成账号的真实会话、明确合成应用配置及原Go授权链路；只发出1次授权POST。production preload向控制器只传4个ID；真实子窗同Session、无父preload／Node，测试传输层阻止官方外部网络，失败未伪报连接、父URL不变。原记录只读续查、刷新后不恢复私密URL、英文和390px DOM无横向越界通过。原检查在切换英文后持有旧中文region定位，另一次探测误用英文文本，均为测试定位修正；后续只读继续，未重复POST。隐藏窗口没有可接纳图片，不据此验收完整视觉。
