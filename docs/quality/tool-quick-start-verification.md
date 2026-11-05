# 添加工具自动配置验证

T-TOOL-QUICK-START-239 · 速码 1.0.0 · 2026-11-05。

| 检查 | 结果 |
| --- | --- |
| Windows、Node 22.16.0，Web 目录运行 `node node_modules/vitest/vitest.mjs run LocalToolsScan ToolQuickConnect QuickConnectIntegration QoderQuickConnectCoordination` | 10 文件、42 个用例通过。覆盖唯一入口、歧义、过期、启动不受支持、草稿、账号变化、未确认写入和精确配置复用 |
| TypeScript 按原 Web 编译配置检查变更入口和关联测试 | 通过。全 Web 类型检查本次执行遇到工作区中并行修改的 `TerminalCandidatePanel.tsx:59` 类型错误，缺少 `TerminalCandidateSource.workbuddy_origin` |
| `node apps/web/scripts/check-i18n.mjs` | 10,309 对词条的语言、占位符和标记检查通过 |
| 隔离 Vite 组件构建、Playwright 驱动 Chrome | 使用实际弹窗和接入组件，注入明确合成的安装观察、节点库存、匹配与登记回执。中文 1366×768 验证首屏选择、键盘展开、同名协议区分、选择后收起和焦点回归。英文 390×844 验证自动选入、名称预填与无横向溢出。两条路径都在明确提交后收到暂停回执，控制台无错误或警告 |

自动选择沿原安装观察期限与当前授权读取，只读填充配置。可执行入口安装、凭据配置、真实 Runner 与厂商模型的运行验收仍按对应合同进行。活动预览使用的构建目录未用于本阶段验证。
