# 自动化人工触发权限复核验收

状态：T-AUTOMATION-MANUAL-AUTHORITY-278 已通过本机定向验证。版本：1.0.0。范围：人工触发入队、调度和历史原因显示。

## 行为

人工触发入队时，同事务保存当前已认证的发起人与原触发的关系。实际派发在现有权威锁内重读该关系，并复核发起人当前的项目写权限和规则启用人的当前写入及审批权限。发起人只需写权限。定时触发和签名事件各自沿用原权限来源。

发起人权限失效、或原触发缺少可核验的发起人记录时，阻止该条触发并记录具体原因。规则本身保持可继续调度；其他授权触发可依序推进。历史中的已开始运行仍按原记录查询。来源表随当前初始库结构初始化，历史触发缺失的发起人由调度按缺证处理。 [合同](../../contracts/automation-events.md)

Multica 的手动触发解析并校验实际发起人，再将该身份传递给派发。速码结合自身异步队列，将发起人授权与规则启用授权分别保存和核对。 [Multica 固定参考](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/server/internal/handler/autopilot.go#L2339-L2395)

## 定向证据

- 修复前真实 HTTP 和 SQLite 流程复现了发起人入队后降为只读、原触发仍创建运行的问题。
- 修复后验证了降权阻止、普通有权开发者正常派发、缺少原发起人时只阻止对应触发，以及重复调度保持原结果。覆盖 39 项 Go 顶层用例、35 项子场景，包含定时、签名事件、旧规则权限、竞争和重启回归。
- 19 项 Web 检查通过。中文与英文历史记录显示发起人相关的准确原因，保留原原因码，无运行关联时不显示跳转。
- 验收限本机 SQLite 与 Web 组件。OceanBase、PostgreSQL、物理多节点和生产规模仍待实测。

```powershell
go test -p 2 ./internal/platform -run '^(TestAutomation|TestAuthorityAutomation)' -count=1 -v
# 在 apps/web 下运行
pnpm exec vitest run src/automation-consumers.test.tsx src/i18n/automation-consumers.test.tsx
```
