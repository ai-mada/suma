# 独立测试工具选择时检查验证

T-TEST-DISPATCH-PREFLIGHT。Windows、Go 1.26.10、Vite 6.1.0、Chrome 156.0.8078.80 与隔离 SQLite。控制面和直接派发弹窗使用 `cf3287f` 加本阶段源码。

- `go test ./internal/platform -run '^TestTaskDispatch(TestOption|Options|ExplicitTests|Route)' -count=1 -v` 通过 14 项与 29 项子用例，现有一个需显式开启的实际 Worker 用例跳过。新用例验证当前项目写权限、严格查询、准确返回值与正式预览一致、同 Worker 拒绝、JUnit 和库存约束、当前共享池路由隔离，以及无计划、运行、用量写入。发布流程保持原定义。
- `go test -overlay <阶段配置> ./cmd/suma-contracts -run '^TestTaskDispatch' -count=1` 通过两项。生成检查核对 1073 个模型与 482 条路径，差异限定为独立测试核对 GET 和两个相关模型。
- 在 `apps/web` 运行 `node node_modules/vitest/vitest.mjs run src/TaskDispatchTestBindings.test.tsx src/task-dispatch-test-bindings-model.test.ts src/TaskDispatchOptionsPanel.test.tsx --maxWorkers 2 --minWorkers 1 --no-cache`，通过 9 项。覆盖原版本复核、跨来源拒绝、撤权清除、未知响应和既有待确认预览。讨论入口 3 项与专家团队入口 1 项通过，覆盖复用同一核对及任务版本变化后清除旧绑定，保留专家团队原方案模式。整库 TypeScript 在 `cf3287f` 加本阶段源码、隔离 12 个已跟踪和 1 个新增的并行改动文件后通过。初次类型检查发现两处共享调用缺少任务版本，补齐后复验。并行仓库交付提交 `d733a16` 进入 `dev` 后，以合并后的工作区再次运行 `node node_modules/typescript/bin/tsc --noEmit`，无诊断。中英 10580 对词条通过。浏览器首轮发现旧库存文案会与当前拒绝冲突，移除后完成定向测试和同一路径复验。
- 真实 Chrome 与控制面通过 5 项。中文桌面显示准确测试绑定、路由和读取时间。英文 390px 窄屏使用键盘复核另会话暂停过的工具，保留原选择，禁止提交预览并显示最新冲突。三次浏览器核对为 200、200、409，均禁止缓存。零计划或运行写入，零意外浏览器错误。工作台其余入口由组件测试和类型检查覆盖。
- 最终报告 `.cache/test-dispatch-preflight/browser-evidence-zuwNdz/result.json`，SHA256 `3057c4f6568fdf5ab80ec729df48e6fc205ab79ffee428dd68187ee8ec8c251f`。同目录保留桌面与窄屏截图。控制进程、Chrome 和回环端口已释放。盘点后清理 324 个浏览器临时文件，收据 `.cache/test-dispatch-preflight/browser-cleanup.json`；生成验证清理收据 `.cache/test-dispatch-preflight/contract/scratch-cleanup.json`。

本阶段覆盖选择时的配置兼容性。正式预览与启动各自复核，实际执行、外部模型、多主机和生产验收继续沿原合同。
