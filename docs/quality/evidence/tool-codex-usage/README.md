# Codex用量协议版本补充

这是初次111项域内回归与初次045219交付之后的独立源码切面；保留旧证据原字节，不重写历史结论。

- [机器收据](codex-usage-verification.json)：官方旧版三字段与新版五字段定义、归一化边界、三个Go文件摘要及目标测试统计。
- [RED原文](codex-three-counter-red.jsonl)：真实三计数解析与本机exit0夹具进程错误拒绝，共12通过／4失败事件（包含失败父项）。
- [GREEN原文](codex-three-counter-green.jsonl)：16通过／0失败／0跳过，包含三／四／五字段形状、30组错误类型及非负整数边界。
- [文件摘要](files-sha256.json)：覆盖本目录除摘要文件自身外的公开文件。

进程是明确标注fixture-transport-only的本机测试程序，没有启动真实Codex任务、模型、登录或厂商API。本机0.164只读version/help另见相邻probe档案，不能据此推断它的实际任务输出计数形状。缺拆分不报告零；既有omitempty也省略显式零拆分，不宣称完整保留全部presence差异。

主集成另行运行完整Go与新运行包；目标回归不替代那些验收。
