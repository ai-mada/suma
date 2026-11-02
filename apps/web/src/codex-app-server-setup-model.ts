import {validDesktopCodexConfiguration} from './desktop-codex-connector-model';
import {runnerSetupLiteral} from './cursor-setup-guide-model';
import {wellFormed} from './i18n/catalog';

export type CodexAppServerSetupInput={shell:''|'powershell'|'bash';executable:string;providerID:string;baseURL:string;credentialEnvRef:string;windowsSandbox:''|'unelevated'|'elevated'|'mxc';enableUserInput:boolean;commands:string};
export type CodexAppServerSetupDraft={shell:'powershell'|'bash';commands:string;localTool:'codex-app-server'};
export type CodexAppServerSetupRequest={adapter_id:'codex-app-server';executable:string;provider_id:string;base_url:string;api_key_env_ref:string;windows_sandbox:CodexAppServerSetupInput['windowsSandbox'];enable_user_input:boolean};
const plain=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value)&&[Object.prototype,null].includes(Object.getPrototypeOf(value));
const closed=(value:Record<string,unknown>,keys:readonly string[])=>keys.every(key=>Object.hasOwn(value,key))&&Reflect.ownKeys(value).every(key=>typeof key==='string'&&keys.includes(key));
const absolute=(value:unknown):value is string=>typeof value==='string'&&!!value&&value.trim()===value&&wellFormed(value)&&new TextEncoder().encode(value).length<=4096&&!/[\p{Cc}\u2028\u2029]/u.test(value)&&!value.split(/[\\/]/).some(part=>part==='.'||part==='..')&&(/^[A-Za-z]:[\\/].+/.test(value)||/^\/(?!\/).+/.test(value)||/^\\\\[^\\/]+[\\/][^\\/]+[\\/].+/.test(value));
const commandPath=(value:unknown):value is string=>absolute(value)&&/\.json$/i.test(value);
export function validCodexAppServerSetupDraft(value:unknown):value is CodexAppServerSetupDraft{return plain(value)&&closed(value,['shell','commands','localTool'])&&(value.shell==='powershell'||value.shell==='bash')&&commandPath(value.commands)&&value.localTool==='codex-app-server';}

/** The target Runner validates and writes a NEW commands file. These public
 * drafts never run a CLI, infer the remote OS, read a secret or select a model. */
export function codexAppServerSetupCommands(value:unknown):{request:CodexAppServerSetupRequest;draft:CodexAppServerSetupDraft;prepare:string}|undefined{
 if(!plain(value)||!closed(value,['shell','executable','providerID','baseURL','credentialEnvRef','windowsSandbox','enableUserInput','commands'])||(value.shell!=='powershell'&&value.shell!=='bash')||!absolute(value.executable)||/\.(?:cmd|bat|ps1|js|mjs|cjs)$/i.test(value.executable)||!commandPath(value.commands)||typeof value.credentialEnvRef!=='string'||typeof value.windowsSandbox!=='string'||!['','unelevated','elevated','mxc'].includes(value.windowsSandbox)||typeof value.enableUserInput!=='boolean')return;
 const configuration={provider_id:value.providerID,base_url:value.baseURL,...(value.credentialEnvRef?{api_key_env_ref:value.credentialEnvRef}:{}),...(value.windowsSandbox?{windows_sandbox:value.windowsSandbox}:{}),enable_user_input:value.enableUserInput};
 if(!validDesktopCodexConfiguration(configuration))return;
 const request:CodexAppServerSetupRequest={adapter_id:'codex-app-server',executable:value.executable,provider_id:configuration.provider_id as string,base_url:configuration.base_url as string,api_key_env_ref:value.credentialEnvRef,windows_sandbox:value.windowsSandbox as CodexAppServerSetupInput['windowsSandbox'],enable_user_input:value.enableUserInput};
 const shell=value.shell,draft:CodexAppServerSetupDraft={shell,commands:value.commands,localTool:'codex-app-server'},json=runnerSetupLiteral(JSON.stringify(request),shell),runner='suma-runner --prepare-tools '+runnerSetupLiteral(value.commands,shell)+' --prepare-codex-stdin';
 const prepare=shell==='powershell'?'& {\n  $codexPreparationPreviousEncoding = $OutputEncoding\n  try {\n    $OutputEncoding = [System.Text.UTF8Encoding]::new($false)\n    '+json+' | '+runner+'\n  } finally {\n    $OutputEncoding = $codexPreparationPreviousEncoding\n  }\n}':"printf '%s' "+json+' | '+runner;
 return {request,draft,prepare};
}
