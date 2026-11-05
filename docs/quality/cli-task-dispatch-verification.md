# 终端任务派发验证

T-CLI-TASK-DISPATCH。Windows、Go 1.26.10、实际编译的 `suma.exe` 与 `suma-control.exe`、回环 HTTP 和隔离 SQLite。源码以 `1af1de6` 及本阶段生产文件构建，隔离并行交付中的源码。

- `go test -overlay <阶段配置> ./internal/platform -run '^TestTaskDispatch(Options|TestOption|ExplicitTests|Route)' -count=1 -v` 通过 15 项及 31 项子用例。旧实测 Worker 用例需显式启用，本次跳过。复现项目 API 凭证此前在两个读取接口均得到 403，修复后核验项目及租户隔离、Agent／Runner 拒绝、管理员降为项目查看者后的差异权限、撤销，以及读取前后计划／运行／用量数相同。
- `go test ./internal/cli -run '^TestTaskDispatch(CLI|OptionsCLI)' -count=1 -v` 通过 3 项。覆盖准确查询、请求正文和操作键、原键重传、无效版本、重复 JSON 键和项目冲突。生成验证 `go test -overlay <阶段配置> ./cmd/suma-contracts -run '^TestTaskDispatch' -count=1` 通过 3 项。生成检查核对 1073 个模型与 482 条路径，仅调整两个读取接口的身份与缓存说明。
- `go build -overlay <阶段配置> -o <阶段目录> ./cmd/suma ./cmd/suma-control` 编译两个实际二进制，`node .cache/cli-task-dispatch/actual-verify.cjs` 通过 6 项，共调用 CLI 12 次。使用真实项目 API 凭证取得工具选项和独立测试绑定，旧测试工具版本返回 409，异项目返回 403。同键预览只保留一个计划；审阅、缺操作键和使用旧计划版本时运行数为 0。明确启动后，同键重放返回同一运行。最终 1 计划、1 运行、0 Worker 尝试、0 用量记录。
- [终端命令与请求示例](../operations/task-dispatch-options.md#终端任务派发)。实际联调报告 `.cache/cli-task-dispatch/actual-evidence-VZFfUk/result.json`，SHA256 `6bf5d002c48512b26ccfa6fa94d3230a24587f3dcddd8093824ba801ca4af4e1`。控制面及回环端口已释放。23 个实际联调临时文件已盘点清理，收据 `.cache/cli-task-dispatch/actual-cleanup.json`；生成检查清理收据 `.cache/cli-task-dispatch/contract/scratch-cleanup.json`。

本阶段验证终端配置、计划准入与原操作恢复。实际 Worker、外部模型、多主机与生产验收继续沿原合同。
