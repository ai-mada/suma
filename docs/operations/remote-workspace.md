# 远程工作区只读观察 AI SOP

状态：T-P4-037／1.0.0，2026-10-27。负责人：Control／Runner运维与当前项目操作人。接口与限额以[合同](../../contracts/workspace-observation.md)为准，内部结构见[架构](../architecture/remote-workspace-broker.md)。本SOP不声明GUI或生产验收完成。

## 使用前提与操作

1. 使用当前项目的真实浏览器会话或项目API凭据，核对当前用户和项目。观察绑定原凭据；重新登录的新会话不能读取旧请求。
2. 从当前目标列表选择仍在运行的原生attempt，保留完整scope。空列表不是宿主路径输入入口，不为观察创建无关运行或重放历史任务。
3. 核对工具档案审批与真实库存；Pool另有当前双边授权和机器凭据。只读观察不需要模型调用。
4. 明确读取目录或相对文件，保留观察时间、原文件SHA、分页状态及请求ID。后续页可传 `expected_sha256`，文件变化时重新读取，不合并不同版本内容。观察完成不等于候选、测试或验收。

CLI使用相同控制面API：

```text
suma --project PROJECT workspace targets
suma --project PROJECT workspace observe --file observation.json --wait
suma --project PROJECT workspace observation REQUEST_ID --wait
```

JSON使用当前API返回的scope、`operation`及封闭 `arguments`；恢复原操作时保留同一操作ID与正文。CLI不连接宿主文件系统或工作区MCP。不要把Bearer、宿主绝对路径、私有日志或模型密钥放进JSON、截图或交付材料。

请求最长60秒、观察租约10秒。等待状态表示队列／读取进度，不表示模型健康。页面metadata检查仍做当前授权；失效时清空已显示内容，不能用缓存正文掩盖撤权或宿主关闭。

机器空轮询返回 `lease:null` 时仍已核验当前权限，但不新增操作回执。同key后续可领取新观察；实际领取后的原key和ACK才是需要恢复的准确效果，不将空轮询当成持久冻结的队列状态。

## 异常处理

| 事实 | 处理 |
| --- | --- |
| `workspace_unavailable`／`workspace_closed` | 核对当前attempt和宿主生命周期；不重建旧句柄，只对当前新目标另建观察 |
| `path_not_allowed`／`sensitive_content`／`text_required` | 保留拒绝事实；不放宽路径策略，不涂改正文后冒用原SHA |
| `cursor_stale`／`file_changed` | 丢弃跨版本分页，读取当前版本并重新固定摘要 |
| `output_limit` | 缩小目录页或行数，保持原生限额 |
| 401／403 | 核对原调用凭据及当前项目／Pool授权；不借另一会话读取旧请求 |
| 409／`expired`／`cancelled` | 核对scope和租约；有效只读请求可由控制面用更高观察fence重领，旧结果不可强制写回 |
| ACK不确定 | Runner journal重发原准确回执，不重读变化后的文件，不重启DAG或模型 |

Control后台每轮最多清除100项到期正文。区分“当前不再披露”和“本地私有恢复文件已物理清理”：Runner journal另有容量与后续checkpoint裁剪规则。清理本地证据遵循项目目录卫生要求，先保留必要审计摘要，不删除活动工作区或唯一用户文件。

## 必要验证

只对受影响边界做定向检查：[Control](../../internal/platform/workspace_observation_test.go)、[真实Worker联合入口](../../internal/platform/workspace_observation_runtime_test.go)、[Runner恢复](../../internal/runner/workspace_observation_worker.go)。联合夹具使用真实原生子进程和工作树，但无模型调用、无正式任务测试；GUI、中英文、键盘、失效清空及实际部署各自记录，不用接口200代替。

不为只读观察重跑历史模型链路、改写主预览数据库或接通终端。远程Git、写入、PTY、LSP／DAP及物理多机／TLS仍需各自设计、授权和验收。
