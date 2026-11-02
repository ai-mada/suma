# 团队工作台恢复：限定验证报告

版本：1.0.0；阶段：T-TEAM-WORKSTATION-RECOVERY-218；日期：2026-11-03（收据时间为 UTC）。
基线：`d13191e500070454ccfaf84f2ce543a49726d0e9`。依据：[实施合同](../../contracts/team-workstation-recovery.md)；机器可读证据索引：[summary](team-workstation-recovery-summary.json)。

## 已交付范围

- 项目工具的共享节点诊断可携带准确公开来源进入原 Pool 工作台，重读当前 Pool、准确 Runner 和原项目 readiness，再明确进入原配置准备与连接。Runner／Grant 位于首 50 条列表之外时仍可准确读取；原未知操作保持优先。
- 只读资格到期时保留同来源表单草稿，禁用生成、复制和继续；明确重读后重新核验资格。账户、组织、节点或项目来源实际改变时仍按原规则隔离。迟到目录结果遇共享来源失效会被取消并拒绝采用。
- 新增 `GET /execution-pools/{pool}/runners/{runner}`，沿当前组织 owner/admin、原 `poolAdminTx` 和准确 `poolRunnerTx` 同事务读取。接口拒绝 query，返回原公开 DTO，使用 `no-store`；不返回凭据、不读取消费项目内容。接口生成及 `-check` 通过，当前 **1000 个 Schema、452 条路径**；无新增模型、表或调度器。
- 桌面离开控制器默认留下，绑定准确窗口、来源和文档代次；可准确捕获的意图采用单次重放，未知意图采用官方同步确认分支。原生产主入口经窗口实际关闭后才进入既有 Runner／服务收尾。留下、离开和任务停止分别记录。

## 验证结果

| 检查 | 结果与实际范围 |
|---|---|
| 不同顶层定向检查 | **22 项终态通过：14 新增、8 直接受影响旧检查**。Root 5、Pool Web 7、Desktop 7、后端与生成合同 3；重复复验不重复计数。 |
| 精确后端读取 | 真实 SQLite／原服务与 HTTP handler；51 个正式登记的测试节点覆盖页外准确定位、当前降权／退出、跨 Pool、缺失对象、闭合 query 和无凭据响应。 |
| Web 类型 | 完整检查首次 exit 2，仅新增 fixture 的两处 `pins` 赋值诊断；修正后该文件 Serena 语义诊断 0。晚到来源修复的两个文件语义诊断也为 0。**未重跑完整 TSC，不记录全局类型 exit 0。** |
| 国际化 | 中英文 **9796 对词条**的语言、占位符、复数和标记检查通过，生产源码硬编码检查为 0。 |
| Web 代表旅程 | 1 次隔离构建，限定 3 组旅程通过；生产 Workspace／准备／连接组件使用合成公开 HTTP 和小型测试路由。中文 1366、英文 390 深色；复制到测试剪贴板的命令未执行。 |
| 桌面实际窗口 | 修正测试驱动后，单个真实 BrowserWindow 的 5 组检查通过；7 份本地化确认选项、8 次真实 beforeunload 事件。对话回应由 facade 模拟；实际导航及英文重载使用官方同步分支。最终 native exit 0、无强制退出、端口关闭。 |

Web 继续旅程记录 52 次 API 请求：51 GET、1 次原只读 `POST /tool-profiles/matches`；两个原生尝试共 69 次 API 请求，业务写入为 0。桌面窗口旅程只有 9 次静态 GET、0 API 请求。主集成已查看 Web 3 张、Desktop 2 张原图；桌面图片展示测试记录窗口，不是系统原生确认按钮点击的证据。

## 初始失败与修复记录

