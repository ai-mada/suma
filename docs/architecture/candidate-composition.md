# 候选合成、精确测试与代码基线架构

状态：T-P4-031／1.0.0 限定本地已验证；日期：2026-10-27。范围与边界以专项报告为准。
依据：[合同](../../contracts/candidate-composition.md)、[PRD](../product/candidate-composition.md)、[交互](../design/candidate-composition.md)。

## 深模块与成熟执行接缝

`CandidateComposition` 是正常代码生产任务领域，使用既有 managed DAG、真实任务派发、可信 Runner command producer、
密封候选传输、独立正式测试及原 Review。管理员目标提供方及 `RepositoryIntegrationPlan` 的028职责不扩张。
前端只组织当前授权的服务端动作，不合并文件、不循环创建运行、不构造候选或审批证明。

每个来源复用 `RepositoryIntegrationBasis` 核对当前真实验收与独立测试，用 `CandidateBinding` 固定准确密封字节。
计划固定集成任务／流程版本、producer、2–16个有序来源、来源摘要、管理员基准和固定提交时间。
预览不创建run；明确启动在同一事务绑定任务和managed run，保存原调用凭据及路由，Go DAG保持唯一派发者。
只替换选中producer为内部 `suma-compose`，保留固定独立质量节点；不支持的模板形状拒绝，不静默删节点。

## Git 原生合成与不可变结果

Runner在独立attempt内导入准确授权bundle，使用成熟 `merge-tree --write-tree` 与 `commit-tree` 保留父历史。
父顺序、作者／提交者、时间、消息与operation固定，使同一次操作不能悄悄产生不同commit。
先持久意图后产生效果，候选只密封一次，上传／ACK重试沿用同一manifest和Journal。
冲突记录真实来源与文件，不产生候选；停止和不确定进程结果走原attempt核对，不凭exit0宣布成功。

Git执行禁止hooks、签名、继承配置、外部属性、custom merge driver、filter、textconv及网络helper；
必须使用隔离对象仓库或明确验证的局部配置闭包，不能仅清空global配置就称安全。
来源最多16、每bundle与输出各最多64MiB，并界定命令时间和输出。

管理员锚点B与下游选择的开发输入M分开保存。合成候选完整携带B..result对象闭包，
只持有B的另一机器也应能导入所有组合祖先。下游从M检出，但其新bundle仍须可由B导入，保留继承溯源。

## 准确执行与代码访问

协议 `suma.candidate-composition.v1` 的 `CandidateCompositionExecution` 固定计划版本、项目、B、有序来源、
sources SHA、固定时间和operation。assignment中的合成／基线能力只来自持久真实run／node绑定，不接受记录或流程字段授予。

`/runner/code-inputs/bundle` 和 `/pool-runner/code-inputs/bundle` 根据真实consumer assignment、input kind、binding与transfer返回准确二进制。
每次读取重核活跃租约、冻结输入成员、原委托、来源当前验收，完整保留Pool机器／slot／grant／credential／dispatch／attempt／fence。
不以知道transfer ID获得访问权，也不放宽旧same-run candidate resolver。外部blob IO在SQL事务外，读取前后重核授权。
已经发生的真实输出仍是事实，后续来源漂移阻止新批准／绑定／代码访问，不能抹去原结果。

## 独立测试、验收与目标集成

原 `candidate_from` 传递准确producer候选给同一run中的独立正式测试节点；调度测试合同保持不变。
组合结果是新的提交，不继承各来源测试通过。集成任务使用原浏览器Review固定准确run正式验收。
只有实际当前验收合格，才能进入既有028受管目标集成或基线绑定；028仍核对完整差异、目标CAS、fence与正负回执。
失败／unknown保留原run恢复；没有自动验收、自动覆盖目标或重新执行未知效果。

## 受保护下游基线

`CodeBaselineBinding` 固定id、project、target task／version、composition／version、accepted basis SHA、完整候选及binding SHA。
绑定要求当前已验收组合和未开始目标任务、预期task／binding版本、理由、明确确认及稳定key。
写入受保护表并增加task版本，使旧dispatch preview失效；替换待执行基线也是明确治理写。

下一次单任务派发或未开始专家计划的明确revision保存完整binding；不选择“最新分支”或自动采用后来组合。
实际派发和fresh code access再次检查task／version、源验收及完整链路。新run产生自己的候选、测试和验收。
不改已开始run、历史plan、IDE全局工作区或目标仓库权限。

`CandidateBinding.bundle_base_commit` 是可选完整对象包锚点，与本次所选开发输入 `base_commit` 分别保留。
共同锚点按 `bundle_base_commit ?? base_commit` 核对，不能为了相等而把实际开发输入改写成管理员基准。
消费侧分别核对普通候选测试导入与受保护代码基线导入，不因新增基线而放宽旧same-run证明。

## UI与AI SOP

默认折叠的同页组件区分只读预览、明确启动、实际执行／独立测试、原Review、028集成及独立基线绑定。
`TaskCodeBaselinePanel` 同时服务合成侧选择下游和目标任务侧选择已验收合成；两者复用同一双CAS、当前读取和原请求恢复实现。
Workflow的`CandidateCodeInputFacts`仅展示真实workspace evidence中的typed输入与Git事实，不发请求，不推导测试、验收或目标写入成功。
技术哈希与原件披露，系统状态中英，来源和人工内容literal。
先当前读取，后明确写；409读最新／保稿／重确认，unknown恢复原key，403或scope变化立即Abort与隔离。
只更新当前展开活动计划，不全项目轮询，也不由浏览器派发。模型不参与权限、Git合成或验收。

合同、源码、定向检查、同机真实Runner／Git、跨物理机器及生产分别记录。当前文档为实现依据，不是验收收据。

## 本地交付证据

有序固定两份已验收候选，Runner以Git原生三方合成精确M；另一执行节点对M独立测试后，测试账号明确Review，再将M显式绑定未开始任务C。C从M开发出新提交，另一节点再独立测试并单独Review。实际同机两Runner、2项新运行、8项正式检查；原来源、管理员锚点与protected baseline全链路可追溯。中英切换、同一来源深链和冷回读保持准确事实；冲突、未知和重试沿用原操作及质量边界。见[专项报告](../quality/candidate-composition-verification.md)。仅本地自有协议夹具验证；未调用厂商或模型，未报告用量不推断为零Token。真实远端、多机、生产验收、完整P0–P7及全部扩展需求继续实施。
