import {useCallback,useEffect,useId,useRef,useState} from 'react';
import {ArrowRightLeft,ChevronDown} from 'lucide-react';
import type {TaskHandoff,TaskHandoffPage} from '../../../packages/contracts/src';
import {ApiError,type ApiCaller,type Session,stateLabel} from './api';
import {useApi} from './api-context';
import {formatDateTime,formatNumber,renderDiagnostic,tr,UiError,useLocale} from './i18n';
import {toolError,toolSessionStorage} from './tool-hooks';
import {ErrorNotice,Field,Loading} from './ui';
import {useLeaveGuard,useWriteAction,WriteButton,WriteFields,WriteNotice,type WriteRecoveryRequest} from './write-action';
import {appendHandoffs,draftAfterHandoffReceipt,emptyHandoffDraft,handoffCommandID,handoffDraftKey,handoffID,handoffPath,handoffsPath,readHandoffDraft,saveHandoffDraft,validHandoff,validHandoffPage,validHandoffReceipt,validHandoffRecovery,validHandoffText,type HandoffDraft} from './task-handoff-model';
import './TaskHandoffPanel.css';

type Props={session:Session;project:string;taskId:string;taskVersion:number;refresh:number;onChanged:()=>void|Promise<void>;selectedHandoff?:string;onForbidden?:(error:unknown)=>void};
const text=(key:string,values?:Record<string,unknown>)=>tr('task-handoff.'+key,values);
const refused=(error:unknown)=>error instanceof ApiError&&[401,403,404].includes(error.status);
type Snapshot<T>={api:ApiCaller;key:string;value?:T;loading:boolean;error?:unknown};

/** One identity/task owns both private drafts and the current API caller. */
export function TaskHandoffPanel(props:Props){
 const api=useApi(),owner=useRef({api,epoch:0});if(owner.current.api!==api)owner.current={api,epoch:owner.current.epoch+1};
 const scope=JSON.stringify([props.session.user.id,props.session.organization.id,props.project,props.taskId]);
 return <HandoffScope key={scope+':'+owner.current.epoch} {...props} scope={scope}/>;
}

function useHandoffRead<T>(path:string|undefined,revision:string,valid:(value:unknown)=>value is T,onDenied:(error:unknown)=>void,merge?:(previous:T,next:T)=>T|undefined,cursor?:(value:T)=>string){
 const api=useApi(),key=JSON.stringify([path,revision]),[snapshot,setSnapshot]=useState<Snapshot<T>>();
 const current=useRef({api,key}),alive=useRef(true),epoch=useRef(0),abort=useRef<AbortController|undefined>(undefined),cached=useRef<Snapshot<T>|undefined>(undefined),denied=useRef(onDenied);current.current={api,key};denied.current=onDenied;
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;epoch.current++;abort.current?.abort();};},[]);
 const read=useCallback(async(more=false):Promise<T|undefined>=>{
  if(!path)return;const previous=cached.current?.api===api&&cached.current.key===key?cached.current.value:undefined,next=more&&previous&&cursor?cursor(previous):'';if(more&&!next)return previous;
  const generation=++epoch.current,controller=new AbortController();abort.current?.abort();abort.current=controller;
  const live=()=>alive.current&&!controller.signal.aborted&&generation===epoch.current&&current.current.api===api&&current.current.key===key;
  const pending={api,key,value:more?previous:undefined,loading:true};cached.current=pending;setSnapshot(pending);
  try{
   const value=await api<unknown>(path+(next?'&before='+encodeURIComponent(next):''),undefined,'GET',undefined,controller.signal);if(!live())return;
   if(!valid(value))throw new UiError('task-handoff.invalid_read');const result=more&&previous&&merge?merge(previous,value):value;
   if(!result)throw new UiError('task-handoff.invalid_read');const state={api,key,value:result,loading:false};cached.current=state;setSnapshot(state);return result;
  }catch(error){if(!alive.current||current.current.api!==api)return;if(refused(error)){denied.current(error);return;}if(!live())return;cached.current=undefined;setSnapshot({api,key,loading:false,error});}
 },[api,key,path,valid,merge,cursor]);
 useEffect(()=>{cached.current=undefined;setSnapshot(undefined);if(path)void read();return()=>{epoch.current++;abort.current?.abort();};},[read]);
 const visible=path&&snapshot?.api===api&&snapshot.key===key?snapshot:undefined;return {...visible,read};
}
const nextCursor=(page:TaskHandoffPage)=>page.next_cursor;

