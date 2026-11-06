import {useEffect,useRef,useState} from 'react';
import type {ToolProfile,ToolProfileDiagnostics} from '../../../packages/contracts/src';
import {ApiError,type ApiCaller} from './api';
import {useApi,useApiView} from './api-context';
import {ToolDoctorCard,type ToolDoctorCheckActions,type ToolDoctorMaintenanceAction} from './ToolDoctorCard';
import {readToolProfileDiagnostics} from './tool-profile-diagnostics-model';
import {sameToolProfile,toolProfileMatchKey} from './tool-profile-reuse-model';
import {ToolProfileImpact,type ToolProfileImpactProps} from './ToolProfileImpact';
import type {ToolInspectionLocator} from './tool-reinspection-model';
import {tr,useLocale} from './i18n';
import './ToolProfileDoctor.css';

export type ToolProfileDoctorProps={project:string;tool:ToolProfile;confirmed?:boolean;autoRefresh?:boolean;pending?:boolean;disabled?:boolean;onReview?:(tool:ToolProfile)=>void;onWorker?:(workerID:string)=>void;onSharedWorker?:(workerID:string)=>void;onRegister?:(tool:ToolProfile)=>void;onInspect?:(tool:ToolProfile,locator?:ToolInspectionLocator)=>void;onRun?:ToolProfileImpactProps['onRun'];onRefreshTool?:()=>void};
type Props=ToolProfileDoctorProps;
const t=(key:string)=>tr('tool-doctor.'+key);
type Snapshot={caller:ApiCaller;key:string;deadline:number;started:number;report?:ToolProfileDiagnostics;error?:unknown};
type AccessState={source:string;caller:ApiCaller;denied:boolean};

