# 需求导航：完整点击区域补充验证

状态：T-P4-004 点击盒实现、消费者回归、类型及格式检查通过。日期：2026-10-24。负责人：UI 实现代理。本报告接续 [197 项导航验证](tool-ui-navigation-verification.md)，旧报告及其原始证据保留不变。[本次证据](evidence/tool-ui-hitbox/README.md)。

## 已确认的问题及本次范围

主集成在 CGLjC2wJ 实际构建中确认：Enter 可以打开精确关联需求，浏览器返回可以恢复原任务，但居中鼠标点击没有激活链接。随后读取真实 DOM 几何：原 `display:inline` 链接分为两行片段，整体包围盒约为 `183.7 × 31 CSS px`，两片段宽约 `60` 和 `183.7 CSS px`；包围盒中心落在 `<dd>` 的空白中，`centerInsideLink=false`。这是本轮已查明的多行 inline 点击区域问题，不能再将鼠标落空归因为未经证实的原生导航或 history 竞争。

仅将 [RecordRequirementReference](../../apps/web/src/RecordLineage.tsx) 改为一个完整的 `inline-flex` 可点击盒：文字与图标在同一组，标识符单独换行，宽度限制在父容器以内，长 ID 可以任意断行。整个盒及内部留白均由原生 `<a>` 承担点击。保留原 href、App 跳转回调、离页保护、修饰键及非主键的行为；没有修改其他来源链接、Go 或共享合同。

本次相对 197 项切面只有两份 UI 文件变化：上述组件和 [真实 DOM 消费者测试](../../apps/web/src/record-navigation-dom.test.tsx)。App、Workflow、api 和前一轮固定任务／工具协议实现保持原摘要。

## RED → GREEN 与最终检查

新增一项真实挂载组件回归，检查单一 flex 链接、两个直接子元素、标签／图标分组、完整 ID 以及宽度与换行约束。JSDOM 没有布局引擎，测试不替换 `getBoundingClientRect`、不伪造命中坐标，也不声称验证真实鼠标几何。

| 检查 | 实际结果 | 原始证据 |
|---|---|---|
| 新结构回归 RED | 新增 1 项失败，原导航 3 项通过 | [RED](evidence/tool-ui-hitbox/ui-requirement-hitbox-red.txt) |
| 目标 GREEN | 4 项通过，含 App 导航、返回与离页保护 | [GREEN](evidence/tool-ui-hitbox/ui-requirement-hitbox-green.txt) |
| 全 web 回归 | 198 项、19 个文件通过，退出 0 | [全套原文](evidence/tool-ui-hitbox/ui-web-hitbox-final.txt) |
| web 类型检查 | 退出 0 | [类型原文](evidence/tool-ui-hitbox/ui-hitbox-typecheck-final.txt) |
| 新 DOM 测试格式 | Prettier 3.6.2 检查通过 | [格式原文](evidence/tool-ui-hitbox/ui-hitbox-format-check.txt) |
| 标准空白检查 | 退出 0，未使用行尾豁免 | [空输出原件](evidence/tool-ui-hitbox/ui-hitbox-whitespace-check.txt) |
| 文档与 API 引用 | 真实运行 `pnpm check:docs`，退出 0 | [检查原文](evidence/tool-ui-hitbox/ui-hitbox-docs-check.txt) |

197 加本次 1 项得到 198；相对 164 项基线，本阶段新增 34 项前端测试。198 是自动化前端用例数量，并非真实浏览器旅程数量。最终检查前后 22 份 UI 源码／测试和 2 份依赖输入摘要一致，见[源码切面](evidence/tool-ui-hitbox/ui-hitbox-source-before.json)与[验证收据](evidence/tool-ui-hitbox/ui-hitbox-verification.json)。App、Workflow、api、RecordLineage、DOM 测试均保留 LF。

## 导航补充文件及文档检查器

本阶段导航交付也包含共享脚本 [scripts/check-docs.mjs](../../scripts/check-docs.mjs) 的 `.original.md` 来源验证，不能将阶段改动描述为仅有 App／组件。归档 Markdown 按相邻 `.provenance.json` 提供的仓库相对 `source_path` 解析原有相对链接；检查器同时拒绝绝对／越界来源、校验 `snapshot_sha256` 并确认原源文件存在，普通文档继续按自身目录检查。[已有来源记录](evidence/tool-ui-navigation/tool-ui-responsive-supplement.original.md.provenance.json)及旧原文保留。

该共享脚本是前一轮文档原件归档的支持项，本轮点击盒没有再次改动它。本次[归档清单](evidence/tool-ui-hitbox/archive-map.json)另列其摘要，文档检查通过记录按真实输出保存。导航完整交付涉及组件与测试、App 路由接线、固定 JSDOM 测试依赖和锁文件、LF 归一化、文档来源检查及证据归档，不能用“只有两文件”概括整阶段；两文件仅指本轮相对 197 切面的增量。

源码已冻结。最终构建的中心命中、普通鼠标点击、Enter、返回原任务及多视口实际验证，由主集成单独记录。本报告不代言尚未回写的真实浏览器结果，也未执行 commit。