function HandoffScope({session,project,taskId,taskVersion,refresh,onChanged,selectedHandoff='',onForbidden,scope}:Props&{scope:string}){
 useLocale();const bodyID=useId(),storage=toolSessionStorage(),draftKey=handoffDraftKey(session.user.id,session.organization.id,project,taskId),initial=useRef(readHandoffDraft(storage,draftKey));
 const action=useWriteAction({scope:'task-handoff:'+scope,storage,validateRequest:request=>validHandoffRecovery(request,project,taskId),validateResponse:(value,request)=>validHandoffReceipt(value,request,project,taskId)});
 const [draft,setDraft]=useState<HandoffDraft>(()=>{
  const saved=initial.current.draft,pending=action.pending;if(initial.current.invalid||!pending||saved.note||saved.recipient||saved.decision)return saved;
  const body=pending.body;if(pending.path==='/task-handoffs')return {...saved,recipient:String(body.to_resource_id),team:String(body.to_team_id||''),note:String(body.note),task_version:Number(body.expected_task_version)};
  const id=handoffCommandID(pending.path);return id?{...saved,decision:{id,version:Number(body.expected_version),action:body.action as 'accept'|'return'|'withdraw',reason:String(body.reason)}}:saved;
 });
 const [invalidDraft,setInvalidDraft]=useState(!!initial.current.invalid),[storageFailed,setStorageFailed]=useState(false),[open,setOpen]=useState(!!selectedHandoff),[acknowledged,setAcknowledged]=useState(false);
 const [denial,setDenial]=useState<unknown>(),[success,setSuccess]=useState<TaskHandoff>(),[refreshError,setRefreshError]=useState<unknown>();
 const alive=useRef(true),callbacks=useRef({onChanged,onForbidden});callbacks.current={onChanged,onForbidden};
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;};},[]);
 const deny=useCallback((error:unknown)=>{if(!alive.current)return;setDenial(error);setSuccess(undefined);setAcknowledged(false);callbacks.current.onForbidden?.(error);},[]);
 const writeDenied=!!action.errorStatus&&[401,403,404].includes(action.errorStatus),blocked=!!denial||writeDenied;
 useEffect(()=>{if(writeDenied)deny(new ApiError(action.errorStatus!,'handoff_access_changed',text('access_changed')));},[writeDenied,action.errorStatus,deny]);
 const revision=JSON.stringify([taskVersion,refresh]),pageValid=useCallback((value:unknown):value is TaskHandoffPage=>validHandoffPage(value,project,taskId),[project,taskId]);
 const history=useHandoffRead(open&&!blocked?handoffsPath(project,taskId):undefined,revision,pageValid,deny,appendHandoffs,nextCursor),page=history.value;
 const locatedValid=useCallback((value:unknown):value is TaskHandoff=>validHandoff(value,project,taskId,selectedHandoff),[project,taskId,selectedHandoff]);
 const located=useHandoffRead(open&&!blocked&&handoffID(selectedHandoff)?handoffPath(project,taskId,selectedHandoff):undefined,revision,locatedValid,deny);
 const {recipient,team,note,decision}=draft,decisionID=decision?.id||'',loadedTarget=page?.items.find(item=>item.id===decisionID);
 const decisionValid=useCallback((value:unknown):value is TaskHandoff=>validHandoff(value,project,taskId,decisionID),[project,taskId,decisionID]);
 const decisionRead=useHandoffRead(open&&!blocked&&decisionID&&decisionID!==selectedHandoff?handoffPath(project,taskId,decisionID):undefined,revision,decisionValid,deny);
 // A selected or edited decision requires its exact authorized source. A
 // successful history page cannot replace a failed or missing exact read.
 const exactTarget=decisionID===selectedHandoff?located.value:decisionRead.value,target=exactTarget?loadedTarget&&loadedTarget.version>exactTarget.version?loadedTarget:exactTarget:undefined;
 const dirty=!!recipient||!!team||!!note||!!decision,locked=action.busy||!!action.pending,selected=page?.recipients.find(item=>item.resource_id===recipient);
 useLeaveGuard((dirty||storageFailed||invalidDraft)&&!action.pending,action.setError);
 useEffect(()=>{if(invalidDraft)return;try{if(storage||dirty)saveHandoffDraft(storage,draftKey,draft);setStorageFailed(false);}catch{setStorageFailed(true);}},[storage,draftKey,draft,dirty,invalidDraft]);
 useEffect(()=>{if(selectedHandoff)setOpen(true);},[selectedHandoff]);
 const focused=useRef<HTMLElement|undefined>(undefined);
 useEffect(()=>{if(!selectedHandoff){focused.current=undefined;return;}const element=document.getElementById(bodyID+'-'+selectedHandoff);if(open&&!blocked&&located.value&&element&&element!==focused.current){element.focus();element.scrollIntoView?.({block:'nearest'});focused.current=element;}},[selectedHandoff,open,blocked,located.value,bodyID]);
 const writable=!blocked&&!invalidDraft&&!storageFailed&&!action.conflict&&!action.persistenceBlocked;
 const canCreate=writable&&!!page?.can_create&&page.task_version<2147483647&&draft.task_version===page.task_version&&!!selected&&(!team||selected.teams.some(item=>item.team_id===team))&&validHandoffText(note)&&!history.loading;
 const canDecide=writable&&!!decision&&!!target&&target.version<2147483647&&target.version===decision.version&&target.allowed_actions.includes(decision.action)&&(!decision.reason||validHandoffText(decision.reason))&&(decision.action==='accept'?acknowledged&&target.basis_current:validHandoffText(decision.reason))&&!history.loading&&!located.loading&&!decisionRead.loading;
 function proposal(change:Partial<HandoffDraft>){setDraft(before=>({...before,task_version:before.task_version??page?.task_version,...change}));}
 function discard(){try{saveHandoffDraft(storage,draftKey,emptyHandoffDraft());setDraft(emptyHandoffDraft());setInvalidDraft(false);setStorageFailed(false);setAcknowledged(false);}catch{setStorageFailed(true);}}
 async function confirmed(result:TaskHandoff|undefined,request:WriteRecoveryRequest){
  if(!result||!alive.current)return;setDraft(before=>draftAfterHandoffReceipt(before,request));setAcknowledged(false);setSuccess(result);setRefreshError(undefined);
  const results=await Promise.allSettled([history.read(),located.read(),decisionRead.read(),Promise.resolve().then(()=>callbacks.current.onChanged())]);
  if(alive.current&&results.some(value=>value.status==='rejected'))setRefreshError(new UiError('task-handoff.refresh_failed'));
 }
 async function create(){
  if(!canCreate||!page||locked)return;setSuccess(undefined);
  const body={project_id:project,task_id:taskId,expected_task_version:draft.task_version!,to_resource_id:recipient,to_team_id:team,note},path='/task-handoffs';
  await confirmed(await action.run<TaskHandoff>(path,body),{path,body,method:'POST',idempotency_key:''});
 }
 async function command(){
  if(!canDecide||!decision||locked)return;setSuccess(undefined);
  const path='/task-handoffs/'+encodeURIComponent(decision.id)+'/commands',body={project_id:project,expected_version:decision.version,action:decision.action,reason:decision.reason||text('accept_default_reason'),acknowledge_invalidation:decision.action==='accept'&&acknowledged};
  await confirmed(await action.run<TaskHandoff>(path,body),{path,body,method:'POST',idempotency_key:''});
 }
 async function rebase(){
  const [current,exact,other]=await Promise.all([history.read(),located.read(),decisionRead.read()]);if(!current)throw new UiError('task-handoff.refresh_failed');
  setDraft(before=>{const latest=[exact,other,...current.items].find(item=>item?.id===before.decision?.id);return {...before,task_version:current.task_version,...(before.decision&&latest?{decision:{...before.decision,version:latest.version}}:{})};});setAcknowledged(false);
 }
 function card(item:TaskHandoff,locatedCard=false){return <article key={item.id} id={locatedCard?bodyID+'-'+item.id:undefined} tabIndex={locatedCard?-1:undefined} className={'task-handoff-item'+(locatedCard?' is-located':'')} data-handoff-item data-handoff-located={locatedCard?item.id:undefined}>
  <div className="task-handoff-item-head"><strong>{item.to_name}</strong><span className={'task-handoff-state state-'+item.state}>{text('state.'+item.state)}</span></div>
  {item.to_team_name&&<small>{item.to_team_name}</small>}<time dateTime={item.created_at}>{formatDateTime(item.created_at)}</time>
  <p className="task-handoff-copy">{item.note}</p>{!!item.decision_reason&&<p className="task-handoff-decision-reason">{item.decision_reason}</p>}
  {!!item.blockers.length&&<ul className="task-handoff-blockers">{item.blockers.map((value,index)=><li key={index}>{renderDiagnostic(value)}</li>)}</ul>}<Basis item={item}/>
  <div className="task-handoff-actions">{(['accept','return','withdraw'] as const).filter(value=>item.allowed_actions.includes(value)).map(value=><button type="button" key={value} data-handoff-command={value} disabled={locked||invalidDraft||!!decision||value==='accept'&&!item.basis_current} onClick={()=>{setDraft(before=>({...before,decision:{id:item.id,version:item.version,action:value,reason:''}}));setAcknowledged(false);}}>{text('action.'+value)}</button>)}</div>
 </article>;}
 return <section className="detail-section task-handoff" aria-label={text('title')}>
  <h3><button type="button" className="task-handoff-toggle" data-handoff-toggle aria-expanded={open} aria-controls={bodyID} disabled={action.busy||open&&!!action.pending} onClick={()=>setOpen(value=>!value)}><ArrowRightLeft size={16} aria-hidden="true"/><span>{text('title')}</span><ChevronDown size={16} className={open?'is-open':''} aria-hidden="true"/></button></h3>
  {open&&<div id={bodyID} className="task-handoff-body" aria-busy={history.loading||located.loading}>
   <WriteNotice action={action} onRebase={!blocked?rebase:undefined}/>
   {blocked?<><ErrorNotice error={toolError(denial||action.error)}/><p className="task-handoff-hint" role="status">{text('private_cleared')}</p></>:<>
    <p className="task-handoff-hint">{text('intro')}</p>
    {(storageFailed||invalidDraft)&&<p className="task-handoff-hint" role="status">{text(invalidDraft?'invalid_draft':'storage_failed')}</p>}
    {(dirty||invalidDraft)&&<button type="button" className="task-handoff-discard" disabled={locked} data-handoff-discard onClick={discard}>{text('discard_draft')}</button>}
    {action.pending&&<div className="task-handoff-recovery" role="status"><strong>{text('original_operation')}</strong><p className="task-handoff-copy">{String(action.pending.body.note||action.pending.body.reason||'')}</p><button type="button" data-handoff-restore disabled={action.busy} onClick={()=>{const pending=action.pending;if(pending)void action.run<TaskHandoff>(pending.path,pending.body,pending.method).then(result=>confirmed(result,pending));}}>{text('restore')}</button></div>}
    {success&&<p className="task-handoff-success" role="status">{text('saved.'+success.state)}</p>}{Boolean(refreshError)&&<ErrorNotice error={refreshError}/>}
    {history.loading&&<Loading/>}{Boolean(history.error)&&<ErrorNotice error={toolError(history.error)} onRetry={()=>void history.read()}/>}
    {selectedHandoff&&<div className="task-handoff-located"><h4>{text('selected_handoff')}</h4>{located.loading&&<Loading/>}{Boolean(located.error)&&<ErrorNotice error={toolError(located.error)} onRetry={()=>void located.read()}/>}<>{located.value&&card(located.value,true)}</></div>}
    {page&&<>
     {!!page.create_blockers.length&&<div className="task-handoff-blockers"><span>{text('create_unavailable')}</span><ul>{page.create_blockers.map((value,index)=><li key={index}>{renderDiagnostic(value)}</li>)}</ul></div>}
     {!page.recipients.length&&<div className="task-handoff-empty"><p>{text('no_recipients')}</p><a href={'?'+new URLSearchParams({project,view:'settings',sub:'members'})}>{text('register_person')}</a></div>}
     {(page.can_create||recipient||note)&&<form data-handoff-create onSubmit={event=>{event.preventDefault();void create();}}>
      <h4>{text('propose')}</h4><WriteFields action={action}>
       <Field label={text('recipient')}><select name="recipient" value={recipient} required disabled={invalidDraft} onChange={event=>proposal({recipient:event.target.value,team:''})}><option value="">{text('choose_recipient')}</option>{recipient&&!selected&&<option value={recipient} disabled>{text('recipient_unavailable')}</option>}{page.recipients.map(item=><option key={item.resource_id} value={item.resource_id}>{item.name}</option>)}</select></Field>
       <Field label={text('team')}><select name="team" value={team} disabled={!selected||invalidDraft} onChange={event=>proposal({team:event.target.value})}><option value="">{text('no_team')}</option>{team&&!selected?.teams.some(item=>item.team_id===team)&&<option value={team} disabled>{text('recipient_unavailable')}</option>}{(selected?.teams||[]).map(item=><option key={item.team_id} value={item.team_id}>{item.name}</option>)}</select></Field>
       <Field label={text('note')}><textarea name="note" value={note} required rows={3} maxLength={4000} disabled={invalidDraft} onChange={event=>proposal({note:event.target.value})}/></Field>
       {!!note&&!validHandoffText(note)&&<p className="task-handoff-hint">{text('text_invalid')}</p>}
       <p className="task-handoff-hint">{text('task_version',{version:formatNumber(draft.task_version??page.task_version)})}</p>
       {draft.task_version!==undefined&&draft.task_version!==page.task_version&&<button type="button" disabled={locked} data-handoff-adopt-proposal onClick={()=>{proposal({task_version:page.task_version});setAcknowledged(false);}}>{text('adopt_task_version')}</button>}
      </WriteFields><WriteButton action={action} disabled={!canCreate}>{text('send')}</WriteButton>
     </form>}
     <div className="task-handoff-history"><div className="task-handoff-history-head"><h4>{text('history')}</h4><span>{formatNumber(page.items.length)} / {formatNumber(page.total)}</span></div>
      {!page.items.length&&<p className="task-handoff-hint">{text('empty')}</p>}{page.items.filter(item=>item.id!==located.value?.id).map(item=>card(item))}
      {page.next_cursor&&<button type="button" className="task-handoff-more" data-handoff-more disabled={history.loading||locked} onClick={()=>void history.read(true)}>{text('more')}</button>}
     </div>
    </>}
    {decision&&<form className="task-handoff-command" data-handoff-decision onSubmit={event=>{event.preventDefault();void command();}}>
     <h4>{text('action.'+decision.action)}</h4><WriteFields action={action}>
      {decision.action==='accept'&&<><p className="task-handoff-impact">{text('accept_impact')}</p><label className="task-handoff-ack"><input type="checkbox" name="acknowledge_invalidation" checked={acknowledged} onChange={event=>setAcknowledged(event.target.checked)}/><span>{text('accept_ack')}</span></label></>}
      <Field label={text(decision.action==='accept'?'accept_reason':'reason')}><textarea name="reason" value={decision.reason} required={decision.action!=='accept'} rows={3} maxLength={4000} onChange={event=>setDraft(before=>({...before,decision:{...decision,reason:event.target.value}}))}/></Field>
      {!!decision.reason&&!validHandoffText(decision.reason)&&<p className="task-handoff-hint">{text('text_invalid')}</p>}
      {(!target||target.version!==decision.version||!target.allowed_actions.includes(decision.action))&&<p className="task-handoff-hint">{text('decision_changed')}</p>}
      {target&&target.version!==decision.version&&target.allowed_actions.includes(decision.action)&&<button type="button" disabled={locked} data-handoff-adopt-decision onClick={()=>{setDraft(before=>({...before,decision:{...decision,version:target.version}}));setAcknowledged(false);}}>{text('adopt_decision_version')}</button>}
      {decisionRead.loading&&<Loading/>}{Boolean(decisionRead.error)&&<ErrorNotice error={toolError(decisionRead.error)} onRetry={()=>void decisionRead.read()}/>}
     </WriteFields><div className="task-handoff-actions"><WriteButton action={action} disabled={!canDecide}>{text('confirm.'+decision.action)}</WriteButton><button type="button" disabled={locked} onClick={()=>{setDraft(before=>({...before,decision:undefined}));setAcknowledged(false);}}>{text('cancel')}</button></div>
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
