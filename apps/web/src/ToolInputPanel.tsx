import {useCallback,useEffect,useId,useLayoutEffect,useRef,useState} from 'react';
import {ChevronDown,MessageSquare,RefreshCw} from 'lucide-react';
import type {ToolInput} from '../../../packages/contracts/src';
import {ApiError,operation} from './api';
import {useApi,useApiView} from './api-context';
import {formatNumber,tr,UiError,useLocale} from './i18n';
import {ErrorNotice} from './ui';
import {toolSessionStorage} from './tool-hooks';
import {useWriteAction,WriteNotice} from './write-action';
import {ToolInputCard} from './ToolInputCard';
import {buildToolInputAnswerBody,buildToolInputIntent,canAnswerToolInput,readToolInputList,toolInputListPath,toolInputMatchesIntent,toolInputRecoveryScope,validToolInputIntent,validToolInputReceipt,validToolInputRecovery,type ToolInputIntent,type ToolInputScope,type ToolInputSubmission} from './tool-input-model';
import './ToolInputPanel.css';

export type ToolInputPanelProps={project:string;runID:string;nodeID:string;attempt:number;fence:number;protocol:'codex-app-server-v1'|'acp-stdio-v1'|'claude-sdk-v1';state?:string;revision?:number;readOnly?:boolean;onChanged?:()=>void|Promise<void>};
type Snapshot={key:string;protocol:string;items:ToolInput[];loading:boolean;loaded?:boolean;error?:unknown;denied?:boolean};
type IntentState={intent?:ToolInputIntent;error?:unknown};
const t=(key:string,values?:Record<string,unknown>)=>tr('tool-inputs.'+key,values);
const activeStates=new Set(['ready','running','waiting','reconciling']);
const refused=(error:unknown)=>error instanceof ApiError&&[401,403,404].includes(error.status);
const canonical=(value:unknown):string=>JSON.stringify(value,(_key,item)=>item&&typeof item==='object'&&!Array.isArray(item)?Object.fromEntries(Object.keys(item).sort().map(key=>[key,item[key]])):item);
const readKey=(scope:ToolInputScope)=>JSON.stringify([toolInputRecoveryScope(scope),scope.attempt,scope.fence]);
function readIntent(storage:ReturnType<typeof toolSessionStorage>,key:string,scope:ToolInputScope):IntentState{
 try{
  if(!storage)throw new Error();const raw=storage.getItem(key);if(!raw)return {};
  if(new TextEncoder().encode(raw).length>1024*1024)throw new Error();const value:unknown=JSON.parse(raw);
  if(!validToolInputIntent(value,scope))throw new Error();return {intent:value};
 }catch{return {error:new UiError('tool-inputs.intent_unavailable')};}
}

/** One current-node reader owns all questions. Answer persistence, native
 * delivery and original-execution closure remain separate observed facts. */
export function ToolInputPanel(props:ToolInputPanelProps){
 const api=useApi(),view=useApiView(),owner=useRef({api,epoch:0});
 if(owner.current.api!==api)owner.current={api,epoch:owner.current.epoch+1};
 if(!view?.userID||!view.organizationID)return null;
 const scope:ToolInputScope={project:props.project,runID:props.runID,nodeID:props.nodeID,attempt:props.attempt,fence:props.fence,protocol:props.protocol,userID:view?.userID||'',organizationID:view?.organizationID||''};
 return <InputController key={JSON.stringify([toolInputRecoveryScope(scope),owner.current.epoch])} {...props} scope={scope}/>;
}

