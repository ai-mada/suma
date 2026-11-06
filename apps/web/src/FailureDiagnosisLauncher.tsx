import {useCallback,useEffect,useId,useRef,useState} from 'react';
import {ChevronDown} from 'lucide-react';
import type {AutomationRunOrigin,FailureDiagnosisIssueSource} from '../../../packages/contracts/src';
import {ApiError,query,stateLabel,type Run,type Session} from './api';
import {useApi} from './api-context';
import {expertID,expertText} from './expert-team-model';
import {formatNumber,tr,UiError,useLocale} from './i18n';
import {automationTriggerPanelPrefix} from './navigation';
import {toolError} from './tool-hooks';
import {ErrorNotice,Field,Loading} from './ui';
import {useDiagnosisRead} from './failure-diagnosis-hooks';
import {diagnosisID,diagnosisLocator} from './failure-diagnosis-model';
import {FailureDiagnosisPanel} from './FailureDiagnosisPanel';
import './FailureDiagnosisPanel.css';

type LauncherView={session:Session;project:string;refresh?:number;onChanged?:()=>void|Promise<void>};
export type FailureDiagnosisLauncherProps=LauncherView&({runID:string;issueID?:never}|{issueID:string;runID?:never});
const t=(key:string,values?:Record<string,unknown>)=>tr('failure-diagnosis.'+key,values);
const validRunID=(value:unknown):value is string=>typeof value==='string'&&/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,100}$/.test(value);
const canLeave=()=>window.dispatchEvent(new Event('suma:navigate',{cancelable:true}));
const ignoreReadFailure=()=>{};

function isRecordedAutomationTime(value:unknown):value is string{
 if(typeof value!=='string'||!/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ$/.test(value))return false;
 const time=Date.parse(value);
 return Number.isFinite(time)&&new Date(time).toISOString().replace('.000Z','Z')===value;
}

function IssueAutomationOrigin({project,origin}:{project:string;origin?:AutomationRunOrigin}){
 if(!origin||!expertID(project)||!expertID(origin.rule_id)||!expertID(origin.occurrence_id)||!expertText(origin.rule_name,200)||
  !Number.isSafeInteger(origin.revision)||origin.revision<1||!['cron','manual','webhook'].includes(origin.source)||!isRecordedAutomationTime(origin.scheduled_at))return null;
 const pause=origin.failure_pause;
 const validPause=pause&&expertID(pause.occurrence_id)&&pause.occurrence_id!==origin.occurrence_id&&
  Number.isSafeInteger(pause.threshold)&&pause.threshold>=2&&pause.threshold<=10&&isRecordedAutomationTime(pause.observed_at);
 const source=tr(origin.source==='cron'?'automation.scheduled_trigger':origin.source==='manual'?'automation.manual_trigger':'automation.signed_event_trigger');
 const href=(trigger:string)=>'?'+new URLSearchParams({project,view:'development',sub:'automations',record:origin.rule_id,panel:automationTriggerPanelPrefix+trigger});
 return <div className="fd-callout fd-automation-origin">
  <h4>{tr('automation.run_origin.title')}</h4>
  <p><strong>{origin.rule_name}</strong> · {tr('automation.run_origin.context',{source,revision:formatNumber(origin.revision)})}</p>
  {validPause&&<p className="fd-muted">{tr('automation.run_origin.pause_context',{threshold:formatNumber(pause.threshold)})}</p>}
  <div className="fd-actions">
   <a data-fd-automation-trigger href={href(origin.occurrence_id)} onClick={event=>{if(!canLeave())event.preventDefault();}}>{tr('automation.run_origin.open')}</a>
   {validPause&&<a data-fd-automation-pause href={href(pause.occurrence_id)} onClick={event=>{if(!canLeave())event.preventDefault();}}>{tr('automation.run_origin.pause_open')}</a>}
  </div>
 </div>;
}

export function FailureDiagnosisLauncher(props:FailureDiagnosisLauncherProps){
 const {session,project,issueID,runID}=props;
 const scope=JSON.stringify([session.user.id,session.organization.id,project,issueID===undefined?'run':'issue',issueID??runID]);
 return <LauncherScope key={scope} {...props}/>;
}

function LauncherScope(props:FailureDiagnosisLauncherProps){
 useLocale();
 const bodyID=useId(),toggle=useRef<HTMLButtonElement>(null),[open,setOpen]=useState(false);
 return <section className="failure-diagnosis-panel fd-launcher">
  <button ref={toggle} data-fd-launch-toggle type="button" className="fd-toggle" aria-expanded={open} aria-controls={bodyID} onClick={()=>{if(!open||canLeave())setOpen(!open);}}>
   <span>{t('launcher_title')}</span><ChevronDown size={16} aria-hidden="true"/>
  </button>
  {open&&<div id={bodyID} className="fd-body">
   <p className="fd-muted">{t('launcher_help')}</p>
   {props.issueID===undefined?<LauncherRun {...props}/>:<LauncherIssue {...props} focusToggle={()=>toggle.current?.focus()}/>}
  </div>}
 </section>;
}

