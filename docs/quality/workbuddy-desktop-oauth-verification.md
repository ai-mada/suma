# WorkBuddy 原生授权窗口验证

T-WORKBUDDY-CONNECTIONS-147；2026-10-30。生产模块为`apps/desktop/src/oauth-window.cjs`，主进程／preload／接入页面已接线并完成定向检查，最终独立候选和构建由本阶段集成者收口。此报告验证模块和真实Electron生命周期；完整平台报告见[授权连接验证](workbuddy-connections-verification.md)。

## 实现

`createWorkBuddyOAuthController(...).open(event, request)`只接项目、授权实例、原用户与组织四个ID。核对主窗口／原mainFrame及来源，再以父窗口同Session先读当前身份、后读准确授权实例；请求固定站内GET、原视角、include凭据、禁止重定向且no-store。当前用户／组织、所有者／项目、pending、到期与真实服务端client ID共同成立才创建窗口。本机client ID／scope约束可选且只能收紧，远程平台不依赖桌面本地供应方配置。

子窗口保留同Session，采用隔离渲染、sandbox及Web安全，不设置preload或Node。只接受固定官方来源及原state的准确回调；见到已提交的回调后，才允许原项目／原连接的安全返回地址。完成时关闭子窗口并聚焦父窗口，不加载父URL、不覆盖草稿；窗口打开或返回不等于平台已连接。普通弹窗仍拒绝，不提供任意URL或命令IPC。

## 定向检查

`node --test apps/desktop/tests/oauth-window.test.cjs`最初12项通过，252.9538ms。增加身份读取及可选operator约束后，新增2项先红后绿172.5929ms，另4项受影响用例177.3046ms通过；后续可选错误说明补1项红绿178.4712ms，累计15项独立用例，复验不重复计数。语法检查通过。初始模块缺失是编译／加载RED；后续真实行为RED分别捕获缺少连接ID被正则隐式转字符串接纳、等待GET使五分钟预算延长，以及未核对当前用户／组织和缺本地client配置被错误拒绝。

日志保存在`.cache/workbuddy-147-desktop/`：`oauth-window-green.log`、`oauth-session-green.log`、`oauth-session-affected.log`、`oauth-description-green.log`及对应RED记录。最后读审修复了与Go bridge的已知可选字段差异：access_denied可带单一error_description（最多2048字节、无控制字符，仅校验）；重复、未知、超限说明及成功code响应夹带说明继续拒绝。未把模拟BrowserWindow用例视为真实窗口验收。

主进程／preload新增3项行为检查和2项受影响workspace回归通过248.055ms；测试准确限定两个允许IPC channel，没有放宽为任意代理。原Web组件新增7项native用例通过，另3项受影响原用例复验通过；类型与7402对语言键检查通过。窗口打开、准确连接GET、focus去重、表单／未确认写入保护、迟到结果和安全失败提示分别覆盖。读取结果必须匹配原connection ID，不能用另一条同项目连接显示成功。原桌面打包WIP没有纳入本阶段最小补丁。

## 实际原生窗口

使用本机已安装Electron35.7.5，隔离验证入口和用户数据位于`.cache/workbuddy-147-desktop/native/`。HTTP服务仅监听随机回环端口；该Session的HTTPS协议处理只返回明确标识的本地授权样页，其他HTTPS拒绝。没有厂商账号、真实code／token或厂商网络调用。操作使用computer-use技能的`@oai/sky`，观察真实BrowserWindow、可访问树、截图及安全生命周期回执。

实际观察通过：

- 同Session的两次当前读取携带合成Cookie和原视角；跨站回调首次GET不带Strict Cookie，清查询后的同源读取恢复同一Cookie。
- 子窗实际页面中`require`、`process`、父窗专用`fixtureBridge`均为`undefined`。
- 添加唯一草稿标记后走准确回调，回调地址查询被移除，子窗关闭并聚焦父窗；父URL、原文档及草稿标记保留。
- 请求额外弹窗后，没有新增原生窗口。
- 越界导航关闭子窗，没有进入该独立Session的HTTPS处理器。
- 父窗关闭时，仍活跃的子窗先关闭；最终回执记录shutdown，随后窗口列表没有本次验证窗口。

实际验证源SHA256为`32186cdd17f75817a70547285b7c738ca02de0e893a559e9e7d8086188267213`；安全回执为`native/receipt.json`，包含事件、布尔值与源摘要，不包含回调code／state／Cookie值。截图已在工具中实际观察，按技能要求没有另存或重新输出图像payload；不虚列不存在的图片文件。

当前模块SHA256为`3f5819eaa6e13cbf23a2179f5c437cfa81fa52089a0f2fb65579b47cfc0b0904`。实际窗口流程均未带error_description，保留原源摘要与观测范围；之后的可选说明解析分支只由上述定向红绿验证，不称当前SHA的完整原生复验。

前两次受限执行分别在创建窗口后退出、以及父页加载阶段发生`ERR_FAILED`；图形子进程报告`0xc0000135`。首份和第二份回执分别保留为`attempt1-receipt.json`、`attempt2-receipt.json`。第二次进程已确认终止后，经工具允许的一次本机执行启动第三次并完成上述流程。只有隔离验证入口禁用硬件加速，生产参数未改变，未使用`--no-sandbox`。现有记录不能单独证明生产默认图形配置或所有目标机器通过。

## 仍需验证

本机协议样例不证明厂商登录、二次身份来源、正式应用回调登记或真实令牌兑换。完整主程序接线、最终Web接入页面、中英／390px／明暗／键盘代表状态与正式打包仍按本阶段总报告记录；不得把窗口返回升级为授权成功、云任务可派发或生产验收。

## 匹配候选的接线验收

主集成在148新基线上仅叠加147明确源文件与最小main补丁，独立类型、语言、控制面／Web构建和5项桥接组合复验通过。production preload／controller和真实候选界面完成18项本机GUI／HTTP检查：明确scope后单次准备、四ID IPC、同Session且无父桥接能力的子窗、外部请求由fixture阻断、父页面保留、原连接仍未授权、英文及390px只读续查；见[总报告](workbuddy-connections-verification.md)。这次只验供应方导航失败／核对路径；上文成功本机callback仍保留其原源摘要，不冒充真实厂商或最终模块全路径复验。最终隐藏窗口没有可接纳图片，设备、明暗、完整键盘、打包及真实厂商授权仍待交付。
