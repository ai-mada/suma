# 执行工具自动发现验证

2026-10-29 · T-DISCOVERY-132。基线`6d02ab6`加本阶段明确所有权的源码，在独立副本验证；未混入正在开发的终端、候选和桌面改动。

选中已授权节点后，页面自动读取当前Runner已上报的非秘密工具；唯一可用项自动选中并建议可编辑名称。人工清单／名称、文件读取与后到响应分别保护；更换节点清除自动来源，手改后可明确恢复同一份仍有效的报告。文件／粘贴作为折叠后的备用入口。所有登记仍默认暂停，启用需要单独确认。

新增只读接口复用原库存和权限事务。私有节点核对当前凭据／来源链；共享节点核对当前路由、准确Grant版本、Pool／Runner及有效凭据。原数据库接收时间60秒内才返回工具；过期、未上报、空上报、失权和内容损坏分别处理。读取不执行探测、不安装工具、不刷新历史上报时间、不授予权限。

## 已执行验证

- Web80项：唯一清单解析33、发现组件16、实际工具工作区21、私有／共享身份3、接入指导7。79项组合检查通过，随后新增中英文数量／时间实际插值回归，发现组件16项复核通过；唯一用例合计80。原新功能测试先失败后实现，手工重新采用和插值问题均保留先红后绿证据。
- Go服务8项及HTTP3项共11项通过，涵盖原数据无业务副作用、无上报／空上报、60秒／未来／损坏时间、重放不续期、撤权／过期／浏览器权限、Pool路由与Grant／凭据失效，以及损坏／超限库存。既有`TestToolProfile`回归通过。
- `cmd/suma-contracts`全套通过；生成并核对690个模型、321条API路径及TypeScript wire types。单一公开合同来自Go，没有新凭据、调度器、第二份秘密配置或账本。
- 最终隔离源码完整Web类型检查通过；6513项配对语言资源／占位符／标记检查通过，生产硬编码0；生产构建通过，保留既有大分块提示。初次共享工作区Web／Go检查遇到其他在途终端的生成类型不匹配；早期扩展检查也曾因磁盘空间及既有authority source检查失败而中止，没有改写这些失败或报告整体通过。完整Web／平台全套未重跑，最终结论限于已提交基线与本阶段源码。
- 真实本地控制服务、隔离SQLite、实际Node只读探测与Runner：浏览器登记节点→尚未上报提示→启动Runner→自动读取／填入→人工改名及手改后恢复→暂停登记→明确启用→撤权后发现403且档案不可派发，全部通过。一次受控503注入验证错误显示和明确重试；不代表生产故障可用性测试。
- 1440桌面与390中英窄屏、无横向溢出、弹窗滚动与固定操作区分别实见，页面错误0。已检查自动填入、窄屏英文和失败恢复截图。没有商业模型调用，没有任务执行或生产验收结论。

## 复现与边界

Web命令：`pnpm exec vitest run src/ToolWorkspace.test.tsx src/ToolDiscoveryPanel.test.tsx src/tool-model.test.ts src/execution-pools/tool-identity.test.tsx src/ToolSetupGuide.test.tsx`；插值修复后复核`ToolDiscoveryPanel.test.tsx`。类型与构建使用当前TypeScript／Vite及独立输出；Go1.26.10、Node22.16.0、Chrome和本地Playwright，测试TEMP/TMP/GOTMPDIR仅进程内指定。Go命令：`go test ./internal/platform -run '^TestToolDiscovery' -count=1`、`go test ./internal/platform -run '^TestToolProfile' -count=1`、`go test ./cmd/suma-contracts -count=1`、`go run ./cmd/suma-contracts --check`。

服务最多读取2MiB／128项；Web继续使用唯一清单解析器的256KiB整体上限，超限明确失败，不截断。读取已有真实上报，不远程自动安装或触发探测；工具配置不代表模型登录／调用健康。自动检测、暂停登记、可派发与正式验收分别记录。

证据及源码哈希清单位于`dist/quality/tool-discovery-132/`。独立审阅覆盖服务／组件、表单／HTTP；两项实际交互显示问题已修复并增加回归。两阶段普通远程推送被自动审核以缺少当前明确外发授权拒绝；仅完成本地commit，未绕过审核。操作见[中文指南](../development/tool-discovery.md)／[English](../en/tool-discovery.md)，结构见[SVG](../design/management-and-tool-discovery.svg)。
