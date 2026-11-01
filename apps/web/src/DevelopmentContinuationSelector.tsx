import {useEffect,useId,useRef,useState} from 'react';
import {GitCommitHorizontal,RefreshCw} from 'lucide-react';
import type {DevelopmentContinuationBinding,DevelopmentContinuationSource,TaskDevelopmentContinuationOption,TaskDevelopmentContinuationOptions} from '../../../packages/contracts/src';
import {ApiError,stateLabel} from './api';
import {useApi,useApiView} from './api-context';
import {formatNumber,tr,UiError,useLocale} from './i18n';
import {ErrorNotice} from './ui';
import {developmentContinuationOptionsPath,readDevelopmentContinuationOptions,sameDevelopmentContinuationSource,validDevelopmentContinuationBinding} from './development-continuation-model';
import './DevelopmentContinuationSelector.css';

export type DevelopmentContinuationSelectorProps={project:string;taskID:string;taskVersion:number;disabled?:boolean;accessBlocked?:boolean;refresh?:number;onChange:(value:{source?:DevelopmentContinuationSource;ready:boolean})=>void;onAccessDenied:()=>void};
type Snapshot={api:ReturnType<typeof useApi>;key:string;base:string;page?:TaskDevelopmentContinuationOptions;items:TaskDevelopmentContinuationOption[];loading:boolean;error?:unknown};
const t=(key:string,values?:Record<string,unknown>)=>tr('development-continuation.'+key,values);
const sourceScope=(page:TaskDevelopmentContinuationOptions)=>JSON.stringify([page.project_id,page.task_id,page.task_version,page.source_task_run_id,page.source_run_version,page.source_run_state]);
const runLink=(project:string,run:string)=>'?'+new URLSearchParams({project,view:'development',sub:'runs',run});

