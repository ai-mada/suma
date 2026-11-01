import {useCallback,useEffect,useLayoutEffect,useRef,useState} from 'react';
import type {SteeringContext,SteeringInstruction} from '../../../packages/contracts/src';
import {ApiError,operation,type Session} from './api';
import {useApi,useApiView} from './api-context';
import {tr,UiError,useLocale} from './i18n';
import {toolSessionStorage} from './tool-hooks';
import {useWriteAction,WriteNotice} from './write-action';
import {TurnSteeringCard} from './TurnSteeringCard';
import {buildTurnSteeringIntent,canSendTurnSteering,readTurnSteeringContext,turnSteeringContextPath,turnSteeringRecoveryScope,validTurnSteeringIntent,validTurnSteeringReceipt,validTurnSteeringRecovery,type TurnSteeringIntent,type TurnSteeringScope} from './turn-steering-model';
import './TurnSteeringPanel.css';

export type TurnSteeringPanelProps={session:Session;project:string;taskID:string;selectedMessage:string;refresh:number;accessBlocked:boolean;disabled?:boolean;onLockChange?:(locked:boolean)=>void;onAccessDenied?:()=>void};
type Snapshot={messageID:string;context?:SteeringContext;loading:boolean;stale?:boolean;error?:unknown;denied?:boolean};
type IntentState={intent?:TurnSteeringIntent;error?:unknown};
const t=(key:string)=>tr('turn-steering.'+key);
const canonical=(value:unknown):string=>JSON.stringify(value,(_key,item)=>item&&typeof item==='object'&&!Array.isArray(item)?Object.fromEntries(Object.keys(item).sort().map(key=>[key,item[key]])):item);
const refused=(error:unknown)=>error instanceof ApiError&&[401,403,404].includes(error.status);
const basis=(context:SteeringContext)=>canonical([context.task_id,context.task_version,context.message,context.can_send,context.targets]);
const sameMessage=(context:SteeringContext,original:TurnSteeringIntent)=>context.message.id===original.message.id&&context.message.sequence===original.message.sequence&&context.message.author_id===original.message.author_id&&context.message.body_sha256===original.message.body_sha256;
function readIntent(storage:ReturnType<typeof toolSessionStorage>,key:string,scope:TurnSteeringScope):IntentState{
 try{if(!storage)throw new Error();const raw=storage.getItem(key);if(!raw)return {};if(new TextEncoder().encode(raw).length>65536)throw new Error();const value:unknown=JSON.parse(raw);if(!validTurnSteeringIntent(value,scope))throw new Error();return {intent:value};}
 catch{return {error:new UiError('turn-steering.intent_unavailable')};}
}

/** The saved human message and original live execution remain server-owned.
 * This controller never constructs native parameters or restarts a turn. */
export function TurnSteeringPanel(props:TurnSteeringPanelProps){
 const api=useApi(),view=useApiView(),owner=useRef({api,epoch:0});
 if(owner.current.api!==api)owner.current={api,epoch:owner.current.epoch+1};
 if(!view?.userID||!view.organizationID||view.userID!==props.session.user.id||view.organizationID!==props.session.organization.id)return null;
 const scope:TurnSteeringScope={project:props.project,task:props.taskID,userID:view.userID,organizationID:view.organizationID};
 return <SteeringController key={JSON.stringify([turnSteeringRecoveryScope(scope),owner.current.epoch])} {...props} scope={scope}/>;
}

