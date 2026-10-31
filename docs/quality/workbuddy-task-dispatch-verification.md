# 任务直接派发到 WorkBuddy：限定验证

阶段：T-WORKBUDDY-TASK-DISPATCH-168；suma 1.0.0；2026-10-31；负责人：主集成／后端／工作台／实际验证代理。状态：任务到原云Runner的隔离联调限定通过，末次时间展示小修单列。依据：[合同](../../contracts/workbuddy-task-dispatch.md)、[架构与AI SOP](../architecture/workbuddy-task-dispatch.md)、[交互](../design/workbuddy-task-dispatch.md)、[机器摘要](workbuddy-task-dispatch-summary.json)。完整31项需求、P0–P7、58限定／34待交付／92总账继续有效。

## 实现范围

任务详情新增独立WorkBuddy入口，本人单个授权账号默认选择，可明确选择已发布知识。先保存固定任务、需求、设计、已发布测试计划及知识的预览，再明确派发；复用原Go DAG、私有／Pool领取、中央一次创建、Worker持久回写和原云节点工作区。无手填JSON、摘要或本机工具Profile；未确认启动沿原body／key核对原Run。

计划预览无供应方调用、解密、运行或委派。正式启动把当前授权／路由复核、原Run、唯一委派、任务来源及版本、设计／测试依据、CAS和回执放入同一事务。供应方completed保留waiting／cloud.delivery_required，不能当作代码候选、测试通过或人工验收。当前代码基线尚无可信云传递，明确拒绝此路径。

## 定向检查

- 后端12项新顶层／15子例、3项原受影响顶层／6子例分别最终通过；15顶层去重，21子例单列。包含私有与Pool正式HTTP领取、受控一次创建及completed不绿、输入变化、当前owner和凭据边界、并发与回滚、初始schema前置拒绝、正式来源及设计／测试范围快照。
- 公共同源合同1项、错误双语1项通过；合同约束上界变化后仅复验同一合同用例，不重复计数。独立增量为3DTO／3路径；802声明模型、803schemas、380路径，原377路径／800schemas无修改。
- Web的模型7项和组件11项最终去重为18项，原失败与受影响复验分别留证；没有再跑一批全18。8个最终文件全LF，Root逐文件核对摘要。一次完整TSC退出0、7934对中英文词条／占位／复数／标记检查通过，生产硬编码0。全部自动化顶层合计35，子例不重复计数。

后端源码14文件摘要`2d83e85fa6f7bcbb4c318ffc2b848de7f2788a955d875335b957845a17360a03`；私有receipt摘要`6f8f97d883eda54ab0c65ffff9d3b1621e50c92d072f2a313cbaf5ba702333e6`。原始命令、stdout／stderr和全部失败保留在忽略目录，不进入Git或安全导出。

Web最终8文件与分批日志receipt摘要`db9921e013cdab5a306a438ddf2e1d1675231565ab0f29311201172846139ae2`，Root独立核对来源和唯一阶段所有权。App仅一个lazy import与任务详情的独立入口接线，保留原本地开发路径。后端和Web独立审查的受影响权限／恢复块最终无其他已知P1/P2；代码审查不代替实际验证。

首次领域检查暴露typed绑定持久化为map后的JSON键序摘要漂移，已在构造时统一表示；精确回归后保持严格摘要校验。另补当前Pool grant资格，撤权／版本变化不能靠旧路由通过。显示工作流使用空数组，node摘要按Core重建依赖后的真实规范形状计算；未改既有运行身份。

初次Root合同编译遇并行落盘中未使用import，退出1；该批双语用例独立退出0，随后只补合同。Web新表单空值保护、知识采用附件深层形状及冲突核对失败保留原状态均按受影响路径修复与复验。没有重跑无关全套。

## 实际联调与视觉

实际验证使用独立假账号／连接／任务、正常HTTP／SQLite、正式私有Worker.Run及受控本机供应方；使用生产派发组件和原云工作区，不mock fetch、不修改活动预览或用户数据库。一次Go宿主编译、一次SDK组件构建、一次正式运行均退出0，8项业务检查通过，无页面／控制台错误或非本机外发。正式门禁84项由Root逐项核对，摘要`165474b480fc6f9b0c13f11da8fa3d58ac8ae9420acb8d0a95423d3a361729c3`。

