# Codex App Server 远程换新验证

T-CODEX-MANAGED-REPIN · Windows amd64 · Go 1.26.10 · SQLite · Chrome 157。Go 验证使用 `9e2bc58` 的隔离快照及本阶段源码，网页使用现有节点检测和维护组件。

- 定向 Go 检查覆盖唯一模板、原扫描摘要、原操作恢复、普通节点与共享池权限及目录回执要求。修复并复验空准备领取及其与安装扫描的竞争。已保存的空回执可按原标识重放，新的领取能处理后来加入的准备任务。隔离探测下的只读 Git 核对明确使用管理员登记的仓库，保留原工作树校验。
- 19 项 Web 组件检查、定向 TypeScript、双语词条、生成合同及文档链接检查通过。并行自动化改动在当前共享工作区尚未生成对应类型，全量 Web 类型检查另有该模块的错误。
- 真实 Windows 联调使用本机 `codex-cli 0.167.0-alpha.3.2`。390px 中文暗色 Chrome 从准确扫描行选择配置并明确确认，只有一次准备提交。普通 Runner 复用本机唯一配置，通过 App Server 读取 9 个模型目录条目，原库存回执完成准备。界面核对新库存并交接准确配置，再通过原接口保存暂停档案。模型供应方请求和业务运行均为 0。
- 初次联调先修正浏览器独立环境及临时仓库的忽略规则，再定位维护空回执和隔离 Git 仓库选择的问题。最终结果使用生产源码与独立联调夹具，未叠加诊断代码。

```text
go run -ldflags '-s -w' ./cmd/suma-contracts --check
go test -ldflags '-s -w' ./internal/runner ./internal/toolpreparationcontract ./internal/platform ./cmd/suma-contracts -run '^(TestManaged(Codex|Preparation|CLI|Generation|ToolPreparation)|TestInstalledCommand)' -count=1
go test -ldflags '-s -w' ./internal/platform -run '^(TestManagedPreparation|TestManagedCodexAppServerPreparation)' -count=1 -timeout=100s
go test -ldflags '-s -w' ./internal/runner -run '^(TestManagedCodexPreparation|TestManagedPreparation|TestManagedGeneration|TestDescribeTools|TestToolInventory)' -count=1 -timeout=90s
go test -ldflags '-s -w' -overlay <阶段夹具> ./internal/platform -run '^TestCodexManagedRepinActual$' -count=1 -timeout=100s -v
node scripts/check-docs.mjs
```

真实联调覆盖单台 Windows 普通节点。共享池走 SQLite 协议回归，物理多机、厂商模型调用及正式部署仍待验。阶段目录保留截图、脚本及结构化回执。

[交互设计](../design/managed-tool-preparation.md#codex-app-server-换新) · [维护合同](../../contracts/managed-tool-preparation.md#codex-app-server-换新)
