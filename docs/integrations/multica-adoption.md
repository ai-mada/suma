# Multica参考、复用与当前交付

状态：T-MULTICA-136；2026-10-29；负责人：主集成／协作领域。依据用户授权，以当前Suma初始1.0.0架构和完整P0–P7／需求1–26为准；[主规格](../suma-architecture-design.html#s21)、[合同](../../contracts/my-work.md)与[交互架构](../design/multica-collaboration.md)相互关联。

## 锁定源码与采用方式

本地`D:\codes\github\multica`工作树读取时干净；当前SHA及通过GitHub SSH443只读核对的main均为[139b13a462c9a9757e18a17578a867eecd99851e](https://github.com/multica-ai/multica/tree/139b13a462c9a9757e18a17578a867eecd99851e)。没有重置、修改或搬运参考工作树。HTTPS直连及首次SSH主机别名失败均保留在本地证据，随后沿已有HostKeyAlias配置完成核对；未关闭主机校验。

此提交的[LICENSE](https://github.com/multica-ai/multica/blob/139b13a462c9a9757e18a17578a867eecd99851e/LICENSE)由Apache文本及附加条件共同组成；附加条件涉及对外托管／商业嵌入、界面品牌与非界面来源说明。[NOTICE](https://github.com/multica-ai/multica/blob/139b13a462c9a9757e18a17578a867eecd99851e/NOTICE)亦明确同一范围。不能仅凭仓库公开将其当普通Apache授权复制进Suma SaaS产品。

本阶段未复制Multica代码、图标、品牌、CSS或界面结构；从其协作行为和模块边界形成Suma自身合同，独立实现。直接复用现有Suma的React、Radix Dialog、i18next、Lucide、SQL方言、当前身份事务、任务详情、sourceDestination、导航目录与操作保护，没有增加cmdk或其他依赖。若后续有明确授权允许直接采用某模块，仍需逐项保存源提交、完整许可／NOTICE、修改范围和适用部署约束，再评估进入主产品；不预设全部商业权利。

## 能力对应与优先顺序

| 能力 | 已核对的Multica源码 | Suma采用及范围 |
|---|---|---|
| 上下文命令入口 | `packages/views/search/search-command.tsx`、`use-root-model.ts`、`packages/core/command-bar/context-store.ts`、`recent-store.ts` | 本阶段CommandPalette：页面与当前项目查询、键盘选择、静态内存最近入口、原导航保护；行为借鉴、独立实现 |
| 人与Agent的交付责任 | 任务／Issue负责人和执行回交的协作模型 | 本阶段MyWork按当前规范人员资源查询；已有任务讨论、DAG、候选、独立测试、验收继续复用；不混淆用户ID与资源ID |
| 完整任务过滤和分组看板 | `server/internal/handler/issue_table_query.go`、`issue_table_group.go`；`packages/views/issues/components/board-view.tsx` | 已有项目任务列表／看板和本阶段精确个人过滤；全项目服务端分组、列分页和共享视图仍待交付 |
| 个人通知与收件箱 | `server/internal/handler/inbox.go`；`packages/views/inbox/components/inbox-page.tsx` | 正式已读／归档、接收人可查询投影和通知outbox消费仍待交付；当前个人任务不是通知系统 |
| 本地／远程runtime、工具派发 | 本地daemon、CLI执行、团队运行及工具探测 | 复用现有Suma Runner／PoolWorker／Journal、统一Go DAG、工具profile与当前授权；已有证据按各适配器列示，不能称所有工具真实接通 |
| 技能与专家团队 | squads、skills及任务协作 | 复用Suma版本化知识／技能、专家团队计划和固定执行合同；后续推进完整知识学习与分发，原58／34／92不变 |

Cursor／Claude／Qoder／Codex／WorkBuddy／Zed／OpenCode／Harness各自的真实适配、版本、健康、执行及取消能力仍由[工具矩阵](development-tool-matrix.md)管理；MCP配置导出不等于厂商CLI已执行。本阶段没有调用真实模型或供应商，也没有引入旁路批准、自动合并或第二套预算账本。

## 继续交付

按当前唯一实施计划，下一步优先补齐项目任务的服务端过滤／分组分页，之后建设当前接收人可靠投影及个人通知生命周期。通知、团队路由、工具接入和智能自动化都须回到当前权限、固定版本、唯一调度、预算与停止回执，而不是以工作台视觉状态替代正式事实。
