import {wellFormed} from './i18n/catalog';
import {localToolPreparationCommand} from './local-tool-preparation-model';

export type RemoteRunnerSetupInput={mode:'environment'|'paired'|'identity';pairingID:string;identityPath:string;shell:''|'powershell'|'bash';control:string;project:string;worker:string;commands:string;state:string;localTool:string;repository:string;baseCommit:string;allowInstalledDiscovery:boolean;allowToolPreparation:boolean};
export const remoteRunnerLocalTools=['codex-jsonl','codebuddy-cli','dsh-multica','claude-code-cli','claude-code-sdk','cursor-cli','qoder-acp','codex-app-server','opencode-sdk'] as const;
const fields=['mode','pairingID','identityPath','shell','control','project','worker','commands','state','localTool','repository','baseCommit'] as const;
const text=(value:string,max:number)=>!!value&&value.trim()===value&&value.length<=max&&wellFormed(value)&&!/[\p{Cc}\u2028\u2029]/u.test(value);
const absolute=(value:string)=>text(value,4096)&&(/^[A-Za-z]:[\\/].+/.test(value)||/^\/(?!\/).+/.test(value)||/^\\\\[^\\/]+[\\/][^\\/]+[\\/].+/.test(value));
const id=(value:string)=>/^[A-Za-z0-9][A-Za-z0-9_.-]{0,199}$/.test(value);

/** Copyable administrator drafts only. Shell literals never evaluate field
 * contents, and no mode receives a credential value from the browser. */
