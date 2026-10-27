# Codex 模型配置到真实 CLI 编码：限定验证

状态：T-P4-041／1.0.0，本地纵向功能验证；负责人：控制面、执行器、工作台。日期：2026-10-27。关联需求9、12、14、18、21、22。[合同](../../contracts/codex-app-server.md)、[PRD](../product/codex-model-execution.md)、[架构与AI SOP](../integrations/codex-app-server.md)、[交互](../design/codex-model-execution.md)、[摘要](codex-app-server-summary.json)。完整P0–P7及需求1–22保持实施，不据此标记生产或全平台完成。

## 实现与证据

| 能力 | 结果与边界 |
|---|---|
| 实际工具目录 | 已安装Codex以独立home完成initialize／model-list，实际8个模型；显式模型、effort和正整数提醒阈值；目录不证明厂商账户授权 |
| 固定配置链路 | 同一任务Plan／节点／attempt保存独立CodexBinding，Preview／Start／Claim重查目录、工具及当前授权；拒原生绑定、私有覆盖和enforce预算，不静默降级 |
| 真实编码 | Control HTTP→Go Runner→实际Codex stdio RPC→自有Responses HTTP→实际apply_patch→独立工作树source.txt修改→7条持久进展及原结果ACK |
| 用量与账本 | 原thread／turn累计快照52输入＋36输出＝88 Token，partial、未定价，notice越界；账本没有填入未经证实的提供方模型、端点或费率 |
| 交互与恢复 | 中英文切换保留模型、推理强度及40 Token提醒草稿；固定预览后刷新仍恢复同一计划；未Start，未产生额外模型请求。错误2xx和原Start核对失败保留原key |
| Windows配置 | typed显式backend；真实ACK分别核对cwd隐含根和附加根；路径通过已打开文件最终路径核验，非致命stderr不污染版本stdout |

最后联合测试为`TestCodexExecutionActualInstalledCLIAndObservedUsageLedger`，明确环境开关与精确安装文件摘要后运行；默认跳过，不被普通套件意外调用。命令回执`20261027T131710093Z.command.json` exit0，300.94秒包含GUI等待，不当作性能指标。实际CLI为0.164.0-alpha.11.1，SHA256 `1103e9e09c71fa2cd0afd6d8f3b9a2d63174fe8efd7cb5bd09f1cb14ef58a098`。完整回执、源码变更证据及请求／响应摘要位于忽略目录，摘要给出准确路径和hash。

本阶段共4次自有协议HTTP请求：失败编码2次、最终成功2次；0厂商推理。成功运行使用1 Token提醒阈值，GUI另一个待办任务只预览40 Token阈值，二者不混算。正式测试、人工验收和发布操作均0；代码修改成功不能替代独立质量门禁。

## 实际界面

GUI只预览任务a8118ce38a20fedb4649c92391b851a1，模型gpt-5.5、中文“中”／英文“Medium”、40 Token提醒。固定计划`896d775e0c80a808a603284636900cec`刷新后仍为同一版本；确认框未勾选，Start未点击。产品自有文案已切换，用户输入的任务／流程名称保留原文。相关console警告／错误0。证据截图`.cache/e2e/codex-execution-stage-041/browser/codex-model-preview-zh.png`不进入Git。仅当前桌面视口实际验证，没有扩展为完整设备／主题验收。

## 保留的失败与诊断

1. `actual/ui-final`在目录探测失败，尚未调用模型：受保护祖先目录枚举失败与版本stderr混入分别修复，保留原FAIL。
2. `actual/ui-reviewed`实际thread ACK为只读而拒绝，0模型请求；显式Windows实现与generation2配置摘要补齐，未放松写入门禁。
3. `actual/ui-final-v2`因主代理在web-v3构建完成前启动夹具而失败，已建的隔离运行未领取；0模型。属于执行顺序错误，保留现场，后续先核对产物。
4. `actual/ui-final-v3`配置ACK成功，但apply_patch不能写入，2次自有HTTP；原失败结果和40 Token已知部分用量仍回写，没有记成成功。进一步零模型command/exec诊断定位外层受限Windows进程嵌套CreateRestrictedToken错误87。
5. 正常宿主对照仍保留内层workspace-write、sole-root、network=false及approval=never。第一次对照因PowerShell受限语言拒.NET方法而失败；改为允许的Set-Content后实际写入成功，再仅重跑必要完整编码路径。未切换到无隔离模式、修改全局权限／ACL、读取用户登录配置或绕过内层安全策略。环境前提纳入接入SOP。

15项scope正反例覆盖错误／额外runtime根、额外附加根及Windows配置；真实no-turn ACK确认与零模型写入诊断分开记录。目录与无turn确认不作为模型执行证据。

## 必要检查与交付

只运行受影响合同、DAG、Control、Runner及语言边界。Control9组、DAG3组和共享合同检查通过；新增覆盖字段保留RED→GREEN；Runner受影响5组后仅重跑路径／版本／scope变更。15项ACK范围检查通过，Windows创建symlink子项因本机权限跳过并如实保留。39个不同Web用例、4610对语言键、最终直接TypeScript、生成合同及受影响包vet通过；重复次数不计为新用例。没有新增依赖或重跑无关完整套件。

Windows5程序最终构建`20261027T133225401Z-53378c2c`通过，449项生产Go输入前后摘要相同：`cbc81fa4da3b3f0ccbd441acd287f643570175393b69c1bb115470debbbc096e`。Web最终web-v3共137资产；该次没有完整构建前hash清单，记录当前290项源码hash、mtime、4项明确双语变化及产物hash，不声称hash前后一致。原失败、原始输出及较早构建仍保留。

主预览原040进程及holder均由Windows错误87确认缺失且4324空闲后，使用041新构建隐藏启动；先备份原WAL一致数据并比对184条DDL原文，没有还原备份。启动前后188表完整内容／结构相同，原会话GUI读取后184非锁表相同；4张锁表身份不变，仅sequence协调变化。当前基线20session／20authority／0Grant，运行仍1failed／2succeeded；本次没有新增登录、凭据、Runner或模型运行。旧040文档18条是历史时点记录，不能代替本次实际基线。



最终并行复核发现并修复取消窗口：initialize／目录／thread ACK前取消，以及session_started持久化回调失败后，原代码仍可能继续提交下一RPC。先保留四分支RED，再以atomic停止状态、统一新请求闸门和输入关闭修复，受影响驱动组9.215秒、精确四分支最终0.144秒通过。原真实编码成功发生在这一修复之前；最终代码仅增加停止闸门，已重建Go、定向回归及Runner vet，没有为了同一成功路径再次调用模型。第一份041构建及原日志作为历史保留。主041初版随后正常退出并以前向构建替换，未恢复任何数据库。

## 后续范围

团队／冲突修复入口独立Codex选择、其他CLI协议、模型授权／完整报价与策略、Codex远程文件broker、PTY／LSP／DAP／全IDE、实体多机与TLS、OceanBase／规模、跨平台安装、渠道／SSO／商业化与生产隔离／网络阻断仍继续实现。observe-only不满足硬额度项目；配置ACK不能证明提供方实际模型或账单。
