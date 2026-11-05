import {useEffect,useRef,useState} from 'react';
import {Cloud,RefreshCw} from 'lucide-react';
import {api,ApiError,operation,time,stateLabel} from './api';
import {tr,useLocale,UiError} from './i18n';
import {ErrorNotice,Field,Modal} from './ui';
import type {TaskViewPreferences,TaskViewScope} from './task-views-storage';
import {taskColumns} from './task-columns-model';
import './SharedTaskViews.css';

export type SharedTaskView={id:string;project_id:string;owner_id:string;name:string;visibility:'private'|'project';version:number;state:'active'|'archived';preferences:TaskViewPreferences;created_at:string;updated_at:string;can_edit:boolean;can_delete:boolean};
type Page={project_id:string;items:SharedTaskView[];total:number;limit:number;next_cursor?:string;as_of:string};
type Props={scope:TaskViewScope;preferences:TaskViewPreferences;onChange:(value:TaskViewPreferences)=>boolean;denied?:boolean};
type Draft={name:string;visibility:'private'|'project';preferences:TaskViewPreferences;editing?:SharedTaskView;remove?:boolean;unavailable?:boolean};
type Write={path:string;method:string;body:Record<string,unknown>;remove:boolean;id?:string};
const deniedError=(error:unknown)=>error instanceof ApiError&&[401,403,404].includes(error.status);
const comparable=(p:TaskViewPreferences)=>JSON.stringify([p.q,p.status,p.owner_id,p.priority,p.requirement_id||'',!!p.unassigned,!!p.priority_unset,!!p.status_unset,p.iteration_id||'',!!p.iteration_unset,p.sort,p.layout,p.group_by,taskColumns(p.columns)]);
/** Project only the saved-view contract; route object/detail state is never uploaded. */
export function sharedViewPreferences(p:TaskViewPreferences):TaskViewPreferences{if(!taskColumns(p.columns))throw new UiError('task-columns.invalid');return {...(p.columns===undefined?{}:{columns:[...p.columns]}),q:p.q,status:p.status,owner_id:p.owner_id,priority:p.priority,sort:p.sort,layout:p.layout,group_by:p.group_by,requirement_id:p.requirement_id||undefined,iteration_id:p.iteration_id||undefined,iteration_unset:p.iteration_unset||undefined,unassigned:p.unassigned||undefined,priority_unset:p.priority_unset||undefined,status_unset:p.status_unset||undefined};}
type AppliedView={id:string;version:number;preferences:TaskViewPreferences};
type SyncPhase='checking'|'current'|'stale'|'offline';
const isOnline=()=>navigator.onLine!==false;
function requireView(row:SharedTaskView,project:string,id?:string){
 if(!row||typeof row.id!=='string'||!row.id||row.project_id!==project||(id!==undefined&&row.id!==id)||typeof row.name!=='string'||!Number.isSafeInteger(row.version)||row.version<1||typeof row.can_edit!=='boolean'||typeof row.can_delete!=='boolean'||!['active','archived'].includes(row.state)||!['project','private'].includes(row.visibility))throw new UiError('shared-task-views.invalid_response');
 if(!row.preferences||typeof row.preferences!=='object'||Array.isArray(row.preferences)||!taskColumns(row.preferences.columns))throw new UiError('task-columns.invalid_response');
}
function readRevision(raw:unknown,project:string){
 if(!raw||typeof raw!=='object'||Array.isArray(raw))throw new UiError('shared-task-views.invalid_response');
 const marker=raw as Record<string,unknown>;
 if(Object.keys(marker).length!==2||marker.project_id!==project||typeof marker.revision!=='string'||!/^[a-f0-9]{64}$/.test(marker.revision))throw new UiError('shared-task-views.invalid_response');
 return marker.revision;
}
export function SharedTaskViews(props:Props){return <ScopedSharedTaskViews key={JSON.stringify(props.scope)} {...props}/>;}
function ScopedSharedTaskViews({scope,preferences,onChange,denied=false}:Props){
 useLocale();
 const project=scope[2],query='?project_id='+encodeURIComponent(project);
 const identity={'X-Suma-Expected-User':scope[0],'X-Suma-Expected-Organization':scope[1]};
 const [rows,setRows]=useState<SharedTaskView[]>([]),[cursor,setCursor]=useState('');
 const [applied,setApplied]=useState<AppliedView|null>(null),[pinned,setPinned]=useState<SharedTaskView|null>(null);
 const [unavailable,setUnavailable]=useState(false),[phase,setPhase]=useState<SyncPhase>('checking');
 const [loading,setLoading]=useState(false),[error,setError]=useState<unknown>(null),[blocked,setBlocked]=useState(false);
 const [draft,setDraft]=useState<Draft|null>(null),[writeError,setWriteError]=useState<unknown>(null);
 const [busy,setBusy]=useState(false),[pending,setPendingState]=useState<Write|null>(null),[conflict,setConflict]=useState(false),[notice,setNotice]=useState('');
 const container=useRef<HTMLDivElement>(null),live=useRef(true),accessBlocked=useRef(false);
 const read=useRef<AbortController|null>(null),write=useRef<AbortController|null>(null),writing=useRef(false);
 const lastRevision=useRef<string|undefined>(undefined);
 const selection=useRef(applied),editor=useRef(draft),pendingWrite=useRef(pending);
 const setPending=(command:Write|null)=>{pendingWrite.current=command;setPendingState(command);};
 selection.current=applied;
 editor.current=draft;
 const active=rows.find(row=>row.id===applied?.id)??(pinned?.id===applied?.id?pinned:null);
 const locked=denied||blocked,offline=phase==='offline';
 const update=active&&applied&&active.version>applied.version?active:null;
 const editingCurrent=draft?.editing&&(rows.find(row=>row.id===draft.editing!.id)??(pinned?.id===draft.editing.id?pinned:null));
 const editorComparison=editingCurrent&&draft?.editing&&editingCurrent.version>=draft.editing.version?editingCurrent:draft?.editing;
 const draftRightsLost=!!draft?.editing&&!(draft.remove?editorComparison?.can_delete:editorComparison?.can_edit);
 const clearSelection=()=>{selection.current=null;setApplied(null);setPinned(null);};
 const stopRead=()=>{read.current?.abort();read.current=null;setLoading(false);};
 const clear=()=>{
  stopRead();write.current?.abort();write.current=null;writing.current=false;lastRevision.current=undefined;
  accessBlocked.current=true;setRows([]);setCursor('');clearSelection();setDraft(null);setPending(null);
  setWriteError(null);setBusy(false);setConflict(false);setUnavailable(false);setNotice('');setBlocked(true);
 };
 const retireView=(id:string)=>{
  setRows(old=>old.filter(row=>row.id!==id));
  if(selection.current?.id===id){clearSelection();setUnavailable(true);}
  setDraft(old=>{
   if(old?.editing?.id!==id)return old;
   return old.remove?null:{...old,editing:undefined,unavailable:true};
  });
 };
 const choose=(row:SharedTaskView)=>{
  const next=sharedViewPreferences(row.preferences);
  if(!onChange(next))return;
  const snapshot={id:row.id,version:row.version,preferences:next};
  selection.current=snapshot;
  setApplied(snapshot);setPinned(row);setUnavailable(false);
 };
 const fetchView=async(id:string,signal:AbortSignal)=>{
  try{
   const value=await api<SharedTaskView>('/task-views/'+encodeURIComponent(id)+query,undefined,'GET',identity,signal);
   requireView(value,project,id);
   return value.state==='active'?value:null;
  }catch(caught){
   if(caught instanceof ApiError&&caught.status===404)return null;
   throw caught;
  }
 };
 const load=async(kind:'auto'|'refresh'|'more'='auto')=>{
  if(denied||read.current||writing.current||pendingWrite.current||kind==='auto'&&accessBlocked.current)return;
  if(!isOnline()){setPhase('offline');return;}
  const controller=new AbortController();read.current=controller;
  const signal=controller.signal,current=()=>live.current&&!signal.aborted&&read.current===controller;
  if(kind==='refresh'){setLoading(true);setError(null);}
  try{
   let revision=lastRevision.current;
   if(kind!=='more'){
    const marker=await api<unknown>('/task-views/sync'+query,undefined,'GET',identity,signal);
    if(!current())return;
    revision=readRevision(marker,project);
    if(kind==='auto'&&revision===lastRevision.current){setError(null);setPhase('current');return;}
   }
   setLoading(true);
   const result=await api<Page>('/task-views'+query+'&limit=20'+(kind==='more'&&cursor?'&cursor='+encodeURIComponent(cursor):''),undefined,'GET',identity,signal);
   if(!current())return;
   if(!result||result.project_id!==project||!Array.isArray(result.items))throw new UiError('shared-task-views.invalid_response');
   const loaded=new Map<string,SharedTaskView>();
   for(const row of result.items){
    requireView(row,project);
    if(row.state!=='active'||loaded.has(row.id))throw new UiError('shared-task-views.invalid_response');
    loaded.set(row.id,row);
   }
   if(kind==='more'){
    setRows(old=>[...old,...result.items.filter(row=>!old.some(previous=>previous.id===row.id))]);
    setCursor(result.next_cursor||'');
    setError(null);
    return;
   }
   // A marker is acquired before listing. Saving it only after every
   // successful read keeps a concurrent update visible on the next check.
   const currentIds=new Set([selection.current?.id,editor.current?.editing?.id]);
   const exact=new Map<string,SharedTaskView|null>();
   for(const id of currentIds){
    if(id&&!loaded.has(id))exact.set(id,await fetchView(id,signal));
    if(!current())return;
   }
   const selectionID=selection.current?.id,editingID=editor.current?.editing?.id;
   const relevant=(id:string)=>loaded.get(id)??exact.get(id);
   const next=selectionID?relevant(selectionID):undefined;
   const editorView=editingID?relevant(editingID):undefined;
   // A selection can change while the exact read is in flight. Retain the
   // old marker so the next pass checks any newly pinned view before applying.
   const selectionMoved=!!selectionID&&!currentIds.has(selectionID)&&!loaded.has(selectionID);
   const editorMoved=!!editingID&&!currentIds.has(editingID)&&!loaded.has(editingID);
   if(!selectionMoved&&!editorMoved)lastRevision.current=revision;
   setRows(result.items);setCursor(result.next_cursor||'');
   if(next===null&&selectionID)retireView(selectionID);
   else if(next&&selectionID)setPinned(next);
   if(editorView===null&&editingID&&editingID!==selectionID)retireView(editingID);
   if(editorView&&editor.current?.editing&&editorView.version>editor.current.editing.version)setConflict(true);
   setError(null);accessBlocked.current=false;setBlocked(false);setPhase('current');
  }catch(caught){
   if(!current())return;
   if(deniedError(caught)){clear();setError(caught);}
   else if(kind!=='auto'||lastRevision.current===undefined)setError(caught);
   setPhase(!isOnline()?'offline':'stale');
  }finally{
   if(current()){read.current=null;setLoading(false);}
  }
 };
 const loadLatest=useRef(load);
 loadLatest.current=load;
 useEffect(()=>{
  live.current=true;
  let intersecting=typeof IntersectionObserver==='undefined';
  let timer:ReturnType<typeof setInterval>|undefined;
  const check=()=>{
   if(intersecting&&document.visibilityState!=='hidden'&&isOnline())void loadLatest.current();
  };
  const reschedule=()=>{
   clearInterval(timer);
   if(!intersecting||document.visibilityState==='hidden'||!isOnline()){
    stopRead();
    if(!isOnline())setPhase('offline');
    return;
   }
   check();
   timer=setInterval(check,15000);
  };
  const observer=typeof IntersectionObserver==='undefined'?undefined:new IntersectionObserver(entries=>{
   const visible=entries.some(entry=>entry.target===container.current&&entry.isIntersecting);
   if(intersecting!==visible){intersecting=visible;reschedule();}
  });
  if(container.current)observer?.observe(container.current);
  reschedule();
  document.addEventListener('visibilitychange',reschedule);
  window.addEventListener('focus',check);
  window.addEventListener('online',reschedule);
  window.addEventListener('offline',reschedule);
  return ()=>{
   live.current=false;clearInterval(timer);observer?.disconnect();
   document.removeEventListener('visibilitychange',reschedule);
   window.removeEventListener('focus',check);
   window.removeEventListener('online',reschedule);
   window.removeEventListener('offline',reschedule);
   read.current?.abort();read.current=null;
   write.current?.abort();write.current=null;
  };
 },[]);
 useEffect(()=>{if(denied)clear();},[denied]);
 const open=(editing?:SharedTaskView,remove=false)=>{
  setDraft({name:editing?.name||active?.name||'',visibility:editing?.visibility||'private',preferences:sharedViewPreferences(remove&&editing?editing.preferences:preferences),editing,remove});
  setWriteError(null);setConflict(false);setNotice('');
 };
 const perform=async(command:Write)=>{
  if(writing.current||locked||!isOnline())return;
  writing.current=true;stopRead();
  const controller=new AbortController();write.current=controller;
  const signal=controller.signal,current=()=>live.current&&!signal.aborted&&write.current===controller;
  setBusy(true);setWriteError(null);
  try{
   const row=await api<SharedTaskView>(command.path,command.body,command.method,identity,signal);
   if(!current())return;
   requireView(row,project,command.id);
   if(row.state!==(command.remove?'archived':'active'))throw new UiError('shared-task-views.invalid_response');
   setPending(null);setDraft(null);setConflict(false);
   setRows(old=>command.remove?old.filter(item=>item.id!==row.id):[row,...old.filter(item=>item.id!==row.id)]);
   if(command.remove)clearSelection();
   else{
    const snapshot={id:row.id,version:row.version,preferences:sharedViewPreferences(row.preferences)};
    selection.current=snapshot;setApplied(snapshot);setPinned(row);
   }
   setUnavailable(false);setNotice(tr(command.remove?'shared-task-views.deleted':'shared-task-views.saved'));
  }catch(caught){
   if(!current())return;
   if(deniedError(caught)){clear();setError(caught);}
   else{
    setWriteError(caught);
    const unknown=!(caught instanceof ApiError)||caught.status===0||caught.status>=500;
    setPending(unknown?command:null);
    setConflict(caught instanceof ApiError&&caught.status===409);
   }
  }finally{
   if(current()){write.current=null;writing.current=false;setBusy(false);}
  }
 };
 const save=()=>{
  if(!draft||draft.unavailable||draftRightsLost||busy||pending||conflict||offline)return;
  const name=draft.name.trim();
  if(!name||Array.from(name).length>80){setWriteError(new Error(tr('shared-task-views.invalid')));return;}
  const base={project_id:project,idempotency_key:operation()},id=draft.editing?.id;
  const body=draft.remove?{...base,expected_version:draft.editing!.version}:{...base,name,visibility:draft.visibility,preferences:draft.preferences,...(id?{expected_version:draft.editing!.version}:{})};
  void perform({path:'/task-views'+(id?'/'+encodeURIComponent(id):'')+(draft.remove?'/archive':''),method:id&&!draft.remove?'PATCH':'POST',body,remove:!!draft.remove,id});
 };
 const latest=async()=>{
  if(!draft?.editing||writing.current||offline||pending)return;
  const id=draft.editing.id;
  writing.current=true;stopRead();
  const controller=new AbortController();write.current=controller;
  const signal=controller.signal,current=()=>live.current&&!signal.aborted&&write.current===controller;
  setBusy(true);setWriteError(null);
  try{
   const row=await fetchView(id,signal);
   if(!current())return;
   if(!row){retireView(id);setConflict(false);return;}
   setRows(old=>[row,...old.filter(item=>item.id!==row.id)]);
   setDraft(old=>old?{...old,editing:row}:old);
   setConflict(false);setNotice(tr('shared-task-views.review'));
  }catch(caught){
   if(!current())return;
   if(deniedError(caught)){clear();setError(caught);}
   else setWriteError(caught);
  }finally{
   if(current()){write.current=null;writing.current=false;setBusy(false);}
  }
 };
 const copyDraft=()=>{
  if(!draft)return;
  setDraft({...draft,editing:undefined,visibility:'private',unavailable:false});
  setConflict(false);setWriteError(null);setNotice('');
 };
 const shownRows=active&&!rows.some(row=>row.id===active.id)?[...rows,active]:rows;
 const syncState=locked?'denied':pending?'pending':phase;
 return <div ref={container} className="shared-task-views" aria-busy={loading}>
  <div className="shared-task-views-bar">
   <label className="shared-task-views-picker"><Cloud size={17}/><span>{tr('shared-task-views.title')}</span>
    <select aria-label={tr('shared-task-views.title')} value={active?.id||''} disabled={locked||busy||!!pending} onChange={event=>{
     const row=shownRows.find(item=>item.id===event.target.value);
     if(row)choose(row);else{clearSelection();setUnavailable(false);}
    }}>
     <option value="">{tr('shared-task-views.custom')}</option>
     {(['private','project'] as const).map(visibility=><optgroup key={visibility} label={tr('shared-task-views.'+visibility)}>{shownRows.filter(row=>row.visibility===visibility&&row.state==='active').map(row=><option key={row.id} value={row.id}>{row.name}</option>)}</optgroup>)}
    </select>
   </label>
   <button type="button" disabled={locked||offline||busy||!!pending||!taskColumns(preferences.columns)} onClick={()=>open()}>{tr('shared-task-views.save')}</button>
   <button type="button" data-shared-view-refresh disabled={denied||offline||loading||busy||!!pending} aria-label={tr('shared-task-views.refresh')} onClick={()=>void load('refresh')}><RefreshCw size={16}/>{tr('shared-task-views.refresh')}</button>
  </div>
  <p className="shared-task-views-sync" data-shared-view-sync-state={syncState} role="status">
   <span className="shared-task-views-sync-dot" aria-hidden="true"/>{tr('shared-task-views.sync_'+syncState)}
  </p>
  {unavailable&&<p data-shared-view-unavailable role="status" className="shared-task-views-status">{tr('shared-task-views.unavailable')}</p>}
  {active&&<><div className="shared-task-views-meta"><span>{tr('shared-task-views.'+active.visibility)}</span><span>{tr('shared-task-views.version',{version:applied?.version??active.version})}</span>{!update&&<span>{time(active.updated_at)}</span>}<span>{tr('shared-task-views.owner',{owner:active.owner_id})}</span></div>
   {update&&<section className="shared-task-views-update" data-shared-view-update-notice>
    <div className="shared-task-views-update-heading"><div><strong>{tr('shared-task-views.update_available',{version:update.version})}</strong><p>{tr('shared-task-views.update_hint')}</p></div>
     <button type="button" data-shared-view-apply-update disabled={locked||phase!=='current'||busy||!!pending||!!draft} onClick={()=>choose(update)}>{tr('shared-task-views.apply_update')}</button>
    </div>
    <details className="shared-task-views-comparison"><summary>{tr('shared-task-views.compare')}</summary>
     <div><h4>{tr('shared-task-views.current_filters')}</h4><PreferenceSummary value={preferences}/></div>
     <div><h4>{tr('shared-task-views.server')}</h4><strong>{update.name}</strong><PreferenceSummary value={update.preferences}/></div>
    </details>
   </section>}
   <div className="shared-task-views-actions">
    {active.can_edit&&<button disabled={locked||offline||busy||!!pending||!!update||!taskColumns(preferences.columns)} onClick={()=>open(active)}>{tr('shared-task-views.edit')}</button>}
    <button disabled={locked||offline||busy||!!pending||!taskColumns(preferences.columns)} onClick={()=>open()}>{tr('shared-task-views.copy')}</button>
    {active.can_delete&&<button disabled={locked||offline||busy||!!pending||!!update} onClick={()=>open(active,true)}>{tr('shared-task-views.delete')}</button>}
   </div>
   {!update&&applied&&comparable(applied.preferences)!==comparable(preferences)&&<p className="shared-task-views-note">{tr('shared-task-views.changed')}</p>}
  </>}
  {!loading&&!locked&&!error&&phase==='current'&&rows.length===0&&<p className="shared-task-views-note">{tr('shared-task-views.empty')}</p>}
  {cursor&&<button disabled={locked||offline||loading||busy||!!pending} onClick={()=>void load('more')}>{tr('shared-task-views.more')}</button>}
  {locked&&<p role="alert" className="shared-task-views-status">{tr('shared-task-views.denied')}</p>}{!!error&&<ErrorNotice error={error}/>}
  {notice&&!draft&&<p role="status" className="shared-task-views-status">{notice}</p>}
  <Modal open={!!draft} onClose={()=>{if(!busy&&!pending)setDraft(null);}} title={tr(draft?.remove?'shared-task-views.delete':draft?.editing||draft?.unavailable?'shared-task-views.edit':'shared-task-views.save')} description={tr(draft?.remove?'shared-task-views.delete_hint':'shared-task-views.scope_hint')}>
   {draft&&<form className="shared-task-views-form" onSubmit={event=>{event.preventDefault();save();}}>
    {!draft.remove&&<><Field label={tr('shared-task-views.name')}><input autoFocus required maxLength={160} value={draft.name} disabled={busy||!!pending} onChange={event=>setDraft({...draft,name:event.target.value})}/></Field>
    <Field label={tr('shared-task-views.visibility')}><select value={draft.visibility} disabled={busy||!!pending||!!draft.editing||!!draft.unavailable} onChange={event=>setDraft({...draft,visibility:event.target.value as Draft['visibility']})}><option value="private">{tr('shared-task-views.private')}</option><option value="project">{tr('shared-task-views.project')}</option></select></Field></>}
    {draft.remove&&<p>{draft.name}</p>}{!!writeError&&<ErrorNotice error={writeError}/>}
    {pending&&<div className="shared-task-views-conflict" role="status"><p>{tr('shared-task-views.unknown')}</p><button type="button" disabled={busy||offline} onClick={()=>void perform(pending)}>{tr('shared-task-views.retry')}</button></div>}
    {draft.unavailable||draftRightsLost?<div className="shared-task-views-conflict" role="status"><p>{tr(draft.unavailable?'shared-task-views.draft_unavailable':'shared-task-views.draft_readonly')}</p>{!draft.remove&&<button type="button" disabled={busy} onClick={copyDraft}>{tr('shared-task-views.copy')}</button>}</div>:
     conflict&&<div className="shared-task-views-conflict"><p>{tr('shared-task-views.conflict')}</p><div className="inline">
      <button type="button" disabled={busy||offline} onClick={()=>void latest()}>{tr('shared-task-views.reload')}</button>
      {!draft.remove&&<button type="button" disabled={busy} onClick={copyDraft}>{tr('shared-task-views.copy')}</button>}
     </div></div>}
    {notice&&<p role="status">{notice}</p>}
    <details className="shared-task-views-comparison" open={Boolean(notice||conflict&&editorComparison)}><summary>{tr('shared-task-views.compare')}</summary>
     {editorComparison&&<div><h4>{tr('shared-task-views.server')}</h4><strong>{editorComparison.name}</strong><PreferenceSummary value={editorComparison.preferences}/></div>}
     <div><h4>{tr('shared-task-views.mine')}</h4><strong>{draft.name}</strong><PreferenceSummary value={draft.preferences}/></div>
    </details>
    <div className="dialog-actions"><button type="button" disabled={busy||!!pending} onClick={()=>setDraft(null)}>{tr('shared-task-views.cancel')}</button>
     <button className={draft.remove?'danger':''} disabled={busy||offline||!!pending||conflict||locked||!!draft.unavailable||draftRightsLost}>{tr(busy?'shared-task-views.saving':draft.remove?'shared-task-views.delete':notice&&draft.editing?'shared-task-views.replace':'shared-task-views.confirm')}</button>
    </div>
   </form>}
  </Modal>
 </div>;
}

function PreferenceSummary({value}: {value:TaskViewPreferences}){
 const values=[['task-views.search',value.q],['task-views.business_status',value.status_unset?tr('task-views.unspecified'):value.status?stateLabel(value.status):''],['records.assignee',value.unassigned?tr('records.unassigned'):value.owner_id],['records.priority',value.priority_unset?tr('task-views.unspecified'):value.priority?(['urgent','high','medium','low'].includes(value.priority)?tr('common.priority.'+value.priority):value.priority):''],['records.linked_requirement',value.requirement_id],['task-collection.iteration',value.iteration_unset?tr('task-collection.iteration_unset'):value.iteration_id],['task-views.sort',tr('task-views.sort_'+value.sort)],['task-views.group',tr(value.group_by==='iteration'?'task-collection.group_iteration':'task-views.group_'+value.group_by)],['task-views.layout',tr(value.layout==='list'?'records.list_view':'records.board_view')],['task-columns.title',taskColumns(value.columns)?.map(id=>tr('task-columns.column.'+id)).join(' · ')||tr('task-columns.invalid')]];
 return <dl>{values.map(([key,text])=><div key={key}><dt>{tr(key!)}</dt><dd>{text||tr('shared-task-views.any')}</dd></div>)}</dl>;
}