/** Current reads stay separate from the original versioned write and receipt. */
export function ToolProfileDoctor(props:Props){
 const api=useApi(),view=useApiView();
 const source=JSON.stringify([view?.userID,view?.organizationID,props.project,props.tool.id,props.tool.version,props.tool.state,props.tool.enabled_by,toolProfileMatchKey({project_id:props.project,worker_id:props.tool.worker_id,profile:props.tool.profile})]);
 const access=useRef<AccessState>({source,caller:api,denied:false});
 if(access.current.source!==source||access.current.caller!==api)access.current={source,caller:api,denied:false};
 if(!view||props.tool.project_id!==props.project)return <ToolDoctorCard state="suspended" disabled onRefresh={()=>{}}/>;
 return <ToolDoctorReader key={source} {...props} api={api} source={source} access={access.current} userID={view.userID} organizationID={view.organizationID}/>;
}
function ToolDoctorReader({api,source,access,userID,organizationID,...props}:Props&{api:ApiCaller;source:string;access:AccessState;userID:string;organizationID:string}){
 useLocale();
 const [revision,setRevision]=useState(0),[snapshot,setSnapshot]=useState<Snapshot>(),[,tick]=useState(0);
 const key=JSON.stringify([source,revision]),intent=useRef({api,key}),mounted=useRef(false),settled=useRef(snapshot);intent.current={api,key};settled.current=snapshot;
 const suspended=!props.confirmed||!!props.pending||!!props.disabled;
 const admission=useRef(false);admission.current=!suspended;
 useEffect(()=>{mounted.current=true;return()=>{mounted.current=false;};},[]);
 const reader=useRef<{active:boolean;started:boolean;controller?:AbortController}|undefined>(undefined);
 // Only a real write lock, identity/source change or unmount cancels a read.
 // A transient detail refresh hides actions but preserves its in-flight owner.
 useEffect(()=>{
  const owner={active:true,started:false,controller:undefined as AbortController|undefined};reader.current=owner;
  return()=>{owner.active=false;owner.controller?.abort();};
 },[api,key,props.pending,props.disabled]);
 useEffect(()=>{
  const owner=reader.current;
  if(suspended||!owner?.active||owner.started||settled.current?.caller===api&&settled.current.key===key)return;
  owner.started=true;const controller=new AbortController();owner.controller=controller;const started=performance.now();
  setSnapshot(undefined);
  void api<unknown>('/tool-profiles/'+encodeURIComponent(props.tool.id)+'/diagnostics?project_id='+encodeURIComponent(props.project),undefined,'GET',{'X-Suma-Expected-User':userID,'X-Suma-Expected-Organization':organizationID},controller.signal).then(value=>{
   if(!owner.active||reader.current!==owner)return;
   const {report,remainingMs}=readToolProfileDiagnostics(value,{project:props.project,toolID:props.tool.id});
   access.denied=false;
   setSnapshot({caller:api,key,report,started,deadline:started+remainingMs});
  }).catch(error=>{if(owner.active&&reader.current===owner&&!controller.signal.aborted){if(error instanceof ApiError&&[401,403,404].includes(error.status))access.denied=true;setSnapshot({caller:api,key,started,deadline:0,error});}});
 },[api,key,props.project,props.tool.id,userID,organizationID,suspended,props.pending,props.disabled]);
 const current=snapshot?.caller===api&&snapshot.key===key?snapshot:undefined,report=current?.report;
 useEffect(()=>{
  if(!report||!current)return;
  let timer:ReturnType<typeof setTimeout>|undefined;
  function expire(){
   const remaining=current!.deadline-performance.now();
   if(remaining<=0){tick(value=>value+1);return;}
   timer=setTimeout(expire,Math.ceil(remaining));
  }
  expire();return()=>clearTimeout(timer);
 },[current,report]);
 const changed=!!report&&(report.tool.version!==props.tool.version||report.tool.worker_id!==props.tool.worker_id||report.tool.state!==props.tool.state||report.tool.enabled_by!==props.tool.enabled_by||!sameToolProfile(report.tool.profile,props.tool.profile));
 const state=suspended?'suspended':!current?'loading':current.error?'error':!report?'loading':changed?'changed':performance.now()>=current.deadline?'expired':'ready';
 // Only the selected visible profile is read automatically. The workspace's
 // existing live switch pauses it; failures and source drift require an explicit
 // retry. No probe, repair, registration or authorization is triggered here.
 useEffect(()=>{
  if(suspended||!props.autoRefresh||!current?.report||current.error||changed||access.denied)return;
  let requested=false,timer:ReturnType<typeof setTimeout>|undefined;const due=Math.max(current.deadline,current.started+15000);
  const recheck=()=>{
   if(requested||!admission.current||document.visibilityState!=='visible'||!mounted.current||intent.current.api!==api||intent.current.key!==key)return;
   const remaining=due-performance.now();
   if(remaining>0){clearTimeout(timer);timer=setTimeout(recheck,Math.ceil(remaining));return;}
   requested=true;setRevision(value=>value+1);
  };
  timer=setTimeout(recheck,Math.max(0,Math.ceil(due-performance.now())));
  document.addEventListener('visibilitychange',recheck);window.addEventListener('focus',recheck);
  return()=>{clearTimeout(timer);document.removeEventListener('visibilitychange',recheck);window.removeEventListener('focus',recheck);};
 },[api,key,current,changed,props.autoRefresh,access,suspended]);
 function live(){return admission.current&&mounted.current&&intent.current.api===api&&intent.current.key===key&&state==='ready'&&!!current&&performance.now()<current.deadline;}
 function guarded(run:()=>void){if(live())run();else tick(value=>value+1);}
 const workerDestination=props.onWorker??props.onSharedWorker,sharedDestination=!props.onWorker&&!!props.onSharedWorker,denied=access.denied;
 function refresh(){if(admission.current&&mounted.current&&intent.current.api===api&&intent.current.key===key)setRevision(value=>value+1);}
 function resolveAction(action:ToolProfileDiagnostics['next_action']):ToolDoctorCheckActions[keyof ToolDoctorCheckActions]{
  if(state!=='ready'||!report)return;
  if(action==='review_profile'&&report.tool.state!=='revoked'&&props.onReview)return {action,onClick:()=>guarded(()=>props.onReview!(report.tool))};
  if(action==='open_worker'&&workerDestination)return {action,onClick:()=>guarded(()=>workerDestination(report.tool.worker_id))};
  if(action==='register_profile'&&props.onRegister)return {action,onClick:()=>guarded(()=>props.onRegister!(report.tool))};
  if(action==='refresh')return {action,onClick:()=>guarded(refresh)};
 }
 const resolvedPrimary=resolveAction(report?.next_action??'none'),primaryAction=resolvedPrimary&&resolvedPrimary.action!=='refresh'?{action:resolvedPrimary.action,onClick:resolvedPrimary.onClick}:undefined;
 function resolveMaintenance():ToolDoctorMaintenanceAction|undefined{
  if(state!=='ready'||!report)return;
  const resolution=report.maintenance_resolution,action=resolution.action;
  if(action==='open_worker'&&workerDestination)return {action,onClick:()=>guarded(()=>workerDestination(report.tool.worker_id))};
  if(action==='inspect'&&props.onInspect&&report.tool.state!=='revoked')return {action,onClick:()=>guarded(()=>props.onInspect!(report.tool))};
  if((action==='view_inspection'||action==='recover_inspection'||action==='view_recovery')&&props.onInspect&&resolution.inspection_id){
   const locator:ToolInspectionLocator={inspectionID:resolution.inspection_id,...(resolution.recovery_id?{recoveryID:resolution.recovery_id}:{})};
   return {action,onClick:()=>guarded(()=>props.onInspect!(report.tool,locator))};
  }
 }
 const maintenanceAction=resolveMaintenance();
 const checkActions:ToolDoctorCheckActions={};
 for(const check of report?.checks??[]){if(check.status==='pass')continue;const action=resolveAction(check.next_action);if(action)checkActions[check.code]=action;}
 return <div className="tool-profile-doctor">
  {props.autoRefresh!==undefined&&<p className="muted" data-tool-doctor-auto={props.autoRefresh?'enabled':'paused'}>{t(props.autoRefresh?'auto_read':'auto_paused')}</p>}
  {!suspended&&!denied&&<div className="tool-profile-doctor-target" data-tool-profile-doctor-target><p><span>{t('selected_target')}</span><strong>{props.tool.name}</strong></p>{sharedDestination&&<p className="tool-profile-doctor-hint">{t('shared_readonly')}</p>}<details data-tool-profile-doctor-basis><summary>{t('target_details')}</summary><dl>{([['target_project',props.project],['target_worker',props.tool.worker_id],['target_adapter',props.tool.profile.adapter],['target_protocol',props.tool.profile.protocol],['target_profile',props.tool.id],['target_hash',props.tool.profile.profile_sha256]] as const).map(([label,value])=><div key={label}><dt>{t(label)}</dt><dd><code>{value}</code></dd></div>)}</dl></details></div>}
  <ToolDoctorCard report={report} state={state} error={current?.error} primaryAction={primaryAction} maintenanceAction={maintenanceAction} checkActions={checkActions} onRefresh={refresh} onRefreshTool={state==='changed'&&props.onRefreshTool?()=>{if(admission.current&&mounted.current&&intent.current.api===api&&intent.current.key===key)props.onRefreshTool!();}:undefined}/>
  {!denied&&<ToolProfileImpact impact={state==='ready'?report?.impact:undefined} onRun={state==='ready'&&props.onRun?(runID,nodeID,attempt)=>guarded(()=>props.onRun!(runID,nodeID,attempt)):undefined}/>}
 </div>;
}
