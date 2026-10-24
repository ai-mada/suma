import {api,ApiError,operation,query,type Session,type Workflow} from './api';
import {mutate} from './operations';
import type {AutomationSpec as WireSpec,AutomationRevision as WireRevision,AutomationRule as WireRule,AutomationOccurrence as WireOccurrence,AutomationPreview as WirePreview} from '../../../packages/contracts/src/index';

export type AutomationSpec=Omit<WireSpec,'trigger'|'misfire'|'overlap'>&{trigger:'cron'|'manual';misfire:'skip'|'latest'|'catch_up';overlap:'allow'|'forbid'|'queue'};
export type AutomationRevision=Omit<WireRevision,'spec'>&{spec:AutomationSpec};
export type AutomationRule=Omit<WireRule,keyof WireSpec|'revisions'>&AutomationSpec&{revisions?:AutomationRevision[]};
export type AutomationOccurrence=WireOccurrence;
export type AutomationResult={rule:AutomationRule;occurrence?:AutomationOccurrence};
export type AutomationPreview=WirePreview;
export type AutomationPage<T>={items:T[];total:number;next_cursor?:string};
export const automationStates:Record<string,string>={draft:'草稿',enabled:'已启用',paused:'已暂停',archived:'已归档'};
export const occurrenceStates:Record<string,string>={queued:'等待派发',started:'已绑定运行',skipped:'已跳过',blocked:'派发受阻'};
export const misfireNames={skip:'跳过错过的时刻',latest:'仅补最近一次',catch_up:'有界补跑'};
export const overlapNames={allow:'允许并行',forbid:'跳过重叠触发',queue:'排队等待'};
export const newAutomationSpec=():AutomationSpec=>({name:'',workflow_id:'',workflow_version:0,trigger:'cron',cron:'0 9 * * 1-5',timezone:'Asia/Shanghai',misfire:'latest',catch_up_limit:5,overlap:'queue'});
export const automationEnabled=(rule:AutomationRule)=>rule.state==='enabled';
export function automationDetailPath(project:string,id:string){return '/automations/'+encodeURIComponent(id)+query(project);}
export function automationListPath(project:string,cursor='',rule=''){return '/automations'+(rule?'/'+encodeURIComponent(rule)+'/occurrences':'')+query(project)+'&limit=40'+(cursor?'&cursor='+encodeURIComponent(cursor):'');}
export function automationPermissions(session:Session,project:string){const grant=session.projects.find(item=>item.id===project);if(!grant)return {write:false,approve:false};const inherited=['owner','admin'].includes(session.user.role||'');return {write:inherited||['admin','developer','reviewer'].includes(grant.role),approve:inherited||['admin','reviewer'].includes(grant.role)};}
export function automationWorkflowOptions(rows:Workflow[]){return [...new Map(rows.filter(row=>row.version>0).map(row=>[row.id+'@'+row.version,row])).values()].sort((a,b)=>a.id.localeCompare(b.id)||b.version-a.version);}
export function automationSpec(input:AutomationSpec):AutomationSpec{
 if(!input.name.trim())throw new Error('填写规则名称。');
 if(new TextEncoder().encode(input.name.trim()).length>200)throw new Error('名称过长，请缩短（中文约 66 字）。');
 if(!input.workflow_id||!Number.isSafeInteger(input.workflow_version)||input.workflow_version<1)throw new Error('请选择真实工作流并填写大于 0 的固定版本。');
 if(!['cron','manual'].includes(input.trigger))throw new Error('请选择定时或人工触发。');
 if(input.trigger==='cron'&&input.cron.trim().split(/\s+/).length!==5)throw new Error('Cron 需要分钟、小时、日、月、星期五段表达式。');
 try{new Intl.DateTimeFormat('en',{timeZone:input.timezone}).format();}catch{throw new Error('填写有效的 IANA 时区，例如 Asia/Shanghai。');}
 if(!Number.isSafeInteger(input.catch_up_limit)||input.catch_up_limit<1||input.catch_up_limit>100)throw new Error('补跑上限应为 1–100 的整数。');
 if(!(input.misfire in misfireNames)||!(input.overlap in overlapNames))throw new Error('请选择有效的错过执行和重叠策略。');
 return {name:input.name.trim(),workflow_id:input.workflow_id,workflow_version:input.workflow_version,trigger:input.trigger,cron:input.trigger==='manual'?'':input.cron.trim(),timezone:input.timezone.trim(),misfire:input.misfire,catch_up_limit:input.catch_up_limit,overlap:input.overlap};
}
export function automationError(error:unknown){if(error instanceof ApiError){if(error.status===403)return '当前账号没有访问或执行此操作的权限，请联系项目管理员。';return error.message+'（'+error.code+'）';}return error instanceof Error?error.message:'读取未完成，请重试。';}

