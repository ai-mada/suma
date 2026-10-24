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
	"--language must be zh-CN or en-US (supported aliases are accepted)":      {"cli.language.invalid", "--language 必须为 zh-CN 或 en-US，也支持已声明的语言别名"},
	"invalid global flags; use --help":                                        {"cli.flags.invalid", "全局参数无效，请查看 --help"},
	"client operation failed":                                                 {"cli.operation.failed", "客户端操作失败"},
	"--control must be an http(s) origin without credentials, query, or path": {"cli.control.invalid", "--control 必须是 http(s) 控制面地址，不能包含凭证、查询参数或路径"},
	"invalid timeout or token":                                                {"cli.options.invalid", "超时时间或凭证无效"},
	"API path must be a relative /api/v1 resource path":                       {"cli.api.path_invalid", "API 路径必须是 /api/v1 下的相对资源路径"},
	"credentials must not be included in API URLs":                            {"cli.api.url_credentials", "API 地址不能包含凭证"},
	"--project is required for this command":                                  {"cli.project.required", "此命令需要 --project"},
	"URL project_id differs from --project":                                   {"cli.project.url_mismatch", "地址中的 project_id 与 --project 不一致"},
	"JSON project_id differs from --project":                                  {"cli.project.body_mismatch", "JSON 中的 project_id 与 --project 不一致"},
	"invalid JSON request":                                                    {"cli.request.json_invalid", "JSON 请求无效"},
	"invalid request":                                                         {"cli.request.invalid", "请求无效"},
	"control request failed":                                                  {"cli.request.failed", "控制面请求失败"},
	"control request cancelled or timed out":                                  {"cli.request.interrupted", "控制面请求已取消或超时"},
	"unable to read bounded response":                                         {"cli.response.unreadable", "无法读取限制大小内的响应"},
	"control response was not valid JSON":                                     {"cli.response.json_invalid", "控制面响应不是有效 JSON"},
	"cannot open JSON file":                                                   {"cli.file.unreadable", "无法打开 JSON 文件"},
	"JSON file exceeds 4 MiB or cannot be read":                               {"cli.file.too_large", "JSON 文件超过 4 MiB 或无法读取"},
	"JSON file must contain an object":                                        {"cli.file.object_required", "JSON 文件必须包含一个对象"},
	"JSON file must contain exactly one object":                               {"cli.file.single_object_required", "JSON 文件必须且只能包含一个对象"},
	"idempotency_key must be a string of at least 8 characters":               {"cli.operation.key_invalid", "idempotency_key 必须是至少 8 个字符的字符串"},
	"invalid operation payload":                                               {"cli.operation.payload_invalid", "操作请求正文无效"},
	"workflow publish requires --file":                                        {"cli.workflow.file_required", "workflow publish 需要 --file"},
	"unknown command or arguments; use --help":                                {"cli.command.unknown", "未知命令或参数，请查看 --help"},
	"run command requires a valid run ID":                                     {"cli.run.id_required", "运行命令需要有效的运行标识"},
	"invalid run flags":                                                       {"cli.run.flags_invalid", "运行参数无效"},
	"run command requires --file with action and expected_version":            {"cli.run.file_required", "run command 需要 --file，文件中须包含 action 和 expected_version"},
	"--file is only supported by run command":                                 {"cli.run.file_unsupported", "只有 run command 支持 --file"},
	"run start requires --workflow, positive --version, and --operation (at least 8 characters)": {"cli.run.start_options", "run start 需要 --workflow、正整数 --version 以及至少 8 个字符的 --operation"},
	"unknown run command": {"cli.run.unknown", "未知运行命令"},
	"command file requires action and positive integer expected_version": {"cli.run.file_invalid", "命令文件需要 action 和正整数 expected_version"},
	"api requires a resource path":                                       {"cli.api.path_required", "api 需要资源路径"},
	"api requires a relative resource path":                              {"cli.api.relative_path_required", "api 需要相对资源路径"},
	"invalid api flags":                                                  {"cli.api.flags_invalid", "api 参数无效"},
	"unsupported API method":                                             {"cli.api.method_unsupported", "不支持此 API 方法"},
	"API mutations require explicit --method and --file":                 {"cli.api.write_options", "API 写操作必须显式提供 --method 和 --file"},
	"GET does not accept --file":                                         {"cli.api.get_file", "GET 不接受 --file"},
	"webhook requires deliver; see --help":                               {"cli.webhook.command_required", "webhook 需要 deliver，请查看 --help"},
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
		return strings.Replace(help, "--timeout 15s", "--timeout 15s --language en-US|zh-CN", 1)
	}
	return helpZH
}

const helpZH = `suma — 速码控制面客户端

全局参数（放在命令前）：--control http://127.0.0.1:4310 --project ID
  --token TOKEN（建议使用 SUMA_TOKEN）--timeout 15s --language zh-CN|en-US
  语言优先级：--language > SUMA_LANGUAGE > LC_ALL/LC_MESSAGES/LANG > zh-CN

只读命令：
  doctor | projects | requirements | tasks | runs | workflows
  run show ID
  run events ID [--after SEQUENCE]
  tui                         只读 Bubble Tea 工作台（交互终端默认入口）

显式写操作：
  webhook deliver --ingress ID --delivery ID --file event.json [--secret-env SUMA_WEBHOOK_SECRET]
  run command ID --file command.json
  workflow publish --file workflow.json
  run start --workflow ID --version N --operation KEY [--task ID] [--expected-version N]
  api /RESOURCE [--method GET|POST|PATCH|DELETE] [--file request.json]

Webhook 密钥只从指定的当前进程环境变量读取；--file - 从标准输入读取。
Webhook 投递保留正文原始字节和投递标识，不发送会话凭证。
JSON API 文件保留 idempotency_key；缺失时生成固定内容标识。
非交互终端的默认入口或 tui 输出 doctor JSON 后退出。
退出码：0 成功，1 API/传输失败，2 用法错误。
`
