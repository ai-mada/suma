package cli

import (
	"errors"
	"os"
	"strings"

	"github.com/ai-mada/suma/internal/localization"
)

// CLI 语言只在当前进程读取，显式参数必须有效；未知环境语言可以回退，
// 不写操作系统、用户或服务端设置。HTTP 诊断始终尊重服务端返回的原文。
func cliLocale(explicit string, explicitSet bool) (localization.Locale, error) {
	if explicitSet {
		if locale, ok := localization.Normalize(explicit); ok {
			return locale, nil
		}
		return localization.DefaultLocale, errors.New("unsupported explicit display language")
	}
	if locale, ok := localization.Normalize(os.Getenv("SUMA_LANGUAGE")); ok {
		return locale, nil
	}
	if locale, ok := localization.EnvironmentLocale(os.LookupEnv); ok {
		return locale, nil
	}
	return localization.DefaultLocale, nil
}

type cliMessage struct{ key, zh string }

// 精确匹配的是本 CLI 自己生成的静态诊断，绝不把远端诊断、用户文本或
// 工具原始输出送进此映射。机器 code、状态和幂等请求正文不受语言影响。
var cliMessages = map[string]cliMessage{
	dispatchRecommendationOptionsCLIUsage:           {"cli.dispatch_recommendation.options_usage", "候选查询需要准确的任务版本、流程版本和开发节点。可选步骤上限、输出 Token 上限和 Token 限额。--require-hard-budget 需要同时提供 --token-limit"},
	dispatchRecommendationOptionsCLIResponseInvalid: {"cli.dispatch_recommendation.options_response", "候选响应须保留请求的任务、流程、节点和约束"},
	dispatchRecommendationListUsage:                 {"cli.dispatch_recommendation.list_usage", "推荐历史需要准确的 TASK，可选 --limit 1..100 和 --before 原分页游标"},
	dispatchRecommendationGetUsage:                  {"cli.dispatch_recommendation.get_usage", "推荐读取需要一个准确的推荐 ID"},
	dispatchRecommendationHistoryResponseInvalid:    {"cli.dispatch_recommendation.history_response", "推荐响应须保留请求范围并包含有效记录"},

	toolProfilesUsage:              {"cli.tool_profiles.usage", "tools 需要 list、catalog、inventory、doctor、matches、replace 或 replacements，请查看 tools --help"},
	toolCatalogUsage:               {"cli.tool_catalog.usage", "tools catalog 需要 --project，支持 --direction all|suma_to_tool|tool_to_suma 和 --query 检索文本，最多 120 个 UTF-8 字节"},
	toolCatalogResponseInvalid:     {"cli.tool_catalog.response", "工具接入目录响应须保留请求项目并包含有效目录"},
	toolReplacementUsage:           {"cli.tool_profiles.replacement_usage", "tools replace 需要 --id 原配置ID、--file JSON文件|- 和明确 --confirm；tools replacements 支持 --id 配置ID、--direction outgoing|incoming、--limit 1–100 及原 --cursor；请查看 tools --help"},
	toolReplacementInputInvalid:    {"cli.tool_profiles.replacement_input", "配置换新需要准确项目与原版本、完整目标配置及节点、明确 pause_original、新建名称或准确嵌套 reuse，以及原稳定操作标识"},
	toolReplacementBrowserRequired: {"cli.tool_profiles.replacement_browser", "tools replace 需要已有登录会话和捕获的原 --expected-user；服务端仍核验当前项目批准权限"},
	toolReplacementUnconfirmed:     {"cli.tool_profiles.replacement_unconfirmed", "换新回执尚未确认；保留原账户、组织、配置ID、JSON正文和操作标识，仅可重试相同已确认请求"},
	toolReplacementHistoryInvalid:  {"cli.tool_profiles.replacement_history", "换新历史必须保留准确项目、配置、方向、不可变快照及原分页游标"},
	toolListUsage:                  {"cli.tool_profiles.list_usage", "tools list 需要 --project。--state 可选 active、enabled、paused、revoked 或 all，--worker 使用完整节点 ID，检索文本最多 120 个 UTF-8 字节，--limit 为 1–200，翻页使用原 --cursor。请查看 tools list --help"},
	toolListResponseInvalid:        {"cli.tool_profiles.list_response", "工具列表响应必须保留原项目、节点筛选、状态、完整档案及分页边界"},
	toolDoctorUsage:                {"cli.tool_profiles.doctor_usage", "tools doctor 需要 --id 档案ID 和 --project；--format 为 json 或 table；请查看 tools --help"},
	toolDoctorResponseInvalid:      {"cli.tool_profiles.doctor_response", "工具体检响应必须保留请求项目、准确完整档案、四项检查、准确维护建议及最多 60 秒的有效展示期限"},
	toolMatchesUsage:               {"cli.tool_profiles.matches_usage", "tools matches 需要 --file 请求JSON|-，或同时提供 --worker 节点与 --command 命令。请查看 tools --help"},
	toolMatchesInvalid:             {"cli.tool_profiles.matches_input", "工具匹配查询需要当前项目、worker_id 和完整准确的 profile"},
	toolMatchesResponseInvalid:     {"cli.tool_profiles.matches_response", "工具匹配响应必须保留原项目、节点、完整档案及完整结果数量"},
	toolInventoryUsage:             {"cli.tool_inventory.usage", "tools inventory 需要 --project 和一个明确的 --worker 节点"},
	toolInventoryConnectUsage:      {"cli.tool_inventory.connect_usage", "生成接入链接需要准确的项目 ID 和一个有效的 --connect-command。请查看 tools --help"},
	toolInventoryOriginInvalid:     {"cli.tool_inventory.origin_invalid", "--workbench-origin 必须是 HTTP(S) 来源地址，不能包含凭证、路径、查询参数或片段"},
	toolInventoryLinkSensitive:     {"cli.tool_inventory.link_sensitive", "接入链接可能暴露当前凭证"},
	toolInventoryResponseInvalid:   {"cli.tool_inventory.response", "工具上报响应必须保留请求范围，并包含有效的观测记录"},
	toolInventoryMatchUnusable:     {"cli.tool_inventory.match_unusable", "所选节点当前没有新鲜上报。请用 tools inventory 查看上报并刷新后再继续"},
	toolInventoryMatchMissing:      {"cli.tool_inventory.match_missing", "节点的新鲜上报中没有所选命令。请用 tools inventory 查看已上报的命令"},
	toolInventoryMatchUnavailable:  {"cli.tool_inventory.match_unavailable", "节点的新鲜上报显示所选命令不可用。请用 tools inventory 查看上报原因"},
	worklogCLIUsage:                {"cli.worklog.action_required", "worklogs 需要 list、get、create、correct、void 或 history，请查看 worklogs --help"},
	worklogCLIListInvalid:          {"cli.worklog.list_options", "工时列表须提供 --from-date 与 --to-date（YYYY-MM-DD，含首尾最多 31 天）、有效筛选参数及 1–50 的 --limit"},
	worklogCLIReadInvalid:          {"cli.worklog.read_options", "读取／历史需要有效工时 ID；历史支持 1–50 的 --limit 与 --cursor"},
	worklogCLICreateInvalid:        {"cli.worklog.create_options", "人工工时新建须提供 --task、正整数 --task-version、--member、--date YYYY-MM-DD、1–1440 的整数 --minutes 及 8–128 个 UTF-8 字节的 --operation；--note 最多 200 字"},
	worklogCLICorrectInvalid:       {"cli.worklog.correct_options", "人工工时更正须提供 ID、正整数 --version 与 --task-version、--task、--date YYYY-MM-DD、1–1440 的整数 --minutes、1–200 字的 --reason 及 8–128 个 UTF-8 字节的 --operation；--note 最多 200 字"},
	worklogCLIVoidInvalid:          {"cli.worklog.void_options", "人工工时作废须提供 ID、正整数 --version、1–200 字的 --reason 及 8–128 个 UTF-8 字节的 --operation"},
	workspaceTreeCLIUsage:          {"cli.workspace_tree.usage", "目录浏览命令：workspace files [--directory 相对目录] [--limit 1..200] [--before 上一页游标]"},
	compositionCLIUsage:            {"cli.candidate_composition.action_required", "candidate-compositions 需要 options、list、get、conflict、revision-context、create、repair、revise-repair、start 或 command；task-code-baseline 需要 get 或 bind，请查看 --help"},
	compositionCLIRevisionInvalid:  {"cli.candidate_composition.revision_basis_required", "修复计划修订需要准确计划、任务、冲突和所选流程／工具版本、明确确认、理由及原操作标识"},
	modelOptionsUsage:              {"cli.model_execution.usage", "模型选项需要：models options --tool 工具ID --expected-tool-version 当前版本"},
	dispatchOptionsCLIUsage:        {"cli.task_dispatch_options.usage", "任务工具推荐需要：tasks dispatch-options TASK --expected-version N --workflow ID --workflow-version N --node ID；可选 --limit 1..200 与 --before 原分页游标"},
	dispatchTestOptionCLIUsage:     {"cli.task_dispatch.test_option_usage", "独立测试预检须提供任务、流程、开发节点和两个工具的准确标识与版本，请查看 tasks dispatch-test-option --help"},
	dispatchPreviewCLIUsage:        {"cli.task_dispatch.preview_usage", "派发预览须提供 --file 请求JSON|- 及正文中的明确 idempotency_key，长度为 8–128 个 UTF-8 字节"},
	dispatchPreviewCLIJSON:         {"cli.task_dispatch.preview_json", "派发预览须提供一个键名唯一的 UTF-8 JSON 对象及明确的原 idempotency_key"},
	dispatchPlanCLIUsage:           {"cli.task_dispatch.plan_usage", "派发计划读取须提供准确的 PLAN 标识"},
	dispatchStartCLIUsage:          {"cli.task_dispatch.start_usage", "启动派发须提供 PLAN、--expected-version 当前版本和 --operation 原标识，长度为 8–128 个 UTF-8 字节"},
	sharedTaskViewUsage:            {"cli.shared_task_views.usage", "task-views 需要 list、get ID、create --file JSON、update ID --file JSON 或 archive ID --file JSON，并提供 --project"},
	taskDiscussionsCLIUsage:        {"cli.task_discussions.usage", "任务讨论需要 list --task、people --task、message 消息ID --task 或 post --file JSON|-；请查看 discussion --help"},
	taskDiscussionsCLIInput:        {"cli.task_discussions.input", "讨论发送须保留与 --project 一致的 project_id、任务ID、正整数任务版本、非空原正文和明确原操作标识；可选唯一人员资源ID或准确知识引用，不能指定作者或角色"},
	inboxCLIUsage:                  {"cli.notification_inbox.usage", "个人收件箱命令：inbox list 支持 --filter inbox、unread 或 archived、--limit 1–50 与 --cursor；inbox read／unread／archive／restore 需要通知ID、--version 原版本与明确 --operation 原操作标识；均须提供 --project"},
	taskDeliveryUsage:              {"cli.task_delivery.usage", "task delivery 需要一个准确任务 ID，并在命令前提供 --project"},
	issueRepairCLIUsage:            {"cli.issue_repair.usage", "issue-repair 需要 show ISSUE，且须在命令前提供 --project。请查看 issue-repair --help"},
	issueRepairCLIResponse:         {"cli.issue_repair.response", "缺陷修复响应的项目、缺陷或原始用例引用不符合请求"},
	testPlanCatalogUsage:           {"cli.test_plan.usage", "test-plan 需要 list 或 show PLAN。list 支持 --state draft、published 或 retired、--task TASK、--limit 1–100 和原 --before。请查看 test-plan --help"},
	taskCollectionCLIUsage:         {"cli.task_collection.usage", "任务集合参数无效：通用参数为 --q、--status 或 --status-unset、--owner 或 --unassigned、--priority 或 --priority-unset、--requirement；tasks list 接受 --sort、--limit 1–100 与 --cursor 原游标；tasks groups 接受 --group-by status、owner 或 priority、--group-limit 1–100 与 --group-cursor 原分组游标"},
	workspaceWriteUsage:            {"cli.workspace_save.usage", "工作区保存需要：save --file JSON [--wait]、save-status ID [--wait] 或 inspect-save ID --expected-version 版本 [--operation 原操作标识] [--wait]"},
	workspaceWriteUnconfirmed:      {"cli.workspace_save.unconfirmed", "尚未确认文件已保存；再次保存前请核对原保存命令"},
	workspaceObservationUsage:      {"cli.workspace_observation.usage", "远程工作区命令：workspace targets；workspace observe --file 请求JSON [--wait]；workspace observation ID [--wait]"},
	workspaceGitCLIUsage:           {"cli.workspace_git.usage", "工作区只读命令：workspace git；或 workspace diff --path 相对路径 [--source worktree|index]"},
	compositionCLIReadInvalid:      {"cli.candidate_composition.read_invalid", "候选合成读取参数无效，请提供有效任务或计划标识及 1–100 的 limit"},
	compositionCLIWriteOptions:     {"cli.candidate_composition.write_options", "候选合成与代码基线写入需要 --file JSON_FILE 或 --file -，并明确提供 --confirm"},
	compositionCLIRepairInvalid:    {"cli.candidate_composition.repair_basis_required", "冲突修复需要准确冲突依据、类型化修复任务、流程和工具版本及稳定操作标识"},
	compositionCLIWriteInvalid:     {"cli.candidate_composition.basis_required", "候选合成需要类型化 JSON、精确整数版本、按序 2–16 个不同来源、明确确认及稳定的 8–128 字符 idempotency_key"},
	diagnosisCLIUsage:              {"cli.failure_diagnosis.action_required", "failure-diagnosis 需要 context、list、get、create 或 command，请查看 failure-diagnosis --help"},
	diagnosisCLIReadInvalid:        {"cli.failure_diagnosis.read_invalid", "失败分诊读取参数无效，请提供 --run、--node、规范正整数 --attempt 及 1–100 的 limit"},
	diagnosisCLIWriteOptions:       {"cli.failure_diagnosis.write_options", "失败分诊写入需要 --file JSON_FILE 或 --file -，并明确提供 --confirm"},
	diagnosisCLIWriteInvalid:       {"cli.failure_diagnosis.basis_required", "失败分诊需要类型化 JSON、精确整数版本、固定来源 SHA256、明确确认和稳定的 8–128 字符 idempotency_key"},
	diagnosisCLIBrowserRequired:    {"cli.failure_diagnosis.browser_required", "失败分诊写入需要既有浏览器会话凭据和原 --expected-user；不支持 API 凭据写入，视图请求头不授予权限"},
	expertPlanUsage:                {"cli.team_plan.action_required", "team-plan 需要 list、get、revisions、create、start 或 command，请查看 team-plan --help"},
	expertPlanReadInvalid:          {"cli.team_plan.read_invalid", "交付计划读取参数无效，请提供有效标识、修订号及 1–100 的 limit"},
	expertPlanScopeResponseInvalid: {"cli.team_plan.scope_response_invalid", "交付计划列表响应与请求的项目、团队、运行或分页范围不符"},
	expertPlanWriteOptions:         {"cli.team_plan.write_options", "交付计划写入需要 --file JSON_FILE 或 --file -，并明确提供 --confirm"},
	expertPlanWriteInvalid:         {"cli.team_plan.basis_required", "交付计划需要精确正整数版本、类型化任务、明确确认及稳定的 8–128 字符 idempotency_key"},
	"skill-catalog requires list, get, revisions, promote, update, retire, or adopt; use skill-catalog --help":              {"cli.skill_catalog.action_required", "skill-catalog 需要 list、get、revisions、promote、update、retire 或 adopt，请查看 skill-catalog --help"},
	"invalid skill-catalog read options; limit must be 1..100 and state published or retired":                               {"cli.skill_catalog.read_invalid", "技能目录读取参数无效，limit 须为 1–100，state 须为 published 或 retired"},
	"skill-catalog requires a valid catalog ID":                                                                             {"cli.skill_catalog.id_required", "技能目录命令需要有效的目录标识"},
	"skill-catalog get accepts only the catalog ID":                                                                         {"cli.skill_catalog.get_options", "skill-catalog get 仅接受目录标识"},
	"skill-catalog writes require --file JSON_FILE or --file - and --confirm":                                               {"cli.skill_catalog.write_options", "技能目录写入需要 --file JSON_FILE 或 --file -，并提供 --confirm"},
	"skill-catalog writes require explicit --confirm; approval and draft publication remain server-controlled":              {"cli.skill_catalog.confirm_required", "技能目录写入须明确提供 --confirm；批准与草稿发布仍由服务端控制"},
	"skill-catalog writes require explicit integer versions, fixed SHA256, reason, and an 8..128 character idempotency_key": {"cli.skill_catalog.basis_required", "技能目录写入须提供明确整数版本、固定 SHA256、说明和 8–128 字符 idempotency_key"},
	"skill-catalog promote and update require acknowledge_sharing=true in JSON":                                             {"cli.skill_catalog.sharing_confirmation", "共享与更新须在 JSON 中明确 acknowledge_sharing=true"},
	"skill-catalog adopt requires acknowledge_draft=true in JSON":                                                           {"cli.skill_catalog.draft_confirmation", "采用须在 JSON 中明确 acknowledge_draft=true"},

	"original-view IDs must be 32 lowercase hexadecimal characters; organization requires user": {"cli.view_context.invalid", "原始页面身份必须是 32 位小写十六进制标识；指定组织时还须指定账户"},
	"--language must be zh-CN or en-US (supported aliases are accepted)":                        {"cli.language.invalid", "--language 必须为 zh-CN 或 en-US，也支持已声明的语言别名"},
	"invalid global flags; use --help":                                                          {"cli.flags.invalid", "全局参数无效，请查看 --help"},
	"client operation failed":                                                                   {"cli.operation.failed", "客户端操作失败"},
	"--control must be an http(s) origin without credentials, query, or path":                   {"cli.control.invalid", "--control 必须是 http(s) 控制面地址，不能包含凭证、查询参数或路径"},
	"invalid timeout or token":                                                                  {"cli.options.invalid", "超时时间或凭证无效"},
	"API path must be a relative /api/v1 resource path":                                         {"cli.api.path_invalid", "API 路径必须是 /api/v1 下的相对资源路径"},
	"credentials must not be included in API URLs":                                              {"cli.api.url_credentials", "API 地址不能包含凭证"},
	"--project is required for this command":                                                    {"cli.project.required", "此命令需要 --project"},
	"URL project_id differs from --project":                                                     {"cli.project.url_mismatch", "地址中的 project_id 与 --project 不一致"},
	"JSON project_id differs from --project":                                                    {"cli.project.body_mismatch", "JSON 中的 project_id 与 --project 不一致"},
	"invalid JSON request":                                                                      {"cli.request.json_invalid", "JSON 请求无效"},
	"invalid request":                                                                           {"cli.request.invalid", "请求无效"},
	"control request failed":                                                                    {"cli.request.failed", "控制面请求失败"},
	"control request cancelled or timed out":                                                    {"cli.request.interrupted", "控制面请求已取消或超时"},
	"unable to read bounded response":                                                           {"cli.response.unreadable", "无法读取限制大小内的响应"},
	"control response was not valid JSON":                                                       {"cli.response.json_invalid", "控制面响应不是有效 JSON"},
	"cannot open JSON file":                                                                     {"cli.file.unreadable", "无法打开 JSON 文件"},
	"cannot read JSON input":                                                                    {"cli.file.input_unreadable", "无法读取 JSON 输入"},
	"JSON input must be valid UTF-8":                                                            {"cli.file.utf8_required", "JSON 输入必须使用有效的 UTF-8 编码"},
	"JSON file exceeds 4 MiB or cannot be read":                                                 {"cli.file.too_large", "JSON 文件超过 4 MiB 或无法读取"},
	"JSON file must contain an object":                                                          {"cli.file.object_required", "JSON 文件必须包含一个对象"},
	"JSON file must contain exactly one object":                                                 {"cli.file.single_object_required", "JSON 文件必须且只能包含一个对象"},
	"idempotency_key must be a string of at least 8 characters":                                 {"cli.operation.key_invalid", "idempotency_key 必须是至少 8 个字符的字符串"},
	"invalid operation payload":                                                                 {"cli.operation.payload_invalid", "操作请求正文无效"},
	"workflow publish requires --file":                                                          {"cli.workflow.file_required", "workflow publish 需要 --file"},
	"unknown command or arguments; use --help":                                                  {"cli.command.unknown", "未知命令或参数，请查看 --help"},
	"run command requires a valid run ID":                                                       {"cli.run.id_required", "运行命令需要有效的运行标识"},
	"invalid run flags":                                                                         {"cli.run.flags_invalid", "运行参数无效"},
	"run command requires --file with action and expected_version":                              {"cli.run.file_required", "run command 需要 --file，文件中须包含 action 和 expected_version"},
	"--file is only supported by run command":                                                   {"cli.run.file_unsupported", "只有 run command 支持 --file"},
	"run start requires --workflow, positive --version, and --operation (at least 8 characters)": {"cli.run.start_options", "run start 需要 --workflow、正整数 --version 以及至少 8 个字符的 --operation"},
	"unknown run command": {"cli.run.unknown", "未知运行命令"},
	"command file requires action and positive integer expected_version": {"cli.run.file_invalid", "命令文件需要 action 和正整数 expected_version"},
	"api requires a resource path":                                       {"cli.api.path_required", "api 需要资源路径"},
	"api requires a relative resource path":                              {"cli.api.relative_path_required", "api 需要相对资源路径"},
	"invalid api flags":                                                  {"cli.api.flags_invalid", "api 参数无效"},
	"unsupported API method":                                             {"cli.api.method_unsupported", "不支持此 API 方法"},
	"API mutations require explicit --method and --file":                 {"cli.api.write_options", "API 写操作必须显式提供 --method 和 --file"},
	"GET does not accept --file":                                         {"cli.api.get_file", "GET 不接受 --file"},
	"knowledge requires list, search, show, history, create, edit, extract, or command; use knowledge --help": {"cli.knowledge.action_required", "knowledge 需要 list、search、show、history、create、edit、extract 或 command，请查看 knowledge --help"},
	knowledgeSearchUsage:                                                                      {"cli.knowledge.search_usage", "knowledge search 支持 --mode keyword|index、--query（最多 200 UTF-8 字节）、--kind memory|spec|skill、--limit 1–20 和 --format json|table；index 须非空查询且禁止 --cursor，keyword 可用原 --cursor（最多 2048 字节）；请查看 knowledge --help"},
	knowledgeIndexSearchResponseInvalid:                                                       {"cli.knowledge.index_search_response", "知识索引检索须保留准确发布来源、Unicode 码点定位和真实提供方候选事实"},
	knowledgeSearchResponseInvalid:                                                            {"cli.knowledge.search_response", "知识发现结果须保留当前项目的准确发布引用及有界扫描的继续依据"},
	"invalid knowledge filters or pagination; limit must be 1..100":                           {"cli.context.query_invalid", "知识筛选或分页参数无效，limit 必须为 1–100"},
	"project context command requires a valid resource ID":                                    {"cli.context.id_required", "项目上下文命令需要有效的资源标识"},
	"project context show accepts only the resource ID":                                       {"cli.context.show_options", "项目上下文 show 仅接受资源标识"},
	"unknown project context command; use knowledge --help or requirement design --help":      {"cli.context.command_unknown", "未知项目上下文命令，请查看 knowledge --help 或 requirement design --help"},
	"requirement design requires an action and requirement ID; use requirement design --help": {"cli.design.action_required", "requirement design 需要操作和需求标识，请查看 requirement design --help"},
	"project context request requires --file JSON_FILE or --file -":                           {"cli.context.file_required", "项目上下文请求需要 --file JSON_FILE 或 --file -"},
	"webhook requires deliver; see --help":                                                    {"cli.webhook.command_required", "webhook 需要 deliver，请查看 --help"},
	"webhook deliver requires --ingress (32 hex), --delivery (8..128 safe characters), --file, and a valid --secret-env name": {"cli.webhook.options_invalid", "webhook deliver 需要 --ingress（32 位十六进制）、--delivery（8–128 个安全字符）、--file 和有效的 --secret-env 名称"},
	"signed delivery requires HTTPS except for loopback development":                                                          {"cli.webhook.https_required", "签名投递需要 HTTPS，本机回环开发除外"},
	"signing key environment must contain 64 lowercase hex characters; do not pass keys in arguments":                         {"cli.webhook.key_invalid", "签名密钥环境变量必须包含 64 位小写十六进制字符，不要通过命令参数传递密钥"},
	"unable to open event JSON file": {"cli.webhook.file_unreadable", "无法打开事件 JSON 文件"},
	"event must be one UTF-8 JSON object at most 2 MiB; exact bytes are retained for signing": {"cli.webhook.body_invalid", "事件必须是至多 2 MiB 的单个 UTF-8 JSON 对象，签名保留原始字节"},
	"invalid ingress request": {"cli.webhook.request_invalid", "接入请求无效"},
	"signed delivery outcome may be unknown; retry the same ingress, delivery ID and exact body; timestamp and signature will be refreshed": {"cli.webhook.outcome_unknown", "签名投递结果可能未知；请使用相同接入点、投递标识和原始正文重试，时间戳与签名会刷新"},
	"signed delivery rejected; inspect the ingress configuration and delivery history before retrying":                                      {"cli.webhook.rejected", "签名投递被拒绝；重试前请检查接入配置和投递历史"},
	"delivery receipt unavailable; outcome may be unknown; retry the same delivery ID and exact body":                                       {"cli.webhook.receipt_unavailable", "投递回执不可用，结果可能未知；请使用相同投递标识和原始正文重试"},
	"invalid delivery receipt; outcome may be unknown; retry the same delivery ID and exact body":                                           {"cli.webhook.receipt_invalid", "投递回执无效，结果可能未知；请使用相同投递标识和原始正文重试"},
	"invalid list response":                     {"cli.tui.list_invalid", "列表响应无效"},
	"terminal session ended or could not start": {"cli.tui.failed", "终端会话已结束或无法启动"},
	taskBatchUsage:                              {"cli.task_batch.action", "task-priority 与 task-status 的批量操作需要 preview 或 apply 及 --file JSON_FILE|-，请查看 --help"},
	taskBatchInputInvalid:                       {"cli.task_batch.invalid", "任务批量请求需要准确项目、1～16 个不同任务 ID 和整数版本、所需目标值，以及提交的原操作编号"},
	taskBatchPreviewInvalid:                     {"cli.task_batch.preview_invalid", "任务批量预览与所选项目、任务或版本不一致"},
	taskBatchUnconfirmed:                        {"cli.task_batch.unconfirmed", "任务批量回执尚未确认，请仅使用相同操作编号和请求正文重试"},
	taskOwnersUsage:                             {"cli.task_owners.action", "task-owners 需要 targets、preview 或 apply，请查看 --help"},
	taskOwnersInvalid:                           {"cli.task_owners.invalid", "负责人变更请求需要准确任务和人员版本；采用还需要预览摘要、明确确认及原操作编号"},
	"handoffs requires list, get, create, accept, return, or withdraw; use handoffs --help":                          {"cli.handoff.action_required", "handoffs 需要 list、get、create、accept、return 或 withdraw，请查看 handoffs --help"},
	"handoff get requires ID and --task":                                                                             {"cli.handoff.get_options", "handoff get 需要交接 ID 和 --task"},
	"invalid handoff list options; provide --task and limit 1..100":                                                  {"cli.handoff.list_options", "交接列表参数无效，请提供 --task，limit 须为 1–100"},
	"handoff create requires --task, positive --version, --to-resource, --note, and --operation (8..128 characters)": {"cli.handoff.create_options", "发起交接需要 --task、正整数 --version、--to-resource、--note 及 8–128 字符的 --operation"},
	"handoff decision requires ID, positive --version, --reason, and --operation (8..128 characters)":                {"cli.handoff.decision_options", "交接决定需要 ID、正整数 --version、--reason 及 8–128 字符的 --operation"},
	"handoff accept requires explicit --acknowledge-invalidation":                                                    {"cli.handoff.acknowledgement", "接收交接须显式提供 --acknowledge-invalidation，确认旧执行与验收依据失效"},

	runWatchUsage:             {"cli.run.watch_usage", "run watch 需要运行 ID，--after 为 0–9007199254740991，--max-events 为 1–100000"},
	runSchedulingUsage:        {"cli.run.scheduling_usage", "run scheduling 需要运行 ID 和 --node 节点 ID"},
	runAttemptsUsage:          {"cli.run.attempts_usage", "run attempts 需要运行 ID 和 --node 节点 ID，可用 --limit 和原 --cursor 续页"},
	runAttemptUsage:           {"cli.run.attempt_usage", "run attempt 需要运行 ID、--node 节点 ID 和正整数 --attempt"},
	runToolEventsUsage:        {"cli.run.tool_events_usage", "run tool-events 需要运行 ID、--node 节点 ID 和正整数 --attempt，可用 --limit 和原 --cursor 续页"},
	runAttemptResponseInvalid: {"cli.run.attempt_response_invalid", "尝试或工具事件响应无效，或与所选项目、运行、节点、尝试不符"},
	runInputsUsage:            {"cli.run.inputs_usage", "run inputs 需要运行 ID、--node、正整数 --attempt 和 --fence，可用 --workbench-origin 指定工作台地址"},
	runInputsResponseInvalid:  {"cli.run.inputs_response_invalid", "工具问题响应无效，或与所选项目、运行、节点和执行尝试不符"},
	runQueueUsage:             {"cli.run.queue_usage", "run queue 可指定 --route project|pool、--run 运行 ID、1–100 的 --limit 和原 --cursor"},
	runWatchRetryExhausted:    {"cli.run.watch_retry_exhausted", "实时事件重连次数已达上限，可从最后一条已输出事件续传"},
	runWatchOutputFailed:      {"cli.run.watch_output_failed", "无法完整写出运行事件"},
	"live run event response must be a UTF-8 event stream": {"cli.run.watch_content_type", "实时运行事件响应须为 UTF-8 事件流"},
	"invalid live run event frame":                         {"cli.run.watch_frame_invalid", "实时运行事件帧无效"},
	"live run event frame exceeds 16 MiB":                  {"cli.run.watch_frame_limit", "实时运行事件帧超过 16 MiB"},
	"live run event stream reported an error":              {"cli.run.watch_stream_error", "实时运行事件流报告错误"},

	workBuddyProgressListUsage: {"cli.run.workbuddy_list_usage", "run workbuddy-list 可指定一个 1–20 的 --limit 和原 --before 游标"},
	workBuddyProgressUsage:     {"cli.run.workbuddy_status_usage", "run workbuddy-status 需要运行 ID 和一个 --node 节点 ID"},
	workBuddyProgressInvalid:   {"cli.run.workbuddy_status_invalid", "WorkBuddy 进度响应无效或与请求的范围不一致"},
	workBuddyProgressRejected:  {"cli.run.workbuddy_status_rejected", "控制面拒绝读取 WorkBuddy 进度"},
}

