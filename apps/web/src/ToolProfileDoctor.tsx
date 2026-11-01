import {useEffect,useRef,useState} from 'react';
import type {ToolProfile,ToolProfileDiagnostics} from '../../../packages/contracts/src';
import type {ApiCaller} from './api';
import {useApi,useApiView} from './api-context';
import {ToolDoctorCard} from './ToolDoctorCard';
import {readToolProfileDiagnostics} from './tool-profile-diagnostics-model';
import {sameToolProfile,toolProfileMatchKey} from './tool-profile-reuse-model';

type Props={project:string;tool:ToolProfile;confirmed?:boolean;pending?:boolean;disabled?:boolean;onReview?:(tool:ToolProfile)=>void;onWorker?:(workerID:string)=>void;onRegister?:()=>void;onRefreshTool?:()=>void};
type Snapshot={caller:ApiCaller;key:string;deadline:number;report?:ToolProfileDiagnostics;error?:unknown};

/** Current reads stay separate from the original versioned write and receipt. */
export function ToolProfileDoctor(props:Props){
 const api=useApi(),view=useApiView();
 const source=JSON.stringify([view?.userID,view?.organizationID,props.project,props.tool.id,props.tool.version,props.tool.state,props.tool.enabled_by,toolProfileMatchKey({project_id:props.project,worker_id:props.tool.worker_id,profile:props.tool.profile})]);
 if(!view||!props.confirmed||props.pending||props.disabled||props.tool.project_id!==props.project)return <ToolDoctorCard state="suspended" disabled onRefresh={()=>{}}/>;
 return <ToolDoctorReader key={source} {...props} api={api} source={source} userID={view.userID} organizationID={view.organizationID}/>;
}
function ToolDoctorReader({api,source,userID,organizationID,...props}:Props&{api:ApiCaller;source:string;userID:string;organizationID:string}){
 const [revision,setRevision]=useState(0),[snapshot,setSnapshot]=useState<Snapshot>(),[,tick]=useState(0);
 const key=JSON.stringify([source,revision]),intent=useRef({api,key}),mounted=useRef(false);intent.current={api,key};
 useEffect(()=>{mounted.current=true;return()=>{mounted.current=false;};},[]);
 useEffect(()=>{
  const controller=new AbortController();let latest=true;const started=performance.now();
  setSnapshot(undefined);
  void api<unknown>('/tool-profiles/'+encodeURIComponent(props.tool.id)+'/diagnostics?project_id='+encodeURIComponent(props.project),undefined,'GET',{'X-Suma-Expected-User':userID,'X-Suma-Expected-Organization':organizationID},controller.signal).then(value=>{
   if(!latest)return;
   const {report,remainingMs}=readToolProfileDiagnostics(value,{project:props.project,toolID:props.tool.id});
   setSnapshot({caller:api,key,report,deadline:started+remainingMs});
  }).catch(error=>{if(latest&&!controller.signal.aborted)setSnapshot({caller:api,key,deadline:0,error});});
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
 const primaryAction=state!=='ready'||!report?undefined:
  report.next_action==='review_profile'&&report.tool.state!=='revoked'&&props.onReview?{action:'review_profile' as const,onClick:()=>guarded(()=>props.onReview!(report.tool))}:
  report.next_action==='open_worker'&&props.onWorker?{action:'open_worker' as const,onClick:()=>guarded(()=>props.onWorker!(report.tool.worker_id))}:
  report.next_action==='register_profile'&&props.onRegister?{action:'register_profile' as const,onClick:()=>guarded(()=>props.onRegister!())}:undefined;
 return <ToolDoctorCard report={report} state={state} error={current?.error} primaryAction={primaryAction} onRefresh={()=>{if(mounted.current&&intent.current.api===api&&intent.current.key===key)setRevision(value=>value+1);}} onRefreshTool={state==='changed'&&props.onRefreshTool?()=>{if(mounted.current&&intent.current.api===api&&intent.current.key===key)props.onRefreshTool!();}:undefined}/>;
}
