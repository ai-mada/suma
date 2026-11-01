export type Locale='zh-CN'|'en-US';
const catalogs={
 'zh-CN':{help:'速码 Claude SDK 宿主 1.0.0\n由速码执行节点启动，通过受控标准输入执行一次固定任务。\n--version 显示宿主版本\n--help 显示帮助\n',arguments_invalid:'宿主启动参数无效。',configuration_invalid:'本机固定配置无效。',request_invalid:'原执行请求无效；请核对原操作。',locale_invalid:'语言配置无效。'},
 'en-US':{help:'suma Claude SDK host 1.0.0\nStarted by a suma Runner for one fixed task over controlled stdin.\n--version Show the host version\n--help Show help\n',arguments_invalid:'Invalid host arguments.',configuration_invalid:'The fixed local configuration is invalid.',request_invalid:'Invalid original execution request; reconcile the original operation.',locale_invalid:'Invalid language configuration.'}
} as const;
export function locale(environment:NodeJS.ProcessEnv):Locale{const raw=environment.SUMA_LANGUAGE;if(raw===undefined||raw==='zh'||raw==='zh-CN')return'zh-CN';if(raw==='en'||raw==='en-US')return'en-US';throw Error('locale_invalid');}
export function display(key:keyof typeof catalogs['zh-CN'],language:Locale){return catalogs[language][key];}
