import {useCallback,useEffect,useRef,useState} from 'react';
import {WorkBuddyCloudCollaboration} from './WorkBuddyCloudCollaboration';
import {CloudHistorySharing,type CloudHistoryShareSelection} from './CloudHistorySharing';
import type {CloudNodeView,WorkBuddyConnection} from '../../../packages/contracts/src';
import {ApiError,type ApiCaller} from './api';
import {useApi,useApiView} from './api-context';
import {tr,UiError,useLocale} from './i18n';
import {toolSessionStorage} from './tool-hooks';
import {useWriteAction,WriteNotice} from './write-action';
import {WorkBuddyCloudTaskPanel,type WorkBuddyCloudActionResult,type WorkBuddyCloudDelegateDraft,type WorkBuddyCloudTaskModel} from './WorkBuddyCloudTaskPanel';
import {cloudDelegateBody,cloudNodePath,cloudRecoveryAction,readCloudConnection,readCloudNode,validCloudReceipt,validCloudRecovery,validCloudScope,type CloudTaskScope,type CloudRunFacts} from './workbuddy-cloud-task-model';

type Props={project:string;runID:string;nodeID:string;revision?:number;execution?:CloudRunFacts;taskTitle?:string;onChanged?:()=>void|Promise<void>};
type ReadResult='ready'|'denied'|'failed';
type Snapshot={caller:ApiCaller;node?:CloudNodeView;connection?:WorkBuddyConnection;reading:boolean;denied:boolean;sourceFailed:boolean;connectionFailed:boolean};
const t=(key:string,values?:Record<string,unknown>)=>tr('workbuddy-cloud-task.'+key,values);
const refused=(error:unknown)=>error instanceof ApiError&&[401,403,404].includes(error.status);

export function WorkBuddyCloudTaskWorkspace(props:Props){
 const view=useApiView(),key=JSON.stringify([view?.userID,view?.organizationID,props.project,props.runID,props.nodeID]);
 return <CloudTaskController key={key} {...props}/>;
}

