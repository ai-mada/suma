import {tr,useLocale} from './i18n';

// Keep the original reason visible. This closed host explanation adds an action
// hint; it never translates, evaluates or replaces user/model evidence.
export function AccountingReasonExplanation({reason}:{reason?:string}){
 useLocale();
 return reason==='Model token or cost allowance is unavailable'?<small className="muted">{tr('accounting-host.blocked_hint')}</small>:null;
}
