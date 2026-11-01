import {useId} from 'react';
import {ArrowRight,CheckCircle2,CircleHelp,ClipboardCheck,RefreshCw,TriangleAlert} from 'lucide-react';
import type {ToolProfileDiagnostics} from '../../../packages/contracts/src';
import {renderDisplayDiagnostic} from './display-diagnostic';
import {formatDateTime,tr,useLocale} from './i18n';
import {ErrorNotice} from './ui';
import './ToolDoctorCard.css';

export type ToolDoctorCardProps={
 report?:ToolProfileDiagnostics;
 state:'loading'|'ready'|'expired'|'changed'|'error'|'suspended';
 disabled?:boolean;
 error?:unknown;
 primaryAction?:{action:'review_profile'|'open_worker'|'register_profile';onClick:()=>void};
 onRefresh:()=>void;
 onRefreshTool?:()=>void;
};
const t=(key:string)=>tr('tool-doctor.'+key);
const stamp=(value:string)=>formatDateTime(value,{dateStyle:'short',timeStyle:'medium',hour12:false});

/** Presentation only: the reader owns source matching, expiry and callbacks. */
export function ToolDoctorCard({report,state,disabled=false,error,primaryAction,onRefresh,onRefreshTool}:ToolDoctorCardProps){
 useLocale();const headingID=useId(),visibleReport=state==='suspended'?undefined:report;
 const firstIssue=visibleReport?.checks.find(check=>check.status==='blocked')??visibleReport?.checks.find(check=>check.status==='not_checked');
 const ready=state==='ready'&&!!visibleReport,action=state==='ready'&&visibleReport&&primaryAction&&visibleReport.next_action===primaryAction.action?primaryAction:undefined;
 const StateIcon=ready?(firstIssue?TriangleAlert:CheckCircle2):state==='loading'?RefreshCw:CircleHelp;
 const summary=ready?(firstIssue?renderDisplayDiagnostic(firstIssue.reason,firstIssue.reason_diagnostic).text:t('prerequisites_checked')):t(state==='error'?'read_failed':state);
 return <section className="tool-doctor" data-tool-doctor-card data-tool-doctor-state={state} aria-labelledby={headingID} aria-busy={state==='loading'}>
  <header className="tool-doctor-heading"><div><ClipboardCheck size={18} aria-hidden="true"/><h4 id={headingID}>{t('title')}</h4></div><span>{t('read_only')}</span></header>
  <div className="tool-doctor-summary" data-tone={ready?(firstIssue?'attention':'checked'):'neutral'} role="status"><StateIcon size={18} aria-hidden="true"/><p>{summary}</p></div>
  {state==='error'&&error!==undefined&&<ErrorNotice error={error}/>}
  {visibleReport&&<>
   <dl className="tool-doctor-times"><div><dt>{t('checked_at')}</dt><dd><time dateTime={visibleReport.checked_at} title={visibleReport.checked_at}>{stamp(visibleReport.checked_at)}</time></dd></div><div><dt>{t('expires_at')}</dt><dd><time dateTime={visibleReport.expires_at} title={visibleReport.expires_at}>{stamp(visibleReport.expires_at)}</time></dd></div></dl>
   {ready&&<p className="tool-doctor-availability">{t(visibleReport.tool.available?'available':'unavailable')}</p>}
   <details className="tool-doctor-details"><summary>{t('checks')}</summary><ol>{visibleReport.checks.map(check=>{
    const StatusIcon=check.status==='pass'?CheckCircle2:check.status==='blocked'?TriangleAlert:CircleHelp;
    return <li key={check.code} data-tool-doctor-check={check.code} data-status={check.status}><div className="tool-doctor-check-heading"><strong>{t('check.'+check.code)}</strong><span><StatusIcon size={15} aria-hidden="true"/>{t('status.'+check.status)}</span></div><p>{renderDisplayDiagnostic(check.reason,check.reason_diagnostic).text}</p></li>;
   })}</ol><p className="tool-doctor-boundary">{t('boundary')}</p></details>
  </>}
  <div className="tool-doctor-actions">
   {action&&<button type="button" className="primary" data-tool-doctor-action={action.action} disabled={disabled} onClick={action.onClick}>{t('action.'+action.action)}<ArrowRight size={15} aria-hidden="true"/></button>}
   {state==='changed'?onRefreshTool&&<button type="button" data-tool-doctor-refresh-tool disabled={disabled} onClick={onRefreshTool}><RefreshCw size={15} aria-hidden="true"/>{t('refresh_tool')}</button>:<button type="button" className="text-button" data-tool-doctor-refresh disabled={disabled||state==='loading'||state==='suspended'} onClick={onRefresh}><RefreshCw size={15} aria-hidden="true"/>{t('refresh')}</button>}
  </div>
 </section>;
}
