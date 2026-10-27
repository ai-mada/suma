# 修复计划修订：前端定向检查与实际界面验证

阶段 T-P4-033／1.0.0；2026-10-27。初始定向检查、隔离实际warm旅程14项及最终前端cold代表性检查通过；另有主预览warm21项精确核对通过。各条目按实际、组件与待验收区分，不把局部结果作为生产或完整平台验收。见[实际报告](candidate-repair-revisions-verification.md)与[摘要](candidate-repair-revisions-summary.json)。完整P0–P7继续实施。

见[合同](../../contracts/candidate-repair-revisions.md)、[交互设计](../design/candidate-repair-revisions.md)、[English](candidate-repair-revisions-ui.en.md)。032的实际运行、Git／SDK和正式测试证据保留在[原报告](candidate-conflict-repair-verification.md)，不计作033新验收。

## 已完成的前端检查

在 `apps/web` 使用已安装的工具直接执行下列命令，均退出0。`pnpm` 的首次入口因 workspace结构核验在测试启动前停止；没有重新安装依赖。

```text
node node_modules/vitest/vitest.mjs run src/CandidateRepairRevisionPanel.test.tsx src/TaskRunHistoryPanel.test.tsx src/candidate-repair-revision-wire.test.ts src/task-run-history-model.test.ts src/candidate-conflict-repair-wire.test.ts src/write-action-recovery.test.tsx src/CandidateConflictRepairPanel.test.tsx src/CandidateCompositionPanel.test.tsx src/TaskCodeBaselinePanel.test.tsx src/UsageWorkspace.test.tsx
node node_modules/typescript/bin/tsc --noEmit
```

另于仓库根执行 `node apps/web/scripts/check-i18n.mjs`，4,335对词条通过语言、占位符、复数、markup及硬编码检查。此处是源冻结时证据；后续只为实际发现的观察说明翻译和Review旧运行摘要各增加一项定向回归，并构建隔离cut。最终类型检查退出0，未变词条目录复用检查，不重跑全部组件。

| 检查范围 | 结果与边界 |
|---|---|
| 10个受影响文件，37项检查 | 全部通过；包含受影响032组件复验，不等于37项新功能或正式候选测试 |
| 修订交互 | 折叠零配置读取；显式选择；同任务新计划；准确URL；无隐式Start；409保留草稿并重新确认 |
| 恢复与权限 | 错误结构200保持原请求；重挂载及历史父版仍可找回；原validator／caller／范围保持；写入403隐藏子计划 |
| 失败资格 | 没有认证时阻断；认证恢复上下文才携带准确 `expected_recovery_sha256`；组件夹具不证明宿主实际执行 |
| 履历与会计显示 | run_id分页去重、原运行／计划链接、nullable观察事实；认证启动失败为已知0 Token时，未定价费用仍未知 |
| 类型与双语 | 使用生成合同；中英4335对通过。完整设备、权限渠道及离线矩阵未执行 |

真实响应结构夹具来自实际公开Service方法，不是浏览器验收：

- `apps/web/src/test-fixtures/candidate-repair-revision.json`：实际 create／context／revise返回的结构及关联；原4份JSON顺序拼接摘要 `57bb3b7a52fdef29144ea1fa0d0eff551d6529c3b67c5036c179b4872c7d18f3`。
- `apps/web/src/test-fixtures/task-run-history.json`：实际List输出，保留绑定version8与旧关联null事实；原JSON摘要 `e9894b5219674e37bf9a6f97fa1e8dc0ee02728bc98952daca6ce748aa92f77f`。

标识符及摘要已确定性脱敏，字段是否存在、strategy、版本、来源顺序和跨响应关系保持。原件留在本阶段忽略证据目录；未提交账号、数据库、日志或截图。

## 实际浏览器结果与剩余步骤

使用独立033实际数据及匹配构建，保留用户标签页和来源。复用已验收A／X，不因刷新重复模型、正式测试或旧032旅程。3项新运行、4项正式断言一次、1次获明确许可的本地夹具Review以及未派发D均由独立执行／数据库证据证明，不能只由截图推断。

| 步骤 | 操作与应核对事实 | 状态 |
|---|---|---|
| 1. 准确定位与折叠 | 准确task／plan深链、固定配置／冲突回链及键盘历史展开均已实测。折叠零读取、路由区分由组件检查覆盖，没有补造未采集的浏览器网络断言。 | 代表性交互已验证；零读取为组件证据 |
| 2. 准备新预览 | 明确选择B→A的ready修订，保持同任务及有序来源，新计划URL准确、无run、Start确认重置；旧链接刷新保持旧定义，旧Start禁止。无效工具／command类型选择由wire及组件门禁覆盖。 | 实际及定向检查通过 |
| 3. 认证失败与替代 | 看到A的真实typed启动前失败，再明确修订为健康B；新预览没有提前替换失败run，另行Start后才绑定新run。B候选由A独立测试，明确Review后绑定仍未开始的D。 | 实际通过 |
| 4. 原操作恢复 | 原path／body／key、异常200、历史父版恢复、409草稿及401／403隐藏由真实HTTP或定向组件覆盖。未再制造浏览器断网／权限撤回或丢失回执故障，不宣称此完整矩阵已实测。 | HTTP／组件通过；浏览器故障矩阵待验收 |
| 5. 执行历史与用量 | 两条实际bound历史保留P2失败与当前P3准确run／plan链接；账本证明启动已知0、费用仍未定价，健康用量未知。legacy observed的nullable时间／版本来自真实Service与组件夹具，不冒称本旅程新造了旧历史。 | 实际bound与会计已验证；nullable为定向证据 |
| 6. 中英、键盘与冷回读 | 代表性中英、键盘历史、准确历史URL刷新及D基线冷回读通过。修正后Review打开显示当前成功run；cold仅打开、不再次提交。无Runner／模型／正式复测，全设备及窄屏矩阵仍待验收。 | 代表性warm／cold通过；完整设备矩阵待验收 |

实际warm14项收据保留最初发现的Review旧摘要问题；最终cold已验证修正后的当前run显示，不覆盖原收据。最终`web-delivery`135个产物，入口摘要`cf059de98de9627fe5edf1ca2faa0e7bf15e375820e4ac149f2cf56d50c873d6`。浏览器显示没有制造宿主证明；修订、运行、测试、人工验收、目标集成及下游绑定仍分别记录。主预览warm另有21项收据，证明135个服务产物、186表结构、182张非锁完整表及34行仅锁sequence各+24；真实厂商、多物理机器和生产验收仍未完成。

## 验收时补充的最少证据

记录匹配的源／构建标识、实际task／旧新plan／run及版本、准确URL、选用的故障分支、原操作编号摘要和必要前后读回。截图及控制台／请求证据留在本阶段忽略目录，保留失败并记录后续修正；不得只以页面文字或截图推断事务、独立测试、账单及验收成功。

实际原件为忽略目录`browser/warm-verification.local.json`、`browser/cold-verification.local.json`、`cold-readback/closed.local.json`及`primary-upgrade/warm-verification.local.json`，完整摘要见[机器报告](candidate-repair-revisions-summary.json)。冷回读进程正常退出0，选定业务schema／行／账户和非DB制品保持；没有重新登录、调用模型、正式复测或执行新的业务命令。限定本地阶段已闭合，完整平台仍在实施。
