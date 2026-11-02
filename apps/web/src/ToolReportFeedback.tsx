import {useEffect,useRef,useState} from 'react';
import {ArrowRight,RefreshCw,Square} from 'lucide-react';
import {ApiError} from './api';
import {useApi,useApiView} from './api-context';
import {tr,useLocale} from './i18n';
import {validToolReportFeedbackTarget,readToolReportFeedback,toolReportFeedbackBaseline,isNewToolReportFeedback,toolReportFeedbackRemainingMs,selectToolReportFeedback,type ToolReportFeedbackTarget,type ToolReportFeedbackSelection} from './tool-report-feedback-model';
import './ToolReportFeedback.css';

export type ToolReportFeedbackProps={project:string;target?:ToolReportFeedbackTarget;intentKey:string;disabled?:boolean;onSelect?:(selection:ToolReportFeedbackSelection)=>void};
type View=NonNullable<ReturnType<typeof readToolReportFeedback>>;
type Baseline=ReturnType<typeof toolReportFeedbackBaseline>;
type Phase='idle'|'checking'|'waiting'|'current'|'new'|'expired'|'unreported'|'forbidden'|'unknown'|'timed_out'|'cancelled';
type Snapshot={phase:Phase;view?:View;receivedAt?:number};
const t=(key:string,values?:Record<string,unknown>)=>tr('tool-report-feedback.'+key,values);

/** The key is an intent boundary, not a cache key. Monotonic source epochs stop
 * A→B→A from resurrecting the previous result or its still-pending response. */
