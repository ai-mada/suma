# Multica Cursor ACP 采用记录

T-CURSOR-ACP-189；suma 1.0.0。以[合同](../../contracts/cursor-acp-execution.md)、[架构](../architecture/cursor-acp-execution.md)及[专项验证](../quality/cursor-acp-execution-verification.md)为准；本记录不改写已有历史源码采用清单。

本轮参考本地锁定 Multica 提交 `5a2cc891ac338eed1919aa11a1eef5957cd82cec` 的统一 CLI 派发与生命周期。其 `server/pkg/agent/cursor.go` 使用 headless stream-json 和 yolo，并非 ACP；速码独立选择已有 ACP SDK 路径，不复用其 yolo。实际探测来源由专项报告逐项记录；不把旧版任务分组代码的许可来源当作当前 Cursor 适配的许可。当前实现沿速码自己的 Go DAG、原 Runner 与质量链完成。

| 参考目标 | suma 采用方式 |
| --- | --- |
| 一个入口接入工具 | 原工具快捷面板与 ACP 消费者，公开 ID固定为 cursor-cli |
| 统一 CLI 派发与生命周期 | suma 独立选择原 acp-go-sdk 0.13.5；严格帧、准确模型配置 ACK及当前范围 |
| 同一工具生命周期 | 原库存、Profile、Task／Discussion／Expert Preview、Start、普通／Pool Claim与 Result |
| 人工参与执行 | 原 session/request_permission 的一次选择，不默认同意或扩大持久权限 |
| 开发交付 | 原隔离工作树、Git 候选、同提交独立测试与当前账号 Review |

没有复制 Multica 的 Cursor 实现、商用部分、品牌、图标或样式；没有将厂商安装包、打包 Node 或依赖文件加入源码。没有带入 yolo／force、自动批准 MCP、宽松帧解析、模型回退、未知 Prompt 重试或旧会话自动恢复。原许可原则见[采用记录](multica-adoption.md)和[历史改编清单](multica-reuse.md)。

协议与宿主依据为当前实际包观察、原 SDK和[官方 ACP](https://cursor.com/docs/cli/acp)、[参数](https://cursor.com/docs/cli/reference/parameters)、[配置](https://cursor.com/docs/cli/reference/configuration)、[权限](https://cursor.com/docs/cli/reference/permissions)与[第三方 hooks](https://cursor.com/docs/reference/third-party-hooks)。本机检查确认固定 Node／index.js 包和附加配置来源后，由 Go 生成私有启动参数；不根据文档编造已连接状态。

真实厂商证据限于 Windows 包版本／帮助／initialize，以及未认证 session/new 的明确拒绝；没有厂商账号或 Prompt。完整开发链使用明确非厂商的受控原生宿主，通过原严格包核验与 SDK，实际产出 Git 候选及独立 Node／JUnit，并由隔离夹具账号 Review。二者不能合称真实厂商模型开发已验收，也不表示 Multica 全部工具、持久会话或完整31项已经交付。
