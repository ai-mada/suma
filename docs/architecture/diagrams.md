# 架构与事务时序

状态：当前源码边界图，非完整目标平台的部署验收。日期：2026-10-23。Owner：架构与执行引擎负责人。

关联：[P0–P7 计划](../delivery/implementation-plan.md)、[能力矩阵](../product/capability-matrix.md)、[验证报告](../quality/verification.md)。图中的外部服务均需要配置；显示连接不表示真实账户或生产集群已验收。Yjs、LiveKit、S3、支付与通用办公仍在目标范围，未画成已运行组件。

## C4：系统上下文

```mermaid
C4Context
    title Suma 当前系统上下文
    Person(user, "项目成员与审批人", "浏览器、桌面、CLI/TUI")
    System(suma, "Suma", "正式业务记录、DAG、Runner 与本地工作区")
    System_Ext(model, "模型与 embedding 服务", "可配置协议；生产账户未验收")
    System_Ext(flow, "外部工作流服务", "管理员注册 HTTP 插件；本地模拟验证")
    System_Ext(agent, "外部 Agent CLI", "已实现版本/help 能力探测")
    Rel(user, suma, "项目授权、编辑、运行与人工验收", "HTTP / 本机文件")
    Rel(suma, model, "模型循环或向量生成", "HTTP")
    Rel(suma, flow, "启动、查询、取消", "HTTP")
    Rel(suma, agent, "显式配置后的探测", "进程")
```

依据：[平台入口](../../internal/platform/http.go)、[Runner 外部协议](../../internal/runner/external.go)、[Agent 目录](../../packages/agent-runtime/src/catalog.ts)。

## C4：容器与持久边界

```mermaid
C4Container
    title Suma 当前容器边界
    Person(user, "用户")
    System_Boundary(suma, "Suma") {
        Container(web, "Web 工作台", "React / Vite / Monaco / ReactFlow", "浏览器页面与状态轮询")
        Container(desktop, "桌面主进程", "Electron", "启动本机控制面、隔离 preload")
        Container(cli, "CLI / TUI", "Go / BubbleTea", "共享 HTTP 客户端")
        Container(control, "控制面", "Go HTTP / platform / dag", "身份、任务、事务调度、投影循环")
        Container(runner, "Runner", "Go", "独立启动；能力登记、进程与回执恢复")
        Container(agent, "原生 Agent 运行时", "TypeScript / Node", "有限模型和工具循环")
        Container(knowledge, "知识索引服务", "TypeScript / Node", "索引协议、模型 profile 与来源墓碑")
        ContainerDb(sql, "正式状态库", "SQLite；可配置 OceanBase / PostgreSQL", "业务、DAG、事件、回执和投影台账")
        ContainerDb(index, "派生索引", "本地 LanceDB；OceanBase 适配可选", "全文与向量，不是正式业务事实")
        ContainerDb(files, "本机文件", "允许的工作区 / Runner journal", "文本保存日志、执行恢复证据")
    }
    Rel(user, web, "访问")
    Rel(user, cli, "操作")
    Rel(user, desktop, "启动")
    Rel(desktop, control, "拉起与健康检查", "loopback")
    Rel(desktop, web, "加载打包页面")
    Rel(web, control, "授权 API / 轮询", "HTTP")
    Rel(cli, control, "授权 API", "HTTP")
    Rel(control, sql, "短事务、锁、版本、回执", "SQL")
    Rel(control, files, "摘要校验及持久保存")
    Rel(control, knowledge, "来源 outbox 与检索", "HTTP")
    Rel(knowledge, index, "派生写入与查询")
    Rel(runner, control, "claim / heartbeat / result", "HTTP")
    Rel(runner, agent, "受控执行", "进程协议")
    Rel(runner, files, "执行 journal / 回执")
```

SQLite 是实际本地验证环境。OceanBase/PostgreSQL 的驱动与适配路径存在，真实集群尚未验收。桌面不会自动启动 Runner 或知识服务；它们需要按[运行手册](../operations/runbook.md)配置。全局 SQL 调度锁是当前一致性策略和吞吐限制，不能据此声称已实现分片高容量调度。

## 任务绑定、执行与人工验收

```mermaid
sequenceDiagram
    actor U as 用户
    participant API as 平台 API
    participant DAG as DAG Engine
    participant DB as 正式 SQL
    participant R as Runner
    participant FX as 外部进程或服务
    U->>API: 启动任务（版本、工作流版本、operation_id）
    API->>DAG: CreateRunBound + BindRunTx
    DAG->>DB: BEGIN；锁与幂等请求摘要检查
    DAG->>DB: 固定工作流；校验任务/需求/前置验收依据
    DAG->>DB: 插入 run、节点、ready、任务绑定与回执
    DAG->>DB: COMMIT
    DAG-->>API: 同一操作重放返回原回执
    R->>API: claim（登记能力）
    API->>DAG: Claim
    DAG->>DB: BEGIN；候选选择；原子资源预留
    DAG->>DB: 创建 attempt、fence 与租约；COMMIT
    API-->>R: 固定 assignment
    R->>R: 持久化执行 journal
    R->>FX: 执行（SQL 事务之外）
    loop 执行期间
        R->>API: heartbeat（attempt / fence）
        API-->>R: 续租或 cancel_requested
    end
    FX-->>R: 退出事实、输出与效果证据
    R->>R: 持久结果，等待回执
    R->>API: complete（attempt / fence / stopped）
    API->>DAG: Complete
    DAG->>DB: BEGIN；拒绝过期 fence / 冲突结果
    DAG->>DB: 结果、事件、后继 ready 与可释放资源原子提交
    API-->>R: durable ACK；重复相同结果可重放
    U->>API: Review（任务版本、operation_id）
    API->>DB: 锁；检查当前 attempt / 测试证据 / 子流程链
    API->>DB: 复查所有依据版本及补偿状态；验收与回执同事务
    API-->>U: accepted 或冲突/缺少证据
```

