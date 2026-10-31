import {CheckCircle2,CircleAlert,Clock3,RefreshCw} from 'lucide-react';
import {tr,useLocale} from './i18n';
import './ToolRegistrationPreflight.css';

export type ToolRegistrationPreflightProps={
 checks:ReadonlyArray<{id:string;label:string;detail:string;state:'ready'|'attention'|'pending'}>;
 next?:string;
 onReview?:()=>void;
 reviewLabel?:string;
 disabled?:boolean;
};
const t=(key:string)=>tr('tool-registration-preflight.'+key);
const icons={ready:CheckCircle2,attention:CircleAlert,pending:Clock3};

/** Presentation of the parent's current facts; never probes or registers a tool. */
export function ToolRegistrationPreflight({checks,next,onReview,reviewLabel,disabled=false}:ToolRegistrationPreflightProps){
 useLocale();
 return <section className="tool-registration-preflight" data-tool-registration-preflight aria-label={t('title')}>
  <h4>{t('title')}</h4>
  <ul className="tool-registration-preflight-checks">
   {checks.map(check=>{const Icon=icons[check.state];return <li key={check.id} data-preflight-check={check.id} data-preflight-state={check.state}>
    <Icon className="tool-registration-preflight-icon" size={18} aria-hidden="true"/>
    <div className="tool-registration-preflight-copy"><div className="tool-registration-preflight-label"><strong>{check.label}</strong><span>{t('state.'+check.state)}</span></div><p>{check.detail}</p></div>
   </li>;})}
  </ul>
  {(next||onReview)&&<div className="tool-registration-preflight-next">
   {next&&<div role="status" aria-atomic="true"><strong>{t('next')}</strong><p>{next}</p></div>}
   {onReview&&<button type="button" data-preflight-review disabled={disabled} onClick={onReview}><RefreshCw size={16} aria-hidden="true"/>{reviewLabel||t('review')}</button>}
  </div>}
 </section>;
}
