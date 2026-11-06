# 调度队列与尝试读取恢复验证

T-SCHEDULING-READ-RECOVERY 修复范围变化和访问失效后的旧列表滞留。恢复入口区分暂时失败和权限失效，前者重试原页，后者重新读取首页。精确尝试深链在重新获准后恢复。 [工作台交互](../design/scheduling-workspace.md#队列与尝试读取恢复)

## 定向验证

- Windows amd64、Node 22.16.0、Vitest 3.0.5。`vitest run src/scheduling-scope.test.tsx` 的 3 项回归先复现再通过，覆盖同项目身份切换与迟到响应、撤权时清除记录和游标、暂时失败时按原游标重试。
- `vitest run src/scheduling-dom.test.tsx src/workflow-scheduling-integration.test.tsx` 合计通过 24 项。新增的真实组件用例先复现尝试列表 403 后残留的工具事件，最终验证已选尝试、工具历史及计数撤下，恢复后重读原选择。单按钮调整后复跑该用例通过，计入原总数。
- `node node_modules/typescript/bin/tsc --noEmit` 通过。`node scripts/check-i18n.mjs` 核对 10650 个词条，未发现语言键或生产硬编码问题。

## 隔离浏览器

Chrome 156.0.8078.80 与 Playwright 1.62.1 使用实际队列组件和应用样式，在 1366×768 和 390×844 的英文界面复核。合成回环接口依次返回首次失败、首页成功、续页失败、续页重试成功、撤权及恢复。六次 GET 均携带准确原视角。续页重试使用同一游标，撤权后的恢复从首页读取。窄屏拒绝提示保留单一读取按钮，无横向溢出。键盘重试在请求期间保留原按钮焦点，再按 Enter 无新增请求。写入与非预期浏览器错误均为零。

原结果、桌面与窄屏截图及清理回执位于 `.cache/scheduling-read-scope/`。本阶段证据覆盖 Web 读取与交互恢复，调度吞吐、跨库、物理多机与生产授权继续按原矩阵验收。
