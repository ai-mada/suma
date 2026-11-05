# 独立包测试修复关单验收

速码 1.0.0，`T-PACKAGE-TEST-REPAIR-254`。负责人为质量、执行池与工作台维护者。关联需求为缺陷修复、独立复验、人工关单、执行追溯和中英文交互。 [接口合同](../../contracts/issue-repair.md) · [领域架构](../architecture/issue-repair.md) · [操作与 AI SOP](../development/issue-repair.md)

## 实际旅程

共享执行池使用真实 Git、SQLite、HTTP 和 Node 进程。构建、源码 JUnit 和包测试由不同 Runner 身份执行。错误构建产生的制品被原包测试命令识别，问题关联原运行、制品和需求。原计划退役后，修复面板仍能读取失败来源。

修复复用同一需求并创建本轮任务。仅 JUnit 通过且正式 Review 已批准时，关闭请求返回 409。纠正构建配置后，用同一包测试命令独立核验修复制品。复验通过后仍等待正式 Review。关单保存准确的包测试证明，重放原请求返回原关单回执。

| 事实 | 实际值 |
| --- | --- |
| 同一登记范围和源码提交 | `packages/library`，`9886b05269f9f6ba75536bf8a34cb3448699f28f` |
| 原失败清单 SHA-256 | `502ebb75ea49130840bc2663cb07803ee4d75ce8569d570d738b93a18268a3d5` |
| 修复清单 SHA-256 | `7584f22cdb3485462a58c23386ef1bab1d7ba87fd6f6257527887853fdd2aaad` |
| 复验证明及正式 Review | `a87fc356bb910ff074e94f785d6e0064`，`491c0135d9980c647e539b77a5f7f34a` |

## 定向检查

- 真实 Pool 旅程 `TestPoolApplicationArtifactsActualBuildAndIndependentPackage` 通过。五项既有修复领域回归覆盖权限、精确回执、任务范围、JUnit 原始问题与关闭重开。
- 范围规则专项覆盖同一登记范围的新提交，以及不同应用、范围、目录、Git 目标、提供方、引用和配置的拒绝。
- 七项前端测试通过。Chromium 使用独立接口夹具验证中文 390 px 和英文 1280 px 的阻断与关闭状态、键盘操作、44 px 窄屏目标和准确导航。TypeScript、生成合同与 10,648 个界面语言键校验通过。

运行日志与截图留在本地忽略目录 `.cache/q254/`。原失败和复验的分开呈现参考 Multica 检查证据的独立视图。 [Multica 固定源码](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/packages/views/issues/components/pull-request-list.tsx#L515-L573)

实际复验修正的是构建配置，源码提交保持相同。不同提交同范围的接受规则经过专项验证。物理多机、OceanBase、生产发布和未登记应用范围的真实复验旅程仍待验收。
