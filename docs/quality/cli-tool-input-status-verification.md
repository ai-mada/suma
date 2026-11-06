# 终端工具问题与送达诊断

速码 1.0.0，`T-CLI-TOOL-INPUT-STATUS-263`。关联需求为任务协作、分布式执行跟踪和中英文 CLI。 [操作](../operations/task-dispatch-options.md#核对智能体等待的问题) · [合同](../../contracts/tool-inputs.md#从终端核对工具问题)

## 设计与范围

工具问题读取在当前授权与执行范围事务内验证原尝试及 fence，区分真实空列表与错误标识。CLI 使用既有登录会话，仅输出状态与送达摘要，并给出定位原尝试的工作台链接。答复已保存、执行器观察、开始写出、写出和原请求关闭分开表达。

参考 Multica 将需要关注的动态与对应任务中的完整上下文分别呈现的做法。速码沿自身尝试、问题和送达证据实现终端诊断。 [Multica 固定参考](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/apps/docs/content/docs/inbox.zh.mdx#L7-L22)

## 阶段验证

- 真实 HTTP 定向验证覆盖普通节点和共享池的 Codex、Cursor 输入。不存在的尝试、错误 fence 和大小写错误的节点标识返回 404。真实空列表与准确的历史尝试保持可读。
- 6 项 CLI 定向测试通过，包括 4 项新命令测试和 2 项复用工作台来源地址的原测试。覆盖准确请求、权限拒绝、无当前配置的历史、异常响应和终端输出隐私。生成 API 合同已核对。
- Windows amd64、Go 1.26.10。实际编译的 CLI 通过回环 HTTP 调用控制面与隔离 SQLite。6 次只读调用分别验证空列表、待回答、已保存未写出、已写出且已关闭、错误 fence 拒绝和项目 API 凭证拒绝。已核对原尝试链接、终端正文隔离和数据库业务指纹。使用隔离 Codex 协议数据，未执行厂商或模型进程。

证据保存在本地忽略目录 `.cache/q263/backend/`、`.cache/q263/cli-test/` 和 `.cache/q263/cli-actual/`。本阶段尚未验证 OceanBase、跨机器执行或真实厂商会话。
