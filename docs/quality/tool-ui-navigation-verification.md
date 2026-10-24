# 需求下钻：真实 ReactDOM 消费者导航补充验证

状态：T-P4-004 导航源码、真实挂载消费者、类型与格式检查通过；真实浏览器结果由主集成代理独立记录。日期：2026-10-24。负责人：UI 实现代理。前代：[窄屏补充](tool-ui-responsive-supplement.md)、[工具工作台验证](tool-ui-verification.md)。[证据索引](evidence/tool-ui-navigation/README.md)。

## 现场反馈与实现范围

主集成在实际任务详情中确认：关联需求链接的 href、项目和需求 ID 正确，没有 inert 祖先或打开的对话框，但普通点击后原任务 URL／详情没有推进。仅断言 HTML 中存在 href 的旧消费者测试不足以证明这条交互可用。

原链接只依赖原生整页激活，没有向当前受控 App 路由发出显式状态转换。此次没有将不确定的宿主／原生导航行为归因为已证实的内部竞争；实现改为与既有工作区按钮一致的路径：

1. 普通主键点击由 [RecordRequirementReference](../../apps/web/src/RecordLineage.tsx)交给 [App](../../apps/web/src/App.tsx) 的内部跳转回调。
2. App 先派发现有可取消 `suma:navigate` 离页保护事件，通过后切换当前项目内的需求视图与精确 ID，清除任务运行／面板上下文。
3. 原 href 保留；Ctrl／Meta／Shift／Alt 等修饰键与非主键点击保持原生行为。没有改成无链接的伪按钮，也没有扩大项目访问权限。
4. URL 仍由既有 route/history effect 写入，返回仍由真实 `history.back`／`popstate` 消费，需求无需位于已加载的 100 条记录中。

没有修改 Go、共享合同、运行状态、实际任务数据或原有 189／194 验收材料。

## 可重复的真实消费者测试

新增 [record-navigation-dom.test.tsx](../../apps/web/src/record-navigation-dom.test.tsx)，使用固定 `jsdom@26.1.0` 作为 web **测试开发依赖**。`apps/web/package.json` 和 `pnpm-lock.yaml` 已记录，生产依赖与生产入口没有增加 JSDOM 导入。可运行：

```text
pnpm install --frozen-lockfile
pnpm --filter @suma/web exec vitest run src/record-navigation-dom.test.tsx
```

测试使用 `react-dom/client.createRoot` 真正挂载 App，并通过原生 `MouseEvent` 进入 React 委托事件系统。记录列表和会话 HTTP 使用明确的合成夹具，其中需求列表为空，精确需求由详情接口返回。

测试在 document 阶段观察消费者是否已处理点击，然后抑制 JSDOM 不支持的整页原生活动。这样 URL／详情变化必须由真实 App 状态与 history effect 完成，不能依赖 href 字符串或测试环境虚构原生导航成功。该方法不代替真实 Chrome／IAB 的点击验收。

三项回归分别验证：

- 点击任务详情链接后，精确需求详情及 URL 出现；调用实际 `history.back()` 后，`popstate` 恢复原任务详情和 ID。
- 现有离页保护可以阻止跳转，普通链接不能绕过未确认操作的保护。
- 实际组件普通点击调用精确需求回调，Ctrl／Meta 保留原生链接决定，不调用内部跳转。

## 实际结果与原始记录

| 检查 | 结果 | 原件 |
|---|---|---|
| 新真实 DOM 消费者 RED | 3 项失败，原链接没有内部消费者处理 | [RED](evidence/tool-ui-navigation/ui-requirement-dom-red.txt) |
| 新真实 DOM 消费者 GREEN | 3 项通过，含完整 App 与 history 返回 | [GREEN](evidence/tool-ui-navigation/ui-requirement-dom-green.txt) |
| 最终 web 回归 | 197 项通过、19 个文件，退出 0 | [LF 最终原文](evidence/tool-ui-navigation/ui-web-navigation-lf-final.txt) |
| 最终 web 类型 | 退出 0 | [类型原文](evidence/tool-ui-navigation/ui-navigation-lf-typecheck-final.txt)、[最终收据](evidence/tool-ui-navigation/ui-navigation-dom-verification.json) |
| 新 DOM 文件格式 | Prettier 3.6.2 `--check`，退出 0 | [格式](evidence/tool-ui-navigation/ui-navigation-lf-format-check.txt) |
| 标准空白检查 | `git diff --check -- apps/web pnpm-lock.yaml`，退出 0；未使用 `cr-at-eol` 豁免 | [原始空输出](evidence/tool-ui-navigation/ui-navigation-lf-whitespace-check.txt) |

初次全套检查中，DOM 测试夹具将接口验收数组直接传给视图类型，出现一处 TypeScript 不匹配；使用实际 `normalizeResponse` 后通过。保留[原类型失败](evidence/tool-ui-navigation/ui-navigation-dom-typecheck-final.txt)、[当时 web197](evidence/tool-ui-navigation/ui-web-navigation-dom-final.txt)和[修正通过](evidence/tool-ui-navigation/ui-navigation-dom-typecheck-green.txt)，它们不是最终 LF 切面的替代物。

194 加本次 3 项为 web197；相对 web164，本阶段新增 33 项前端测试。197 不是全平台测试数量，也不是 197 条真实浏览器旅程。

## 最终源码、依赖与行尾切面

本次最终检查前后逐文件摘要一致，包含 22 份 UI 源码／测试和 2 份依赖输入。App.tsx、Workflow.tsx、api.ts 只将已有 CRLF 转为 LF，保留原紧凑格式；RecordLineage 和新 DOM 测试原本是 LF。行尾前后摘要及数量见[归一化记录](evidence/tool-ui-navigation/ui-navigation-lf-normalization.json)。

[最终检查前摘要](evidence/tool-ui-navigation/ui-navigation-source-final-before.json)与[最终验证收据](evidence/tool-ui-navigation/ui-navigation-dom-verification.json)明确绑定当前 24 份输入；更早的[初轮摘要](evidence/tool-ui-navigation/ui-navigation-dom-source-before.json)按原字节保留。没有回写旧源码快照或重命名原失败。[依赖安装原文](evidence/tool-ui-navigation/ui-navigation-dom-install.txt)保留网络重试；最终安装退出 0，JSDOM 版本精确固定为 26.1.0。

## 窄屏截图诊断更正

主集成后续确认，修复前 390 PNG 存在宿主 1.5 倍 backing／捕获截幅因素，不能单凭该图断言真实 DOM 的工具栏裁切。已经确认的产品缺口是大画布作为窄屏默认及缺少完整节点列表。相应措辞已在窄屏补充报告更正；[前版原文](evidence/tool-ui-navigation/tool-ui-responsive-supplement.original.md)、[来源证明](evidence/tool-ui-navigation/tool-ui-responsive-supplement.original.md.provenance.json)及[更正收据](evidence/tool-ui-navigation/diagnostic-correction.json)分别保留。旧截图与旧归档未更改。

本报告没有接管浏览器、重新构建网页或执行 commit。主集成需在最终构建上独立验证普通点击、URL／需求详情、返回原任务，并单独记录真实浏览器、视口、焦点、运行与验收证据。这里没有用 JSDOM 代替真实后端、浏览器布局或生产验收。
