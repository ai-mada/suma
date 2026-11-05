# 应用范围构建与发布证据验证

T-APPLICATION-SCOPED-BUILD-250 · 速码 1.0.0 · 2026-11-06。负责人为平台集成。[操作](../development/application-release-sets.md#从应用源码版本构建) · [范围合同](../../contracts/application-release-sets.md#scoped-build-evidence)

## 从已登记版本到发布集合

从已登记应用源码版本选择单一范围，使用项目发布 Runner 的登记构建档案。命令节点固定应用版本、版本摘要、范围、Git 子树、提交及原仓库回执。控制面在领取、新上传及成功回执阶段核对登记依据，Runner 从实际 Git 工作树的对应目录运行和收集输出。原独立包测试继续证明实际封存制品。发布集合依原版本证据判断候选归属。

制品、发布候选与集合详情展示相同构建事实。范围证明与包测试分开展示；普通项目制品保留项目级来源。展开证据可核对快照摘要与 Git 回执摘要。

## 验证记录

| 范围 | 环境与核对结果 |
| --- | --- |
| 真实构建、封存与发布 | Windows、SQLite、真实 Git、实际 Node 构建与两个独立包测试 Worker。库和 Web 两个应用使用同一仓库提交与不同子树，分别生成、封存及测试制品。各自的候选进入同一依赖闭合发布集合，经独立计划批准后，在隔离静态环境验证健康结果。 |
| 证明边界 | Runner 拒绝替换后的 Git 子树。控制面拒绝把另一应用的证明提交到当前尝试，原结果仍可确认。互换同提交的两份候选时，发布预览报告来源不匹配。普通项目制品的发布候选流程仍通过。 |
| 上传恢复 | 三项定向 Runner 测试通过。上传封存响应丢失后，恢复原应用范围证明并重用原操作，构建命令仅运行一次。 |
| 界面 | 七项定向组件测试通过。实际 Chrome 使用隔离 HTTP 夹具，在中文 390 px 制品页和英文 1280 px 候选页核对精确版本和树、键盘展开、普通构建、无横向溢出及零写入。 |

关键命令为 `go test ./internal/platform -run '^(TestApplicationReleaseSetActualArtifactsPlansAndIndependentDeployment|TestArtifactsRealBytesDerivedProofAndAcceptedCandidate)$' -count=1 -v`。两项平台测试分别通过，耗时 32.38 秒与 8.91 秒。生成的 OpenAPI 和 TypeScript 合同包含应用构建证明，Web 类型检查与中英文目录检查通过。数据库和外部静态服务为独立夹具。应用范围证明核对工作目录和 Git 树，跨目录依赖和命令访问仍由管理员策略管控。多源码范围构建、共享执行池制品协议、容器发布及生产环境属于后续验收。

## Multica 参考

参考本地 Multica 固定提交 `e8b6a97643ccc35efbcf2a4675c8d088a288ed6e`。Multica 的 PR 界面分别呈现合并状态、CI 结论和过期检查，GitHub 同步以当前 HEAD 核对检查依据。速码独立实现该交互原则，分别保留应用 Git 来源、构建、包测试和部署证据。[界面参考](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/packages/views/issues/components/pull-request-list.tsx#L402-L419) · [HEAD 核对](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/server/internal/handler/github.go#L284-L291)
