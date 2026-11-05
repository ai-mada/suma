# 终端运行跟踪验证

T-CLI-RUN-WATCH。Windows amd64、Go 1.26.10、Node 22.16.0、回环 HTTP 与隔离 SQLite。实际二进制以 `b345fcf` 加本阶段五个生产文件构建，隔离并行应用发布改动。

- `go test -overlay <阶段配置> ./internal/cli -run '^Test(RunWatch|RunScheduling|RunQueue|RunStart|CommandFileHasStableOperationAndScope|OriginalViewIsExplicitAndStableAcrossRequests|RedirectDoesNotForwardToken|ProjectContext|TaskDispatch)' -count=1 -v` 通过 21 项及 29 项子用例。覆盖完整帧与重连、序号缺口和重复、精确数字与脱敏、权限退出、短写保留原游标、原会话请求头、现有 JSON 请求和派发命令。其后同范围的传输专项通过新增边界用例，覆盖 16 MiB 上限及协议边界。
- `go build -overlay <阶段配置> -o <阶段目录> ./cmd/suma ./cmd/suma-control` 构建实际二进制；`node .cache/cli-run-watch/actual-verify.cjs` 通过 7 项，实际调用 CLI 9 次。观察真实 HTTP 流的回环代理记录首次游标 0 与续传游标 2。单连接超时后，后续持久暂停事件 5 只输出一次；持有游标 5 的连接遇项目 API 凭证撤销，收到 401 `unauthenticated` 并退出，未再重连。
- 两页队列返回两个确切节点；节点准入分别显示执行能力待核对和运行已暂停。其他运行的游标返回 400，异项目凭证返回 403。隔离夹具最终记录 2 个运行、5 个事件、0 Worker 尝试、0 用量和 0 派发计划。观察命令沿原只读接口，暂停及撤销来自明确的夹具操作。
- 最终实际报告 `.cache/cli-run-watch/actual-evidence-zQr8mS/result.json`，SHA256 `70876c5389535f139339bd822e62b5df3fd85f79af35b9cfc72058d8bb4dd522`。控制进程和回环端口均已释放。盘点后清理 21 个实际联调临时文件，收据 `.cache/cli-run-watch/actual-cleanup.json`；集成测试收据 `.cache/cli-run-watch/regression/cleanup.json`。

本阶段验证终端事件传输、当前授权及调度查询。真实 Worker 运行、其他数据库、多主机和生产验收仍按现有合同推进。
