# 当前能力选择与保存恢复 AI SOP

状态：1.0.0／T-P4-034。负责人：平台集成。关联需求：扩展需求21、22；[API合同](../../contracts/task-dispatch-options.md)、[架构](../architecture/intelligent-capability-selection-notes.md)、[交互](../design/task-dispatch-options.md)。实际证据统一收录于[阶段验证](../quality/task-dispatch-options-verification.md)。

## 工具选择

1. 确认当前组织、项目、任务版本、已发布流程版本及目标智能体节点。保留任务设计、代码基线和知识来源；不要把执行状态当作业务验收。
2. 打开任务派发中的“可用工具推荐”。服务端在当前授权事务内读取工具库存、私有／资源池路由，并使用原派发编译器校验流程。该读取不调用模型、不创建计划或运行、不预留额度。
3. 阅读当前页的可用与不兼容原因。本页推荐按稳定 ID 顺序选取首个兼容项。需要其他工具时可继续翻页，读取失败后可重试当前页，也可返回上一页或首页。原选择器仍可用于手动选择。
4. 采用仅填写工具，保留真实档案版本、命令和摘要；然后明确预览。原预览固定任务、流程、工具、设计、基线及所选知识。修改依据后重新预览，不复用旧批准。
5. 只有原正式启动入口可创建执行；Start／Claim再次核验当前权限、库存、路由、预算、资源和精确输入。未知结果先恢复原操作，不能换操作标识或盲目重派。
6. 正式执行后分别跟踪节点、独立准确候选测试、人工验收和发布。当前推荐接口不改变任何门禁，不为生产操作自动批准。

CLI只读查询：`suma --project PROJECT tasks dispatch-options TASK --expected-version VERSION --workflow WORKFLOW --workflow-version VERSION --node NODE`。使用现有身份与项目范围；`--limit`为1–200，`--before`只能复用该任务／流程／节点依据的原分页游标。HTTP、CLI和浏览器使用同一领域服务。

已知工具配置 ID 时，可追加 `--tool TOOL` 直接核对该配置，无需逐页查找。该参数与 `--limit`、`--before` 互斥。授权范围内的配置返回一项，`eligible` 和 `reason_code` 分别给出当前兼容性和原因，`tool.version`、`tool.profile.profile_sha256` 用于核对所读版本。停用和撤销也会返回当前诊断。当前项目无法读取的配置返回读取错误。预览与启动仍需各自显式操作。

## 终端任务派发

将项目 API 凭证放入进程环境的 `SUMA_TOKEN`，所有命令使用同一个 `--project`。工具选项要求当前项目读取资格。独立测试核对、正式预览和启动要求当前项目写权限。CLI 输出 JSON 及可供脚本判断的退出状态。

```text
suma --project PROJECT tasks dispatch-options TASK --expected-version TASK_VERSION --workflow WORKFLOW --workflow-version FLOW_VERSION --node DEV_NODE
suma --project PROJECT tasks dispatch-options TASK --expected-version TASK_VERSION --workflow WORKFLOW --workflow-version FLOW_VERSION --node DEV_NODE --tool DEV_TOOL
suma --project PROJECT tasks dispatch-test-option TASK --expected-version TASK_VERSION --workflow WORKFLOW --workflow-version FLOW_VERSION --node DEV_NODE --tool DEV_TOOL --expected-tool-version DEV_VERSION --test-node TEST_NODE --test-tool TEST_TOOL --expected-test-tool-version TEST_VERSION
suma --project PROJECT tasks dispatch-preview --file request.json
suma --project PROJECT tasks dispatch-plan PLAN
suma --project PROJECT tasks dispatch-start PLAN --expected-version PLAN_VERSION --operation ORIGINAL_START_KEY
```

