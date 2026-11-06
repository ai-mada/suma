import {useEffect,useId,useRef,useState} from 'react';
import {ChevronDown,ListFilter} from 'lucide-react';
import type {TaskDispatchOptions} from '../../../packages/contracts/src';
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
type Props={session:Session;scope:DispatchOptionsScope;disabled?:boolean;onAdopt:(tool:ToolProfile)=>void;onUnavailable:()=>void};
export function TaskDispatchOptionsPanel(props:Props){
 // Remount every identity/basis, including the user's organization, before a
 // stale page can render or adopt into a different task's selector.
 return <OptionsScope key={JSON.stringify([props.session.user.id,props.session.organization.id,dispatchOptionsScopeKey(props.scope)])} {...props}/>;
}
function OptionsScope({session,scope,disabled=false,onAdopt,onUnavailable}:Props){
 useLocale();const api=useApi(),bodyID=useId(),[open,setOpen]=useState(false),[page,setPage]=useState<TaskDispatchOptions>(),[cursorTrail,setCursorTrail]=useState(firstDispatchPage),[loading,setLoading]=useState(false),[error,setError]=useState<unknown>(),epoch=useRef(0),controller=useRef<AbortController|undefined>(undefined),unavailable=useRef(onUnavailable);
 unavailable.current=onUnavailable;
 useEffect(()=>()=>{epoch.current++;controller.current?.abort();},[]);
 async function read(trail:readonly string[]){
  const before=trail[trail.length-1];
  const request=++epoch.current;controller.current?.abort();const abort=new AbortController();controller.current=abort;
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
  }finally{if(request===epoch.current&&!abort.signal.aborted)setLoading(false);}
 }
 function toggle(){
  if(open){epoch.current++;controller.current?.abort();setLoading(false);setPage(undefined);setError(undefined);setOpen(false);}
  else{setOpen(true);void read(firstDispatchPage);}
 }
 const recommended=page&&selectedDispatchOption(page);
 return <section className="candidate-composition task-dispatch-options">
  <button type="button" data-dispatch-options-toggle className="composition-toggle" aria-expanded={open} aria-controls={bodyID} disabled={disabled} onClick={toggle}><ListFilter size={16}/><span>{t('title')}</span><ChevronDown size={16}/></button>
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
     {recommended?<p className="composition-muted">{t('recommended',{name:recommended.name})}</p>:<p>{t('none_eligible')}</p>}
     <ul className="composition-run-history" aria-label={t('tools')}>
      {page.items.map(option=>{
       const isRecommended=option.tool.id===recommended?.id;
       const action=t(isRecommended?'adopt':'adopt_option');
       return <li key={option.tool.id} data-dispatch-option={option.tool.id}>
        <div className="composition-heading">
         <strong>{option.tool.name}</strong>
         <div className="composition-actions">
          <span>{t('reason_'+option.reason_code)}</span>
          {option.eligible&&<button type="button" data-dispatch-options-adopt={isRecommended?'':undefined} aria-label={t('adopt_named',{action,name:option.tool.name})} disabled={disabled||loading} onClick={()=>onAdopt(option.tool)}>{action}</button>}
         </div>
        </div>
        <p className="composition-muted">{t('tool_basis',{worker:option.tool.worker_id,version:formatNumber(option.tool.version),toolVersion:option.tool.profile.tool_version})}</p>
        {!option.eligible&&option.reason&&<p>{toolReason(option.reason,option.reason_diagnostic)}</p>}
       </li>;
      })}
     </ul>
    </>}
    <details><summary>{t('boundaries')}</summary><dl className="composition-facts"><dt>{t('route')}</dt><dd>{page.execution_route.pool_id?t('route_pool',{pool:page.execution_route.pool_id}):t('route_local')}</dd><dt>{t('model')}</dt><dd>{t('model_depends_on_tool')}</dd><dt>{t('cost')}</dt><dd>{t('cost_unknown')}</dd></dl><p className="composition-muted">{t('recheck')}</p></details>
   </>}
  </div>}
 </section>;
}
