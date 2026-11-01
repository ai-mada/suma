import {useCallback,useEffect,useLayoutEffect,useRef,useState} from 'react';
import type {ToolInspection,ToolProfile} from '../../../packages/contracts/src';
import {ApiError,operation} from './api';
import {useApi,useApiView} from './api-context';
import {tr,UiError,useLocale} from './i18n';
import {toolSessionStorage} from './tool-hooks';
import {useWriteAction} from './write-action';
import {ErrorNotice} from './ui';
import {buildToolReinspectionIntent,inspectionMatchesTool,readToolInspection,readToolInspections,toolInspectionListPath,toolInspectionPath,toolReinspectionRecoveryScope,validToolInspectionReceipt,validToolReinspectionIntent,validToolReinspectionRecovery,type ToolReinspectionIntent,type ToolReinspectionScope} from './tool-reinspection-model';
import './ToolReinspectionPanel.css';

export type ToolReinspectionPanelProps={project:string;tool:ToolProfile;disabled?:boolean;accessBlocked?:boolean;onLockChange?:(locked:boolean)=>void;onAccessDenied?:()=>void;onObserved?:()=>void;onDefer?:()=>void};
type IntentState={intent?:ToolReinspectionIntent;error?:unknown};
type Snapshot={source:string;items:ToolInspection[];loading:boolean;error?:unknown;stale?:boolean;denied?:boolean};
const t=(key:string)=>tr('tool-reinspection.'+key);
const canonical=(value:unknown):string=>JSON.stringify(value,(_key,item)=>item&&typeof item==='object'&&!Array.isArray(item)?Object.fromEntries(Object.keys(item).sort().map(key=>[key,item[key]])):item);
const unresolved=(row:ToolInspection)=>['pending','claimed','unknown'].includes(row.state);
const refused=(error:unknown)=>error instanceof ApiError&&[401,403,404].includes(error.status);
function readIntent(storage:ReturnType<typeof toolSessionStorage>,key:string,scope:ToolReinspectionScope):IntentState{
 try{if(!storage)throw Error();const raw=storage.getItem(key);if(raw===null)return {};if(new TextEncoder().encode(raw).length>1048576)throw Error();const value:unknown=JSON.parse(raw);if(!validToolReinspectionIntent(value,scope))throw Error();return {intent:value};}
 catch{return {error:new UiError('tool-reinspection.intent_unavailable')};}
}

