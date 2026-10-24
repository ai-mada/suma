# 工具执行运行时与原生 Agent 实测

状态：T-P4-004 域内回归与本机只读授权实测通过；完整产品和正式交付由主集成单独验收。日期：2026-10-24。负责人：Runtime代理与主集成。关联：[工具执行合同](../../contracts/tool-execution.md)、[运行时设计](../agents/tool-execution-runtime.md)、[原生内核](../agents/runtime.md)、[证据入口](evidence/tool-runtime/README.md)。

本报告证明本机工具探测、固定配置与入口指纹、结构化输出、库存故障处理，以及原生CLI访问真实控制面的具体范围。模型端均使用明确标注的本地fixture，没有真实模型、厂商账户、计费或生产验收结论。主集成分别记录初次Go479／初次24包及后续post-review追加全套与运行包；这里的111、8和3属于不同切面，不相加为产品功能数量。

## 不同源码切面

| 切面 | 实际结果 | 证据含义 |
|---|---|---|
| Runner域内最终回归 | `go test ./internal/runner -json -count=1`：111个通过事件，0失败、0测试跳过，61.973秒；`go vet ./internal/runner ./cmd/suma-runner`退出0 | 含父／子测试和进程helper；当时39份Runtime源文件前后摘要一致，原生CLI5项及类型检查通过 |
| 后续原生模型工具名称补充 | 原生全套8项、类型、构建与CLI version/help均退出0；补充RED为5通过、3失败 | 仅`index.ts`和`runtime.test.ts`变化；33份Go源码与旧切面保持字节一致，新增原生8项不冒充旧5项的结果 |
| 原生CLI→真实控制面 | 三个实际授权用例通过；真实查询核对同一3个Task ID | Node和控制面是真实进程，模型接口是本机HTTP fixture；不代表执行过真实厂商模型 |

[原始111收据](evidence/tool-runtime/runtime-verification.json)、[原39文件快照](evidence/tool-runtime/runtime-source-start.json)保持原字节；[TS补充收据](evidence/tool-runtime/native-alias-verification.json)保存两文件旧／新摘要和当时原生源码、构建摘要。[历史核对](evidence/tool-runtime/source-cut-verification.json)记录归档时匹配关系，没有把历史收据改写为后续源码已经通过测试。之后的Codex用量兼容修复另有新Go切面，见下方补充。

## 工具执行与故障语义

实际进程与HTTP测试覆盖以下边界，最终通过记录在[Runner原始JSONL](evidence/tool-runtime/runtime-suite-final.jsonl)。

- 只有本机允许且可用的命令声明`command:<id>`。禁用执行、缺少Git隔离、未安装或不支持的协议不会被标记为可派发；非法配置及超128条配置明确拒绝，不截断。
- 配置与实际可执行文件摘要绑定profile；Suma还绑定唯一JS入口字节。实际Node测试保持路径、版本及解释器不变后修改入口字节，确认指纹变化和执行前拒绝。该绑定不覆盖入口导入的完整模块树。
- stdout JSONL与有界脱敏stderr分开。分片UTF-8、消息及有限活动按白名单转换；原始工具参数、命令和推理内容不进入结构化事实。stderr里的终态不能伪造成功。
- Suma事件匹配本次stdin request_id；Codex按thread.started→turn.started→一个终态推进。缺失／外来ID、重复开始、终态后活动、格式或终态冲突不能成功。官方必需用量字段缺失不会被填成已报告零值。
- Codex只接受固定stdin与匹配sandbox的preset；字面prompt、resume、cwd、配置和危险bypass覆盖明确不可用。模型文本和工具事件不能替代DAG最终结果、独立测试或人工验收。
- typed进度与最终证据持久化后回写，稳定operation在丢ACK及重启后保留。库存503测试中，真实200ms进程继续完成且心跳持续，新领取等待库存恢复；原报告先以原operation恢复，再创建新TTL报告。取消依赖实际OS停止事实，工具缺终态不会被伪装成成功。

