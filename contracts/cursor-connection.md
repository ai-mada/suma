# Cursor 接入核验合同

T-CURSOR-CONNECTION-166，速码 1.0.0，2026-10-31。本合同记录当时的被动发现范围；当前执行与受核 Node 包探测以 [Cursor ACP 执行合同](cursor-acp-execution.md)为准，不能把下文的历史 planned 状态当作当前目录状态。参考 Multica 的工具发现与统一接入方式，独立实现，不复制 Cursor 专有源码或分发包。

## 用户结果与边界

工具快捷接入允许选择 Cursor，桌面端一次检查当前 PATH 上固定名称的入口，浏览器提供准确的官方指南。明确区分编辑器、Agent 候选入口及正式派发资格；本次检查不安装、登录、运行候选程序、读取配置或调用模型，不注册 Runner、不创建任务或执行配置。远程节点和当前电脑的观察不能混同。

保留原 `suma-installed-tools-v1` IPC 合同和两次当前身份／项目授权核对。不新增 HTTP 接口或可用执行 Profile。Cursor 结果为闭合数据：`adapter_id=cursor-cli`、`product=Cursor`、`launch_supported=false`，没有 `command` 或 `version`。`installed` 只表示固定候选文件存在，不能证明厂商身份、版本、授权或健康。

| reason_code | installed | executable | 解释 |
| --- | --- | --- | --- |
| `not_installed` | false | 缺失 | 在此次检查范围未发现 Agent／编辑器 |
| `cursor_editor_only` | false | 缺失 | 只发现编辑器入口；没有 Agent 候选 |
| `cursor_agent_unverified` | true | 绝对路径 | 原生候选存在，身份／协议／写执行未核验 |
| `cursor_launcher_unsupported` | true | 绝对路径 | 启动脚本候选存在，尚不能交给平台执行 |
| `cursor_scan_incomplete` | false | 缺失 | 文件访问失败或结果不完整，不能判断未安装 |

仅检查已规范化的绝对 PATH 目录及固定 `agent`／`cursor-agent` 名称；Windows仅接受普通本地盘符目录，UNC共享与设备命名空间在任何文件访问前拒绝为检查不完整，避免隐式远程访问。Windows 包括 `.exe/.cmd/.bat/.ps1`，其他系统检查原生名称与脚本候选。编辑器的 `cursor`／`cursor.cmd` 只能形成 editor-only 提示。跳过非正规文件及不可信符号链接，不执行任意名为 agent 的程序；取消／截止时间保留原失败。多候选优先未验证原生入口，其次启动脚本；不能用路径或存在性自动扩大执行权限。

UI 对范围、观察时间、加载、失败及无桌面桥分别给出中英文提示。每次重新检查先撤下上次事实；项目／账号／API 身份变化或卸载丢弃旧响应。Cursor 接入指南不能进入普通登记／启用／派发路径，也不能通过高级配置生成虚假的执行模板。保留既有工具快捷连接、原操作恢复和权限门禁。

## 写执行的后续准入条件

当前官方 Windows 包 `2026.10.22-27dcf5f` 采用启动脚本、打包 Node 与 index.js；下载并检查包不等于安装或协议实测。后续必须分别解决入口及依赖完整性、固定版本／参数、配置自动发现、MCP／hooks／第三方配置隔离、真实文件隔离、共享 Git 元数据权限、模型身份与观察用量、停止／核对及同提交独立测试。没有证据前不允许编造 native 入口、安全参数或 ready 状态。

正式执行继续复用原 Go 调度器、当前授权、Profile、独立工作树、Journal／outbox、fence、候选、独立测试与人工验收。禁止盲目复制 `--yolo`／`--force`、自动批准 MCP、未知结果新会话重发或把配置当健康。需求范围与完整平台交付清单不变。

官方依据：[安装](https://cursor.com/docs/cli/installation)、[参数](https://cursor.com/docs/cli/reference/parameters)、[配置](https://cursor.com/docs/cli/reference/configuration)、[权限](https://cursor.com/docs/cli/reference/permissions)、[无头模式](https://cursor.com/docs/cli/headless)、[输出协议](https://cursor.com/docs/cli/reference/output-format)、[文件隔离](https://cursor.com/docs/agent/security/run-modes)、[第三方配置导入](https://cursor.com/docs/reference/third-party-hooks)。

## 所有权与验收

Root 负责共享合同、桌面 IPC 闭合校验、目录事实、文档和 Git；Go 代理负责固定路径被动发现；Web 代理负责快捷接入与 Cursor 指引。先验证纯文件发现和 renderer 边界，再验证一次代表性桌面交互／浏览器状态；不重复 T165 或完整设备矩阵。不把夹具当作真实 Cursor 执行、物理多机或生产验收。