function InputController({scope,state,revision,readOnly=false,onChanged}:ToolInputPanelProps&{scope:ToolInputScope}){
 useLocale();const api=useApi(),panelID=useId(),storage=toolSessionStorage(),recoveryScope=toolInputRecoveryScope(scope),intentKey='suma-tool-input-intent:'+recoveryScope;
 const [intentState,setIntentState]=useState<IntentState>(()=>readIntent(storage,intentKey,scope)),intent=useRef(intentState.intent);
 const liveScope=useRef(scope);liveScope.current=scope;
 const effectiveScope=intentState.intent?.scope||scope,effectiveKey=readKey(effectiveScope);
 const [snapshot,setSnapshot]=useState<Snapshot>({key:effectiveKey,protocol:'',items:[],loading:!!scope.userID&&!!scope.organizationID,denied:!scope.userID||!scope.organizationID});
 const current=useRef(snapshot),alive=useRef(true),owner=useRef(api),generation=useRef(0),controller=useRef<AbortController|undefined>(undefined),flight=useRef<{key:string;promise:Promise<boolean>}|undefined>(undefined);
 const [open,setOpen]=useState(false),[backgroundPaused,setBackgroundPaused]=useState(false),polls=useRef(0),commandBusy=useRef(false),notify=useRef(onChanged);
 const toggleRef=useRef<HTMLButtonElement>(null),lastFocus=useRef<HTMLElement|null>(null),retired=useRef<ToolInputIntent|undefined>(undefined),settled=useRef<{intent:ToolInputIntent;value:ToolInput}|undefined>(undefined);
 owner.current=api;notify.current=onChanged;
 const setIntent=(value:IntentState)=>{intent.current=value.intent;setIntentState(value);};
 const visible=snapshot.key===effectiveKey?snapshot:{key:effectiveKey,protocol:'',items:[],loading:true};
 const action=useWriteAction({scope:'tool-inputs:'+recoveryScope,storage,validateRequest:request=>validToolInputRecovery(request,scope),
  validateResponse:(raw,request)=>!!intent.current&&validToolInputReceipt(raw,request,intent.current.scope,intent.current)});
 const writeDenied=!!action.errorStatus&&[401,403,404].includes(action.errorStatus);
 const store=(value:Snapshot)=>{current.current=value;setSnapshot(value);};
 const deny=useCallback(()=>{++generation.current;controller.current?.abort();flight.current=undefined;const value={key:readKey(intent.current?.scope||liveScope.current),protocol:'',items:[],loading:false,denied:true};current.current=value;setSnapshot(value);},[]);
 useLayoutEffect(()=>{if(writeDenied)deny();},[writeDenied,deny]);
 useLayoutEffect(()=>{const focused=lastFocus.current;if(focused&&!focused.isConnected&&document.activeElement===document.body){toggleRef.current?.focus();lastFocus.current=null;}});
 // Only a definite CAS rejection whose write journal has cleared can retire
 // metadata. Unknown responses retain both original journals and their key.
 useLayoutEffect(()=>{
  const original=intent.current;
  if(!alive.current||owner.current!==api||!action.conflict||action.errorStatus!==409||action.pending||!original)return;
  try{if(!storage)throw new Error();const raw=storage.getItem(intentKey);if(raw!==null&&raw!==JSON.stringify(original))throw new Error();storage.removeItem(intentKey);retired.current=original;setIntent({});}
  catch{setIntent({intent:original,error:new UiError('tool-inputs.intent_unavailable')});}
 },[api,action.conflict,action.errorStatus,!!action.pending,intentKey]);
 const read=useCallback((background=false):Promise<boolean>=>{
  const target=intent.current?.scope||liveScope.current,key=readKey(target);
  if(!target.userID||!target.organizationID)return Promise.resolve(false);
  if(flight.current?.key===key){if(!background)store({...current.current,loading:true});return flight.current.promise;}
  const epoch=++generation.current,abort=new AbortController();controller.current?.abort();controller.current=abort;
  const previous=current.current.key===key&&!current.current.denied?current.current:undefined;
  store({key,protocol:previous?.protocol||'',items:previous?.items||[],loading:!background||!previous?.loaded,loaded:previous?.loaded});
  const owns=()=>alive.current&&owner.current===api&&!abort.signal.aborted&&generation.current===epoch&&readKey(intent.current?.scope||liveScope.current)===key;
  const job=(async()=>{
   try{
    const raw=await api<unknown>(toolInputListPath(target),undefined,'GET',{'X-Suma-Expected-User':target.userID,'X-Suma-Expected-Organization':target.organizationID},abort.signal);
    if(!owns())return false;const page=readToolInputList(raw,target),original=intent.current;
    if(original&&!page.items.some(item=>toolInputMatchesIntent(item,original)))throw new UiError('tool-inputs.intent_unavailable');
    store({key,protocol:page.protocol,items:page.items,loading:false,loaded:true});return true;
   }catch(error){if(!owns())return false;if(refused(error))deny();else store({key,protocol:previous?.protocol||'',items:previous?.items||[],loading:false,loaded:previous?.loaded,error});return false;}
  })();
  flight.current={key,promise:job};void job.finally(()=>{if(flight.current?.promise===job)flight.current=undefined;});return job;
 },[api,recoveryScope,deny]);
 const readNow=useRef(read);readNow.current=read;
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;++generation.current;controller.current?.abort();flight.current=undefined;};},[]);
 useEffect(()=>{void read();},[read,revision,effectiveKey]);
 const historical=readOnly||!!state&&!activeStates.has(state)||effectiveScope.attempt!==scope.attempt||effectiveScope.fence!==scope.fence,capable=visible.protocol===({'codex-app-server-v1':'codex-user-input-v1','acp-stdio-v1':'cursor-acp-input-v1','claude-sdk-v1':'claude-sdk-input-v1'} as const)[effectiveScope.protocol];
 useEffect(()=>{
  if(historical||!activeStates.has(state||'')||!capable||visible.denied||visible.error||backgroundPaused)return;
  const timer=window.setInterval(()=>{
   if(document.visibilityState==='hidden'||flight.current||commandBusy.current)return;
   if(polls.current>=120){setBackgroundPaused(true);return;}++polls.current;void readNow.current(true);
  },2000);return()=>window.clearInterval(timer);
 },[historical,state,capable,visible.denied,!!visible.error,backgroundPaused]);
 const savedIntent=intentState.intent,hasRecovery=!!savedIntent||!!action.pending;
 const requestMatches=!action.pending||!!savedIntent&&canonical(action.pending)===canonical(savedIntent.request);
 const intentUnavailable=!!intentState.error||!!action.pending&&(!savedIntent||!requestMatches);
 const locked=historical||!capable||action.busy||hasRecovery||intentUnavailable||action.persistenceBlocked||action.conflict||visible.denied||writeDenied||visible.loading||!!visible.error;
 const recoveryInput=savedIntent?visible.items.find(item=>toolInputMatchesIntent(item,savedIntent)):undefined;
 const commandGate=useRef({locked,intentUnavailable,requestMatches,recoveryInput,denied:!!visible.denied||writeDenied,loading:visible.loading,error:visible.error});
 commandGate.current={locked,intentUnavailable,requestMatches,recoveryInput,denied:!!visible.denied||writeDenied,loading:visible.loading,error:visible.error};
 const pendingCount=capable&&!historical&&effectiveScope.attempt===scope.attempt&&effectiveScope.fence===scope.fence?visible.items.filter(item=>canAnswerToolInput(item)).length:0;
 async function refresh(){
  polls.current=0;setBackgroundPaused(false);const restored=readIntent(storage,intentKey,scope),original=intent.current;
  if(original&&(!restored.intent||canonical(original)!==canonical(restored.intent)))setIntent({intent:original,error:new UiError('tool-inputs.intent_unavailable')});else setIntent(restored);
  const ready=await readNow.current();if(ready&&writeDenied)action.setError('');return ready;
 }
 async function answer(input?:ToolInput,submission?:ToolInputSubmission){
  const gate=commandGate.current;
  if(!alive.current||owner.current!==api||commandBusy.current||gate.denied||gate.intentUnavailable)return;
  let original=intent.current;
  if(original&&(!gate.recoveryInput||gate.loading||gate.error||!gate.requestMatches))return;
  if(!original){
   if(!input||!submission||gate.locked||readKey(scope)!==readKey(liveScope.current)||current.current.key!==readKey(scope)||!current.current.items.some(row=>row===input)||input.id===retired.current?.input_id||input.attempt!==scope.attempt||input.fence!==scope.fence||!canAnswerToolInput(input))return;
   const key=operation(),body=buildToolInputAnswerBody(input,scope,submission,key);if(!body)return;
   original=buildToolInputIntent(input,scope,{path:'/tool-inputs/'+encodeURIComponent(input.id)+'/answer',method:'POST',idempotency_key:key,body});
   if(!original)return;
   try{if(!storage)throw new Error();storage.setItem(intentKey,JSON.stringify(original));setIntent({intent:original});}
   catch{setIntent({error:new UiError('tool-inputs.intent_unavailable')});action.setError(new UiError('tool-inputs.intent_unavailable'));return;}
  }
  commandBusy.current=true;const caller=api;
  try{
   const value=settled.current&&canonical(settled.current.intent)===canonical(original)?settled.current.value:await action.run<ToolInput>(original.request.path,original.request.body,original.request.method);
   if(!value||!alive.current||owner.current!==caller||intent.current!==original)return;
   settled.current={intent:original,value};
   try{if(!storage)throw new Error();storage.removeItem(intentKey);setIntent({});}
   catch{setIntent({intent:original,error:new UiError('tool-inputs.intent_unavailable')});return;}
   settled.current=undefined;++generation.current;controller.current?.abort();flight.current=undefined;
   // The original receipt, rather than a later list head, settles the write.
   // After cleanup the single reader explicitly returns to the current tuple.
   await readNow.current();if(!alive.current||owner.current!==caller)return;
   try{await notify.current?.();}catch{/* Parent refresh cannot undo the saved answer. */}
  }finally{commandBusy.current=false;}
 }
 if(visible.loaded&&!capable&&!visible.items.length&&!hasRecovery&&!intentUnavailable&&!visible.error&&!visible.denied&&!writeDenied)return null;
 return <section className="tool-input-panel" data-tool-input-panel aria-label={t('title')} onFocusCapture={event=>{lastFocus.current=event.target as HTMLElement;}}>
  <div className="tool-input-panel-heading"><button ref={toggleRef} type="button" className="tool-input-panel-toggle" data-tool-input-toggle aria-expanded={open} aria-controls={panelID} onClick={()=>{if(open)toggleRef.current?.focus();setOpen(value=>!value);}}><MessageSquare size={16} aria-hidden="true"/><span>{t('title')}</span>{pendingCount>0&&!visible.denied&&<span className="tool-input-panel-count">{formatNumber(pendingCount)}</span>}<ChevronDown size={15} aria-hidden="true"/></button>
   <button type="button" data-tool-input-refresh aria-label={t('refresh')} disabled={visible.loading||action.busy} onClick={()=>void refresh()}><RefreshCw size={15} aria-hidden="true"/></button></div>
  {!open&&pendingCount>0&&!visible.denied&&<p className="tool-input-panel-note" role="status">{t('pending_summary',{count:formatNumber(pendingCount)})}</p>}
  <WriteNotice action={action} onRebase={async()=>{if(!await refresh())throw new UiError('tool-inputs.refresh_failed');}}/>
  {visible.denied||writeDenied?<p className="tool-input-panel-note" role="status">{t('access_denied')}</p>:<>
   {intentUnavailable&&<p className="warning" role="status">{t('intent_unavailable')}</p>}
   {hasRecovery&&<div className="tool-input-panel-recovery"><p role="status">{t('original_pending')}</p><button type="button" data-tool-input-recover disabled={action.busy||intentUnavailable||!recoveryInput||visible.loading||!!visible.error} onClick={()=>void answer()}>{t('recover')}</button></div>}
   {!!visible.error&&<ErrorNotice error={visible.error}/>}{backgroundPaused&&<p className="tool-input-panel-note" role="status">{t('background_paused')}</p>}
   <div id={panelID} hidden={!open} className="tool-input-panel-body" aria-busy={visible.loading}>
    {effectiveScope.protocol==='codex-app-server-v1'&&<p className="tool-input-panel-note">{t('experimental')}</p>}{historical&&<p className="tool-input-panel-note">{t('historical_readonly')}</p>}{!capable&&!!visible.items.length&&<p className="tool-input-panel-note">{t('capability_disabled')}</p>}
    {visible.loading&&!visible.loaded&&<p role="status">{t('loading')}</p>}{visible.loaded&&!visible.items.length&&<p className="tool-input-panel-note">{t('empty')}</p>}
    {visible.items.map(input=><ToolInputCard key={input.id} input={input} readOnly={historical||!capable||input.attempt!==scope.attempt||input.fence!==scope.fence||input.id===retired.current?.input_id} disabled={!!locked||!canAnswerToolInput(input)} onAnswer={submission=>void answer(input,submission)}/>)}
   </div>
  </>}
 </section>;
}
