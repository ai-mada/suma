import {useEffect,useId,useRef,useState} from 'react';
import {ChevronDown,ListFilter,RefreshCw} from 'lucide-react';
import type {TaskDispatchOptions} from '../../../packages/contracts/src';
import {useApi} from './api-context';
import {ApiError,type Session} from './api';
import {formatDateTime,formatNumber,tr,UiError,useLocale} from './i18n';
import {toolReason} from './ToolFacts';
import {toolError} from './tool-hooks';
import type {ToolProfile} from './tool-model';
import {dispatchOptionsPath,dispatchOptionsScopeKey,selectedDispatchOption,validDispatchOptions,type DispatchOptionsScope} from './task-dispatch-options-model';
import {ErrorNotice,Loading} from './ui';
import './CandidateCompositionPanel.css';

const t=(key:string,values?:Record<string,unknown>)=>tr('task-dispatch-options.'+key,values);
type Props={session:Session;scope:DispatchOptionsScope;disabled?:boolean;onAdopt:(tool:ToolProfile)=>void;onUnavailable:()=>void};
export function TaskDispatchOptionsPanel(props:Props){
 // Remount every identity/basis, including the user's organization, before a
 // stale page can render or adopt into a different task's selector.
 return <OptionsScope key={JSON.stringify([props.session.user.id,props.session.organization.id,dispatchOptionsScopeKey(props.scope)])} {...props}/>;
}
function OptionsScope({scope,disabled=false,onAdopt,onUnavailable}:Props){
 useLocale();const api=useApi(),bodyID=useId(),[open,setOpen]=useState(false),[page,setPage]=useState<TaskDispatchOptions>(),[pageNumber,setPageNumber]=useState(1),[loading,setLoading]=useState(false),[error,setError]=useState<unknown>(),epoch=useRef(0),controller=useRef<AbortController|undefined>(undefined),unavailable=useRef(onUnavailable);
 unavailable.current=onUnavailable;
 useEffect(()=>()=>{epoch.current++;controller.current?.abort();},[]);
 async function read(before='',number=1){
  const request=++epoch.current;controller.current?.abort();const abort=new AbortController();controller.current=abort;
  // Never keep the previous observation looking current during a new read.
  setPage(undefined);setError(undefined);setLoading(true);
  try{
   const value=await api<unknown>(dispatchOptionsPath(scope,before),undefined,'GET',undefined,abort.signal);
   if(request!==epoch.current||abort.signal.aborted)return;
   if(!validDispatchOptions(value,scope)||before&&value.next_cursor===before)throw new UiError('task-dispatch-options.invalid_read');
   setPage(value);setPageNumber(number);
  }catch(problem){
   if(request!==epoch.current||abort.signal.aborted)return;
   setPage(undefined);setError(problem);
   if(problem instanceof ApiError&&[401,403].includes(problem.status))unavailable.current();
  }finally{if(request===epoch.current&&!abort.signal.aborted)setLoading(false);}
 }
 function toggle(){
  if(open){epoch.current++;controller.current?.abort();setLoading(false);setPage(undefined);setError(undefined);setOpen(false);}
  else{setOpen(true);void read();}
 }
 const recommended=page&&selectedDispatchOption(page);
 return <section className="candidate-composition task-dispatch-options">
  <button type="button" data-dispatch-options-toggle className="composition-toggle" aria-expanded={open} aria-controls={bodyID} disabled={disabled} onClick={toggle}><ListFilter size={16}/><span>{t('title')}</span><ChevronDown size={16}/></button>
  {open&&<div id={bodyID} className="composition-body" aria-busy={loading}>
   <p className="composition-muted">{t('intro')}</p>
   <div className="composition-actions"><button type="button" data-dispatch-options-refresh disabled={disabled||loading} onClick={()=>void read()}><RefreshCw size={14}/>{t('refresh')}</button>{page&&<small>{t('current_page',{page:formatNumber(pageNumber),count:formatNumber(page.items.length),total:formatNumber(page.total)})}</small>}</div>
   {loading&&<Loading/>}
   {!!error&&<><ErrorNotice error={toolError(error)}/><button type="button" disabled={disabled||loading} onClick={()=>void read()}>{t('retry')}</button></>}
   {page&&<>
    <p className="composition-muted">{t('observed',{time:formatDateTime(page.observed_at,{year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',second:'2-digit'})})}</p>
    {!page.items.length?<p data-dispatch-options-empty>{t('empty')}</p>:<>
     <ul className="composition-run-history" aria-label={t('tools')}>
      {page.items.map(option=><li key={option.tool.id} data-dispatch-option={option.tool.id}>
       <div className="composition-heading"><strong>{option.tool.name}</strong><span>{t('reason_'+option.reason_code)}</span></div>
       <p className="composition-muted">{t('tool_basis',{worker:option.tool.worker_id,version:formatNumber(option.tool.version),toolVersion:option.tool.profile.tool_version})}</p>
       {!option.eligible&&option.reason&&<p>{toolReason(option.reason,option.reason_diagnostic)}</p>}
      </li>)}
     </ul>
     {recommended?<div className="composition-callout"><p>{t('recommended',{name:recommended.name})}</p><button type="button" data-dispatch-options-adopt disabled={disabled||loading} onClick={()=>onAdopt(recommended)}>{t('adopt')}</button></div>:<p>{t('none_eligible')}</p>}
    </>}
    {page.next_cursor&&<button type="button" data-dispatch-options-more disabled={disabled||loading} onClick={()=>void read(page.next_cursor,pageNumber+1)}>{t('next')}</button>}
    <details><summary>{t('boundaries')}</summary><dl className="composition-facts"><dt>{t('route')}</dt><dd>{page.execution_route.pool_id?t('route_pool',{pool:page.execution_route.pool_id}):t('route_local')}</dd><dt>{t('model')}</dt><dd>{recommended?.profile.model_execution_protocol==='suma-model-execution-v1'?tr('model-execution.select_after_tool'):t('model_fixed')}</dd><dt>{t('cost')}</dt><dd>{t('cost_unknown')}</dd></dl><p className="composition-muted">{t('recheck')}</p></details>
   </>}
  </div>}
 </section>;
}
