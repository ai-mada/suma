# 从执行详情核对工具配置

T-RUN-TOOL-RETURN 于 2026-11-06 在 Windows、Node 22.16.0、Go 1.26.10、SQLite 与 Chrome 157.0.8103.38 完成定向验证。服务端使用 `5914ee1` 的隔离快照，前端使用实际工作台与服务端语言目录。

- 在 `apps/web` 运行 `node node_modules/vitest/vitest.mjs run src/ToolBindingStatus-consumers.test.tsx src/display-diagnostic-consumers.test.tsx src/tool-model.test.ts src/ToolWorkspace-experience.test.tsx --maxWorkers=1 --minWorkers=1`。共 63 项，覆盖四类节点、无绑定不读取、配置不匹配、读取失效、原导航保护和尝试链接编码。旧暂停确认夹具改用当前授权诊断，并检查切换语言后保留确认与焦点。
- `node node_modules/typescript/bin/tsc --noEmit` 和 `node scripts/check-i18n.mjs` 通过。
- `go test -ldflags '-s -w' -overlay <阶段验证夹具> ./internal/platform -run '^TestRunToolReturnActual$' -count=1 -timeout=120s -v` 通过。服务端创建报告型测试工具、登记库存并记录两次失败尝试，随后暂停配置。项目只读成员在英文亮色桌面用键盘和中文暗色 390px 界面用触屏，从第 1 次尝试打开准确配置并返回。两次返回都显示原尝试的错误，节点最新尝试仍为第 2 次。浏览器请求只读，SQLite 持久记录摘要保持一致。

截图和结构化回执保存在阶段证据目录。执行结果由受控 Runner 夹具上报。实际厂商进程、生产工具节点和多机验收保持各自的待验证范围。