未知结果进入 reconciling，不自动推断失败、停止或安全重试。只有停止事实成立才能释放对应执行资源。SQL 回执只能保证状态提交和重复命令处理；它不能让任意外部副作用自动获得 exactly-once 语义。

依据：[任务绑定](../../internal/platform/delivery.go)、[验收](../../internal/platform/evidence.go)、[调度](../../internal/dag/scheduler.go)、[Runner journal](../../internal/runner/journal.go)。验证：[安全回归](../../internal/platform/security_evidence_test.go)、[引擎回归](../../internal/dag/engine_test.go)。

## 子流程与显式补偿

```mermaid
sequenceDiagram
    participant P as 父运行 / Engine
    participant DB as SQL
    participant C as 固定版本子运行
    actor U as 有权限的用户
    P->>DB: 事务创建父节点 attempt 与唯一 childrun 关联
    P->>C: 物化固定 workflow_id / version
    Note over P,C: 父节点等待不占 Worker 槽；关联可在重启后恢复
    C->>DB: 正常执行并持久化终态与证据
    P->>DB: Tick / 后续推进读取 child 终态
    P->>DB: 更新父节点与后继 ready
    U->>P: 暂停或取消父运行
    P->>C: 持久传播控制请求
    Note over P,C: 请求不等于停止；子运行实际停止证据仍需核对
    U->>P: compensate（稳定版本、operation_id）
    P->>DB: BEGIN；固定成功 attempt 的显式 effect 引用
    P->>DB: 创建内部补偿工作流及逆依赖独立运行
    P->>DB: hook 原子撤销关联任务与后继验收依据
    P->>DB: 保存关联与回执；COMMIT
    Note over P,DB: 原运行历史不改写为已撤销；补偿结果独立记录
```

补偿只执行显式配置的受控动作。未配置的副作用没有自动撤销能力；内部补偿图禁止公共启动、覆盖发布或作为普通子流程引用。依据：[子流程](../../internal/dag/subworkflows.go)、[补偿](../../internal/dag/compensation.go)、[事务 hook](../../internal/dag/commands.go)；验证：[子流程测试](../../internal/dag/subworkflow_test.go)、[补偿测试](../../internal/dag/compensation_test.go)。

## 来源、索引与删除墓碑

```mermaid
sequenceDiagram
    actor U as 用户
    participant API as 平台
    participant DB as 正式 SQL
    participant P as 索引投影器
    participant K as KnowledgeIndex
    participant I as 原生索引
    U->>API: 新增 / 更新 / 归档 / 删除来源
    API->>DB: 同事务来源版本 + outbox
    P->>DB: 读取一批 pending；关闭 rows
    loop 每条独立处理
        P->>K: upsert 或 delete（租户、项目、source、revision）
        K->>K: 持久 manifest pending / profile / revision
        K->>I: 替换当前来源切块或删除
        K->>K: 持久 active 和 watermark
        K-->>P: 已应用或陈旧事件幂等忽略
        P->>DB: outbox indexed
    end
    Note over P,DB: 丢失 ACK 可重放；永久 4xx/坏 payload 入 failed 台账<br/>408/429/网络错误留待重试；后续条目继续
    P->>K: 晚到旧 upsert
    K-->>P: 墓碑或较高 revision 拒绝覆盖
    U->>API: 搜索
    API->>K: 按租户/项目检索
    K->>I: FTS 或向量查询
    I-->>K: 候选切块
    K->>K: 过滤 active / profile / 当前 revision / 墓碑
    K-->>API: 片段与来源引用
    API->>DB: 重新授权并核对来源可见状态和版本
    API-->>U: 仅返回当前有权且有效的来源
```

索引网络调用不占用 SQL 写事务。归档/删除的文档产生 delete 事件，关键词回退同时过滤这些来源。索引 manifest 与数据之间通过 pending/active 防止暴露中间态；未完成写入依靠 outbox 重放，当前方案并非跨存储分布式事务。

依据：[projector.go](../../internal/platform/projector.go)、[search.go](../../internal/platform/search.go)、[KnowledgeIndex](../../packages/knowledge/src/index.ts)。验证：[投影测试](../../internal/platform/projection_test.go)、[原生索引测试](../../packages/knowledge/tests/index.test.ts)。

## 失败事件恢复

```mermaid
sequenceDiagram
    participant E as Engine / Tick / Complete
    participant DB as SQL
    participant P as ProjectFailures
    participant S as FailureIssue
    E->>DB: 状态迁移与 run_failed 事件同事务
    P->>DB: 查询未消费失败事件
    P->>S: 按 tenant / project / run 的固定 operation_id
    S->>DB: 问题创建或原回执重放
    P->>DB: 插入消费 sequence 台账
    Note over P,DB: 问题创建后崩溃、台账前失败：下轮重放原回执<br/>重启和 run 后续重试不会抹除历史失败事件
```

投影表在 [Service.Migrate](../../internal/platform/database.go) 通过 [MigrateProjections](../../internal/platform/projection_schema.go) 安装；两个运行循环只执行数据操作。验证：[失败投影测试](../../internal/platform/failure_projection_test.go)。

## 图表验收边界

本页对照源码和本地测试编写，Mermaid 为可维护源码。文档链接检查不代表图形渲染、部署拓扑、外部账户或容量验收；最终交付需在使用方文档渲染器中确认显示，并按能力矩阵补齐未验收项。
