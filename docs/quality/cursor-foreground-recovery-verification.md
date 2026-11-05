# Cursor ACP 前台停止与原尝试核对

T-CURSOR-RECOVERY-228，速码 1.0.0，2026-11-05。基线 `fd9ce459ada398ed1c05a0d0fffa8463d632fe7e`。 [执行合同](../../contracts/cursor-acp-execution.md)

Cursor 的前台输入关闭后，Runner 继续使用原协议、工具来源和进程控制器。停止请求返回后，原生 `Wait` 另有两秒等待上限。如果进程结果仍未出现，Runner 报告原尝试的未知结果，保留工作区供核对。这条路径不读取尚可能变化的进程状态、输出或工作区，也不产生成功候选。已取得退出结果的路径继续验证原 RPC、工具来源和停止事实。

前台停止未知时，浏览器从当前运行的固定工作流识别 Cursor。操作者核对原执行节点的进程树和任务影响后，选择实际结果并记录依据。历史尝试用于查看证据。提交沿原运行版本和幂等键恢复。

## 本轮证据

- Runner 四个定向检查通过。新增回归在修复前的两个分支均卡住，修复后均按边界返回。受控进程回归覆盖正常关闭、帮助探测、输出截断、RPC 错误、安装来源变化、取消和原生非零退出。命令及结果位于 `.cache/cursor-recovery-228/focused.json`、`focused.log` 和保留的 `red.*`。
- 普通节点和共享执行池通过原 HTTP、SQLite 与真实 Journal 的回执故障测试。控制面先接受未知结果，测试拦截第一次回执。重开 Journal 后重放同一请求字节，运行版本与原执行代次不变，开发节点保持待核对，独立测试不领取，原账本仍只有一条待核对记录。首轮测试把运行总状态误当节点状态，修正断言后两组通过。命令和日志位于 `.cache/cursor-recovery-228/go/`。
- Web 三个相关文件共五项通过，类型检查通过。中英文 10,036 对词条通过，生产硬编码 0。覆盖当前尝试、历史尝试、未选择时禁用、明确结论与原请求回放。
- Chrome 在 390×844 下验证了实际工作流组件和共享弹窗。弹窗完整可见，内容无横向溢出，提交按钮等待结论和证据。验证使用本地合成只读响应，写入和浏览器错误均为 0。首轮夹具误将事件流返回为 JSON，修正响应后复用同一构建通过。截图及记录位于 `.cache/cursor-recovery-228/ui/`。
- Linux amd64 Runner 交叉构建通过。 [验证摘要](cursor-foreground-recovery-summary.json)

## Multica 参考与后续

本轮核对了公开 `main` 的固定提交 `f230f9f55bdfc4de68015473b07de329a5079f87`。上游分别检查进程退出和进程清理回执，在守护进程重新登记后按原 runtime 归属处理孤儿任务。速码沿现有 Runner、Journal 和运行授权实现本轮行为。上游附加许可条款按固定版本记录。 [公开提交](https://github.com/multica-ai/multica/commit/f230f9f55bdfc4de68015473b07de329a5079f87) · [停止回执源码](https://github.com/multica-ai/multica/blob/f230f9f55bdfc4de68015473b07de329a5079f87/server/cmd/multica/cmd_daemon_shutdown_result.go#L12-L44) · [许可证](https://github.com/multica-ai/multica/blob/f230f9f55bdfc4de68015473b07de329a5079f87/LICENSE#L20-L88)

下一段优先统一桌面与 Worker 的关闭回执，把原进程身份、清理结果和持久化结果接到现有停止状态。真实 Cursor 账号与供应商执行、Linux 实机、macOS、物理多机和原 31 项平台验收仍在范围内。
