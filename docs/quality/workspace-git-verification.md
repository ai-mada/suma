# 实时Git工作区与原生执行限额验证

状态：T-P4-035／1.0.0，限定本地验证。负责人：平台集成。隔离真实Git／Monaco及执行限额定向检查通过；最终前端配置提示、隔离正常闭合及主预览保持均已验证。完整P0–P7及需求1–22继续实施。

关联：[PRD](../product/workspace-git.md)、[架构](../architecture/workspace-git.md)、[交互](../design/workspace-git.md)、[AI SOP](../operations/workspace-git.md)、[合同](../../contracts/workspace-git.md)、[模型绑定设计](../architecture/035-model-selection-design.md)、[English](../en/workspace-git.md)。

## 源码与必要检查

- 当前项目HTTP／CLI只读Git状态和工作树／暂存区准确文本，严格参数与当前身份；前后权限、目录及Git元数据复核。
- 复用Git CLI、Go目录约束与受限执行模式，禁外部过滤器／钩子／网络及越界元数据；真实新增、删除、重命名、未跟踪、未提交／分离HEAD、恶意配置与敏感路径均有定向检查。
- 真实复现`D  main.go\0?? main.go\0`，修正同路径暂存删除后重建的重复记录，保持两种来源事实。另补并发文件替换／消失核对，未重跑无关旧分组。
- HTTP与CLI先复现404／未知命令，再通过准确项目、参数、无效输入不请求后端等边界检查；同源OpenAPI／TypeScript生成及校验通过。
- UI6项受影响检查、Monaco动作2项检查、TypeScript及国际化检查通过；修正后4419对语言键，覆盖模块无未处理硬编码。实际发现的未启用配置错误只新增1项呈现检查，没有重复整个Git或保存测试集。
- 原生限额定向验证包括普通Execute实际子进程stdin、修复生产payload builder及实际子进程stdin、非法参数的执行前失败，以及修复128步／每次1024额度与普通16步的流边界。原始限额丢失被准确修正；修复payload检查不冒充完整厂商修复任务。

环境为Windows、缓存Go1.26.10、已安装Git／Monaco0.52.2／Vitest3.0.5／Vite／TypeScript，无新增依赖。只检查受影响路径，没有完整测试套件、SDK全历史用例、历史来源开发或正式候选测试重放。

详细命令与源码摘要：`.cache/e2e/workspace-git-stage-035/domain/verification.local.json`、`ui/verification-final.local.json`、`ui/workspace-disabled-ux-fix.local.json`；原生限额记录在`.cache/e2e/native-execution-limits-stage-035/verification.local.json`。Go四程序一次构建；原生SDK11项直接输入与161项实际依赖输入逐字节不变，复用原产物。

## 真实界面与持久记录

在已闭合034数据的独立副本中使用原测试会话，仅启动Control59210，无Runner／模型／新登录／凭证／任务／计划／正式测试。新建的专属普通Git夹具先固定HEAD counter1、index counter2、磁盘counter3（CRLF／中文／🧪），另有暂存重命名、磁盘删除、未跟踪及一个dummy受限文件。

1. 原界面绑定该专属目录。真实Monaco输入counter9但不保存；切换Git仍显示磁盘2→3，暂存区准确显示1→2。
2. 返回编辑器，counter9草稿仍在；原生Ctrl+S一次成功保存。再次进入Git，工作树准确显示2→9。
3. 实际显示四项安全变更，受限文件名称未暴露；已删除文件编辑入口禁用。中英文、方向键标签导航、刷新及实际磁盘保持通过。
4. Control203008正常退出0，holder退出0，闭合校验通过。所有186表schema保持；177张非受控／非锁表完整旧行不变，原工作区操作等受控表旧行保持。仅目标绑定及1项completed文件操作／1项accepted操作新增；activity／general operations新增均为0。四张内部锁表99行身份保持，sequence各增加49。
5. 源034所有数据文件及辅助文件状态保持；035闭合数据库为`bdd0943f4192437cbd5ea61062f6ed1f957adebecf46fb9a108b07ee64bd97de`。夹具HEAD`24bd1b5e89bf3ccae3450c186ef8e5e7e7ce1237`、index摘要`48540592beb4546e4fdabfcdf76f703d9a74bd6d13b52a5c3d2db335aaa7a6e4`及除counter文件外的全部仓库文件／元数据保持。准确counter9摘要为`f0fbcbf6cd7987649b19c91765a9d0e08af6e7513477d80056fa092b1ed17683`。

