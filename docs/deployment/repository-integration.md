# 受管代码集成服务部署

状态：1.0.0，T-P4-028 实现；负责人：平台集成；需求：已验收候选进入受控目标分支。实际验证范围见[验收报告](../quality/repository-integration-verification.md)。

## 进程与信任边界

复用 Go、Git、HTTP、持久 DAG 与现有 Runner。`suma-repository-provider` 是管理员维护的独立进程，管理预先指定的裸仓库和分支。当前控制面支持正常证书验证的HTTPS提供方地址及明确配置的字面回环IP HTTP地址，不使用代理或重定向；提供方命令本身只监听回环地址，跨主机部署需要管理员管理的HTTPS入口。协议支持与实际TLS、身份联邦及容量验收分别记录。Runner 通过现有项目或 Pool 凭证领取固定信封，不能提交任意仓库路径。Provider 配置不代表拥有业务项目权限。

`scripts/build-runtime.ps1` 将新进程纳入既有运行时构建清单。它可以独立部署；浏览器、CLI 和桌面业务仍共用控制面。开发构建可执行 `go build ./cmd/suma-repository-provider`。跨平台构建成功与实际安装、签名、启动验收分别记录。

## 配置顺序

1. 使用独立裸仓库，先备份 Git 对象和引用；确认目标 `refs/heads/main` 已存在。此服务不会修改普通工作树或向远端 push。
2. 为回调、Runner 访问、控制面核对分别设置三个不同的进程环境密钥，至少 32 字节。配置只填写环境变量名。不要将值写入文档、命令行、日志或 Git。
3. 管理员保存 Provider JSON，固定项目、Git 可执行文件、裸仓库和分支。为服务使用独立持久目录及独立系统账户，限制本机配置和仓库写权限。
4. 以 `--config <配置文件> --state <持久目录> --addr 127.0.0.1:4351 --language zh-CN` 启动。`--language en-US` 提供英文命令帮助；进程错误保留机器代码。
5. 在控制面的 `SUMA_REPOSITORY_PROVIDERS_JSON` 登记同一受众、提供方、目标别名、地址及回调／核对环境变量名。启动时严格检查，不接受重复受众、别名或复用密钥。
6. 在 Runner 的 `--external-config` 登记固定服务，并在该进程中提供其独立访问密钥。Pool 使用本项目的显式执行范围配置；通用外部执行和发布权限不会自动继承。
7. 项目批准者从任务详情的“集成已测试候选 → 管理目标绑定”绑定预置别名。界面读取实际分支、配置摘要和可用性，不提供任意路径或凭据输入。

Provider 配置示意（占位标识需要替换）：

```json
{
  "provider_id": "repository-local",
  "tenant_id": "organization-id",
  "project_id": "project-id",
  "control_url": "http://127.0.0.1:4324",
  "control_token_env": "SUMA_REPOSITORY_CALLBACK_TOKEN",
  "runner_token_env": "SUMA_REPOSITORY_RUNNER_TOKEN",
  "reconcile_token_env": "SUMA_REPOSITORY_RECONCILE_TOKEN",
  "git_executable": "C:/Program Files/Git/cmd/git.exe",
  "targets": {
    "main": {"bare_repository": "D:/suma-data/repositories/project.git", "ref": "refs/heads/main"}
  }
}
```

控制面登记条目使用 `id`、`tenant_id`、`project_id`、`target_aliases`、`origin`、`token_env`、`reconcile_token_env`。Runner 条目示意：

```json
{
  "repository-local": {
    "url": "http://127.0.0.1:4351",
    "token_env": "SUMA_REPOSITORY_RUNNER_TOKEN",
    "protocol": "suma.repository-integration.v1"
  }
}
```

应用固定源码检查另使用独立可选`source_read_token_env`，不是上述Runner或核对密钥的别名；未配置禁止源码读取。两端配置、固定Node解析器和限额见[应用源码检查部署](application-source-inspector.md)。仓库集成权限不自动授予应用目录写入或版本登记权限。

## 停止、备份与核对

正常发送停止信号，等待 HTTP 排空、持久状态关闭和退出回执。保留控制面事务库、Provider 状态库、完整裸仓库对象／引用、制品原件及管理员配置的匹配快照。不要只恢复 SQLite 后覆盖 Git 引用，也不要清理未知操作的对象或回执。

结果未知时，保留目标屏障，从原任务计划执行“核对原操作”。读取缺失回执不能判为“没有执行”：应用阶段必须在当前专用核对授权下，以同一个 Git 事务写入负向回执与 fence。准备阶段可由当前核对授权永久关闭原操作，不更改目标分支；新准备必须建立新计划。禁止手工改业务状态、数据库租约或 Git receipt 来放行。

真实差异超过 512 KiB、超过 1,000 文件、不支持的二进制审查、目标分叉、配置变化均明确阻止。目标已变化时重新开发并独立测试候选，再建立审批；当前交付不合成 merge/squash 结果。详见[正式合同](../../contracts/repository-integration.md)与[架构及 AI SOP](../architecture/repository-integration.md)。
