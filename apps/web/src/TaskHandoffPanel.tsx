import {useCallback,useEffect,useId,useRef,useState} from 'react';
import {ArrowRightLeft,ChevronDown} from 'lucide-react';
import type {TaskHandoff,TaskHandoffPage} from '../../../packages/contracts/src';
import {type Session,query,stateLabel} from './api';
import {useApi} from './api-context';
import {formatDateTime,formatNumber,renderDiagnostic,tr,UiError,useLocale} from './i18n';
import {toolError,toolSessionStorage} from './tool-hooks';
import {ErrorNotice,Field,Loading} from './ui';
import {useWriteAction,WriteButton,WriteFields,WriteNotice,type WriteRecoveryRequest} from './write-action';
import './TaskHandoffPanel.css';

type Props={session:Session;project:string;taskId:string;taskVersion:number;refresh:number;onChanged:()=>void|Promise<void>};
type Decision={id:string;version:number;action:'accept'|'return'|'withdraw';reason:string;acknowledged:boolean};
type ReadState={page?:TaskHandoffPage;loading:boolean;error?:unknown};
const text=(key:string,values?:Record<string,unknown>)=>tr('task-handoff.'+key,values);
const states=new Set(['pending','accepted','returned','withdrawn']);
const validText=(value:unknown)=>typeof value==='string'&&!!value.trim()&&new TextEncoder().encode(value).length<=4000;
const validID=(value:unknown)=>typeof value==='string'&&value.length>0&&value.length<=64&&!/[\x00-\x1f\x7f]/.test(value);
const version=(value:unknown)=>typeof value==='number'&&Number.isSafeInteger(value)&&value>0;

function validRecovery(request:WriteRecoveryRequest,project:string,taskId:string){
 const body=request.body;if(request.method!=='POST'||body.project_id!==project)return false;
 if(request.path==='/task-handoffs')return Object.keys(body).every(key=>['project_id','task_id','expected_task_version','to_resource_id','to_team_id','note','idempotency_key'].includes(key))&&body.task_id===taskId&&version(body.expected_task_version)&&validID(body.to_resource_id)&&(body.to_team_id===undefined||body.to_team_id===''||validID(body.to_team_id))&&validText(body.note);
 const match=request.path.match(/^\/task-handoffs\/([^/]+)\/commands$/);
 try{return !!match&&validID(decodeURIComponent(match[1]))&&Object.keys(body).every(key=>['project_id','expected_version','action','reason','acknowledge_invalidation','idempotency_key'].includes(key))&&version(body.expected_version)&&['accept','return','withdraw'].includes(String(body.action))&&validText(body.reason)&&typeof body.acknowledge_invalidation==='boolean'&&(body.action!=='accept'||body.acknowledge_invalidation===true);}catch{return false;}
}

/** The keyed boundary also protects callers that do not key their inspector. */
export function TaskHandoffPanel(props:Props){
 const scope=JSON.stringify([props.session.user.id,props.session.organization.id,props.project,props.taskId]);
 return <HandoffScope key={scope} {...props} scope={scope}/>;
}

