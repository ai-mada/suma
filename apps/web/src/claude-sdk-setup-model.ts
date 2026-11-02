import {runnerSetupLiteral} from './cursor-setup-guide-model';
import {wellFormed} from './i18n/catalog';
import {validDesktopClaudeSDKCredentialEnvRef as credential} from './desktop-claude-sdk-connector-model';
export type ClaudeSDKSetupInput={shell:''|'powershell'|'bash';node:string;host:string;nativeCLI:string;model:string;credentialEnvRef:string;commands:string};
export type ClaudeSDKSetupDraft={shell:'powershell'|'bash';commands:string;localTool:'claude-code-sdk'};
export type ClaudeSDKSetupRequest={adapter_id:'claude-code-sdk';executable:string;host:string;native_cli_executable:string;model:string;credential_env_ref:string;reviewed_cli_version:'2.1.314'};
const object=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==='object'&&!Array.isArray(v)&&[Object.prototype,null].includes(Object.getPrototypeOf(v));
const fields=(v:Record<string,unknown>,keys:readonly string[])=>keys.every(k=>Object.hasOwn(v,k))&&Reflect.ownKeys(v).every(k=>typeof k==='string'&&keys.includes(k));
const pathText=(v:unknown):v is string=>typeof v==='string'&&v!==''&&wellFormed(v)&&new TextEncoder().encode(v).length<=4096&&!/[\p{Cc}\u2028\u2029]/u.test(v);
const windows=(v:string)=>/^[A-Za-z]:[\\/]/.test(v)||/^[\\/]{2}/.test(v);
const absolute=(v:unknown):v is string=>pathText(v)&&(/^[A-Za-z]:[\\/].+/.test(v)||/^\/(?!\/).+/.test(v)||/^[\\/]{2}[^\\/]+[\\/][^\\/]+[\\/].+/.test(v));
const basename=(v:string)=>v.split(windows(v)?/[\\/]/:/\//).at(-1)||'';
const commandPath=(v:unknown):v is string=>absolute(v)&&/\.json$/i.test(v);
const model=(v:unknown):v is string=>typeof v==='string'&&v.length<=256&&/^claude-[a-z][a-z0-9-]*[0-9][a-z0-9.-]*$/.test(v);
export function validClaudeSDKSetupDraft(v:unknown):v is ClaudeSDKSetupDraft{return object(v)&&fields(v,['shell','commands','localTool'])&&(v.shell==='powershell'||v.shell==='bash')&&commandPath(v.commands)&&v.localTool==='claude-code-sdk';}
/** Public administrator drafts only. The Runner normalizes paths on its own
 * platform and verifies actual executables, the complete host asset manifest,
 * CLI version and a new output outside that directory before preparing it. */
export function claudeSDKSetupCommands(v:unknown):{prepare:string;describe:string;request:ClaudeSDKSetupRequest;draft:ClaudeSDKSetupDraft}|undefined{
 if(!object(v)||!fields(v,['shell','node','host','nativeCLI','model','credentialEnvRef','commands'])||(v.shell!=='powershell'&&v.shell!=='bash')||!absolute(v.node)||!['node','node.exe'].includes(basename(v.node).toLowerCase())||!absolute(v.host)||basename(v.host)!=='suma-claude.bundle.mjs'||!absolute(v.nativeCLI)||['node','node.exe'].includes(basename(v.nativeCLI).toLowerCase())||/\.(?:cmd|bat|ps1|js|mjs|cjs)$/i.test(v.nativeCLI)||!model(v.model)||!credential(v.credentialEnvRef)||!commandPath(v.commands))return;
 const shell=v.shell,request:ClaudeSDKSetupRequest={adapter_id:'claude-code-sdk',executable:v.node,host:v.host,native_cli_executable:v.nativeCLI,model:v.model,credential_env_ref:v.credentialEnvRef,reviewed_cli_version:'2.1.314'},draft:ClaudeSDKSetupDraft={shell,commands:v.commands,localTool:'claude-code-sdk'};
 const json=runnerSetupLiteral(JSON.stringify(request),shell),prepareRunner='suma-runner --prepare-tools '+runnerSetupLiteral(v.commands,shell)+' --prepare-sdk-stdin';
 const prepare=shell==='powershell'?'& {\n  $claudeSDKSetupPreviousEncoding = $OutputEncoding\n  try {\n    $OutputEncoding = [System.Text.UTF8Encoding]::new($false)\n    '+json+' | '+prepareRunner+'\n  } finally {\n    $OutputEncoding = $claudeSDKSetupPreviousEncoding\n  }\n}':"printf '%s' "+json+' | '+prepareRunner;
 const describe='suma-runner --commands '+runnerSetupLiteral(v.commands,shell)+' --allow-exec --repository '+runnerSetupLiteral('REPOSITORY',shell)+' --base-commit '+runnerSetupLiteral('COMMIT',shell)+' --describe-tools';
 return {prepare,describe,request,draft};
}