最终闭合`.cache/e2e/workspace-git-stage-035/actual/closed.local.json` SHA`9f8573174a084c2e897cf97b048571be2157cd515017207aaa07c38e521f8962`；界面`browser/verification.local.json`记录14项通过。截图保留在同一忽略目录，不进入Git。

## 实际发现与必要交付修正

首次主预览检查发现：服务未设置本地工作区时，后端已有`binding_available=false`，界面却显示“当前身份无权限”并仍提供绑定表单。仅依据已有准确能力状态修正四个前端文件，显示清晰部署管理员说明并隐藏无效表单；已配置的Git／编辑路径未改。新增单一检查先失败再通过，最终TypeScript及4419对语言检查通过。

保留已验证首次产物；最终前端在独立`web-delivery`构建，构建前后源码准确保持，原活动`web-release`135项资产保持。只追加未启用状态的只读界面验证；Go、SDK、Git正向和快捷键证据复用，不重放模型或正式任务。两个前端构建分别对应首次功能与实际发现的状态修正，不将旧产物冒称最终产物。

## 最终交付与主预览保持

最终前端为 `web-delivery`：361项输入摘要 `9570e02eec74e1f00dd396ad3ae414877797bd09c0240763fa3a6f7d886653e1`，135项资产，index摘要 `7e3b69324a6be69b07a7cdaa4450d3cf3ca6dc595a51b69f2a2210b2b188bd25`。Go444项输入摘要 `7bd50d92a4af3982c3a4cc9b2ce51a23c862724468e651d68754b82acc3d213d`；Control摘要 `0bc5111228b71fa6a504ee418543f98c6a2f1598d7099723579a3df7c6917295`。最终提示只改变未启用工作区的分支，未重复已验证Git正向旅程。

59211隔离副本使用最终前端，实际确认中英文禁用原因、无路径输入／绑定按钮、任务导航与刷新；无Git命令、保存、模型或业务写入。Control263140及holder268648正常退出0，186表schema和182张非锁表完整行保持，所有源文件不变；4张锁表99行仅sequence各增加22。界面8项检查、闭合与源数据核对均通过。

主预览先前034与035首轮服务均由各自原holder正常关闭。最终Control271036／holder264708，仍在 `http://pool.localhost:4324/` 使用原数据和会话；只更新最终前端，不重建Go、不回退数据库。前向切换前后全部186表完整schema及行相同；真实中英、原任务v3已验收详情、刷新及键盘关闭通过。界面读取后182张非锁表完整行不变，四张锁表34行仅sequence各增加23；18authority／0Grant／18session与原1failed／2succeeded保持。135本地资产核对摘要，HTTP只取health及实际入口两资产，共3次请求。

主预览未配置允许的本地目录，准确显示未启用说明且无绑定表单；不借主预览声称正向Git已验收，后者仅对应59210专属夹具。原始发现证据、首轮构建与最终修正分别保留。最终warm记录 SHA `250edd5d61f0813e0d5d5344238d207c8eb632285a07d09a27504ef7815b10f2`；[机器摘要](workspace-git-summary.json)固定各项准确证据引用。

## 仍需完成

链接工作树的项目仓库授权、受管Git写命令、远端节点工作区、完整PTY／LSP／DAP、跨关闭草稿、内置编辑器菜单中文化、独立可写模型／参数／费率绑定及真实厂商／规模验收继续实施。SDK当前保守累计每次请求的输出许可；后续在完整可验证用量下回收未使用许可，改善多步执行效率，不能把当前许可当成实际消耗或厂商账单。

本报告只证明所列Windows／SQLite／普通本机Git与真实Monaco行为；没有厂商调用、分布式数据库、物理多机或生产验收声明。全部临时数据、日志、凭证、截图和二进制留在忽略目录，仅必要源码／设计／精简摘要提交。
