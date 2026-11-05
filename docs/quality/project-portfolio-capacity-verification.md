# 跨项目周负荷与容量验证

T-PORTFOLIO-CAPACITY。Windows、Go 1.26.10、Vite 6.1.0、Chrome 156.0.8078.80 和隔离 SQLite。控制面基于 `fecb97b` 加本阶段源码。

- `go test ./internal/platform -run '^Test(ProjectPortfolio|ProjectScheduleOverlap|OrganizationWorkCalendar)' -count=1 -v` 通过 18 项及 4 项子用例。核对按完整日期区间和例外加权、周外未知例外、明确零与未知、当前调用者／负责人授权和租户隔离、只读凭证边界、恰好 2000 项和 2001 项拒绝。不可见项目不参与结果或上限。
- `go test -overlay <阶段配置> ./cmd/suma-contracts -run '^Test(ProjectPortfolio|ProjectScheduleOverlap)' -count=1` 通过 3 项。`suma-contracts -check` 核对 1070 个模型、481 条路径；与父提交相比只增加容量 GET 和四个模型。首次测试把公共语言请求头也计入查询参数，按参数位置校验后复验通过。
- `node node_modules/vitest/vitest.mjs run <阶段相关文件> --no-cache` 通过 18 项，包含周容量 4 项、项目组合 5 项、排期重叠 5 项和组织工作日历 4 项。整库 `node node_modules/typescript/bin/tsc --noEmit` 通过。此前组织日历的测试在未确认请求存在时调用 `Object.keys`，补上显式检查后复验通过。中英 10565 对词条检查通过；微小工时文案用文字表达，避免模板把小于号解释为标记。
- Chrome 实际工作台及隔离 SQLite 通过 13 项，零浏览器错误。覆盖 1366px 中文明亮、390px 英文深色、键盘筛选和切周、准确任务及日历导航、浏览器返回和撤权刷新。演示数据中，成员周容量为 36 小时，当前可见任务共 44 小时；撤销一个项目的访问权后变为 20 小时，原任务从明细移除。已目视核对桌面和窄屏任务明细。受限环境首次在创建浏览器页面前退出，最终在获准的本机 Chrome 验证。Vite 提示已有较大产物分块，构建成功。
- 最终证据 `.cache/portfolio-capacity/browser-evidence-3bVgMW/result.json`，SHA256 `486ad8a865b15b82f78027d5126c837860026cd059ceca7f70ab937c6a8a870c`。同目录保留桌面、窄屏和窄屏明细截图。浏览器、控制进程和回环端口已释放，408 个浏览器临时文件盘点后清理，收据 `.cache/portfolio-capacity/browser-cleanup.json`。生成验证临时目录盘点与清理收据 `.cache/portfolio-capacity/contract/scratch-cleanup.json`。

周视图只统计当前可读项目中起止日期齐全的未结任务，2000 条上限内提供完整结果。组织总容量治理、外部工作日历、趋势预测、其他数据库和规模验收仍按实施计划推进。
