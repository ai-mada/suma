// Display metadata is a projection for SDK consumers. The event's original
// message, model/tool payloads and stable protocol facts remain run evidence.
export type NativeLocale='zh-CN'|'en-US';
export interface NativeDisplay {message_key:string;message_params?:Record<string,string|number>;locale:NativeLocale;localized_message:string}
export function normalizeLocale(value:unknown):NativeLocale|undefined{
 if(typeof value!=='string'||Buffer.byteLength(value,'utf8')>128||/\p{Cc}/u.test(value))return undefined;
 try{const locale=new Intl.Locale(value.trim().replaceAll('_','-')).maximize();if(locale.language==='zh'&&locale.script==='Hans')return 'zh-CN';if(locale.language==='en'&&locale.script==='Latn')return 'en-US';}catch{}
 return undefined;
}
export function environmentLocale(environment:NodeJS.ProcessEnv):NativeLocale{
 const configured=normalizeLocale(environment.SUMA_LANGUAGE);if(configured)return configured;
 for(const name of ['LC_ALL','LC_MESSAGES','LANG']){const value=environment[name];if(value?.trim())return normalizeLocale(value.split(/[.@]/,1)[0])??'zh-CN';}
 return 'zh-CN';
}
type Entry={en:string;zh:string;params?:Record<string,'integer'|'text'>};
export const nativeCatalog={
 'native.endpoint.unsafe':{en:'HTTPS or a loopback HTTP endpoint is required',zh:'端点必须使用 HTTPS 或本机回环 HTTP 地址'},
 'native.endpoint.rejected':{en:'Endpoint rejected request (HTTP {{status}})',zh:'端点拒绝请求（HTTP {{status}}）',params:{status:'integer'}},
 'native.response.empty':{en:'Empty upstream response',zh:'上游响应为空'},
 'native.response.too_large':{en:'Upstream response exceeded 2 MiB',zh:'上游响应超过 2 MiB'},
 'native.response.invalid_json':{en:'Upstream response is not JSON',zh:'上游响应不是有效 JSON'},
 'native.response.object_required':{en:'Expected an object',zh:'响应必须为对象'},
 'native.request.invalid_limits':{en:'Explicit max_steps/max_tokens must be positive integers within the supported bounds',zh:'显式 max_steps/max_tokens 必须是支持范围内的正整数'},
 'native.usage.invalid_counter':{en:'Model usage counters must be nonnegative safe integers',zh:'模型用量计数必须是非负安全整数'},
 'native.usage.lossless_unavailable':{en:'Runtime cannot verify exact usage counters; Node22 JSON source context is required',zh:'运行时无法验证精确用量计数，需要 Node22 JSON 原文上下文支持'},
 'native.usage.integer_literal_required':{en:'Model usage counters must use exact integer literals',zh:'模型用量计数必须使用精确整数字面量'},
 'native.usage.conflicting_counters':{en:'Conflicting model usage counters',zh:'模型用量计数互相冲突'},
 'native.tools.too_many':{en:'Too many tools',zh:'工具数量超过上限'},
 'native.tools.invalid_name':{en:'Invalid tool name',zh:'工具名称无效'},
 'native.tools.duplicate_names':{en:'Control service returned duplicate tool names',zh:'控制服务返回了重复的工具名称'},
 'native.tools.alias_collision':{en:'Provider tool alias collision',zh:'提供方工具别名冲突'},
 'native.request.invalid_goal':{en:'A goal of 1–32000 characters is required',zh:'目标必须包含 1–32000 个字符'},
 'native.config.missing_model':{en:'Configure SUMA_MODEL and SUMA_MODEL_BASE_URL to run an agent',zh:'运行智能体前请配置 SUMA_MODEL 和 SUMA_MODEL_BASE_URL'},
 'native.config.missing_provider':{en:'Configure SUMA_MODEL_BASE_URL for an OpenAI-compatible provider',zh:'请配置兼容 OpenAI 的提供方地址 SUMA_MODEL_BASE_URL'},
 'native.config.missing_control_auth':{en:'Control token and project are required for tools',zh:'使用工具需要控制服务凭证和项目标识'},
 'native.config.invalid_locale':{en:'Unsupported explicit native display language',zh:'原生显示语言必须为 zh-CN 或 en-US，也支持已声明的语言别名'},
 'native.tools.catalog_missing':{en:'Control service did not return a tool catalog',zh:'控制服务未返回工具目录'},
 'native.limit.output_allowance':{en:'Agent output allowance exhausted',zh:'智能体输出额度已用尽'},
 'native.usage.allowance_exceeded':{en:'Provider reported output beyond the reserved request allowance',zh:'提供方报告的输出量超过该请求的预留额度'},
 'native.response.no_choices':{en:'Model returned no choices',zh:'模型未返回候选结果'},
 'native.limit.output_truncated':{en:'Model output was truncated',zh:'模型输出被截断'},
 'native.response.no_text':{en:'Model returned no text',zh:'模型未返回文本'},
 'native.limit.tool_calls':{en:'At most eight calls are allowed per step',zh:'每一步最多允许八次工具调用'},
 'native.tools.malformed_call':{en:'Malformed tool call',zh:'工具调用格式无效'},
 'native.tools.unknown_tool':{en:'Model requested a tool outside the authenticated catalog',zh:'模型请求了认证目录以外的工具'},
 'native.tools.invalid_arguments':{en:'Tool arguments must be a JSON object',zh:'工具参数必须是 JSON 对象'},
 'native.tools.output_too_large':{en:'Tool result exceeded 64 KiB',zh:'工具结果超过 64 KiB'},
 'native.limit.steps':{en:'Agent reached the configured step limit',zh:'智能体已达到配置的步骤上限'},
 'native.run.aborted':{en:'Agent aborted or timed out',zh:'智能体已取消或超时'},
 'native.run.failed':{en:'Agent request failed; inspect provider and control configuration',zh:'智能体请求失败，请检查提供方与控制服务配置'},
 'native.cli.request_limit':{en:'Request exceeded 64 KiB',zh:'请求超过 64 KiB'},
 'native.cli.invalid_json':{en:'Request must be a JSON object',zh:'请求必须是有效的 JSON 对象'},
 'native.cli.busy':{en:'An agent request is already running',zh:'已有智能体请求正在运行'},
 'native.cli.invalid_language':{en:'--language must be zh-CN or en-US (supported aliases are accepted)',zh:'--language 必须为 zh-CN 或 en-US，也支持已声明的语言别名'},
 'native.cli.invalid_arguments':{en:'Unsupported arguments; use --help.',zh:'不支持的参数，请查看 --help。'},
 'native.catalog.not_found':{en:'CLI not found on PATH',zh:'在 PATH 中未找到 CLI'},
 'native.catalog.requirements_mismatch':{en:'Installed CLI help does not match adapter requirements',zh:'已安装 CLI 的帮助信息不符合适配器要求'},
 'native.catalog.shell_wrapper':{en:'Shell wrapper is not probed automatically',zh:'不会自动探测 Shell 包装脚本'},
 'native.catalog.probe_timeout':{en:'CLI probe timed out',zh:'CLI 探测超时'},
 'native.catalog.probe_output_limit':{en:'CLI probe exceeded output limit',zh:'CLI 探测输出超过上限'},
 'native.catalog.probe_exited':{en:'CLI probe exited {{status}}',zh:'CLI 探测退出，状态码 {{status}}',params:{status:'text'}},
 'native.catalog.probe_failed':{en:'CLI probe failed; inspect the original reason',zh:'CLI 探测失败，请检查原始原因'},
} as const satisfies Record<string,Entry>;
export type NativeMessageKey=keyof typeof nativeCatalog;
// Only declared parameters enter display metadata. Invalid data cannot replace
// original evidence or be echoed via a fallback translation.
export function nativeDisplay(key:NativeMessageKey,locale:unknown,params:Record<string,unknown>={}):NativeDisplay|undefined{
 if(!Object.hasOwn(nativeCatalog,key)||!params||typeof params!=='object'||Array.isArray(params))return undefined;
 const entry:Entry=nativeCatalog[key];const schema=entry.params??{};
 if(Object.keys(params).length!==Object.keys(schema).length)return undefined;
 const snapshot:Record<string,string|number>={};
 for(const [name,type] of Object.entries(schema)){if(!Object.hasOwn(params,name))return undefined;const value=params[name];if(type==='integer'){if(typeof value!=='number'||!Number.isSafeInteger(value)||value<0)return undefined;}else if(typeof value!=='string'||!value||Buffer.byteLength(value,'utf8')>512||/\p{Cc}/u.test(value))return undefined;snapshot[name]=value as string|number;}
 const resolved=normalizeLocale(locale)??'zh-CN';const template=resolved==='en-US'?entry.en:entry.zh;
 return {message_key:key,...(Object.keys(snapshot).length?{message_params:snapshot}:{}),locale:resolved,localized_message:template.replace(/\{\{([a-z]+)\}\}/g,(_match,name:string)=>String(snapshot[name]))};
}

