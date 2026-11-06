# Runner 工具库存指纹校验

速码 1.0.0，`T-RUNNER-EXECUTABLE-DIGEST-266`。关联需求为开发工具快速接入、共享执行节点与派发稳定性。 [执行合同](../../contracts/tool-execution.md#administrator-profiles-and-live-observations)

Multica 将工具发现与独立执行探测分离。速码在现有 Runner 内按一次库存扫描共享相同路径的主程序指纹，各配置独立核对自身协议和能力。发起任务时使用独立的执行前校验。 [Multica 固定参考](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/server/internal/daemon/agents_refresh.go#L10-L62)

## 验证记录

- Windows 与 Go 1.26.10。11 项定向测试通过，覆盖读取上限、文件增长或截断、取消、相同大小和时间戳的文件替换、并发复用、检测到变化后拒绝本轮复用、新扫描重新计算、库存能力和执行前拒绝配置漂移。当前 Windows 账号缺少创建文件符号链接的权限，相关两个子用例跳过。
- 使用编译后的 `suma-runner --describe-tools` 只读探测实际安装的 `codex-cli 0.166.0-alpha.6`。两个命令配置均可用，主程序 SHA-256 一致，配置 SHA-256 不同。验证范围是版本、帮助与配置库存，无模型或任务调用。
- Linux amd64 的 Runner 测试程序交叉编译通过，尚未在 Linux 主机运行。证据保存在本机忽略目录 `.cache/q266/`。
