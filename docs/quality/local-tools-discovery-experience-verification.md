# 本机工具发现交互限定验证

T-TOOL-DISCOVERY-202；suma 1.0.0；2026-11-02。状态：本展示与交互切片完成限定验证。完整31项／P0–P7保持active，58 limited／34 pending／92项基线不因此改写。

工具扫描现在优先展示已发现、异常及当前选中的入口；仅将`installed=false && reason_code=not_installed`且非选中项放入默认收起的“其他工具”。两组保留原对象、顺序、九项协议及原检测依据、已有连接和配置动作，CLI／SDK不合并。统计只描述安装观察，不推导连接、模型健康、启用或派发资格。空态复用调用者提供的原节点接入入口。

| 验证 | 已观察结果 |
| --- | --- |
| 定向检查 | 10个不同顶层全部通过：6新／4直接受影响旧；纯投影3新，组件7（3新／4旧） |
| 类型与中英文 | Web类型、国际化各1次，exit0；9364成对键、生产硬编码违规0 |
| 构建 | 初始隔离组件构建1次；真实CSS缺陷修复后仅补1次构建，exit0；JavaScript产物SHA完全相同 |
| 最终来源 | UI5源＋投影2源，共7源；仅CSS以批准overlay覆盖旧manifest，旧摘要保留 |
| 代表界面 | 中文1366浅色、英文390深色两个独立组；2次浏览器启动、3次组执行，最终2图均由Root查看 |
| 审查 | 投影与非自写UI分别独立审查，无遗留确认P1／P2；CSS另有独立窄诊与受影响布局续验 |

原scope／当前调用者、StrictMode、缓存、过期、inflight与force语义保持。展开或选择不增加扫描、安装、配置写入或HTTP；只有原明确重检触发force。原生details／summary支持键盘，scope重挂载重置展开状态。选中未发现项仍在主列表。只有本次明确点击且原按钮持有焦点时记录一次焦点意图；行迁移后不覆盖另一个外部控件的焦点。

纯投影仅做一次有效GREEN，没有为了模块缺失制造RED。组件新增的3个展示行为先真实失败，随后完整7项通过；焦点断言并入原检查，没有伪称另做独立焦点RED。未重复Go、SDK或完整测试套件。

GUI首次默认执行因EPERM无法读取已安装Playwright，浏览器启动数为0；保留`gui.command.json`。授权后的首次浏览器执行中，英文组通过，中文按钮实际只有40.390625／38.390625px，未达到44px。Root查看失败图，独立窄诊定位共享foundation高特异性规则使用继承的`--control-height:36px`，覆盖组件按钮规则。修复只在`.local-tools-scan`设置`--control-height:44px`，没有修改全局设计令牌或使用`!important`。

CSS修复后仅续验受影响中文组并通过。英文原目标已达到44px，且前后JavaScript产物同为`bdd5b1ec81917f40f83f2d76be2501bf3b16e93c733738c2146e409a956f3b2f`，原图与snapshots精确复用；不宣称修改后重跑英文。两次已启动的验证辅助进程均退出0。最终`web/visual/result.json`的run为`20261102T013540098Z`，SHA256为`9f90ef24925e55e816260053d5c543f1b322565f273a52e684794c51af9ae59f`。

上述界面使用生产组件、合成安装观察及导航回调。真实API、厂商、模型、原生开发工具和数据库调用均0，不冒充真实Quick后端、OS IPC、Electron安装或派发验收。初始权限失败、组件RED、中文CSS失败、旧manifest与最终证据均保留；只复验必要变化。

参见[机器摘要](local-tools-discovery-experience-summary.json)、[合同](../../contracts/local-tools-discovery-experience.md)及[完整实施计划](../delivery/implementation-plan.md)。原始命令、来源、截图与日志位于忽略目录`.cache/tool-discovery-202`，不进入Git。真实厂商／模型、多机、跨平台安装、其他数据库、规模、生产及完整平台仍待分别交付。
