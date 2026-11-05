# 共享池混合质量门禁验收

阶段 `T-POOL-MIXED-QUALITY-252`，速码 1.0.0。负责人为质量、执行池与工作台维护者。关联需求为同一交付范围的独立测试、人工验收、发布证据和异常恢复。状态为阶段验证通过。 [接口合同](../../contracts/test-plans.md) · [操作指南](../development/test-plans.md) · [English guide](../en/test-plans.md)

## 验收路径

1. 项目启用双向授权的共享池，登记应用源码版本。构建、源码测试和包测试分别使用固定工具，构建和包测试使用不同的已批准 Pool Runner。
2. 为独立交付任务发布质量计划，绑定同时含构建、JUnit 和包测试的流程。只含包测试的流程会被拒绝。若选择了作为应用源码依据的原验收任务，创建返回冲突，原任务和运行数保持原状。
3. 源码用例先完成时，质量页仍显示包测试待验证，正式验收拒绝提前批准。修改包测试档案版本后，原运行无法领取这项工作。恢复精确档案后，可领取原运行。
4. 包测试消费构建封存的同一清单。错误授权版本、槽位、Runner 或混用 JUnit 回执均无法提交有效证据。恢复授权后的重复解析返回原收据。核验后，任务正式验收并形成合格候选。

## 已完成验证

| 证据 | 结果 |
| --- | --- |
| `TestPoolApplicationArtifactsActualBuildAndIndependentPackage` | Windows 隔离夹具使用实际 Git 工作树、SQLite、HTTP 和 Node 进程，三个独立 Pool 身份完成构建、JUnit 和包测试。源码任务重新绑定回滚、正式验收、候选生成均通过 |
| `TestTaskDeliveryRepositoryDispatchPoolActualIndependentQuality` | 同机 Pool 任务派发、候选交接和独立 JUnit 回归通过，包含应用源码交接 |
| `TestTestPlanMixedPackageEvidenceKeepsJUnitAndArtifactGates` | 项目独占模式回归通过，覆盖混合成功、缺报告、结果未知及纯包测试的拒绝路径 |
| 工作台组件与模型 | 17 项定向检查通过。中英文 10,629 个键、占位符等一致性检查及 TypeScript 类型检查通过 |
| Chromium | 使用合成 API 数据验证中文 390 px 和英文 1280 px，JUnit 计数独立、可聚焦证据链接、无横向溢出及界面写入 |

最终共享池夹具封存清单 SHA-256 为 `7584f22cdb3485462a58c23386ef1bab1d7ba87fd6f6257527887853fdd2aaad`，候选绑定的源码提交为 `8cdb6210ddbad2e421cad149c4b700166207062e`，应用范围为 `packages/library`。运行日志和浏览器截图留在本地忽略目录 `.cache/q252/`。

工作台参考 Multica 对独立检查与合并结论的展示区分，速码通过现有测试计划和制品模块实现。 [Multica 固定参考](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/packages/views/issues/components/pull-request-list.tsx#L515-L573)

真实执行验证覆盖同机隔离环境。物理多机、OceanBase、生产发布和完整平台规模验收仍是独立交付项。
