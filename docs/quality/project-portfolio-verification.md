# 组织项目组合验证

T-ORG-PROJECT-PORTFOLIO。Windows、Go 1.26.10、SQLite、Vite 6.1.0、Chrome。组织项目列表来自当前浏览器会话授权，验收、阻塞、活动风险和里程碑来自各自正式记录。

- `go test -overlay <阶段隔离配置> ./internal/platform -run '^TestProjectPortfolio' -count=1 -v` 覆盖 4 项。浏览器身份、项目 API／Runner／Agent 隔离、封闭查询和缓存、任务计数与里程碑、搜索、顺序、游标和当前权限。首轮聚合用例的风险夹具缺少正式合同所需的空关联列表，并尝试创建不合合同的无日期里程碑；修正夹具后原用例独立复验通过。
- `go test ./internal/platform ./cmd/suma-contracts -run '^TestProjectPortfolio' -count=1 -v` 在最终接线与并行仓库阶段合入后通过全部 5 项，含前述 4 项平台测试及 1 项生成合同测试。`suma-contracts -check` 通过 1059 个模型、477 条路径及 TypeScript；结构比较确认只有新增组合接口与三个类型变化。
- `node node_modules/vitest/vitest.mjs run src/ProjectPortfolio.test.tsx src/management-navigation.test.tsx` 分批通过 10 个不同用例。覆盖当前行事实、分页恢复、权限拒绝、组织切换后的迟到响应、组合导航及刚授权项目的会话重读。`node node_modules/typescript/bin/tsc --noEmit` 通过。双语目录 10425 对，生产硬编码 0。
- 正式 HTTP 创建 3 个项目、4 项未验收任务、3 个里程碑和 2 条风险。真实 Chrome／SQLite 最终 12 项通过、零运行错误。验证组织所有者与只读成员看到 3／2 个项目，只有有权数据参与列表与统计；验证字面搜索、精确项目概览／设置、真实撤权后刷新、清空查询和双语控件。控制面从 `7936058` 加本阶段服务和路由的隔离输入构建，前端使用最终界面。首轮 10 项通过后目视发现搜索词条缺失及窄屏表单偏高，修复后验证中英文按钮、同排 44px 触控区、390px 无横向溢出及 1366px 代表界面。
- 最终报告 `.cache/project-portfolio/browser-evidence-5PgLef/result.json`，SHA256 `22a7791124a8226c713448bc07191e446bad880b188365045d5b627748b42d6e`。报告及截图保留。Chrome 已关闭，隔离控制进程结束并释放端口。按哈希盘点清理 292 个浏览器构建与运行临时文件，收据位于 `.cache/project-portfolio/browser-cleanup.json`。生成检查清理了 23 个临时文件，收据位于 `.cache/project-portfolio/contract/scratch-cleanup.json`。

真实组合规模、跨库和跨项目资源冲突待单独验收。浏览器夹具没有正式人工验收动作，非零验收计数由 SQLite 定向测试验证。
