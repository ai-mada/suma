# Doctor维护闭环限定验证

状态：本阶段限定通过；T-TOOL-DOCTOR-199；suma1.0.0；2026-11-02。完整31项／P0–P7目标继续。

[合同](../../contracts/tool-doctor-maintenance.md) · [PRD](../product/tool-doctor-maintenance.md) · [架构与AI SOP](../architecture/tool-doctor-maintenance.md) · [交互](../design/tool-doctor-maintenance.md) · [机器摘要](tool-doctor-maintenance-summary.json)。

## 结果

原诊断在同一当前授权读事务内返回准确维护建议，保留四项派发前提及原next_action。原worker槽位、inspection／recovery、不可变来源、当前期限与索引复用；pending等待、unknown核对、proof_stored等待机器ACK，关闭后再明确新检查。Web可准确定位同节点另一配置的原操作，本机未决原body／key优先；CLI新增中英文table且保留默认JSON，只读不自动执行。无新API路径、业务表、派发器或依赖；生成合同新增一个闭形模型。

29个不同定向顶层（7新、22直接受影响旧），其中9Go／31Go子例另计、20Web／4无关skip。一次完整Web类型检查通过，9272对词条及语言／参数／复数／标记检查通过、生产硬编码0。一次成功合同生成及一次-check通过。未重复无关旧整套或原生SDK／模型／Runner执行检查。

一次真实HTTP／SQLite链路：22请求、8诊断读，覆盖原paused配置→明确检查→pending／claimed→unknown→明确原核对→requested／proof_stored→原机器ACK关闭→新检查ID；同worker另一配置指向准确原inspection并GET核对所属。每次Doctor读检查原业务操作／检查／核对／运行数量无新增，原四项与review_profile保持，业务run0。机器是自有HTTP合同夹具，零原生进程、零模型／供应方调用；零进程proof不能当作重新验证Windows原生停止，后者沿T195独立证据。私有SQLite快照、原回执及请求明细保存在忽略目录。

中文1366与英文390深色两组实际生产组件交互使用合成只读API，均通过；截图原文是夹具来源内容，不是界面硬编码。Root查看首轮两图后发现长弹窗标题使关闭按钮拉伸，修共享CSS为原44×44触控目标，保留原图后仅重建隔离产物并续验EN，复用未受影响CN截图／快照；两次原生宿主退出0，页面错误及非预期请求0，最终几何44×44。两组、最终两图、修前后保留共三图；未为CSS重复类型、国际化或20行为检查。活动预览产物未覆盖。

## 原失败与审查

新维护／CLI／闭形合同和Web定位的真实失败保留。旧Doctor两次GET整段比较曾因独立checked_at不同失败；仅修测试时间比较，仍核有效、非倒退及其余完整事实。一个未变新top在该失败组合命令内独立通过并复用，不宣称那条命令整包通过。Root合同检查一次曾遇并行helper未发布的vet依赖缺口；源码冻结后同项最终exit0，没有关闭vet。共享原ID规范收口后只复验受影响CLI新top。

控制面、非自写共享合同／CLI及Web消费者分别完成独立窄审，冻结输入摘要匹配。全部测试名、原命令回执、16个Web源及其他拥有者清单、实际链、界面输入和图像SHA见机器摘要；不会由本阶段窄审推导整个平台无缺陷。

## 剩余范围与复现

配置换新原子交接继续[后续计划](../delivery/tool-management-next.md)。真实厂商账号／模型、物理多机、真实OceanBase／PostgreSQL、规模、各系统安装及完整生产验收仍待分别完成。Multica本地固定提交已检查并记录许可，未复制源码，远端最新源码尚未锁定；官方README只作产品参考。

验证使用本机缓存Go1.26.10、Node22.16.0、Vitest3.0.5及已安装的Vite／Electron／Playwright，TEMP与产物均在项目.cache。关键入口为go test ./internal/platform -run '^TestToolDoctorMaintenance'、go test ./internal/cli -run '^TestToolDoctorCLI'、go test ./cmd/suma-contracts -run '^TestToolProfileDiagnosticsContractKeepsOriginalAvailabilityAndFourFacts$'及go run ./cmd/suma-contracts -check。实际链显式设置SUMA_TOOL_DOCTOR_199_ACTUAL=1；再次运行必须保留旧证据并使用独立final目录，不能清空原数据库重置通过。
