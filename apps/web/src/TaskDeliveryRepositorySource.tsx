import {useCallback,useEffect,useId,useRef,useState} from 'react';
import {ChevronDown,GitBranch,RefreshCw} from 'lucide-react';
import type {RepositoryIntegrationTarget,TaskDeliveryRepositorySource as Source,TaskDeliveryRunTarget} from '../../../packages/contracts/src';
import {ApiError,query} from './api';
import {useApi} from './api-context';
import {formatDateTime,tr,useLocale} from './i18n';
import {Loading} from './ui';
import {RepositorySourceWorkbench} from './RepositorySourceWorkbench';
import {savedWorkbenchPlan,workbenchScope,workbenchScopeKey,type RepositoryWorkbenchScope} from './repository-source-workbench-model';
import {toolSessionStorage} from './tool-hooks';

type SourceScope={project:string;taskId:string;taskVersion:number;attempt:TaskDeliveryRunTarget};
type Props=Omit<SourceScope,'attempt'>&{owner:string;organization:string;attempt?:TaskDeliveryRunTarget;disabled:boolean;onLockChange:(locked:boolean)=>void;onOpenRun?:(run:string)=>void};
type Target=Pick<RepositoryIntegrationTarget,'id'|'version'|'name'|'ref'>;
type Failure='denied'|'changed'|'unavailable'|'invalid';
const t=(key:string)=>tr('task-delivery.repository_source.'+key);
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const id=(value:unknown):value is string=>typeof value==='string'&&/^[A-Za-z0-9_-]{1,64}$/.test(value);
const version=(value:unknown):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>0;
const label=(value:unknown,limit:number):value is string=>typeof value==='string'&&value.length<=limit&&!!value.trim();

function registeredTargets(value:unknown,project:string):Target[]|undefined{
 if(!object(value)||!Array.isArray(value.items)||value.total!==value.items.length||value.items.length>100)return;
 const targets:Target[]=[],seen=new Set<string>();
 for(const target of value.items){
  if(!object(target)||target.project_id!==project||!id(target.id)||!version(target.version)||!label(target.name,120)||!label(target.ref,512)||!Array.isArray(target.blockers)||!['active','paused','retired'].includes(String(target.status))||typeof target.active_operation_id!=='string'||seen.has(target.id))return;
  seen.add(target.id);
  if(target.status==='active'&&!target.active_operation_id&&!target.blockers.length)targets.push({id:target.id,version:target.version,name:target.name,ref:target.ref});
 }
 return targets;
}

function validSource(value:unknown,scope:SourceScope,target:Target):value is Source{
 if(!object(value)||Object.keys(value).length!==11||value.task_id!==scope.taskId||value.task_version!==scope.taskVersion||value.run_id!==scope.attempt.run_id||value.node_id!==scope.attempt.node_id||value.attempt!==scope.attempt.attempt||value.target_id!==target.id||value.target_version!==target.version||value.ref!==target.ref)return false;
 const format=value.object_format,head=value.head_commit,time=value.observed_at;
 return typeof head==='string'&&(format==='sha1'?/^[0-9a-f]{40}$/.test(head):format==='sha256'&&/^[0-9a-f]{64}$/.test(head))&&typeof time==='string'&&time.length<=64&&Number.isFinite(Date.parse(time));
}

function failure(error:unknown):Failure{
 if(error instanceof ApiError){
  if(error.status===401||error.status===403)return 'denied';
  if(error.status===404||error.status===409)return 'changed';
 }
 return 'unavailable';
}

export function TaskDeliveryRepositorySource({project,taskId,taskVersion,attempt,owner,organization,disabled,onLockChange,onOpenRun}:Props){
 useLocale();
 const [open,setOpen]=useState(false),panel=useId(),storage=toolSessionStorage(),locator='suma-repository-workbench-current:'+owner;
 const [work,setWork]=useState<{scope:RepositoryWorkbenchScope;head?:string}>();
 const [locked,setLocked]=useState(false);
 const button=useRef<HTMLButtonElement>(null);
 let saved:RepositoryWorkbenchScope|undefined;
 try{
  const value:unknown=JSON.parse(storage?.getItem(locator)||'null');
  if(workbenchScope(value)){
   const key=workbenchScopeKey(owner,value);
   if(storage?.getItem('suma-write-recovery-v1:'+key)||savedWorkbenchPlan(storage?.getItem('suma-repository-workbench-plan:'+key)))saved=value;
  }
 }catch{/* Reads still work when this browser cannot persist a workbench. */}
 function select(scope:RepositoryWorkbenchScope,head?:string){
  try{storage?.setItem(locator,JSON.stringify(scope));}catch{/* The write hook refuses untracked operations. */}
  setWork({scope,head});
 }
 function prepareSource(target:Target,value?:Source){
  if(!attempt)return;
  select({taskVersion,runID:attempt.run_id,nodeID:attempt.node_id,attempt:attempt.attempt,targetID:target.id,targetVersion:target.version},value?.head_commit);
 }
 function closeWorkbench(){setWork(undefined);queueMicrotask(()=>button.current?.focus());}
 const current=!!(attempt&&work&&taskVersion===work.scope.taskVersion&&attempt.run_id===work.scope.runID&&attempt.node_id===work.scope.nodeID&&attempt.attempt===work.scope.attempt);
 if(!attempt&&!work&&!saved)return null;
 return <div className="task-delivery-repository">
  {attempt&&<button type="button" className="task-delivery-repository-toggle" data-repository-source-toggle aria-expanded={open} aria-controls={panel} disabled={locked||disabled} ref={button} onClick={()=>setOpen(value=>!value)}>
   <GitBranch size={14} aria-hidden="true"/>{t('toggle')}<ChevronDown size={14} aria-hidden="true"/>
  </button>}
  {open&&attempt&&<RepositorySourceBody key={JSON.stringify([taskVersion,attempt])} project={project} taskId={taskId} taskVersion={taskVersion} attempt={attempt} panel={panel} locked={!!work||disabled} onPrepare={prepareSource}/>}
  {!work&&saved&&<button type="button" className="task-delivery-repository-toggle" data-repository-source-resume disabled={disabled} onClick={()=>select(saved)}>{t('workbench.resume')}</button>}
  {work&&<RepositorySourceWorkbench key={workbenchScopeKey(owner,work.scope)} owner={owner} organization={organization} project={project} task={taskId} scope={work.scope} checkedHead={work.head} current={current} disabled={disabled}
   onClose={closeWorkbench} onLockChange={value=>{setLocked(value);onLockChange(value);}} onOpenRun={onOpenRun}/>}
 </div>;
}

