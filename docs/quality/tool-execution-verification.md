# T-P4-004 工具派发、执行事实与质量闭环

日期：2026-10-24。状态：本机纵向阶段验收通过；完整平台继续实施。负责人：主集成与三个并行代理。需求：P2/P4 外部智能体和开发工具，P3固定需求任务，P5独立测试、验收与问题回流。关联：[架构](../architecture/tool-execution.md)、[交互](../design/tool-workspace.md)、[执行合同](../../contracts/tool-execution.md)、[完整台账](../delivery/implementation-plan.md)。

本阶段交付固定工具档案、真实现场资格、任务执行计划、结构化进展与严格结果合同。任务负责人、Runner身份、工具配置、工具自报、实际执行、独立测试和人工验收分别记录。本报告限定于本机SQLite与Windows实际进程；不扩大到厂商模型质量、生产、分布式容量或整个P0–P7验收。

## 实现与自动验证

| 范围 | 结果与证据 |
|---|---|
| Go最终全套 | `go test -p 1 -json ./... -count=1`与`go vet ./...`均退出0；512个测试通过事件、0失败、0测试跳过，10个受测包及4个命令编译包。含父／子测试和进程helper，不是512个独立用户场景。[最终原始记录](evidence/tool-go-post-review-tests.jsonl)、[最终源码与环境摘要](evidence/tool-go-post-review-verification.json)。初轮479原始切面保留，不能代替停止和Codex协议修订后的512 |
| 工具与运行时 | Runner域内111个通过事件；原生Agent最终8项，严格协议、指纹漂移、库存暂断、取消回执、错误凭证和跨项目边界分别核对。[运行时专项](tool-runtime-verification.md) |
| 界面与类型 | 最终工作区212项（web198、agent8、desktop3、knowledge3）、类型及生产构建均通过，输入前后不变。包含3项实际React DOM点击/路由/返回回归。初轮203及响应式208、各UI切面保留原始字节。[最终工作区](evidence/tool-workspace-final.json)、[测试原文](evidence/tool-workspace-final-tests.txt)、[类型](evidence/tool-workspace-final-types.txt)、[构建](evidence/tool-web-final-build.txt)、[界面专项](tool-ui-verification.md) |
| API合同 | 147个模型、117条API路径及344个Schema引用；工具事件与DAG事件类型独立，7条新增工具／任务计划路径。普通生成程序曾暴露测试辅助函数误用，修复后实际构建、生成及非写入漂移检查通过；原失败保留 |
| 交付 | 六目标×控制面、Runner、CLI、Deployer，共24个程序编译、独立模块和格式、字节核验通过。[最终修订运行包](../deployment/tool-stop-runtime-bundles.md)。初次[045219运行包](../deployment/tool-runtime-bundles.md)及177份归档保持原字节；最终包另有179份归档。交叉编译不代替目标操作系统运行、安装和签名 |

Go验证在Windows宿主、真实SQLite、GOMAXPROCS=2、串行包测试、仓库内临时目录下完成。最终189份Go测试及运行输入前后不变；116份运行输入与最终包源摘要`3cdf6ee27eee0ffc6ec731a62c435c474708eeae8db0b48fbb6023bb28fd8b76`匹配。Go原始日志逐字节归档；汇总只更新归档文件名及归档说明，[512原汇总](evidence/tool-core/go-verification-post-review-original.json)和[479原汇总](evidence/tool-core/go-verification-original.json)分别保留。响应式及导航补充仅修改前端，不改变最终Go切面。

[最终字节核验](evidence/tool-final-validation.json)再次核对189份Go、105份工作区输入、两批运行包档案和6份原图摘要；最终浏览器确实加载`index-M_630Zj3`，四宽度没有页面横向溢出。接口为147个导出模型及统一`Error`，实际148个Schema组件、117路径；原生成日志的147是模型数，不含统一错误组件。

## 实际界面旅程

本机独立数据位于忽略的`.cache/e2e/tool-stage`，控制面只监听loopback 4322，浏览器使用localhost避免与旧阶段127.0.0.1会话互相覆盖。初始化账号、项目、纯执行节点身份及固定工作流通过本地API准备；以下需求、拆解、工具档案、启用、执行计划、启动、验收、回流及暂停均通过实际产品界面完成。未复制用户生产数据。

执行器明确标注为确定性Suma JSONL协议夹具，读取本次固定任务／需求JSON后选择真实代码变更、失败或等待分支。没有调用厂商模型、使用模型密钥或产生模型账单；真实过程包括Node/Python进程、Git工作树与提交、独立unittest、HTTP回写和正式SQL状态，不以模拟结果代替这些事实。

