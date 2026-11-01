import {useEffect,useRef,useState} from 'react';
import type {ToolProfile,ToolProfileDiagnostics} from '../../../packages/contracts/src';
import {ApiError,type ApiCaller} from './api';
import {useApi,useApiView} from './api-context';
import {ToolDoctorCard} from './ToolDoctorCard';
import {readToolProfileDiagnostics} from './tool-profile-diagnostics-model';
import {sameToolProfile,toolProfileMatchKey} from './tool-profile-reuse-model';
import {ToolProfileImpact} from './ToolProfileImpact';
import {tr,useLocale} from './i18n';
import './ToolProfileDoctor.css';

export type ToolProfileDoctorProps={project:string;tool:ToolProfile;confirmed?:boolean;pending?:boolean;disabled?:boolean;onReview?:(tool:ToolProfile)=>void;onWorker?:(workerID:string)=>void;onSharedWorker?:(workerID:string)=>void;onRegister?:(tool:ToolProfile)=>void;onRun?:(runID:string,nodeID:string)=>void;onRefreshTool?:()=>void};
type Props=ToolProfileDoctorProps;
const t=(key:string)=>tr('tool-doctor.'+key);
type Snapshot={caller:ApiCaller;key:string;deadline:number;report?:ToolProfileDiagnostics;error?:unknown};
type AccessState={source:string;caller:ApiCaller;denied:boolean};

/** Current reads stay separate from the original versioned write and receipt. */
export function ToolProfileDoctor(props:Props){
 const api=useApi(),view=useApiView();
 const source=JSON.stringify([view?.userID,view?.organizationID,props.project,props.tool.id,props.tool.version,props.tool.state,props.tool.enabled_by,toolProfileMatchKey({project_id:props.project,worker_id:props.tool.worker_id,profile:props.tool.profile})]);
 const access=useRef<AccessState>({source,caller:api,denied:false});
 if(access.current.source!==source||access.current.caller!==api)access.current={source,caller:api,denied:false};
 if(!view||!props.confirmed||props.pending||props.disabled||props.tool.project_id!==props.project)return <ToolDoctorCard state="suspended" disabled onRefresh={()=>{}}/>;
 return <ToolDoctorReader key={source} {...props} api={api} source={source} access={access.current} userID={view.userID} organizationID={view.organizationID}/>;
}
function ToolDoctorReader({api,source,access,userID,organizationID,...props}:Props&{api:ApiCaller;source:string;access:AccessState;userID:string;organizationID:string}){
 useLocale();
 const [revision,setRevision]=useState(0),[snapshot,setSnapshot]=useState<Snapshot>(),[,tick]=useState(0);
 const key=JSON.stringify([source,revision]),intent=useRef({api,key}),mounted=useRef(false);intent.current={api,key};
 useEffect(()=>{mounted.current=true;return()=>{mounted.current=false;};},[]);
 useEffect(()=>{
  const controller=new AbortController();let latest=true;const started=performance.now();
  setSnapshot(undefined);
  void api<unknown>('/tool-profiles/'+encodeURIComponent(props.tool.id)+'/diagnostics?project_id='+encodeURIComponent(props.project),undefined,'GET',{'X-Suma-Expected-User':userID,'X-Suma-Expected-Organization':organizationID},controller.signal).then(value=>{
   if(!latest)return;
   const {report,remainingMs}=readToolProfileDiagnostics(value,{project:props.project,toolID:props.tool.id});
   access.denied=false;
   setSnapshot({caller:api,key,report,deadline:started+remainingMs});
  }).catch(error=>{if(latest&&!controller.signal.aborted){if(error instanceof ApiError&&[401,403,404].includes(error.status))access.denied=true;setSnapshot({caller:api,key,deadline:0,error});}});
  return()=>{latest=false;controller.abort();};
 },[api,key,props.project,props.tool.id,userID,organizationID]);
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
 const state=!current?'loading':current.error?'error':!report?'loading':changed?'changed':performance.now()>=current.deadline?'expired':'ready';
 function live(){return mounted.current&&intent.current.api===api&&intent.current.key===key&&state==='ready'&&!!current&&performance.now()<current.deadline;}
 function guarded(run:()=>void){if(live())run();else tick(value=>value+1);}
 const workerDestination=props.onWorker??props.onSharedWorker,sharedDestination=!props.onWorker&&!!props.onSharedWorker,denied=access.denied;
 const primaryAction=state!=='ready'||!report?undefined:
  report.next_action==='review_profile'&&report.tool.state!=='revoked'&&props.onReview?{action:'review_profile' as const,onClick:()=>guarded(()=>props.onReview!(report.tool))}:
  report.next_action==='open_worker'&&workerDestination?{action:'open_worker' as const,onClick:()=>guarded(()=>workerDestination(report.tool.worker_id))}:
  report.next_action==='register_profile'&&props.onRegister?{action:'register_profile' as const,onClick:()=>guarded(()=>props.onRegister!(report.tool))}:undefined;
 return <div className="tool-profile-doctor">
  {!denied&&<div className="tool-profile-doctor-target" data-tool-profile-doctor-target><p><span>{t('selected_target')}</span><strong>{props.tool.name}</strong></p>{sharedDestination&&<p className="tool-profile-doctor-hint">{t('shared_readonly')}</p>}<details data-tool-profile-doctor-basis><summary>{t('target_details')}</summary><dl>{([['target_project',props.project],['target_worker',props.tool.worker_id],['target_adapter',props.tool.profile.adapter],['target_protocol',props.tool.profile.protocol],['target_profile',props.tool.id],['target_hash',props.tool.profile.profile_sha256]] as const).map(([label,value])=><div key={label}><dt>{t(label)}</dt><dd><code>{value}</code></dd></div>)}</dl></details></div>}
  <ToolDoctorCard report={report} state={state} error={current?.error} primaryAction={primaryAction} onRefresh={()=>{if(mounted.current&&intent.current.api===api&&intent.current.key===key)setRevision(value=>value+1);}} onRefreshTool={state==='changed'&&props.onRefreshTool?()=>{if(mounted.current&&intent.current.api===api&&intent.current.key===key)props.onRefreshTool!();}:undefined}/>
  {!denied&&<ToolProfileImpact impact={state==='ready'?report?.impact:undefined} onRun={state==='ready'&&props.onRun?(runID,nodeID)=>guarded(()=>props.onRun!(runID,nodeID)):undefined}/>}
 </div>;
}