function SteeringController({scope,selectedMessage,refresh,accessBlocked,disabled=false,onLockChange,onAccessDenied}:TurnSteeringPanelProps&{scope:TurnSteeringScope}){
 useLocale();const api=useApi(),storage=toolSessionStorage(),recoveryScope=turnSteeringRecoveryScope(scope),intentKey='suma-turn-steering-intent:'+recoveryScope;
 const [intentState,setIntentState]=useState<IntentState>(()=>readIntent(storage,intentKey,scope)),intent=useRef(intentState.intent);
 const chosenMessage=useRef(selectedMessage),parent=useRef({accessBlocked,disabled});chosenMessage.current=selectedMessage;parent.current={accessBlocked,disabled};
 const messageID=intentState.intent?.message.id||selectedMessage;
 const [snapshot,setSnapshot]=useState<Snapshot>({messageID,loading:!!messageID&&!accessBlocked}),current=useRef(snapshot);
 const [selection,setSelection]=useState<{basis:string;id:string}>(),[backgroundPaused,setBackgroundPaused]=useState(false);
 const alive=useRef(true),owner=useRef(api),generation=useRef(0),controller=useRef<AbortController|undefined>(undefined),flight=useRef<{messageID:string;promise:Promise<boolean>}|undefined>(undefined),commandBusy=useRef(false),polls=useRef(0),denialReported=useRef(false);
 const callbacks=useRef({onLockChange,onAccessDenied});callbacks.current={onLockChange,onAccessDenied};owner.current=api;
 const settled=useRef<{intent:TurnSteeringIntent;value:SteeringInstruction}|undefined>(undefined);
 const setIntent=(next:IntentState)=>{intent.current=next.intent;setIntentState(next);};
 const store=(next:Snapshot)=>{current.current=next;setSnapshot(next);};
 const action=useWriteAction({scope:'turn-steering:'+recoveryScope,storage,validateRequest:request=>validTurnSteeringRecovery(request,scope),
  validateResponse:(raw,request)=>!!intent.current&&validTurnSteeringReceipt(raw,request,scope,intent.current)});
 const writeDenied=!!action.errorStatus&&[401,403,404].includes(action.errorStatus);
 const visible:Snapshot=!accessBlocked&&snapshot.messageID===messageID?snapshot:{messageID,loading:!!messageID&&!accessBlocked,denied:accessBlocked};
 const context=visible.denied||writeDenied?undefined:visible.context;
 const deny=useCallback(()=>{
  ++generation.current;controller.current?.abort();flight.current=undefined;const value={messageID:intent.current?.message.id||chosenMessage.current,loading:false,denied:true};current.current=value;setSnapshot(value);setSelection(undefined);
  if(!denialReported.current){denialReported.current=true;callbacks.current.onAccessDenied?.();}
 },[]);
 useLayoutEffect(()=>{if(writeDenied)deny();},[writeDenied,deny]);
 useLayoutEffect(()=>{if(accessBlocked){++generation.current;controller.current?.abort();flight.current=undefined;store({messageID,loading:false,denied:true});setSelection(undefined);}},[accessBlocked]);
 useLayoutEffect(()=>{
  const original=intent.current;if(!original||!action.conflict||action.errorStatus!==409||action.pending||!alive.current||owner.current!==api)return;
  try{if(!storage)throw new Error();const raw=storage.getItem(intentKey);if(raw!==null&&raw!==JSON.stringify(original))throw new Error();storage.removeItem(intentKey);setIntent({});}
  catch{setIntent({intent:original,error:new UiError('turn-steering.intent_unavailable')});}
 },[api,action.conflict,action.errorStatus,!!action.pending,intentKey]);
 const read=useCallback((background=false):Promise<boolean>=>{
  const source=intent.current?.message.id||chosenMessage.current;
  if(parent.current.accessBlocked)return Promise.resolve(false);
  if(!source){store({messageID:'',loading:false});return Promise.resolve(true);}
  if(flight.current?.messageID===source){if(!background)store({...current.current,loading:true});return flight.current.promise;}
  const ticket=++generation.current,abort=new AbortController();controller.current?.abort();controller.current=abort;
  const previous=current.current.messageID===source&&!current.current.denied?current.current.context:undefined;
  store({messageID:source,context:previous,loading:true,stale:!!previous});
  const owns=()=>alive.current&&owner.current===api&&!abort.signal.aborted&&generation.current===ticket&&!parent.current.accessBlocked&&(intent.current?.message.id||chosenMessage.current)===source;
  const job=(async()=>{
   try{
    const raw=await api<unknown>(turnSteeringContextPath(scope,source),undefined,'GET',{'X-Suma-Expected-User':scope.userID,'X-Suma-Expected-Organization':scope.organizationID},abort.signal);
    if(!owns())return false;const next=readTurnSteeringContext(raw,scope,source),original=intent.current;
    if(original&&!sameMessage(next,original))throw new UiError('turn-steering.intent_unavailable');
    denialReported.current=false;store({messageID:source,context:next,loading:false});return true;
   }catch(error){if(!owns())return false;if(refused(error))deny();else store({messageID:source,context:previous,loading:false,stale:!!previous,error});return false;}
  })();flight.current={messageID:source,promise:job};void job.finally(()=>{if(flight.current?.promise===job)flight.current=undefined;});return job;
 },[api,recoveryScope,deny]);
 const readNow=useRef(read);readNow.current=read;
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;++generation.current;controller.current?.abort();flight.current=undefined;callbacks.current.onLockChange?.(false);};},[]);
 useEffect(()=>{void read();},[read,messageID,refresh,accessBlocked]);
 useEffect(()=>{polls.current=0;setBackgroundPaused(false);},[messageID]);
 const savedIntent=intentState.intent,hasRecovery=!!savedIntent||!!action.pending,requestMatches=!action.pending||!!savedIntent&&canonical(action.pending)===canonical(savedIntent.request);
 const intentUnavailable=!!intentState.error||!!action.pending&&(!savedIntent||!requestMatches);
 const ownLocked=action.busy||hasRecovery||intentUnavailable||action.persistenceBlocked;
 useEffect(()=>{callbacks.current.onLockChange?.(ownLocked);},[ownLocked]);
 useEffect(()=>{
  if(disabled||accessBlocked||visible.denied||visible.error||backgroundPaused||!context?.targets.some(target=>target.state==='active'))return;
  const timer=window.setInterval(()=>{if(document.visibilityState==='hidden'||flight.current||commandBusy.current)return;if(polls.current>=120){setBackgroundPaused(true);return;}++polls.current;void readNow.current(true);},2000);
  return()=>window.clearInterval(timer);
 },[disabled,accessBlocked,visible.denied,!!visible.error,backgroundPaused,!!context?.targets.some(target=>target.state==='active')]);
 const currentBasis=context?basis(context):'',selectedSessionID=selection?.basis===currentBasis?selection.id:'';
 const readBlocked=accessBlocked||!!visible.denied||writeDenied||visible.loading||!!visible.error||!!visible.stale;
 const freshBlocked=disabled||readBlocked||ownLocked||action.conflict;
 const restoreReady=!!savedIntent&&!!context&&sameMessage(context,savedIntent)&&!readBlocked&&!intentUnavailable&&requestMatches;
 const gate=useRef({context,basis:currentBasis,selectedSessionID,freshBlocked,restoreReady,readBlocked,disabled});gate.current={context,basis:currentBasis,selectedSessionID,freshBlocked,restoreReady,readBlocked,disabled};
 async function refreshCurrent(){
  polls.current=0;setBackgroundPaused(false);const original=intent.current,restored=readIntent(storage,intentKey,scope);
  if(original&&(!restored.intent||canonical(restored.intent)!==canonical(original)))setIntent({intent:original,error:new UiError('turn-steering.intent_unavailable')});else setIntent(restored);
  const ready=await readNow.current();if(ready&&writeDenied)action.setError('');return ready;
 }
 function select(id:string){const now=gate.current;if(!alive.current||owner.current!==api||now.freshBlocked||!now.context)return;const target=now.context.targets.find(item=>item.id===id);if(target&&canSendTurnSteering(now.context,target,scope))setSelection({basis:now.basis,id});}
 async function send(id?:string){
  const now=gate.current;if(!alive.current||owner.current!==api||commandBusy.current||now.readBlocked)return;
  let original=intent.current;
  if(original&&!now.restoreReady)return;
  if(!original){
   if(now.freshBlocked||!now.context||!id||id!==now.selectedSessionID||current.current.context!==now.context)return;
   const target=now.context.targets.find(item=>item.id===id);if(!target||!canSendTurnSteering(now.context,target,scope))return;
   const key=operation();original=buildTurnSteeringIntent(now.context,target,scope,{path:'/task-execution-steering',method:'POST',idempotency_key:key,body:{project_id:scope.project,task_id:scope.task,message_id:now.context.message.id,expected_task_version:now.context.task_version,session_id:target.id,expected_session_version:target.version,idempotency_key:key}});
   if(!original)return;
   try{if(!storage)throw new Error();storage.setItem(intentKey,JSON.stringify(original));setIntent({intent:original});}
   catch{setIntent({error:new UiError('turn-steering.intent_unavailable')});action.setError(new UiError('turn-steering.intent_unavailable'));return;}
  }
  commandBusy.current=true;const caller=api;
  try{
   const value=settled.current&&canonical(settled.current.intent)===canonical(original)?settled.current.value:await action.run<SteeringInstruction>(original.request.path,original.request.body,original.request.method);
   if(!value||!alive.current||owner.current!==caller||intent.current!==original||parent.current.accessBlocked)return;
   settled.current={intent:original,value};
   try{if(!storage)throw new Error();storage.removeItem(intentKey);setIntent({});}
   catch{setIntent({intent:original,error:new UiError('turn-steering.intent_unavailable')});return;}
   settled.current=undefined;setSelection(undefined);++generation.current;controller.current?.abort();flight.current=undefined;await readNow.current();
  }finally{commandBusy.current=false;}
 }
 if(!messageID&&!hasRecovery&&!intentUnavailable&&!action.error)return null;
 const denied=accessBlocked||visible.denied||writeDenied,error=denied?new UiError('turn-steering.access_denied'):intentUnavailable?new UiError('turn-steering.intent_unavailable'):visible.error;
 return <section className="turn-steering-panel" data-turn-steering-panel aria-label={t('title')}>
  <WriteNotice action={action} onRebase={async()=>{if(!await refreshCurrent())throw new UiError('turn-steering.refresh_failed');}}/>
  {backgroundPaused&&!denied&&<p className="turn-steering-panel-note" role="status">{t('background_paused')}</p>}
  <TurnSteeringCard context={denied?undefined:context} selectedSessionID={selectedSessionID} loading={!denied&&visible.loading} stale={visible.stale} error={error}
   disabled={disabled||action.busy||intentUnavailable||!!denied} readOnly={hasRecovery||action.conflict||action.persistenceBlocked||!context?.can_send} pending={!denied&&hasRecovery}
   onSelect={select} onSend={id=>void send(id)} onRefresh={accessBlocked||action.busy?undefined:()=>void refreshCurrent()} onRestore={!denied&&!action.busy&&restoreReady?()=>void send():undefined}/>
 </section>;
}
