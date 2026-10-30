# 桌面托管本机 Runner 合同

T-MULTICA-DESKTOP-153，suma 1.0.0。本合同接续[工具快速接入](tool-quick-connect.md)，复用[工具执行](tool-execution.md)及原项目节点、凭据、Go Worker 与 DAG；不增设执行权威。[产品](../docs/product/desktop-managed-runner.md)、[架构](../docs/architecture/desktop-managed-runner.md)和[交互](../docs/design/desktop-managed-runner.md)分别说明目标与使用方式。

## 当前范围

仅支持固定 `codex-jsonl`、`codebuddy-cli`、`dsh-multica` 三个入口，由桌面现有 control 程序运行本机 Worker。当前发行接口限定本机 loopback、SQLite、真实浏览器会话、当前项目管理权限及机器 HMAC。普通网页、远程 SaaS 控制面、其他数据库及任意程序地址不通过此私有接口获授本机能力。

## 页面与原生边界

`window.sumaDesktop.manageLocalTool` 只接受准确字段：`action`、`project_id`、`user_id`、`tenant_id`、`adapter_id`、`locale`。action 为 `connect|status|stop`；三个身份 ID 为32位小写 hex；语言为 `zh-CN|en-US`。页面仅在平台为 `win32|darwin|linux`、版本为 `1.0.0` 且原生函数实际存在时使用该能力。

成功结果为 `{status:"ok",connection}`。connection 仅包含准确 `project_id,adapter_id,phase`，以及可选 `worker_id,base_commit,expires_at,repository_name`；worker 为 `desktop-` 加32位小写 hex，完整提交为40或64位 hex。失败为 `{status:"failed",code}`。本机路径、token、命令、环境和私有目录不进入此连接接口的页面结果。

phase 闭合为 `unconfigured|needs_workspace|not_installed|unavailable|prepared|issuance_unknown|starting|connected|stopping|stopped|authorization_required|binding_changed|origin_changed|credential_unrecoverable|stop_unconfirmed|failed`。安装、启动、连接、工具档案登记、启用、供应方授权及任务结果是分别成立的事实。

`connect` 必须由用户明确点击；恢复仍使用同一作用域和本机原记录。`status` 不发行凭据、不启动 Worker；核对发现失权或绑定变化时沿停止路径处理。`stop` 只请求停止本机节点，不代表任务已完成、已取消或已验收。强停或清理未确认保持 `stop_unconfirmed`。

## 原操作与发行

主进程核验准确主窗口／主 frame／固定服务 origin，并在受保护读取前后确认真实本人、组织和项目。通过私有 `/api/v1/desktop-runners/context` 获取当前绑定摘要、目录身份、实例身份与真实 Git HEAD；仅主进程接收路径。

在任何发行 POST 前，主进程将原随机 token、token SHA-256、节点名、稳定操作 ID、实例／origin／身份／项目／adapter／目录及完整提交写入 OS 加密 journal。`/desktop-runners/register` 只接收摘要，不接收或返回 secret。服务端在同一当前授权事务内保存原独立 Grant、凭据 slot／digest／scope、节点、原回执和活动；任一失败整批回滚。

原 Grant 有效期24小时。原回执只证明历史发行：即使过期也可保存原历史结果，启动仍须核验当前节点状态和有效期，不能因回执成功延长授权。未知发行核对原请求与操作，不换 token、不换操作 ID；凭据不可恢复、授权到期或绑定变化均停止继续启动。明确续权与原连接处置的完整交互仍待交付。

## 本机执行与连接事实

仅固定 control 程序接收私有 stdin 首帧，协议为 `suma-desktop-managed-runner-v1`，准确九字段为 `protocol,control_url,project_id,worker_id,adapter_id,repository,base_commit,state_directory,token`，上限16KiB。拒绝多余、重复或不匹配字段；secret 不进入参数、继承环境或配置导出。

必须是已明确绑定的真实 Git root、可解析的完整提交，以及 Git 忽略的 `.cache/runner/worktrees`。缺少目录时复用原生目录选择及当前绑定合同；不自动初始化仓库、提交文件或修改忽略规则。Go 重新发现固定工具，使用原 Worker、fencing、持久 journal／outbox 和原 DAG 运行。

仅本次 spawn 后、超过启动前库存水位的新鲜报告，且准确匹配当前工具协议及固定 Profile，才报告 `connected`。原报告有效期仍为60秒，不因读取延长。随后151登记保持暂停，启用仍需独立确认。主进程每15秒核对当前会话、目录绑定和节点资格，发现失权或变化时停止节点。

## 存储、地址与待验边界

journal 位于应用私有 userData，使用 Electron `safeStorage`、独占所有者锁和原记录校验；Linux 的 `basic_text` 或未知存储后端不接受为安全凭据存储。Runner 状态单独保留供原运行核对，不能通过清理目录解决未知副作用。

本机服务默认 `127.0.0.1:4319`，可在桌面启动进程设置 `SUMA_DESKTOP_PORT` 为1024–65535。端口被占用即失败，不随机切换；原 journal 的显式 origin 不匹配时拒绝复用。

Windows Electron35.7.5软件离屏环境已限定验证真实 DPAPI、生产窄IPC、HMAC摘要发行、正式 Go Worker、启动后新鲜库存、暂停登记／独立启用、停止、同节点原操作恢复、整应用正常重开及退出登录后停止；17项实际检查与来源摘要见[验证记录](../docs/quality/desktop-managed-runner-verification.md)。全程仅合成 CodeBuddy 兼容的只读探测，0任务／0运行／0模型调用；首次 GUI 因页面拒绝小数秒时间失败，修复后沿原节点和 journal 续验。

目录签名与 Go 绑定是真实实现，OS 目录对话框由 fixture 固定返回自有 Git root，未验真实系统选择器界面。真实供应方安装／登录／开发、强制崩溃恢复、macOS／Linux凭据存储、安装包、多主机、规模及生产仍待验；24小时续权的完整交互仍待交付。