function CloudTaskController({project,runID,nodeID,revision,execution,taskTitle,onChanged}:Props){
 useLocale();const caller=useApi(),view=useApiView();
 const scope:CloudTaskScope={project,runID,nodeID,userID:view?.userID||'',organizationID:view?.organizationID||''};
 const scopeKey=JSON.stringify([scope.userID,scope.organizationID,project,runID,nodeID]);
 const [snapshot,setSnapshot]=useState<Snapshot>(),[classification,setClassification]=useState(0),[parentRefreshFailed,setParentRefreshFailed]=useState(false);
 const [shareSelection,setShareSelection]=useState<CloudHistoryShareSelection>(),[shareReset,setShareReset]=useState(0);
 const clearShareSelection=useCallback(()=>{setShareSelection(undefined);setShareReset(value=>value+1);},[]);
 const alive=useRef(true),epoch=useRef(0),controller=useRef<AbortController|undefined>(undefined),activeCaller=useRef(caller),current=useRef<Snapshot|undefined>(undefined),commandBusy=useRef(false);
 const waiting=useRef<((result:WorkBuddyCloudActionResult)=>void)|undefined>(undefined);
 const readFlight=useRef<Promise<ReadResult>|undefined>(undefined),readOwner=useRef<ApiCaller|undefined>(undefined),readQueued=useRef(false);
 activeCaller.current=caller;current.current=snapshot?.caller===caller?snapshot:undefined;
 const action=useWriteAction({scope:'workbuddy-cloud-task:'+scopeKey,storage:toolSessionStorage(),validateRequest:request=>validCloudRecovery(request,scope),validateResponse:(value,request)=>validCloudReceipt(value,request,scope,current.current?.node)});
 const node=current.current?.node,connection=current.current?.connection,denied=!!current.current?.denied;
 const live=(generation?:number)=>alive.current&&activeCaller.current===caller&&(generation===undefined||epoch.current===generation);
 const store=(value:Snapshot)=>{current.current=value;setSnapshot(value);};
 const deny=()=>{
  ++epoch.current;controller.current?.abort();
  clearShareSelection();
  store({caller,reading:false,denied:true,sourceFailed:false,connectionFailed:false});
 };
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;++epoch.current;controller.current?.abort();waiting.current?.({kind:'unknown'});waiting.current=undefined;};},[]);
 useEffect(()=>{if(action.errorStatus&&[401,403,404].includes(action.errorStatus))deny();},[action.errorStatus]);
 // Hook state is committed before an undefined run result is classified. This
 // avoids reading a previous render's status and treating a lost write as safe.
 useEffect(()=>{
  if(action.busy||!waiting.current)return;
  const resolve=waiting.current;waiting.current=undefined;
  resolve(action.errorStatus&&[401,403,404].includes(action.errorStatus)?{kind:'denied'}:action.pending||action.unknown?{kind:'unknown'}:action.conflict?{kind:'conflict'}:{kind:'failed'});
 },[classification,action.busy,action.errorStatus,action.unknown,action.conflict,!!action.pending,action.persistenceBlocked]);

 const performRead=async():Promise<ReadResult>=>{
  if(!validCloudScope(scope))return 'failed';
  const generation=++epoch.current;controller.current?.abort();const abort=new AbortController();controller.current=abort;
  const previous=current.current&&!current.current.denied?current.current:undefined;
  store({caller,node:previous?.node,connection:previous?.connection,reading:true,denied:false,sourceFailed:false,connectionFailed:false});
  const headers={'X-Suma-Expected-User':scope.userID,'X-Suma-Expected-Organization':scope.organizationID};
  try{
   const raw=await caller<unknown>(cloudNodePath(scope),undefined,'GET',headers,abort.signal);
   if(!live(generation))return 'failed';
   const result=await readCloudNode(raw,scope);
   if(!live(generation))return 'failed';
   if(!result)throw new UiError('workbuddy-cloud-task.invalid_source');
   let account:WorkBuddyConnection|undefined,connectionFailed=false;
   try{
    const rawAccount=await caller<unknown>('/workbuddy-connections/'+encodeURIComponent(result.binding.connection_id)+'?project_id='+encodeURIComponent(project),undefined,'GET',headers,abort.signal);
    if(!live(generation))return 'failed';
    account=readCloudConnection(rawAccount,scope,result);
    if(!account)throw new UiError('workbuddy-cloud-task.invalid_connection');
   }catch(error){
    if(!live(generation))return 'failed';
    // Another member's private connection is not needed to read shared task
    // observations. A current session/project refusal still retires all data.
    if(error instanceof ApiError&&error.status===404)account=undefined;
    else if(refused(error))throw error;
    else connectionFailed=true;
   }
   if(!live(generation))return 'failed';
   store({caller,node:result,connection:account,reading:false,denied:false,sourceFailed:false,connectionFailed});
   action.clearConflict();setParentRefreshFailed(false);return 'ready';
  }catch(error){
   if(!live(generation))return 'failed';
   if(refused(error)){deny();return 'denied';}
   store({caller,node:previous?.node,connection:previous?.connection,reading:false,denied:false,sourceFailed:true,connectionFailed:previous?.connectionFailed||false});
   return 'failed';
  }
 };
 // A run revision reuses the selected run's shared snapshot subscription.
 // Readers coalesce; a revision or explicit refresh arriving during a read
 // requests one follow-up snapshot rather than cancelling another caller.
 const read=(freshAfterCurrent=false):Promise<ReadResult>=>{
  if(readFlight.current&&readOwner.current===caller){if(freshAfterCurrent)readQueued.current=true;return readFlight.current;}
  if(readFlight.current)controller.current?.abort();
  readOwner.current=caller;readQueued.current=false;
  const drain=async():Promise<ReadResult>=>{
   let result:ReadResult='failed';
   do{readQueued.current=false;result=await performRead();}while(readQueued.current&&live());
   return result;
  };
  const flight=drain();readFlight.current=flight;
  const finished=()=>{if(readFlight.current===flight){readFlight.current=undefined;readOwner.current=undefined;}};
  void flight.then(finished,finished);return flight;
 };
 useEffect(()=>{if(validCloudScope(scope))void read(true);},[caller,scopeKey,revision]);

 const reload=async()=>{if(await read(true)==='failed')throw new UiError('workbuddy-cloud-task.source_read_failed');};
 const run=async(path:string,body:Record<string,unknown>,method='POST'):Promise<WorkBuddyCloudActionResult>=>{
  if(commandBusy.current||denied||!node||current.current?.reading)return {kind:'failed'};
  commandBusy.current=true;
  try{
   const value=await action.run<unknown>(path,body,method);
   if(!live())return {kind:'unknown'};
   if(value===undefined)return await new Promise<WorkBuddyCloudActionResult>(resolve=>{waiting.current=resolve;setClassification(value=>value+1);});
   // A confirmed operation stays confirmed even if its follow-up read fails.
   // The separate source failure keeps prior facts and disables new writes.
   const state=await read(true);
   if(!live())return {kind:'unknown'};
   if(state==='denied')return {kind:'denied'};
   try{await onChanged?.();}catch{if(live())setParentRefreshFailed(true);}
   return {kind:'confirmed'};
  }finally{commandBusy.current=false;}
 };
 const eligible=!!node&&!!connection&&connection.state==='connected'&&connection.grant_generation===node.binding.grant_generation&&connection.scopes.includes('user.task.invokable')&&connection.scopes.includes('user.task.readable');
 const sourceVersion=JSON.stringify([node?.run_version,node?.node_spec_sha256,node?.request_sha256,node?.allowed_actions,node?.delegation?.id,node?.delegation?.version,node?.submission?.id,node?.submission?.version,connection?.id,connection?.version,connection?.state,connection?.grant_generation]);
 const writesAllowed=!!node&&!denied&&!current.current?.reading&&!current.current?.sourceFailed&&!action.busy&&!action.pending&&!action.persistenceBlocked;
 const delegate=async(draft:WorkBuddyCloudDelegateDraft):Promise<WorkBuddyCloudActionResult>=>{
  if(!node||!connection||!writesAllowed||!eligible||!node.allowed_actions.includes('delegate')||node.run_version>=2147483647||connection.version>=2147483647)return {kind:'failed',reason:'permission_changed'};
  if(draft.sourceVersion!==sourceVersion||draft.connectionID!==node.binding.connection_id||draft.connectionID!==connection.id||draft.connectionVersion!==connection.version||draft.grantGeneration!==node.binding.grant_generation)return {kind:'conflict'};
  return run('/workbuddy-cloud-delegations',{...cloudDelegateBody(node,connection,draft.expiresAt)});
 };
 const revoke=async():Promise<WorkBuddyCloudActionResult>=>{
  if(!node?.delegation||!writesAllowed||!node.allowed_actions.includes('revoke'))return {kind:'failed',reason:'permission_changed'};
  return run('/workbuddy-cloud-delegations/'+encodeURIComponent(node.delegation.id)+'/revoke',{project_id:project,expected_version:node.delegation.version});
 };
 const observe=async():Promise<WorkBuddyCloudActionResult>=>{
  if(!node?.submission||!writesAllowed||!eligible||!node.allowed_actions.includes('observe'))return {kind:'failed',reason:'permission_changed'};
  return run('/workbuddy-cloud-submissions/'+encodeURIComponent(node.submission.id)+'/observe',{project_id:project,expected_version:node.submission.version});
 };
 const pending=action.pending,mayRecover=!!pending&&!!node&&!denied&&!current.current?.reading&&!current.current?.sourceFailed&&validCloudRecovery(pending,scope,node);
 const reconcile=async():Promise<WorkBuddyCloudActionResult>=>{
  const original=action.pending;
  if(!original||!mayRecover||!validCloudRecovery(original,scope,current.current?.node))return {kind:'unknown'};
  return run(original.path,original.body,original.method);
 };
 const model:WorkBuddyCloudTaskModel={
  scopeKey,sourceVersion,state:!validCloudScope(scope)?'unavailable':denied?'denied':!node?(current.current?.reading||!current.current?'loading':'unavailable'):'ready',unavailableReason:validCloudScope(scope)?'source_missing':'permission_changed',
  execution:execution?.runVersion===node?.run_version?execution:undefined,
  task:{title:taskTitle||node?.binding.request.name||t('source_task'),runLabel:t('source_run',{id:runID}),nodeLabel:t('source_node',{id:nodeID}),prompt:node?.binding.request.prompt||'',name:node?.binding.request.name,requestDigest:node?.request_sha256||'',nodeDigest:node?.node_spec_sha256||''},
  connections:connection?[{id:connection.id,label:connection.open_id||connection.id,version:connection.version,grantGeneration:connection.grant_generation,eligible:eligible&&!!node?.allowed_actions.includes('delegate'),unavailableReason:connection.grant_generation!==node?.binding.grant_generation?'node_mismatch':'connection_unavailable'}]:[],
  actions:{delegate:writesAllowed&&eligible&&!!node?.allowed_actions.includes('delegate')&&node.run_version<2147483647&&connection!.version<2147483647,revoke:writesAllowed&&!!node?.allowed_actions.includes('revoke'),refresh:writesAllowed&&eligible&&!!node?.allowed_actions.includes('observe')},
  delegation:node?.delegation?{id:node.delegation.id,version:node.delegation.version,state:node.delegation.state as 'active'|'revoked',expiresAt:node.delegation.expires_at,connectionLabel:connection?.open_id||t('fixed_connection',{id:node.delegation.connection_id}),createsConsumed:node.delegation.creates_consumed as 0|1}:undefined,
  submission:node?.submission?{id:node.submission.id,version:node.submission.version,state:node.submission.state as 'prepared'|'unknown'|'task_known',taskID:node.submission.task?.task_id,providerStatus:node.submission.task?.status,observedAt:node.submission.observed_at,lastError:node.submission.last_error}:undefined,
  recovery:pending?{action:cloudRecoveryAction(pending)||'delegate',state:'unknown',canReconcile:mayRecover}:undefined,
 };
 return <div className="workbuddy-cloud-workspace">
  {!denied&&current.current?.reading&&node&&<p className="cloud-task-hint" role="status">{t('reading_current')}</p>}
  {!denied&&(current.current?.sourceFailed||parentRefreshFailed)&&node&&<p className="cloud-task-inline-error" role="alert">{t('source_read_failed')}</p>}
  {!denied&&current.current?.connectionFailed&&node&&<p className="cloud-task-hint" role="status">{t('connection_read_failed')}</p>}
  {!denied&&<WriteNotice action={action}/>}
  <WorkBuddyCloudTaskPanel model={model} onDelegate={delegate} onRevoke={revoke} onRefresh={observe} onReload={reload} onReconcile={reconcile}/>
  {!denied&&node?.submission?.state==='task_known'&&node.submission.task&&<WorkBuddyCloudCollaboration source={{project_id:project,run_id:runID,node_id:nodeID,submission_id:node.submission.id,submission_version:node.submission.version,task_id:node.submission.task.task_id,node_spec_sha256:node.node_spec_sha256,request_sha256:node.request_sha256}} revision={revision} disabled={!!current.current?.reading||!!current.current?.sourceFailed} onShareSelection={setShareSelection} shareReset={shareReset}/>}
  <CloudHistorySharing project={project} runID={runID} nodeID={nodeID} selection={shareSelection} onClearSelection={clearShareSelection}/>
 </div>;
}
