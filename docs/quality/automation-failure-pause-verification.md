# 自动化连续失败保护验收

阶段 T-AUTOMATION-FAILURE-PAUSE-286，版本 1.0.0。 [交互](../design/automation-workspace.md#连续失败保护与恢复) · [规则合同](../../contracts/automation-events.md#连续失败保护)

- Windows 上以真实 SQLite 和 HTTP 验证策略校验、成功打断失败序列、同一修订连续失败触发暂停、排队触发标记受阻、已有运行保留和新修订恢复。注入队列写入错误后，规则暂停及队列状态一并回滚。关闭或省略保护时仍可派发。
- Web 相关 13 项定向检查通过。最终界面把「编辑策略」放在自动暂停说明内，复跑该流程通过。英文浅色 1366px 的 Chrome 连接真实控制面，在界面设置阈值 2、保存并确认启用。真实运行依次失败后，下次触发被拦截，原页刷新显示暂停。中文深色 390px 读取同一持久状态，可从提示直接进入编辑，保留阈值。未出现页面脚本错误。
- 生成合同、TypeScript、中英文词条和文档检查通过。

```powershell
go test -p 2 ./internal/platform ./cmd/suma-contracts -run '^(TestAutomationFailurePause|TestAutomationAndStreamContracts)' -count=1 -v
go run -p 2 ./cmd/suma-contracts -check
pnpm --filter @suma/web test src/AutomationFailurePolicy.test.tsx src/AutomationOccurrences.test.tsx src/automation-model.test.ts --minWorkers 1 --maxWorkers 2
pnpm --filter @suma/web typecheck
pnpm --filter @suma/web check:i18n
node scripts/check-docs.mjs
```

本阶段在下一次实际派发前评估保护条件。跨进程与其他数据库的规模验证、收件箱及外部通知仍待交付或实测。