func localizeFailure(locale localization.Locale, f *failure) *failure {
	if f == nil || f.remote {
		return f
	}
	if locale != localization.EnUS {
		locale = localization.DefaultLocale
	}
	copy := *f
	source := f.Message
	const uncertain = "; outcome may be unknown: retry only with the same operation key and payload"
	base := strings.TrimSuffix(source, uncertain)
	if message, ok := cliMessages[base]; ok {
		known := true
		copy.MessageKey, copy.Locale, copy.Localized = message.key, locale, &known
		if locale == localization.ZhCN {
			copy.Message = message.zh
		}
		if base != source {
			copy.MessageKey += ".outcome_unknown"
			if locale == localization.ZhCN {
				copy.Message += "；操作结果可能未知：只可使用相同操作标识和请求正文重试"
			}
		}
	}
	return &copy
}

func (c *client) text(zh, en string) string {
	if c.locale == localization.EnUS {
		return en
	}
	return zh
}

func helpForLocale(locale localization.Locale) string {
	if locale == localization.EnUS {
		return strings.Replace(help, "--timeout 15s", "--timeout 15s --language en-US|zh-CN", 1) + projectContextHelp(locale) + handoffsHelp(locale) + skillCatalogHelp(locale) + expertPlansHelp(locale) + failureDiagnosisHelp(locale) + candidateCompositionHelp(locale) + worklogsHelp(locale) + taskCollectionHelp(locale) + sharedTaskViewsHelp(locale) + notificationInboxHelp(locale) + taskDiscussionsHelp(locale) + taskReviewRequestsHelp(locale)
	}
	return helpZH + projectContextHelp(locale) + handoffsHelp(locale) + skillCatalogHelp(locale) + expertPlansHelp(locale) + failureDiagnosisHelp(locale) + candidateCompositionHelp(locale) + worklogsHelp(locale) + taskCollectionHelp(locale) + sharedTaskViewsHelp(locale) + notificationInboxHelp(locale) + taskDiscussionsHelp(locale) + taskReviewRequestsHelp(locale)
}

