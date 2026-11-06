# 节点诊断中的已关闭结果

速码 1.0.0，`T-RUNNER-CLOSED-RESULT-INSPECTION-273`。关联需求为分布式运维、结果回执与中英文 CLI。 [诊断合同](../../contracts/runner-journal-inspection.md) · [值班指南](../agents/runner-journal-inspection.md)

参考 Multica 分别统计待发送和失败终态回执的做法。速码在原只读日志摘要中分别展示待回执、已回执和原尝试关闭后留在本机的结果。总览提供每槽位计数，原槽位命令提供脱敏的逐条标记。 [Multica 固定参考](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/server/internal/daemon/terminal_report_queue.go#L566-L585)

## 阶段验证

- Windows、Go 1.26.10。12 项 Go 定向测试通过。覆盖矛盾的拒收标记、改变的执行身份、混合槽位、只读和参数冲突。当前 Windows 账号无符号链接权限，相关用例和子用例跳过。
- 编译后的 `suma-runner` 实际读取受控合成槽位日志。三类结果各 1 条，逐条标记保留原运行定位，输出无原载荷或操作标识。两种语言的总览 JSON 一致，诊断前后文件摘要一致。
- 6 项 Web 测试、TypeScript 和中英文词条检查通过。Chromium 粘贴同一份真实 CLI 输出，在中文 390 像素与英文 1280 像素核对键盘操作、三类计数、准确槽位命令和横向布局。浏览器验证使用本地组件夹具，实机多节点和跨平台界面仍待验收。证据在 `.cache/q273/`。
