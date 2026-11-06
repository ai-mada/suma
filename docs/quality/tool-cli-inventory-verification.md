# 终端节点上报与档案复用验证

T-TOOL-CLI-INVENTORY 将指定节点的已保存上报与精确档案匹配串联。使用原用户和项目权限读取，按服务端剩余时长复核新鲜度，再选取原命令的完整配置。 [操作](../operations/task-dispatch-options.md#从节点上报查找已有工具档案) · [合同](../../contracts/tool-profile-reuse.md#从当前节点上报复用档案)

## 定向检查

- Windows amd64、Go 1.26.10。节点读取的 3 项顶层测试和 15 个子用例通过，覆盖准确项目及节点、原视角、有效期、重复命令、无效档案及拒绝。`read/green.json` 与 `read/green.log` 记录测试输入和结果。
- `go test -overlay … ./internal/cli -run '^TestToolMatches' -count=1 -v` 通过 6 项顶层测试，含 3 项新增回归和 3 项原文件输入检查。覆盖普通与共享 Worker、参数歧义、错误范围、未上报、过期、传输耗尽有效期、缺少指定命令和不可用。测试核对无效依据不会发起匹配请求。`matches/green.json` 与 `matches/green.log` 记录原件。
- 两组新测试先复现命令缺失或旧行为。范围限定在现有两条只读接口及其 CLI 调用。测试及临时目录均在 `.cache/tool-inventory-reuse/`。

## 实际共享节点联调

在 `1655ce7` 上用 Go overlay 构建实际 `suma.exe`，通过回环 HTTP 调用真实控制面和 SQLite。测试建立双组织共享池、当前授权、准确节点和已登记暂停档案。共 9 次 CLI 调用，发出 9 次 GET 和 1 次只读匹配 POST。

验证未上报、新鲜、过期三种状态。新鲜上报的准确命令匹配到原暂停档案。未上报、未知命令、不可用或过期均仅发出库存 GET，撤回共享授权后返回 403。每次调用前后核对八张业务表的完整指纹，结果一致。命令、输入 SHA256、结果与清理记录位于 `.cache/tool-inventory-reuse/actual/evidence/`。

本轮覆盖普通与共享节点的终端读取、复用门禁及 SQLite 下的实际授权。物理多机、厂商真实启动及生产规模继续沿原矩阵验收。
