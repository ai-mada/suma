import {api,ApiError,operation,query} from './api';
import {automationError} from './automation-model';
import type {AutomationIngress,AutomationIngressSpec} from '../../../packages/contracts/src/index';
export type {AutomationIngress,AutomationIngressSpec,AutomationIngressRevision,AutomationIngressEvent,AutomationIngressDelivery,AutomationIngressCreateRequest,AutomationIngressCommandRequest} from '../../../packages/contracts/src/index';
export const ingressEvents:Record<string,string[]>={github:['push','workflow_run.completed','ping'],suma:['git.push','ci.completed','monitor.firing','monitor.resolved']};
export const ingressStates:Record<string,string>={paused:'已暂停',enabled:'已启用',revoked:'已撤销'};
export const ingressDeliveryStates:Record<string,string>={accepted:'已创建触发',ignored:'已忽略',blocked:'门禁阻止'};
const ingressReasons:Record<string,string>={
 connection_check:'这是连接检查，不会创建触发；需要执行时请投递入口允许的业务事件。',
 branch_deleted:'来源分支或标签已删除，不会创建触发；请核对来源事件。',
 action_not_completed:'来源工作流动作尚未达到 completed，不创建触发；请等待完成事件。',
 ingress_inactive:'入口未启用或尚未配置密钥。请核对入口状态与密钥元数据，再确认启用。',
 rule_inactive_or_revised:'规则未启用或入口绑定修订已变化。请核对当前规则，必要时编辑来源改绑，再确认启用。',
 activation_authority_revoked:'入口或规则启用人的当前项目写入或审批权限不足。请联系项目管理员核对授权，再由有权账号确认启用。',
 workflow_unavailable:'固定工作流当前不可用。请核对并恢复该流程版本，再确认启用；平台审批与测试门禁继续生效。',
 filter_mismatch:'事件未匹配入口的受限条件，未创建触发。请核对交付上下文与当时来源修订的过滤条件。',
 ingress_delivery_unavailable:'无法读取此触发的关联交付记录，已阻止派发。请联系管理员核对持久记录。',
 ingress_revised_or_rotated:'入口来源修订或密钥已变化，原排队触发已阻止。请核对新配置；旧交付不会重新评估。',
 ingress_pause:'入口已暂停，原排队触发已阻止。核对后可重新启用；旧交付不会重新触发。',
 ingress_revise:'入口来源已修改，原排队触发已阻止。请核对当前来源与规则修订，再确认启用。',
 ingress_revoke:'入口已撤销，原排队触发已阻止。历史仍保留；需要继续接入时请创建新入口。',
 ingress_rotate_key:'入口密钥已变更，原排队触发已阻止。请在发送端保存当前密钥，再确认启用。'
};
export function ingressReason(raw:string){const code=raw.split(';')[0],known=Object.prototype.hasOwnProperty.call(ingressReasons,code);return {known,message:known?ingressReasons[code]:'服务端已保留此次判定原因，请展开原始原因核对。',raw};}
export const newIngressSpec=():AutomationIngressSpec=>({name:'',provider:'github',events:['push'],repository:'',repository_id:'',ref:'',conclusions:[]});
const bytes=(text:string)=>new TextEncoder().encode(text).length;
const controls=/[\u0000-\u001f\u007f]/;
export function ingressSpec(input:AutomationIngressSpec):AutomationIngressSpec{
 const allowed=['name','provider','events','repository','repository_id','ref','conclusions'];
 if(Object.keys(input).some(key=>!allowed.includes(key)))throw new Error('事件入口只接受受限过滤字段，不能提供执行命令或其他参数。');
 if(!input.name.trim()||bytes(input.name.trim())>200||controls.test(input.name))throw new Error('入口名称应为 1–200 UTF-8 字节，且不含控制字符。');
 const events=ingressEvents[input.provider];if(!events)throw new Error('请选择 GitHub 或 Suma 签名协议。');
 if(!Array.isArray(input.events)||input.events.length<1||input.events.length>8||new Set(input.events).size!==input.events.length||input.events.some(event=>!events.includes(event)))throw new Error('选择当前协议支持的事件，不能重复。');
 for(const field of ['repository','repository_id','ref'] as const)if(typeof input[field]!=='string'||controls.test(input[field]))throw new Error('来源过滤字段不能包含控制字符。');
 if(bytes(input.repository)>201||input.repository_id.length>30||input.repository_id&&!/^[1-9][0-9]*$/.test(input.repository_id))throw new Error('仓库名最多 201 字节，仓库 ID 应为最多 30 位的正十进制数字。');
 if(input.provider==='github'&&(!/^[^\s/]+\/[^\s/]+$/.test(input.repository)||!input.repository_id))throw new Error('GitHub 入口必须同时指定 owner/repo 和仓库数字 ID。');
 if(bytes(input.ref)>250||input.ref&&!/^refs\/(?:heads|tags)\/.+$/.test(input.ref))throw new Error('分支和标签过滤需使用 refs/heads/分支名 或 refs/tags/标签名；留空表示所有分支和标签。');
 if(!Array.isArray(input.conclusions)||input.conclusions.length>16||new Set(input.conclusions).size!==input.conclusions.length||input.conclusions.some(value=>!/^[-a-z0-9_.]{1,40}$/.test(value)))throw new Error('来源结论最多 16 个不重复的小写状态，每个最多 40 字节。');
 return {name:input.name.trim(),provider:input.provider,events:[...input.events],repository:input.repository,repository_id:input.repository_id,ref:input.ref,conclusions:[...input.conclusions]};
}
export function ingressCanEnable(entry:AutomationIngress,rule:{version:number;revision:number;state:string}){return entry.state!=='revoked'&&entry.key_configured&&entry.rule_revision===rule.revision&&rule.state==='enabled';}
export const ingressPanel=(id:string)=>id?'ingress:'+id:'';
export const selectedIngress=(panel:string)=>panel.startsWith('ingress:')?panel.slice(8):'';
export const ingressPath=(id:string)=>'/automation-ingresses/'+encodeURIComponent(id);
export function ingressKey(){return Array.from(crypto.getRandomValues(new Uint8Array(32)),byte=>byte.toString(16).padStart(2,'0')).join('');}
export async function ingressFingerprint(secret:string){if(!/^[0-9a-f]{64}$/.test(secret))throw new Error('签名密钥必须是 64 个小写十六进制字符。');return Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256',new TextEncoder().encode(secret))),byte=>byte.toString(16).padStart(2,'0')).join('');}
export function ingressSpecOf(entry:AutomationIngress):AutomationIngressSpec{return {name:entry.name,provider:entry.provider,events:[...entry.events],repository:entry.repository,repository_id:entry.repository_id,ref:entry.ref,conclusions:[...entry.conclusions]};}

type StorageLike=Pick<Storage,'getItem'|'setItem'|'removeItem'>;
export type IngressPending={path:string;body:Record<string,unknown>;idempotency_key:string;fingerprint?:string;secret_lost?:boolean};
const clone=<T,>(value:T):T=>JSON.parse(JSON.stringify(value));
function rejectSecrets(value:unknown){if(!value||typeof value!=='object')return;for(const[key,item]of Object.entries(value)){if(/secret|token|password|credential|authorization/i.test(key))throw new Error('恢复元数据不能包含秘密。');rejectSecrets(item);}}
// Only metadata reaches tab storage. A rotation's original secret lives here
// until its receipt arrives or the user explicitly closes the secret dialog.
export class IngressRecovery{
 private value?:IngressPending;private secret?:string;private busy=false;private key:string;readonly error:string='';
 constructor(scope:string,private project:string,private rule:string,private storage:StorageLike|undefined){
  this.key='suma-ingress-recovery-v1:'+scope;
  try{const raw=storage?.getItem(this.key);if(raw){const saved=JSON.parse(raw) as IngressPending;this.validate(saved.path,saved.body,true);if(!saved.idempotency_key||typeof saved.idempotency_key!=='string'||saved.body.action==='rotate_key'&&!/^[a-f0-9]{64}$/.test(saved.fingerprint||''))throw new Error('invalid');this.value=saved;}}
  catch{this.error='事件入口恢复记录无法读取。未发送新操作，请保留本页并核对原操作。';}
 }
 get pending():IngressPending|undefined{return this.value?{...clone(this.value),secret_lost:this.value.body.action==='rotate_key'&&!this.secret}:undefined;}
 private validate(path:string,body:Record<string,unknown>,saved=false){
  if(path!=='/automations/'+encodeURIComponent(this.rule)+'/ingresses'&&!/^\/automation-ingresses\/[A-Za-z0-9._%~-]+\/commands$/.test(path))throw new Error('恢复路径不属于当前规则的事件入口。');
  if(!body||body.project_id!==this.project)throw new Error('事件入口请求缺少当前项目范围。');
  const{secret,...metadata}=body;rejectSecrets(metadata);if(saved&&secret!==undefined)throw new Error('持久恢复记录不能包含签名密钥。');
  if(body.action==='rotate_key'){if(!saved&&typeof secret!=='string')throw new Error('请先生成签名密钥。');if(secret!==undefined&&!/^[a-f0-9]{64}$/.test(String(secret)))throw new Error('签名密钥必须是 64 个小写十六进制字符。');}
  else if(secret!==undefined)throw new Error('元数据操作不能包含签名密钥。');
 }
 async run<T=AutomationIngress>(path:string,body:Record<string,unknown>):Promise<T>{
  if(this.error)throw new Error(this.error);if(this.busy||this.value)throw new Error('原操作仍待回执，请先恢复同一操作。');this.validate(path,body);this.busy=true;
  try{const{secret,...metadata}=body;const saved:IngressPending={path,body:clone(metadata),idempotency_key:operation(),...(secret?{fingerprint:await ingressFingerprint(String(secret))}:{})};
   try{if(!this.storage)throw new Error('unavailable');this.storage.setItem(this.key,JSON.stringify(saved));}catch{throw new Error('无法保存事件入口恢复元数据，未发送请求。');}
   this.value=saved;this.secret=secret as string|undefined;return await this.send<T>(false);
  }finally{this.busy=false;}
 }
 async restore<T=AutomationIngress>():Promise<T>{if(this.busy)throw new Error('原操作仍在等待回执。');if(!this.value)throw new Error('没有待恢复的事件入口操作。');if(this.pending?.secret_lost)throw new Error('当次密钥已离开内存，请读取最新元数据并核对指纹。');this.busy=true;try{return await this.send<T>(true);}finally{this.busy=false;}}
 private async send<T>(restoring:boolean):Promise<T>{const saved=this.value!;try{const result=await api<T>(saved.path,{...saved.body,...(this.secret?{secret:this.secret}:{}),idempotency_key:saved.idempotency_key});this.clear();return result;}catch(error){if(!restoring&&error instanceof ApiError&&error.status>0&&error.status<500)this.clear();throw error;}}
 forgetSecret(){this.secret=undefined;}
 reconcile(entry:AutomationIngress){if(!this.pending?.secret_lost||this.value?.path!==ingressPath(entry.id)+'/commands')return false;if(entry.key_fingerprint===this.value.fingerprint&&entry.version>Number(this.value.body.expected_version)){this.clear();return true;}return false;}
 acknowledgeMetadata(entry:AutomationIngress){if(!this.pending?.secret_lost||this.value?.path!==ingressPath(entry.id)+'/commands'||entry.version<Number(this.value.body.expected_version))throw new Error('请先读取原入口的最新元数据。');this.clear();}
 private clear(){this.value=undefined;this.secret=undefined;try{this.storage?.removeItem(this.key);}catch{/* Replaying the same stable operation remains safe. */}}
}

type Page<T>={items:T[];total:number;next_cursor?:string};
type Snapshot<T>={items:T[]|undefined;total:number;next_cursor:string;loading:boolean;error:string;more_error:boolean};
export class IngressPager<T>{
 private state:Snapshot<T>={items:undefined,total:0,next_cursor:'',loading:false,error:'',more_error:false};private generation=0;
 constructor(private path:string,private project:string,private identity:'id'|'revision'='id'){}
 snapshot(){return {...this.state};}
 invalidate(){this.generation++;}
 first(){return this.read('',false,++this.generation);}
 async more(){if(this.state.loading||!this.state.next_cursor)return;return this.read(this.state.next_cursor,true,this.generation);}
 private async read(cursor:string,append:boolean,generation:number){this.state={...this.state,loading:true,error:'',more_error:append};try{
  const page=await api<Page<T>>(this.path+query(this.project)+'&limit=40'+(cursor?'&cursor='+encodeURIComponent(cursor):''));if(generation!==this.generation)return;
  if(!Array.isArray(page.items)||!Number.isSafeInteger(page.total)||page.total<0||page.items.some(item=>{const id=(item as Record<string,unknown>)[this.identity];return this.identity==='id'?typeof id!=='string'||!id:!Number.isSafeInteger(id)||Number(id)<1;}))throw new Error('尚未读取到可确认的事件入口记录范围。');
  this.state={items:[...new Map([...(append?this.state.items||[]:[]),...page.items].map(item=>[(item as Record<string,unknown>)[this.identity],item])).values()],total:page.total,next_cursor:page.next_cursor||'',loading:false,error:'',more_error:false};
 }catch(error){if(generation===this.generation)this.state={...this.state,loading:false,error:automationError(error)};}}
}
