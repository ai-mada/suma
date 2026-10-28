# 派发推荐接口与执行合同

状态：当前1.0.0，T-ROUTING-122。实现：`internal/platform/dispatch_recommendation*`、`internal/dispatchrecommendationcontract`、`internal/toolcontract/dispatch_recommendation.go`、Runner、原生SDK及Web。公开Schema从Go统一生成，见 `docs/api/openapi.json`；证据见质量报告。

| 操作 | 合同 |
|---|---|
| POST `/dispatch-recommendations/options` | 浏览器项目只读；固定任务／流程／知识／约束；不预留、不探测、不调用模型 |
| POST `/dispatch-recommendations` | 当前浏览器项目写；来源摘要、分析配置版本、ModelSelection、明确同意、原操作ID；只建立分析运行 |
| GET `/dispatch-recommendations` | 当前项目API或浏览器读；task、limit1–100、before；先核验每条历史上下文的当前读权限 |
| GET `/dispatch-recommendations/{id}` | 当前项目读；历史、当前依据、允许动作、原输出和真实模型绑定 |
| POST `/dispatch-recommendations/{id}/adopt` | 当前浏览器项目写、版本CAS、来源摘要、五字段选择和理由；事务内保存原派发计划及受保护来源 |

固定协议 `suma-dispatch-recommendation-v1`，提案包含schema_version=1、summary、rationale、1–3项recommendations、最多5项questions。每个选择严格只有candidate_id、max_steps、max_output_tokens、reasoning_effort、reason；三个可空参数必须显式null，拒绝重复键、别名和未知字段。原生类型使用数字步骤／输出和null effort；Codex使用明确effort及null数字；固定工具全部参数null。

选择摘要以Go `encoding/json` 对固定五字段的编码计算SHA256，包含null与HTML/U+2028/U+2029转义，浏览器使用相同专用编码。服务端重新计算并核对当前候选，不信任浏览器摘要。公开普通计划接口不接受推荐来源证明。

分析配置必须是当前启用的只读Suma JSONL配置，独占推荐能力，声明统一ModelExecutionProtocol，不具备workspace capability。参数固定max_steps=1、max_output_tokens≤8192；推荐输入≤64000 UTF-8字节，传输≤512KiB，结果≤32768字节。Provider地址和密钥仍由Go控制，SDK只收到原attempt的受认证loopback网关与不可变绑定。

执行领取、心跳和结果在原Runner／Pool事务重核。创建者原凭据的source、family、expiry与当前项目权限必须有效；限定项目证明复用已持有的治理锁，不在回调中扩大组织锁集。旧worker/fence或错误绑定不能回写。有效原结果即使来源过期仍保留实际用量和失败事实；未知结果进入核对，不能自动重试第二次分析。

采用的来源、输出和选择摘要受保护；启动时原TaskDispatchPlan再次核对。只读状态不构成人工验收或部署授权。Native、Codex和固定配置的预算语义不同；平台报价是配置估算，不能标作供应商账单。