// SDK configuration is checked before a run emits evidence or the catalog
// reads PATH entries. Only absence means default; invalid explicit input never
// silently enables work under a different display language.
export class NativeLocaleError extends RangeError{
 readonly code='invalid_locale';
 readonly display=nativeDisplay('native.config.invalid_locale','zh-CN')!;
 constructor(){super('Unsupported explicit native display language');this.name='NativeLocaleError';}
}
export function requireNativeLocale(value:unknown):NativeLocale{
 if(value===undefined)return 'zh-CN';const locale=normalizeLocale(value);if(!locale)throw new NativeLocaleError();return locale;
}

export function nativeHelp(locale:NativeLocale,catalog=false):string{
 if(catalog)return locale==='en-US'?'Suma adapter catalog: read-only version/help probes; JSON on stdout, execution remains disabled.\n--language zh-CN|en-US selects display metadata; original reason and version remain unchanged.\n':'速码适配器目录：只读探测版本与帮助，标准输出为 JSON，执行保持禁用。\n--language zh-CN|en-US 选择显示元数据语言；原始原因与版本保持不变。\n';
 return locale==='en-US'?'Suma agent runtime: newline-delimited JSONL requests on stdin and bounded JSONL events on stdout.\nConfigure SUMA_MODEL, SUMA_MODEL_BASE_URL and optional SUMA_MODEL_API_KEY.\nProject tools require SUMA_CONTROL_URL, SUMA_PROJECT_ID and a separate agent-scope SUMA_AGENT_TOKEN. Runner credentials are never used for agent tools.\n--language zh-CN|en-US > SUMA_LANGUAGE > LC_ALL/LC_MESSAGES/LANG > zh-CN.\n--version / --help perform read-only probes without a model request.\n':'速码智能体运行时：从标准输入读取逐行 JSONL 请求，向标准输出写入有界 JSONL 事件。\n请配置 SUMA_MODEL、SUMA_MODEL_BASE_URL，可选 SUMA_MODEL_API_KEY。\n项目工具需要 SUMA_CONTROL_URL、SUMA_PROJECT_ID 和独立的智能体范围凭证 SUMA_AGENT_TOKEN。Runner 凭证不会用于智能体工具。\n语言优先级：--language zh-CN|en-US > SUMA_LANGUAGE > LC_ALL/LC_MESSAGES/LANG > zh-CN。\n--version / --help 为只读探测，不发起模型请求。\n';
}
export function nativeOptions(args:string[],environment:NodeJS.ProcessEnv):{locale:NativeLocale;action:'run'|'help'|'version';error?:NativeMessageKey}{
 let locale=environmentLocale(environment);let action:'run'|'help'|'version'='run';let languageSet=false;
 for(let i=0;i<args.length;i++){
  const arg=args[i];
  if(arg==='--language'||arg.startsWith('--language=')){const value=arg==='--language'?args[++i]:arg.slice('--language='.length);const normalized=normalizeLocale(value);if(languageSet||!normalized)return {locale,action:'run',error:'native.cli.invalid_language'};locale=normalized;languageSet=true;continue;}
  if((arg==='--help'||arg==='--version')&&action==='run'){action=arg==='--help'?'help':'version';continue;}
  return {locale,action:'run',error:'native.cli.invalid_arguments'};
 }
 return {locale,action};
}
