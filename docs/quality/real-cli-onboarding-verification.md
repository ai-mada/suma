# 真实 Codex CLI 接入与探测诊断验证

T-REAL-CLI-ONBOARDING-226；速码 1.0.0；2026-11-03。负责人：主集成代理。基线 `f716279e627295fcf8d742f29bd71a21e74d7454`。本报告描述阶段交付，完整 31 项／P0–P7 及 58 limited、34 pending／92 能力矩阵继续。[产品](../product/real-cli-onboarding.md) · [架构](../integrations/real-cli-onboarding.md) · [English](../integrations/real-cli-onboarding.en.md) · [交互](../design/real-cli-onboarding.md) · [SOP](../operations/real-cli-onboarding-sop.md) · [机器摘要](real-cli-onboarding-summary.json)。

## 交付行为

原探测器统一使用可信私有配置、工作和临时目录，保留 helper 已准备的 Codex features／SDK／Cursor 配置；stdout 是版本事实来源，stderr 不再污染版本。原维护 owner、Journal、Windows Job 和不可变开始／停止回执继续负责进程生命周期。停止未知保留现场，禁止继续探测。

原安装观察增加可选非空的三字段诊断与 `scan_incomplete`，共享校验、Schema、普通／Pool HTTP、桌面投影和浏览器严格读取共同约束；不存在原始 stderr、程序路径、命令、环境值或自动修复指令。生产组件显示准确阶段和固定中英文原因，复用该工具／节点的原指南和明确重检。已安装的失败也计入“需处理”，安装数不变。

## 必要定向检查

18 项不同顶层用例全部通过：Go 8、Web 9、Desktop 1；其中新增 14、直接受影响原用例 4。Go 子用例不另算顶层。真实厂商接入实跑及代表界面组另计，不混入这 18 项。

| 检查 | 结果 |
| --- | --- |
| Go 私有 HOME、可信配置保真、stdout、停止未知保留与分类投影 | 4 个新顶层，通过 |
| 普通／Pool HTTP 严格字段、当前授权、持久化和原回执重放 | 2 个新顶层，通过 |
| 共享诊断合法组合与生成 Schema | 2 个新顶层，通过 |
| Web 严格读取、准确指南和本机／远程明确重检 | 3 个新顶层，通过 |
| 本机显示计数修正 | 1 个新顶层、同模块原 3 个顶层，通过 |
| 历史切换保留未知停止、新动作与原请求恢复分离 | 1 个新顶层、原恢复顶层 1 个，通过；原本阶段远程指南顶层补跑通过 |
| Desktop 严格诊断投影及 Cursor 当前源状态 | 1 个新顶层，通过 |
| 完整 Web 类型检查 | 初次及最后结构修复后均 0 诊断；仅因新结构和恢复分离补查，没有重复整个测试套件 |
| 原完整国际化检查 | 9971 对词条；占位、复数、标记及语言一致性通过；生产硬编码 0 |
| 同源合同生成 | 1014 模型 Schema、458 API 路径；新增 1 模型，新增路径／表／依赖 0 |

精确命令、stdout、stderr 和 JSON 事件保存在 `.cache/real-cli-onboarding-226/go/`、`checks/`。不重跑上一阶段未改变的全套 generation、授权、ACK 丢失恢复和独测验收用例。

## 已安装 Codex 的真实纵向验证

原 HTTP／SQLite／普通嵌入式维护 Worker 完成：准确安装扫描 → 显式两空字符串配置 → 不可变 generation → 原库存关联事务和 ACK → 原 HTTP 暂停档案。首次编译和实跑通过；随后补强跨平台未知停止错误传播，再编译并复核最终 Windows 源码。两次均通过，旧证据保留；以下数量为每次完整旅程，合计 8 个元数据进程。

- 原程序大小 368970544 字节，读取前后 SHA256 均为 `3097c909fb6d5cd63510d44aceb1d42df9f6e85722a3e447e477def1ecf60e16`，未复制程序。
- 实际版本 `codex-cli 0.166.0-alpha.5.1`；扫描和准备共 2 次 `--version`、2 次 `exec --help`，4 个原生元数据进程。
- 原 generation 1、prepared 提交 1、绑定库存提交 1、档案 1；档案 `paused`、`available=false`。启用、业务领取／运行、Prompt、登录命令和模型调用请求均为 0。
- 4 组原不可变回执均为 Windows Job、`parent_stopped=true`、`tree_stopped=true`、exit 0；开始／停止文件摘要匹配。未新增另一进程管理器，也未声称执行了独立 OS PID 快照。
- 测试宿主正常 exit 0；原回环监听端口关闭已复核。私有测试目录和持久证据保留，未清理用户账号数据。
- 嵌入签名仅读取证书主体等静态元数据，未验证在线证书链；没有网络抓包，不声称证明不透明厂商程序内部零联网。

