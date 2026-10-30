# 桌面托管 Runner：限定验证

阶段：**T-MULTICA-DESKTOP-153**；2026-10-30；suma 1.0.0。当前状态为 **Windows真实本机链路限定通过**，完整58项限定实现／34项待交付／92项不变。[合同](../../contracts/desktop-managed-runner.md)、[产品](../product/desktop-managed-runner.md)、[架构](../architecture/desktop-managed-runner.md)、[交互与AI SOP](../design/desktop-managed-runner.md)、[English](../en/desktop-managed-runner.md)及[机器摘要](desktop-managed-runner-summary.json)共同记录范围。

本阶段完成固定三工具入口的本机凭据托管和明确启动，继续151原库存、暂停登记及独立启用。实际GUI仅使用合成 CodeBuddy 兼容的只读探测入口；正式 Go Worker、原授权／fencing／outbox为真实实现，未创建业务任务或调用模型。

## 运行来源与产物

候选基线为 `fc4cebe057ef246454e2a5be3adad7c04a6a955a`。初始44个变更输入的源码摘要为 `eefb84a99ab85631fff8ead73570efd935fc4ce966449d9347c778233c192161`；修复库存时间解析后，46个变更输入的运行源码摘要为 `388a1a7dbc48cdeb5a518ae80ab89feb2e9f8a28d00883640fd18a9de7184427`。算法为按路径排序的变更文件 `{path,sha256}` 数组紧凑JSON的SHA256，未变源码由准确基线归档补齐；152云派发、其他在途改动和私有运行数据未混入。

Go control二进制始终为 `eb36303495faae44f0acf14d9028164029b2cb8d601453fd10300e9ad7f95ae1`。Web初版入口为 `aa35c5d34e60a57b8ff0b3f73af5e64144e4ff79a4acdd0767b9428d11869966`，修复版为 `68fc6a8a8838ab4af2ca1477d5c44a4ff82140316f223ad7dc7e07cac1d05882`。第二版仅变更解析器及其新回归文件，保留旧Web产物；Go与原生运行代码未重编。

上述摘要属于历史运行候选。后续主进程接线测试及本文等文档收口另行记录，不把最终提交文件反写成原编译输入。两份原生测试在工作树与历史运行候选中的摘要差异仅为各两处CRLF归一化；归一化后逐字相同。最终提交另外去掉两份测试的空白末行；生产控制器及journal源码原始字节摘要与运行候选一致，未为纯格式变化重跑测试。

## 检查分组与实际命令

| 分组 | 实际结果及计数口径 | 现有证据 |
| --- | --- | --- |
| 后端发行与凭据 | 18个顶层／19个子场景；10个发行场景及8个原凭据回归 | `.cache/managed153-backend-evidence.json`及两份JSONL；完整执行参数未保存在该摘要，不补造命令 |
| 工具库存时间精度 | 14个顶层／8个子场景去重通过；原时间字符串比较断言先失败，改为时间比较后仅复验该项 | `.cache/managed153-tool-time-evidence.json`及初跑／修正JSONL |
| 隔离候选Go | 29个顶层／34个子场景：control 4、runner 4、platform 21；exit0 | `candidate/go/20261030T072808122Z.command.json`及同名stdout |
| 中英诊断／目录 | 3个顶层通过，含1个已在候选中的平台诊断及2个Catalog检查 | `integration/go/20261030T071453107Z.command.json`及同名stdout |
| Web限定行为 | 本阶段原23项加新解析器2项为25项；新解析器2＋原面板16＋Quick10的28项是受影响复验，不能再加到25 | `.cache/managed153/ui/`的`focused.log`、`review-fixes.log`和`fractional-inventory.log` |
| 原生Node合同 | 29个新用例是初26项加后来3项的去重并集；15项既有安装／生命周期基线随初批执行。没有一次最终29或44项全跑；后续4／4／7项复验不累加 | `.cache/e2e/managed153/native-summary.json`；原stdout未落盘，只保留实际命令、结果和工具输出标识，不冒称原始日志 |
| Root桥接／接线 | 前序4项窄桥接检查加当前main接线1项，共5项，单列于原生29项之外 | main接线1项日志`.cache/e2e/managed153/main-bridge.txt`；前序4项为主集成实际工具回执 |

Go各组存在明确重叠：发行10项包含在候选平台组；精度与凭据回归重叠一项、与候选重叠一项；中英平台诊断也已在候选中。此处只分组报告，不把18／14／29／3粗加为总数。

以下命令均已实际执行，工作区与精确程序地址、环境及退出码保存在对应现存档案；本文收口未重跑：