1. 创建“订单访问权限与固定任务执行”需求，填写验收条目，编辑拆解为成功、失败回流、停止核对三项任务。档案由受限`--describe-tools`清单建立，初始暂停；Runner真实上报匹配配置及程序摘要后，在界面明确启用v2。
2. 成功任务创建固定预览，审阅任务v1、需求v1、流程v1、智能体节点、工具v2和独立测试。刷新同一计划后，预留编号仍显示“尚未创建正式运行”；显式启动只创建原计划的运行`10158abedf0d680f439ca0f2fac2d434`。
3. Agent在独立工作树提交候选`c31c01956f3098f3b5c07b2c94ceaaac3b388dfe`，仅变更`app.py`。独立`quality`测试节点固定同一来源attempt/fence/提交和证据，另建工作树，实际运行2个unittest通过。人员在界面记录明确评审后，任务才由进行中v2转为已验收v3；记录明确仅验收本机夹具范围，未合并或生产发布。
4. 失败任务运行`68c1e6179992238819c5bf4aaef19183`实际退出7，开发节点失败、依赖测试跳过；正式投影只有一个问题`76e136e48e633dbf4d8ad2581bba7b9b`。界面回流后刷新，再次回流仍只有一个关联需求`f5fefa67395e77f71d6f1941795e76d2`；需求可下钻来源问题及原运行。
5. 停止任务运行`230cfddc70b539f9b5b212ad675b9d05`进入实际等待进程。界面取消先形成`stop_requested`与取消中，Runner停止进程树并写回`stopped=true / outcome_unknown=false`后才进入cancelled。工具协议终态仍为unknown，未冒充成功；测试attempt=0，工作树无变化，任务没有人工验收。
6. 从停止节点下钻工具配置，明确暂停v2→v3，再返回原运行和原选中节点。新派发资格显示暂停，历史运行、固定v2依据和实际结果不被新状态改写。

正式数据只读事务证明3个计划、3次运行、6节点、4个attempt、35事件（10条typed tool事件）、3任务、2需求、1问题、1评审。原检出仍为基准`58a26b8bb6d71eda9346b2b846355c242f72d446`且干净。见[正式关联取证](evidence/tool-browser-execution.json)、[只读采集器](evidence/tool-core/collect-final-tool-evidence.py)、[实际程序与源码摘要](evidence/live-runtime-final.json)。数据库快照证明持久事实；重复点击和浏览器加载资产另由实际界面操作记录，不从数据库推断。

上述首组三条旅程使用初次045219 Windows交付控制面SHA `c06d3553c46278704dd976f5ab1dcef52d3b25a96f48d76e3bed4d08092b4eb8`及Runner SHA `12c8681ec9d7e1ba325252e264e5d7aaa7d73d9c053cc3d6c3466292b44f56cb`。原生CLI三授权用例在更早候选控制面1e上完成，首次包源输入一致但不是同一二进制；其专项报告明确区分。

独立复核再发现零退出停止边界及官方不同Codex版本用量形状后，重新执行完整512、构建并独立复核051829批次24包。最终Windows控制面SHA `d357669448f9ccee63752b7177dcea18b56eeb9efaff01052332da43439a4c0f`、Runner SHA `644adbf34d68fce6e65779819118751a64c2eb2872110915d0acdd6e4399d8ab` 已实际激活同一本机数据。界面再次启用工具v4，为停止任务v2创建新计划`10e881907be8782183ac296ddaa01cdf`和第4次运行`a64dfd23ac07b393626d7f397f12d06c`，实际进程运行、取消中、停止回执和cancelled完成；协议仍unknown、无代码变化、未执行依赖测试、未人工验收。之后工具暂停v5。原首组运行、问题和验收保留，不将首次旅程改称在最终字节上执行。

## 发现和修复保留

