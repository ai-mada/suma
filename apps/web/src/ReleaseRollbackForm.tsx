import {useState} from 'react';
import {RefreshCw} from 'lucide-react';
import {formatNumber,tr,UiError,useLocale} from './i18n';
import {HealthPolicyFacts,ReleaseBlockers,ReleaseFingerprint,ReleaseWriteNotice} from './ReleaseFacts';
import {releaseError,type ReleaseAction} from './release-hooks';
import {hasReleaseAction,planCreateRequest,type PlanInput,type PlanPreview,type ReleaseDeployment,type ReleasePlan} from './release-model';
import {ErrorNotice} from './ui';

export type RollbackReview={input:PlanInput;preview:PlanPreview};
type Source=Pick<ReleaseDeployment,'project_id'|'operation_id'|'environment_id'|'artifact_sha256'>;

/** A rollback preview must belong to the selected historical operation.
 * Create rechecks the same preview hash and current environment generation. */
export function rollbackReview(source:Source,generation:number,value:RollbackReview):RollbackReview{
 const {input,preview}=value;
 if(input.project_id!==source.project_id||input.action!=='rollback'||input.rollback_source_operation_id!==source.operation_id||
    input.environment_id!==source.environment_id||input.expected_environment_generation!==generation||
    preview.action!=='rollback'||preview.environment.project_id!==source.project_id||preview.environment.id!==source.environment_id||
    preview.environment.generation!==generation||!/^[A-Za-z0-9_-]{1,64}$/.test(input.candidate_id)||preview.candidate_id!==input.candidate_id||
    !/^[a-f0-9]{64}$/.test(preview.preview_sha256)||typeof preview.eligibility!=='boolean'||
    !Array.isArray(preview.blockers)||!Array.isArray(preview.allowed_actions)||!Array.isArray(preview.health?.checks)){
  throw new UiError('release.validation.preview_stale');
 }
 return value;
}

type Props={
 source:Source;
 initial:RollbackReview;
 refresh:()=>Promise<RollbackReview>;
 action:ReleaseAction;
 onRestore:()=>void;
 onCancel:()=>void;
 onSaved:(plan:ReleasePlan)=>void;
};

export function ReleaseRollbackForm({source,initial,refresh,action,onRestore,onCancel,onSaved}:Props){
 useLocale();
 const [review,setReview]=useState<RollbackReview|undefined>(initial),[reading,setReading]=useState(false),[error,setError]=useState<unknown>();
 const blocked=action.busy||!!action.pending;
 const environment=review?.preview.environment;

 async function reload(){
  setReview(undefined);setError(undefined);setReading(true);
  try{setReview(await refresh());action.clearError();}
  catch(error){setError(error);}
  finally{setReading(false);}
 }

 return <form className="release-dialog-form" onSubmit={async event=>{
  event.preventDefault();
  if(!review||blocked||reading||action.conflict)return;
  try{
   const body=planCreateRequest(review.input,review.input,review.preview);
   const result=await action.run<ReleasePlan>('/release-plans',body);
   if(result)onSaved(result);
  }catch(error){setError(error);}
 }}>
  <ReleaseWriteNotice action={action}/>
  {action.pending&&<button type="button" disabled={action.busy} onClick={onRestore}>{tr('release.recovery.restore')}</button>}
  {!!error&&<ErrorNotice error={releaseError(error)}/>}
  <p className="release-form-help">{tr('release.prepare.rollback_help')}</p>
  <div className="release-target-summary">
   <ReleaseFingerprint value={source.operation_id} label={tr('release.plan.rollback_source')}/>
   <ReleaseFingerprint value={source.artifact_sha256}/>
   {environment&&<><strong>{environment.name}</strong><p>{tr('release.prepare.generations',{active:formatNumber(environment.active_generation),reserved:formatNumber(environment.generation)})}</p></>}
  </div>
  <button type="button" disabled={blocked||reading} onClick={()=>void reload()}><RefreshCw size={14}/>{reading?tr('release.prepare.checking'):tr('release.prepare.refresh')}</button>
  {review&&<section className="release-preview">
   <div className="release-section-heading"><h3>{tr('release.prepare.preview_title')}</h3><strong>{review.preview.eligibility?tr('release.prepare.eligible'):tr('release.prepare.blocked')}</strong></div>
   <ReleaseFingerprint value={review.preview.preview_sha256} label={tr('release.prepare.preview_hash')}/>
   <ReleaseBlockers items={review.preview.blockers}/>
   <HealthPolicyFacts policy={review.preview.health}/>
   <p className="muted">{tr('release.prepare.not_created')}</p>
  </section>}
  <div className="dialog-actions">
   <button type="button" disabled={blocked} onClick={onCancel}>{tr('release.form.cancel')}</button>
   <button className="primary" disabled={blocked||reading||action.conflict||!review||!hasReleaseAction(review.preview,'create')}>{action.busy?tr('release.form.saving'):tr('release.prepare.save')}</button>
  </div>
 </form>;
}
