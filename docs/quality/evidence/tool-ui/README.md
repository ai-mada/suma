# T-P4-004 前端公开证据

范围：组件／模型测试、web 类型检查、只读格式检查和源码切面；实际浏览器与执行旅程由主集成单独记录。完整解释见[前端验证报告](../../tool-ui-verification.md)。

## 当前结果

- [web189](ui-web-lifecycle-link-final.txt)、[类型](ui-typecheck-lifecycle-link-final.txt)、[18 份当前源码](ui-lifecycle-link-verification.json)。
- [Prettier 3.6.2](ui-final-format-version.txt)、[13 份新文件检查](ui-final-format-check.txt)、[已跟踪 UI 空白检查](ui-final-whitespace-check.txt)、[格式收据](ui-final-format-verification.json)。
- [逐字节来源映射](archive-map.json)、[归档复核](archive-verification.json)。

## 历史原件

183、185、186 项测试和对应源码 JSON 均是各自时间的切面，不是当前源码的另一份等价验收。原始收据中的旧 `.cache` 路径、178 项过程备注和前代摘要全部保留，不能据此覆盖顶层最终数量或真实原始日志。

| 组别 | 文件 |
|---|---|
| 初轮行为 | [RED12](ui-red.txt)、[GREEN12](ui-model-green.txt) |
| 终态等待输入 | [RED](ui-waiting-terminal-red.txt)、[GREEN](ui-current-green.txt) |
| 独立测试前置 | [RED](ui-task-prerequisite-red.txt)、[GREEN](ui-prerequisite-green.txt) |
| 固定绑定与当前资格 | [RED](ui-binding-status-red.txt)、[GREEN](ui-binding-status-green.txt) |
| 观察原因边界 | [RED](ui-inventory-reason-boundary-red.txt)、[GREEN](ui-inventory-reason-boundary-green.txt) |
| 任务结构化协议 | [RED](ui-task-protocol-boundary-red.txt)、[GREEN](ui-task-protocol-boundary-green.txt) |
| 精确需求路由 | [RED](ui-requirement-link-red.txt)、[GREEN](ui-requirement-link-green.txt) |
| 预留编号与已启动事实 | [RED](ui-reserved-plan-red.txt)、[GREEN](ui-reserved-plan-and-link-green.txt) |
| 初轮类型 | [原失败](ui-typecheck-first.txt)、[当时修复](ui-typecheck-green.txt) |
| web183 历史切面 | [web](ui-web-final.txt)、[类型](ui-typecheck-final.txt)、[源码](ui-source-verification.json) |
| web185 历史切面 | [web](ui-web-reason-boundary-final.txt)、[类型](ui-typecheck-reason-boundary-final.txt)、[源码](ui-reason-boundary-verification.json) |
| web186 历史切面 | [web](ui-web-task-protocol-final.txt)、[类型](ui-typecheck-task-protocol-final.txt)、[源码](ui-task-protocol-verification.json) |

34 份原始输出与 JSON 均从明确允许清单逐字节复制，没有重新格式化或覆盖。只读格式检查没有修改应用源码；未复制实际凭证、密钥、数据库、控制面日志或现场浏览器数据。秘密模式检查仅用于归档材料，不宣称已审计全部仓库或生产环境。