- 真实浏览器发现ready计划具有预留run_id，旧界面误当已启动而隐藏确认。只读证据证明当时0正式运行；修复为按`state=started`显示真实运行入口，ready继续审阅启动。[原事实](evidence/ui-reserved-run-readonly.json)、[界面回归](tool-ui-verification.md)。
- 实际控制面停止测试发现严格工具终态校验误拒绝“已确认停止／协议未知”；修复后保持unknown、接受实际停止，不放宽成功。原RED和GREEN在[核心档案](evidence/tool-root-archive.json)。
- 最后独立审核发现`Stopped=true`、零退出、无/failed/completed工具证据时，活动运行仍可能落入成功。统一所有generic及绑定工具的确认停止为失败，取消中的运行为取消，未知效果继续核对；没有降低证据约束。[最终RED](evidence/tool-core/stopped-zero-exit-red-attempt3.jsonl)、[GREEN38个通过事件](evidence/tool-core/stopped-zero-exit-green.jsonl)、[历次说明](evidence/tool-core/stopped-zero-exit-verification.json)保留，前两次测试查询夹具问题与真实产品失败分开。5处正常完成的测试夹具移除误用Stopped，不改真实停止用例或增加生产兼容绕过。
- 旧测试依赖未登记worker及隐含任意能力，改为真实项目绑定身份和精确command能力；生产没有增加兼容绕过。全套最终479通过。
- 原生模型工具名含点，不符合已核对的官方名称合同；增加确定性安全别名，平台调用和事件仍使用原规范名，未知别名拒绝。模型端是HTTP fixture，未据此声称真实厂商调用成功。
- 官方Codex旧版Serialize真实输出3项基础计数，新版可包含推理/缓存写入拆分。适配器归一化为3项必需及严格可选拆分，空对象、缺基础字段、错型、负数、非整数或溢出仍拒绝。真实解析及实际子进程RED→GREEN16个通过事件；这不证明本机0.164只发送3项，也不声称真实模型执行。[官方源码与独立补充](evidence/tool-codex-usage/README.md)。
- 实际390宽度原执行页仍以大画布占据首屏，缺少完整可操作节点列表；补充≤850的完整节点列表、状态与attempt、键盘直达详情和返回焦点，桌面继续图视图。宿主截图的1.5倍backing比例曾造成截幅，不能仅凭早期截幅图断言控件真实溢出；最终DOM几何和完整原图分开记录。[组件补充](tool-ui-responsive-supplement.md)。

## 最终交互与重启证据

最终第4次停止运行及原三条旅程不变的独立核对见[只读补充](evidence/tool-post-review-database.json)、[最终程序身份](evidence/tool-post-review-runtime.json)。采集器首轮嵌套字段投影错误保留在[原失败](evidence/tool-core/post-review-collector-failure.json)，修复采集器后只读复核通过；没有为采集错误重跑任务或修改产品记录。

实际浏览器验证390、768、1366、1920 CSS宽度，深浅主题、键盘进入节点、返回列表、启用审阅Escape回到原按钮、精确运行/节点返回和页面刷新。[实际观察与加载资产](evidence/tool-browser-visual.json)、[原始截图清单](evidence/tool-visual-files.json)分别记录CSS尺寸和原生截图像素；宿主backing比例为1.5，截图没有编辑。手机默认完整节点列表、桌面恢复依赖图；当前暂停资格不改写已发生的工具或运行事实。

最后实际点击发现任务关联需求并未完成导航；增加与App一致的内部导航和离页保护，保留修饰键原生行为。键盘通过后进一步核对鼠标命中：旧复合行内链接分成两行，中心落在DD空白；改为完整可点击盒后，实际中心命中A，鼠标点击、Enter和浏览器返回均在最终`index-M_630Zj3`上通过。新增jsdom开发测试环境，实际挂载App点击精确未加载需求、浏览器返回原任务及拒绝离页，另有命中盒结构回归；JSDOM不证明实际布局。见[导航专项](tool-ui-navigation-verification.md)、[点击区域专项](tool-ui-hitbox-verification.md)。开发依赖和锁文件已记录在[第三方清单](../delivery/third-party.md)。

上述为本阶段部分页面走查和少量正式节点，不是完整四宽度五旅程、全浏览器、WCAG或千节点性能验收。初轮与修订切面分别保留，不用新的通过结果改写旧截图和测试原件。

## 未完成范围与后续

Codex与Suma结构化合同已经实现和本地协议验证；已安装CLI版本/help探测不能证明账号授权、实际模型质量、成本或远端会话恢复。OpenCode目前没有核实完整运行终态，明确不可派发；Hermes、WorkBuddy、Zed、harness/dsh等不登记为已完成执行集成。未报告的usage保持未知，不把缺失用量视为零账单；零值缺省序列化的精细计量需后续成本合同完善。

OceanBase CE4.4.2 LTS BP3、可选PostgreSQL真实集群，S3、跨节点Git对象传送、完整公平调度与模型预算结算、真正合并、终端/LSP/DAP、智能修复熔断、离线协作、办公媒体、计费支付、跨平台实机安装和规模目标继续按完整台账实施。旧桌面安装包保持历史切面，本阶段未重新打包或执行安装升级。响应式和键盘验证仅覆盖本阶段部分页面，不代表完整WCAG、全部旅程或1000节点性能达标；Monaco/ELK大分块警告继续披露。