浏览器只有3次POST：预览1、启动2。第一次真实Go启动201且原事务已提交后，测试代理仅把浏览器响应截断为不完整JSON；界面明确待核对，再沿相同path／body／key取得原Run的201。两次正文SHA相同，唯一plan／run／delegation／submission／供应方Create各1；供应方准确GET1返回completed。原Worker真实走inventory、claim、heartbeat、lookup、create、refresh、progress、results，最终journal待确认inventory／progress／result全部清空。

最终Run为running／v3；原node为waiting／cloud.delivery_required、attempt1／fence1；task为in_progress／v2。候选、test_reports、review、artifact、release_candidate均0；test_plan_runs=1是任务测试范围的依据绑定，不是执行或通过记录。GUI种子只有任务和需求文本，没有发布设计／测试计划或选择知识；这些上下文的领域回归不能误写成本旅程的实际验收。

中文1366与英文390两张原图均由Root独立查看。三份正式全局CSS按main顺序加载，计算令牌为16／24／18px，两个视口均无横向溢出；中文图为滚动后的预览内容／明确派发区，英文图为恢复计划后的footer及原运行按钮焦点区。它们不是完整初始弹窗或全App截图。初次目视发现30分钟有效期只显日期，最终补充时分展示；该小修的定向证据与原门禁／两图分开，未重8项业务、构建或截图。

末次小修只改有效期一行：现有Intl日期格式追加short时间，并保留time.datetime机器时间；原正常预览用例增加可见HH:mm及机器时间断言，真实RED后该1项通过／10跳过，18个不同Web不再加计。Root分别读取最终组件与测试的Serena语义诊断，均为空。最终组件SHA`a87285692c452474048b553327d760828dddcfa98c9248e47191800fbd3a59c6`；窄修receipt SHA`d72fdfe9e10fcbfa50494cc357285ff31ac4c779a5f01430d12bc7d567c01c87`。逆向还原这一展示行和一条测试断言可精确重建两个原文件摘要，另6个Web文件不变；原完整TSC／8业务／两图仍绑定此前源码，不能冒充最终时分文本又经过浏览器或全量类型检查。

页面、Electron窗口与代理正常关闭，native／proxy／Go进程均退出0，10个观察PID不再存在，两个端口释放，无强停。Worker协作停止返回context_canceled_after_normal_stop，不能写成Worker退出0。原临时DB／DataDir清理前以参数化VACUUM保存2,891,776字节一致私有快照，SHA`25da52ce9aeb5228049d4d82a4584941c3a79e5e13bc6094be11ca10d9d3d3b0`；原临时路径已不存在。

安全交付位于忽略的`dist/quality/tool-execution-168/`，13个载荷加manifest共14文件；manifest SHA`17ff8dd9ece24b3e41efcefdefa3c489213e58cd39dae7c1e338ef690f48fd98`。源指纹、Worker HTTP、未知回执、退出、布局、两图与摘要逐项一致。cookie／token／密钥／原正文／数据库／浏览器配置／二进制和私有日志不进入安全导出或Git。宿主二进制仅私有留存，SHA`3633ac6883da7c51d26f4cc114666e48809df7f07341ca9f6ee2ad8a383985c4`。

固定测试计划只保存范围快照；无测试映射、测试报告或通过记录。前端关闭Dialog后保留writer，身份／组织／项目／调用方变化清除旧内容；计划GET的valid不能授予执行。实际全App导航、其他主题／设备、无障碍完整矩阵不从组件联调推定。

## 未完成范围

真实WorkBuddy账号及模型／费用、持续ACP输入与权限、可信云候选／基线、同提交独立测试和正式验收继续单独交付；供应方终态不会解锁这些门禁。通用ACP／Qoder、其他工具、物理多机、其他数据库、容器隔离、安装升级、容量及生产验收继续按完整平台目标推进。既有Run公共Node空数组／null协议类型的全局收口尚未完成。

阶段交付只提交必要源码、同源合同、设计文档及本报告／摘要；缓存、私有DB、凭据、原始请求／日志、二进制及截图均不提交。原有21项工作按既存摘要保护，Git交付结果在最终提交后另行核对。本次完整TSC／语言／490文档链接及380路径／3500Schema引用各检查一次；后续状态／证据文字不重复全量检查；三份设计入口补充的7个现有本地链接已单独核对。
