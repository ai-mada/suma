# WorkBuddy 产物时间线验证

T-WORKBUDDY-ARTIFACT-TIMELINE，2026-11-05。[合同](../../contracts/workbuddy-cloud-turns.md) · [交互](../design/workbuddy-cloud-turns.md)。

- Windows Go 1.26.10，运行 `go test ./internal/platform ./cmd/suma-contracts -run '^(TestCloudTurnProjectionArtifactPreservesOnlySafeObservation|TestCloudTurnRuntimePermissionUnblocksPromptHTTPAndRedactsSplitSecret|TestCloudTurnContractKeepsOwnerOnceOnlyAndTypedControls)$' -count=1`，3 个顶层用例通过。真实 ACP 解码和受控 HTTP/SSE 传输确认原会话能收到产物通知，权限、原回合完成与已知凭据脱敏正确。传输测试使用内存桥记录回合事件，供应商服务和数据库验收各自独立。
- `go run ./cmd/suma-contracts` 生成 1014 个模型和 458 条 API 路径。初次使用系统 Go 无法在用户缓存下载 toolchain，后续使用项目内已存在的 Go 1.26.10 和模块缓存。构建和测试临时目录均在任务忽略目录。
- `apps/web` 运行 `npm.cmd run test -- src/WorkBuddyCloudTurns.test.tsx src/workbuddy-cloud-turns-model.test.ts`，16 项通过。类型检查通过。国际化校验 10,028 对词条，生产硬编码 0。
- 运行 `node .cache/workbuddy-artifact-timeline/browser.cjs`。使用现有 Playwright Core、本机 Chrome、生产组件和合成回环 HTTP。中文浅色 1366px 与英文深色 390px 各通过一组，验证键盘展开、类型与动作、事件序号、纯文本、原回合状态和无横向溢出。共四个 API 读取、零写入、零页面与控制台错误，浏览器及服务器关闭。构建记录包含既有 Radix 客户端指令提示。
- 浏览器回执 `.cache/workbuddy-artifact-timeline/browser-Z68rky/result.json`，SHA256 `248c793c65f1c87fca00c896ee1979c9f5a5c221fc7757251145732ea0eae69f`，目录内留存两幅图。

本阶段覆盖原回合的产物事件和受限预览。完整产物清单、真实文件接纳与供应商实机验收继续按完整计划推进。
