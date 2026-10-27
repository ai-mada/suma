# 项目工作区 Git 与编辑执行限额合同

状态：T-P4-035／1.0.0，实施合同。负责人：平台集成、控制面、工作台、执行节点。关联扩展需求14、15、21、22；不缩减完整P0–P7及1–22范围。

## 当前工作区与正式交付

实时Git工作区用于审阅编辑中的代码，不作为已封候选、正式测试或人工验收证明。平台已有候选、合成、代码基线及准确候选测试合同保持独立。读取不能暂存、提交、合并、切分支、运行钩子、修改仓库、派发工具或调用模型。

工作区权限沿用当前项目文件读取：绑定必须位于管理员配置的目录内，读取前后重新核验原项目身份及同一绑定。对普通仓库、未提交仓库和分离HEAD给出实际状态。链接工作树的外置共同仓库需要单独项目仓库授权；现有单一文件root绑定不足以证明该权限，返回明确`unsupported_layout`，不能据全局允许目录读取另一个项目对象库。该缺口保留在后续仓库授权计划中，不宣称完整IDE完成。

## 接口

两个GET均要求单个`project_id`，拒绝未知、重复或空参数；无写入body。`source`只能为`worktree`或`index`，缺省`worktree`。路径必须为相对当前绑定的合法UTF-8路径，不准绝对路径、遍历、凭据目录或链接。

- `GET /workspace/git?project_id=...` → `WorkspaceGitStatus`。
- `GET /workspace/git/diff?project_id=...&path=...&source=worktree|index` → `WorkspaceGitDiff`。
- CLI：`suma --project PROJECT workspace git`；`suma --project PROJECT workspace diff --path RELATIVE [--source worktree|index]`，同一身份、项目范围及正式接口。

`WorkspaceGitStatus`字段：`project_id`、`available`、`reason_code`、`branch`、`head`、`unborn`、`detached`、`files: WorkspaceGitChange[]`、`total`、`limit`、`truncated`、`restricted_files`、`observed_at`。`WorkspaceGitChange`字段：`path`、`previous_path`、`index_status`、`worktree_status`、`untracked`、`conflicted`。状态码使用Git原生单字符，包括未变化空格；未跟踪为`?`。没有HEAD或分支时为空字符串；全部字段显式返回。

状态最多展示200项，完整有界输出内计算实际可显示总数及是否截断。敏感路径不输出名称，仅提供受限项数量；Git输出超过整体上限明确拒绝，不把部分数据说成完整列表。`available=false`明确区分`not_repository`、`git_unavailable`、`unsupported_layout`、`unsafe_config`；不把“已配置”当成“Git可用”。

`WorkspaceGitDiff`字段：`project_id`、`path`、`previous_path`、`source`、`head`、`original`、`modified`、`original_sha256`、`modified_sha256`、`binary`、`observed_at`。返回真实两侧文本供成熟Monaco DiffEditor只读显示；每侧最多1MiB。`index`比较HEAD与暂存区，`worktree`比较暂存区与当前文件。新文件原文为空，删除后文本为空；空文本必须带真实空字节SHA256。重命名使用准确旧路径读取来源。冲突不能获得可靠对应时明确拒绝差异预览，仍可用原文件编辑器处理。

二进制／非UTF8内容只返回`binary=true`及范围元数据，两侧文本及摘要均为空字符串，不发送原始二进制或伪造空文件摘要。文本摘要必须为准确UTF-8字节的SHA256。工作区可以在列表读取后继续编辑，差异是点击时当前观察；HEAD、索引、仓库控制路径或绑定在单次读取中变化返回冲突，不默默把新基础当成原基础。界面绑定当前项目、路径、source及HEAD，旧响应不得覆盖新选择。

## Git执行边界

复用已安装Git及平台成熟受限执行模式，无shell、网络或新依赖。固定`--git-dir`、`--work-tree`，不向祖先寻找仓库。请求总截止时间8秒；状态输出最多2MiB，错误输出最多8KiB。只传最低必需环境，禁终端提示、可选索引写、replace对象、系统／用户配置、外部attributes、自动GC／maintenance、网络协议、子模块递归、fsmonitor、钩子、外部差异及textconv。

在执行可能读取工作区的Git命令前，检查并拒绝本地include／includeIf、外部filter／process等可执行配置；拒绝元数据链接、外置common目录、替代对象库等越界布局。读取文件使用目录约束，缺失路径仅用于准确Git删除记录，任何仍存在的父级都不能是链接或越出绑定。配置变化重新核验。此限制是受管本地读取边界，不等于完整操作系统进程隔离。

## 保存快捷键与真实执行限额

保存通过Monaco原生`addAction(CtrlCmd | KeyS)`注册，读取最新当前保存状态。正在保存、无文件、无修改或明确冲突时不新增写入；原未知请求优先恢复其原API调用者、内容、key与基线。一次未完成的快捷键保存不得重复派发；卸载释放注册。切换到Git审阅保留原编辑模型与草稿，不卸载丢内容。

原生工作区执行在已有工具身份核验后、创建工作区／MCP／主进程前验证节点`max_steps`与`max_output_tokens`，后者准确映射SDK输入`max_tokens`。普通步数缺省8、上限16；具有准确受管冲突修复能力时缺省64、上限128。输出总额度缺省4096，普通上限32768、修复上限131072，与现有SDK累计输出allowance和账本预留输入一致。拒绝显式null、非整数、零、负数及越界值，不丢弃后恢复较宽默认值。

修复流的模型请求证明必须按准确已验证的修复协议接受最多128步／单次1024额度；普通流仍最多16步／单次4096。该修正不使可写模型自动获得计价资格，也不允许环境中的任意模型变化冒充固定模型版本。独立模型绑定／费率合同另行实施。