export function automationMoment(value:string,timezone='UTC'){
 let local='时间暂不可读',zone=timezone;try{local=new Intl.DateTimeFormat('zh-CN',{timeZone:timezone,year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',second:'2-digit',hour12:false}).format(new Date(value));}catch{zone='UTC';try{local=new Date(value).toISOString();}catch{/* Keep the raw server time below for audit. */}}
 return {local,timezone:zone,utc:value};
}
const reasonNames:Record<string,string>={
 activation_authority_or_workflow_unavailable:'当前启用人的权限或固定工作流不可用，已停止本窗口派发。',
 activation_authority_revoked:'启用人的项目写入或审批权限已撤销，已停止派发。',
 workflow_unavailable:'固定工作流版本不可用，已停止派发。',
 rule_inactive_or_revised:'规则已停用或规格已变更，此次触发停止派发。',
 rule_revised:'规则规格已变更，此次触发停止派发。',rule_revise:'规则策略已修改，原规格的等待触发停止派发。',
 rule_pause:'规则已暂停，此次等待触发停止派发。',rule_archive:'规则已归档，此次等待触发停止派发。',
 overlap_forbid:'前次运行尚未终结，按重叠策略跳过此次触发。',overlap_wait:'前次运行尚未终结，此次触发排队等待。',queue_wait:'较早的触发仍在排队，此次继续等待派发。',
 misfire_skip:'错过的触发窗口已跳过。',misfire_latest:'错过的触发已合并，仅保留最近一次。',misfire_catch_up:'按恢复策略补跑此次触发。',misfire_catch_up_limit:'恢复窗口补跑数量已达上限，剩余触发跳过。',already_resolved:'此次触发已经处理。'
};
export function automationReason(raw:string,scheduledAt='',timezone='UTC'){
 const[code,...parts]=raw.split(';'),fields=new Map(parts.map(part=>{const separator=part.indexOf('=');return [part.slice(0,separator),part.slice(separator+1)];}));
 const start=fields.get('window_start')||(fields.has('window_end')?scheduledAt:''),end=fields.get('window_end')||(fields.has('window_start')?scheduledAt:'');
 const valid=(value:string)=>!!value&&Number.isFinite(Date.parse(value));
 const range=valid(start)&&valid(end)?`${automationMoment(start,timezone).local} 至 ${automationMoment(end,timezone).local}（${timezone}）`:'';
 return {message:reasonNames[code]||'服务端原因',range,raw};
}
type RevisionSnapshot={items:AutomationRevision[]|undefined;total:number;next_cursor:string;loading:boolean;error:string;more_error:boolean};
// Revision numbers are the immutable history identity; these records have no id.
export class AutomationRevisionPager{
 private state:RevisionSnapshot={items:undefined,total:0,next_cursor:'',loading:false,error:'',more_error:false};private generation=0;
 constructor(private project:string,private rule:string){}
 snapshot(){return {...this.state};}
 async first(){return this.read('',false,++this.generation);}
 async more(){if(this.state.loading||!this.state.next_cursor)return;return this.read(this.state.next_cursor,true,this.generation);}
 private async read(cursor:string,append:boolean,generation:number){
  this.state={...this.state,loading:true,error:'',more_error:append};
  try{const page=await api<AutomationPage<AutomationRevision>>('/automations/'+encodeURIComponent(this.rule)+'/revisions'+query(this.project)+'&limit=40'+(cursor?'&cursor='+encodeURIComponent(cursor):''));
   if(generation!==this.generation)return;if(!Array.isArray(page.items)||!Number.isSafeInteger(page.total)||page.items.some(item=>!Number.isSafeInteger(item.revision)||item.revision<1||!item.spec))throw new Error('规格修订历史尚未返回可确认的记录范围。');
   this.state={items:[...new Map([...(append?this.state.items||[]:[]),...page.items].map(item=>[item.revision,item])).values()],total:page.total,next_cursor:page.next_cursor||'',loading:false,error:'',more_error:false};
  }catch(error){if(generation===this.generation)this.state={...this.state,loading:false,error:automationError(error)};}
 }
}

type RecoveryStorage=Pick<Storage,'getItem'|'setItem'|'removeItem'>;
export type AutomationPending={path:string;body:Record<string,unknown>;idempotency_key:string};
const clone=<T,>(value:T):T=>JSON.parse(JSON.stringify(value));
function validateRequest(path:string,body:unknown){
 if(!/^\/automations(?:\/[A-Za-z0-9._%~-]+\/commands)?$/.test(path))throw new Error('只可恢复当前自动化的规则写入与命令。');
 function inspect(value:unknown){if(!value||typeof value!=='object')return;for(const[key,item]of Object.entries(value)){if(/password|token|secret|credential|authorization/i.test(key))throw new Error('自动化恢复记录不能保存秘密凭证。');inspect(item);}}
 inspect(body);if(!body||typeof body!=='object'||typeof (body as Record<string,unknown>).project_id!=='string')throw new Error('自动化请求缺少项目范围。');
}
// A tab-scoped non-secret envelope survives a lost receipt and browser reload.
// operations.mutate replays its immutable body and supplied stable operation ID.
export class AutomationRecovery{
 private value?:AutomationPending;private busy=false;private key:string;readonly error:string='';
 constructor(scope:string,private storage:RecoveryStorage|undefined){this.key='suma-automation-recovery-v1:'+scope;try{const raw=storage?.getItem(this.key);if(raw){const saved=JSON.parse(raw) as AutomationPending;validateRequest(saved.path,saved.body);if(typeof saved.idempotency_key!=='string'||!saved.idempotency_key)throw new Error('invalid');this.value=saved;}}catch{this.error='自动化恢复记录无法读取，未发送新请求。请保留本页并由管理员核对原操作。';}}
 get pending(){return this.value?clone(this.value):undefined;}
 async restore<T=unknown>():Promise<T>{if(!this.value)throw new Error('没有待恢复的自动化操作。');return this.run<T>(this.value.path,this.value.body);}
 async run<T=unknown>(path:string,body:Record<string,unknown>):Promise<T>{
  if(this.error)throw new Error(this.error);if(this.busy)throw new Error('原操作仍在等待回执，请勿重复提交。');
  const restoring=!!this.value;
  if(!this.value){validateRequest(path,body);const saved={path,body:clone(body),idempotency_key:operation()};try{if(!this.storage)throw new Error('unavailable');this.storage.setItem(this.key,JSON.stringify(saved));}catch{throw new Error('无法保存自动化恢复记录，未发送请求。请启用本页会话存储后重试。');}this.value=saved;}
  this.busy=true;const saved=this.value;
  try{const result=await mutate<T>(saved.path,{...saved.body,idempotency_key:saved.idempotency_key},'POST','automation:'+this.key+':'+saved.idempotency_key);this.clear();return result;}
  catch(error){if(!restoring&&error instanceof ApiError&&error.status>0&&error.status<500)this.clear();throw error;}finally{this.busy=false;}
 }
 private clear(){this.value=undefined;try{this.storage?.removeItem(this.key);}catch{/* Same saved operation remains safe to replay. */}}
}
