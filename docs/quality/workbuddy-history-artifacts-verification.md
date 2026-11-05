# WorkBuddy 会话产物回放验证

T-WORKBUDDY-HISTORY-ARTIFACTS-237 · 速码 1.0.0 · 2026-11-05。 [会话合同](../../contracts/workbuddy-cloud-collaboration.md) · [证据摘要](workbuddy-history-artifacts-summary.json)

原授权会话通过 `session/load` 收到的已识别产物通知，按本地序号保存在本人历史中。页面显示产物类型和变更动作。只保存两个闭合枚举，历史覆盖继续标记为未核实。载入原记录沿用当前项目和所有者权限，分页只读取持久数据。

| 验证 | 结果 |
| --- | --- |
| Go | 31 项相关测试通过，另有两项须显式启用的历史场景跳过。回环 HTTP／SSE 与 SQLite 核对原会话归属、入库、回读、无额外连接、文本额度和分享拒绝。闭合合同及生成器定向检查通过。 |
| Web | 三份相关测试文件共 41 项通过，覆盖历史产物投影、分页和分享选择。Web 类型检查及 10,279 对中英文词条通过。 |
| 浏览器 | Chrome 156、390×844，实际组件使用受控接口。英文深色及中文浅色均通过键盘分页，两条产物记录只有元数据，49 条正文仍可选择分享。共六次接口读取，零写入、页面错误及横向溢出。首次夹具中文断言用词不同，更新断言后复用原构建。 |
| 合同生成 | 原生成器核对 1,039 个模型、467 个 API 路径及 TypeScript 合同通过。 |

采用 Multica 的类型化、按序渲染记录方式，复用速码的原 ACP 解码、授权和历史服务。WorkBuddy 文档描述了会话产物通知和独立沙箱列表接口，本阶段只使用原会话收到的通知。实际供应商回放覆盖、沙箱列表接入、可信 Git 候选、同提交独立测试和真实多机仍待独立验证。 [Multica 记录序列](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/packages/views/common/task-transcript/build-timeline.ts#L4-L20) · [WorkBuddy Open API](https://open.workbuddy.cn/docs/openapi)
