# 独立包测试问题回流验收

速码 1.0.0，`T-PACKAGE-TEST-ISSUE-253`。负责人为质量、调度与工作台维护者。关联需求为正式质量门禁、执行异常处理、问题回流和中英文交互。 [接口合同](../../contracts/test-plans.md#逐例门禁与问题回流) · [中文操作](../development/test-plans.md#问题回流人工验收与恢复) · [English guide](../en/test-plans.md#read-evidence-and-resolve-failures)

## 行为与边界

- 从质量页中的当前包测试行登记问题。提交前确认原因、原节点和执行次数。服务端重核当前计划、任务、项目写权限、尝试和 fence，确认失败执行使用了封存制品。
- 问题和受保护的尝试关联在同一事务内保存。同一个失败尝试复用同一问题，恢复操作沿用原正文和操作标识。问题从质量页与任务交付可达，原问题可回流为需求。
- 未开始、失联、超时、结果未知、缺少证据或过期的尝试沿原运行诊断处理。创建问题不会改变原测试结论。后续修复关单依据见[包测试复验合同](../../contracts/issue-repair.md)。

## 证据

| 检查 | 结果 |
| --- | --- |
| 共享池真实执行 `TestPoolApplicationArtifactsActualBuildAndIndependentPackage` | Windows 隔离 Git、SQLite、HTTP 和独立 Runner 身份。真实包测试读取同一封存制品后退出 7；源码 JUnit 保持独立。已认证的模拟未知结果保持不可登记。覆盖无效尝试、错误 fence、过期、回执恢复和重复提交去重 |
| 任务与需求交接 | 同一问题出现在准确任务交付，从原问题回流需求。记录保留来源运行、节点、尝试和封存制品 |
| 原行为回归 | 项目独占模式混合门禁及 JUnit 用例问题去重通过。共 3 项 Go 定向测试 |
| 前端 | 22 项质量及任务交付定向检查。中文 390 px 和英文 1280 px 的 Chromium 隔离样例验证键盘打开、填写确认、准确提交正文、返回原问题和结果未知时隐藏登记动作 |
| 合同与语言 | TypeScript 类型、10,637 个界面语言键、489 条 API 路径、生成物和质量诊断定向检查通过。全量后端语言检查仍有两项失败，已在修改前的 `8d7b014` 复现同一组既有问题 |

实际运行日志与中英文截图位于本地忽略目录 `.cache/q253/`。固定参考来自 Multica 将检查状态及证据与原工作对象关联的展示方式。 [Multica 检查交互](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/packages/views/issues/components/pull-request-list.tsx#L515-L573)

本轮实际后端使用同机 Windows 隔离执行，浏览器使用独立的接口夹具。物理多机、OceanBase、生产验证及包专属修复关单仍待交付。