最终原始回执：`.cache/real-cli-onboarding-226/actual/20261103T073127434021300Z/receipt.json`；首次回执仍在 `actual/20261103T071508881414200Z/receipt.json`。最终汇总为 `.cache/real-cli-onboarding-226/final-evidence.json`，首次汇总为 `verification-evidence.json`。最终 Go 编译输入 26 项全部与当前摘要相符；首次源清单保留为 `first-actual-compiled-sources.json`。上述 CLI 版本和帮助验证不能替代账号、模型、费用或代码执行可用性。

## 简洁界面验证

一次隔离构建、一次现有 Electron／Playwright 原生窗口；生产 `RemoteToolScanPanel`／`LocalToolsScanPanel`，合成公共 HTTP 与观察桥：

1. 中文 1366px：帮助合同失败展示准确原因，指南回调交给 CodeBuddy 与准确 Worker，不误用已选 Claude SDK；不堆叠重复准备按钮。
2. 英文 390px：明确点击重检只调用原本机 `force=true`，随后停止未知显示保留记录并禁用顶部和行级再次探测。

两组通过，3 张图由主代理查看；无横向溢出，操作按钮 44px。请求均为 GET，业务 POST／真实 DB／Worker／CLI／模型为 0；原生窗口正常 exit 0、forced=false、端口关闭。原图和输入摘要在 `.cache/real-cli-onboarding-226/web-actual/visual/20261103T071448727Z/`。

检查图片发现已安装失败的“需处理”计数为 0，随后修正纯显示模型并通过 1 新＋3 原用例。另发现远程历史选择可以忘记已观察的未知停止，已修正：当前 owner 内保留该事实，切换记录或分页遗漏不能解锁；只有准确原 source／claim／fence 的更高版本闭合及停树证明可解除。新检测、准备和库存采用被阻断，原 body／key 的回执恢复保持可用；迟到库存读取也不能越过保护变化。

只为该保护追加一次独立构建与原生界面组，通过中文宽屏历史 A→B、全局提示、新动作禁用和合成闭合后解锁，主代理查看 1 张补充图；不声称真实机器已恢复。该组也是 0 POST、正常 exit 0／无强停／端口关闭。最终 bundle 的 370 个输入摘要全部匹配当前源。总计 2 次构建、2 个原生会话、3 组／4 图；首次图保持计数和历史保护修正前原样，原 bundle 与当前源有准确 3 处差异，不冒称最终一致。补充证据在 `.cache/real-cli-onboarding-226/web-history-recovery/visual/20261103T073203646Z/`，完整差异在 `final-evidence.json`。

## 参考、资源与剩余验证

**Unix 已知接入限制（源级确认，未实跑）：**现有 `process_unix.Finish` 的进程组 `ESRCH` 不能证明 detached 后代停止。当前不能形成完整停树证明，因此同一可信目录的连续探测及正式扫描／准备接纳仍被阻断，私有目录保留。本阶段已补强原 Finish 接缝：包括无 collector 的独立探测，缺少完整树停止证明都明确返回 `stop_unverified`，不会捕获版本文本后仍对外宣称探测成功。Windows 原成功条件保持，定向断言覆盖。Unix 完整接入须以成熟 Linux／macOS containment 和目录核对机制另行闭合；未将组为空降格当作整树停止，未宣称跨平台等价。

重新读取 Multica 固定提交 `2115a513a375bfe2910b76725b617504faee4b25` 的 LICENSE 与实际 daemon 探测代码。许可证仍有 Part I 附加条件；本阶段未复制其产品代码或素材。安装发现和协议资格分离的做法供架构参考；新 ZCode 发现属于待实现范围，不视为速码已具备。

Claude SDK 0.3.314 的发布元数据不能证明缺失的真实 native 已安装；官方 CLI 安全选项的版本要求保持，托管 hooks、文件工具限制、OS 隔离和 observe-only 预算分别表达。当前阶段不以自有程序替代 Claude 厂商验收。

构建空间不足通过可核验的任务缓存迁移与重复备份去重恢复，源文件、用户文件和唯一证据未删；过程记录在本阶段忽略目录中。所有新临时、构建、DB、图像和供应商程序均不提交 Git。

继续待验证：真实模型及预算执行、代码候选／同提交独测／人工验收、其他 CLI／SDK／AppServer 实接、完整桌面 IPC 与应用导航、物理多机、其他数据库与操作系统、安装升级、规模和生产。原 31 项／P0–P7 全平台未完成。
