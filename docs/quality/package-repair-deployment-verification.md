# 修复关联候选部署与回滚验收

速码 1.0.0，`T-PACKAGE-REPAIR-DEPLOY-256`。关联需求为缺陷修复、审批、版本发布、健康证据与回滚追溯。 [发布控制合同](../../contracts/release-control.md#修复交接到发布候选) · [操作与 AI SOP](../development/issue-repair.md)

## 可核对的行为

- 发布计划和部署详情从服务端返回原修复轮次，可返回问题查看对应历史证据。该来源和当前发布资格分别显示。来源投影仅核对固定记录，无需额外读取封存制品字节。
- 原问题重开后，正向发布的新预览和已批准计划的执行均受阻。当前有权审批人仍可撤销原批准。
- 同环境的健康历史部署可打开回滚审阅。表单展示服务器已计算的回滚预览，固定环境代数及历史操作。创建后沿原计划重新审批与执行。

## 实际验证

`TestPoolApplicationArtifactsActualBuildAndIndependentPackage` 在 Windows 同机隔离环境通过，使用真实 Git、SQLite、Node、共享池 Runner、受管发布 Runner 和本地静态提供方。包测试问题复验关单后创建关联候选，再经正式审批发布。管理部署沿已配置的专用 Runner 路由。问题重开后，新预览和已批准正向执行均返回 409，仍可撤销旧批准。实际 HTTP 响应在回滚后恢复原修复制品字节，两个健康证据分别保留。部署和回滚详情返回同一原修复轮次。

| 步骤 | 环境代数 | 实际操作 |
| --- | --- | --- |
| 发布修复候选 | 1 | `deploy-06780c6fb55e15a03c1489b560805c3d` |
| 发布不同制品 | 2 | `deploy-14832b706ebaa7683da40d05f86be535` |
| 回滚至第 1 代健康部署的制品 | 3 | `deploy-515c14c1b1adf978da16f6af996e292e` |

恢复的封存清单 SHA-256 为 `7584f22cdb3485462a58c23386ef1bab1d7ba87fd6f6257527887853fdd2aaad`，实际响应正文 SHA-256 为 `7c226c42a5f566331bbeaf4800cbbffbce66d4ba595447d45b09e6e96ce59d1d`。运行日志保存在本地忽略目录 `.cache/q256/`。

30 项前端定向检查通过，覆盖原请求回执恢复、固定回滚预览和修复轮次导航。独立 Chromium 夹具在中文 390 px 和英文 1280 px 验证键盘打开历史关闭记录、明确创建、准确预览摘要和环境代数。界面未读取候选的正向发布资格。TypeScript、10,673 个界面语言键、生成接口与文档链接检查通过。

此处完成的是隔离提供方部署和回滚验证。物理多机、OceanBase、真实生产环境及共享池直接承接受管部署仍待验收。

交互参考 Multica 对检查事实、原工作项与动作入口的分层。 [Multica 固定源码](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/packages/views/issues/components/pull-request-list.tsx#L515-L573)
