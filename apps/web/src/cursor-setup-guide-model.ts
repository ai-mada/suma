import {validDesktopACPCredentialReference} from './desktop-acp-connector-model';
import {wellFormed} from './i18n/catalog';
export type CursorSetupInput={shell:''|'powershell'|'bash';packageRoot:string;reviewedVersion:string;model:string;credentialEnvRef:string;commands:string};
export type CursorSetupDraft={shell:'powershell'|'bash';commands:string;localTool:'cursor-cli'};
export type CursorSetupRequest={adapter_id:'cursor-cli';executable:string;reviewed_version:string;model:string;credential_env_ref:string};
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value)&&[Object.prototype,null].includes(Object.getPrototypeOf(value));
const closed=(value:Record<string,unknown>,keys:readonly string[])=>keys.every(key=>Object.hasOwn(value,key))&&Reflect.ownKeys(value).every(key=>typeof key==='string'&&keys.includes(key));
const trim=(value:string)=>value.replace(/^[\u0009-\u000d\u0020\u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+|[\u0009-\u000d\u0020\u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+$/g,'');
const text=(value:unknown,max:number):value is string=>typeof value==='string'&&!!value&&wellFormed(value)&&trim(value)===value&&new TextEncoder().encode(value).length<=max&&!/\p{Cc}/u.test(value);
const windowsPath=(value:string)=>/^[A-Za-z]:[\\/]/.test(value);
const absolute=(value:unknown):value is string=>text(value,4096)&&(windowsPath(value)||/^\/(?!\/)/.test(value));
const commandPath=(value:unknown):value is string=>absolute(value)&&/\.json$/i.test(value);
export const runnerSetupLiteral=(value:string,shell:CursorSetupDraft['shell'])=>"'"+(shell==='powershell'?value.replaceAll("'","''"):value.replaceAll("'","'\"'\"'"))+"'";
export function validCursorSetupDraft(value:unknown):value is CursorSetupDraft{
 return object(value)&&closed(value,['shell','commands','localTool'])&&(value.shell==='powershell'||value.shell==='bash')&&commandPath(value.commands)&&value.localTool==='cursor-cli';
}
/** Administrator drafts only. The local Go builder inspects the package and
 * creates the new command file; neither shell text nor this view proves that
 * the host, model, credential or execution is available. */
export function cursorSetupCommands(value:unknown):{prepare:string;describe:string;request:CursorSetupRequest;draft:CursorSetupDraft}|undefined{
 if(!object(value)||!closed(value,['shell','packageRoot','reviewedVersion','model','credentialEnvRef','commands'])||(value.shell!=='powershell'&&value.shell!=='bash')||!absolute(value.packageRoot)||!text(value.reviewedVersion,256)||!text(value.model,256)||!validDesktopACPCredentialReference(value.credentialEnvRef)||!commandPath(value.commands))return;
 const shell=value.shell,root=value.packageRoot,windows=windowsPath(root),separator=windows&&root.includes('\\')?'\\':'/',trailing=windows?/[\\/]$/.test(root):root.endsWith('/'),executable=root+(trailing?'':separator)+(windows?'node.exe':'node');
 if(!text(executable,4096))return;
 const request:CursorSetupRequest={adapter_id:'cursor-cli',executable,reviewed_version:value.reviewedVersion,model:value.model,credential_env_ref:value.credentialEnvRef},draft:CursorSetupDraft={shell,commands:value.commands,localTool:'cursor-cli'};
 const json=runnerSetupLiteral(JSON.stringify(request),shell),prepareRunner='suma-runner --prepare-tools '+runnerSetupLiteral(value.commands,shell)+' --prepare-acp-stdin';
 const prepare=shell==='powershell'?'& {\n  $cursorSetupPreviousEncoding = $OutputEncoding\n  try {\n    $OutputEncoding = [System.Text.UTF8Encoding]::new($false)\n    '+json+' | '+prepareRunner+'\n  } finally {\n    $OutputEncoding = $cursorSetupPreviousEncoding\n  }\n}':"printf '%s' "+json+' | '+prepareRunner;
 const describe='suma-runner --commands '+runnerSetupLiteral(value.commands,shell)+' --allow-exec --repository '+runnerSetupLiteral('REPOSITORY',shell)+' --base-commit '+runnerSetupLiteral('COMMIT',shell)+' --describe-tools';
 return {prepare,describe,request,draft};
}
