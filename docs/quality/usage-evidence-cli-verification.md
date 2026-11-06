# CLI 用量尝试与证据验收

状态 T-USAGE-EVIDENCE-CLI-281。版本 1.0.0。范围为 CLI 读取存档用量和证据。操作入口和续页示例见[中文 SOP](../operations/model-execution.md#在终端核对用量证据)及[英文说明](../en/model-execution.md#read-usage-evidence-from-the-cli)。

`usage attempts` 支持账月和状态筛选。使用列表返回的 `id` 运行 `usage evidence`，按准确的尝试读取原始结果、后续观察和人工确认。两条命令复用项目授权、工作台登录会话和原操作者核对。CLI 返回结构化 JSON，保留证据原序、显式零值和精确整数。 [用量合同](../../contracts/usage-accounting.md)

## 定向验证

- 在 Windows、Go 1.26.10 上启动真实本地 HTTP 控制面，使用隔离 SQLite、测试账号和预置账本。新增命令前用例因未知命令失败。实现后核对账月和状态筛选、两级续页、相同时间与小数秒证据的顺序、空列表、原始结果和人工确认标记、跨尝试游标及会话身份不符。SQLite 触发器禁止读请求写入用量表。
- 回环响应覆盖范围不符、重复字段、未绑定游标、越界计数、未知字段和私密错误详情。参数错误在访问服务前被拒绝。
- 构建 `suma`，核对中英文子命令帮助。文档本地链接及 API 引用通过检查。

```powershell
go test -p 2 ./internal/cli -run '^TestUsage' -count=1 -v
go build -p 2 -o .cache/q281/suma.exe ./cmd/suma
.cache/q281/suma.exe --language zh-CN usage --help
.cache/q281/suma.exe --language en-US usage --help
node scripts/check-docs.mjs
```

本次证据覆盖本地读取。其他数据库、远程部署和厂商账单仍待独立验证。