func projectContextHelp(locale localization.Locale) string {
	if locale == localization.EnUS {
		return contextHelpEN
	}
	return contextHelpZH
}

const contextHelpEN = `
Governed project context (global flags still go before the command):
  knowledge list [--kind memory|spec|skill] [--state draft|published|retired]
                 [--query TEXT] [--limit 1..100] [--cursor NEXT_CURSOR]
  knowledge search [--mode keyword|index] [--query TEXT] [--kind memory|spec|skill] [--limit 1..20]
                   [--cursor NEXT_CURSOR] [--format json|table]
  knowledge show ID
  knowledge history ID [--limit 1..100] [--cursor NEXT_CURSOR]
  knowledge create|extract --file request.json
  knowledge edit|command ID --file request.json
  requirement design show REQUIREMENT_ID
  requirement design history|adoptions REQUIREMENT_ID [--limit 1..100] [--cursor NEXT_CURSOR]
  requirement design save|preview|command|adopt REQUIREMENT_ID --file request.json

Project context and JSON API inputs accept --file - (stdin), at most 4 MiB and valid UTF-8.
history and revisions are aliases; command and commands are aliases.
Draft edits replace the complete draft. save uses PUT. command action is publish or retire.
Project API credentials may edit drafts; publication, retirement and adoption require
a currently authorized browser approver and captured original account/organization.
Preview is read-only POST: no operation key, task changes or reservations.
Review plan_sha256 before explicit adoption; adoption does not start a run.
After an unknown write result, retain the original caller, path, body and operation key.
Never refresh identity or retry automatically. Read current state before the next CAS.
Response snapshots, user text and numeric lexemes stay unchanged across languages.
search discovers eligible publications only; default mode is keyword and output is JSON.
keyword permits browsing without --query and scans at most 200 candidates per page;
an empty page with next_cursor requires explicit continuation through --cursor.
index requires a nonempty --query and forbids --cursor. It reports actual provider mode,
generation, watermark and candidate scope, without automatic keyword fallback.
References are exact id/revision/sha256 values; discovery does not adopt, publish or start a task.
`