早期[回归失败](evidence/tool-runtime/runtime-suite-attempt1.jsonl)中，旧测试夹具以空capability领取命令；收紧生产命令匹配后，夹具改为真实`command:local`，没有增加生产绕过。最初API／类型未接线的编译RED与实际协议、脱敏、指纹行为RED分别保留，不能把编译失败描述成已执行故障注入。

## 原生provider工具名称

控制面使用`project.overview`等canonical名称；模型provider的函数名使用确定的`tool_`加canonical SHA-256前32个十六进制字符，符合[官方Chat FunctionDefinition名称限制](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create)。单次授权目录建立严格反向Map；模型不能直接用canonical名称或未知别名绕过。HTTP invoke及工具事件保存canonical，assistant工具调用历史保留provider别名。

[实际HTTP fixture测试](evidence/tool-runtime/native-alias-green.txt)验证合法wire名称、`project.a`与`project_a`分别映射、canonical反向派发、assistant历史，以及未知名称和重复canonical目录拒绝。实现还包含alias冲突检查；没有声称制造过真实SHA碰撞。构建输出为`tsc`，CLI仍报告`suma-agent-runtime 0.1.0`；旧Go收据不因此被覆盖。

## 真实控制面只读授权旅程

服务为`127.0.0.1:4322`独立验证控制面。已构建Node CLI使用本机临时HTTP模型fixture，该fixture严格校验函数名并从真实目录的`project.tasks`描述选别名。第二次模型请求实际携带控制面查询结果，确认三个真实Task ID后才返回completed。

| 场景 | 实际CLI退出 | 模型fixture请求 | 核对结果 |
|---|---|---|---|
| 独立Agent凭证，Runner环境放错误decoy | 0 | 2 | 真实project.tasks结果的3个ID与授权基线一致；canonical事件正确 |
| 将Runner凭证放入Agent凭证变量 | 1 | 0 | 真实控制面403，未发出开始／完成或工具执行事实 |
| 使用不属于该凭证的项目参数 | 1 | 0 | 真实控制面403；没有创建另一个项目，此用例不冒充真实跨组织全矩阵 |

[机器收据](evidence/tool-runtime/native-live-scope.json)、[原始事件](evidence/tool-runtime/native-live-scope.stdout.jsonl)和[复现辅助程序](evidence/tool-runtime/native-live-scope.py)保留该范围。期间主集成可能推进任务，检查只固定ID集合，不断言状态或版本不变。没有创建项目、任务、令牌，没有启动Runner或修改控制服务；自建模型fixture已关闭。没有provider API key或计费请求，用量未报告时不把原始零占位解释成实测token消耗。

### 候选程序与交付字节分别记录

上述三用例发生在候选控制程序`1e61a39a31554065a1486dc1ab40486296672d605d982b9169895c739e477021`，不是初次045219交付控制程序`c06d3553c46278704dd976f5ab1dcef52d3b25a96f48d76e3bed4d08092b4eb8`。候选身份来自主集成当时的运行记录；原生原收据本身记录endpoint与原生编译文件，没有保存控制进程字节摘要。

主集成记录两者使用同一116项运行包输入，即114个Go源文件加`go.mod`／`go.sum`，输入集合摘要`498300c9adffa8327982faa25da7d745d1a9ecb08f62265691d5673deacb318e`。[来源说明](evidence/tool-runtime/control-provenance.json)保留当时切面；原字段中“final delivered”是该次记录当时的称呼，不覆盖后续post-review运行包。相同源码输入不能将测试改称在另一份二进制上执行。初次Go479与之后追加审查、GUI／Runner运行及最终字节见主集成[总验证入口](verification.md)和[运行包记录](../deployment/tool-runtime-bundles.md)。本报告不替代这些结论。

## 已安装 Codex 真实只读探测补充

通过初次045219交付Runner的`--describe-tools`，对既有官方App目录中的native `codex.exe`执行只读`--version`与`exec --help`；没有执行PATH前面的npm `.ps1/.cmd` shim、安装程序、登录或模型任务。本次未启动Runner轮询／任务模式，未提供project／worker或控制面token，只启动初次交付字节的独立describe进程。

