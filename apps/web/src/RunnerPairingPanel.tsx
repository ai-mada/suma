import {useEffect,useLayoutEffect,useRef,useState} from 'react';
import type {RunnerPairing} from '../../../packages/contracts/src';
import {ApiError} from './api';
import {useApi,useApiView} from './api-context';
import {tr,UiError} from './i18n';
import {toolSessionStorage} from './tool-hooks';
import {useWriteAction,WriteNotice} from './write-action';
import {RunnerPairingCard} from './RunnerPairingCard';
import {RunnerPairingInbox} from './RunnerPairingInbox';
import {readRunnerPairing,validRunnerPairingReceipt,validRunnerPairingRequest} from './runner-pairing-model';

export type RunnerPairingPanelProps={project:string;disabled?:boolean;onLockChange:(locked:boolean)=>void;onPaired:(pairing:RunnerPairing)=>void};
type Read={caller:ReturnType<typeof useApi>;path:string;loading:boolean;value?:RunnerPairing;error?:unknown};
const locatorID=(value:string)=>/^[a-f0-9]{32}$/.test(value);
const refused=(value:unknown)=>value instanceof ApiError&&[401,403,404].includes(value.status);

/** Only public locators and typed administrator intent enter browser storage.
 * The machine retains the credential and its original proposal independently. */
