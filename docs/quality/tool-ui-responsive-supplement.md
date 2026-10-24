# 窄屏运行页：节点操作与工具栏补充验证

状态：T-P4-004 窄屏 UI 源码、消费者／模型回归、类型和格式检查通过；最终浏览器布局、点击、焦点、取消操作与桌面视口恢复由主集成代理单独验收。日期：2026-10-24。负责人：UI 实现代理。前代报告：[工具工作台验证](tool-ui-verification.md)。

## 问题与改动

主集成提供的[修复前 390 原始截图](evidence/tool-ui-responsive/before-stop-cdp-390.png)是诊断输入。随后主集成确认该 PNG 受 IAB 宿主 1.5 倍 backing 表面与捕获截幅影响，不能单凭它断言运行选择区或工具栏在实际 DOM 中裁切。确定的产品缺口是窄屏仍以约 600px 画布作为默认、缺少完整可操作节点列表。原图保持原字节，不作为控件裁切或修复后的视觉证据；真实 DOM 矩形、完整视口捕获和操作由主集成单独记录。

前一版对截幅图的诊断表述依据不足，已在本段纠正；[前版原文及其原摘要](evidence/tool-ui-navigation/tool-ui-responsive-supplement.original.md)作为历史保留，旧归档收据中的报告摘要对应此前版本。[更正记录](evidence/tool-ui-navigation/diagnostic-correction.json)区分原快照与本次更正，没有修改原图或旧归档。

本次只改动 [Workflow.tsx](../../apps/web/src/Workflow.tsx)，新增 [ResponsiveWorkflow.tsx](../../apps/web/src/ResponsiveWorkflow.tsx)、[响应式样式](../../apps/web/src/responsive-workflow.css)和[消费者／模型测试](../../apps/web/src/responsive-workflow.test.tsx)。没有修改 Go、共享合同、正式运行状态或先前归档。

- 以实际 `matchMedia('(max-width:850px)')` 切换。运行与诊断模式在窄屏显示完整节点列表，不挂载占位大画布；桌面继续使用 React Flow。编排模式保留原编辑入口。
- 运行名称、完整运行 ID、正式状态、选中节点及当前尝试独立显示，长内容可以换行。两个选择器使用可收缩的宽度；工具栏动作换行，恢复窄屏下暂停／取消的文字与状态标记。
- 节点使用原生按钮，名称、状态、尝试、fence 和正式原因来自当前 Run。完整范围可按名称、ID 或状态筛选；没有只取前 100 项。未领取节点不冒充已经开始了第 0 次尝试。
- 选择节点进入原有详情与核对操作；返回列表将焦点放到可见列表入口。定位依赖节点／运行选择变化，相同节点的状态刷新不会据此重复定位。实际滚动与键盘焦点仍需浏览器验证。
- 当前组件内最多保留 32 个上下文的图视口，窄屏列表与桌面图切换时复用对应视口。没有新增自动恢复、重试、审批、测试通过或验收行为。

## 实际执行的检查

| 检查 | 结果 | 原始记录 |
|---|---|---|
| 新消费者／模型 RED | 5 项失败，证明原组件尚未具备完整列表与运行上下文 | [RED](evidence/tool-ui-responsive/ui-responsive-workflow-red.txt) |
| 新消费者／模型 GREEN | 5 项通过 | [GREEN](evidence/tool-ui-responsive/ui-responsive-workflow-green.txt) |
| 全 web 回归 | `pnpm --filter @suma/web test`：194 项通过、18 个测试文件，退出 0 | [完整结果](evidence/tool-ui-responsive/ui-web-responsive-final.txt) |
| web 类型 | `pnpm --filter @suma/web typecheck`：退出 0 | [类型原文](evidence/tool-ui-responsive/ui-responsive-typecheck-final.txt)、[收据](evidence/tool-ui-responsive/ui-responsive-verification.json) |
| 新文件格式 | Prettier 3.6.2 `--check`，3 个新增文件通过，退出 0 | [格式原文](evidence/tool-ui-responsive/ui-responsive-format-check.txt) |
| 已跟踪 UI 空白检查 | `git -c core.whitespace=blank-at-eol,blank-at-eof,space-before-tab,cr-at-eol diff --check -- apps/web`：退出 0 | [原始空输出](evidence/tool-ui-responsive/ui-responsive-whitespace-check.txt)、[收据](evidence/tool-ui-responsive/ui-responsive-verification.json) |

五项新测试验证：长运行名和真实状态／尝试不丢失；135 个合成节点均生成原生可选按钮、选中标记唯一；当前 fence、未知原因与验收分开；筛选使用全部节点；ready 且 attempt0 表示尚未领取。135 个节点是明确的 SSR 测试夹具，不是实际运行规模或低配性能验收。

前代 web189 加本次 5 项为 web194；与此前 web164 基线相比，本阶段新增 30 项前端测试。它们不等于平台全部测试或 194 条实际用户旅程。

## 源码与原始证据切面

本次 UI 切面包含 21 份文件：前代 18 份中只有 Workflow.tsx 变化，另加 3 份响应式文件。前代 189 切面、37 份既有 UI 归档和原报告均保留，旧摘要继续对应它们自己的时间点，不能改写为本次结果。

本次 8 份原始材料为 6 份测试／检查输出、1 份当前 UI 源码收据和 1 份主集成提供的修复前截图，均逐字节保存。[来源映射](evidence/tool-ui-responsive/archive-map.json)记录格式、尺寸和摘要；[归档复核](evidence/tool-ui-responsive/archive-verification.json)检查当前 21 份源码以及前代原件保持情况。

## 尚待主集成的实际验证

本报告没有操作浏览器或重建生产网页。需要以真实 DOM 尺寸核对 390／768 下选择器、状态、完整按钮可见，选择节点后可直接核对、返回列表的焦点可见；在可取消的真实运行上核对原取消门禁与停止回执；检查深浅主题、桌面图、尺寸往返的视口与持续事件不跳动。单元／SSR 通过不能证明这些已完成，也不证明完整 WCAG、所有五条旅程或千节点目标已通过。
