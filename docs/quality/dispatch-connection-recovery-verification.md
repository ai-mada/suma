# 派发与云授权读取恢复验证

T-DISPATCH-CONNECTION-RECOVERY 覆盖任务派发的兼容工具、智能推荐历史和工具管理的 WorkBuddy 授权核对。 [交互与行为依据](../design/tool-management.md#派发与云账号读取恢复)

## 定向检查

Windows amd64、Node 22.16.0，在 `apps/web` 下运行以下命令。

```powershell
node ./node_modules/vitest/vitest.mjs run src/TaskDispatchOptionsPanel.test.tsx src/DispatchRecommendation.test.tsx
node ./node_modules/vitest/vitest.mjs run src/WorkBuddyConnections.test.tsx --maxWorkers 1 --minWorkers 1
node ./node_modules/typescript/bin/tsc --noEmit --incremental false
node scripts/check-i18n.mjs
```

- 派发 16 项通过。覆盖后页读取失败后按原游标重试、刷新、返回，拒绝重复游标，隐藏未确认页的操作，保留已独立读取的推荐。键盘焦点停留在触发操作上。
- WorkBuddy 42 项通过。覆盖原连接核对重试、读取失败时隐藏链接和旧提示、授权代次变化后的原记录定位、现有首次授权与撤权处理。重试路径只读取原连接，保留一次原授权提交。
- Web 类型检查通过。中英文 10,700 个词条的键、插值、复数和标记校验通过，生产源码未检出新增硬编码文案。回归前后结果和输入摘要分别保存在 `.cache/dispatch-page-recovery/` 与 `.cache/workbuddy-exact-read/`。

## 隔离浏览器

用 `node .cache/dispatch-connection-recovery/browser.mjs` 在独立回环页面渲染生产组件，通过受控 HTTP 响应复现暂时失败。Windows Chrome 覆盖中文 390px 深色派发与英文 1366px 浅色 WorkBuddy。两条浏览器旅程均通过。

- 键盘打开候选、请求第二页、处理 503、重试相同游标并返回首页。错误时隐藏旧候选，操作保持焦点，窄屏触达 44px 且无页面横向溢出。
- 模拟弹窗受阻后核对授权。准确连接读取返回 503，官方链接和旧就绪提示均收起，只留下“重新核对连接”。键盘重试同一连接并显示已断开记录，旧链接未恢复。仅发出一次原授权 POST。

结果、源摘要、两张截图和临时目录清理回执保存在 `.cache/dispatch-connection-recovery/`。浏览器访问限于本地夹具。此阶段验收界面恢复，真实 WorkBuddy OAuth 和完整生产部署分别沿原矩阵验收。12ui 包仓库访问受限，界面使用现有速码设计令牌与控件。