const contextHelpZH = `
受治理的项目上下文（全局参数仍放在命令前）：
  knowledge list [--kind memory|spec|skill] [--state draft|published|retired]
                 [--query TEXT] [--limit 1..100] [--cursor NEXT_CURSOR]
  knowledge search [--mode keyword|index] [--query TEXT] [--kind memory|spec|skill] [--limit 1..20]
                   [--cursor NEXT_CURSOR] [--format json|table]
  knowledge show ID
  knowledge history ID [--limit 1..100] [--cursor NEXT_CURSOR]
  knowledge create|extract --file request.json
  knowledge edit|command ID --file request.json
  requirement design show REQUIREMENT_ID
  requirement design history|adoptions REQUIREMENT_ID [--limit 1..100] [--cursor NEXT_CURSOR]
  requirement design save|preview|command|adopt REQUIREMENT_ID --file request.json

项目上下文与 JSON API 输入均支持 --file -（标准输入），最多 4 MiB，须使用有效 UTF-8。
history 与 revisions 同义；command 与 commands 同义。
编辑须完整替换草稿；save 使用 PUT；command 的 action 为 publish 或 retire。
项目 API 凭证可编辑草稿；发布、退役和采用须由当前有批准权限的浏览器身份
执行，并携带操作时原账户与组织上下文。服务端决定当前权限和版本是否有效。
preview 是只读 POST，不添加操作标识、不更改任务、不预留资源。
明确采用前须审阅 plan_sha256；采用不会启动运行。
写入结果未知时，保留原调用者、路径、正文和操作标识，不自动刷新身份或重试。
下一次 CAS 前重新读取当前状态；用户原文、快照和数字字面量不随语言改变。
search 只发现当前可用的已发布知识；默认 keyword 模式，默认输出 JSON。
keyword 模式省略 --query 可浏览，每页最多扫描 200 条候选；
空页若含 next_cursor，须显式通过 --cursor 继续。
index 模式须提供非空 --query 且禁止 --cursor；显示真实提供方模式、代际、水位
和候选范围，不会自动回退关键词检索。
准确引用包含 id、revision 和 sha256；发现不会采用、发布或启动任务。
`

