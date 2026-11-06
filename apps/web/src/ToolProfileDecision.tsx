import {useEffect,useLayoutEffect,useRef,useState} from 'react';
import {RefreshCw} from 'lucide-react';
import type {ToolProfileDiagnostics} from '../../../packages/contracts/src';
import {useApi,useApiView} from './api-context';
import {query,type ApiCaller} from './api';
import {formatNumber,tr as t,UiError,useLocale} from './i18n';
import {ErrorNotice} from './ui';
import {ToolProfileFacts} from './ToolFacts';
import {ToolProfileImpact} from './ToolProfileImpact';
import {readToolProfileDiagnostics} from './tool-profile-diagnostics-model';
import {sameToolProfile} from './tool-profile-reuse-model';
import type {ToolAction} from './tool-hooks';
import type {ToolProfile} from './tool-model';
import './ToolProfileDecision.css';

type Decision = 'enable'|'pause'|'revoke';
type Props = {
  project:string;
  value:ToolProfile;
  decision:Decision;
  justSaved?:true;
  action:ToolAction;
  onSaved:(value:ToolProfile)=>void;
  onCancel:()=>void;
};

type Review = {caller:ApiCaller;source:ToolProfile;request:number} & (
  {status:'ready';report:ToolProfileDiagnostics;deadline:number} |
  {status:'error';error:unknown}
);

/** The review is a bounded current observation. The existing command still
 * checks the profile version and current write authority at submission. */
