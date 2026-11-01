# Multica 运行中协作采用记录

T-TURN-STEERING-188；suma1.0.0；负责人：主集成。当前交付以[合同](../../contracts/turn-steering.md)、[架构](../architecture/turn-steering.md)、[交互](../design/turn-steering.md)与阶段限定验证为准，不改写其他阶段的历史源码采用记录。

本轮本地参考锁定 `D:\codes\github\multica` 提交 `5a2cc891ac338eed1919aa11a1eef5957cd82cec`，工作树未修改。参考 `server/pkg/agent/codex.go` 的同一活回合追加输入、`server/internal/daemon/task_supplement.go` 的就绪后串行投递，以及任务评论旁的明确发送和送达事实。Web独立勘察的其他提交只作为其单独证据，不与此本地SHA合称同一来源。

Suma复用自身现成的Go DAG、受保护Assignment、当前身份事务、原命令回执、任务讨论、Runner Journal、单stdin writer、React组件和i18next。原native `turn/steer` 参数与ACK按本机固定CLI Schema和[官方协议](https://learn.chatgpt.com/docs/app-server#steer-an-active-turn)独立实现；没有引入新的SDK、派发器、客户端预算账本或私有配置开关。

采纳明确评论来源、准确活目标和串行投递；写入、原生接受、代码效果、独立测试和人工验收分别记录。未知副作用不自动重答，不把超时重试、另开回合或正常触发fallback带入。既有评论仅保存消息，用户另行选择原目标并明确发送。

本轮未复制Multica源代码、CSS、品牌、图标或商用部分。原仓库LICENSE包含Apache文本及附加条件，采用方式沿[原许可与复用记录](multica-adoption.md)、[既有改编清单](multica-reuse.md)逐项记录。本能力不表示Multica全部工具、团队运行、会话恢复与SaaS能力已完整交付；完整31项与P0–P7继续。
