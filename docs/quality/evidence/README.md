# 当前交付证据索引

状态：本机证据，2026-10-24；负责人：平台集成。证据只适用于[验证报告](../verification.md)明确的范围，完整目标见[实施台账](../../delivery/implementation-plan.md)。

## P5 真实制品与发布

| 证据 | 内容 |
|---|---|
| [最终Go记录](go-tests-release.jsonl)、[汇总](go-tests-release-summary.json) | 281个通过事件（含子测试/helper）、8个受测包；命令包只编译 |
| [真实发布旅程](release-browser-execution.json) | 实际2份blob与证明、5个部署操作、7个运行、provider代数5、原检出干净；修复后问题与需求各1个 |
| [包测试](release-b-package-test.png)、[制品证明](release-artifact-proofs.png)、[计划审阅](release-plan-review.png) | 真实固定C/下载字节/人工门禁 |
| [B健康异常](release-b-final-unhealthy.png)、[实际B站点](release-b-site.png)、[需求来源](release-requirement-lineage.png) | 真实404与已切换事实分开；需求可返回原问题/发布 |
| [回退批准](release-rollback-approved.png)、[最终宽屏](release-final-1920.png)、[实际A站点](release-rollback-a-site.png) | 独立新计划/批准/操作；A恢复，原故障保留 |
| [390](release-final-390.png)、[768](release-final-768.png)、[1366](release-final-1366.png)、[1920浅色](release-final-light-1920.png)、[范围与键盘记录](release-browser-visual.json) | 部分发布页面实际宽度及弹层Escape回焦点；非完整可访问性验收 |
| [执行协议回执](release-execution-verification.json)、[原始事件](release-execution-tests.jsonl) | 实际上传/包测试、Provider崩溃重启、ACK丢失及固定操作恢复，范围由收据限定 |
| [发布控制回执](release-control-review.json)、[独立审核](release-authority-review.json) | 实际SQLite并发、精确授权、原unknown保留、当前租约/only-managed/撤权与重放边界 |

宿主捕获、过渡帧及IAB重复合成截图已按[捕获收据](visual-capture-receipt.json)保存摘要，原件归档到忽略的任务诊断目录，不作为最终视觉验收。最终四宽度PNG使用独立Chrome标签页在布局稳定后捕获，记录实际innerWidth；390审阅弹层及键盘另有IAB实际验证。这些是部分页面证据，不能替代完整视觉矩阵。

## 治理、计划与候选历史切面

| 证据 | 内容 |
|---|---|
| [完整 Go 汇总](go-tests-wave2-summary.json)、[原始结果](go-tests-wave2.jsonl) | 205项通过、0失败/跳过；包括子测试 |
| [真实候选执行](candidate-browser-execution.json) | SQLite正式状态、4次尝试、2个Runner、固定提交、journal与文件摘要核对 |
| [实际候选测试页面](candidate-test-1920.png)、[人工验收](candidate-human-acceptance.png) | 来源attempt2、测试C而非B、正式任务accepted |
| [首轮真实失败](candidate-runner-first-failure.png) | Windows环境缺失的失败事实保留；不计作验证通过 |
| [Windows环境修复](runner-windows-environment-execute.json) | 同一许可配置经过真实Execute完成编辑、测试、vet |
| [治理与候选独立审查](governance-planning-review.json) | 撤权竞态、严格版本/字段、未知效果、来源冻结等复现与修复 |
| [团队身份](governance-team.png)、[组织隔离](governance-isolated-organization.png) | 实际页面创建/切换、真实账号团队归属、空组织无旧项目数据 |
| [基线差异](planning-baseline-comparison.png)、[容量](planning-capacity.png)、[迭代完成](planning-iteration-completed.png) | 实际业务数据及门禁；容量是估算分配而非实际工时 |
| [390页面](planning-390.png)、[390完整排期表单](planning-form-390.png)、[768页面](planning-768.png)、[1366页面](planning-1366.png)、[1920运行](candidate-test-1920.png) | 部分页面实际尺寸走查，不代表全部旅程矩阵 |
| [新计划深色主题](planning-capacity-dark-1920.png) | 容量、未知值及范围说明的实际深色页面 |
| [打包后实际工作台](packaged-control-candidate-1920.png) | Windows 目录包中的控制面与网页实际启动，恢复已验收候选；不等同 Electron 原生 GUI 验收 |
| [六平台构建收据](../../deployment/evidence/runtime-build-manifest.json)、[源码切面](../../deployment/evidence/runtime-source-manifest.json)、[链接模块](../../deployment/evidence/runtime-linked-modules.json) | Windows/Linux/macOS × amd64/arm64 共 18 个 Go 程序实际编译和目标信息检查；未在其他操作系统执行 |

## 2026-10-23 历史纵向基线

| 证据 | 内容 |
|---|---|
| [Go 测试摘要](go-tests-summary.json) | 108个用例、5个受测包及vet结果 |
| [实际旅程](browser-journey.json) | 双Runner真实执行、人工验收、问题与去重需求回流 |
| [人工验收页面](browser-task-accepted.png) | 页面提交后正式任务accepted |
| [最终浅色运行页面](browser-run-succeeded.png) | 固定v2执行、两个成功测试节点 |
| [最终深色运行页面](browser-run-dark.png) | 深色主题、节点与图例可读性 |
| [源码与文档清单](source-sha256.json) | 非忽略交付文件逐个SHA256；不含自身 |
| [Windows桌面收据](../../deployment/evidence/windows-package-receipt.json) | 实际打包结果，未签名、无GUI与安装结论 |
| [Windows逐文件清单](../../deployment/evidence/windows-unpacked-sha256.json) | 整个桌面目录内容摘要 |
| [第三方元数据](../../delivery/third-party-dependencies.json) | 依赖、许可文件及锁文件摘要 |

使用 PowerShell7 执行 `scripts/snapshot-delivery.ps1` 可以在源码或文档变化后重新生成交付清单。构建二进制、目录包有独立收据；重新构建后必须重新采集。截图源于真实浏览器，本轮没有生成图替代产品运行结果。