export function ToolReportFeedback(props:ToolReportFeedbackProps){
 useLocale();const api=useApi(),view=useApiView(),source=JSON.stringify([props.project,props.target,props.intentKey,view?.userID,view?.organizationID]),owner=useRef({api,source,epoch:0});
 if(owner.current.api!==api||owner.current.source!==source)owner.current={api,source,epoch:owner.current.epoch+1};
 if(!view?.userID||!view.organizationID||!validToolReportFeedbackTarget(props.target,props.project))return <section className="tool-report-feedback" data-tool-report-feedback><p role="status" data-tool-report-status="target_required">{t('target_required')}</p><button type="button" data-tool-report-check disabled>{t('check')}</button></section>;
 return <FeedbackScope key={owner.current.epoch} {...props} target={props.target}/>;
}
function FeedbackScope({project,target,disabled=false,onSelect}:ToolReportFeedbackProps&{target:ToolReportFeedbackTarget}){
 const api=useApi();useLocale();const [snapshot,setSnapshot]=useState<Snapshot>({phase:'idle'}),[busy,setBusy]=useState(false),[waiting,setWaiting]=useState(false);
 const alive=useRef(true),operation=useRef(0),flight=useRef<AbortController|undefined>(undefined),pollTimer=useRef<ReturnType<typeof setTimeout>|undefined>(undefined),deadlineTimer=useRef<ReturnType<typeof setTimeout>|undefined>(undefined);
 const wait=useRef<{operation:number;deadline:number;baseline?:Baseline}|undefined>(undefined),latest=useRef({snapshot,disabled,onSelect});latest.current={snapshot,disabled,onSelect};
 function stopTimers(){clearTimeout(pollTimer.current);clearTimeout(deadlineTimer.current);pollTimer.current=undefined;deadlineTimer.current=undefined;wait.current=undefined;}
 function terminate(phase:Phase){++operation.current;flight.current?.abort();flight.current=undefined;stopTimers();setBusy(false);setWaiting(false);setSnapshot({phase});}
 function current(id:number,controller:AbortController){return alive.current&&id===operation.current&&!controller.signal.aborted;}
 function schedule(id:number){if(!wait.current||wait.current.operation!==id||document.visibilityState==='hidden')return;if(performance.now()>=wait.current.deadline){terminate('timed_out');return;}clearTimeout(pollTimer.current);pollTimer.current=setTimeout(()=>void read(id),3000);}
 async function read(id:number){
  if(!alive.current||latest.current.disabled||id!==operation.current||flight.current||document.visibilityState==='hidden')return;
  if(wait.current&&performance.now()>=wait.current.deadline){terminate('timed_out');return;}
  const controller=new AbortController();flight.current=controller;setBusy(true);const started=performance.now();
  try{
   const query='project_id='+encodeURIComponent(project),[catalog,inventory]=await Promise.all([api<unknown>('/adapter-catalog?'+query,undefined,'GET',undefined,controller.signal),api<unknown>('/tool-discovery?'+query+'&worker_id='+encodeURIComponent(target.workerID),undefined,'GET',undefined,controller.signal)]);
   if(!current(id,controller))return;const receivedAt=performance.now(),view=readToolReportFeedback(catalog,inventory,target,project,receivedAt-started);if(!view)throw Error('Unverified inventory');
   if(wait.current&&receivedAt>=wait.current.deadline){terminate('timed_out');return;}
   let phase:Phase=view.discovery.state==='unreported'?'unreported':view.remainingMs>0?'current':'expired';
   if(wait.current){
    if(!wait.current.baseline){wait.current.baseline=toolReportFeedbackBaseline(view);phase='waiting';}
    else if(isNewToolReportFeedback(view,wait.current.baseline)){stopTimers();setWaiting(false);phase='new';}
    else phase='waiting';
   }
   setSnapshot({phase,view,receivedAt});
  }catch(error){
   if(!current(id,controller))return;controller.abort();
   if(error instanceof ApiError&&[401,403,404].includes(error.status)){stopTimers();setWaiting(false);setSnapshot({phase:'forbidden'});}else setSnapshot({phase:'unknown'});
  }finally{if(flight.current===controller){flight.current=undefined;if(alive.current&&id===operation.current){setBusy(false);schedule(id);}}}
 }
 function begin(shouldWait:boolean){
  if(disabled||flight.current||wait.current)return;const id=++operation.current;setSnapshot({phase:'checking'});
  if(shouldWait){wait.current={operation:id,deadline:performance.now()+60000};setWaiting(true);deadlineTimer.current=setTimeout(()=>{if(alive.current&&wait.current?.operation===id)terminate('timed_out');},60000);}
  if(document.visibilityState==='hidden'){if(!shouldWait)setSnapshot({phase:'unknown'});return;}void read(id);
 }
 useEffect(()=>{
  alive.current=true;
  const visibility=()=>{if(document.visibilityState==='hidden'){clearTimeout(pollTimer.current);flight.current?.abort();return;}if(wait.current){if(performance.now()>=wait.current.deadline)terminate('timed_out');else void read(wait.current.operation);}};
  document.addEventListener('visibilitychange',visibility);
  return()=>{alive.current=false;++operation.current;flight.current?.abort();stopTimers();document.removeEventListener('visibilitychange',visibility);};
 },[]);
 useEffect(()=>{if(disabled)terminate('cancelled');},[disabled]);
 // Server TTL is discounted by request duration in the pure reader, then only
 // by elapsed monotonic time here. Expiry never waits for another poll.
 useEffect(()=>{if(!snapshot.view||snapshot.receivedAt===undefined)return;const observed=snapshot.view,at=snapshot.receivedAt,remaining=toolReportFeedbackRemainingMs(observed,performance.now()-at);if(remaining<=0){if(observed.discovery.state==='fresh'&&!wait.current)setSnapshot(before=>before.view===observed?{...before,phase:'expired'}:before);return;}
  const timer=setTimeout(()=>setSnapshot(before=>before.view===observed&&before.receivedAt===at?{...before,phase:wait.current?'waiting':'expired'}:before),Math.ceil(remaining));return()=>clearTimeout(timer);
 },[snapshot.view,snapshot.receivedAt]);
 const elapsed=snapshot.receivedAt===undefined?0:Math.max(0,performance.now()-snapshot.receivedAt),remaining=snapshot.view?toolReportFeedbackRemainingMs(snapshot.view,elapsed):0;
 function choose(commandID:string){const source=latest.current;if(source.disabled||!alive.current||!source.snapshot.view||source.snapshot.receivedAt===undefined||!source.onSelect)return;const selection=selectToolReportFeedback(source.snapshot.view,commandID,Math.max(0,performance.now()-source.snapshot.receivedAt));if(!selection)return;terminate('idle');source.onSelect(selection);}
 return <section className="tool-report-feedback" data-tool-report-feedback aria-busy={busy} aria-label={t('title')}>
  <h4>{t('title')}</h4><p className="tool-report-feedback-target">{t('target',{worker:target.workerID,tool:target.localTool})}</p><div className="tool-report-feedback-actions">
   <button type="button" data-tool-report-check disabled={disabled||busy||waiting} onClick={()=>begin(false)}><RefreshCw size={15} aria-hidden="true"/>{t('check')}</button><button type="button" data-tool-report-wait disabled={disabled||busy||waiting} onClick={()=>begin(true)}>{t('wait')}</button>{(busy||waiting)&&<button type="button" data-tool-report-cancel disabled={disabled} onClick={()=>terminate('cancelled')}><Square size={14} aria-hidden="true"/>{t('cancel')}</button>}
  </div><p role="status" data-tool-report-status={snapshot.phase} {...(snapshot.phase==='new'?{'data-tool-report-new':true}:remaining>0?{'data-tool-report-current':true}:{})}>{t('status.'+snapshot.phase)}</p>
  {snapshot.view?.discovery.observedAt&&<p className="tool-report-feedback-meta">{t('observed_at')} <time dateTime={snapshot.view.discovery.observedAt}>{snapshot.view.discovery.observedAt}</time></p>}
  {snapshot.view&&remaining>0&&<ul className="tool-report-feedback-choices">{snapshot.view.choices.map(item=><li key={item.profile.command_id}><div><strong>{item.profile.command_id}</strong><span>{item.profile.protocol}</span>{!item.available&&<><p>{t('unavailable')}</p><details><summary>{t('source_reason')}</summary><code>{item.reason}</code></details></>}</div>{item.available&&onSelect&&<button type="button" data-tool-report-selection={item.profile.command_id} disabled={disabled||busy} onClick={()=>choose(item.profile.command_id)}>{t('return')}<ArrowRight size={15} aria-hidden="true"/></button>}</li>)}</ul>}
  {snapshot.view&&remaining>0&&snapshot.view.choices.length===0&&<p>{t('no_matching')}</p>}<p className="tool-report-feedback-meta">{t('boundary')}</p>
 </section>;
}
