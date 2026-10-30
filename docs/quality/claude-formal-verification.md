# Claude正式Worker链路与回执恢复验证

阶段T-CLAUDE-FORMAL-159；产品1.0.0；日期2026-10-30。Windows SQLite、正式HTTP／Worker、受控CLI协议及人工拒绝回流已完成限定验证。真实Claude安装、登录和供应方调用均未验证；完整58 limited／34 pending／92总项不变，平台目标未完成。执行合同沿用[Claude CLI合同](../../contracts/claude-cli.md)和[158实现记录](claude-cli-verification.md)，机器口径见[本阶段摘要](claude-formal-summary.json)。

## 实际链路

本轮通过原任务GUI明确预览、单独确认Start，沿正式控制面和两个本机注册Worker身份执行。开发Worker运行一次原生受控CLI协议样例，收到1264字节完整固定任务stdin；生成一个真实干净提交及原候选bundle。不同身份的独立测试Worker导入同一准确提交，运行Node测试并提交341字节`suma-junit-report-v1`报告：1例、1通过、0失败、0跳过。

原Run始终只有一个，ID为`45dcca7671ba25fcbdf390e14a9be736`，状态succeeded；实际工作流为`managed-45dcca7671ba25fcbdf390e14a9be736`、版本1。开发和测试节点均保持attempt1／fence1及准确工具绑定。两个journal各1条记录，result_acked均为true、pending_results均为0。原脏草稿和两份绑定仓库HEAD均未改变。

候选提交为`3b048d070fbd169c02e2a7897035fc2e63dbd25a`。正式Worker／HTTP、真实Git提交和独立Node测试已经运行；CLI任务端是受控协议样例，原生任务调用1次、实际供应方0、模型调用0。固定模型回显和partial observed usage仅证明协议证据呈现，不能解释为真实厂商模型成功或完整计费。没有seed逐用例正式任务测试计划，不能用这份JUnit替代完整测试库验收。

## 范围与计数

以下是不同范围的原记录，不相加为单一通过总数：

| 范围 | 记录与结果 |
| --- | --- |
| HTTP准备 | 10项断言；journal共46次请求：30读、16 POST，其中15成功、1拒绝；不含浏览器及Worker传输 |
| 准备写入 | 初始业务／工作流14次，后续正式Review请求1次；原成功写入未重建 |
| 正式执行事实 | 7项断言；不同本机Worker身份2个、原Run1个、开发CLI调用1次、JUnit1例通过 |
| 初始GUI | 记录7项断言；预览和原Start共2次POST，产品回执校验失败且driver退出1，不能标为整轮成功 |
| 原Start恢复 | 2项检查完成；仅1次原body／key重放，原pending清除并打开同一Run；随后SSE等待超时使driver退出1 |
| 只读续验 | 5项检查通过；写入0、外部请求0、renderer错误0，driver退出0 |
| 独立reviewer | 7项检查通过；明确拒绝1次，driver退出0，无额外写入、外部请求或renderer错误 |
| 浏览器操作总边界 | 正式业务POST传输4次，其中唯一新变更3次、原Start重放1次；Run和开发没有重复 |

实际GUI使用原159数据、工作流、候选和Run。中文1366截图为owner查看原执行；英文390深色视口截图为独立reviewer拒绝后查看同一执行。原记录已用view_image检查两图；长手机截图保留但未交付。不同本机身份不等于物理第二主机或共享Pool验收。

## 实际发现与修复

初次Start已经成功创建正式Run，但前端错误地把Run的`workflow_id/version`与预览中发布工作流的ID／版本比较。`CreateManagedRunBound`实际生成`managed-<runID>`、版本1，原Plan继续保留来源工作流；因此成功HTTP回执被当作未知，原意图仍待恢复。

同根追踪确认4个实际入口：直接任务Start、讨论开发Start、终端候选Start和候选交接的Run读取。修复共用当前Plan／保留run_id的准确托管Run绑定，保留各入口原资格、节点、来源和模型检查。其余模型选择、推荐采用和候选预览命中的比较操作是Plan与请求／来源关系，不属于这个Run身份错误；没有据文本相似扩大修复。

