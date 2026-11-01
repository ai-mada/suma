# suma 命令行与终端浏览器

入口为 `cmd/suma`。Windows 运行程序按批次保存在 `dist/runtime/windows-amd64/<批次>/suma.exe`；运行前应核对批次清单及其源码版本。历史 `dist/suma.exe` 已于 2026-10-30 清理。它连接现有控制面，项目授权、版本校验和操作回执均由同一 API 执行。

## 配置和身份

全局选项放在命令之前：

```powershell
# SUMA_TOKEN 从当前进程环境读取；不要将真实凭证写入脚本、仓库或命令历史。
.\dist\suma.exe --control http://127.0.0.1:4310 --project PROJECT_ID doctor
.\dist\suma.exe --project PROJECT_ID tasks
```

`--control` 默认 `http://127.0.0.1:4310`，只接受没有用户名、密码、路径、查询串的 HTTP(S) 服务地址。`--project` 为项目 ID，项目内请求必须提供。`--token` 可显式传入凭证，但推荐当前进程的 `SUMA_TOKEN`，避免凭证出现在进程参数。CLI 不保存凭证、不输出当前凭证、不自动登录、不创建凭证。

使用项目范围的 `api` 凭证；`runner` 与 `agent` 凭证不能用于业务查询。凭证创建和有效期以控制面的身份配置为准。远程服务应使用 HTTPS。

`--timeout` 为单次请求时限，默认 `15s`，例如 `--timeout 30s`。Ctrl+C 取消请求。CLI 不自动重试，不跟随 HTTP 重定向，避免将凭证发送到其他地址。

浏览器 Bearer 会话命令需要显式保存原页面身份：`--expected-user USER_ID` 与 `--expected-organization ORGANIZATION_ID`（也可由当前进程 `SUMA_EXPECTED_USER`／`SUMA_EXPECTED_ORGANIZATION` 指定）。仅账户全局偏好使用 user 参数，不能带组织参数。CLI 每次调用只捕获一次这些标识，不自动查询最新 session 替换；未知结果重试应保留标识、操作 key 与正文。标识必须是 32 位小写十六进制。普通项目 `api` 凭证不需要浏览器上下文；这些参数不授予权限。

## 只读命令

```powershell
.\dist\suma.exe projects
.\dist\suma.exe --project PROJECT_ID requirements
.\dist\suma.exe --project PROJECT_ID tasks
.\dist\suma.exe --project PROJECT_ID runs
.\dist\suma.exe --project PROJECT_ID workflows
.\dist\suma.exe --project PROJECT_ID run show RUN_ID
.\dist\suma.exe --project PROJECT_ID run events RUN_ID --after 42
```

这些命令将服务端 JSON 写到标准输出；列表返回的分页字段、运行事件的 `next_cursor` 均保留。可用 `api` 命令显式读取后续列表页或事件游标。

`doctor` 读取 `/healthz`、当前 `/auth/session` 和项目 `/capabilities`，报告服务响应、身份和服务端声明的能力。没有项目时能力标为 `not_checked`。它不将“已配置”升级为“已验证”，任一实际检查失败时退出码为 1。

## Bubble Tea 终端浏览器

在交互终端直接运行 `suma` 或 `suma tui`，进入真正的 Go Bubble Tea 界面。支持项目、需求、任务、运行和流程列表；选择项目会更新后续查询上下文。Enter 打开详情，运行详情按 `e` 读取持久事件。

| 按键 | 操作 |
|---|---|
| Tab / Shift+Tab | 切换列表 |
| ↑ / ↓ 或 k / j | 选择条目、滚动详情 |
| Enter | 选择项目或读取详情 |
| Esc / Backspace | 返回列表 |
| r | 重新读取当前页 |
| n / p | 下一页 / 上一页 |
| e | 运行详情的事件 |
| q / Ctrl+C | 退出 |

