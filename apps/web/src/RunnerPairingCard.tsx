import {useEffect,useId,useState} from 'react';
import {Check,Copy,Fingerprint,RefreshCw,ShieldCheck} from 'lucide-react';
import type {RunnerPairing} from '../../../packages/contracts/src';
import {formatDateTime,formatNumber,tr,useLocale} from './i18n';
import {runnerPairingCommand} from './runner-pairing-model';
import {ErrorNotice} from './ui';
import './RunnerPairingCard.css';

export type RunnerPairingCardProps={pairing?:RunnerPairing;loading:boolean;stale?:boolean;disabled?:boolean;error?:unknown;pending?:boolean;onCreate:()=>void;onRefresh:()=>void;onApprove:()=>void;onReject:()=>void;onRestore?:()=>void;onContinue?:()=>void};
const t=(key:string)=>tr('runner-pairing.'+key);
const stamp=(value:string)=>formatDateTime(value,{dateStyle:'short',timeStyle:'medium',hour12:false});

/** The parent owns current reads, original pending writes and all commands. */
export function RunnerPairingCard({pairing,loading,stale=false,disabled=false,error,pending=false,onCreate,onRefresh,onApprove,onReject,onRestore,onContinue}:RunnerPairingCardProps){
 useLocale();const heading=useId(),confirmationHint=useId(),[shell,setShell]=useState<''|'powershell'|'bash'>(''),[identityPath,setIdentityPath]=useState(''),[confirmed,setConfirmed]=useState(''),[copied,setCopied]=useState(''),[copyFailed,setCopyFailed]=useState('');
 const scope=JSON.stringify([pairing?.id,pairing?.project_id,pairing?.version,pairing?.state,pairing?.credential_sha256,pairing?.control_url,pairing?.reason_code,pairing?.can_approve]),blocked=disabled||loading||stale||pending||!!error;
 useEffect(()=>{setConfirmed('');},[scope,loading,stale,pending,!!error]);
 const proposed=pairing?.state==='proposed',approved=pairing?.state==='approved',terminal=pairing?.state==='rejected'||pairing?.state==='expired';
 const reason=pairing?.reason_code,reasonKey=reason&&['awaiting_machine','approval_required','control_changed','identity_approved','pairing_rejected','pairing_expired'].includes(reason)?'reason.'+reason:undefined;
 const command=pairing&&shell&&(pairing.state==='waiting'||proposed)?runnerPairingCommand(pairing,shell,identityPath):undefined;
 const canApprove=!!pairing&&proposed&&pairing.can_approve&&!blocked&&confirmed===scope;
 async function copy(){if(!command||blocked)return;try{await navigator.clipboard.writeText(command);setCopied(command);setCopyFailed('');}catch{setCopyFailed(command);}}
 const machine=pairing&&(pairing.device_name||pairing.credential_sha256)?<div className="runner-pairing-device"><div><strong>{t('device_name')}</strong><p data-runner-pairing-device>{pairing.device_name||t('not_reported')}</p></div><div><strong>{t('control_url')}</strong><code data-runner-pairing-control>{pairing.control_url}</code></div><div><strong><Fingerprint size={16} aria-hidden="true"/>{t('fingerprint')}</strong><code data-runner-pairing-fingerprint>{pairing.credential_sha256||t('not_reported')}</code></div><p className="runner-pairing-help">{t('fingerprint_help')}</p></div>:undefined;
 const commandEditor=pairing&&(pairing.state==='waiting'||proposed)&&!pending?<div className="runner-pairing-command">
  <label>{t('shell')}<select data-runner-pairing-shell value={shell} disabled={blocked} onChange={event=>{setShell(event.target.value as typeof shell);setCopyFailed('');}}><option value="">{t('choose_shell')}</option><option value="powershell">PowerShell</option><option value="bash">Bash</option></select></label>
  <label>{t('identity_path')}<input data-runner-pairing-identity value={identityPath} disabled={blocked} maxLength={4096} autoComplete="off" spellCheck={false} onChange={event=>{setIdentityPath(event.target.value);setCopyFailed('');}}/></label><p className="runner-pairing-help">{t('identity_help')}</p>
  <div className="runner-pairing-command-heading"><strong>{t('command')}</strong><button type="button" data-runner-pairing-copy disabled={blocked||!command} onClick={()=>void copy()}>{command&&copied===command?<Check size={15} aria-hidden="true"/>:<Copy size={15} aria-hidden="true"/>}{t(command&&copied===command?'copied':'copy')}</button></div>
  {command?<pre tabIndex={0} data-runner-pairing-command aria-label={t('command')}>{command}</pre>:<p className="runner-pairing-help" role="status">{t('command_incomplete')}</p>}
  {command&&copyFailed===command&&<p role="alert">{t('copy_failed')}</p>}
 </div>:undefined;
 return <section className="runner-pairing-card" data-runner-pairing-card data-runner-pairing-state={pairing?.state||'empty'} aria-labelledby={heading} aria-busy={loading}>
  <header className="runner-pairing-heading"><h3 id={heading}><ShieldCheck size={19} aria-hidden="true"/>{t('title')}</h3>{pairing&&<span className="runner-pairing-state">{t('state.'+pairing.state)}</span>}</header>
  {!pairing&&<p>{t('intro')}</p>}
  {error!==undefined&&!!error&&<ErrorNotice error={error}/>}
  {loading&&<p className="runner-pairing-notice" role="status">{t('loading')}</p>}
  {!loading&&stale&&<p className="runner-pairing-notice" role="status">{t('stale')}</p>}
  {pending&&<p className="runner-pairing-notice" role="status">{t('pending')}</p>}
  {pairing&&<>
   <p className="runner-pairing-help">{t(reasonKey||'state_hint.'+pairing.state)}</p>
   {proposed&&machine}
   {pairing.state==='waiting'&&commandEditor}
   {proposed&&!pending&&<label className="runner-pairing-confirm"><input type="checkbox" data-runner-pairing-confirm checked={confirmed===scope} disabled={blocked||!pairing.can_approve} aria-describedby={confirmationHint} onChange={event=>setConfirmed(event.target.checked?scope:'')}/><span id={confirmationHint}>{t('confirm')}</span></label>}
   {approved&&pairing.worker_id&&<dl className="runner-pairing-result"><dt>{t('worker')}</dt><dd><code data-runner-pairing-worker>{pairing.worker_id}</code></dd></dl>}
  </>}
  <div className="runner-pairing-actions">
   {pending?onRestore&&<button type="button" className="primary" data-runner-pairing-restore disabled={disabled||loading} onClick={onRestore}>{t('restore')}</button>:<>
    {(!pairing||terminal)&&<button type="button" className="primary" data-runner-pairing-create disabled={blocked} onClick={onCreate}>{t(pairing?'create_another':'create')}</button>}
    {proposed&&<button type="button" className="primary" data-runner-pairing-approve disabled={!canApprove} onClick={()=>{if(canApprove){setConfirmed('');onApprove();}}}>{t('approve')}</button>}
    {pairing&&(pairing.state==='waiting'||proposed)&&<button type="button" data-runner-pairing-reject disabled={blocked} onClick={onReject}>{t('reject')}</button>}
    {pairing&&approved&&pairing.worker_id&&onContinue&&<button type="button" className="primary" data-runner-pairing-continue disabled={blocked} onClick={onContinue}>{t('continue')}</button>}
    {pairing&&<button type="button" data-runner-pairing-refresh disabled={disabled||loading} onClick={onRefresh}><RefreshCw size={15} aria-hidden="true"/>{t('refresh')}</button>}
    {!pairing&&!!error&&<button type="button" data-runner-pairing-refresh disabled={disabled||loading} onClick={onRefresh}>{t('refresh')}</button>}
   </>}
  </div>
  {pairing&&<details className="runner-pairing-disclosure" data-runner-pairing-evidence key={scope}>
   <summary>{t('evidence')}</summary>
   <dl className="runner-pairing-facts"><div><dt>{t('project')}</dt><dd><code>{pairing.project_id}</code></dd></div><div><dt>{t('locator')}</dt><dd><code>{pairing.id}</code></dd></div><div><dt>{t('version')}</dt><dd>{formatNumber(pairing.version)}</dd></div><div><dt>{t('expires_at')}</dt><dd><time dateTime={pairing.expires_at} title={pairing.expires_at}>{stamp(pairing.expires_at)}</time></dd></div>{!machine&&<div className="runner-pairing-full"><dt>{t('control_url')}</dt><dd><code data-runner-pairing-control>{pairing.control_url}</code></dd></div>}{pairing.credential_expires_at&&<div className="runner-pairing-full"><dt>{t('credential_expires_at')}</dt><dd><time dateTime={pairing.credential_expires_at} title={pairing.credential_expires_at}>{stamp(pairing.credential_expires_at)}</time></dd></div>}</dl>
   {!proposed&&machine}
  </details>}
  {proposed&&commandEditor&&<details className="runner-pairing-disclosure" data-runner-pairing-rerun key={'command:'+scope}><summary>{t('rerun_command')}</summary>{commandEditor}</details>}
 </section>;
}
