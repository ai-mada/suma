# 工具准备与准确交接限定验证

T-TOOL-PREPARATION-203；suma 1.0.0；2026-11-02。状态：本阶段完成限定验证。完整31项／P0–P7保持active，58 limited／34 pending／92项基线不因此改写。

已选工具沿原Quick、工作区和准备指南交接准确协议与节点，同一worker上的旧配置不能覆盖新工具选择。Claude SDK与OpenCode SDK准备失败保留六类有限原因，仅在对应SDK的unavailable连接事实中展示；不返回路径、秘密值、原异常或stderr。原因不是配置、模型认证或健康权威，未知登记、续权、停止与原回执恢复保持优先。

| 验证 | 已观察结果 |
| --- | --- |
| 定向检查 | 29个不同顶层全部通过，14新／15直接受影响旧：Go4（3／1）、宿主7（3／4）、Web诊断7（4／3）、交接UI11（4／7）；续验不重复计数 |
| 类型与中英文 | Web完整类型检查1次exit0；国际化1次exit0，9407成对键、生产硬编码违规0；最后诊断显示调整仅补单文件语义检查exit0 |
| 构建 | Go真实编译1次成功；此前PowerShell参数绑定失败发生在编译器启动前，计0次编译。Web隔离组件构建3次，后两次分别对应必要CSS修复及最终诊断显示修复 |
| 真实命令行 | 1次原Runner读取界面生成的七字段JSON，exit0，写出全新单命令固定app-server配置；后续精确复用，不再次执行 |
| 代表界面 | 中文1366、英文390两个独立组；3次浏览器启动、5次组执行。最终中文续验通过，英文复用已接受的单列44px界面，两张最终图均由Root实际查看 |
| 独立审查 | 宿主、Go配置准备、Web诊断、UI交接关键增量分别只读审查，遗留确认P1／P2为0 |

新增的`--prepare-codex-stdin`复用原Runner固定Codex App Server规范、native文件检查和rooted、O_EXCL、Sync全新文件写入器。七个明确输入为adapter、原生程序路径、provider、base URL、凭据环境名称、Windows sandbox及user-input布尔值；不接收模型、argv或秘密值。混合模式、显式false模式和无效目标先于私有输入或身份读取拒绝。新增准备模式1个，HTTP路径、数据表、依赖、权限和公共Schema新增均0。

真实命令行使用自有`suma-runner.exe`作为被检查的native文件夹具，不运行它冒充厂商Codex程序，也不验证厂商版本、模型、登录、Worker或控制面数据库。界面API／IPC／配对观察均为合成数据；真实CLI配置生成与组件交互证据分开记录。真实命令行程序、输入和输出配置SHA分别为`5270e97f0d3a183108854b4b4b664ebc3f24e5e2ca7e493a1536619ccf6a4584`（程序）、`4f5f8759db74f5eda3a10fc0c3e5483c44297ebac4bb8c66e3ef3938bb16a5d2`（输入）、`4a93aacd6c2618d94d14be40b6d2f4dfce088ac508af0589c91b3dbd68761012`（配置）；后续GUI仅复用相同依据。

两项确认P2均保留真实失败与修复后受影响检查。宿主异步资源失败原本绕过当前授权／来源核验，现在所有SDK准备早返回先复用原authorize、caller和context核验；权限、调用者或绑定漂移优先于旧准备原因。Workspace原先从可清空选择状态推导序号，清空后新选择被消费者高水位拒绝；现在所有非空生产入口使用独立单调ref。原SDK登记未知也先以同一body、key和worker核对原回执，仍未知时不开始新的资源观察；ACK已知后才继续本次明确连接。

首轮GUI为`20261102T024658454Z`：中文功能通过，但技术摘要实际32px，未达到44px；英文功能通过，但Root拒绝其双列窄屏布局及拉伸控件。修复限定于组件CSS令牌／选择器和Codex窄屏单列，不改全局密度或使用important。CSS续验`20261102T025606525Z`两组通过。随后只去除准确准备原因旁不相关的通用Git／工作区提示，保留未分类unavailable的原提示、busy进度和未知操作核对；仅原受影响检查1项与单文件语义检查续验。

最终`20261102T030952785Z`只重跑受影响中文组，44px触达、无溢出及准确处理入口通过；英文精确复用025606的已接受截图与观察，不宣称又运行英文或真实CLI。最终结果SHA为`a34ced6b4de9b4349179ab935d1f2be93c117c97626f03d43ba0982eb722c0dc`。最终两图唯一归档为：

- `web/visual/images/20261102T025606525Z-en-codex-preparation.png`，SHA `de0a523cbbb5264c94506d9b9372c81b9c01921da051c6ffc019c169e03ca2cf`。
- `web/visual/images/20261102T030952785Z-cn-preparation-recovery.png`，SHA `111ac1a58e35304459907a1082e0e4e4e5f22b58cfdfdd5c8203ebd9f66d906e`。

归档边界见忽略目录内`image-archive.json`：025606两图在末次中文续验前已复制到唯一归档；初始失败中文图保留。早期024658英文双列图曾由Root查看，原hash、结果和来源记录保留，但原本机PNG别名在CSS续验中被替换，未另存其磁盘字节；不称全部初始截图完整保留，也不重渲染制造旧证据。

参见[机器摘要](tool-preparation-recovery-summary.json)、[合同](../../contracts/tool-preparation-recovery.md)和[完整实施计划](../delivery/implementation-plan.md)。原命令、失败、来源清单、批准overlay、日志及可用图像放在忽略目录`.cache/tool-preparation-203`，不进入Git。真实Electron／原生IPC、厂商工具与模型、多机、其他数据库、跨平台安装、规模、生产和完整平台仍需各自交付与验收。
