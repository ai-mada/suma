# 本地 ACP CLI 执行：验证与交付边界

T-ACP-RUNTIME-170；suma 1.0.0；2026-10-31；负责人：主集成／SDK协议／Runner／权限领域／工作台。状态：限定实现、定向检查及实际旅程通过。关联[合同](../../contracts/acp-cli-execution.md)、[架构与AI SOP](../architecture/acp-cli-execution.md)、[交互](../design/acp-cli-execution.md)、[接入说明](../integrations/acp-cli-execution.md)及[机器摘要](acp-runtime-summary.json)。完整31／P0–P7／58限定／34待交付／92总账保留。

## 实现与参考

通用stdio引擎复用既有Go ACP SDK v0.13.5，首个固定策略为Qoder；工具目录、配置、版本／摘要探测、暂停登记、启用、固定模型、任务预览／派发、原Worker／工作树、journal、候选及独立质量链已接通。原生Windows入口可配置，脚本包装器和真实厂商登录仍需独立验证。参考本地Multica固定提交`e8ae72c9dfc1b4474b61a9087f51b33cbee6a048`的运行时及交互方式，未复制当前附加许可适配源码，未新增依赖或第二派发器。

准确模型配置ACK先于一次Prompt；观察用量不冒充硬预算，组织／项目enforce在预览、Start、claim及accounting Check／Reserve均拒绝。一次权限绑定原attempt／fence／dispatch和当前机器／真人权威；同body/key恢复，always及不完整细节不批准。Browser保留人类已存决定，machine投递资格另行检查；原ID历史读取不授予新执行权限。客户端FS／terminal拒绝不等于CLI自身OS隔离。

## 定向检查与独立审查

36个Go不同顶层（协议8、权限13、Runner9、主集成6）与24个Web不同顶层分批通过；重验受影响原用例不重复计数。协议原25子项与审查新增3子项、权限5子项以及其他组内子例分开记录。没有重复原169、完整Runner或完整平台测试。

独立审查发现并修复三项P2：嵌套模型字段大小写别名覆盖准确ACK（含ACK后配置更新的异步窗口）；权限Kind及数字等原始值遗漏已知秘密过滤；权限回复字节已交付而Writer未返回时误判unknown。新增3个精确子例RED→GREEN，原25不重跑；审查独立复核最终6源及30份来源／证据摘要。

原协议证据SHA`a0c97c9cec48102ab8a267658aeb5d9f4829168b43cd8f863c12ca05c5dcd08b`，修订SHA`b518dba04cb0d13157d3b82d27a915df78aeaa37a17914649a927a4957eceec5`，最终6源摘要`d1e0ee997d7cb89d7d88b823ca58c1f9fa4d62717c7ff6f0157be034f077a55d`。Runner证据`9d3a82c467e97bfac884a90a6cd6a8486ec2da7901fc819c57555f651f81c162`；权限证据`973bef7014e3b69059f9f714ac29e8c77ffc0a3b0a531c6e06dd7aadce1e15fb`；Web最终26源及原始／wire修订关系见历史修订SHA`7d4ea87b740c545cb73ec3c241262b891a6bdf0010a4dc432cd6b124e408bdfb`。

真实失败保留：SDK字段、权限与写入时序RED；Runner生产进度首事件因共享协议白名单遗漏而零Prompt失败，现Profile与Event使用同一闭合协议目录并精准复验；权限Pool夹具漏字段和JSON空值夹具污染分别修正。主集成预算夹具首次未释放自身事务被终止，补显式rollback后仅重验2顶层／6子例通过。未将这些失败改写成生产故障或全部首次即通过。

共同检查：完整Web noEmit退出0；8087对中英文／占位／复数／标记通过，生产硬编码0。生成825声明模型／826schemas／391路径，6个新权限操作使用准确browser／private Runner／Pool凭据。生成合同测试验证真实handler DTO、状态和权限分离。文档链接与引用最终检查另记下文，不重复已完成共享检查。

最终一次`node scripts/check-docs.mjs`退出0：497份Markdown本地链接、391路径及3598个Schema引用通过。随后仅补写本检查结果，不重复执行。

## 一次实际旅程

101个当前来源独立核SHA并签门禁`7b67aa9a8b95a2f51abc8ff43c2fd42f24ba3c36d704d5abb639abd19d26e0dc`，显式上界120。保留最初pending配置与冻结回执；只执行2次必要Go编译（宿主、受控原生CLI）、1次Vite生产组件构建和1次旅程，四阶段及Node均退出0。

受控CLI明确不是Qoder，也未访问厂商或商业模型。真实Server／SQLite、当前假owner、正式任务及已发布测试计划、两私有Worker和生产组件完成8项检查：原任务与固定工具；GUI预览而不执行；明确Start／原attempt及fence；默认收起后准确权限详情与once选项；首决定已提交200后截断JSON及同path/body/key恢复；真实clean候选和独立同SHA测试；英文窄屏只读恢复；准确一Prompt／一权限回复及无正式验收的最终事实。

只有1个正式Run／1个权限／1个候选、2个attempt（开发及独立测试均attempt1／fence1），SDK Prompt1、模型选择ACK1、权限回复1。候选提交`7596a199b835a654fb8c9558177346f019d47b4a`由另一Worker在同提交工作树验证；正式发布测试计划revision2，JUnit报告1份／用例1项通过，来源和XML摘要current。Run succeeded v5，task仍in_progress v2，project-focus为`run_succeeded_review_pending`，reviews0且verified_run为空。没有自动验收、合并或发布。

Root及另一工作台代理实际查看两原图：中文1366×960权限卡包含准确有效期、原选项、安全详情滚动区和禁用always；英文390×844为滚动后的固定模型／ACK／停止及交付边界事实区域。无可见横向裁切，真实键盘焦点及生产CSS令牌16／24／18px记录在原旅程中。它们是生产组件代表视口，不是完整App、完整设备／主题矩阵或厂商验收。页面、console及外部请求错误均0。

宿主、Electron及代理正常退出0，两Worker按取消上下文正常排空；13个观察PID最终精确复核均不存在，首次短暂present及后续准确观察保留，未强停。2端口释放；私有一致快照3055616字节，SHA`90e079b5a411e7903e2d6e0c341e182fc5a38fe202df4a88f8210302edca798b`；原临时数据库已清理，原隔离仓库保留。此为同机两Worker，不能外推物理多机。

安全证据导出14份至忽略的`dist/quality/acp-runtime-170/`，manifest SHA`f2a30c9010e4371df1ea1ccd972667c4f6d3680c448b5c12141a72a9b1ede5db`。源代码、合同、设计及本紧凑证据入Git；原密钥／cookie／body、数据库、日志、二进制、profile和图片均不提交。原21项未提交工作按阶段前摘要保护。

## 剩余范围

真实Qoder／账户／模型及费用、官方包装器和自动安装／本机发现、完整App与跨系统、多物理节点、OS沙箱、强制预算、其他CLI／厂商策略、可信云交付、人工验收／合并／发布、存储及规模验收继续原完整目标。本次受控协议及当前组件不能证明这些项目完成。
