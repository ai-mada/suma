# T-P4-036 开发工具连接检查限定验证

实现：`internal/cli/developer_connection.go`，入口 `suma ... developer check`。边界及中英文用法见[操作说明](../integrations/developer-connection-check.md)。远程工作区 Broker 为[后续设计](../architecture/remote-workspace-broker.md)，未实现。

2026-10-27，Windows 仓库私有 Go 1.26.10、现有官方 MCP Go SDK v1.8.0；进程级 `TEMP`／`TMP`／`GOTMPDIR` 位于 `.cache/developer-check036/tmp`，缓存使用仓库已有忽略目录，未修改全局环境。

- RED：新增真实 SQLite／控制面用例后，现有 CLI 在 `developer check` 返回 unknown-command／退出 2，符合缺失功能。
- GREEN：`go test ./internal/cli -run '^TestDeveloperCheck' -count=1` 通过。真实控制面分别使用当前 Agent 与项目 API 凭证完成初始化、当前工具目录、准确项目概览；API 目录虽声明写能力，实际协议仍只调用固定只读 overview。任务／需求／DAG 运行／测试表均无新增；撤销原 API 凭证后再次检查退出 1，无成功报告。
- 必要拒绝边界：官方 SDK 服务返回错项目、工具错误或未声明只读时不确认连接；HTTP 重定向没有到达第二服务，超时与超量响应不重试；无项目、非法项目、缺失凭证、远端明文及自定义 endpoint 参数均在发起请求前拒绝。错误及报告不披露凭证或服务器错误原文，CN／EN 分支均有执行。
- `go test ./internal/cli -count=1` 通过，覆盖完整 CLI 包；`git diff --check` 通过。未重复无关平台完整套件、正式任务测试、模型或厂商调用。主预览／数据库／原会话未改动。

执行环境及并行集成记录：PATH 中 Go 1.24.5 首次因版本不足未启动测试，随后使用上述仓库私有 Go 1.26.10 完成 RED／GREEN。后续 `go vet ./internal/cli` 遇到并行新增 `model_execution.go` 的暂态 `undefined: repeatedFlags`；集成者已补该分支并负责最终共享编译／vet，本子任务不将该次 vet 报为通过。最后仅将非超时故障夹具的请求截止时间放宽至 2 秒以减少负载噪声，产品代码不变；按集成者要求复用既有 CLI 验证，不重复整包测试。

此证据是限定本地真实 MCP 调用及受控故障验证，不是厂商、物理多机、IDE 客户端常驻连接或完整远程工作区验收。
