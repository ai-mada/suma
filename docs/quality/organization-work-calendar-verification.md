# 组织人员工作日历验证

T-ORG-WORK-CALENDAR。Windows、Go 1.26.10、Vite 6.1.0、Chrome 156.0.8078.80 和隔离 SQLite。

- `go test -overlay <阶段隔离配置> ./internal/platform -run '^TestOrganizationWorkCalendar' -count=1 -v` 通过 5 项，含 4 个初始结构子场景。覆盖未知与明确零、日期例外、原回执和跨连接竞争、权限降级／撤销、跨组织、浏览器范围与关闭参数、严格整数及重复字段，以及异常结构在初始化写入前拒绝。
- `go test -overlay <阶段隔离配置> ./cmd/suma-contracts -run '^Test(OrganizationWorkCalendarContract|ResourceCalendarContract)' -count=1` 通过两项。`suma-contracts -check` 核对 1066 个模型、480 条路径。结构比对确认新增组织日历 GET／PUT 及两个类型，复用原工作制与例外结构。
- `node node_modules/vitest/vitest.mjs run src/OrganizationWorkCalendarWorkspace.test.tsx src/ResourceCalendarEditor.test.tsx src/resource-calendar-model.test.ts --maxWorkers 2 --minWorkers 1 --no-cache` 通过 14 项。四项工作区用例在整理测试时序后复验通过。管理导航的五项回归通过，整库 `tsc --noEmit` 与 10521 对双语词条通过，生产硬编码 0。
- Chrome 和隔离 SQLite 最终 9 项通过，零运行错误。管理员键盘保存目标成员的版本 2 后，当周例外产生 17.75 小时，下一周普通工作制为 21.75 小时；旧请求返回版本 1 原回执。普通成员只能读本人，在 390px 英文深色界面无编辑入口。目视同时复核 1366px 中文明亮界面。首次 Chrome 在受限环境创建页面前退出，使用同一构建在受控本机复验成功。
- 最终浏览器报告 `.cache/org-work-calendar/browser-evidence-q9rStp/result.json`，SHA256 `260bb51c819aa8c61bb6c1176497677232ce8af017fc13883c4571210f968dc7`。控制面基于 `e0c5d36` 加本阶段隔离源码。Chrome、控制进程和回环端口均已释放。盘点后清理 296 个浏览器临时文件，保留报告、截图、验收脚本及 `.cache/org-work-calendar/browser-cleanup.json`。生成验证的 11 个临时文件已盘点清理，收据位于 `.cache/org-work-calendar/contract/scratch-cleanup.json`。

本阶段提供组织级可用工时依据。组织计划负荷、工时超配、外部假期和跨数据库验证继续按实施计划推进。
