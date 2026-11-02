import {validCommandCompositionFile,validCommandCompositionOutput} from './command-composition-model';
import {runnerSetupLiteral} from './cursor-setup-guide-model';
import {validDesktopACPCredentialReference} from './desktop-acp-connector-model';
import {wellFormed} from './i18n/catalog';

export type QoderSetupInput={shell:''|'powershell'|'bash';executable:string;reviewedVersion:string;model:string;credentialEnvRef:string;commands:string};
export type QoderSetupDraft={shell:'powershell'|'bash';commands:string;localTool:'qoder-acp'};
type QoderPreparationRequest={adapter_id:'qoder-acp';executable:string;reviewed_version:string;model:string;credential_env_ref:string};
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value)&&[Object.prototype,null].includes(Object.getPrototypeOf(value));
const closed=(value:Record<string,unknown>,keys:readonly string[])=>Reflect.ownKeys(value).length===keys.length&&keys.every(key=>Object.hasOwn(value,key));
const text=(value:unknown,max:number):value is string=>typeof value==='string'&&!!value&&value.trim()===value&&wellFormed(value)&&new TextEncoder().encode(value).length<=max&&!/[\p{Cc}\u2028\u2029]/u.test(value);
// Shared local-file syntax; the native preparer alone attests the actual file,
// parent chain and hash. A script is never a native executable target.
const nativePath=(value:unknown):value is string=>validCommandCompositionFile(value)&&(/^[A-Za-z]:[\\/]/.test(value)?/\.exe$/i.test(value):!/\.(?:cmd|bat|ps1|sh|js|mjs|cjs)$/i.test(value));

/** A local reference, not evidence that a file was written or Qoder ran. */
export function validQoderSetupDraft(value:unknown):value is QoderSetupDraft{
  return object(value)&&closed(value,['shell','commands','localTool'])&&(value.shell==='powershell'||value.shell==='bash')&&validCommandCompositionOutput(value.commands)&&value.localTool==='qoder-acp';
}

/** Only the original static ACP preparation mode. Generating or copying this
 * command does not probe a version, authenticate or make a model request. */
export function qoderSetupCommands(value:unknown):{request:QoderPreparationRequest;prepare:string}|undefined{
  if(!object(value)||!closed(value,['shell','executable','reviewedVersion','model','credentialEnvRef','commands'])||(value.shell!=='powershell'&&value.shell!=='bash')||!nativePath(value.executable)||!text(value.reviewedVersion,256)||!text(value.model,256)||typeof value.credentialEnvRef!=='string'||value.credentialEnvRef!==''&&!validDesktopACPCredentialReference(value.credentialEnvRef)||!validCommandCompositionOutput(value.commands))return;
  const request:QoderPreparationRequest={adapter_id:'qoder-acp',executable:value.executable,reviewed_version:value.reviewedVersion,model:value.model,credential_env_ref:value.credentialEnvRef};
  const json=runnerSetupLiteral(JSON.stringify(request),value.shell),runner='suma-runner --prepare-tools '+runnerSetupLiteral(value.commands,value.shell)+' --prepare-acp-stdin';
  const prepare=value.shell==='powershell'?'& {\n  $qoderSetupPreviousEncoding = $OutputEncoding\n  try {\n    $OutputEncoding = [System.Text.UTF8Encoding]::new($false)\n    '+json+' | '+runner+'\n  } finally {\n    $OutputEncoding = $qoderSetupPreviousEncoding\n  }\n}':"printf '%s' "+json+' | '+runner;
  return {request,prepare};
}
