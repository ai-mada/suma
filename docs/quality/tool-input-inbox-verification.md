# 智能体工具问题收件箱

速码 1.0.0，`T-TOOL-INPUT-INBOX-264`。关联需求为执行等待、团队通知、原尝试定位和中英文交互。 [收件箱合同](../../contracts/notification-inbox.md#智能体请求人工输入) · [交互设计](../design/notification-inbox.md#工具问题的处理入口)

## 设计与范围

正式任务运行第一次保存原工具问题时，在同一事务向具备项目写权限的人员负责人和明确关注者投影通知。普通 Codex 非阻塞输入保持原执行记录。通知只展示请求发生过的事实，准确定位原运行、节点和尝试。内容与当前回答资格由原工作台按权限读取。

参考 Multica 将需跟进的智能体动态汇总到个人收件箱并连接原任务上下文的方式。速码沿自身持久工具问题、正式任务绑定和个人通知回执实现。 [Multica 固定参考](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/apps/docs/content/docs/inbox.zh.mdx#L7-L22)

## 阶段验证

- Windows、Go 1.26.10、真实控制面 HTTP 与隔离 SQLite。5 项定向后端测试通过，其中新增集成覆盖普通 Runner 与共享池。验证通知写入失败时整笔回滚、原请求重放去重、接收者资格、撤权、任务绑定失效，以及回答后仍定位原尝试。返回的通知未包含问答正文。
- 53 项定向 Web 测试、TypeScript、中英文目录和生成 API 合同检查通过。隔离 Chromium 在中文 390 像素和英文 1280 像素下验证键盘操作、跨项目原节点与尝试、无横向溢出，打开入口没有产生写请求。
- 原始证据位于本地忽略目录 `.cache/q264/backend/` 和 `.cache/q264/inbox-browser/`。HTTP 联调使用隔离工具协议数据，浏览器使用 API 测试数据。外部账号、实际模型进程、跨机器共享池和 OceanBase 本阶段未实测。
