# Codex App Server 模板选择验证

T-CODEX-PREP-TEMPLATE-CHOICE · Windows amd64 · Go 1.26.10 · SQLite · Chrome 157。Go 使用 `d562b44` 的隔离源码快照及本阶段文件。

- 普通节点和共享池核验单模板解析、多模板明确选择、未知模板拒绝、同代次候选撤下时停止原待领取请求，以及重放保持原回执。Runner 核对候选排序、上限、准确本机参数、安装摘要和原代次恢复。
- 两组 Web 组件共 20 项检查通过，覆盖切换选项撤销旧确认、错模板回执及按原请求恢复。定向 TypeScript、双语词条、生成合同和文档链接通过。共享工作区全量 TypeScript 检查另有并行通知改动的合同错误。
- 390px 中文暗色 Chrome 先选择 `codex-dev` 并确认，切换到 `codex-review` 后观察确认取消，重新确认后只提交一次。Windows 普通 Runner 使用实际安装的 `codex-cli 0.167.0-alpha.3.2`，读取 9 项模型目录。控制面对比所选模板的准确配置摘要，库存确认后交接至暂停登记。供应方模型请求与业务运行均为 0。
- 首次实际联调发现深目录下 Git 读取临时主目录配置超出 Windows 路径长度。维护探测复用原隔离 Git 环境后在同一夹具复验通过。最终联调未加载诊断补丁。

```text
go run -ldflags '-s -w' ./cmd/suma-contracts --check
go test -ldflags '-s -w' ./internal/runner ./internal/toolpreparationcontract ./internal/platform ./cmd/suma-contracts -run '^(TestManaged(Codex|Preparation|CLI|Generation|ToolPreparation)|TestInstalledCommand)' -count=1 -timeout=120s -v
go test -ldflags '-s -w' ./internal/runner -run '^(TestManagedCodexPreparation|TestManagedPreparation|TestManagedGeneration|TestDescribeTools|TestToolInventory)' -count=1 -timeout=90s
go test -ldflags '-s -w' -overlay <阶段夹具> ./internal/platform -run '^TestCodexPreparationTemplateChoiceActual$' -count=1 -timeout=100s -v
node scripts/check-docs.mjs
```

截图和结构化回执保存在阶段证据目录。共享池为 SQLite 协议验证，物理多机、真实供应方模型调用及生产部署仍待验。

[工具交互](../design/managed-tool-preparation.md#codex-app-server-换新) · [接口与权限](../../contracts/managed-tool-preparation.md#codex-app-server-换新)