export function RunnerPairingPanel(props:RunnerPairingPanelProps){
 const api=useApi(),view=useApiView(),owner=useRef({api,epoch:0});
 if(owner.current.api!==api)owner.current={api,epoch:owner.current.epoch+1};
 return <PairingScope key={JSON.stringify([view?.userID,view?.organizationID,props.project,owner.current.epoch])} {...props} userID={view?.userID||''} organizationID={view?.organizationID||''}/>;
}
function PairingScope({project,disabled=false,onLockChange,onPaired,userID,organizationID}:RunnerPairingPanelProps&{userID:string;organizationID:string}){
 const api=useApi(),storage=toolSessionStorage(),scope=JSON.stringify([userID,organizationID,project]),locatorKey='suma-runner-pairing:'+scope;
 const [selected,setSelected]=useState(()=>{try{return storage?.getItem(locatorKey)||'';}catch{return 'unreadable-locator';}});
 const [revision,setRevision]=useState(0),[listRevision,setListRevision]=useState(0),[read,setRead]=useState<Read>(),[created,setCreated]=useState<RunnerPairing>(),[localError,setLocalError]=useState<unknown>(),[denied,setDenied]=useState(false);
 const callback=useRef({onLockChange,onPaired});callback.current={onLockChange,onPaired};
 const alive=useRef(false),caller=useRef(api);caller.current=api;
 const path=locatorID(selected)?'/runner-pairings/'+encodeURIComponent(selected)+'?project_id='+encodeURIComponent(project):'';
 const intent=useRef({api,path});intent.current={api,path};
 const action=useWriteAction({scope:'runner-pairing:'+scope,storage,
  validateRequest:request=>validRunnerPairingRequest(request,project),
  validateResponse:(value,request)=>{
   if(!validRunnerPairingReceipt(value,request,project))return false;
   if(request.path==='/runner-pairings'){
    const pairing=readRunnerPairing(value,project);
    // Save the public locator before the generic writer clears its original
    // request. Failed storage keeps the same body/key recoverable.
    try{if(!storage)throw new Error();if(storage.getItem(locatorKey)!==pairing.id)storage.setItem(locatorKey,pairing.id);}
    catch{setLocalError(new UiError('runner-pairing.locator_failed'));return false;}
   }
   return true;
  }});
 const locked=action.busy||!!action.pending,invalidLocator=!!selected&&!locatorID(selected),hasView=!!userID&&!!organizationID;
 const writeRefused=[401,403,404].includes(action.errorStatus||0);
 useEffect(()=>{if(writeRefused){setDenied(true);setCreated(undefined);setRead(undefined);}},[writeRefused]);
 useLayoutEffect(()=>{callback.current.onLockChange(locked);});
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;callback.current.onLockChange(false);};},[]);
 useEffect(()=>{
  if(!path||!hasView||denied||disabled)return;
  const controller=new AbortController();let latest=true;
  setRead(before=>({caller:api,path,loading:true,...(before?.caller===api&&before.path===path?{value:before.value}:{})}));
  void api<unknown>(path,undefined,'GET',{'X-Suma-Expected-User':userID,'X-Suma-Expected-Organization':organizationID},controller.signal).then(value=>{
   if(!latest||!alive.current||caller.current!==api||intent.current.path!==path)return;
   const pairing=readRunnerPairing(value,project);if(pairing.id!==selected)throw new UiError('runner-pairing.invalid_read');
   setRead({caller:api,path,loading:false,value:pairing});
  }).catch(error=>{
   if(!latest||controller.signal.aborted||!alive.current||caller.current!==api||intent.current.path!==path)return;
   setRead(before=>({caller:api,path,loading:false,error,...(!refused(error)&&before?.caller===api&&before.path===path?{value:before.value}:{})}));
   if(refused(error)){setCreated(undefined);setDenied(true);}
  });
  return()=>{latest=false;controller.abort();};
 },[api,path,revision,hasView,denied,disabled,project,selected,userID,organizationID]);
 const current=read?.caller===api&&read.path===path?read:undefined;
 const pairing=denied||writeRefused?undefined:current?.value||(created?.id===selected?created:undefined);
 const loading=!!path&&!denied&&!disabled&&(!current||current.loading),stale=!!selected&&(!current?.value||current.loading||!!current.error);
 const error=!hasView?new UiError('runner-pairing.view_required'):invalidLocator?new UiError('runner-pairing.invalid_locator'):localError||action.error||current?.error;
 const viewBlocked=disabled||!hasView||denied||writeRefused||invalidLocator;
 const blocked=viewBlocked||action.persistenceBlocked;
 function refresh(){if(blocked||locked)return;if(action.error)action.clearConflict();setLocalError(undefined);setListRevision(value=>value+1);if(!path)return;setRead(before=>before?.caller===api&&before.path===path?{...before,loading:true,error:undefined}:undefined);setRevision(value=>value+1);}
 function remember(value:RunnerPairing){setCreated(value);setSelected(value.id);setLocalError(undefined);setRead({caller:api,path:'/runner-pairings/'+encodeURIComponent(value.id)+'?project_id='+encodeURIComponent(project),loading:true,value});setRevision(number=>number+1);setListRevision(number=>number+1);}
 function selectExisting(id:string){
  if(blocked||locked||!locatorID(id))return;
  try{if(!storage)throw new Error();if(storage.getItem(locatorKey)!==id)storage.setItem(locatorKey,id);}catch{setLocalError(new UiError('runner-pairing.locator_failed'));return;}
  intent.current={api,path:'/runner-pairings/'+encodeURIComponent(id)+'?project_id='+encodeURIComponent(project)};
  action.clearConflict();setLocalError(undefined);setCreated(undefined);setRead(undefined);setSelected(id);setRevision(number=>number+1);
 }
 function listDenied(){setDenied(true);setCreated(undefined);setRead(undefined);setLocalError(new UiError('runner-pairing-inbox.access_changed'));}
 async function run(path:string,body:Record<string,unknown>,method='POST'){
  const owner=api;callback.current.onLockChange(true);
  const value=await action.run<unknown>(path,body,method);
  if(value&&alive.current&&caller.current===owner)remember(readRunnerPairing(value,project));
 }
 async function create(){
  if(blocked||locked||action.conflict||action.persistenceBlocked||selected&&(!current?.value||stale))return;
  await run('/runner-pairings',{project_id:project});
 }
 async function command(actionName:'approve'|'reject'){
  const value=current?.value;
  if(blocked||locked||stale||!value||loading||!(actionName==='approve'?value.state==='proposed'&&value.can_approve:value.state==='waiting'||value.state==='proposed'))return;
  await run('/runner-pairings/'+encodeURIComponent(value.id)+'/commands',{project_id:project,expected_version:value.version,action:actionName});
 }
 async function restore(){
  if(viewBlocked||action.busy||!action.pending)return;
  const original=action.pending;await run(original.path,original.body,original.method);
 }
 function proceed(){const value=current?.value;if(!blocked&&!locked&&!stale&&!loading&&value?.state==='approved'&&value.worker_id&&alive.current&&intent.current.api===api)callback.current.onPaired(value);}
 return <><RunnerPairingInbox project={project} disabled={blocked||locked} accessBlocked={viewBlocked} selectedID={selected} refreshToken={listRevision} onSelect={selectExisting} onDenied={listDenied}/>{pairing&&['waiting','proposed','approved'].includes(pairing.state)&&<div className="runner-pairing-actions"><button type="button" data-runner-pairing-new disabled={blocked||locked||stale||loading||!!error} onClick={()=>void create()}>{tr('runner-pairing.create_another')}</button></div>}<WriteNotice action={{...action,error:''}} onRebase={async()=>{refresh();}}/><RunnerPairingCard pairing={pairing} loading={loading} stale={stale} disabled={action.busy||(action.pending?viewBlocked:blocked)} error={error} pending={!!action.pending} onCreate={()=>void create()} onRefresh={refresh} onApprove={()=>void command('approve')} onReject={()=>void command('reject')} onRestore={()=>void restore()} onContinue={proceed}/></>;
}
