# 验收责任请求与原质量闭环限定验证

阶段：T-REVIEW-142；2026-10-30；初始产品1.0.0；负责人：质量协作／主集成。状态：代码、定向检查与实际本机链路通过，最终GUI待验。完整P0–P7、需求1–26及58／34／92总账保持在途。

## 交付与权威

明确当前规范审核人员、准确task/run绑定、请求与通知原子提交、有界历史、取消及原回执、原Review关联实际决定、原退回问题回流；含专用CLI和原任务详情面板。

[需求](../product/task-review-requests.md) · [合同](../../contracts/task-review-requests.md) · [架构](../architecture/task-review-requests.md) · [交互与AI SOP](../design/task-review-requests.md) · [English](../en/task-review-requests.md)。初始任务验收只接受accepted／rejected，移除approved输入别名；真实工具／运行／部署审批词汇保留。测试计划界面修复遗漏developer／reviewer的可写角色，发布／退役仍使用服务端allowed_actions。规范人员SQL提取为共享私有解析器；讨论READ语义保留，审核目录另要求当前approve角色。

请求不授予或收窄验收权限，不表示测试通过。binding_current仅说明直接任务/run版本、状态和内容绑定；正式Review仍独立核验完整质量依据。同事务关联准确Review与实际决定人，旧Review重放不关闭新请求。读取、私人已读、归档和取消均不推进DAG或任务验收。

## 已执行证据

- 93项独立Go顶层用例通过：领域／CLI／当前权限／Inbox／原质量及完整合同生成包。最后canonical决定输入的受影响复验不重复累加；命令receipt及逐项名称在[紧凑摘要](task-review-requests-summary.json)。每批60或90秒上限，未重复全项目Go套件。
- 58项独立Web用例、8个文件。首轮57通过／1个新测试失败；定位为GET不发送写入专属身份头的夹具假设错误，仅修测试后重跑该文件9项，最终独立58项通过。没有修改生产scope保护来迎合夹具。
- 完整Web类型检查、中英7292对键及0生产硬编码、合同生成／check、隔离Web构建41.76秒通过。构建保留既有大分块提示，没有性能达标声明。
- 12项实际本机HTTP／编译CLI／编译Runner检查通过：三账号、真实Node非代码断言、明确请求／代审／取消／退回／回流。非代码测试不认证代码候选，也不替代前阶段同提交独立代码测试的证据。

## 实际链路与异常核对

新建44142自有隔离实例、owner／reviewer／viewer账号。当前目录包含真实reviewer、排除viewer；CLI发送固定交付请求及重放原回执，接收人个人收件箱得到准确来源。另一合法owner通过原Review接受，closed请求显示真实owner；创建新请求后恢复旧Review不关闭新请求；明确取消及原取消回执恢复不改变已验收任务。

第二项真实测试后，指定reviewer通过原Review退回：准确请求closed、任务blocked、原正式问题唯一创建。退回重放不重复问题；取消／决定后的历史个人通知保留。最终控制面还实际拒绝approved别名（400），任务和请求历史未变化。

初次实际Runner因测试工作目录未创建而没有宣告可用能力。先读取库存、空journal、准确run/node/事件，确认test节点ready、attempt/fence=0，只有run_created/node_ready，未领取／执行。原Runner已终结后建立原任务目录，以新登记的自有Runner恢复同一未领取run，没有再创建第一项run、伪造结果或重置数据库。原worker的一次性凭据未持久化，因此新恢复worker使用新凭据及独立journal；旧凭据不从数据库挖取，也不回显。真实断言随后通过。这是夹具配置修正，不称为执行失联／未知副作用恢复已全面验收。

## 设计、缺口与提交

默认面板在原任务详情交付门禁处，人员选择和说明按需展开；个人通知打开原请求，历史定位不依赖全部分页。中英、范围隔离、未知写入恢复、晚旧响应和角色操作已做定向组件验证。

浏览器访问已由用户恢复到同一43959来源。本次整合已观察28项中的26项通过；测试计划角色与PUT问题已通过代码、真实HTTP及组件修复。最终双角色保存及Review草稿保护GUI复验仍在等待工具审批，不记通过。实现阶段按审阅白名单提交dev，缓存、凭据、数据库、构建、截图及其他桌面开发不提交。详见[整合报告](collaboration-integration-verification.md)与[摘要](collaboration-integration-summary.json)。

没有外部厂商、模型、代码候选实测、其他数据库、物理多机、规模、安装或生产验收。通知偏好、IM／邮件outbox、多项目聚合、完整智能审核与全平台继续待办。