| 项目 | 本次实际观察 |
|---|---|
| Runner切面 | 初次`20261024T045219750Z-715ba2c8`，SHA `12c8681ec9d7e1ba325252e264e5d7aaa7d73d9c053cc3d6c3466292b44f56cb` |
| Native exe | `C:\Users\Administrator\AppData\Local\OpenAI\Codex\bin\8925276705115dea\codex.exe` |
| 版本 | `codex-cli 0.164.0-alpha.11.1` |
| exe SHA-256 | `c6144bc19b2705f25a40d8488b3d57d17a223c5df3c5f65e3c58d72601956c1e`，探测前后相同 |
| profile SHA-256 | `d2e4575891eabff50e33d1ae48453b2a488bcb28f0888f3b93259bd75a2f3afe` |
| 本机结果 | available=true，describe退出0；原因仅installed executable and protocol probe，model authentication and task success unverified |

配置校验使用`exec --json --sandbox read-only -`、`read_only=true`，实际调用仍仅version/help，没有执行该task preset。`--allow-exec`只用于本次本地描述的显式许可，不表示已向平台登记worker、启用profile或验证模型认证／健康。没有读取privatecontext、使用token／provider key、请求控制面或模型API；没有做OS级网络捕获。

[新补充档案](evidence/tool-codex-probe/README.md)保存describe原文、原始回执及切面说明。既有41份Runtime档案与摘要清单逐字节未变化；本次只读CLI证据不冒充后续post-review程序或完整厂商执行验收。

## Codex用量协议版本补充

追加审查核对了两份官方Serialize源码：[rust-v0.100.0](https://github.com/openai/codex/blob/rust-v0.100.0/codex-rs/exec/src/exec_events.rs#L54-L63)只定义input_tokens、cached_input_tokens、output_tokens；[锁定970111f6](https://github.com/openai/codex/blob/970111f6cb62cc02fe36a82ea7d71b37a2889c97/codex-rs/exec/src/exec_events.rs#L54-L68)还定义reasoning及cache_write。当前[官方非交互文档](https://developers.openai.com/codex/noninteractive)示例含reasoning。不能用旧示例断言本机0.164实际只发三个计数，也不能用最新Rust字段要求拒绝旧官方合法终态。

归一化现要求三个基础计数，两个拆分可选；存在的所有计数严格校验非负int64。未报告拆分不输出到wire、不估算费用、不当成报告零；既有omitempty同样省略显式零拆分，因此本次不宣称完整保留“报告零／未报告”的所有差异。空usage、基础字段缺失以及任何计数的null、字符串、布尔、负数、小数、溢出均拒绝，缺失可选拆分不影响一致成功终态。

新增目标回归先真实失败：三计数解析与exit0本机fixture进程均被错误拒绝，RED共12通过／4失败事件；最小修复后16通过／0失败、0跳过，package耗时3.272秒。[新补充证据](evidence/tool-codex-usage/README.md)保存RED／GREEN原始字节与三个Go文件摘要。测试中的Codex只是明确标注fixture-transport-only的本机进程，没有模型、登录或厂商API。原111项与初次24包收据未被覆盖；新全Go与后续打包结果由主集成记录。

## 归档与限制

35份公开原件逐字节复制，[归档映射](evidence/tool-runtime/archive-map.json)记录来源、大小及双份SHA；[完整摘要](evidence/tool-runtime/files-sha256.json)覆盖归档目录除自身外的文件。[敏感检查](evidence/tool-runtime/sensitivity-review.json)对照当前私有上下文值及高危模式，未发现真实秘密。单测中静态假bearer字面量保留并单独标识。没有归档私有context、真实令牌、命令配置、数据库、外部登录文件、进程日志或二进制。

这不证明真实厂商模型／账户、全部角色权限、远端会话恢复、输入中介、完整终端、外部桥接重启查询或完整传递供应链已经验收。OpenCode／Hermes／dsh完整执行协议、OceanBase／PostgreSQL真实集群、跨平台实机与目标规模仍分别待验收；完整范围保持在[实施台账](../delivery/implementation-plan.md)。