const helpZH = `suma — 速码控制面客户端

全局参数（放在命令前）：--control http://127.0.0.1:4310 --project ID
  --token TOKEN（建议使用 SUMA_TOKEN）--timeout 15s --language zh-CN|en-US
  浏览器会话命令：--expected-user ID [--expected-organization ID]
  使用操作时原页面的身份，重试不自动切换身份；项目 API 凭证无需这些参数。
  语言优先级：--language > SUMA_LANGUAGE > LC_ALL/LC_MESSAGES/LANG > zh-CN

只读命令：
  --version                   输出产品发布版本，不请求 API
  doctor | projects | requirements | tasks | runs | workflows
  task-owners --help          预览并原子变更明确所选任务的负责人
  task-priority --help        预览或提交任务优先级批量变更
  task-status --help          预览或提交任务计划状态批量变更
  task delivery TASK          读取当前项目的任务交付证据
  test-plan --help            查询测试计划列表和详情
  issue-repair show ISSUE     读取缺陷修复及原始用例引用
  run show ID
  run events ID [--after SEQUENCE]
  run watch RUN [--after SEQUENCE] [--max-events 1..100000]
  run scheduling RUN --node NODE
  run attempts RUN --node NODE [--limit 1..100] [--cursor 原游标]
  run attempt RUN --node NODE --attempt 尝试号
  run tool-events RUN --node NODE --attempt 尝试号 [--limit 1..100] [--cursor 原游标]
  run inputs RUN --node NODE --attempt 尝试号 --fence 租约令牌 [--workbench-origin 工作台地址]
  run workbuddy-status RUN --node NODE
  run workbuddy-list [--limit 1..20] [--before NEXT_CURSOR]
  run queue [--route project|pool] [--run RUN] [--limit 1..100] [--cursor CURSOR]
  tasks dispatch-options TASK --expected-version N --workflow ID --workflow-version N --node ID
  tasks recommendation-options TASK --expected-version N --workflow ID --workflow-version N --node ID
       [--max-steps N] [--max-output-tokens N] [--token-limit N] [--require-hard-budget]
  tasks recommendations TASK [--limit 1..100] [--before 原游标]
  tasks recommendation ID
  tasks dispatch-test-option --help   核对准确的独立测试工具
  tasks dispatch-plan PLAN
  models options --tool 工具ID --expected-tool-version 当前版本
  tools list [--state 状态] [--worker 节点ID] [--query 检索文本] [--limit 1..200] [--cursor 原游标]
  tools catalog [--direction all|suma_to_tool|tool_to_suma] [--query 检索文本]
  tools inventory --worker 节点ID [--connect-command 命令ID [--workbench-origin 工作台地址]]
  tools matches --worker 节点ID --command 命令ID
  tools matches --file JSON   使用登录会话查询完整匹配的已有工具档案（只读）
  developer check            验证当前项目的真实MCP连接（只读）
  workspace save-status ID [--wait]
  workspace targets | workspace observe --file 请求JSON [--wait] | workspace observation ID [--wait]
  workspace files [--directory 相对目录] [--limit 100] [--before 上一页游标]
  workspace git | workspace diff --path 相对路径 [--source worktree|index]
  tui                         只读 Bubble Tea 工作台（交互终端默认入口）

显式写操作：
  tasks dispatch-preview --file REQUEST_JSON|-
  tasks dispatch-start PLAN --expected-version N --operation KEY
  workspace save --file request.json [--wait]
  workspace inspect-save ID --expected-version N [--operation KEY] [--wait]
  webhook deliver --ingress ID --delivery ID --file event.json [--secret-env SUMA_WEBHOOK_SECRET]
  run command ID --file command.json
  workflow publish --file workflow.json
  run start --workflow ID --version N --operation KEY [--task ID] [--expected-version N]
  api /RESOURCE [--method GET|POST|PUT|PATCH|DELETE] [--file request.json]

Webhook 密钥只从指定的当前进程环境变量读取；--file - 从标准输入读取。
Webhook 投递保留正文原始字节和投递标识，不发送会话凭证。
JSON API 文件保留 idempotency_key；缺失时生成固定内容标识。
非交互终端的默认入口或 tui 输出 doctor JSON 后退出。
退出码：0 成功，1 API/传输失败，2 用法错误。
`
