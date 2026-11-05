# Codex 开发候选与独立验收

在任务派发时选择已登记的 Codex App Server、模型、推理强度和可选用量提醒阈值，再选择独立测试工具。速码保存本次运行的配置。运行详情沿这次派发的工作流版本展示固定选择，匹配当前节点和尝试的 CLI 回执到达后，再显示确认结果及超阈值提醒。

用量提醒随 CLI 上报更新。缓存写入计数缺失时，保留已知的小计并继续记录部分用量。此配置的计费模式为 `observe_only`，阈值用于提醒。 [Codex App Server 文档](https://developers.openai.com/codex/app-server/)

开发结束后，Runner 核对本轮完成回执和进程树停止证明，从独立工作树封存实际改动。父进程使用受保护的 Git 路径和原封存检查点，候选绑定同一运行和尝试。新增文件须通过原 Git 忽略规则。发现已忽略的新增文件、独立暂存状态或忽略规则改动时，保留工作区供复核。独立节点导入已封存的提交，执行固定测试并上报 JUnit。测试通过后，具备权限的复核人仍须在任务验收入口提交结论。

进度回执丢失时，恢复原操作标识、请求体和执行代次。封存副作用不明时，保留原提交引用及检查点供核对。取消后在等待上限内无法收集原进程结果时，报告未知结果并保留原工作区。进程停止、封存、独立测试、验收和基线采用分别显示。

## 执行节点

Windows 使用现有 Job 对象。Linux 节点可在 Runner 服务环境中设置 `SUMA_RUNNER_LINUX_PID_NAMESPACE=1`。部署环境需允许创建用户与 PID 命名空间及使用 pidfd。工具在私有 PID 命名空间中以 PID 1 运行，保留执行节点原 UID 和 GID。内核结束原命名空间并回收后代后，Runner 才取得整树停止证明。该能力负责进程生命周期。文件权限、网络隔离和挂载策略由执行环境另行配置。 [Linux PID 命名空间](https://man7.org/linux/man-pages/man7/pid_namespaces.7.html)

Linux 运行验证需在符合上述条件的节点执行。未配置的 Linux 和 macOS 暂停需要整树停止证明的工具探测。启用前应执行文档所列的进程收尾检查。 [阶段验证](../quality/codex-candidate-delivery-verification.md)

## English operator guide

Choose the registered Codex App Server, model, reasoning effort and optional token notice when dispatching. Select an independent test tool in the same plan. Run details show the settings from the workflow revision that launched the run. A matching receipt from the current node and attempt adds the CLI acknowledgment and any threshold notice. The threshold operates in observe-only mode.

After the original turn completes and its process tree stops, the Runner seals the worktree edits using the existing Git checkpoint. New files must pass the baseline Git ignore rules. Changes to ignore rules require review. A separate worker imports and tests the sealed commit. The JUnit result opens the review gate, then an authorized reviewer records the decision on the task. Lost progress acknowledgments replay the original operation. An expired wait during cancellation leaves the outcome unknown for reconciliation.

On Linux, configure `SUMA_RUNNER_LINUX_PID_NAMESPACE=1` on a worker whose host permits user/PID namespace creation and pidfds. The tool runs as namespace PID 1 with the worker's existing UID and GID. Use the deployment's filesystem and network isolation as appropriate. Live Linux qualification and macOS containment remain pending. [Verification](../quality/codex-candidate-delivery-verification.md)
