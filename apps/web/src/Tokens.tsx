import {useRef,useState,type FormEvent} from 'react';
import {Copy,KeyRound,Plus} from 'lucide-react';
import {time} from './api';
import {Field,ErrorNotice,Empty} from './ui';
import {tr as t,useLocale,UiError,formatNumber} from './i18n';
import {TokenCreation,type TokenScope,type ScopedToken} from './token-requests';
const scopes:TokenScope[]=['runner','agent','api'];
const scopeTitle=(scope:TokenScope)=>t('workspace.token.scope_'+scope);
type UnknownReceipt={operation_id:string;scope:TokenScope;label:string};
type Props={project:string;projectName:string;admin:boolean};
export function Tokens(props:Props){
 useLocale();
 return props.project?<ProjectTokens key={props.project} {...props}/>:<Empty title={t('workspace.select_project')}/>;
}
function ProjectTokens({project,projectName,admin}:Props){
 useLocale();
 const creation=useRef(new TokenCreation()),storageKey='suma-token-unknown:'+project;
 const [unknown,setUnknown]=useState<UnknownReceipt|undefined>(()=>{
  try{const saved=sessionStorage.getItem(storageKey);if(!saved)return;const receipt=JSON.parse(saved);return typeof receipt.operation_id==='string'&&typeof receipt.label==='string'&&scopes.includes(receipt.scope)?receipt:undefined;}catch{return undefined;}
 });
 const [scope,setScope]=useState<TokenScope>(unknown?.scope||'runner'),[result,setResult]=useState<ScopedToken>(),[error,setError]=useState<unknown>(),[busy,setBusy]=useState(false),[copied,setCopied]=useState(false),[receiptPersisted,setReceiptPersisted]=useState(!!unknown);
 async function submit(e:FormEvent<HTMLFormElement>){
  e.preventDefault();if(busy||unknown||!admin)return;
  const label=String(new FormData(e.currentTarget).get('label')||'');setBusy(true);setError(undefined);setResult(undefined);setCopied(false);
  try{
   const r=await creation.current.create(project,label,scope);setResult(r);setCopied(false);
   try{sessionStorage.removeItem(storageKey);}catch{/* The one-time secret stays only in this mounted view. */}
  }catch(err){
   if(creation.current.unknown){
    const receipt={operation_id:creation.current.operationID||'',scope,label};setUnknown(receipt);
    try{sessionStorage.setItem(storageKey,JSON.stringify(receipt));setReceiptPersisted(true);}catch{setReceiptPersisted(false);}
    setError(new UiError('workspace.token.unknown'));
   }else setError(err);
  }finally{setBusy(false);}
 }
 return <section className="panel settings-panel"><div className="panel-heading"><div><h2>{t('workspace.token.title')}</h2><p>{t('workspace.token.description',{hours:formatNumber(24)})}</p></div><KeyRound size={22}/></div>
  {!admin&&<div className="warning">{t('workspace.token.admin_only')}</div>}{Boolean(error)&&<ErrorNotice error={error}/>}
  <form onSubmit={e=>void submit(e)}><Field label={t('workspace.token.bound_project')}><input readOnly value={projectName}/><small><code>{project}</code></small></Field><Field label={t('workspace.token.label')}><input name="label" placeholder={t('workspace.token.label_placeholder')} required maxLength={100} disabled={busy||!!unknown||!admin}/></Field><Field label={t('workspace.token.scope')} hint={t('workspace.token.scope_'+scope+'_help')}><select value={scope} onChange={e=>setScope(e.target.value as TokenScope)} disabled={busy||!!unknown||!admin}>{scopes.map(value=><option key={value} value={value}>{scopeTitle(value)}</option>)}</select></Field><button className="primary" disabled={busy||!!unknown||!admin}><Plus size={15}/>{t(busy?'workspace.token.creating':unknown?'workspace.token.waiting':'workspace.token.create')}</button></form>
  {unknown&&<div className="warning"><strong>{t('workspace.token.operation')}</strong><p><code>{unknown.operation_id}</code></p><p>{unknown.label} · {scopeTitle(unknown.scope)}</p><p>{t(receiptPersisted?'workspace.token.receipt_preserved':'workspace.token.receipt_not_persisted')}</p></div>}
  {result&&<div className="token-result"><p>{t('workspace.token.created',{at:time(result.expires_at)})}</p><p>{t('workspace.token.result_scope',{scope:scopes.includes(result.scope)?scopeTitle(result.scope):result.scope,project:result.project_id})}</p><code>{result.token}</code><button onClick={async()=>{try{await navigator.clipboard.writeText(result.token);setCopied(true);}catch{setError(new UiError('workspace.token.clipboard_failed'));}}}><Copy size={15}/>{t(copied?'workspace.token.copied':'workspace.token.copy')}</button></div>}
  <div className="detail-section"><h3>{t('workspace.token.connection_title')}</h3><p className="muted">{t('workspace.token.'+scope+'_instructions')}</p></div>
 </section>;
}
