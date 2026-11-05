# WorkBuddy 账号连接与恢复验证

T-WORKBUDDY-ACCOUNT-RECOVERY，2026-11-05，Windows amd64。范围为原连接授权、浏览器返回和可选部署配置。操作员配置见[容器部署](../deployment/containers.md#workbuddy-账号授权)，连接约束见[领域合同](../../contracts/workbuddy-connections.md)。

| 检查 | 结果 |
| --- | --- |
| Go 1.26.10，`go test ./internal/platform ./internal/workbuddy ./cmd/suma-control -run 'Test(WorkBuddyConnections\|WorkBuddyControlConfiguration\|OAuth)' -count=1` | 三个包通过。覆盖原连接替换后旧回调零次兑换、新回调一次兑换，以及已取得兑换资格的操作保持原连接版本隔离。使用短路径隔离缓存，首次尝试因项目磁盘空间不足中止。 |
| Node 22.16.0，`node ./node_modules/vitest/vitest.mjs run src/WorkBuddyConnections.test.tsx src/workbuddy-connection-presentation.test.ts`，在 `apps/web` 运行 | 38 项通过。新增用例先复现缺失的替换操作，再验证原连接与版本、切回后只读核对、不同授权代次不报成功。 |
| `node apps/web/node_modules/typescript/bin/tsc --noEmit -p apps/web/tsconfig.json` 和 `node scripts/check-i18n.mjs` | 类型检查通过。早先并行工作期间的类型检查遇到终端候选字段缺失，稍后复验通过。10309 个语言键和生产文本检查通过。 |
| `node --test apps/desktop/tests/oauth-bridge-main.test.cjs apps/desktop/tests/oauth-window.test.cjs` | 18 项通过，涵盖原会话、窄 IPC、窗口及受保护回调。 |
| Edge 与 Playwright，隔离 Vite 组件，中文 390px | 键盘展开配置要求，页面无横向溢出。模拟从未配置切换到已配置，在原记录选择权限生成新链接。官方页请求由测试拦截。关闭测试标签页后触发 focus，观察到一次生成请求、一次精确连接读取，界面显示授权已保存并收起旧链接，页面错误 0。截图 SHA-256 为 `8b0eeddddc568132c5c2b8a4fed553debff0101cfbbb869ec2ec85e109514957`。 |
| Docker Compose 5.5.1 `config`，Helm 3.19.0 `template`，`bash -n deploy/entrypoint.sh` | SQLite、PostgreSQL、OceanBase 三种配置均通过模板核对，应用密钥走秘密引用。Helm 拒绝部分配置和缺少主密钥的共享数据库配置。入口脚本语法检查通过。 |

真实 WorkBuddy 应用登记、真实用户同意与令牌回调仍需在已配置的公开源联调。本阶段使用本地测试供应方和浏览器模拟响应。部署、云任务资格及完整平台验收各自保留原状态。
