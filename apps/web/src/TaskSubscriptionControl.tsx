import {useCallback,useEffect,useLayoutEffect,useRef,useState} from 'react';
import {Bell,RefreshCw} from 'lucide-react';
import type {TaskSubscription,TaskSubscriptionReceipt} from '../../../packages/contracts/src';
import {ApiError,type ApiCaller,type Session} from './api';
import {useApi,useApiView} from './api-context';
import {tr,UiError,useLocale} from './i18n';
import {toolSessionStorage} from './tool-hooks';
import {ErrorNotice} from './ui';
import {useWriteAction,WriteNotice} from './write-action';
import {readTaskSubscription,subscriptionCommandPath,subscriptionPath,validTaskSubscriptionReceipt,validTaskSubscriptionRequest,validTaskSubscriptionScope,type TaskSubscriptionScope} from './task-subscription-model';
import './TaskSubscriptionControl.css';

export type TaskSubscriptionControlProps={session:Session;project:string;taskId:string;refresh:number;accessBlocked:boolean;onAccessDenied?:()=>void};
const t=(key:string)=>tr('task-subscriptions.'+key);
const refusal=(error:unknown)=>error instanceof ApiError&&[401,403,404].includes(error.status);
export function TaskSubscriptionControl(props:TaskSubscriptionControlProps){
 const api=useApi(),view=useApiView(),owner=useRef({api,epoch:0});
 if(owner.current.api!==api)owner.current={api,epoch:owner.current.epoch+1};
 const scope={project:props.project,taskID:props.taskId,userID:props.session.user.id,organizationID:props.session.organization.id};
 const allowed=!!view&&view.userID===scope.userID&&view.organizationID===scope.organizationID&&validTaskSubscriptionScope(scope);
 return <SubscriptionScope {...props} key={JSON.stringify([scope,owner.current.epoch,props.accessBlocked,allowed])} scope={scope} blocked={props.accessBlocked||!allowed}/>;
}
type Snapshot={caller:ApiCaller;value?:TaskSubscription;loading:boolean;error?:unknown;denied?:boolean};
function SubscriptionScope({scope,blocked,refresh,onAccessDenied}:TaskSubscriptionControlProps&{scope:TaskSubscriptionScope;blocked:boolean}){
 useLocale();const api=useApi(),[snapshot,setSnapshot]=useState<Snapshot>(),[confirmed,setConfirmed]=useState(false);
 const current=useRef<Snapshot|undefined>(undefined),alive=useRef(true),epoch=useRef(0),controller=useRef<AbortController|undefined>(undefined),notified=useRef(false),commandBusy=useRef(false);
 const action=useWriteAction({scope:'task-subscription:'+JSON.stringify([scope.userID,scope.organizationID,scope.project,scope.taskID]),storage:toolSessionStorage(),validateRequest:request=>validTaskSubscriptionRequest(request,scope),validateResponse:(value,request)=>validTaskSubscriptionReceipt(value,request,scope)});
 const store=(next:Snapshot)=>{current.current=next;setSnapshot(next);};
 const clearPrivate=useCallback(()=>{
  if(!alive.current)return;++epoch.current;controller.current?.abort();current.current={caller:api,loading:false,denied:true};setSnapshot(current.current);setConfirmed(false);
  if(!notified.current){notified.current=true;onAccessDenied?.();}
 },[api,onAccessDenied]);
 const read=useCallback(async():Promise<TaskSubscription|undefined>=>{
  if(blocked||!validTaskSubscriptionScope(scope))return;
  const generation=++epoch.current;controller.current?.abort();const abort=new AbortController();controller.current=abort;
  const previous=current.current?.caller===api&&!current.current.denied?current.current.value:undefined;
  current.current={caller:api,value:previous,loading:true};setSnapshot(current.current);
  try{
   const raw=await api<unknown>(subscriptionPath(scope),undefined,'GET',{'X-Suma-Expected-User':scope.userID,'X-Suma-Expected-Organization':scope.organizationID},abort.signal);
   if(!alive.current||abort.signal.aborted||generation!==epoch.current)return;
   const value=readTaskSubscription(raw,scope);if(!value)throw new UiError('task-subscriptions.invalid_read');
   store({caller:api,value,loading:false});notified.current=false;setConfirmed(false);return value;
  }catch(error){
   if(!alive.current||abort.signal.aborted||generation!==epoch.current)return;
   if(refusal(error)){clearPrivate();return;}
   store({caller:api,value:previous,loading:false,error});
  }
 },[api,blocked,scope.project,scope.taskID,scope.userID,scope.organizationID,clearPrivate]);
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;++epoch.current;controller.current?.abort();};},[]);
 useEffect(()=>{void read();return()=>{++epoch.current;controller.current?.abort();};},[read,refresh]);
 const writeDenied=!!action.errorStatus&&[401,403,404].includes(action.errorStatus);
 useLayoutEffect(()=>{if(writeDenied)clearPrivate();},[writeDenied,clearPrivate]);
 const visible=snapshot?.caller===api?snapshot:undefined,denied=blocked||writeDenied||!!visible?.denied,value=denied?undefined:visible?.value;
 const locked=commandBusy.current||action.busy||!!action.pending||action.conflict||denied||!value||!!visible?.loading||!!visible?.error;
 async function submit(recover=false){
  const original=action.pending;
  if(commandBusy.current||action.busy||denied||!value||visible?.loading||visible?.error||!recover&&(!!original||action.conflict||value.version>=2147483647)||recover&&!original)return;
  commandBusy.current=true;setConfirmed(false);
  try{
   const result=original&&recover?await action.run<TaskSubscriptionReceipt>(original.path,original.body,original.method):await action.run<TaskSubscriptionReceipt>(subscriptionCommandPath(scope),{project_id:scope.project,task_id:scope.taskID,expected_version:value.version,action:value.following?'unfollow':'follow'});
   if(result&&alive.current){const latest=await read();if(alive.current&&!latest)setConfirmed(true);}
  }finally{commandBusy.current=false;}
 }
 const feedback=!!action.pending||!!action.error||!!visible?.error||denied||confirmed;
 return <div className={'task-subscription-control'+(feedback?' is-expanded':'')} aria-busy={action.busy||!!visible?.loading}>
  <button data-task-subscription-toggle type="button" className="task-subscription-toggle" aria-pressed={value?.following} aria-label={t(denied?'unavailable':value?.following?'unfollow':'follow')} disabled={locked||value?.version===2147483647} onClick={()=>void submit()}><Bell size={15} aria-hidden="true"/>{t(denied?'unavailable':action.busy?'saving':!visible||visible.loading?'loading':!value?'unavailable':value.following?'following':'follow')}</button>
  {feedback&&<div className="task-subscription-feedback">
   {denied?<p role="alert">{t('access_changed')}</p>:<><WriteNotice action={action} onRebase={async()=>{if(!await read())throw new UiError('task-subscriptions.refresh_failed');}}/>{visible?.error&&<ErrorNotice error={visible.error}/>}</>}
   {confirmed&&!denied&&<p role="status">{t('confirmed')}</p>}
   {action.pending&&!denied&&<div className="task-subscription-recovery"><p>{t('original_pending')}</p><button data-task-subscription-recover type="button" disabled={action.busy||!value||visible?.loading||!!visible?.error} onClick={()=>void submit(true)}>{t('recover')}</button></div>}
   {!blocked&&(denied||!!action.pending||!!visible?.error)&&<button data-task-subscription-refresh type="button" disabled={action.busy||!!visible?.loading} onClick={()=>{action.setError('');void read();}}><RefreshCw size={13} aria-hidden="true"/>{t('refresh')}</button>}
  </div>}
 </div>;
}