function RepositorySourceBody({panel,locked,onPrepare,...scope}:SourceScope&{panel:string;locked:boolean;onPrepare:(target:Target,observed?:Source)=>void}){
 const api=useApi(),targetField=useId(),request=useRef<AbortController|undefined>(undefined);
 const [targets,setTargets]=useState<Target[]>(),[selected,setSelected]=useState(''),[observed,setObserved]=useState<Source>();
 const [busy,setBusy]=useState(true),[error,setError]=useState<Failure>();
 const start=useCallback(()=>{
  request.current?.abort();
  const controller=new AbortController();request.current=controller;
  setBusy(true);setError(undefined);setObserved(undefined);
  return controller;
 },[]);
 const loadTargets=useCallback(async()=>{
  const controller=start();
  setTargets(undefined);setSelected('');
  try{
   const result=await api<unknown>('/repository-integration-targets'+query(scope.project),undefined,'GET',undefined,controller.signal);
   if(controller.signal.aborted)return;
   const options=registeredTargets(result,scope.project);
   if(!options){setError('invalid');return;}
   setTargets(options);
   if(options.length===1)setSelected(options[0].id);
  }catch(error){if(!controller.signal.aborted)setError(failure(error));}
  finally{if(!controller.signal.aborted)setBusy(false);}
 },[api,scope.project,start]);
 useEffect(()=>{void loadTargets();return()=>request.current?.abort();},[loadTargets]);

 const target=targets?.find(option=>option.id===selected);
 async function checkSource(){
  if(!target||busy||locked||error==='denied'||error==='changed')return;
  const controller=start();
  const search=new URLSearchParams({project_id:scope.project,task_version:String(scope.taskVersion),run_id:scope.attempt.run_id,target_id:target.id,target_version:String(target.version)});
  try{
   const result=await api<unknown>('/tasks/'+encodeURIComponent(scope.taskId)+'/delivery/repository-source?'+search,undefined,'GET',undefined,controller.signal);
   if(controller.signal.aborted)return;
   if(!validSource(result,scope,target)){setError('invalid');return;}
   setObserved(result);
  }catch(error){
   if(controller.signal.aborted)return;
   const state=failure(error);
   if(state==='denied'||state==='changed'){setTargets(undefined);setSelected('');}
   setError(state);
  }finally{if(!controller.signal.aborted)setBusy(false);}
 }

 return <div id={panel} className="task-delivery-repository-body" aria-busy={busy||undefined}>
  <p className="task-delivery-muted">{t('intro')}</p>
  {!!targets?.length&&<div className="task-delivery-repository-controls">
   <label htmlFor={targetField}>{t('target')}</label>
   <select id={targetField} data-repository-source-target value={selected} disabled={locked} onChange={event=>{
    request.current?.abort();setSelected(event.target.value);setObserved(undefined);setError(undefined);setBusy(false);
   }}>
    <option value="">{t('choose')}</option>
    {targets.map(option=><option key={option.id} value={option.id}>{option.name} · {option.ref}</option>)}
   </select>
   <button type="button" data-repository-source-check disabled={!target||busy||locked} onClick={()=>void checkSource()}>
    <RefreshCw size={13} aria-hidden="true"/>{t('check')}
   </button>
  </div>}
  {busy&&<Loading/>}
  {!busy&&targets?.length===0&&<p className="task-delivery-muted" role="status">{t('empty')}</p>}
  {error&&<p className="task-delivery-warning" role="status">{t(error)}</p>}
  {!busy&&(!targets?.length||error)&&<button type="button" data-repository-source-reload disabled={locked} onClick={()=>void loadTargets()}>{t('reload')}</button>}
  {observed&&<div className="task-delivery-repository-result" role="status" data-repository-source-result>
   <strong>{t('observed')}</strong>
   <dl>
    <div><dt>{t('ref')}</dt><dd><code>{observed.ref}</code></dd></div>
    <div><dt>{t('commit')}</dt><dd><code>{observed.head_commit}</code></dd></div>
    <div><dt>{t('time')}</dt><dd><time dateTime={observed.observed_at}>{formatDateTime(observed.observed_at,{dateStyle:'medium',timeStyle:'medium'})}</time></dd></div>
   </dl>
   <p className="task-delivery-muted">{t('boundary')}</p>
  </div>}
  {target&&<button type="button" data-repository-source-prepare disabled={locked||busy} onClick={()=>onPrepare(target,observed)}>{t('workbench.prepare')}</button>}
 </div>;
}
