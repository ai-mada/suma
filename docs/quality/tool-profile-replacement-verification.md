# 工具配置换新限定验证

T-TOOL-REPLACEMENT-200；suma 1.0.0；2026-11-02；状态：本纵向功能完成限定验证。完整31项／P0–P7仍进行中，58 limited／34 pending／92项基线不因此改写。

一次明确操作已贯通配置对比、准确目标选择、创建或复用、可选暂停旧项、原回执恢复及双向历史。新建保持paused/v1，复用保持原状态；没有自动启用、迁移既有运行或终止进程。接口、CLI、Web沿原权限、事务和唯一写恢复入口。

| 验证 | 已观察结果 |
| --- | --- |
| 定向检查 | 21个不同顶层：Go9／Web12，其中12新／9直接受影响旧；Go另10子场景 |
| 合同与构建 | 生成和一致性检查各2次，第二次由版本上界修复触发；全Web类型1次、隔离构建初次及必要CSS修复后各1次通过 |
| 中英文 | 9344成对键，生产硬编码违规0；新增错误、帮助、确认、历史与回执均纳入统一目录 |
| 实际控制面 | 自有隔离SQLite＋真实TCP HTTP22请求、3条关系，触发器故障证明整笔回滚；相同原请求在库存过期后恢复原回执 |
| 实际接口边界 | 当前浏览器批准权优先，worker凭据不能恢复浏览器批准；同key改body拒绝，分页无重复或缺项，原Profile摘要保留 |
| 代表界面 | 中文1366浅色、英文390深色两组通过；原组件＋合成领域API，最终无页面错误和意外请求，两图已人工查看 |
| 独立审查 | 控制面、非自写消费者／合同、Web与最终证据分工审查，无遗留确认P1/P2 |

中文组验证明确跨节点新建、选择暂停、paused/v1快照、另读当前v2再进入启用审核和分页历史。英文组验证明确复用enabled/v6、提交成功但回执丢失、刷新后保留原body/key、第二次逐字相同请求恢复单一效果，未自动启用。

首次界面脚本因已消失的beforeunload对话重复应答失败；中文成功证据保留。英文续验又因原页面离开保护而超时，保留失败记录及已观察native exit0。最终脚本只对自有测试窗口作一次明确“离开”决定，使用[Electron官方will-prevent-unload事件](https://www.electronjs.org/docs/latest/api/web-contents#event-will-prevent-unload)，没有移除产品保护或清理原操作。最终native exit0；初次native退出码未记录，随后PID不存在不冒充exit0。前三次启动对应两组，不将续验算新功能覆盖，也未因驱动修复重建产品。随后人工看图发现Quick通用输入规则覆盖复选框宽度；独立CSS审查确认后提高组件选择器特异性并固定18px flex。仅为此补做一次必要构建与两组受影响排版，最终checkbox均18×18px。英文另存完整视口，文本行矩形均在组件范围内，排除原长元素截图裁切疑点。总启动4次，最后native exit0。

本轮领域与合同审查修复两项实际问题，另修复上述CSS排版问题：关系先提交后超过后续2MiB读取上限，以及新Schema的版本上限大于原INT32领域。前者有真实失败复现并验证同raw提交前校验与回滚；后者来自源码审查并做原定向边界跟进，不伪称执行过RED。页预算计量保持逐项线性，不反复序列化整页。

关系最多2MiB，列表items预算8MiB；超限目前明确409并全部回滚。真实数据库矩阵、厂商登录／模型／费用、物理多机、完整登录App／设备主题矩阵、规模、跨平台安装与生产验收仍待各自证明。本阶段实际接口使用自有库存报告，不声称真实工具探测；native工具、模型调用和业务运行均0。

参见[机器摘要](tool-profile-replacement-summary.json)、[需求](../product/tool-profile-replacement.md)、[架构与AI SOP](../architecture/tool-profile-replacement.md)、[数据一致性](../data/tool-profile-replacement.md)、[交互](../design/tool-profile-replacement.md)、[English](../en/tool-profile-replacement.md)、[合同](../../contracts/tool-profile-replacement.md)及[后续计划](../delivery/tool-management-next.md)。精确命令、源哈希、原失败、DB、截图及日志在仓库忽略的.cache/tool-replacement-200保留；缓存、截图、数据库、日志、依赖、凭据和两份无关历史审计不入Git。