export function DevelopmentContinuationSelector(props:DevelopmentContinuationSelectorProps){
 const api=useApi(),view=useApiView(),owner=useRef({api,epoch:0});
 if(owner.current.api!==api)owner.current={api,epoch:owner.current.epoch+1};
 return <ContinuationScope key={JSON.stringify([view?.userID,view?.organizationID,props.project,props.taskID,owner.current.epoch])} {...props}/>;
}
function ContinuationScope({project,taskID,taskVersion,disabled=false,accessBlocked=false,refresh=0,onChange,onAccessDenied}:DevelopmentContinuationSelectorProps){
 useLocale();const api=useApi(),view=useApiView(),choiceName=useId(),[enabled,setEnabled]=useState(false),[revision,setRevision]=useState(0),[snapshot,setSnapshot]=useState<Snapshot>(),[selected,setSelected]=useState<{scope:string;source:DevelopmentContinuationSource}>(),[denial,setDenial]=useState<unknown>();
 const callbacks=useRef({onChange,onAccessDenied});callbacks.current={onChange,onAccessDenied};
 const gate=useRef({disabled,accessBlocked,epoch:0});if(gate.current.disabled!==disabled||gate.current.accessBlocked!==accessBlocked)gate.current={disabled,accessBlocked,epoch:gate.current.epoch+1};
 const base=JSON.stringify([project,taskID,taskVersion]),key=JSON.stringify([base,refresh,revision,gate.current.epoch]),hasView=!!view?.userID&&!!view.organizationID,blocked=disabled||accessBlocked||!!denial;
 const intent=useRef({key,enabled,blocked});intent.current={key,enabled,blocked};
 const sequence=useRef(0),controller=useRef<AbortController|undefined>(undefined),inFlight=useRef(false),emitted=useRef('');
 const display=!accessBlocked&&!denial&&snapshot?.api===api&&snapshot.base===base?snapshot:undefined,current=display?.key===key?display:undefined,page=current?.page;
 const displayedChoice=display?.page&&selected?.scope===sourceScope(display.page)?display.items.find(item=>sameDevelopmentContinuationSource(item.selection,selected.source)):undefined,chosen=current?displayedChoice:undefined;
 const ready=!accessBlocked&&!denial&&(!enabled||!!current&&!current.loading&&!current.error&&!!chosen),loading=enabled&&!blocked&&hasView&&(!current||current.loading),error=denial||current?.error||(enabled&&!hasView?new UiError('development-continuation.view_required'):undefined);
 useEffect(()=>{
  if(disabled&&!accessBlocked&&!denial)return;
  const result=ready&&enabled&&chosen?{source:chosen.selection,ready:true}:{ready},signature=JSON.stringify(result);
  if(emitted.current!==signature){emitted.current=signature;callbacks.current.onChange(result);}
 },[disabled,accessBlocked,denial,enabled,ready,chosen]);
 async function read(before?:string,previous?:Snapshot){
  if(blocked||!enabled||!hasView||inFlight.current)return;
  const abort=new AbortController(),ticket=++sequence.current;controller.current=abort;inFlight.current=true;
  if(!before)setSelected(undefined);
  setSnapshot({api,key,base,page:before?previous?.page:undefined,items:before?previous?.items||[]:[],loading:true});
  const owns=()=>sequence.current===ticket&&!abort.signal.aborted&&intent.current.key===key&&intent.current.enabled&&!intent.current.blocked;
  try{
   const value=await api<unknown>(developmentContinuationOptionsPath({project,task:taskID,limit:20,...(before?{before}:{})}),undefined,'GET',{'X-Suma-Expected-User':view!.userID,'X-Suma-Expected-Organization':view!.organizationID},abort.signal);
   if(!owns())return;
   const next=readDevelopmentContinuationOptions(value,{project,task:taskID,taskVersion,limit:20});
   if(before&&(!previous?.page||sourceScope(previous.page)!==sourceScope(next)||previous.page.total!==next.total))throw new UiError('development-continuation.changed');
   const items=before?[...(previous?.items||[])]:[];
   for(const item of next.items){const duplicate=items.find(old=>sameDevelopmentContinuationSource(old.selection,item.selection));if(duplicate){if(duplicate.candidate.transfer_id!==item.candidate.transfer_id||duplicate.candidate.head_commit!==item.candidate.head_commit||duplicate.candidate.bundle_sha256!==item.candidate.bundle_sha256||duplicate.candidate.evidence_sha256!==item.candidate.evidence_sha256)throw new UiError('development-continuation.changed');}else items.push(item);}
   setSnapshot({api,key,base,page:next,items,loading:false});
  }catch(failure){
   if(!owns())return;
   setSelected(undefined);
   if(failure instanceof ApiError&&[401,403,404].includes(failure.status)){setSnapshot(undefined);setDenial(failure);callbacks.current.onAccessDenied();}
   else setSnapshot({api,key,base,items:[],loading:false,error:failure});
  }finally{if(sequence.current===ticket)inFlight.current=false;}
 }
 useEffect(()=>{
  if(enabled&&!blocked&&hasView)void read();
  return()=>{sequence.current++;controller.current?.abort();inFlight.current=false;};
 },[api,key,enabled,disabled,accessBlocked,denial,hasView]);
 function toggle(next:boolean){if(blocked)return;sequence.current++;controller.current?.abort();inFlight.current=false;setEnabled(next);setSelected(undefined);setSnapshot(undefined);}
 function reload(){if(blocked||loading||inFlight.current)return;setRevision(value=>value+1);}
 return <section className="development-continuation" data-development-continuation-selector>
  <label className="development-continuation-toggle"><input type="checkbox" data-continuation-toggle checked={enabled} disabled={blocked} onChange={event=>toggle(event.target.checked)}/><span><strong>{t('toggle')}</strong><span>{t(enabled?'enabled_hint':'default_hint')}</span></span></label>
  {enabled&&<div className="development-continuation-body">
   <p className="development-continuation-boundary">{t('boundary')}</p>
   {!!error&&<ErrorNotice error={error}/>}{loading&&<p role="status">{t('loading')}</p>}
   {current?.page?.reason&&<ErrorNotice error={new UiError('development-continuation.reason.'+current.page.reason)}/>}
   {!!display?.items.length&&<fieldset className="development-continuation-options" disabled={blocked||loading||!!error||!current}><legend>{t('choose')}</legend>{display.items.map(item=><label className="development-continuation-option" key={JSON.stringify(item.selection)}><input type="radio" name={choiceName} data-continuation-source={item.selection.node_id} checked={!!displayedChoice&&sameDevelopmentContinuationSource(displayedChoice.selection,item.selection)} onChange={()=>{if(blocked||loading||!current||error)return;setSelected({scope:sourceScope(current.page!),source:item.selection});}}/><span><strong>{item.node_name||item.selection.node_id}</strong><code>{item.candidate.head_commit}</code><span className="development-continuation-meta">{t('producer',{node:item.selection.node_id,attempt:formatNumber(item.selection.attempt),fence:formatNumber(item.selection.fence)})}</span></span></label>)}</fieldset>}
   {current&&page&&<>{page.source_task_run_id&&<p className="development-continuation-meta">{t('source_run')}: <a href={runLink(project,page.source_task_run_id)}>{page.source_task_run_id}</a> · {stateLabel(page.source_run_state)} · {t('run_version',{version:formatNumber(page.source_run_version)})}</p>}<p className="development-continuation-meta">{t('range',{loaded:formatNumber(current.items.length),total:formatNumber(page.total)})}</p>{!page.total&&!page.reason&&<p role="status">{t('empty')}</p>}</>}
   <div className="development-continuation-actions"><button type="button" data-continuation-refresh disabled={blocked||loading||!hasView} onClick={reload}><RefreshCw size={15} aria-hidden="true"/>{t('refresh')}</button>{page?.next_cursor&&<button type="button" data-continuation-more disabled={blocked||loading||!!error} onClick={()=>{if(current&&!inFlight.current)void read(page.next_cursor,current);}}>{t('more')}</button>}</div>
  </div>}
 </section>;
}

export function DevelopmentContinuationFacts({binding,project,taskID}:{binding:DevelopmentContinuationBinding;project:string;taskID?:string}){
 useLocale();if(!validDevelopmentContinuationBinding(binding,project,taskID))return <ErrorNotice error={new UiError('development-continuation.invalid_binding')}/>;
 const candidate=binding.source;
 return <section className="development-continuation development-continuation-facts" data-development-continuation-facts><h4><GitCommitHorizontal size={17} aria-hidden="true"/>{t('facts_title')}</h4><p>{t('boundary')}</p><dl><div><dt>{t('source_run')}</dt><dd><a href={runLink(project,binding.source_task_run_id)}>{binding.source_task_run_id}</a></dd></div><div><dt>{t('head_commit')}</dt><dd><code>{candidate.head_commit}</code></dd></div><div><dt>{t('producer_label')}</dt><dd>{t('producer',{node:candidate.source.node_id,attempt:formatNumber(candidate.source.attempt),fence:formatNumber(candidate.source.fence)})}</dd></div></dl><details><summary>{t('evidence')}</summary><p className="development-continuation-meta">{t('source_versions',{task:formatNumber(binding.source_task_version),run:formatNumber(binding.source_run_version)})}</p><pre tabIndex={0}>{JSON.stringify(binding,null,2)}</pre></details></section>;
}
