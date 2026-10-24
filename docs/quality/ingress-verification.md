# 签名事件入口与实际执行验收

状态：T-P4-003 本机纵向验收通过，完整产品仍在实施。日期：2026-10-24。负责人：主集成、控制面、工作台、部署与运行包代理；独立代理只读复核。关联：[实施台账](../delivery/implementation-plan.md)、[合同](../../contracts/automation-ingress.md)、[接入说明](../integrations/automation-ingress.md)。

## 本阶段交付

规则可附加 GitHub 或 Suma 签名来源，保持原人工／cron 策略、固定流程及规则修订。创建入口只保存暂停元数据；设置密钥、核对启用、暂停、修改、撤销分别操作。交付回执与触发排队同事务写入，再由平台调度器复核当前授权并创建运行。外部事件中的分支、SHA 和成功结论只是来源证据，不会被注入执行命令，不代表平台测试或人工验收已通过。

GitHub 支持受限 push（分支和标签）、workflow_run.completed 和不触发运行的 ping。Suma v1 支持 git.push、ci.completed、monitor.firing、monitor.resolved，签名绑定入口、时间、交付标识与原始正文。CLI 从进程环境读取秘密，拒绝非 loopback 明文目标及重定向，不附带平台会话凭据。固定正文及身份去重、暂停和修订保留旧回执，密钥加密保存并防止历史重用。协议边界见合同，不能将其描述成任意 CI 或监控厂商的原生适配器。

## 自动验证

| 检查 | 实际结果与证据 |
|---|---|
| Go 全套 | `go test -p 1 -json ./... -count=1`：385 个通过测试事件、0 失败、0 测试跳过；含父／子测试和 helper，9 个受测包，4 个命令包仅编译。`go vet ./...` 退出 0。[原始记录](evidence/ingress-go-tests.jsonl)、[命令、时间和源码清单](evidence/ingress-go-verification.json) |
| JS／TS 工作区 | 173 项：web164、desktop3、agent3、knowledge3；[原始记录](evidence/ingress-js-tests.txt)。最后仅修改暂停提示文案，再执行 [12 项消费者检查](evidence/ingress-ui-final-consumers.txt)及生产构建；没有把该文案变动后的结果冒充另一轮全套测试 |
| 类型与生产网页 | [类型检查](evidence/ingress-typecheck.txt)、[最终构建](evidence/ingress-web-build.txt)退出 0；实际浏览器加载 `index-DqUuHkug.js`。Monaco／ELK 大分块提示仍存在，未验收低配千节点图性能 |
| 合同 | 131 模型、110 API 路径、315 Schema 引用；新增 7 个入口路径。公开 hook 使用签名认证而不是管理会话。生成器确定性、类型／接口漂移及文档链接单独检查 |
| 部署配置 | 8 项独立部署测试，0 失败、0 跳过；6 种 Compose 合并配置、3 种数据库 values 及 4 种非法配置、入口脚本语法检查。[归档映射](evidence/ingress-deployment/archive-map.json)。未安装 Helm，未执行 Helm render／lint 或容器／集群运行 |

最终 Go 记录对应 03:24:23Z 至 03:26:45Z 的宿主串行执行，受测源码前后摘要不变。早先一轮因 `store.test.exe: Access is denied` 未完成，且随后增加标签处理，不能作为最终验收；[原失败](evidence/ingress-go-pretag-access-denied.jsonl)和[诊断](evidence/ingress-go-pretag-access-denied-summary.json)保留。该环境执行限制经宿主运行解决，没有放宽产品权限或测试断言。本轮最终 0 测试跳过与上一阶段目录链接跳过分别记录。

针对入口的 SQLite 验证覆盖双连接并发、签名错误、旧回执、正文／身份冲突、当前角色撤权、启用者失权、轮换／修改／暂停、持久防重用、别名限额与头部投毒、数据库时间窗和未配置版本。CLI 与生成器从真实失败到通过的原始记录保留在[测试档案](evidence/ingress-test-archive.json)。测试通过事件不等于同数量独立产品场景。

## 实际本机旅程

使用独立 `.cache/e2e` SQLite、loopback HTTP、浏览器界面配置、最终交付 CLI 和真实 Runner；未读写用户正式 `.suma` 数据。GitHub、CI 和监控正文为明确构造的夹具元数据，下游实际执行隔离 Git 开发、固定候选构建、真实包字节上传及下载后的 HTTP 测试。

