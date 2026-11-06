# 工具目录检索验证

T-ADAPTER-DISCOVERY 将 Web 和终端接到同一个项目授权目录。Web 支持跨方向浏览及精确筛选，终端输出保留原能力和限制的 JSON。 [目录合同](../../contracts/adapter-catalog.md#web-与终端目录检索) · [交互与命令](../design/adapter-connections.md#信息层级与异常恢复)

## 定向检查

- Windows amd64、Node 22.16.0、Vitest 3.0.5。`node node_modules/vitest/vitest.mjs run src/AdapterConnections.test.tsx src/AdapterConnectionsIntegration.test.tsx` 通过 15 项。新回归先复现旧行为。验证跨方向及多词检索、精确配置和原权限，并更新旧集成夹具以经过当前外层折叠与配置入口。
- `node node_modules/typescript/bin/tsc --noEmit` 通过。`node scripts/check-i18n.mjs` 核对 10667 个词条，未发现语言键或生产硬编码问题。
- Go 1.26.10。CLI 及相关工具读取定向检查通过 7 项，覆盖本地筛选、双语帮助、错误项目、无效响应和已有档案查询。原基线复现命令缺失。准确过滤器和命令保存在 `.cache/adapter-discovery/cli/`。

## 浏览器与终端

隔离 Chrome 156.0.8078.80 与 Playwright 1.62.1 在英文 1366×768 和 390×844 复核 6 项。真实目录组件经合成回环接口读取，覆盖 Zed 精确匹配、方向筛选、结果数、详情及准确指南、焦点返回与访问拒绝。首轮发现搜索输入列被压缩，修正后两种宽度均通过输入几何检查。最终 2 次 GET、0 次写入、0 次非预期浏览器错误。截图、复验摘要和清理记录位于 `.cache/adapter-discovery/browser/`。

在 `bef640b` 上构建实际 `suma.exe`，向本机真实目录 HTTP 处理器与 SQLite 发出 4 次命令。项目 API 凭证查到唯一 `zed-client`，同一查询切到执行方向返回空集合。其他现有项目返回 403，原凭证撤销后返回 401。四张业务表的读取前后指纹一致。 [原操作](../design/adapter-connections.md#信息层级与异常恢复)

实际终端和合成浏览器证据分别保存在 `.cache/adapter-discovery/actual/evidence/` 和 `.cache/adapter-discovery/browser/`。目录查询与配置入口已验，真实厂商安装、账号连接、任务执行和生产部署继续按原矩阵验收。
