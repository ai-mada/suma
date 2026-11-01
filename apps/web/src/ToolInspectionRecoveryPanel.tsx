import {useCallback,useEffect,useLayoutEffect,useRef,useState} from 'react';
import type {ToolInspection,ToolInspectionRecovery,ToolProfile} from '../../../packages/contracts/src';
import {ApiError,operation} from './api';
import {useApi,useApiView} from './api-context';
import {toolSessionStorage} from './tool-hooks';
import {useWriteAction} from './write-action';
import {ErrorNotice} from './ui';
import {tr,UiError,useLocale} from './i18n';
import {toolReinspectionRecoveryScope,type ToolReinspectionScope} from './tool-reinspection-model';
import {readToolMaintenanceStatus} from './tool-maintenance-model';
import {buildToolInspectionRecoveryIntent,readToolInspectionRecoveries,readToolInspectionRecovery,readToolInspectionRecoveryIntent,validToolInspectionRecoveryReceipt,validToolInspectionRecoveryWrite,type ToolInspectionRecoveryIntent} from './tool-inspection-recovery-model';

export type ToolInspectionRecoveryPanelProps={scope:ToolReinspectionScope;tool:ToolProfile;inspection?:ToolInspection;disabled?:boolean;accessBlocked?:boolean;onLockChange:(locked:boolean)=>void;onBusyChange?:(busy:boolean)=>void;onAccessDenied?:()=>void;onChanged?:(inspectionID:string)=>void;onDefer?:()=>void;onOpenWorker?:(workerID:string)=>void};
type Saved={ready:boolean;intent?:ToolInspectionRecoveryIntent;error?:unknown};
type Snapshot={source:string;rows:ToolInspectionRecovery[];loading:boolean;error?:unknown;denied?:boolean;stale?:boolean};
const t=(key:string,values?:Record<string,string|number>)=>tr('tool-reinspection.recovery.'+key,values);
const canonical=(value:unknown):string=>JSON.stringify(value,(_key,item)=>item&&typeof item==='object'&&!Array.isArray(item)?Object.fromEntries(Object.keys(item).sort().map(key=>[key,item[key]])):item);
const active=(r:ToolInspectionRecovery)=>r.state==='requested'||r.state==='proof_stored';
export function ToolInspectionRecoveryPanel(props:ToolInspectionRecoveryPanelProps){
 const api=useApi(),view=useApiView(),owner=useRef({api,epoch:0});if(owner.current.api!==api)owner.current={api,epoch:owner.current.epoch+1};
 if(!view||view.userID!==props.scope.userID||view.organizationID!==props.scope.organizationID)return null;
 return <RecoveryController key={JSON.stringify([toolReinspectionRecoveryScope(props.scope),owner.current.epoch])} {...props}/>;
}
function RecoveryController({scope,tool,inspection,disabled=false,accessBlocked=false,onLockChange,onBusyChange,onAccessDenied,onChanged,onDefer,onOpenWorker}:ToolInspectionRecoveryPanelProps){
 useLocale();const api=useApi(),storage=toolSessionStorage(),base=toolReinspectionRecoveryScope(scope),intentKey='suma-tool-inspection-recovery-intent:'+base;
 const [saved,setSaved]=useState<Saved>({ready:false}),original=useRef<ToolInspectionRecoveryIntent|undefined>(undefined),[selected,setSelected]=useState(''),selectedRef=useRef(selected);selectedRef.current=selected;
 const target=saved.intent?.inspection??inspection,source=canonical([target,selected]),sourceRef=useRef(source);sourceRef.current=source;
 const [snapshot,setSnapshot]=useState<Snapshot>({source,rows:[],loading:!!target}),current=useRef(snapshot),[confirming,setConfirming]=useState(false),[busy,setBusy]=useState(false),[paused,setPaused]=useState(false);
 const alive=useRef(true),caller=useRef(api),epoch=useRef(0),abort=useRef<AbortController|undefined>(undefined),flight=useRef<Promise<boolean>|undefined>(undefined),commandBusy=useRef(false),refused=useRef(false),reportedDenied=useRef(false),polls=useRef(0),reportedClosed=useRef('');caller.current=api;
 const incoming=useRef({inspection,tool,disabled,accessBlocked});incoming.current={inspection,tool,disabled,accessBlocked};
 const callbacks=useRef({onLockChange,onBusyChange,onAccessDenied,onChanged,onDefer,onOpenWorker});callbacks.current={onLockChange,onBusyChange,onAccessDenied,onChanged,onDefer,onOpenWorker};
 const firstFresh=useRef<ToolInspectionRecoveryIntent|undefined>(undefined),settled=useRef<{intent:ToolInspectionRecoveryIntent;row:ToolInspectionRecovery}|undefined>(undefined);
 const store=(value:Snapshot)=>{current.current=value;setSnapshot(value);};const save=(value:Saved)=>{original.current=value.intent;setSaved(value);};
 const action=useWriteAction({scope:'tool-inspection-recovery:'+base,storage,validateRequest:r=>validToolInspectionRecoveryWrite(r,scope),validateResponse:(value,r)=>original.current?validToolInspectionRecoveryReceipt(value,r,scope,original.current):false});
 async function loadSaved():Promise<Saved>{try{if(!storage)throw Error();const raw=storage.getItem(intentKey);if(raw===null)return {ready:true};if(new TextEncoder().encode(raw).length>1048576)throw Error();const intent=await readToolInspectionRecoveryIntent(JSON.parse(raw),scope);if(storage.getItem(intentKey)!==raw)throw Error();return {ready:true,intent};}catch{return {ready:true,error:new UiError('tool-reinspection.recovery.intent_unavailable')};}}
 useEffect(()=>{let live=true;void loadSaved().then(value=>{if(live&&alive.current&&caller.current===api)save(value);});return()=>{live=false;};},[api,base]);
 const deny=useCallback(()=>{refused.current=true;++epoch.current;abort.current?.abort();flight.current=undefined;store({source:sourceRef.current,rows:[],loading:false,denied:true});setConfirming(false);if(!reportedDenied.current){reportedDenied.current=true;callbacks.current.onAccessDenied?.();}},[]);
 const writeDenied=!!action.errorStatus&&[401,403,404].includes(action.errorStatus);
 useLayoutEffect(()=>{if(writeDenied)deny();},[writeDenied,deny]);
 useLayoutEffect(()=>{if(accessBlocked){refused.current=true;++epoch.current;abort.current?.abort();flight.current=undefined;store({source:sourceRef.current,rows:[],loading:false,denied:true});setConfirming(false);}},[accessBlocked]);
 const read=useCallback((background=false):Promise<boolean>=>{
  const target=original.current?.inspection??incoming.current.inspection;if(!target||incoming.current.accessBlocked)return Promise.resolve(false);if(flight.current)return flight.current;
  const originalSource=sourceRef.current,ticket=++epoch.current,controller=new AbortController(),id=original.current?'':selectedRef.current;abort.current?.abort();abort.current=controller;
  const previous=current.current.source===originalSource&&!refused.current?current.current.rows:[];store({source:originalSource,rows:previous,loading:true,denied:refused.current,stale:!!previous.length});
  const owns=()=>alive.current&&caller.current===api&&epoch.current===ticket&&!controller.signal.aborted&&!incoming.current.accessBlocked&&sourceRef.current===originalSource;
  const job=(async()=>{try{
   const path=id?'/tool-inspection-recoveries/'+encodeURIComponent(id):'/tool-inspections/'+encodeURIComponent(target.id)+'/recoveries';
   const raw=await api<unknown>(path+'?project_id='+encodeURIComponent(scope.project),undefined,'GET',{'X-Suma-Expected-User':scope.userID,'X-Suma-Expected-Organization':scope.organizationID},controller.signal);
   if(!owns())return false;const rows=id?[await readToolInspectionRecovery(raw,scope,target)]:await readToolInspectionRecoveries(raw,scope,target);if(id&&rows[0].id!==id)throw new UiError('tool-reinspection.recovery.invalid_read');if(!owns())return false;
   refused.current=false;reportedDenied.current=false;store({source:originalSource,rows,loading:false});return true;
  }catch(error){if(!owns())return false;if(error instanceof ApiError&&[401,403,404].includes(error.status))deny();else store({source:originalSource,rows:previous,loading:false,stale:!!previous.length,denied:refused.current,error});return false;}})();flight.current=job;void job.finally(()=>{if(flight.current===job)flight.current=undefined;});if(!background)polls.current=0;return job;
 },[api,base,deny]);const readNow=useRef(read);readNow.current=read;
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;++epoch.current;abort.current?.abort();flight.current=undefined;callbacks.current.onLockChange(false);callbacks.current.onBusyChange?.(false);};},[]);
 useEffect(()=>{setConfirming(false);++epoch.current;abort.current?.abort();flight.current=undefined;if(saved.ready)void read();},[read,saved.ready,source,accessBlocked]);
 const visible=!accessBlocked&&snapshot.source===source?snapshot:{source,rows:[],loading:!!target&&!accessBlocked,denied:accessBlocked||refused.current},hidden=accessBlocked||!!visible.denied||writeDenied;
 const matches=!action.pending||!!saved.intent&&canonical(action.pending)===canonical(saved.intent.request),intentError=!!saved.error||!!action.pending&&(!saved.intent||!matches),locked=!saved.ready||busy||action.busy||!!saved.intent||!!action.pending||intentError||action.persistenceBlocked;
 useLayoutEffect(()=>{callbacks.current.onLockChange(locked);},[locked]);
 useLayoutEffect(()=>{callbacks.current.onBusyChange?.(busy||action.busy||commandBusy.current);},[busy,action.busy]);
 const rows=hidden?[]:visible.rows,row=!saved.intent?(rows.find(value=>value.id===selected)??rows.find(active)??rows[0]):undefined,readBlocked=hidden||visible.loading||!!visible.error||!!visible.stale;
 const status=readToolMaintenanceStatus(tool.maintenance)?.status,known=tool.worker_id===target?.worker_id&&status?.protocol==='suma-tool-maintenance-v1'&&(status.state==='supported'||status.state==='expired');
 const freshBlocked=disabled||locked||readBlocked||!target||target.state!=='unknown'||!known||rows.some(active)||row?.state==='closed_unknown';
 const gate=useRef({freshBlocked,readBlocked,target});gate.current={freshBlocked,readBlocked,target};
 useEffect(()=>{if(saved.intent||disabled||hidden||visible.loading||visible.error||paused||!row||!active(row))return;const timer=window.setInterval(()=>{if(document.visibilityState==='hidden'||flight.current||commandBusy.current)return;if(polls.current>=120){setPaused(true);return;}++polls.current;void readNow.current(true);},2000);return()=>window.clearInterval(timer);},[!!saved.intent,disabled,hidden,visible.loading,!!visible.error,paused,row?.id,row?.state]);
 useEffect(()=>{if(hidden||readBlocked||!row||row.state!=='closed_unknown'||!target)return;const key=row.id+':'+row.version;if(reportedClosed.current===key)return;reportedClosed.current=key;callbacks.current.onChanged?.(target.id);},[hidden,readBlocked,row,target?.id]);
 function removeMetadata(intent:ToolInspectionRecoveryIntent){if(!storage||storage.getItem(intentKey)!==JSON.stringify(intent))throw Error();storage.removeItem(intentKey);save({ready:true});}
 useEffect(()=>{const intent=original.current;if(!intent||firstFresh.current!==intent||action.busy||action.pending||!action.errorStatus||action.errorStatus<400||action.errorStatus>=500)return;try{removeMetadata(intent);firstFresh.current=undefined;}catch{save({ready:true,intent,error:new UiError('tool-reinspection.recovery.intent_unavailable')});}},[action.busy,!!action.pending,action.errorStatus]);
 async function refresh(){if(!alive.current||caller.current!==api||incoming.current.accessBlocked||commandBusy.current)return;const before=original.current,next=await loadSaved();if(!alive.current||caller.current!==api)return;if(before&&(!next.intent||canonical(before)!==canonical(next.intent)))save({ready:true,intent:before,error:new UiError('tool-reinspection.recovery.intent_unavailable')});else save(next);setPaused(false);polls.current=0;const ok=await readNow.current();if(ok&&writeDenied)action.setError('');}
 async function send(restore=false){
  if(!alive.current||caller.current!==api||commandBusy.current||incoming.current.accessBlocked||gate.current.readBlocked)return;let intent=original.current;const capturedSource=sourceRef.current;commandBusy.current=true;callbacks.current.onBusyChange?.(true);setBusy(true);
  try{
   if(restore){if(!intent||!matches)return;firstFresh.current=undefined;}
   else{
    const target=gate.current.target;if(gate.current.freshBlocked||!confirming||!target||intent)return;const key=operation();intent=await buildToolInspectionRecoveryIntent(target,scope,{path:'/tool-inspections/'+encodeURIComponent(target.id)+'/recoveries',method:'POST',idempotency_key:key,body:{project_id:scope.project,expected_version:target.version,inspection:{inspection_id:target.id,claim_id:target.claim_id,fence:target.fence},idempotency_key:key}});
    if(!intent||!alive.current||caller.current!==api||sourceRef.current!==capturedSource||incoming.current.accessBlocked||incoming.current.disabled)return;
    try{if(!storage||storage.getItem(intentKey)!==null)throw Error();storage.setItem(intentKey,JSON.stringify(intent));save({ready:true,intent});firstFresh.current=intent;}catch{save({ready:true,error:new UiError('tool-reinspection.recovery.intent_unavailable')});return;}
   }
   try{if(!storage||storage.getItem(intentKey)!==JSON.stringify(intent))throw Error();}catch{save({ready:true,intent,error:new UiError('tool-reinspection.recovery.intent_unavailable')});return;}
   const result=settled.current&&canonical(settled.current.intent)===canonical(intent)?settled.current.row:await action.run<ToolInspectionRecovery>(intent.request.path,intent.request.body,intent.request.method);
   if(!result||!alive.current||caller.current!==api||original.current!==intent||incoming.current.accessBlocked)return;settled.current={intent,row:result};try{removeMetadata(intent);}catch{save({ready:true,intent,error:new UiError('tool-reinspection.recovery.intent_unavailable')});return;}settled.current=undefined;selectedRef.current=result.id;setSelected(result.id);setConfirming(false);polls.current=0;setPaused(false);
  }finally{commandBusy.current=false;if(alive.current&&caller.current===api){setBusy(false);callbacks.current.onBusyChange?.(false);}}
 }
 function defer(){if(alive.current&&caller.current===api&&!action.busy&&!commandBusy.current)callbacks.current.onDefer?.();}
 if(saved.ready&&!target&&!saved.error&&!action.pending)return null;
 return <section className="tool-inspection-recovery" data-tool-inspection-recovery-panel aria-busy={busy||action.busy||visible.loading}><h4>{t('title')}</h4><p className="tool-reinspection-boundary">{t('boundary')}</p>
  {hidden?<p role="alert">{t('access_denied')}</p>:<>
   {target&&<p><strong>{target.worker_id}</strong> · <code>{target.id}</code></p>}
   {saved.intent?<p role="status">{t('original_pending')}</p>:row?<p role="status" data-tool-recovery-state={row.state}>{t('state.'+row.state)}</p>:<p>{t(visible.loading||!saved.ready?'loading':'no_authorization')}</p>}
   {!saved.intent&&!row?.proof&&<p>{t('proof_missing')}</p>}
   {target&&<details data-tool-recovery-source><summary>{t('source')}</summary><dl>{([['request',target.id],['claim',target.claim_id??''],['fence',target.fence],['profile',target.tool_profile_id],['revision',target.tool_profile_version],['digest',target.profile_sha256]] as const).map(([key,value])=><div key={key}><dt>{tr('tool-reinspection.'+key)}</dt><dd><code>{value}</code></dd></div>)}</dl></details>}
   {row?.proof&&<details data-tool-recovery-proof><summary>{t('proof')}</summary><p>{t('native_count',{count:row.proof.native_process_count})}</p><p>{t('proof_facts')}</p><code>{row.proof_sha256}</code></details>}
   {!known&&!saved.intent&&<p>{t('capability_required')}</p>}
   {!!visible.error&&<ErrorNotice error={visible.error}/>}{!!(saved.error||action.error)&&<ErrorNotice error={saved.error??action.error}/>}
   {confirming&&!saved.intent&&<div className="tool-recovery-confirm"><p>{t('confirm')}</p><button type="button" className="primary" data-tool-recovery-confirm disabled={freshBlocked||busy} onClick={()=>void send()}>{t('confirm_action')}</button><button type="button" disabled={busy} onClick={()=>setConfirming(false)}>{t('cancel')}</button></div>}
  </>}
  {paused&&<p>{tr('tool-reinspection.background_paused')}</p>}
  <div className="tool-reinspection-actions">{!hidden&&saved.intent&&<button type="button" className="primary" data-tool-recovery-restore disabled={busy||action.busy||readBlocked||!matches} onClick={()=>void send(true)}>{t('restore')}</button>}{!hidden&&!saved.intent&&!confirming&&target?.state==='unknown'&&!rows.some(active)&&row?.state!=='closed_unknown'&&<button type="button" className="primary" data-tool-recovery-authorize disabled={freshBlocked||busy} onClick={()=>{if(!gate.current.freshBlocked&&!commandBusy.current)setConfirming(true);}}>{t('authorize')}</button>}<button type="button" data-tool-recovery-refresh disabled={busy||action.busy||visible.loading||accessBlocked||!saved.ready} onClick={()=>void refresh()}>{t('refresh')}</button>{!hidden&&target&&onOpenWorker&&!row?.proof&&<button type="button" disabled={busy||action.busy} onClick={()=>{if(alive.current&&caller.current===api&&!commandBusy.current&&!gate.current.readBlocked)callbacks.current.onOpenWorker?.(target.worker_id);}}>{t('open_worker')}</button>}{onDefer&&<button type="button" data-tool-recovery-defer disabled={busy||action.busy} onClick={defer}>{tr('tool-reinspection.defer')}</button>}</div>
 </section>;
}
