# T-P4-004 核心原始记录

负责人：主集成。日期：2026-10-24。关联：[阶段验收](../../tool-execution-verification.md)。

正式最终全套在父目录`tool-go-post-review-tests.jsonl`与`tool-go-post-review-verification.json`：512个通过测试事件，0失败、0测试跳过，vet通过。初轮`tool-go-tests.jsonl`与`tool-go-verification.json`的479切面保持原字节。本目录保留两次原汇总及工具合同、真实停止、绑定投影、TaskDispatch、生成器的关键RED→GREEN。`core-platform-all-initial.jsonl`属于旧worker测试身份错误的失败切面；不能替代最终全套。`contracts-build-first-failure.txt`是普通生成器构建的原始失败，后续生成与漂移检查成功。

文件来源、大小和摘要见[根归档映射](../tool-root-archive.json)及[最终Go归档](../tool-post-go-archive.json)。`collect-final-tool-evidence.py`仅从独立本机数据库只读投影公开字段，不含账号密码、token、master key、配置环境或数据库副本；正式快照与进程验证分别位于父目录。归档采集器保留原始相对目录语义，供审阅；复跑需放回记录中的原位置并提供自己的独立测试数据，源码仓库不分发私有数据或凭证。