源码输入为peer的7个文件（4产品源、3测试源）及root的2份通用中英文错误文案，共9项。原定向RED分别记录直接入口3项、讨论1项、终端变体2项失败；最终只跑受影响3个测试文件，6例通过，包含3新增、3增强旧例。另2个无关旧终端用例未选运行，日志显示skipped；不计为通过或新增。未扩大旧套件，Go源码及Go测试未因此重跑。真实159GUI只覆盖直接派发旅程，讨论／终端变体只有组件覆盖，没有新增对应实际旅程。

## 恢复边界与异常保留

旧窗口进程已确认退出，新窗口初始实测没有同scope pending。测试宿主在确认旧窗口关闭后，重新载入原GUI已经发出的非秘密body和key，再完成原操作重放与同Run确认。该事实验证原操作恢复，不证明产品可跨窗口自动持久恢复；该完整目标缺口仍未实现／未验证。

原Start恢复的两项检查已经完成后，driver在真实持续SSE页面等待networkidle而超时。随后按元素就绪继续5项只读检查，没有重新POST、创建Run或执行开发。原退出1和失败记录保留，不能把后续只读通过改写成整个driver从未失败。

准备时缺少操作key的邀请接受被400拒绝，之后没有重建邀请或重做成功变更；初始模态语言选择器定位超时发生在写入前。原浏览器关闭协议异常及其进程不存在审计也保留。以上驱动／准备异常与真实托管Run回执产品缺陷分列，没有补造缺失日志或额外旅程。

## 人工反馈与执行结果分开

独立reviewer从个人收件箱打开原正式Review请求。查看来源及填写决定没有自动标记已读或提交。一次明确拒绝后，原请求closed／decision=rejected，任务版本3、status=blocked；生成Issue版本1、status=open，与原task／Run及同一review_id准确关联。通知仍未读，原Run仍succeeded。

这证明执行成功与人工交付接受是独立事实。当前结果是拒绝且问题待修复，不是已接受、已部署，也没有完成修复、复测或重新验收。

## 冻结来源、构建与收尾

本次复用已冻结控制面和Runner，不新建Go平台二进制；受控协议样例编译1次。修复后的隔离前端仅构建1次，Vite退出0、48.17秒；类型检查退出0，7705对语言键／参数／复数／标记检查通过，生产硬编码0。构建日志仍含chunk-size提示，不声称无警告。

隔离候选基于`8042114b73d166443140a50e01972a3546f5fc01`，源摘要`b0f3e48e4bede631ead5efdf150784eaf50ffb5c5e3031ba7ca5ddd47cd15295`，最终入口摘要`2af2aecde0cc153a0982d3f1c77412efc4472dfd6babd5bbf8908a8d4aa18adb`。候选只包含159修复和通用文案，排除在途160及其他WIP。控制面二进制摘要`f5f83b56b4ca362e3119ecce7ccaa1beb21998310fdbabeca9b625ac01da5097`；Runner摘要`44a2826536195f291576cafdd0924365c145f72d0af5bde7eb110c09e64e5bad`。

安全交付目录为`dist/quality/claude-formal-159/`，含两张截图、安全摘要和manifest；manifest SHA256为`72e27974175f60a643ffae4f0820d8c1047c1e31f42322bc379d4daa019a7bcd`。本报告已核对清单中三个文件的字节数和SHA，原修复冻结记录／候选来源及原始证据继续保留。未交付凭据、cookie、私有HTTP正文、原始任务输入、journal或数据库。

两个Worker及控制面均在准确可执行路径和UTC创建时间核验后Stop-Process，随后确认准确进程不存在；wrapper终结退出0不代表Worker／控制面优雅退出。自有浏览器已关闭，数据库、journal、仓库和原证据保留，删除独有文件0。本次写报告没有运行测试、构建或Git，也没有改其他源码／canonical文档。

## 尚未验收

真实Claude安装／认证／供应方任务和计费，跨窗口自动恢复、物理多机、共享Pool、分布式控制、其他数据库、规模、安装包及生产仍待验收。人工拒绝后的完整修复／复测／重验、合并、应用发布和部署仍是后续工作。58／34／92不变，不能以本轮受控端到端证据宣称完整平台完成。
