export type Locale='zh-CN'|'en-US';
const catalog={
 configuration_invalid:{'zh-CN':'OpenCode 运行时配置无效。','en-US':'OpenCode runtime configuration is invalid.'},
 probe_failed:{'zh-CN':'OpenCode 运行时探测失败。','en-US':'OpenCode runtime probe failed.'},
 request_invalid:{'zh-CN':'OpenCode 运行时请求无效。','en-US':'OpenCode runtime request is invalid.'},
 execute_required:{'zh-CN':'OpenCode 运行时需要一条 execute 请求。','en-US':'OpenCode runtime requires one execute request.'},
 language_invalid:{'zh-CN':'语言参数无效，请使用 zh-CN 或 en-US。','en-US':'Invalid language; use zh-CN or en-US.'}
} as const;
export function display(key:keyof typeof catalog,locale:Locale){return catalog[key][locale]+'\n';}
const normalize=(value:string):Locale|undefined=>/^zh([_-]|$)/i.test(value)?'zh-CN':/^en([_-]|$)/i.test(value)?'en-US':undefined;
export function options(args:string[],env:NodeJS.ProcessEnv){
 let locale:Locale='zh-CN';for(const key of ['SUMA_LANGUAGE','LC_ALL','LC_MESSAGES','LANG']){const value=env[key];if(value){locale=normalize(value)||'zh-CN';break;}}
 const rest:string[]=[];let language=false,error=false;
 for(let i=0;i<args.length;i++){const arg=args[i];if(arg==='--language'||arg.startsWith('--language=')){const value=arg==='--language'?args[++i]:arg.slice(11);const parsed=typeof value==='string'?normalize(value):undefined;if(language||!['zh-CN','en-US'].includes(value)||!parsed)error=true;else locale=parsed;language=true;}else rest.push(arg);}
 return {locale,args:rest,error};
}
export function help(locale:Locale){return 'suma-opencode-runtime 1.0.0\n'+(locale==='en-US'?'Fixed opencode-sdk-v1 JSONL execute/cancel protocol; private OpenCode 1.19.4 server and six authorized workspace tools. Configuration comes only from SUMA_OPENCODE_CONFIG. --probe performs offline configuration, asset and version checks; no prompt or model request.\n--language zh-CN|en-US > SUMA_LANGUAGE > LC_ALL/LC_MESSAGES/LANG > zh-CN.\n':'固定 opencode-sdk-v1 JSONL execute/cancel 协议，使用私有 OpenCode 1.19.4 服务和六项授权工作区工具。配置仅来自 SUMA_OPENCODE_CONFIG。--probe 离线核对配置、制品和版本，不发起提示或模型请求。\n语言优先级：--language zh-CN|en-US > SUMA_LANGUAGE > LC_ALL/LC_MESSAGES/LANG > zh-CN。\n');}
