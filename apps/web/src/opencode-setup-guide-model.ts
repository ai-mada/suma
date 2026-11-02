import {runnerSetupLiteral} from './cursor-setup-guide-model';
import {validDesktopOpenCodeConfiguration} from './desktop-opencode-connector-model';
import {wellFormed} from './i18n/catalog';

export type OpenCodeSetupInput={shell:''|'powershell'|'bash';node:string;manifest:string;executable:string;provider:string;model:string;endpoint:string;credential:string;commands:string};
export type OpenCodeSetupDraft={shell:'powershell'|'bash';commands:string;localTool:'opencode-sdk'};
export type OpenCodeSetupRequest={adapter_id:'opencode-sdk';node_executable:string;runtime_manifest:string;native_cli_executable:string;provider_id:string;model:string;base_url:string;api_key_env_ref:string;reviewed_version:'1.19.4'};
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value)&&[Object.prototype,null].includes(Object.getPrototypeOf(value));
const closed=(value:Record<string,unknown>,keys:readonly string[])=>keys.every(key=>Object.hasOwn(value,key))&&Reflect.ownKeys(value).every(key=>typeof key==='string'&&keys.includes(key));
const pathText=(value:unknown):value is string=>typeof value==='string'&&!!value&&value.trim()===value&&wellFormed(value)&&new TextEncoder().encode(value).length<=4096&&!/[\p{Cc}\u2028\u2029]/u.test(value);
const absolute=(value:unknown):value is string=>pathText(value)&&!value.split(/[\\/]/).some(part=>part==='.'||part==='..')&&!/^[\\/]{2}[?.][\\/]/.test(value)&&(/^[A-Za-z]:[\\/].+/.test(value)||/^\/(?!\/).+/.test(value)||/^\\\\[^\\/]+[\\/][^\\/]+[\\/].+/.test(value));
const basename=(value:string)=>value.split(/^[A-Za-z]:[\\/]|^\\\\/.test(value)?/[\\/]/:/\//).at(-1)||'';
const commandPath=(value:unknown):value is string=>absolute(value)&&/\.json$/i.test(value);
export function validOpenCodeSetupDraft(value:unknown):value is OpenCodeSetupDraft{return object(value)&&closed(value,['shell','commands','localTool'])&&(value.shell==='powershell'||value.shell==='bash')&&commandPath(value.commands)&&value.localTool==='opencode-sdk';}

/** Static administrator input only. The original Runner validates actual files,
 * the reviewed manifest closure and the new-file destination on its own OS.
 * This request has no argv, bundle override, check IDs or credential values. */
export function openCodeSetupCommands(value:unknown):{request:OpenCodeSetupRequest;draft:OpenCodeSetupDraft;prepare:string;describe:string}|undefined{
 if(!object(value)||!closed(value,['shell','node','manifest','executable','provider','model','endpoint','credential','commands'])||(value.shell!=='powershell'&&value.shell!=='bash')||!absolute(value.node)||!['node','node.exe'].includes(basename(value.node).toLowerCase())||!absolute(value.manifest)||basename(value.manifest)!=='suma-opencode.manifest.json'||!absolute(value.executable)||['node','node.exe'].includes(basename(value.executable).toLowerCase())||/\.(?:cmd|bat|ps1|sh|js|mjs|cjs|json)$/i.test(value.executable)||typeof value.credential!=='string'||!commandPath(value.commands))return;
 const configuration={provider_id:value.provider,model:value.model,base_url:value.endpoint,...(value.credential?{api_key_env_ref:value.credential}:{})};
 if(!validDesktopOpenCodeConfiguration(configuration))return;
 const request:OpenCodeSetupRequest={adapter_id:'opencode-sdk',node_executable:value.node,runtime_manifest:value.manifest,native_cli_executable:value.executable,provider_id:configuration.provider_id,model:configuration.model,base_url:configuration.base_url,api_key_env_ref:value.credential,reviewed_version:'1.19.4'};
 const shell=value.shell,draft:OpenCodeSetupDraft={shell,commands:value.commands,localTool:'opencode-sdk'},json=runnerSetupLiteral(JSON.stringify(request),shell),runner='suma-runner --prepare-tools '+runnerSetupLiteral(value.commands,shell)+' --prepare-opencode-stdin';
 const prepare=shell==='powershell'?'& {\n  $openCodePreparationPreviousEncoding = $OutputEncoding\n  try {\n    $OutputEncoding = [System.Text.UTF8Encoding]::new($false)\n    '+json+' | '+runner+'\n  } finally {\n    $OutputEncoding = $openCodePreparationPreviousEncoding\n  }\n}':"printf '%s' "+json+' | '+runner;
 const describe='suma-runner --commands '+runnerSetupLiteral(value.commands,shell)+' --allow-exec --repository '+runnerSetupLiteral('REPOSITORY',shell)+' --base-commit '+runnerSetupLiteral('COMMIT',shell)+' --describe-tools';
 return {request,draft,prepare,describe};
}
