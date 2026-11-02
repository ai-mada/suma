import {readInstalledToolDiscovery,type InstalledToolFact} from './tool-quick-connect-model';
import {wellFormed} from './i18n/catalog';
export type LocalToolsScanScope=Readonly<{project:string;userID:string;organizationID:string}>;
export type LocalToolsScanRequest={action:'scan';project_id:string;user_id:string;tenant_id:string;locale:'zh-CN'|'en-US';force?:boolean};
export type LocalToolsScanResult={status:'observed';report:{protocol:'suma-installed-tools-v1';items:InstalledToolFact[]};scan:{id:string;project_id:string;user_id:string;tenant_id:string;observed_at:string;expires_at:string;cached:boolean}}|{status:'failed';code:string};
export type LocalToolsScanBridge=Readonly<{platform:'win32'|'darwin'|'linux';version:'1.0.0';discoverInstalledTools:(request:LocalToolsScanRequest)=>Promise<unknown>}>;
export const scanAdapterIDs=['codex-jsonl','codebuddy-cli','dsh-multica','claude-code-cli','cursor-cli','qoder-acp','codex-app-server','opencode-sdk','claude-code-sdk'] as const;
const encoder=new TextEncoder();
const object=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==='object'&&!Array.isArray(v)&&(Object.getPrototypeOf(v)===Object.prototype||Object.getPrototypeOf(v)===null);
const closed=(v:Record<string,unknown>,required:readonly string[],optional:readonly string[]=[])=>required.every(key=>Object.hasOwn(v,key))&&Reflect.ownKeys(v).every(key=>typeof key==='string'&&(required.includes(key)||optional.includes(key)));
const id=(v:unknown):v is string=>typeof v==='string'&&/^[a-f0-9]{32}$/.test(v);
const text=(v:unknown,max:number):v is string=>typeof v==='string'&&!!v&&wellFormed(v)&&encoder.encode(v).length<=max&&!/[\p{Cc}\u2028\u2029]/u.test(v);
const hash=(v:unknown):v is string=>typeof v==='string'&&/^[a-f0-9]{64}$/.test(v);
function instant(value:unknown):bigint|undefined{
 if(typeof value!=='string')return;
 const match=/^(\d{4}-\d\d-\d\dT(?:[01]\d|2[0-3]):[0-5]\d:[0-5]\d)(?:\.(\d{1,9}))?Z$/.exec(value);if(!match)return;
 const ms=Date.parse(match[1]+'Z');if(!Number.isFinite(ms)||new Date(ms).toISOString().slice(0,19)!==match[1])return;
 return BigInt(ms)*1000000n+BigInt((match[2]||'').padEnd(9,'0'));
}
export function localToolsScanBridge(value:unknown=(window as unknown as {sumaDesktop?:unknown}).sumaDesktop):LocalToolsScanBridge|undefined{
 return object(value)&&typeof value.platform==='string'&&['win32','darwin','linux'].includes(value.platform)&&value.version==='1.0.0'&&typeof value.discoverInstalledTools==='function'?value as LocalToolsScanBridge:undefined;
}
export function validLocalToolsScanScope(scope:LocalToolsScanScope):boolean{return object(scope)&&closed(scope,['project','userID','organizationID'])&&id(scope.project)&&id(scope.userID)&&id(scope.organizationID);}
function readQoder(row:Record<string,unknown>):InstalledToolFact|undefined{
 if(row.product!=='Qoder CLI'||typeof row.installed!=='boolean'||row.launch_supported!==false||typeof row.reason_code!=='string'||!['not_installed','unsupported_launcher','probe_failed','acp_configuration_required'].includes(row.reason_code))return;
 if(Object.hasOwn(row,'executable')&&(!text(row.executable,4096)||!(/^[A-Za-z]:[\\/]/.test(row.executable)||/^\/(?!\/)/.test(row.executable)||/^\\\\[^\\]+\\[^\\]+/.test(row.executable))))return;
 if(Object.hasOwn(row,'version')&&!text(row.version,256)||Object.hasOwn(row,'executable_sha256')&&!hash(row.executable_sha256))return;
 if(row.reason_code==='not_installed'&&(row.installed||['executable','version','executable_sha256'].some(key=>Object.hasOwn(row,key))))return;
 if(row.reason_code==='acp_configuration_required'&&(!row.installed||!row.executable||!row.version||!row.executable_sha256))return;
 return {adapter_id:'qoder-acp',product:'Qoder CLI',installed:row.installed,launch_supported:false,reason_code:row.reason_code,...(typeof row.executable==='string'?{executable:row.executable}:{}),...(typeof row.version==='string'?{version:row.version}:{}),...(typeof row.executable_sha256==='string'?{executable_sha256:row.executable_sha256}:{})};
}
/** Scan time is the original desktop observation. Parsing never starts a new
 * sixty-second period or infers execution health from an installation row. */
export function readLocalToolsScanResult(value:unknown,scope:LocalToolsScanScope):LocalToolsScanResult|undefined{
 if(!validLocalToolsScanScope(scope)||!object(value))return;
 if(closed(value,['status','code'])&&value.status==='failed'&&typeof value.code==='string'&&/^[a-z][a-z0-9_]{0,79}$/.test(value.code))return {status:'failed',code:value.code};
 if(!closed(value,['status','report','scan'])||value.status!=='observed'||!object(value.report)||!closed(value.report,['protocol','items'])||value.report.protocol!=='suma-installed-tools-v1'||!object(value.scan))return;
 const scan=value.scan;
 if(!closed(scan,['id','project_id','user_id','tenant_id','observed_at','expires_at','cached'])||!id(scan.id)||scan.project_id!==scope.project||scan.user_id!==scope.userID||scan.tenant_id!==scope.organizationID||typeof scan.cached!=='boolean')return;
 const start=instant(scan.observed_at),end=instant(scan.expires_at);
 if(start===undefined||end===undefined||end-start!==60000000000n)return;
 const rows=value.report.items;
 if(!Array.isArray(rows)||Object.getPrototypeOf(rows)!==Array.prototype||rows.length!==scanAdapterIDs.length||Reflect.ownKeys(rows).length!==scanAdapterIDs.length+1)return;
 const items:InstalledToolFact[]=[];
 for(let index=0;index<scanAdapterIDs.length;index++){
  if(!Object.hasOwn(rows,index))return;
  const row=rows[index],adapter=scanAdapterIDs[index];
  if(!object(row)||!closed(row,['adapter_id','product','installed','launch_supported','reason_code'],['executable','version','executable_sha256'])||row.adapter_id!==adapter||!Object.values(row).every(v=>typeof v!=='string'||wellFormed(v)))return;
  for(const key of ['executable','version','executable_sha256'])if(Object.hasOwn(row,key)&&typeof row[key]!=='string')return;
  const item=adapter==='qoder-acp'?readQoder(row):readInstalledToolDiscovery({status:'observed',report:{protocol:'suma-installed-tools-v1',items:[row]}},adapter);
  if(!item)return;items.push(item);
 }
 return {status:'observed',report:{protocol:'suma-installed-tools-v1',items},scan:{id:scan.id,project_id:scope.project,user_id:scope.userID,tenant_id:scope.organizationID,observed_at:scan.observed_at as string,expires_at:scan.expires_at as string,cached:scan.cached}};
}
