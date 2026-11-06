# 终端执行尝试与工具事件核对

速码 1.0.0，`T-CLI-ATTEMPT-HISTORY-262`。关联需求为分布式执行观察、异常核对和中英文 CLI。 [终端操作](../operations/task-dispatch-options.md#核对某次执行尝试) · [调度合同](../../contracts/scheduling-kernel.md)

## 设计与范围

终端复用工作台的持久尝试列表、单次尝试和按尝试分页的工具事件读取。列表与详情核对准确运行和节点，工具事件还核对项目、尝试号、fence、执行者和分页窗口。读取继续沿当前项目授权。未登记到具体 Worker 的内部子流程尝试保留原记录语义。

参考 Multica 在问题下将运行状态、运行历史和详细日志分别呈现的方式。速码沿现有 DAG 持久记录和工具事件游标提供终端诊断。 [Multica 固定参考](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/packages/views/issues/components/execution-log-section.tsx#L48-L64)

## 阶段验证

- Windows amd64、Go 1.26.10。实际编译的 `suma.exe` 通过回环 HTTP 读取真实控制面和隔离 SQLite 中的持久记录。调度器实际创建两次尝试并写入三条测试进度。7 次终端调用各发起一次 GET。列表正确续页，详情读取原失败结果与执行者，工具事件沿固定水位只返回原尝试的两条记录。错误尝试返回 404，跨项目凭证返回 403。各次调用前后业务数据库指纹一致。
- 定向 CLI 测试覆盖请求参数、项目和原会话范围、错误响应、内部子流程尝试及分页游标。中英文帮助与常见参数错误使用现有 CLI 语言目录。
- 原始证据保存在本地忽略目录 `.cache/q262/cli-actual/`。本阶段验证真实控制面读取和调度器持久化，事件来自隔离测试任务，外部智能体、模型调用和跨机器恢复未实测。