```text
go build -trimpath -buildvcs=false -o D:/codes/ai-dev-os/suma/.cache/e2e/managed153/candidate/bin/suma-control.exe ./cmd/suma-control
go test ./cmd/suma-control ./internal/runner ./internal/platform -run '^(TestDesktop|TestToolInventorySubsecond)' -json -count=1 -timeout=90s
go test ./internal/platform ./internal/localization -run '^(TestDesktopRunnerDiagnostics|TestCatalog)' -json -count=1 -timeout=45s
npm test -- --run src/tool-discovery-model.test.ts src/ToolDiscoveryPanel.test.tsx src/ToolQuickConnect.test.tsx
node --test apps/desktop/tests/local-service-port.test.cjs apps/desktop/tests/managed-local-runner-journal.test.cjs apps/desktop/tests/managed-local-runner.test.cjs apps/desktop/tests/installed-tools.test.cjs apps/desktop/tests/lifecycle.test.cjs
node .cache/e2e/managed153/browser/continue.cjs
```

Go使用本机1.26.10工具链、进程局部缓存及临时目录；Node为22.16.0。隔离Web的两轮 `tsc --noEmit`、语言检查和Vite构建均exit0；均为7528键、零生产硬编码。初版构建65.232秒，修复版78.848秒，仅写各自隔离输出目录。准确Node／TypeScript／Vite argv见`candidate/web-checks.json`和`candidate/web-r2-checks.json`。

## 实际Windows GUI／HTTP／Worker

[安全回执](../../dist/quality/desktop-managed-runner-153/receipt.json)记录17项不同实际检查。环境是真实 Electron35.7.5、Windows DPAPI和软件离屏渲染；使用生产preload及目录／安装发现／本机管理三个窄IPC，renderer没有Node。生产目录签名及Go绑定实际执行，但系统目录对话框由fixture固定返回自有Git root，不把它称为系统选择器UI验收。

HMAC摘要发行、真实24小时Grant、固定control私有stdin、正式Go Worker、启动后新水位库存、一次暂停登记及一次独立启用均实际贯通。只有一个worker和一个原发行操作；全部五次注册传输保持原正文、凭据摘要与操作标识，全部五个正式Worker子进程均exit0／close。显式Stop通过EOF，明确重连沿原节点；整应用正常重开解密原journal且不自动启动；当前会话退出后原生重验使Worker停止。全程0任务／0运行／0模型调用，合成工具只响应版本／帮助／协议探测。

中文1366连接、英文390暂停及启用的三张实际图片已目视：准确节点、合成版本、建议名称和独立Enable入口可见。图片及摘要见[交付清单](../../dist/quality/desktop-managed-runner-153/delivery-manifest.json)，未观察到renderer异常。

## 真实失败与修复沿革

首次`first_real_connect`的15秒GUI等待实际失败：DPAPI、绑定、发行及原生`connected`已成立，两类节点读取200、真实库存fresh且available，但页面`timestamp()`仅接受整秒，拒绝服务端的`.895Z`／`.622Z`，未出现配置名称。原始失败checkpoint、诊断图及本机journal保留在私有fixture；没有重发节点掩盖故障。

随后严格支持UTC 0–9位小数，用Gregorian毫秒回校和BigInt纳秒校验`observed<=asOf<expires`、精确60000000000ns期限，剩余毫秒向下取整。两个新增回归先实际失败再通过；Web修复版沿同一worker、secret、操作和journal续验，最终17项通过。早期诊断还保留了在退出观察完成前读取关闭状态的失败断言；最终以实际close及五进程核对为准。

独立审查还修复了过期历史回执与当前启动资格混淆、Git前置错误分类、恢复权限只能准备原连接、真实失权返回映射、同worker停止重连回调去重等问题。旧main接线测试白名单未包含新增固定通道而失败，后只更新测试固定四通道和端口reservation替身；实际main接线1项通过，不改变生产源码，也不作为安装包验收。

## 交付与剩余边界

仅交付`dist/quality/desktop-managed-runner-153/`中的安全回执、来源元数据、清单和三张图片；不交付token、cookie、账户信息、数据库、请求正文、加密凭据journal或本机原日志。原始本地fixture保留供核对，不能通过清理日志解除未知操作。

真实供应方安装／登录／编码、OS目录对话框、强制崩溃恢复、macOS／Linux系统凭据存储、其他数据库、多主机、安装包、规模及生产仍未验收。整应用正常重开不代表强制崩溃恢复；软件离屏不代表正常桌面安装包全验。24小时Grant到期后的明确续权与原节点处置交互仍待交付，不自动发行新token。

Multica参考pin为`890628511e423864d21c694231277805ce9f113c`，工作树保留`4157f5b26e3acbbd53ae5abbb659c579dc7320eb`；五个关键源及LICENSE blob一致。新增复制产品源码0，沿用Suma已有成熟依赖。原始安全回执中的早期`T-LOCAL-RUNNER-153`标签保留为历史；本阶段入口统一使用T-MULTICA-DESKTOP-153，不改写原证据。