/** One explicitly requested maintenance report; no installation, enable or task dispatch. */
export function ToolReinspectionPanel(props:ToolReinspectionPanelProps){
 const api=useApi(),view=useApiView(),owner=useRef({api,epoch:0});
 if(owner.current.api!==api)owner.current={api,epoch:owner.current.epoch+1};
 if(!view?.userID||!view.organizationID||props.project!==props.tool.project_id)return null;
 const scope:ToolReinspectionScope={project:props.project,toolID:props.tool.id,userID:view.userID,organizationID:view.organizationID};
 return <ReinspectionController key={JSON.stringify([toolReinspectionRecoveryScope(scope),owner.current.epoch])} {...props} scope={scope}/>;
}
function ReinspectionController({scope,tool,disabled=false,accessBlocked=false,onLockChange,onAccessDenied,onObserved,onDefer}:ToolReinspectionPanelProps&{scope:ToolReinspectionScope}){
 useLocale();const api=useApi(),storage=toolSessionStorage(),base=toolReinspectionRecoveryScope(scope),intentKey='suma-tool-reinspection-intent:'+base;
 const [intentState,setIntentState]=useState<IntentState>(()=>readIntent(storage,intentKey,scope)),intent=useRef(intentState.intent);
 const [selected,setSelected]=useState(''),selectedRef=useRef(selected);selectedRef.current=selected;
 const currentTool=useRef(tool),parent=useRef({disabled,accessBlocked});currentTool.current=tool;parent.current={disabled,accessBlocked};
 const source=canonical([intentState.intent?.tool??tool,selected]),sourceRef=useRef(source);sourceRef.current=source;
 const [snapshot,setSnapshot]=useState<Snapshot>({source,items:[],loading:!accessBlocked}),snapshotRef=useRef(snapshot);
 const [backgroundPaused,setBackgroundPaused]=useState(false),[,renderCommand]=useState(0),alive=useRef(true),caller=useRef(api),epoch=useRef(0),controller=useRef<AbortController|undefined>(undefined),flight=useRef<Promise<boolean>|undefined>(undefined),commandBusy=useRef(false),polls=useRef(0),accessDenied=useRef(false),denialReported=useRef(false);
 caller.current=api;const callbacks=useRef({onLockChange,onAccessDenied,onObserved,onDefer});callbacks.current={onLockChange,onAccessDenied,onObserved,onDefer};
 const settled=useRef<{intent:ToolReinspectionIntent;value:ToolInspection}|undefined>(undefined);
 // Only a refusal of this mounted view's first fresh submission proves that
 // its metadata can be discarded. Hydrated/restored operations may already
 // have committed even when their independent write journal was consumed.
 const firstSubmission=useRef<ToolReinspectionIntent|undefined>(undefined);
 const store=(value:Snapshot)=>{snapshotRef.current=value;setSnapshot(value);};
 const setIntent=(value:IntentState)=>{intent.current=value.intent;setIntentState(value);};
 const action=useWriteAction({scope:'tool-reinspection:'+base,storage,validateRequest:request=>validToolReinspectionRecovery(request,scope),validateResponse:(value,request)=>!!intent.current&&validToolInspectionReceipt(value,request,scope,intent.current)});
 const writeDenied=!!action.errorStatus&&[401,403,404].includes(action.errorStatus);
 const deny=useCallback(()=>{
  accessDenied.current=true;++epoch.current;controller.current?.abort();flight.current=undefined;store({source:sourceRef.current,items:[],loading:false,denied:true});
  if(!denialReported.current){denialReported.current=true;callbacks.current.onAccessDenied?.();}
 },[]);
 useLayoutEffect(()=>{if(accessBlocked){accessDenied.current=true;++epoch.current;controller.current?.abort();flight.current=undefined;store({source:sourceRef.current,items:[],loading:false,denied:true});}},[accessBlocked]);
 useLayoutEffect(()=>{if(writeDenied)deny();},[writeDenied,deny]);
 const read=useCallback((background=false):Promise<boolean>=>{
  if(parent.current.accessBlocked)return Promise.resolve(false);
  if(flight.current)return flight.current;
  const originalSource=sourceRef.current,ticket=++epoch.current,abort=new AbortController(),originalID=intent.current?'':selectedRef.current;controller.current?.abort();controller.current=abort;
  const previous=snapshotRef.current.source===originalSource&&!accessDenied.current?snapshotRef.current.items:[];
  store({source:originalSource,items:previous,loading:true,stale:!!previous.length,denied:accessDenied.current});
  const owns=()=>alive.current&&caller.current===api&&epoch.current===ticket&&!abort.signal.aborted&&!parent.current.accessBlocked&&sourceRef.current===originalSource;
  const job=(async()=>{
   try{
    const raw=await api<unknown>(originalID?toolInspectionPath(scope,originalID):toolInspectionListPath(scope),undefined,'GET',{'X-Suma-Expected-User':scope.userID,'X-Suma-Expected-Organization':scope.organizationID},abort.signal);
    if(!owns())return false;const items=originalID?[readToolInspection(raw,scope,originalID)]:readToolInspections(raw,scope);
    accessDenied.current=false;denialReported.current=false;store({source:originalSource,items,loading:false});return true;
   }catch(error){if(!owns())return false;if(refused(error))deny();else store({source:originalSource,items:previous,loading:false,stale:!!previous.length,denied:accessDenied.current,error});return false;}
  })();flight.current=job;void job.finally(()=>{if(flight.current===job)flight.current=undefined;});if(!background)polls.current=0;return job;
 },[api,base,deny]);
 const readNow=useRef(read);readNow.current=read;
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;++epoch.current;controller.current?.abort();flight.current=undefined;callbacks.current.onLockChange?.(false);};},[]);
 useEffect(()=>{++epoch.current;controller.current?.abort();flight.current=undefined;void read();},[read,source,accessBlocked]);
 const visible=!accessBlocked&&snapshot.source===source?snapshot:{source,items:[],loading:!accessBlocked,denied:accessBlocked||accessDenied.current};
 const hidden=accessBlocked||!!visible.denied||writeDenied;
 const original=intentState.intent,requestMatches=!action.pending||!!original&&canonical(action.pending)===canonical(original.request);
 const intentUnavailable=!!intentState.error||!!action.pending&&(!original||!requestMatches);
 const locked=action.busy||!!original||!!action.pending||intentUnavailable||action.persistenceBlocked;
 useEffect(()=>{callbacks.current.onLockChange?.(locked);},[locked]);
 const rows=hidden?[]:visible.items,row=!original?(rows.find(value=>value.id===selected)??rows.find(unresolved)??rows[0]):undefined;
 const readBlocked=hidden||visible.loading||!!visible.error||!!visible.stale;
 const freshBlocked=disabled||readBlocked||locked||rows.some(unresolved)||tool.state==='revoked';
 const gate=useRef({freshBlocked,readBlocked,row,rows});gate.current={freshBlocked,readBlocked,row,rows};
 useEffect(()=>{
  if(original||disabled||hidden||visible.error||visible.loading||backgroundPaused||!row||!['pending','claimed'].includes(row.state))return;
  const timer=window.setInterval(()=>{if(document.visibilityState==='hidden'||flight.current||commandBusy.current)return;if(polls.current>=120){setBackgroundPaused(true);return;}++polls.current;void readNow.current(true);},2000);
  return()=>window.clearInterval(timer);
 },[!!original,disabled,hidden,!!visible.error,visible.loading,backgroundPaused,row?.id,row?.state]);
 function removeIntent(originalIntent:ToolReinspectionIntent){
  if(!storage)throw Error();const raw=storage.getItem(intentKey);if(raw!==null&&raw!==JSON.stringify(originalIntent))throw Error();storage.removeItem(intentKey);setIntent({});
 }
 useEffect(()=>{
  const originalIntent=intent.current;if(!originalIntent||firstSubmission.current!==originalIntent||action.busy||action.pending||!action.errorStatus||action.errorStatus<400||action.errorStatus>=500)return;
  try{removeIntent(originalIntent);firstSubmission.current=undefined;}catch{setIntent({intent:originalIntent,error:new UiError('tool-reinspection.intent_unavailable')});}
 },[action.busy,!!action.pending,action.errorStatus]);
 async function refresh(){
  if(!alive.current||caller.current!==api||parent.current.accessBlocked)return;
  const saved=intent.current,reloaded=readIntent(storage,intentKey,scope);if(saved&&(!reloaded.intent||canonical(saved)!==canonical(reloaded.intent)))setIntent({intent:saved,error:new UiError('tool-reinspection.intent_unavailable')});else setIntent(reloaded);
  setBackgroundPaused(false);polls.current=0;const ready=await readNow.current();if(ready&&writeDenied)action.setError('');
 }
 async function send(restore=false){
  if(!alive.current||caller.current!==api||commandBusy.current||parent.current.accessBlocked||gate.current.readBlocked)return;
  let originalIntent=intent.current;
  if(restore){if(!originalIntent||!requestMatches)return;firstSubmission.current=undefined;}
  else{
   if(gate.current.freshBlocked||originalIntent)return;
   const key=operation(),request={path:'/tool-profiles/'+encodeURIComponent(scope.toolID)+'/inspections',method:'POST',idempotency_key:key,body:{project_id:scope.project,expected_version:currentTool.current.version,idempotency_key:key}};
   originalIntent=buildToolReinspectionIntent(currentTool.current,scope,request);if(!originalIntent)return;
   try{if(!storage||storage.getItem(intentKey)!==null)throw Error();storage.setItem(intentKey,JSON.stringify(originalIntent));setIntent({intent:originalIntent});firstSubmission.current=originalIntent;}
   catch{setIntent({error:new UiError('tool-reinspection.intent_unavailable')});return;}
  }
  try{if(!storage||storage.getItem(intentKey)!==JSON.stringify(originalIntent))throw Error();}
  catch{setIntent({intent:originalIntent,error:new UiError('tool-reinspection.intent_unavailable')});return;}
  commandBusy.current=true;const originalCaller=api;
  try{
   const value=settled.current&&canonical(settled.current.intent)===canonical(originalIntent)?settled.current.value:await action.run<ToolInspection>(originalIntent.request.path,originalIntent.request.body,originalIntent.request.method);
   if(!value||!alive.current||caller.current!==originalCaller||intent.current!==originalIntent||parent.current.accessBlocked)return;
   settled.current={intent:originalIntent,value};
   try{removeIntent(originalIntent);}catch{setIntent({intent:originalIntent,error:new UiError('tool-reinspection.intent_unavailable')});return;}
   settled.current=undefined;selectedRef.current=value.id;setSelected(value.id);polls.current=0;setBackgroundPaused(false);
  }finally{commandBusy.current=false;if(alive.current&&caller.current===originalCaller)renderCommand(value=>value+1);}
 }
 function review(){const now=gate.current;if(!alive.current||caller.current!==api||parent.current.disabled||now.readBlocked||locked||!now.row||now.row.state!=='completed'||!inspectionMatchesTool(now.row,currentTool.current))return;callbacks.current.onObserved?.();}
 function defer(){if(!alive.current||caller.current!==api||action.busy||commandBusy.current)return;callbacks.current.onDefer?.();}
 const sourceTool=original?.tool??tool,sourceRow=row;
 const sourceFields:[string,string|number][]=[['worker',sourceRow?.worker_id??sourceTool.worker_id],['profile',sourceRow?.tool_profile_id??sourceTool.id],['revision',sourceRow?.tool_profile_version??sourceTool.version],['digest',sourceRow?.profile_sha256??sourceTool.profile.profile_sha256],['command',sourceRow?.command_id??sourceTool.profile.command_id]];
 if(sourceRow){sourceFields.push(['request',sourceRow.id],['fence',sourceRow.fence],['created',sourceRow.created_at]);if(sourceRow.inventory_operation_id)sourceFields.push(['inventory',sourceRow.inventory_operation_id]);}
 return <section className="tool-reinspection" data-tool-reinspection-panel aria-busy={action.busy||visible.loading}>
  <div className="tool-reinspection-heading"><h4>{t('title')}</h4></div>
  <p className="tool-reinspection-boundary">{t('boundary')}</p>
  {hidden?<p role="alert">{t('access_denied')}</p>:<>
   <div className="tool-reinspection-target"><strong>{sourceTool.name}</strong><span>{sourceRow?.worker_id??sourceTool.worker_id}</span></div>
   {original?<p role="status">{t('original_pending')}</p>:row?<div className="tool-reinspection-status" role="status" data-tool-reinspection-state={row.state}><p>{t('state.'+row.state)}</p>{row.result_code&&<p>{t('result.'+row.result_code)}</p>}</div>:<p role="status">{t(visible.loading?'loading':'empty')}</p>}
   {row&&!inspectionMatchesTool(row,tool)&&<p>{t('changed')}</p>}
   <details data-tool-reinspection-source><summary>{t('source')}</summary><dl>{sourceFields.map(([label,value])=><div key={label}><dt>{t(label)}</dt><dd><code>{value}</code></dd></div>)}</dl></details>
   {rows.length>1&&!original&&<details><summary>{t('history')}</summary><ul className="tool-reinspection-history">{rows.filter(value=>value.id!==row?.id).map(value=><li key={value.id}><strong>{value.id}</strong><p>{t('state.'+value.state)}</p></li>)}</ul></details>}
  </>}
  {!hidden&&!!visible.error&&<ErrorNotice error={visible.error}/>}
  {!hidden&&(intentState.error||action.error)&&<ErrorNotice error={intentState.error??action.error}/>}
  {backgroundPaused&&<p role="status">{t('background_paused')}</p>}
  <div className="tool-reinspection-actions">
   {!hidden&&original&&<button type="button" className="primary" data-tool-reinspection-restore disabled={action.busy||readBlocked||!requestMatches} onClick={()=>void send(true)}>{t('restore')}</button>}
   {!hidden&&!original&&<button type="button" className={row?.state==='completed'?'':'primary'} data-tool-reinspection-create disabled={freshBlocked} onClick={()=>void send()}>{t('create')}</button>}
   {!hidden&&row?.state==='completed'&&inspectionMatchesTool(row,tool)&&onObserved&&<button type="button" className="primary" data-tool-reinspection-review disabled={disabled||readBlocked||locked} onClick={review}>{t('review')}</button>}
   <button type="button" data-tool-reinspection-refresh disabled={action.busy||visible.loading||accessBlocked} onClick={()=>void refresh()}>{t('refresh')}</button>
   {onDefer&&<button type="button" data-tool-reinspection-defer disabled={action.busy||commandBusy.current} onClick={defer}>{t('defer')}</button>}
  </div>
 </section>;
}