function HandoffScope({session,project,taskId,taskVersion,refresh,onChanged,scope}:Props&{scope:string}){
 useLocale();
 const api=useApi(),bodyID=useId(),[open,setOpen]=useState(false),[readState,setReadState]=useState<ReadState>({loading:false});
 const action=useWriteAction({scope:'task-handoff:'+scope,storage:toolSessionStorage(),validateRequest:request=>validRecovery(request,project,taskId)});
 const pendingBody=action.pending?.body;
 const [recipient,setRecipient]=useState(typeof pendingBody?.to_resource_id==='string'?pendingBody.to_resource_id:''),[team,setTeam]=useState(typeof pendingBody?.to_team_id==='string'?pendingBody.to_team_id:''),[note,setNote]=useState(typeof pendingBody?.note==='string'?pendingBody.note:'');
 const [decision,setDecision]=useState<Decision>(),[success,setSuccess]=useState<TaskHandoff>(),[refreshError,setRefreshError]=useState<unknown>();
 const epoch=useRef(0),controller=useRef<AbortController|undefined>(undefined),pageRef=useRef<TaskHandoffPage|undefined>(undefined),alive=useRef(true),changed=useRef(onChanged);
 changed.current=onChanged;
 const read=useCallback(async(more=false):Promise<TaskHandoffPage|undefined>=>{
  if(!open)return;
  const previous=pageRef.current,cursor=more?previous?.next_cursor:'';
  if(more&&!cursor)return previous;
  const generation=++epoch.current;controller.current?.abort();const abort=new AbortController();controller.current=abort;
  if(!more)pageRef.current=undefined;
  setReadState({page:more?previous:undefined,loading:true});
  try{
   const path='/task-handoffs'+query(project)+'&task_id='+encodeURIComponent(taskId)+'&limit=20'+(cursor?'&before='+encodeURIComponent(cursor):'');
   const page=await api<TaskHandoffPage>(path,undefined,'GET',undefined,abort.signal);
   if(abort.signal.aborted||generation!==epoch.current)return;
   if(!Array.isArray(page.items)||page.items.length>20||!Number.isSafeInteger(page.total)||page.total<page.items.length||typeof page.next_cursor!=='string'||!Array.isArray(page.recipients)||!Array.isArray(page.create_blockers)||typeof page.can_create!=='boolean'||!version(page.task_version)||page.items.some(item=>item.project_id!==project||item.task_id!==taskId||!validID(item.id)||!version(item.version)||!states.has(item.state)||!Array.isArray(item.allowed_actions)||!Array.isArray(item.blockers))||page.next_cursor!==''&&!page.items.some(item=>item.id===page.next_cursor))throw new UiError('task-handoff.invalid_read');
   const value={...page,items:more?[...new Map([...(previous?.items||[]),...page.items].map(item=>[item.id,item])).values()]:page.items};
   if(value.items.length>value.total)throw new UiError('task-handoff.invalid_read');
   pageRef.current=value;setReadState({page:value,loading:false});return value;
  }catch(error){
   if(abort.signal.aborted||generation!==epoch.current)return;
   pageRef.current=undefined;setReadState({loading:false,error});
  }
 },[api,open,project,taskId]);
 useEffect(()=>{if(open)void read();return()=>{epoch.current++;controller.current?.abort();};},[read,open,taskVersion,refresh]);
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;};},[]);
 const page=readState.page,selected=page?.recipients.find(item=>item.resource_id===recipient),target=decision&&page?.items.find(item=>item.id===decision.id);
 const locked=action.busy||!!action.pending;
 const canCreate=!!page?.can_create&&!!selected&&(!team||selected.teams.some(item=>item.team_id===team))&&validText(note)&&!readState.loading;
 const canDecide=!!decision&&!!target&&target.version===decision.version&&target.allowed_actions.includes(decision.action)&&(!decision.reason||validText(decision.reason))&&(decision.action==='accept'?decision.acknowledged&&target.basis_current:validText(decision.reason))&&!readState.loading;
 async function confirmed(result:TaskHandoff|undefined){
  if(!result||!alive.current)return;
  if(result.project_id!==project||result.task_id!==taskId||!states.has(result.state)){action.setError(new UiError('task-handoff.invalid_read'));return;}
  setSuccess(result);setRecipient('');setTeam('');setNote('');setDecision(undefined);setRefreshError(undefined);
  const results=await Promise.allSettled([read(),Promise.resolve().then(()=>changed.current())]);
  if(alive.current&&results.some(result=>result.status==='rejected'))setRefreshError(new UiError('task-handoff.refresh_failed'));
 }
 async function create(){
  if(!canCreate||!page)return;setSuccess(undefined);
  await confirmed(await action.run<TaskHandoff>('/task-handoffs',{project_id:project,task_id:taskId,expected_task_version:page.task_version,to_resource_id:recipient,to_team_id:team,note}));
 }
 async function command(){
  if(!canDecide||!decision)return;setSuccess(undefined);
  await confirmed(await action.run<TaskHandoff>('/task-handoffs/'+encodeURIComponent(decision.id)+'/commands',{project_id:project,expected_version:decision.version,action:decision.action,reason:decision.reason||text('accept_default_reason'),acknowledge_invalidation:decision.action==='accept'&&decision.acknowledged}));
 }
 async function rebase(){
  const value=await read();if(!value)throw new UiError('task-handoff.refresh_failed');
  if(decision){const latest=value.items.find(item=>item.id===decision.id);if(latest)setDecision({...decision,version:latest.version,acknowledged:false});}
 }
 return <section className="detail-section task-handoff" aria-label={text('title')}>
  <h3><button type="button" className="task-handoff-toggle" data-handoff-toggle aria-expanded={open} aria-controls={bodyID} disabled={action.busy||open&&!!action.pending} onClick={()=>setOpen(value=>!value)}><ArrowRightLeft size={16} aria-hidden="true"/><span>{text('title')}</span><ChevronDown size={16} className={open?'is-open':''} aria-hidden="true"/></button></h3>
  {open&&<div id={bodyID} className="task-handoff-body" aria-busy={readState.loading}>
   <p className="task-handoff-hint">{text('intro')}</p>
   <WriteNotice action={action} onRebase={rebase}/>
   {action.pending&&<div className="task-handoff-recovery" role="status"><strong>{text('original_operation')}</strong><p className="task-handoff-copy">{String(action.pending.body.note||action.pending.body.reason||'')}</p><button type="button" data-handoff-restore disabled={action.busy} onClick={()=>{const pending=action.pending;if(pending)void action.run<TaskHandoff>(pending.path,pending.body,pending.method).then(confirmed);}}>{text('restore')}</button></div>}
   {success&&<p className="task-handoff-success" role="status">{text('saved.'+success.state)}</p>}
   {Boolean(refreshError)&&<ErrorNotice error={refreshError}/>}
   {readState.loading&&<Loading/>}
   {Boolean(readState.error)&&<ErrorNotice error={toolError(readState.error)} onRetry={()=>void read()}/>}
   {page&&<>
    {!!page.create_blockers.length&&<div className="task-handoff-blockers"><span>{text('create_unavailable')}</span><ul>{page.create_blockers.map((value,index)=><li key={index}>{renderDiagnostic(value)}</li>)}</ul></div>}
    {!page.recipients.length&&<div className="task-handoff-empty"><p>{text('no_recipients')}</p><a href={'?'+new URLSearchParams({project,view:'settings',sub:'members'})}>{text('register_person')}</a></div>}
    {page.can_create&&!!page.recipients.length&&<form data-handoff-create onSubmit={event=>{event.preventDefault();void create();}}>
     <h4>{text('propose')}</h4><WriteFields action={action}>
      <Field label={text('recipient')}><select name="recipient" value={recipient} required onChange={event=>{setRecipient(event.target.value);setTeam('');}}><option value="">{text('choose_recipient')}</option>{recipient&&!selected&&<option value={recipient} disabled>{text('recipient_unavailable')}</option>}{page.recipients.map(item=><option key={item.resource_id} value={item.resource_id}>{item.name}</option>)}</select></Field>
      <Field label={text('team')}><select name="team" value={team} disabled={!selected} onChange={event=>setTeam(event.target.value)}><option value="">{text('no_team')}</option>{(selected?.teams||[]).map(item=><option key={item.team_id} value={item.team_id}>{item.name}</option>)}</select></Field>
      <Field label={text('note')}><textarea name="note" value={note} required rows={3} maxLength={4000} onChange={event=>setNote(event.target.value)}/></Field>
      {!!note&&!validText(note)&&<p className="task-handoff-hint">{text('text_invalid')}</p>}
      <p className="task-handoff-hint">{text('task_version',{version:formatNumber(page.task_version)})}</p>
     </WriteFields><WriteButton action={action} disabled={!canCreate||action.persistenceBlocked}>{text('send')}</WriteButton>
    </form>}
    <div className="task-handoff-history"><div className="task-handoff-history-head"><h4>{text('history')}</h4><span>{formatNumber(page.items.length)} / {formatNumber(page.total)}</span></div>
     {!page.items.length&&<p className="task-handoff-hint">{text('empty')}</p>}
     {page.items.map(item=><article key={item.id} className="task-handoff-item" data-handoff-item>
      <div className="task-handoff-item-head"><strong>{item.to_name}</strong><span className={'task-handoff-state state-'+item.state}>{text('state.'+item.state)}</span></div>
      {item.to_team_name&&<small>{item.to_team_name}</small>}<time dateTime={item.created_at}>{formatDateTime(item.created_at)}</time>
      <p className="task-handoff-copy">{item.note}</p>
      {!!item.decision_reason&&<p className="task-handoff-decision-reason">{item.decision_reason}</p>}
      {!!item.blockers.length&&<ul className="task-handoff-blockers">{item.blockers.map((value,index)=><li key={index}>{renderDiagnostic(value)}</li>)}</ul>}
      <Basis item={item}/>
      <div className="task-handoff-actions">{(['accept','return','withdraw'] as const).filter(value=>item.allowed_actions.includes(value)).map(value=><button type="button" key={value} data-handoff-command={value} disabled={locked||value==='accept'&&!item.basis_current} onClick={()=>setDecision({id:item.id,version:item.version,action:value,reason:'',acknowledged:false})}>{text('action.'+value)}</button>)}</div>
     </article>)}
     {page.next_cursor&&<button type="button" className="task-handoff-more" data-handoff-more disabled={readState.loading||locked} onClick={()=>void read(true)}>{text('more')}</button>}
    </div>
    {decision&&<form className="task-handoff-command" data-handoff-decision onSubmit={event=>{event.preventDefault();void command();}}>
     <h4>{text('action.'+decision.action)}</h4><WriteFields action={action}>
      {decision.action==='accept'&&<><p className="task-handoff-impact">{text('accept_impact')}</p><label className="task-handoff-ack"><input type="checkbox" name="acknowledge_invalidation" checked={decision.acknowledged} onChange={event=>setDecision({...decision,acknowledged:event.target.checked})}/><span>{text('accept_ack')}</span></label></>}
      <Field label={text(decision.action==='accept'?'accept_reason':'reason')}><textarea name="reason" value={decision.reason} required={decision.action!=='accept'} rows={3} maxLength={4000} onChange={event=>setDecision({...decision,reason:event.target.value})}/></Field>
      {!!decision.reason&&!validText(decision.reason)&&<p className="task-handoff-hint">{text('text_invalid')}</p>}
      {(!target||target.version!==decision.version||!target.allowed_actions.includes(decision.action))&&<p className="task-handoff-hint">{text('decision_changed')}</p>}
     </WriteFields><div className="task-handoff-actions"><WriteButton action={action} disabled={!canDecide||action.persistenceBlocked}>{text('confirm.'+decision.action)}</WriteButton><button type="button" disabled={locked} onClick={()=>setDecision(undefined)}>{text('cancel')}</button></div>
    </form>}
   </>}
  </div>}
 </section>;
}

function Basis({item}:{item:TaskHandoff}){
 const [open,setOpen]=useState(false),basis=item.basis;
 return <details className="task-handoff-basis" data-handoff-basis onToggle={event=>setOpen(event.currentTarget.open)}><summary>{text('basis')}</summary>{open&&<dl>
  <dt>{text('basis_task')}</dt><dd>v{formatNumber(basis.task_version)}</dd>
  <dt>{text('basis_owner')}</dt><dd><code>{basis.from_owner_id||'—'}</code></dd>
  {basis.requirement_id&&<><dt>{text('basis_requirement')}</dt><dd><code>{basis.requirement_id}</code> · v{formatNumber(basis.requirement_version)}</dd></>}
  {basis.source_run_id&&<><dt>{text('basis_run')}</dt><dd><code>{basis.source_run_id}</code> · v{formatNumber(basis.source_run_version)} · {stateLabel(basis.source_run_state)}</dd></>}
  <dt>{text('basis_task_sha')}</dt><dd><code>{basis.task_basis_sha256}</code></dd>
  {basis.delivery_sha256&&<><dt>{text('basis_delivery')}</dt><dd><code>{basis.delivery_sha256}</code></dd></>}
 </dl>}</details>;
}
