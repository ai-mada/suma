import {History,RefreshCw} from 'lucide-react';
import {formatNumber,tr,useLocale} from './i18n';
import type {DesktopToolRecovery} from './desktop-tool-connector-model';
import './DesktopRecoveryStatus.css';

export type DesktopRecoveryStatusProps={
 value?:DesktopToolRecovery;
 busy?:boolean;
 uncertain?:boolean;
 disabled?:boolean;
 onRecover?:()=>void;
 onReadStatus?:()=>void;
};
const t=(key:string)=>tr('desktop-recovery-status.'+key);
const reasonCodes=new Set(['no_saved_results','receipt_missing','receipt_mismatch','unsupported_pending','state_changed','source_changed','access_changed','invalid_state','active_worker','result_unknown','proof_expired']);

/** Only aggregate facts for the current receipt-reconciliation batch. The
 * parent validates metadata and owns the original-operation callbacks. */
export function DesktopRecoveryStatus({value,busy=false,uncertain=false,disabled=false,onRecover,onReadStatus}:DesktopRecoveryStatusProps){
 useLocale();
 const state=busy?'checking':uncertain?'unknown':value?.phase??'unprepared',checking=state==='checking';
 const reason=value?.reason_code&&reasonCodes.has(value.reason_code)?value.reason_code:undefined;
 return <section className="desktop-recovery-status" data-desktop-recovery-status data-desktop-recovery-state={state} aria-label={t('title')} aria-busy={checking}>
  <h4><History size={17} aria-hidden="true"/>{t('title')}</h4>
  <p className="desktop-recovery-result" role="status" aria-live="polite" aria-atomic="true">{t('state.'+state)}</p>
  {value&&<dl className="desktop-recovery-counts"><div><dt>{t('total')}</dt><dd data-desktop-recovery-total>{formatNumber(value.total)}</dd></div><div><dt>{t('recovered')}</dt><dd data-desktop-recovery-recovered>{formatNumber(value.recovered)}</dd></div><div><dt>{t('remaining')}</dt><dd data-desktop-recovery-remaining>{formatNumber(value.remaining)}</dd></div></dl>}
  {reason&&<p className="desktop-recovery-reason" data-desktop-recovery-reason>{t('reason.'+reason)}</p>}
  {(onRecover||onReadStatus)&&<div className="desktop-recovery-actions">
   {onRecover&&<button type="button" className="primary" data-desktop-recovery-action disabled={disabled||checking} onClick={onRecover}><RefreshCw size={15} aria-hidden="true"/>{t(state==='unknown'?'reconcile':'recover')}</button>}
   {onReadStatus&&<button type="button" data-desktop-recovery-read disabled={disabled||busy} onClick={onReadStatus}>{t('read_status')}</button>}
  </div>}
  <p className="desktop-recovery-boundary">{t('boundary')}</p>
  {value?.operation_id&&<details className="desktop-recovery-details"><summary>{t('details')}</summary><dl className="desktop-recovery-meta"><div><dt>{t('operation')}</dt><dd><code>{value.operation_id}</code></dd></div></dl></details>}
 </section>;
}
