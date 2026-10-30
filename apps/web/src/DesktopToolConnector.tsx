import {useEffect,useRef,useState} from 'react';
import {ChevronDown,Monitor,RefreshCw,Square} from 'lucide-react';
import type {WorkspaceMetadata} from '../../../packages/contracts/src';
import {useApi,useApiView} from './api-context';
import type {ApiCaller} from './api';
import {DesktopWorkspacePicker} from './DesktopWorkspacePicker';
import {validWorkspaceMetadata} from './desktop-workspace-model';
import {desktopToolBridge,readDesktopToolResult,validDesktopToolScope,type DesktopToolAction,type DesktopToolBridge,type DesktopToolConnection,type DesktopToolPhase,type DesktopToolScope} from './desktop-tool-connector-model';
import {tr,useLocale} from './i18n';
import './DesktopToolConnector.css';

const t=(key:string)=>tr('desktop-tool-connector.'+key);
export type DesktopToolConnectorProps={project:string;adapterID:string;onConnected:(workerID:string)=>void;secondary?:boolean;disabled?:boolean};
export function DesktopToolConnector(props:DesktopToolConnectorProps){
 const api=useApi(),view=useApiView(),bridge=desktopToolBridge();
 const scope={project:props.project,userID:view?.userID??'',organizationID:view?.organizationID??'',adapterID:props.adapterID};
 if(!bridge||!validDesktopToolScope(scope))return null;
 return <ConnectorScope {...props} key={JSON.stringify(scope)} scope={scope} api={api} bridge={bridge}/>;
}
type Snapshot={api:ApiCaller;bridge:DesktopToolBridge;connection?:DesktopToolConnection;uncertain?:DesktopToolAction;failed?:boolean;refusal?:DesktopToolPhase};
const refusals=new Set(['authorization_required','binding_changed','origin_changed','credential_unrecoverable','stop_unconfirmed','needs_workspace','not_installed','unavailable']);
function ConnectorScope({scope,api,bridge,disabled=false,secondary=false,onConnected}:DesktopToolConnectorProps&{scope:DesktopToolScope;api:ApiCaller;bridge:DesktopToolBridge}){
 const locale=useLocale(),[snapshot,setSnapshot]=useState<Snapshot>(),[busy,setBusy]=useState<DesktopToolAction>(),[workspaceBlocked,setWorkspaceBlocked]=useState(false);
 const intent=useRef({api,bridge,epoch:0}),mounted=useRef(true),inflight=useRef(false),reported=useRef('');
 if(intent.current.api!==api||intent.current.bridge!==bridge)intent.current={api,bridge,epoch:intent.current.epoch+1};
 const current=snapshot?.api===api&&snapshot.bridge===bridge?snapshot:undefined,connection=current?.connection;
 const phase=current?.refusal??(current?.uncertain?(current.uncertain==='stop'?'stop_unconfirmed':'issuance_unknown'):connection?.phase);
 const live=(epoch:number)=>mounted.current&&epoch===intent.current.epoch;
 function unconfirmed(previous:Snapshot|undefined,action:DesktopToolAction,code?:string):Snapshot{
  const before=previous?.api===api&&previous.bridge===bridge?previous:undefined,connection=before?.connection;
  const priorRefusal=before?.refusal??(connection&&refusals.has(connection.phase)?connection.phase:undefined);
  const refusal=code==='managed_runner_access_changed'?'authorization_required':code&&refusals.has(code)?code as DesktopToolPhase:action==='status'?priorRefusal:undefined;
  const uncertain=action==='status'?before?.uncertain??(connection?.phase==='stopping'||connection?.phase==='stop_unconfirmed'?'stop':'status'):action;
  return {api,bridge,connection,uncertain,failed:true,...(refusal?{refusal}:{})};
 }
 async function run(action:DesktopToolAction){
  if(inflight.current||action!=='status'&&(disabled||workspaceBlocked))return;
  const epoch=intent.current.epoch;inflight.current=true;setBusy(action);
  try{
   const raw=await bridge.manageLocalTool({action,project_id:scope.project,user_id:scope.userID,tenant_id:scope.organizationID,adapter_id:scope.adapterID,locale});
   if(!live(epoch))return;const result=readDesktopToolResult(raw,scope);
   if(!result)throw Error('unconfirmed native response');
   if(result.status==='failed')setSnapshot(previous=>unconfirmed(previous,action,result.code));
   else setSnapshot({api,bridge,connection:result.connection});
  }catch{if(live(epoch))setSnapshot(previous=>unconfirmed(previous,action));}
  finally{if(live(epoch)){inflight.current=false;setBusy(undefined);}}
 }
 useEffect(()=>{
  mounted.current=true;inflight.current=false;setBusy(undefined);setSnapshot(undefined);reported.current='';void run('status');
  return()=>{mounted.current=false;intent.current.epoch++;};
 },[api,bridge]);
 useEffect(()=>{
  if(!current)return;
  // Reset only after a confirmed state change, never while a read is loading
  // or its result is unknown. Reconnecting the original node needs a new read
  // of its inventory even though its worker identity remains unchanged.
  if(current.refusal){reported.current='';return;}
  if(current.uncertain||!connection)return;
  if(connection.phase!=='connected'){reported.current='';return;}
  if(!connection.worker_id)return;
  if(reported.current===connection.worker_id)return;reported.current=connection.worker_id;onConnected(connection.worker_id);
 },[current,connection,onConnected]);
 useEffect(()=>{
  if(busy||!connection||current?.uncertain)return;
  const wait=connection.phase==='starting'||connection.phase==='stopping'?2000:connection.phase==='connected'&&connection.expires_at?Math.max(1000,Date.parse(connection.expires_at)-Date.now()):undefined;
  if(wait===undefined)return;const timer=setTimeout(()=>void run('status'),wait);return()=>clearTimeout(timer);
 },[connection,busy,current]);
 const blocked=!!busy||disabled||workspaceBlocked;
 const resume=!!current?.uncertain||phase==='prepared'||phase==='issuance_unknown'||phase==='failed';
 const canConnect=!!current&&phase!=='connected'&&phase!=='starting'&&phase!=='stopping'&&phase!=='stop_unconfirmed'&&phase!=='needs_workspace'&&phase!=='binding_changed'&&phase!=='origin_changed'&&phase!=='credential_unrecoverable'&&phase!=='authorization_required';
 const canStop=!!connection?.worker_id&&(phase==='connected'||phase==='starting');
 const content=<>
  <p className="desktop-tool-description">{t('help')}</p>
  <p role="status" className={current?.failed||phase==='stop_unconfirmed'?'desktop-tool-warning':'desktop-tool-state'}>{t(busy?busy==='status'?'checking':busy==='connect'?'connecting':'stopping':phase??'checking')}</p>
  {current?.uncertain&&!current.refusal&&<p className="desktop-tool-warning">{t('unknown')}</p>}
  {connection?.repository_name&&<p className="desktop-tool-repository">{connection.repository_name}</p>}
  <div className="desktop-tool-actions">
   {canConnect&&<button data-local-tool-connect type="button" className={secondary?'':'primary'} disabled={blocked} onClick={()=>void run('connect')}><Monitor size={15} aria-hidden="true"/>{t(resume?'resume':'connect')}</button>}
   <button data-local-tool-status type="button" disabled={blocked} onClick={()=>void run('status')}><RefreshCw size={14} aria-hidden="true"/>{t(current?.uncertain||phase==='issuance_unknown'||phase==='stop_unconfirmed'?'check_original':'refresh')}</button>
   {canStop&&<button data-local-tool-stop type="button" disabled={blocked} onClick={()=>void run('stop')}><Square size={13} aria-hidden="true"/>{t('stop')}</button>}
  </div>
  {phase==='needs_workspace'&&<ConnectorWorkspace api={api} project={scope.project} disabled={!!busy||disabled} onBlockedChange={setWorkspaceBlocked} onBound={()=>void run('status')}/>}
  <p className="desktop-tool-boundary">{t('boundary')}</p>
 </>;
 return <section className="desktop-tool-connector" aria-label={t('title')} aria-busy={!!busy}>
  {secondary?<details><summary><Monitor size={16} aria-hidden="true"/>{t('title')}<ChevronDown className="desktop-tool-disclosure" size={15} aria-hidden="true"/></summary><div className="desktop-tool-content">{content}</div></details>:<><div className="desktop-tool-heading"><Monitor size={16} aria-hidden="true"/><strong>{t('title')}</strong></div>{content}</>}
 </section>;
}
function ConnectorWorkspace({api,project,disabled,onBlockedChange,onBound}:{api:ApiCaller;project:string;disabled:boolean;onBlockedChange:(blocked:boolean)=>void;onBound:()=>void}){
 const [snapshot,setSnapshot]=useState<{api:ApiCaller;metadata?:WorkspaceMetadata;failed?:boolean}>(),[revision,setRevision]=useState(0),epoch=useRef(0);
 async function refresh(){
  const generation=epoch.current;const value=await api<unknown>('/workspace?project_id='+encodeURIComponent(project),undefined,'GET');
  if(generation!==epoch.current||!validWorkspaceMetadata(value,project))return;
  setSnapshot({api,metadata:value});return value;
 }
 useEffect(()=>{let active=true;void refresh().then(value=>{if(active&&!value)setSnapshot({api,failed:true});}).catch(()=>{if(active)setSnapshot({api,failed:true});});return()=>{active=false;epoch.current++;};},[api,project,revision]);
 const current=snapshot?.api===api?snapshot:undefined;
 if(!current)return <p role="status">{t('workspace_reading')}</p>;
 if(!current.metadata)return <div role="alert"><p>{t('workspace_failed')}</p><button type="button" disabled={disabled} onClick={()=>setRevision(value=>value+1)}>{t('refresh')}</button></div>;
 return <DesktopWorkspacePicker project={project} metadata={current.metadata} active disabled={disabled} onBound={onBound} onRefresh={refresh} onBlockedChange={onBlockedChange}/>;
}