1. 在界面创建“Git 与 CI 签名来源验证”，固定流程 v1，显式启用；创建 GitHub、Suma 两个暂停来源，分别配置后核对启用。第三个来源仅验证首次配置指引，始终没有密钥、交付或运行。
2. 实际进行 18 次本地协议／CLI 请求。12 项协议检查覆盖标签、重复、无签名头的 delivery 别名、同 ID 不同正文冲突、分支过滤、ping、事件头投毒、监控订阅、过期时间及缺失签名；另 6 次 CLI 请求覆盖首次、重复、暂停、新修订历史和控制面重启恢复。[协议记录](evidence/ingress-protocol-execution.json)。
3. 持久数据共 7 条交付：3 条 accepted、3 条 ignored、1 条 blocked；accepted 只表示已创建触发。3 次实际运行均成功，共 9 个节点；6 份构建／包测试证明及 3 份实际原件重新计算摘要，均匹配封存清单。
4. 暂停来源、修改 Suma 来源到 r2，保留 r1 交付；r2 暂停时新交付受阻，旧 r1 请求仍恢复原回执。随后暂停父规则。全部来源现在暂停，已开始运行的事实不被撤销或重画。
5. 以当前运行包中的 Windows 控制服务重启，最终 CLI 再投递原 CI 与 GitHub 标签，均返回原身份和原触发，不增加运行。GitHub 和 CI 的持久重复计数均为 3。读取实际执行仓库前先验证其 Git 顶层，原检出 `285e78f217245baea199b448d83a9035de15ced1` 保持干净。

上述运行分别是 CI `242b298381a8d2bc65237c52e443a644`、监控 `7bd88c964771b8a24f69c3091cfe5054`、Git 标签 `8beee23f3d0cd305eb8e8f6884c6c17f`。正式快照、封存摘要、运行节点、当前实际进程与 CLI、来源历史及重启回执见[机器可读旅程](evidence/ingress-browser-execution.json)。不据此宣称对应业务任务已人工验收或版本已部署。

## 界面与视觉

实际普通 Chrome 局部检查 390×844、768×1024、1366×900、1920×1080。读取真实 DOM viewport、clientWidth 和 scrollWidth，所检页面无横向溢出；不能只根据请求的尺寸给截图命名。已改进桌面规则列表粘滞定位、受阻原因中文说明、首次设置与已有密钥轮换文案。交付受限上下文在宽屏 Escape 后返回原按钮；手机首次配置只查看提示，不生成／提交秘密，Escape 后回到“设置签名密钥”，保留规则和入口 URL。

深浅主题、关联运行下钻、固定候选节点与真实输出已检查。检查期间最终 Chrome error 日志为空；这不是全站无错误或完整 WCAG 结论。尺寸覆盖及主题已恢复，临时 Chrome 页关闭，IAB 保留结果页。[视觉记录](evidence/ingress-browser-visual.json)记录过渡帧和两次截图超时；原图保留，稳定后重新捕获，没有改图。

- [390 首次配置](evidence/ingress-initial-key-390.jpg)、[390 入口](evidence/ingress-entry-390.jpg)、[768 交付](evidence/ingress-tablet-768.jpg)。
- [1366 最终规则定位](evidence/ingress-desktop-final-1366.jpg)、[1366 深色](evidence/ingress-dark-1366.jpg)、[1920 交付](evidence/ingress-deliveries-1920.jpg)、[1920 上下文](evidence/ingress-context-1920.jpg)。
- [真实运行与固定构建](evidence/ingress-run-1366.jpg)。[原始图片清单](evidence/ingress-visual-files.json)保存真实格式、尺寸与摘要；`ingress-desktop-1366.jpg` 是粘滞定位完善前的原图。

## 运行包、数据保护与边界

六目标×四程序共 24 份新运行包切面为 `20261024T033010763Z-94b39f3b`，101 份运行源码的摘要为 `3c971b45a835621efb1cc8e9f7d16c638df02e6d6cd3d67b2d81bf6198178727`。实际 Windows 控制服务 SHA-256 为 `bfdf0eacb72baf1d7b4c0633ac6e7f066b222f8f7ad5e171291c95c4a70f95c6`。跨编译不代表另外五个平台实跑或安装；历史 Electron 安装包未重建，不能称已包含本轮功能。详见[当前运行包](../deployment/ingress-runtime-bundles.md)。

SQLite 自动生成的 `automation-ingress.key` 是 32 个原始字节，必须与数据库一起备份；环境配置使用 canonical base64 的 32 字节主密钥，多副本必须共享同一个密钥。0600 文件模式不等于 Windows ACL 已单独验收。禁止将主密钥、签名原文、会话、运行数据库或缓存提交。检索及详情只返回受限事件元数据，不保存原始正文。

GitHub native 签名没有受签时间，未见过的旧有效正文无法证明新鲜性；持久正文与交付身份去重不能替代该协议缺失。真实公网 GitHub、厂商 CI／监控账户、完整设备治理、OceanBase／PostgreSQL 集群、密钥恢复演练、容器与 Helm、SaaS 目标负载、所有五旅程和完整跨平台视觉／安装矩阵仍待验收。历史 855 文件复核中两份当前部署文档有预期更新，其余历史文件保持原字节；[后续集成复核](evidence/ingress-historical-integration-check.json)与不可变原收据分别保留。