1. Root 首次 Vitest worker 配置启动失败，执行 0 个用例；随后两项测试的诊断入口／模拟值断言失败，仅修 fixture 后复验这两项。两者均不作为产品行为 RED。
2. Desktop 生产退出顺序检查保留真实 RED，修正为窗口实际关闭后才进入原收尾。初次定向批次有两项原 OAuth fixture 无法解析既有模块，修 fixture 后仅续验这两项；取消后旧意图退休也完成受影响检查。
3. Pool 表单到期卸载会丢失私有输入的问题由源码审阅确认；修正后新增一项草稿保留／禁用／明确重读检查通过，没有声称运行 RED。
4. 全局类型首次两处 fixture 诊断及后续局部语义 0 均保留，不用局部结果替代完整检查收据。
5. Web 实际验证先遇已安装 Playwright 访问 EPERM（原生启动 0）；首个原生尝试已到准确恢复卡／准备入口，旧共用 Modal 的 44px 过宽断言失败。保留该正向入口，使用同一构建仅继续剩余旅程。新卡和准备控件检查 44px；原共用 Modal 关闭按钮 36px、语言控件 42px 单独保留，未声称全应用触达均达标。
6. Desktop 首次实际验证被 Playwright 默认处理瞬时 CDP 对话引发的未处理拒绝中断，完成组数 0、native 退出码未知。增加仅观察的公开 `page.on('dialog')` 监听后，生产控制器独占决策，5 组必要旅程通过；未使用 Page accept／dismiss 或宿主放行绕过。
7. GUI 后独立审阅确认共享来源读失败时，晚到目录仍可沿旧来源打开 Quick。原同一顶层检查扩展后真实 RED→GREEN；修复加入中止与准确票据核验。**图像／构建保留修复前输入，新增分支由定向检查覆盖，未重建或重跑 GUI。**

## 证据与来源

最终源码、收据、逐项名称及图像 SHA256 进入 [summary](team-workstation-recovery-summary.json)。原始日志和图像保留在本机忽略目录 `.cache/team-workstation-recovery-218/`、`.cache/desktop-departure-218/`，不提交缓存、数据库或凭据。

| 证据组 | 最终索引 |
|---|---|
| Root 工具／路由及晚到结果修复 | `root-source-manifest-final.json`、`root-source-overlay-final.json`、`checks/late-source-{red,green}/result.json` |
| Pool 工作台及草稿保留 | `web/source-manifest-final.json`、`web/summary-final.json` |
| 原生控制器与生产 main 边界检查 | `.cache/desktop-departure-218/source-manifest.json`、`summary.json` |
| 后端与生成合同 | `backend/source-manifest.json`、`backend/summary.json` |
| Web／Desktop 限定实际结果 | `web-actual/summary.json`、`desktop-actual/summary.json` |

Multica 官方参考固定至 [`a7a61f15f148ba8c901fe24a36a61d6d73e247d6`](https://github.com/multica-ai/multica/commit/a7a61f15f148ba8c901fe24a36a61d6d73e247d6)，精确对象已获取，本地 HEAD `e8b6a976…` 与工作树状态保留。采用退出负责人、先停领取再排空、未知停止事实保留等结构；本阶段未复制其源码或界面。[许可证](https://github.com/multica-ai/multica/blob/a7a61f15f148ba8c901fe24a36a61d6d73e247d6/LICENSE)仍含附加商业、品牌和署名条件。

## 验收边界

本阶段证明准确共享节点恢复交接和有限桌面离开行为。Web 使用合成后端，桌面对话使用模拟回应；生产 main 的先后顺序由独立边界测试覆盖，真实 Worker 排空、系统原生按钮、完整安装版桌面和全 App 未在此实际旅程验收。正式开发、同提交独立测试、人工验收和发布继续引用各自证据。

原完整目标 **31 项／P0–P7，58 limited／34 pending／92 基线继续 active**。物理多机、真实厂商／模型、其他数据库、跨平台安装、完整设备主题矩阵、容量及生产交付继续推进。提交和推送由主集成完成，本报告不提前记录尚未发生的 Git 结果。
