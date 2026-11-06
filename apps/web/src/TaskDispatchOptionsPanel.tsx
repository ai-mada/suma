import {useEffect,useId,useImperativeHandle,useRef,useState,type Ref} from 'react';
import {ChevronDown,ChevronRight,CircleCheck,ListFilter} from 'lucide-react';
import type {TaskDispatchOptions,TaskDispatchToolOption} from '../../../packages/contracts/src';
import {useApi} from './api-context';
import {ApiError,type Session} from './api';
import {formatDateTime,formatNumber,tr,UiError,useLocale} from './i18n';
import {toolReason} from './ToolFacts';
import {toolError} from './tool-hooks';
import type {ToolProfile} from './tool-model';
import {dispatchOptionsPath,dispatchOptionsScopeKey,selectedDispatchOption,validDispatchOptions,type DispatchOptionsScope} from './task-dispatch-options-model';
import {ErrorNotice} from './ui';
import {DispatchPageNavigation,firstDispatchPage} from './DispatchPageNavigation';
import './CandidateCompositionPanel.css';
import './TaskDispatchOptionsPanel.css';

const t=(key:string,values?:Record<string,unknown>)=>tr('task-dispatch-options.'+key,values);
export type TaskDispatchOptionsHandle={open:()=>void};
type Props={
 session:Session;
 scope:DispatchOptionsScope;
 disabled?:boolean;
 panelRef?:Ref<TaskDispatchOptionsHandle>;
 onAdopt:(tool:ToolProfile)=>void;
 onUnavailable:()=>void;
};
export function TaskDispatchOptionsPanel(props:Props){
 // Remount every identity/basis, including the user's organization, before a
 // stale page can render or adopt into a different task's selector.
 return <OptionsScope key={JSON.stringify([props.session.user.id,props.session.organization.id,dispatchOptionsScopeKey(props.scope)])} {...props}/>;
}
function OptionsScope({session,scope,disabled=false,panelRef,onAdopt,onUnavailable}:Props){
 useLocale();const api=useApi(),bodyID=useId(),[open,setOpen]=useState(false),[page,setPage]=useState<TaskDispatchOptions>(),[cursorTrail,setCursorTrail]=useState(firstDispatchPage),[loading,setLoading]=useState(false),[error,setError]=useState<unknown>(),epoch=useRef(0),controller=useRef<AbortController|undefined>(undefined),unavailable=useRef(onUnavailable);
 const toggleButton=useRef<HTMLButtonElement>(null);
 const opened=useRef(false);
 unavailable.current=onUnavailable;
 useEffect(()=>()=>{epoch.current++;controller.current?.abort();},[]);
 async function read(trail:readonly string[]){
  if(disabled||controller.current)return;
  const before=trail[trail.length-1];
  const request=++epoch.current;
  const abort=new AbortController();
  controller.current=abort;
  // Keep the requested cursor for recovery without exposing an older page.
  setCursorTrail(trail);setPage(undefined);setError(undefined);setLoading(true);
  try{
   const value=await api<unknown>(dispatchOptionsPath(scope,before),undefined,'GET',undefined,abort.signal);
   if(request!==epoch.current||abort.signal.aborted)return;
   if(!validDispatchOptions(value,scope,session.organization.id)||(value.next_cursor&&trail.includes(value.next_cursor)))throw new UiError('task-dispatch-options.invalid_read');
   setPage(value);
  }catch(problem){
   if(request!==epoch.current||abort.signal.aborted)return;
   setPage(undefined);setError(problem);
   if(problem instanceof ApiError&&[401,403,404].includes(problem.status)){
    setCursorTrail(firstDispatchPage);
    unavailable.current();
   }
  }finally{
   if(controller.current===abort){
    controller.current=undefined;
    if(request===epoch.current&&!abort.signal.aborted)setLoading(false);
   }
  }
 }
 function openPanel(){
  if(disabled||opened.current)return;
  // Set the guard before starting the read so two activations in one frame
  // cannot send duplicate requests.
  opened.current=true;
  setOpen(true);
  void read(firstDispatchPage);
 }
 function toggle(){
  if(disabled)return;
  if(!opened.current){openPanel();return;}
  opened.current=false;
  epoch.current++;
  controller.current?.abort();
  controller.current=undefined;
  setLoading(false);
  setPage(undefined);
  setError(undefined);
  setOpen(false);
 }
 useImperativeHandle(panelRef,()=>({
  open(){
   if(disabled||!toggleButton.current)return;
   toggleButton.current.focus();
   openPanel();
  },
 }));
 const recommended=page&&selectedDispatchOption(page);
 const compatible=page?.items.filter(option=>option.eligible)??[];
 const excluded=page?.items.filter(option=>!option.eligible)??[];
 return <section className="candidate-composition task-dispatch-options">
  <button ref={toggleButton} type="button" data-dispatch-options-toggle className="composition-toggle" aria-expanded={open} aria-controls={bodyID} disabled={disabled} onClick={toggle}><ListFilter size={16} aria-hidden="true"/><span>{t('title')}</span><ChevronDown size={16} aria-hidden="true"/></button>
  {open&&<div id={bodyID} className="composition-body">
   <p className="composition-muted">{t('intro')}</p>
   <DispatchPageNavigation label={t('pagination')} number={cursorTrail.length} loading={loading} failed={!!error}
    summary={page&&t('page_count',{count:formatNumber(page.items.length),total:formatNumber(page.total)})}
    onFirst={!disabled&&!loading&&cursorTrail.length>1?()=>void read(firstDispatchPage):undefined}
    onPrevious={!disabled&&!loading&&cursorTrail.length>1?()=>void read(cursorTrail.slice(0,-1)):undefined}
    onNext={!disabled&&!loading&&page?.next_cursor?()=>void read([...cursorTrail,page.next_cursor]):undefined}
    onRefresh={!disabled&&!loading?()=>void read(cursorTrail):undefined}/>
   {!!error&&<ErrorNotice error={toolError(error)}/>}
   {page&&<>
    <p className="composition-muted">{t('observed',{time:formatDateTime(page.observed_at,{year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',second:'2-digit'})})}</p>
    {!page.items.length?<p data-dispatch-options-empty>{t('empty')}</p>:<>
     {recommended?<p className="composition-muted">{t('recommended',{name:recommended.name})}</p>:<p className="dispatch-tool-options-empty">{t(page.next_cursor?'none_eligible':'none_eligible_last')}</p>}
     {compatible.length>0&&<ul className="dispatch-tool-option-list" role="list" aria-label={t('tools')}>
      {compatible.map(option=><DispatchToolOptionRow key={option.tool.id} option={option} recommended={option.tool.id===recommended?.id} disabled={disabled||loading} onAdopt={onAdopt}/>)}
     </ul>}
     {excluded.length>0&&<details className="dispatch-tool-options-exclusions" data-dispatch-options-exclusions>
      <summary>
       <ChevronRight size={16} aria-hidden="true"/>
       <span>{t('exclusions',{count:formatNumber(excluded.length)})}</span>
      </summary>
      <ul className="dispatch-tool-option-list" role="list" aria-label={t('excluded_tools')}>
       {excluded.map(option=><DispatchToolOptionRow key={option.tool.id} option={option} recommended={false} disabled={disabled||loading} onAdopt={onAdopt}/>)}
      </ul>
     </details>}
    </>}
    <details><summary>{t('boundaries')}</summary><dl className="composition-facts"><dt>{t('route')}</dt><dd>{page.execution_route.pool_id?t('route_pool',{pool:page.execution_route.pool_id}):t('route_local')}</dd><dt>{t('model')}</dt><dd>{t('model_depends_on_tool')}</dd><dt>{t('cost')}</dt><dd>{t('cost_unknown')}</dd></dl><p className="composition-muted">{t('recheck')}</p></details>
   </>}
  </div>}
 </section>;
}

function DispatchToolOptionRow({option,recommended,disabled,onAdopt}:{
 option:TaskDispatchToolOption;
 recommended:boolean;
 disabled:boolean;
 onAdopt:(tool:ToolProfile)=>void;
}){
 const {tool,eligible}=option;
 const action=t(recommended?'adopt':'adopt_option');
 return <li data-dispatch-option={tool.id}>
  <div className="dispatch-tool-option-main">
   <strong>{tool.name}</strong>
   <p className="composition-muted">{t('tool_basis',{worker:tool.worker_id,version:formatNumber(tool.version),toolVersion:tool.profile.tool_version})}</p>
  </div>
  <div className="dispatch-tool-option-actions">
   <span className="dispatch-tool-option-state">
    {eligible&&<CircleCheck size={15} aria-hidden="true"/>}{t('reason_'+option.reason_code)}
   </span>
   {eligible&&<button type="button" data-dispatch-options-adopt={recommended?'':undefined} aria-label={t('adopt_named',{action,name:tool.name})} disabled={disabled} onClick={()=>onAdopt(tool)}>{action}</button>}
  </div>
  {!eligible&&option.reason&&<p className="dispatch-tool-option-reason">{toolReason(option.reason,option.reason_diagnostic)}</p>}
 </li>;
}
