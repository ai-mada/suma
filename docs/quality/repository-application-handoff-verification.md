# 受管仓库合并到应用源码版本的交接验证

T-REPOSITORY-APPLICATION-HANDOFF-249 · 速码 1.0.0 · 2026-11-06。负责人为平台集成。真实 Git 与应用源码登记链路、界面组件及 Chrome 交互均已通过。[前序合并验证](repository-source-integration-verification.md) · [应用操作](../development/project-applications.md) · [发布集合边界](../../contracts/application-release-sets.md)

## 交接操作与证据

1. 从任务的仓库集成面板打开已确认应用的 Git 回执，进入项目应用。链接携带原计划标识与版本。应用页重新读取并核对集成回执，展示提交、目标引用和返回原回执的入口。
2. 选择拥有对应受管仓库代码范围的应用，打开源码版本登记表单。匹配范围使用确切计划与版本，其他范围由用户填写。当前证据缺失、版本已变化或范围不匹配时，交接界面提示当前限制。
3. 用户填写版本名称和理由，预览应用代码范围及依赖证据，完成必要的缺口确认，再明确提交登记。应用源码版本详情保留对应任务、运行、Git 提交与回执入口。构建制品、发布候选、发布计划和部署各自建立证据。

## 实际验证

在 Windows 同一主机运行两个真实 `suma-runner` 进程，分别封存来源与执行独立测试。隔离环境使用 SQLite、真实 Git 裸仓库、回环 HTTP、受管 Git Provider 和本地源码检查器。测试通过后执行正式验收、受管分支预演、批准和 Git 应用，再用该回执登记应用源码版本。

验证命令为 `go test ./internal/platform -run '^TestTaskDeliveryRepositoryDispatchPoolActualIndependentQuality$' -count=1 -v`，配置 `SUMA_TEST_REPOSITORY_POOL_RUNNER` 指向真实 Runner，并使用已构建的源码检查器。全链路 21.70 秒，应用交接子测试 2.48 秒。

| 范围 | 验证结果 |
| --- | --- |
| 同一 Git 来源 | 独立测试、验收、受管应用和应用源码版本均绑定提交 `0fd03598cbb659ac6d32a2793fa08f0599cc827b`。登记快照保留原验收、原操作回执摘要及对象 ID、代码范围与 Git 树。 |
| 版本与依赖门禁 | 预览使用过期集成版本返回冲突。无受支持清单的样本保留依赖覆盖缺口，需明确确认后才能登记。 |
| 显式登记与重试 | 预览后版本列表为空。登记绑定预览摘要，重放同一操作返回原版本，版本总数仍为一。发布候选、发布集合、发布计划与部署记录数量均为零。 |
| 工作台交互 | 两组组件测试共 12 项通过，并复验来源漂移时已确认预览仍无法提交。实际 Chrome 在中文 390 px 与英文 1280 px 验证键盘进入、确切来源固定、返回原回执、零写入及无横向溢出。浏览器使用隔离 HTTP 夹具。 |

TypeScript 定向类型检查及双语资源一致性检查通过。真实厂商账号、跨主机接入、应用范围构建制品和生产部署仍属独立验收。

## Multica 参考

参考本地 Multica 提交 `e8b6a97643ccc35efbcf2a4675c8d088a288ed6e`。其 PR 列表将合并状态与检查结果分别呈现，并用一行服务端判定说明关联任务的后续动作。速码独立实现该交互原则，将原 Git 回执、应用源码登记及后续发布作为各自可追溯的事实。[后续动作](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/packages/views/issues/components/pull-request-list.tsx#L245-L251) · [合并状态和检查结果](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/packages/views/issues/components/pull-request-list.tsx#L399-L451)