export function remoteRunnerSetupCommands(value:unknown):{prepare?:string;report?:string;start?:string;discoveryStart?:string;loopback:boolean}{
 if(!value||typeof value!=='object'||Array.isArray(value)||![Object.prototype,null].includes(Object.getPrototypeOf(value)))return {loopback:false};
 const row=value as Record<string,unknown>;
 if(Reflect.ownKeys(row).length!==fields.length+2||!fields.every(key=>Object.hasOwn(row,key)&&typeof row[key]==='string')||!Object.hasOwn(row,'allowInstalledDiscovery')||typeof row.allowInstalledDiscovery!=='boolean'||!Object.hasOwn(row,'allowToolPreparation')||typeof row.allowToolPreparation!=='boolean'||row.allowToolPreparation&&!row.allowInstalledDiscovery)return {loopback:false};
 const input=row as RemoteRunnerSetupInput;
 if(!['environment','paired','identity'].includes(input.mode)||!['powershell','bash'].includes(input.shell)||input.localTool!==''&&!remoteRunnerLocalTools.some(tool=>tool===input.localTool))return {loopback:false};
 const quote=(literal:string)=>"'"+(input.shell==='powershell'?literal.replaceAll("'","''"):literal.replaceAll("'","'\"'\"'"))+"'";
 const preparationFlag=input.allowToolPreparation?' --allow-tool-preparation':'',taskDiscoveryFlag=(input.allowInstalledDiscovery?' --allow-installed-tool-discovery':'')+preparationFlag,maintenanceFlags=' --allow-installed-tool-discovery --maintenance-only'+preparationFlag;
 // Explicit generic tools share the same new-file boundary as the concise
 // preparation card. Keep the original all-tools mode and runtime args intact.
 const commandPath=absolute(input.commands)&&/\.json$/i.test(input.commands),prepare=input.localTool===''?(commandPath?'suma-runner --prepare-tools '+quote(input.commands):undefined):localToolPreparationCommand({shell:input.shell,commands:input.commands,localTool:input.localTool})?.prepare;
 const workspace=absolute(input.repository)&&/^(?:[a-f0-9]{40}|[a-f0-9]{64})$/.test(input.baseCommit)?' --workspace '+quote(input.repository)+' --repository '+quote(input.repository)+' --base-commit '+quote(input.baseCommit)+' --allow-exec':undefined;
 let safeURL=false,loopback=false;
 if(text(input.control,2048)&&/^https?:\/\//.test(input.control)&&!/[\\?#]/.test(input.control)){
  try{const url=new URL(input.control),authority=input.control.slice(input.control.indexOf('://')+3).split('/')[0];loopback=['localhost','127.0.0.1','[::1]'].includes(url.hostname);safeURL=!!url.hostname&&!url.username&&!url.password&&!authority.includes('@')&&(url.protocol==='https:'||url.protocol==='http:'&&loopback);}catch{/* An incomplete address remains a draft. */}
 }
 if(input.mode==='paired'){
  const identityArgs=safeURL&&id(input.project)&&id(input.worker)&&/^[a-f0-9]{32}$/.test(input.pairingID)?' --paired-runner '+quote(input.pairingID)+' --control '+quote(input.control)+' --project '+quote(input.project)+' --worker '+quote(input.worker):undefined;
  const args=safeURL&&id(input.project)&&id(input.worker)&&/^[a-f0-9]{32}$/.test(input.pairingID)&&commandPath&&workspace?' --paired-runner '+quote(input.pairingID)+' --control '+quote(input.control)+' --project '+quote(input.project)+' --worker '+quote(input.worker)+' --commands '+quote(input.commands)+workspace:undefined;
  return {prepare,report:args?'suma-runner --report-tools'+args:undefined,start:args?'suma-runner'+args+taskDiscoveryFlag:undefined,...(input.allowInstalledDiscovery&&identityArgs?{discoveryStart:'suma-runner'+identityArgs+maintenanceFlags}:{}),loopback:safeURL&&loopback};
 }
 if(input.mode==='identity'){
  const identityArgs=safeURL&&id(input.project)&&(input.worker===''||id(input.worker))&&absolute(input.identityPath)?' --identity '+quote(input.identityPath)+' --control '+quote(input.control)+' --project '+quote(input.project)+(input.worker?' --worker '+quote(input.worker):''):undefined;
  const args=safeURL&&id(input.project)&&(input.worker===''||id(input.worker))&&commandPath&&workspace&&absolute(input.identityPath)?' --identity '+quote(input.identityPath)+' --control '+quote(input.control)+' --project '+quote(input.project)+(input.worker?' --worker '+quote(input.worker):'')+' --commands '+quote(input.commands)+workspace:undefined;
  return {prepare,report:args?'suma-runner --report-tools'+args:undefined,start:args?'suma-runner'+args+taskDiscoveryFlag:undefined,...(input.allowInstalledDiscovery&&identityArgs?{discoveryStart:'suma-runner'+identityArgs+maintenanceFlags}:{}),loopback:safeURL&&loopback};
 }
 const required=input.shell==='powershell'?"if (-not $env:SUMA_RUNNER_TOKEN) { throw 'SUMA_RUNNER_TOKEN is required' }":': "${SUMA_RUNNER_TOKEN:?SUMA_RUNNER_TOKEN is required}"';
 const discoveryStart=input.allowInstalledDiscovery&&safeURL&&id(input.project)&&id(input.worker)&&absolute(input.state)?required+'\nsuma-runner --control '+quote(input.control)+' --project '+quote(input.project)+' --worker '+quote(input.worker)+' --state '+quote(input.state)+maintenanceFlags:undefined;
 if(!safeURL||!commandPath||!id(input.project)||!id(input.worker)||!absolute(input.state)||!workspace)return {prepare,...(discoveryStart?{discoveryStart}:{}),loopback:safeURL&&loopback};
 const args=' --control '+quote(input.control)+' --project '+quote(input.project)+' --worker '+quote(input.worker)+' --commands '+quote(input.commands)+' --state '+quote(input.state)+workspace;
 const report=required+'\nsuma-runner --report-tools'+args,start=required+'\nsuma-runner'+args+taskDiscoveryFlag;
 return {prepare,report,start,...(discoveryStart?{discoveryStart}:{}),loopback};
}
