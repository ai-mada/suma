import {useCallback,useState} from 'react';
import {ArrowUpRight,RefreshCw} from 'lucide-react';
import {query} from './api';
import {useApi} from './api-context';
import {formatNumber,tr,useLocale} from './i18n';
import {ReleaseFingerprint,ReleaseWriteNotice} from './ReleaseFacts';
import {releaseError,type ReleaseAction} from './release-hooks';
import type {CandidateDetail} from './release-model';
import {repairReleaseCandidateRequest,repairReleaseIssueHref,repairReleaseReview,type RepairReleaseTarget} from './repair-release-handoff';
import {useTestPlanRead} from './test-plans-hooks';
import {ErrorNotice,Field,Loading} from './ui';
import {useLeaveGuard} from './write-action';

const ignoreReadError=()=>{};
const canLeave=()=>window.dispatchEvent(new Event('suma:navigate',{cancelable:true}));
const label=(key:string)=>tr('release.repair_handoff.'+key);

type Props={
 project:string;
 target:RepairReleaseTarget;
 refresh:number;
 action:ReleaseAction;
 onCancel:()=>void;
 onSaved:(result:CandidateDetail)=>void;
 onRestore:()=>void;
};

export function RepairReleaseCandidateForm({project,target,refresh,action,onCancel,onSaved,onRestore}:Props){
 useLocale();
 const api=useApi(),[name,setName]=useState('');
 const valid=useCallback((value:unknown):value is object=>value!==null&&typeof value==='object'&&!Array.isArray(value),[]);
 const read=useTestPlanRead(api,'/issues/'+encodeURIComponent(target.issue)+'/repair'+query(project),refresh,valid,ignoreReadError);
 const review=repairReleaseReview(read.value,project,target);
 const blocked=action.busy||!!action.pending;
 useLeaveGuard(!!name.trim()&&!action.pending,action.setError);

 const href=(view:string,sub:string,record:string)=>'?'+new URLSearchParams({project,view,sub,record});
 const link=(event:React.MouseEvent<HTMLAnchorElement>)=>{if(!canLeave())event.preventDefault();};
 return <form className="release-dialog-form" data-repair-release-review onSubmit={async event=>{
  event.preventDefault();
  if(!review||!name.trim()||blocked||action.conflict||read.loading||read.error)return;
  const result=await action.run<CandidateDetail>('/release-candidates',repairReleaseCandidateRequest(project,name,review.handoff));
  if(result){setName('');onSaved(result);}
 }}>
  <ReleaseWriteNotice action={action}/>
  {action.pending&&<button type="button" disabled={action.busy} onClick={onRestore}>{tr('release.recovery.restore')}</button>}
  {read.loading&&<Loading/>}
  {!!read.error&&<ErrorNotice error={releaseError(read.error)} onRetry={()=>void read.read()}/>}
  {!read.loading&&!read.error&&!review&&<div className="warning">
   <p role="status">{label('unavailable')}</p>
   <a className="release-link" href={repairReleaseIssueHref(project,target.issue)} onClick={link}>{label('open_issue')}<ArrowUpRight size={13} aria-hidden="true"/></a>
  </div>}
  {review&&<>
   <div className="release-target-summary">
    <div className="release-section-heading">
     <h3>{label('basis')}</h3>
     <span>{tr('release.repair_handoff.cycle',{cycle:formatNumber(target.cycle)})}</span>
    </div>
    <div className="release-actions">
     <a className="release-link" href={repairReleaseIssueHref(project,target.issue)} onClick={link}>{label('open_issue')}<ArrowUpRight size={13} aria-hidden="true"/></a>
     <a className="release-link" href={href('development','tasks',review.handoff.task_id)} onClick={link}>{review.taskTitle||label('open_task')}<ArrowUpRight size={13} aria-hidden="true"/></a>
     <a className="release-link" href={href('operations','artifacts',review.handoff.artifact_id)} onClick={link}>{label('open_artifact')}<ArrowUpRight size={13} aria-hidden="true"/></a>
    </div>
    <ReleaseFingerprint value={review.manifest}/>
    <ReleaseFingerprint value={review.commit} label={tr('release.proof.commit')}/>
    <details><summary>{label('proofs')}</summary>
     <ReleaseFingerprint value={review.handoff.review_id} label={label('review')}/>
     <ReleaseFingerprint value={review.handoff.build_attestation_id} label={tr('release.candidate.build')}/>
     <ReleaseFingerprint value={review.handoff.package_test_attestation_id} label={tr('release.candidate.test')}/>
    </details>
   </div>
   <fieldset className="form-fields" disabled={blocked||action.conflict}>
    <Field label={tr('release.candidate.name')}><input name="name" required maxLength={200} value={name} onChange={event=>setName(event.target.value)} placeholder={tr('release.candidate.name_placeholder')}/></Field>
   </fieldset>
  </>}
  {action.conflict&&!blocked&&<div className="conflict-banner">
   <p>{label('conflict')}</p>
   <button type="button" disabled={read.loading} onClick={async()=>{await read.read();action.clearError();}}><RefreshCw size={14}/>{tr('release.candidate.refresh')}</button>
  </div>}
  <div className="dialog-actions">
   <button type="button" disabled={blocked} onClick={()=>{setName('');onCancel();}}>{tr('release.form.cancel')}</button>
   <button className="primary" type="submit" disabled={!review||read.loading||!!read.error||!name.trim()||blocked||action.conflict}>{action.busy?tr('release.candidate.saving'):tr('release.candidate.save')}</button>
  </div>
 </form>;
}