- 核对结果中的 `binding` 提供 `node_id`、`tool_profile_id` 和 `tool_profile_version`。准备正式预览时，分别放入 `test_bindings` 项的 `node_id`、`tool_profile_id` 和 `expected_tool_version`。
- `request.json` 使用原 `TaskDispatchPreviewRequest`，包含任务、流程、开发节点和工具的固定版本、明确的 `idempotency_key`，及适用的模型、知识或测试覆盖。使用 `--file -` 可从标准输入读取。
- 审阅计划的准确任务、工具、路由和独立测试绑定，再显式启动。预览结果未知时保留原请求文件和键；启动结果未知时沿原计划、原版本及 `--operation` 恢复。退出码 2 表示本机用法错误，1 表示控制面或传输错误。

以下是带独立测试覆盖的预览正文。命令将 `--project` 传给服务端；若正文也带 `project_id`，两处须一致。

```json
{
  "task_id": "TASK",
  "expected_task_version": 4,
  "workflow_id": "WORKFLOW",
  "workflow_version": 2,
  "agent_node_id": "DEV_NODE",
  "tool_profile_id": "DEV_TOOL",
  "expected_tool_version": 3,
  "test_bindings": [
    {
      "node_id": "TEST_NODE",
      "tool_profile_id": "TEST_TOOL",
      "expected_tool_version": 5
    }
  ],
  "idempotency_key": "original-preview-key-01"
}
```