export function ProfileDecisionForm({project,value,decision,justSaved,action,onSaved,onCancel}:Props) {
  useLocale();
  const api=useApi(),view=useApiView(),userID=view?.userID,organizationID=view?.organizationID;
  const maintenance=decision!=='enable';
  const [current,setCurrent]=useState(value),[enableReadError,setEnableReadError]=useState<unknown>();
  const [review,setReview]=useState<Review>(),[request,setRequest]=useState(0);
  const [confirmedFor,setConfirmedFor]=useState<ToolProfile|ToolProfileDiagnostics>();
  const [,tick]=useState(0);
  const savedNotice=useRef<HTMLParagraphElement>(null);
  const compactReview=justSaved&&current.state==='paused';
  const profilePath='/tool-profiles/'+encodeURIComponent(value.id);
  const snapshot=review?.caller===api&&review.source===value&&review.request===request?review:undefined;
  const ready=snapshot?.status==='ready'?snapshot:undefined;
  const deadline=ready?.deadline;

  useLayoutEffect(()=>{if(justSaved)savedNotice.current?.focus();},[justSaved]);

  useEffect(()=>{
    if(!maintenance||!userID||!organizationID||value.project_id!==project)return;
    const controller=new AbortController(),started=performance.now();
    const basis={caller:api,source:value,request};
    void api<unknown>(profilePath+'/diagnostics'+query(project),undefined,'GET',{
      'X-Suma-Expected-User':userID,'X-Suma-Expected-Organization':organizationID,
    },controller.signal).then(raw=>{
      if(controller.signal.aborted)return;
      const {report,remainingMs}=readToolProfileDiagnostics(raw,{project,toolID:value.id});
      const tool=report.tool;
      if(tool.version<value.version||tool.worker_id!==value.worker_id||!sameToolProfile(tool.profile,value.profile)||
        tool.version===value.version&&tool.state!==value.state)throw new UiError('tool-doctor.invalid_response');
      setReview({...basis,status:'ready',report,deadline:started+remainingMs});
    }).catch(error=>{
      if(!controller.signal.aborted)setReview({...basis,status:'error',error});
    });
    return ()=>controller.abort();
  },[api,maintenance,organizationID,profilePath,project,request,userID,value]);

  useEffect(()=>{
    if(deadline===undefined)return;
    const remaining=deadline-performance.now();
    if(remaining<=0)return;
    const timer=setTimeout(()=>tick(count=>count+1),Math.ceil(remaining)+1);
    return ()=>clearTimeout(timer);
  },[deadline]);

  const working=action.busy||!!action.pending;
  const scopeReady=!!userID&&!!organizationID&&value.project_id===project;
  const commandRefused=action.errorStatus===401||action.errorStatus===403||action.errorStatus===404;
  const reviewState=action.conflict?'conflict':commandRefused||!scopeReady?'error':!snapshot?'loading':
    snapshot.status==='error'?'error':performance.now()>=snapshot.deadline?'expired':'ready';
  const reviewed=reviewState==='ready'?ready?.report:undefined;
  const tool=maintenance?reviewed?.tool:current;
  const applicable=!!tool&&(decision==='pause'?tool.state==='enabled':decision==='revoke'?tool.state!=='revoked':true);
  const approvalSource=maintenance?reviewed:current;
  const confirmed=!!approvalSource&&confirmedFor===approvalSource;

  function refreshReview(){
    if(working)return;
    setConfirmedFor(undefined);
    setRequest(previous=>previous+1);
    action.clearError();
  }

  return (
    <form className="tool-form tool-decision" onSubmit={async event=>{
      event.preventDefault();
      if(!confirmed||!applicable||!tool||working||action.conflict||
        maintenance&&(!ready||performance.now()>=ready.deadline))return;
      if(maintenance)setConfirmedFor(undefined);
      const result=await action.run<ToolProfile>(profilePath+'/commands',{
        project_id:project,action:decision,expected_version:tool.version,
      });
      if(result)onSaved(result);
    }}>
      {justSaved&&<p ref={savedNotice} tabIndex={-1} data-quick-paused-for-review>{t('tool-quick-connect.saved_for_review',{name:value.name})}</p>}
      {maintenance?<section className="tool-decision-impact" data-tool-decision-impact data-state={reviewState}>
        <div className="tool-decision-review-header">
          <div><h3>{t('tool-management.decision.review')}</h3><p>{t('tool-management.decision.scope')}</p></div>
          <button type="button" className="tool-decision-refresh" data-tool-decision-refresh
            disabled={working||!scopeReady||reviewState==='loading'} onClick={refreshReview}>
            <RefreshCw size={16} aria-hidden="true"/>{t('tool-management.decision.refresh')}
          </button>
        </div>
        {reviewState==='loading'&&<p role="status">{t('tool-management.decision.loading')}</p>}
        {reviewState==='error'&&(commandRefused?<p role="status">{t('tool-management.decision.access_changed')}</p>:
          <ErrorNotice error={snapshot?.status==='error'?snapshot.error:new UiError('tool-doctor.invalid_response')}/>)}
        {reviewState==='expired'&&<p role="status">{t('tool-management.decision.expired')}</p>}
        {reviewState==='conflict'&&<p role="status">{t('tool-management.decision.conflict')}</p>}
        {reviewed&&<>
          <dl className="tool-quick-review-facts tool-decision-facts">
            <div><dt>{t('tool-management.decision.configuration')}</dt><dd>{reviewed.tool.name}</dd></div>
            <div><dt>{t('tool-management.decision.version')}</dt><dd>{formatNumber(reviewed.tool.version)}</dd></div>
            <div><dt>{t('tool-quick-connect.node')}</dt><dd>{reviewed.tool.worker_id}</dd></div>
            <div><dt>{t('tool-workspace.command')}</dt><dd>{reviewed.tool.profile.command_id}</dd></div>
          </dl>
          {reviewed.tool.version!==value.version&&<p role="status">{t('tool-management.decision.version_changed')}</p>}
          {!applicable&&<p role="status">{t(reviewed.tool.state==='revoked'?'tool-management.decision.already_removed':'tool-management.decision.already_paused')}</p>}
          <ToolProfileImpact impact={reviewed.impact}/>
        </>}
      </section>:compactReview?<dl className="tool-quick-review-facts">
        <div><dt>{t('tool-quick-connect.node')}</dt><dd>{current.worker_id}</dd></div>
        <div><dt>{t('tool-workspace.command')}</dt><dd>{current.profile.command_id}</dd></div>
        <div><dt>{t('development.facts.fixed_version')}</dt><dd>{current.profile.tool_version}</dd></div>
        <div><dt>{t('development.facts.execution_permission')}</dt><dd>{t(current.profile.read_only?'tool-quick-connect.access_read_only':'tool-quick-connect.access_write')}</dd></div>
      </dl>:<ToolProfileFacts profile={current}/>}
      {decision==='enable'&&current.profile.adapter==='qoder'&&current.profile.protocol==='acp-stdio-v1'&&<p className="muted">{t('acp-runtime.setup_policy')}</p>}
      {!!enableReadError&&<ErrorNotice error={enableReadError}/>}
      <p className={maintenance?'warning':'muted'}>
        {compactReview?t('tool-quick-connect.enable_review_hint'):decision==='enable'
          ?t('development.tools.enabling_still_requires_matching_current_identity_live')
          :decision==='pause'?t('development.tools.pausing_blocks_subsequent_dispatch_only_operations_already')
          :t('tool-workspace.remove_hint')}
      </p>
      <label className="check">
        <input type="checkbox" data-tool-decision-confirm checked={confirmed} disabled={!applicable||working||action.conflict}
          onChange={event=>setConfirmedFor(event.target.checked?approvalSource:undefined)}/>
        {maintenance&&!tool?t('tool-management.decision.confirm_after_review'):decision==='revoke'
          ?t('tool-workspace.remove_confirm',{version:tool?.version})
          :t('development.tools.confirm_'+decision,{version:tool?.version})}
      </label>
      {!maintenance&&action.conflict&&<div className="conflict-banner">
        <p>{t('development.tools.the_state_changed_your_decision_is_retained')}</p>
        <button type="button" onClick={async()=>{
          try{
            setCurrent(await api<ToolProfile>(profilePath+query(project)));
            setEnableReadError(undefined);
            action.clearError();
          }catch(error){setEnableReadError(error);}
        }}>{t('development.tools.read_current_configuration_keep_decision')}</button>
      </div>}
      <div className="dialog-actions">
        <button type="button" disabled={working} onClick={onCancel}>{t('development.workflow.cancel')}</button>
        <button type="submit" className="primary" disabled={working||action.conflict||!confirmed||!applicable}>
          {decision==='enable'?t('development.tools.confirm_enabling_this_configuration'):decision==='pause'
            ?t('development.tools.confirm_pausing_this_configuration'):t('tool-workspace.remove')}
        </button>
      </div>
      {compactReview&&<details className="tool-quick-review-details">
        <summary>{t('tool-quick-connect.diagnostics')}</summary><ToolProfileFacts profile={current}/>
      </details>}
    </form>
  );
}
