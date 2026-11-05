# WorkBuddy 终端进度验证

T-WORKBUDDY-CLI-PROGRESS 为团队自动化提供指定节点的只读记录。已保存的供应方状态和平台运行状态分别返回。固定原因码、查询范围和响应字段经过校验，未知外部作用仍保留待核对状态。 [领域合同](../../contracts/workbuddy-cloud-execution.md#终端读取固定节点进度) · [终端操作](../operations/task-dispatch-options.md#终端运行跟踪)

## 定向验证

- Windows amd64、Go 1.26.10、Node 22.16.0。平台测试 `go test -overlay … ./internal/platform -run '^TestWorkBuddyCloud(Progress|HTTP|Node)' -count=1 -v` 通过 10 项顶层测试，覆盖浏览器和项目 API 当前授权、已准备及未知提交、保留的旧观察、响应闭合和原浏览器权限。新增请求摘要冲突和方法拒绝断言后，3 项进度测试复验通过。原基线复现新路径返回 404。
- CLI 定向回归通过 17 项顶层测试和 24 项子测试。原基线复现未知命令。新路径覆盖严格参数、准确响应范围、缺字段和未知字段拒绝、当前拒绝与双语帮助。执行输入与过滤器见 `.cache/workbuddy-cli-progress/cli/green.json`。
- `go test -overlay … ./cmd/suma-contracts -run '^TestWorkBuddyCloud' -count=1 -v` 通过 8 项。对 `36ff02b` 的隔离生成只增加一条 OpenAPI 路径、一个闭合模型及对应 TypeScript 类型。其余合并基线合同一致。生成器 `-check` 校验 1077 个模型和 490 条路径通过。 [生成路径](../api/openapi.json)

## 实际终端与控制面

在 `36ff02b` 上冻结 8 个生产输入，通过 Go overlay 编译实际 `suma.exe`，向回环 HTTP 上的真实平台处理器与 SQLite 发送四次命令。项目 API 凭证读取 `not_recorded`，随后读到已保存的 `completed` 观察时平台运行仍为 `running`。错误项目返回 403，原凭证撤销后返回 401。输出中未出现测试用的私人值。

读取前后的八张业务表指纹一致。四次读取期间供应方创建与查询计数均为零。已有观察由受控本机供应方夹具在查询前写入。第一次联调通过时并行提交更新了基线，已在新基线上复验。构建输入、命令、结果和清理回执位于 `.cache/workbuddy-cli-progress/actual/evidence/`。

证据覆盖当前控制面和 SQLite 上的只读链路。真实供应方、物理多机、生产权限与外部协作端到端仍按各自验收矩阵跟进。