function LauncherIssue({issueID,focusToggle,...view}:LauncherView&{issueID:string;focusToggle:()=>void}){
 const api=useApi(),{project}=view,validID=diagnosisID(issueID);
 const valid=useCallback((value:FailureDiagnosisIssueSource)=>!!value&&value.project_id===project&&value.issue_id===issueID&&
  (value.run_id===undefined||validRunID(value.run_id)),[project,issueID]);
 // The issue association is independent of the editable record. Recheck it on
 // opening or explicit refresh so unrelated updates cannot discard a draft.
 const source=useDiagnosisRead(api,'/failure-diagnosis/issue-source'+query(project)+'&issue_id='+encodeURIComponent(issueID),validID,0,valid,ignoreReadFailure);
 const recheck=()=>{focusToggle();void source.read();};
 if(!validID)return <p className="warning">{t('launcher_issue_invalid')}</p>;
 if(source.loading)return <p className="fd-muted" role="status">{t('launcher_issue_loading')}</p>;
 if(source.value?.run_id&&!source.error)return <>
  <IssueAutomationOrigin project={project} origin={source.value.automation_origin}/>
  <LauncherRun key={source.value.run_id} {...view} runID={source.value.run_id} onRecheck={recheck}/>
 </>;
 return <>
  {source.error?<ErrorNotice error={source.error instanceof UiError?t('launcher_issue_invalid'):toolError(source.error)}/>:<p className="fd-muted" role="status">{t('launcher_issue_empty')}</p>}
  <div className="fd-actions"><button data-fd-launch-refresh type="button" onClick={()=>{if(canLeave())recheck();}}>{t('refresh')}</button></div>
 </>;
}

function LauncherRun({session,project,runID,refresh=0,onChanged,onRecheck}:LauncherView&{runID:string;onRecheck?:()=>void}){
 const api=useApi(),[selected,setSelected]=useState(''),[tick,setTick]=useState(0),[forbidden,setForbidden]=useState<unknown>();
 const validID=validRunID(runID);
 const failed=useCallback((error:unknown)=>{if(error instanceof ApiError&&[401,403].includes(error.status)){setForbidden(error);setSelected('');}},[]);
 const valid=useCallback((value:Run)=>!!value&&value.id===runID&&value.project_id===project&&Number.isSafeInteger(value.version)&&value.version>0&&typeof value.state==='string'&&Array.isArray(value.nodes)&&value.nodes.length<=10000&&value.nodes.every(node=>!!node&&typeof node.id==='string'&&typeof node.state==='string'&&Number.isSafeInteger(node.attempt)&&node.attempt>=0)&&new Set(value.nodes.map(node=>node.id)).size===value.nodes.length,[project,runID]);
 const detail=useDiagnosisRead(api,'/runs/'+encodeURIComponent(runID)+query(project),validID&&!forbidden,refresh+tick,valid,failed),run=detail.value;
 const choices=run?.nodes.filter(node=>['failed','reconciling'].includes(node.state)&&diagnosisLocator({run_id:runID,node_id:node.id,attempt:node.attempt}))||[],node=choices.find(node=>node.id===selected),unknown=!!run&&(run.state==='reconciling'||run.nodes.some(value=>value.state==='reconciling'));
 useEffect(()=>{if(run&&selected&&!choices.some(value=>value.id===selected))setSelected('');},[run,selected]);
 function refreshSource(){
  if(!canLeave())return;
  if(onRecheck){onRecheck();return;}
  setForbidden(undefined);setTick(value=>value+1);
 }
 const link='?'+new URLSearchParams({project,view:'development',sub:'runs',run:runID,...(node?{node:node.id,attempt:String(node.attempt)}:{})});
 return <>
  <div className="fd-actions">
   {validID&&!forbidden&&!detail.error&&(!onRecheck||run)&&<a data-fd-launch-run href={link} onClick={event=>{if(!canLeave())event.preventDefault();}}>{t('open_source_run')}</a>}
   <button data-fd-launch-refresh type="button" disabled={detail.loading} onClick={refreshSource}>{t('refresh')}</button>
  </div>
   {!validID&&<p className="warning">{t('launcher_invalid_run')}</p>}{detail.loading&&<Loading/>}
   {forbidden?<><p className="fd-warning">{t('launcher_access')}</p><ErrorNotice error={toolError(forbidden)}/></>:<>
    {Boolean(detail.error)&&<ErrorNotice error={detail.error instanceof UiError?t('launcher_invalid_run'):toolError(detail.error)}/>}
    {run&&<>{unknown&&<p className="fd-warning" role="status">{t('launcher_unknown')}</p>}{choices.length?<Field label={t('launcher_node')}><select name="fd_current_node" aria-label={t('launcher_node')} value={selected} disabled={detail.loading||!!detail.error} onChange={event=>{if(canLeave())setSelected(event.target.value);}}><option value="">{t('launcher_choose_node')}</option>{choices.map(value=><option key={value.id} value={value.id}>{value.name||value.id} · {stateLabel(value.state)} · {t('launcher_attempt',{value:formatNumber(value.attempt)})}</option>)}</select></Field>:<p className="fd-muted">{t('launcher_empty')}</p>}
     {node&&node.state==='failed'&&!unknown&&!detail.error&&<FailureDiagnosisPanel session={session} project={project} source={{run_id:run.id,node_id:node.id,attempt:node.attempt}} initiallyOpen refresh={refresh} onChanged={onChanged}/>}
    </>}
   </>}
 </>;
}
