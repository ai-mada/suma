import {Shield,RefreshCw} from 'lucide-react';
import {formatDateTime,formatNumber,tr,useLocale} from './i18n';
import type {DesktopToolRenewalReason} from './desktop-tool-connector-model';
import './DesktopCredentialStatus.css';

export type DesktopCredentialStatusProps={
 credentialExpiresAt?:string;
 credentialVersion?:number;
 inventoryExpiresAt?:string;
 renewalUnknown?:boolean;
 renewalReason?:DesktopToolRenewalReason;
 renewing?:boolean;
 disabled?:boolean;
 onRenew?:()=>void;
};
const t=(key:string)=>tr('desktop-credential-status.'+key);
function expiry(value?:string){
 return value&&Number.isFinite(Date.parse(value))?<time dateTime={value}>{formatDateTime(value,{dateStyle:'short',timeStyle:'medium'})}</time>:t('unverified');
}

/** Expiry records and observations are distinct; only the parent owns renewal. */
export function DesktopCredentialStatus({credentialExpiresAt,credentialVersion,inventoryExpiresAt,renewalUnknown=false,renewalReason,renewing=false,disabled=false,onRenew}:DesktopCredentialStatusProps){
 useLocale();
 return <section className="desktop-credential-status" data-desktop-credential-status aria-label={t('title')} aria-busy={renewing}>
  <h4><Shield size={17} aria-hidden="true"/>{t('title')}</h4>
  <dl>
   <div><dt>{t('credential_expiry')}</dt><dd data-desktop-credential-expiry>{expiry(credentialExpiresAt)}</dd></div>
   <div><dt>{t('credential_version')}</dt><dd data-desktop-credential-version>{typeof credentialVersion==='number'&&Number.isSafeInteger(credentialVersion)&&credentialVersion>0?formatNumber(credentialVersion):t('unverified')}</dd></div>
   <div><dt>{t('inventory_expiry')}</dt><dd data-desktop-inventory-expiry>{expiry(inventoryExpiresAt)}</dd></div>
  </dl>
  {(renewalUnknown||renewing)&&<p className="desktop-credential-result" role="status" data-desktop-credential-result>{t(renewing?renewalUnknown?'checking':'renewing':'unknown')}</p>}
  {renewalReason&&<p data-desktop-credential-reason role="status">{t('reason.'+renewalReason)}</p>}
  {onRenew&&<div className="desktop-credential-actions"><button type="button" data-desktop-credential-renew disabled={disabled||renewing} onClick={onRenew}><RefreshCw size={15} aria-hidden="true"/>{t(renewalUnknown?'reconcile':'renew')}</button><p>{t('connect_after')}</p></div>}
 </section>;
}