启动回执中的运行 ID 可用于 `suma --project PROJECT run show RUN` 或 `run events RUN`。后续由调度器领取，任务验收沿原质量流程。 [独立测试合同](../../contracts/test-dispatch-bindings.md#选择时检查)

## 在终端核对派发推荐

候选查询使用已有登录会话与原用户、组织。准确指定当前任务版本、流程版本及开发节点。终端默认查询常用步骤与输出上限，也可提供更严格的 Token 限额。

```text
suma --project PROJECT --expected-user USER --expected-organization ORG tasks recommendation-options TASK --expected-version 3 --workflow FLOW --workflow-version 2 --node develop
suma --project PROJECT tasks recommendations TASK --limit 20
suma --project PROJECT tasks recommendation RECOMMENDATION
```

如需按硬预算筛选候选，在第一条命令中追加 `--require-hard-budget --token-limit 20000`。候选及排除范围以响应的 `coverage` 为准。历史可用项目 API 凭证读取，按返回的 `next_cursor` 继续。读取历史时保留原推荐、当时依据和当前有效性。 [推荐合同](../../contracts/dispatch-recommendations.md#终端只读核对)

## 从节点上报查找已有工具档案

已知节点 ID 时，直接列出该节点的固定配置与历史。此查询沿原项目授权，完整 ID 区分大小写。可组合 `--state`、`--query`、`--limit`，续页时保留相同条件并使用原 `--cursor`。

```sh
suma --project PROJECT tools list --worker team-codex --state all
```

在项目工具管理中确定执行节点 ID，再用已有登录会话读取。共享资源池节点使用项目当前获准的 Worker ID。会话通过 `SUMA_TOKEN` 提供，并固定原用户和组织。

```text
suma --project PROJECT --expected-user USER --expected-organization ORG tools inventory --worker WORKER
suma --project PROJECT --expected-user USER --expected-organization ORG tools matches --worker WORKER --command COMMAND
```

`inventory` 返回该节点最近一次上报的状态和时间。`fresh` 可显示工具明细及单项可用性，`stale` 和 `unreported` 分别表示旧上报或无上报。第二条命令按原上报里的 `profile.command_id` 选取完整固定档案，直接查看相同配置的已有记录。上报已过期或所选命令缺失或不可用时，命令提示重新读取当前上报。匹配结果继续区分已启用、暂停和撤销，配置变更沿原显式操作进行。 [工具档案复用合同](../../contracts/tool-profile-reuse.md#从当前节点上报复用档案)

### 从终端进入快捷接入

```sh
suma --control https://suma.example.com --project PROJECT tools inventory --worker team-codex --connect-command codex
```

将命令 ID 替换为节点上报中的实际 `profile.command_id`，打开结果中的 `url` 并点击「核对此工具」。API 和工作台地址分开时，加 `--workbench-origin https://workbench.example.com`。按需沿原登录会话指定 `--expected-user` 和 `--expected-organization`。核对后可查看已有档案或明确保存为暂停。若配置摘要变化，请在终端重新读取节点上报并生成链接。 [接入合同](../../contracts/tool-profile-reuse.md#从终端进入快捷接入)

## 终端运行跟踪

启动后使用回执中的运行 ID。先跟随持久事件，再用队列和精确节点读取核对当前调度条件。

```text
suma --project PROJECT run watch RUN
suma --project PROJECT run watch RUN --after SEQUENCE --max-events 20
suma --project PROJECT run queue --limit 50
suma --project PROJECT run queue --route pool --run RUN --limit 50
suma --project PROJECT run queue --route pool --run RUN --cursor NEXT_CURSOR
suma --project PROJECT run scheduling RUN --node NODE
suma --project PROJECT run workbuddy-status RUN --node NODE
```

- `run watch` 每行输出一条事件 JSON。断线从最后成功输出的序号重连。脚本重启时可传同一运行最后完整一行的 `sequence`。其他运行使用各自游标。
- `--max-events` 取 1–100000，到达条数后结束。省略时持续跟随，按 Ctrl+C 退出。单连接受全局 `--timeout` 约束，读取权限变化或收到无效协议时会明确退出。
- `--route project` 查看项目专用路径，`--route pool` 查看固定到共享池的运行。加 `--run RUN` 可在路径范围内定位准确运行。省略条件时查看当前项目的全部对应项。续页应重复原来的筛选条件和项目，再传原 `next_cursor`。
- 队列展示当前已就绪节点及服务端分页结果。找到运行和节点后，`run scheduling` 返回当前阻塞、所需资源和执行路由依据。历史事件、当前准入、实际结果与人工验收分别读取。
- `run queue` 和 `run scheduling` 输出可包含 `tool_binding` 与 `source: "tool"` 阻塞。固定引用可定位原工具档案，暂停、撤销、变化、缺失和启用权限失效各有独立代码。无工具阻塞时仍按输出中的其他准入条件与领取核验判断。
- 共享池的 `pool_runner_registration_missing` 表示缺少活动登记，`pool_runner_credential_unavailable` 表示活动登记均缺少当前有效凭证。两项由当前项目授权读取，实际能力和领取状态需独立核对。
- `run workbuddy-status` 读取指定 WorkBuddy 节点的本地已保存进度，输出一份 JSON。可用项目 API 凭证读取。`not_recorded` 表示本地没有提交记录，`unknown` 表示外部提交待核对。供应方完成与平台交付分别判断。需要跟随平台事件时，继续使用同一运行的 `run watch`。

### 核对某次执行尝试

```sh
suma --project PROJECT run attempts RUN --node NODE --limit 20
suma --project PROJECT run attempt RUN --node NODE --attempt 2
suma --project PROJECT run tool-events RUN --node NODE --attempt 2 --limit 20
```

先从尝试列表定位执行者和尝试号，再读取该次尝试的执行状态、结果来源、核对记录或子运行。工具事件是该次尝试已经保存的进度事实。两种列表各自使用返回的 `next_cursor` 续页，保留同一项目、运行、节点和尝试范围。工具事件页还保留原观测窗口的 `watermark`，可区分已读取的历史窗口与随后发生的事件。

命令输出 JSON，可使用当前项目 API 凭证。当前登录会话可用 `--expected-user` 和 `--expected-organization` 核对原账号与组织。指定尝试不存在时显示服务端拒绝结果。待核对的执行继续在工作台按原操作处理。

### 核对智能体等待的问题

从 `run attempt` 的响应取得准确尝试号与 fence。使用当前登录会话查询该次尝试。

```sh
suma --project PROJECT --expected-user USER --expected-organization ORG run inputs RUN --node NODE --attempt 2 --fence 7
```

结果包含问题 ID、状态、答复是否已保存，以及已观察到的写入阶段和关闭状态。打印内容只包含诊断摘要。打开 `workbench_url` 可按当前网页权限阅读原问题和提交答复。前后端分开部署时可加 `--workbench-origin`。如果尝试号或 fence 不匹配，请重新读取原尝试。 [工具问题合同](../../contracts/tool-inputs.md#从终端核对工具问题)

[持久事件合同](../../contracts/automation-events.md#持久运行事件) · [调度工作台](../design/scheduling-workspace.md)

当前项目的 WorkBuddy 已保存提交可集中读取。默认每页 10 条，`--limit` 支持 1 至 20，续页传入同一项目和读者下的原 `next_cursor`。

```sh
suma --project PROJECT run workbuddy-list --limit 10
suma --project PROJECT run workbuddy-list --limit 10 --before NEXT_CURSOR
```

输出中的平台状态、提交状态和最近供应方观察来自已保存记录。打开对应节点使用 `run workbuddy-status` 核对单条详情。

分页输出可携带 `task_links`。按 `run_id`、`node_id` 匹配条目，可读取原正式任务编号、当前任务版本、运行关系及可用的任务标题。`current` 表示仍关联该运行，`changed` 表示已改绑，`unbound` 表示已解绑。该链接定位任务和交付，节点进度仍以对应条目为准。

```sh
suma --project PROJECT run workbuddy-list --filter attention --limit 10
suma --project PROJECT run workbuddy-list --filter attention --limit 10 --before NEXT_CURSOR
```

用 `--task TASK` 查看指定正式任务的已保存提交，可同时限定待处理。续查时保留同一任务、筛选和原游标。空批次仍带游标时可继续查找。

```sh
suma --project PROJECT run workbuddy-list --task TASK --filter attention
suma --project PROJECT run workbuddy-list --task TASK --filter attention --before NEXT_CURSOR
```

`--filter attention` 查当前项目已保存的待处理记录。筛选页有游标时可继续，即使本页条目为空。每次调用读取一个批次，下一次保留同一项目、读者和筛选。

## IDE 保存回执恢复

1. 保存请求固定当前项目、路径、原文件hash、提交内容、操作标识和原API调用者。请求结果未确定时保留它们；后续编辑只是新的草稿。
2. 只有返回路径、提交内容和UTF-8字节的SHA-256均准确匹配，才确认成功并释放原请求。不能只凭200、界面文字或代理成功响应确认写入。
3. 回执无效、连接中断或服务端要求核对时，显示恢复入口；当前会话保留原请求及新草稿。后续403／409不能解释更早未知写入的结果。
4. 明确恢复原请求；原回执成功后仅更新已保存基线，保留后来编辑的草稿。再次明确保存时使用新操作标识及新基线hash。
5. 首次确定的409按现有冲突比较处理。不要用刷新、切换文件、切换组织或覆盖远端来消除未确认状态。浏览器关闭后的持久草稿和跨设备编辑恢复仍待后续交付。

## 排障边界

- 没有推荐：检查原任务和流程是否仍存在且当前；再检查工具档案启用状态、真实Runner上报、路由与流程兼容性。不要把“已配置”显示成“已在线”。
- 推荐后预览失败：以正式错误为准；推荐是观察，不是执行许可或资源保留。权限失效后隐藏旧候选；旧读取迟到不能覆盖新的任务依据。
- 模型／费用：当前可写工具固定模型；费用未知。不得将未知费用折成零，也不能通过推荐切换模型参数或绕过用量账本。
- 本地启动配置：`SUMA_PUBLIC_URL`须符合现有安全校验；HTTP本机使用受支持的loopback地址。开发用自定义`.localhost`浏览器Origin应加入明确允许列表，不关闭来源校验。

禁止将本机故障注入代理用于真实用户文件或生产流量。集成验证只使用命名隔离副本、任务拥有的工作区及既有测试身份；原主预览、凭证、测试／模型计费与历史候选均需保持可追溯。