终端浏览器只有只读操作。它是控制面浏览器；编辑代码、真实终端、语言服务、调试和完整 IDE 能力另有实施范围。标准输入或输出不是 TTY 时，默认入口及 `tui` 自动输出 `doctor` JSON 后退出，不启动屏幕循环。

## 显式写操作与恢复

发布工作流使用本地 JSON 文件，可直接提供流程对象或 `{ "workflow": {...}, "idempotency_key": "..." }` 信封：

```powershell
.\dist\suma.exe --project PROJECT_ID workflow publish --file workflow.json
.\dist\suma.exe --project PROJECT_ID run start --workflow WORKFLOW_ID --version 2 --operation start-task-42-attempt-1 --task TASK_ID --expected-version 3
```

启动必须明确固定 `--version` 和稳定的 `--operation`（至少 8 字符）。`--task` 可选；绑定任务时可提供其 `--expected-version`，服务端仍会验证任务、流程和项目权限。要另建一次有意的新运行，应使用新的操作身份；恢复上次结果未知的调用时必须复用原身份和参数。

运行控制命令必须使用 JSON 文件：

```json
{
  "action": "pause",
  "expected_version": 3,
  "idempotency_key": "pause-run-42-version-3",
  "payload": {}
}
```

```powershell
.\dist\suma.exe --project PROJECT_ID run command RUN_ID --file command.json
```

命令文件要求 `action` 与正整数 `expected_version`；具体动作、节点、审批和补偿约束由服务端校验。文件中的 `project_id` 如存在，必须等于当前项目。

写入文件没有 `idempotency_key` 时，CLI 从控制面地址、项目、请求方法、资源路径及 JSON 内容计算稳定 SHA256 操作身份，原文件不被改写。相同调用和文件内容可恢复同一操作。要有意重复相同写入，请在文件中明确提供新的身份。收到 409 后应读取当前版本并决定下一操作；CLI 不替你修改版本或换身份。

超时、连接中断或取消后的写入可能已发生。先查询回执或当前状态，再用原身份、原版本和原参数恢复；不要因缺少响应而另建请求。

## 通用 API

```powershell
.\dist\suma.exe --project PROJECT_ID api '/tasks?limit=25&cursor=OPAQUE_CURSOR'
.\dist\suma.exe --project PROJECT_ID api /requirements --method POST --file requirement.json
```

资源路径相对 `/api/v1`。GET 默认只读。POST、PATCH、DELETE 必须显式给出 `--method` 和 JSON 对象 `--file`，文件身份规则同上；服务端不支持的方法仍会返回实际错误。CLI 拒绝绝对地址、路径穿越、URL 内凭证及与当前上下文不一致的项目，项目身份同时放入查询和写入正文。

错误以 `{ "error": { "code", "message", "status"?, "details"? } }` 写到标准错误。退出码：0 成功，1 服务端/网络/终端失败，2 命令或输入错误。响应上限 16 MiB，输入文件上限 4 MiB。

## 验证范围

`internal/cli` 的 HTTP 合同测试覆盖项目与 Bearer 身份、服务端错误和退出状态、文件操作身份稳定性、固定版本启动、流程发布、事件游标、超时无重试、重定向不转发凭证、取消及非 TTY 退出。Bubble Tea 模型测试覆盖只读项目切换和旧响应隔离。实际交互终端的键盘/尺寸行为需要单独实机验收；模型测试和可执行产物构建不代表完整跨终端验收。

## 本机工具安装检测（T-TOOLS-151）

`suma-control --discover-installed-tools --local-tool codex-jsonl --language en-US`
与`suma-runner --discover-installed-tools --local-tool codebuddy-cli --language zh-CN`
共用固定安装／版本／协议探测；省略local-tool检测三个已支持入口。
模式在业务凭据、数据库和执行日志初始化前返回，不启动服务或任务。
报告仅为安装事实与本机模板，不能导入成平台授权或派发证据。
工具暂停登记、独立启用及未知操作恢复见[快速接入合同](../../contracts/tool-quick-connect.md)。
