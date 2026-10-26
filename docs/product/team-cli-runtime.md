# 团队多工具执行接入

状态：T-P4-021 功能本地已实现，协议、原生进程和Node入口定向验证通过；Node实际任务、最终中英界面及原预览前向更新均已本地验证。Owner：执行节点、工具治理与工作台。范围：[执行合同](../../contracts/team-cli-runtime.md)。

## 用户目标与交互

团队管理员把不同机器上的开发工具接入同一项目或共享执行池；任务负责人选择已启用且有当前真实库存的工具，预览固定任务／需求／流程／工具版本后派发。开发实际在私有Git工作树内执行，进度和结果复用持久Journal回写；固定候选仍交独立测试和明确人工评审。

工具设置新增默认折叠的“接入开发工具”：选择Codex、CodeBuddy Code或Harness，复制无秘密本地模板，按四步配置、探测、导入、启用。仅CodeBuddy和Harness可选择原生可执行文件或Node入口；Node模式填写实际绝对JavaScript入口，空值或相对路径时不能复制。保留主列表和当前详情，中英文切换不重置选择，窄屏采用自然换行及44px操作入口。模板不等于已注册／可用／已执行；探测或注册不构成任务完成。

## 本阶段可用边界

| 工具 | 实现 | 当前证据 |
| --- | --- | --- |
| Codex | 复用现有固定exec JSONL及workspace-write | 原已验证合同继续有效 |
| DeepSeek Harness/dsh | 原生或固定Node入口、Multica profile、v1 probe、双向execute/cancel、请求身份与唯一整体result | 原生进程夹具、隔离Git、停止回执与Node入口探测／摘要；无厂商模型 |
| CodeBuddy Code | 原生或固定Node入口、headless stream-json、会话／整体result／用量，等待授权与后台任务阻断 | 原生进程夹具、独立decoder与Node入口探测／摘要；无厂商模型 |
| WorkBuddy | 独立产品；官方连接器支持MCP/CLI等扩展，平台既有入站MCP为可复用入口 | 未进行WorkBuddy客户端真实接入，不将CodeBuddy冒充WorkBuddy |

厂商CLI未安装或协议探测失败时不能标可用。Windows的npm`.cmd`包装器不通过shell执行；管理员可改用Node直调实际绝对`.js`／`.mjs`／`.cjs`入口，原生可执行安装继续可用。配置绑定Node本体、入口内容和完整命令配置，不声明已覆盖入口加载的全部模块。DSH仍需事先安装兼容的Multica runtime profile，Suma不会自动下载或改写其配置。CodeBuddy保留本机权限策略；拒绝授权、等待输入或未完成后台任务不能转换为成功。

## 验收与后续

必须区分：声明／探测／实际执行／候选／正式测试／任务评审。新适配器不能消费平台Runner凭证，不能从任务覆盖argv/cwd/profile/会话；缺少终态、重复终态、错误身份及无结果退出均不能按exit0成功。配置、机器／池授权、固定版本与质量门禁继续在服务端执行。

团队步骤交接、专家组策略、受管仓库三种合并、WorkBuddy服务接入、模型辅助的知识／记忆／技能萃取与发布同步、SSO/A2A及规模验收属于完整目标，继续实施。上述新功能不替代这些要求。
