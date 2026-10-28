import {nativeCatalog,type NativeCatalogEntry,type NativeMessageKey} from './native-catalog.js';
export {nativeCatalog,type NativeMessageKey} from './native-catalog.js';
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
// Only declared parameters enter display metadata. Invalid data cannot replace
// original evidence or be echoed via a fallback translation.
export function nativeDisplay(key:NativeMessageKey,locale:unknown,params:Record<string,unknown>={}):NativeDisplay|undefined{
 if(!Object.hasOwn(nativeCatalog,key)||!params||typeof params!=='object'||Array.isArray(params))return undefined;
 const entry:NativeCatalogEntry=nativeCatalog[key];const schema=entry.params??{};
 if(Object.keys(params).length!==Object.keys(schema).length)return undefined;
 const snapshot:Record<string,string|number>={};
 for(const [name,type] of Object.entries(schema)){if(!Object.hasOwn(params,name))return undefined;const value=params[name];if(type==='integer'){if(typeof value!=='number'||!Number.isSafeInteger(value)||value<0)return undefined;}else if(typeof value!=='string'||!value||/\p{Cs}/u.test(value)||Buffer.byteLength(value,'utf8')>512||/\p{Cc}/u.test(value))return undefined;snapshot[name]=value as string|number;}
 const resolved=normalizeLocale(locale)??'zh-CN';const template=resolved==='en-US'?entry.en:entry.zh;
 return {message_key:key,...(Object.keys(snapshot).length?{message_params:snapshot}:{}),locale:resolved,localized_message:template.replace(/\{\{\.([a-z][a-z0-9_]*)\}\}/g,(_match,name:string)=>String(snapshot[name]))};
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
 const planning=locale==='en-US'?'--planning-probe reports the fixed model and endpoint origin as JSON without a model request.\n':'--planning-probe 以 JSON 报告固定模型与端点来源，不发起模型请求。\n';
 const extraction=locale==='en-US'?'--knowledge-extraction-probe reports the dedicated text-only model identity as offline JSON.\nKnowledge extraction accepts only frozen source input, disables all tools and allows one model request.\n':'--knowledge-extraction-probe 以离线 JSON 报告专用文本提炼模型标识。\n知识提炼仅接收固定来源输入，禁用全部工具，每次最多请求模型一次。\n';
 const operations=locale==='en-US'?'--intelligent-operations-probe reports a fixed, text-only draft planner as offline JSON.\nIntelligent operations request the model once with no tools; applying local drafts requires a separately reviewed platform preview.\n':'--intelligent-operations-probe 以离线 JSON 报告固定的文本草稿规划能力。\n智能操作每次仅请求模型一次且禁用工具；采用本地草稿需要另行审阅平台预览。\n';
 const diagnosis=locale==='en-US'?'--failure-diagnosis-probe reports the dedicated fixed model identity as offline JSON.\nFailure diagnosis uses only frozen failure evidence, disables all tools and allows one model request; repair proposals require human review.\n':'--failure-diagnosis-probe 以离线 JSON 报告专用固定模型标识。\n失败分诊只使用固定失败证据，禁用全部工具，每次最多请求模型一次；修复提案需要人工审核。\n';
 return (locale==='en-US'?'Suma agent runtime: newline-delimited JSONL requests on stdin and bounded JSONL events on stdout.\nConfigure SUMA_MODEL, SUMA_MODEL_BASE_URL and optional SUMA_MODEL_API_KEY.\nProject tools require SUMA_CONTROL_URL, SUMA_PROJECT_ID and a separate agent-scope SUMA_AGENT_TOKEN. Runner credentials are never used for agent tools.\n--language zh-CN|en-US > SUMA_LANGUAGE > LC_ALL/LC_MESSAGES/LANG > zh-CN.\n--version / --help perform read-only probes without a model request.\n':'速码智能体运行时：从标准输入读取逐行 JSONL 请求，向标准输出写入有界 JSONL 事件。\n请配置 SUMA_MODEL、SUMA_MODEL_BASE_URL，可选 SUMA_MODEL_API_KEY。\n项目工具需要 SUMA_CONTROL_URL、SUMA_PROJECT_ID 和独立的智能体范围凭证 SUMA_AGENT_TOKEN。Runner 凭证不会用于智能体工具。\n语言优先级：--language zh-CN|en-US > SUMA_LANGUAGE > LC_ALL/LC_MESSAGES/LANG > zh-CN。\n--version / --help 为只读探测，不发起模型请求。\n')+planning+extraction+diagnosis+operations;
}
export function nativeOptions(args:string[],environment:NodeJS.ProcessEnv):{locale:NativeLocale;action:'run'|'help'|'version'|'planning-probe'|'knowledge-extraction-probe'|'failure-diagnosis-probe'|'intelligent-operations-probe';error?:NativeMessageKey}{
 let locale=environmentLocale(environment);let action:'run'|'help'|'version'|'planning-probe'|'knowledge-extraction-probe'|'failure-diagnosis-probe'|'intelligent-operations-probe'='run';let languageSet=false;
 for(let i=0;i<args.length;i++){
  const arg=args[i];
  if(arg==='--language'||arg.startsWith('--language=')){const value=arg==='--language'?args[++i]:arg.slice('--language='.length);const normalized=normalizeLocale(value);if(languageSet||!normalized)return {locale,action:'run',error:'native.cli.invalid_language'};locale=normalized;languageSet=true;continue;}
  if((arg==='--help'||arg==='--version')&&action==='run'){action=arg==='--help'?'help':'version';continue;}
  if(arg==='--planning-probe'&&action==='run'){action='planning-probe';continue;}
  if(arg==='--knowledge-extraction-probe'&&action==='run'){action='knowledge-extraction-probe';continue;}
  if(arg==='--intelligent-operations-probe'&&action==='run'){action='intelligent-operations-probe';continue;}
  if(arg==='--failure-diagnosis-probe'&&action==='run'){action='failure-diagnosis-probe';continue;}
  return {locale,action:'run',error:'native.cli.invalid_arguments'};
 }
 return {locale,action};
}
