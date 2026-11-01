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
	toolProfilesUsage:             {"cli.tool_profiles.usage", "tools 需要 list、doctor 或 matches，请查看 tools --help"},
	toolListUsage:                 {"cli.tool_profiles.list_usage", "tools list 需要 --project；--state 为 active、enabled、paused、revoked 或 all；检索文本最多 120 个 UTF-8 字节，--limit 为 1–200，翻页使用原 --cursor；请查看 tools --help"},
	toolListResponseInvalid:       {"cli.tool_profiles.list_response", "工具列表响应必须保留原项目、状态、完整档案及分页边界"},
	toolDoctorUsage:               {"cli.tool_profiles.doctor_usage", "tools doctor 需要 --id 档案ID 和 --project；请查看 tools --help"},
	toolDoctorResponseInvalid:     {"cli.tool_profiles.doctor_response", "工具体检响应必须保留请求项目、准确完整档案、四项检查及最多 60 秒的有效展示期限"},
	toolMatchesUsage:              {"cli.tool_profiles.matches_usage", "tools matches 需要 --file 请求JSON 或 --file -；请查看 tools --help"},
	toolMatchesInvalid:            {"cli.tool_profiles.matches_input", "工具匹配查询需要当前项目、worker_id 和完整准确的 profile"},
	toolMatchesResponseInvalid:    {"cli.tool_profiles.matches_response", "工具匹配响应必须保留原项目、节点、完整档案及完整结果数量"},
	worklogCLIUsage:               {"cli.worklog.action_required", "worklogs 需要 list、get、create、correct、void 或 history，请查看 worklogs --help"},
	worklogCLIListInvalid:         {"cli.worklog.list_options", "工时列表须提供 --from-date 与 --to-date（YYYY-MM-DD，含首尾最多 31 天）、有效筛选参数及 1–50 的 --limit"},
	worklogCLIReadInvalid:         {"cli.worklog.read_options", "读取／历史需要有效工时 ID；历史支持 1–50 的 --limit 与 --cursor"},
	worklogCLICreateInvalid:       {"cli.worklog.create_options", "人工工时新建须提供 --task、正整数 --task-version、--member、--date YYYY-MM-DD、1–1440 的整数 --minutes 及 8–128 个 UTF-8 字节的 --operation；--note 最多 200 字"},
	worklogCLICorrectInvalid:      {"cli.worklog.correct_options", "人工工时更正须提供 ID、正整数 --version 与 --task-version、--task、--date YYYY-MM-DD、1–1440 的整数 --minutes、1–200 字的 --reason 及 8–128 个 UTF-8 字节的 --operation；--note 最多 200 字"},
	worklogCLIVoidInvalid:         {"cli.worklog.void_options", "人工工时作废须提供 ID、正整数 --version、1–200 字的 --reason 及 8–128 个 UTF-8 字节的 --operation"},
	workspaceTreeCLIUsage:         {"cli.workspace_tree.usage", "目录浏览命令：workspace files [--directory 相对目录] [--limit 1..200] [--before 上一页游标]"},
	compositionCLIUsage:           {"cli.candidate_composition.action_required", "candidate-compositions 需要 options、list、get、conflict、revision-context、create、repair、revise-repair、start 或 command；task-code-baseline 需要 get 或 bind，请查看 --help"},
	compositionCLIRevisionInvalid: {"cli.candidate_composition.revision_basis_required", "修复计划修订需要准确计划、任务、冲突和所选流程／工具版本、明确确认、理由及原操作标识"},
	modelOptionsUsage:             {"cli.model_execution.usage", "模型选项需要：models options --tool 工具ID --expected-tool-version 当前版本"},
	dispatchOptionsCLIUsage:       {"cli.task_dispatch_options.usage", "任务工具推荐需要：tasks dispatch-options TASK --expected-version N --workflow ID --workflow-version N --node ID；可选 --limit 1..200 与 --before 原分页游标"},
	sharedTaskViewUsage:           {"cli.shared_task_views.usage", "task-views 需要 list、get ID、create --file JSON、update ID --file JSON 或 archive ID --file JSON，并提供 --project"},
	taskDiscussionsCLIUsage:       {"cli.task_discussions.usage", "任务讨论需要 list --task、people --task、message 消息ID --task 或 post --file JSON|-；请查看 discussion --help"},
	taskDiscussionsCLIInput:       {"cli.task_discussions.input", "讨论发送须保留与 --project 一致的 project_id、任务ID、正整数任务版本、非空原正文和明确原操作标识；可选唯一人员资源ID或准确知识引用，不能指定作者或角色"},
	inboxCLIUsage:                 {"cli.notification_inbox.usage", "个人收件箱命令：inbox list 支持 --filter inbox、unread 或 archived、--limit 1–50 与 --cursor；inbox read／unread／archive／restore 需要通知ID、--version 原版本与明确 --operation 原操作标识；均须提供 --project"},
	taskCollectionCLIUsage:        {"cli.task_collection.usage", "任务集合参数无效：通用参数为 --q、--status 或 --status-unset、--owner 或 --unassigned、--priority 或 --priority-unset、--requirement；tasks list 接受 --sort、--limit 1–100 与 --cursor 原游标；tasks groups 接受 --group-by status、owner 或 priority、--group-limit 1–100 与 --group-cursor 原分组游标"},
	workspaceWriteUsage:           {"cli.workspace_save.usage", "工作区保存需要：save --file JSON [--wait]、save-status ID [--wait] 或 inspect-save ID --expected-version 版本 [--operation 原操作标识] [--wait]"},
	workspaceWriteUnconfirmed:     {"cli.workspace_save.unconfirmed", "尚未确认文件已保存；再次保存前请核对原保存命令"},
	workspaceObservationUsage:     {"cli.workspace_observation.usage", "远程工作区命令：workspace targets；workspace observe --file 请求JSON [--wait]；workspace observation ID [--wait]"},
	workspaceGitCLIUsage:          {"cli.workspace_git.usage", "工作区只读命令：workspace git；或 workspace diff --path 相对路径 [--source worktree|index]"},
	compositionCLIReadInvalid:     {"cli.candidate_composition.read_invalid", "候选合成读取参数无效，请提供有效任务或计划标识及 1–100 的 limit"},
	compositionCLIWriteOptions:    {"cli.candidate_composition.write_options", "候选合成与代码基线写入需要 --file JSON_FILE 或 --file -，并明确提供 --confirm"},
	compositionCLIRepairInvalid:   {"cli.candidate_composition.repair_basis_required", "冲突修复需要准确冲突依据、类型化修复任务、流程和工具版本及稳定操作标识"},
	compositionCLIWriteInvalid:    {"cli.candidate_composition.basis_required", "候选合成需要类型化 JSON、精确整数版本、按序 2–16 个不同来源、明确确认及稳定的 8–128 字符 idempotency_key"},
	diagnosisCLIUsage:             {"cli.failure_diagnosis.action_required", "failure-diagnosis 需要 context、list、get、create 或 command，请查看 failure-diagnosis --help"},
	diagnosisCLIReadInvalid:       {"cli.failure_diagnosis.read_invalid", "失败分诊读取参数无效，请提供 --run、--node、规范正整数 --attempt 及 1–100 的 limit"},
	diagnosisCLIWriteOptions:      {"cli.failure_diagnosis.write_options", "失败分诊写入需要 --file JSON_FILE 或 --file -，并明确提供 --confirm"},
	diagnosisCLIWriteInvalid:      {"cli.failure_diagnosis.basis_required", "失败分诊需要类型化 JSON、精确整数版本、固定来源 SHA256、明确确认和稳定的 8–128 字符 idempotency_key"},
	diagnosisCLIBrowserRequired:   {"cli.failure_diagnosis.browser_required", "失败分诊写入需要既有浏览器会话凭据和原 --expected-user；不支持 API 凭据写入，视图请求头不授予权限"},
	expertPlanUsage:               {"cli.team_plan.action_required", "team-plan 需要 list、get、revisions、create、start 或 command，请查看 team-plan --help"},
	expertPlanReadInvalid:         {"cli.team_plan.read_invalid", "交付计划读取参数无效，请提供有效标识、修订号及 1–100 的 limit"},
	expertPlanWriteOptions:        {"cli.team_plan.write_options", "交付计划写入需要 --file JSON_FILE 或 --file -，并明确提供 --confirm"},
	expertPlanWriteInvalid:        {"cli.team_plan.basis_required", "交付计划需要精确正整数版本、类型化任务、明确确认及稳定的 8–128 字符 idempotency_key"},
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
	"knowledge requires list, show, history, create, edit, extract, or command; use knowledge --help":                         {"cli.knowledge.action_required", "knowledge 需要 list、show、history、create、edit、extract 或 command，请查看 knowledge --help"},
	"invalid knowledge filters or pagination; limit must be 1..100":                                                           {"cli.context.query_invalid", "知识筛选或分页参数无效，limit 必须为 1–100"},
	"project context command requires a valid resource ID":                                                                    {"cli.context.id_required", "项目上下文命令需要有效的资源标识"},
	"project context show accepts only the resource ID":                                                                       {"cli.context.show_options", "项目上下文 show 仅接受资源标识"},
	"unknown project context command; use knowledge --help or requirement design --help":                                      {"cli.context.command_unknown", "未知项目上下文命令，请查看 knowledge --help 或 requirement design --help"},
	"requirement design requires an action and requirement ID; use requirement design --help":                                 {"cli.design.action_required", "requirement design 需要操作和需求标识，请查看 requirement design --help"},
	"project context request requires --file JSON_FILE or --file -":                                                           {"cli.context.file_required", "项目上下文请求需要 --file JSON_FILE 或 --file -"},
	"webhook requires deliver; see --help":                                                                                    {"cli.webhook.command_required", "webhook 需要 deliver，请查看 --help"},
	"webhook deliver requires --ingress (32 hex), --delivery (8..128 safe characters), --file, and a valid --secret-env name": {"cli.webhook.options_invalid", "webhook deliver 需要 --ingress（32 位十六进制）、--delivery（8–128 个安全字符）、--file 和有效的 --secret-env 名称"},
	"signed delivery requires HTTPS except for loopback development":                                                          {"cli.webhook.https_required", "签名投递需要 HTTPS，本机回环开发除外"},
	"signing key environment must contain 64 lowercase hex characters; do not pass keys in arguments":                         {"cli.webhook.key_invalid", "签名密钥环境变量必须包含 64 位小写十六进制字符，不要通过命令参数传递密钥"},
	"unable to open event JSON file":                                                                                          {"cli.webhook.file_unreadable", "无法打开事件 JSON 文件"},
	"event must be one UTF-8 JSON object at most 2 MiB; exact bytes are retained for signing":                                 {"cli.webhook.body_invalid", "事件必须是至多 2 MiB 的单个 UTF-8 JSON 对象，签名保留原始字节"},
	"invalid ingress request": {"cli.webhook.request_invalid", "接入请求无效"},
	"signed delivery outcome may be unknown; retry the same ingress, delivery ID and exact body; timestamp and signature will be refreshed": {"cli.webhook.outcome_unknown", "签名投递结果可能未知；请使用相同接入点、投递标识和原始正文重试，时间戳与签名会刷新"},
	"signed delivery rejected; inspect the ingress configuration and delivery history before retrying":                                      {"cli.webhook.rejected", "签名投递被拒绝；重试前请检查接入配置和投递历史"},
	"delivery receipt unavailable; outcome may be unknown; retry the same delivery ID and exact body":                                       {"cli.webhook.receipt_unavailable", "投递回执不可用，结果可能未知；请使用相同投递标识和原始正文重试"},
	"invalid delivery receipt; outcome may be unknown; retry the same delivery ID and exact body":                                           {"cli.webhook.receipt_invalid", "投递回执无效，结果可能未知；请使用相同投递标识和原始正文重试"},
	"invalid list response":                     {"cli.tui.list_invalid", "列表响应无效"},
	"terminal session ended or could not start": {"cli.tui.failed", "终端会话已结束或无法启动"},
	taskOwnersUsage:                             {"cli.task_owners.action", "task-owners 需要 targets、preview 或 apply，请查看 --help"},
	taskOwnersInvalid:                           {"cli.task_owners.invalid", "负责人变更请求需要准确任务和人员版本；采用还需要预览摘要、明确确认及原操作编号"},
	"handoffs requires list, get, create, accept, return, or withdraw; use handoffs --help":                          {"cli.handoff.action_required", "handoffs 需要 list、get、create、accept、return 或 withdraw，请查看 handoffs --help"},
	"handoff get requires ID and --task":                                                                             {"cli.handoff.get_options", "handoff get 需要交接 ID 和 --task"},
	"invalid handoff list options; provide --task and limit 1..100":                                                  {"cli.handoff.list_options", "交接列表参数无效，请提供 --task，limit 须为 1–100"},
	"handoff create requires --task, positive --version, --to-resource, --note, and --operation (8..128 characters)": {"cli.handoff.create_options", "发起交接需要 --task、正整数 --version、--to-resource、--note 及 8–128 字符的 --operation"},
	"handoff decision requires ID, positive --version, --reason, and --operation (8..128 characters)":                {"cli.handoff.decision_options", "交接决定需要 ID、正整数 --version、--reason 及 8–128 字符的 --operation"},
	"handoff accept requires explicit --acknowledge-invalidation":                                                    {"cli.handoff.acknowledgement", "接收交接须显式提供 --acknowledge-invalidation，确认旧执行与验收依据失效"},
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
`

const contextHelpZH = `
受治理的项目上下文（全局参数仍放在命令前）：
  knowledge list [--kind memory|spec|skill] [--state draft|published|retired]
                 [--query TEXT] [--limit 1..100] [--cursor NEXT_CURSOR]
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
  run show ID
  run events ID [--after SEQUENCE]
  tasks dispatch-options TASK --expected-version N --workflow ID --workflow-version N --node ID
  models options --tool 工具ID --expected-tool-version 当前版本
  tools list [--state 状态] [--query 检索文本] [--limit 1..200] [--cursor 原游标]
  tools matches --file JSON   使用登录会话查询完整匹配的已有工具档案（只读）
  developer check            验证当前项目的真实MCP连接（只读）
  workspace save-status ID [--wait]
  workspace targets | workspace observe --file 请求JSON [--wait] | workspace observation ID [--wait]
  workspace files [--directory 相对目录] [--limit 100] [--before 上一页游标]
  workspace git | workspace diff --path 相对路径 [--source worktree|index]
  tui                         只读 Bubble Tea 工作台（交互终端默认入口）

显式写操作：
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
