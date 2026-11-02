import {useEffect,useRef,useState} from 'react';
import type {Session} from './api';
import {useApi,useApiView} from './api-context';
import {tr,useLocale} from './i18n';
import {parseTaskAcceptancePanel} from './task-delivery-navigation-model';

type Props={session:Session;project:string;taskID:string;taskVersion:number;runID:string;panel:string;blocked?:boolean;onOpenReview:()=>boolean|void};
type Phase='waiting'|'invalid'|'scope_changed'|'basis_changed'|'blocked'|'unconfirmed'|'opened';
const t=(phase:Phase)=>tr('task-delivery-navigation.locator.'+phase);

/** Consume only a navigation intent. The original Review dialog still owns
 * current approval authority, evidence validation and the eventual write. */
export function TaskAcceptanceLocator({session,project,taskID,taskVersion,runID,panel,blocked=false,onOpenReview}:Props){
 useLocale();const api=useApi(),view=useApiView(),parsed=parseTaskAcceptancePanel(panel);
 const relevant=!!panel&&panel.startsWith('task-acceptance');
 const scopeMatches=!!view&&view.userID===session.user.id&&view.organizationID===session.organization.id&&!!project&&!!taskID;
 const basisMatches=!!parsed&&parsed.taskVersion===taskVersion&&parsed.runID===runID;
 const key=JSON.stringify([session.user.id,session.organization.id,view?.userID,view?.organizationID,project,taskID,panel]);
 const source=JSON.stringify([key,taskVersion,runID]);
 const owner=useRef({api,source,epoch:0});
 if(owner.current.api!==api||owner.current.source!==source)owner.current={api,source,epoch:owner.current.epoch+1};
 const captured=owner.current,callback=useRef(onOpenReview);callback.current=onOpenReview;
 // Consumption deliberately excludes the mutable current task version and
 // caller epoch: refreshing or returning to an earlier source cannot reopen
 // a locator that already opened its original Review once.
 const consumed=useRef(new Set<string>()),attempted=useRef(new Set<string>());
 const cycle=useRef({key,blocked,value:0});
 if(cycle.current.key!==key)cycle.current={key,blocked,value:0};
 else if(cycle.current.blocked!==blocked){if(!blocked)cycle.current.value++;cycle.current.blocked=blocked;}
 const attemptKey=JSON.stringify([key,cycle.current.value]);
 const [outcome,setOutcome]=useState<{key:string;phase:'opened'|'unconfirmed'}>();
 useEffect(()=>{
  if(owner.current!==captured||!relevant||!parsed||!scopeMatches||!basisMatches||blocked||consumed.current.has(key)||attempted.current.has(attemptKey))return;
  attempted.current.add(attemptKey);
  // Reserve before the callback so synchronous parent renders cannot open the
  // same intent twice. A refusal is not consumption and needs an explicit
  // blocked→unblocked transition before another attempt, never a render loop.
  consumed.current.add(key);
  try{
   const accepted=callback.current()!==false;
   if(!accepted)consumed.current.delete(key);
   if(owner.current===captured)setOutcome({key,phase:accepted?'opened':'unconfirmed'});
  }catch{
   consumed.current.delete(key);
   if(owner.current===captured)setOutcome({key,phase:'unconfirmed'});
  }
 },[captured,relevant,!!parsed,scopeMatches,basisMatches,blocked,key,attemptKey]);
 if(!relevant)return null;
 const phase:Phase=!parsed?'invalid':!scopeMatches?'scope_changed':!basisMatches?'basis_changed':blocked?'blocked':consumed.current.has(key)?'opened':outcome?.key===key?outcome.phase:'waiting';
 return <p className="muted" role="status" data-task-acceptance-locator data-state={phase}>{t(phase)}</p>;
}
