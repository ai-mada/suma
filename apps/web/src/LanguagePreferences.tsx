import {useCallback,useEffect,useLayoutEffect,useMemo,useRef,useState} from 'react';
import {Languages,ShieldCheck,UserRound} from 'lucide-react';
import type {AccountPreferences,OrganizationPreferences} from '../../../packages/contracts/src';
import {api,ApiError,type Session} from './api';
import {ErrorNotice,Field} from './ui';
import {useLeaveGuard} from './write-action';
import {applySavedLocaleDefaults,followLocaleDefaults,getLocaleSource,formatNumber,tr as t,UiError,useLocale} from './i18n';
import {LocalePreferenceRecovery} from './i18n/preference-write';
import './language-preferences.css';

type Target='account'|'organization';
type Preference=AccountPreferences|OrganizationPreferences;
type PreferenceLocale=''|'zh-CN'|'en-US';
type Resource={state:'loading'|'ready'|'error';record?:Preference;error?:unknown};
type Snapshot={scope:string;account:Resource;organization:Resource};
const initial=(scope:string):Snapshot=>({scope,account:{state:'loading'},organization:{state:'loading'}});
function preferenceTimestamp(value:string){
 const match=/^(\d{4}-\d{2}-\d{2})T(?:[01]\d|2[0-3]):[0-5]\d:[0-5]\d(?:\.\d{1,9})?(?:Z|[+-](?:[01]\d|2[0-3]):[0-5]\d)$/.exec(value);
 if(!match||!Number.isFinite(Date.parse(value)))return false;
 const day=new Date(match[1]+'T00:00:00Z');
 return Number.isFinite(day.getTime())&&day.toISOString().slice(0,10)===match[1];
}
/** A missing or foreign response is unknown, never a synthetic saved v0. */
export function decodeLocalePreference(value:unknown,target:Target,id:string):Preference{
 const record=value&&typeof value==='object'&&!Array.isArray(value)?value as Record<string,unknown>:undefined;
 if(!record||record[target==='account'?'user_id':'organization_id']!==id||typeof record.locale!=='string'||!['','zh-CN','en-US'].includes(record.locale)||typeof record.version!=='number'||!Number.isSafeInteger(record.version)||record.version<0||record.version>2147483647||typeof record.updated_at!=='string')throw new UiError('preferences.protocol');
 if(record.version===0?record.locale!==''||record.updated_at!=='':!preferenceTimestamp(record.updated_at))throw new UiError('preferences.protocol');
 return {...record} as unknown as Preference;
}
type Context={scope:string;active:boolean;reads:Record<Target,number>;snapshot:Snapshot;observed:Partial<Record<Target,Preference>>};
const samePreference=(left:Preference,right:Preference)=>left.version===right.version&&left.locale===right.locale&&left.updated_at===right.updated_at;
function observePreference(ctx:Context,target:Target,record:Preference,historical=false){
 const previous=ctx.observed[target];
 if(previous&&record.version<previous.version){if(historical)return;throw new UiError('preferences.protocol');}
 if(previous&&record.version===previous.version&&!samePreference(previous,record))throw new UiError('preferences.protocol');
 ctx.observed[target]=record;
}
export function useLocalePreferenceScope(session:Session|null,refresh=0){
 const user=session?.user.id||'',organization=session?.organization.id||'',scope=session?user+':'+organization:'';
 const [snapshot,setSnapshot]=useState<Snapshot>(()=>initial(scope));
 const currentScope=useRef(scope);currentScope.current=scope;
 const context=useRef<Context>({scope:'',active:false,reads:{account:0,organization:0},snapshot:initial(''),observed:{}});
 const publish=useCallback((ctx:Context,next:Snapshot)=>{
  if(!ctx.active||context.current!==ctx||currentScope.current!==ctx.scope)return false;
  ctx.snapshot=next;setSnapshot(next);
  applySavedLocaleDefaults(next.account.state==='ready'?next.account.record?.locale:undefined,next.organization.state==='ready'?next.organization.record?.locale:undefined);
  return true;
 },[]);
 const read=useCallback(async(target:Target)=>{
  const ctx=context.current;if(!session||!ctx.active||ctx.scope!==scope||currentScope.current!==scope)return;
  const epoch=++ctx.reads[target];publish(ctx,{...ctx.snapshot,[target]:{state:'loading'}});
  try{
   const record=decodeLocalePreference(await api<unknown>('/'+target+'/preferences'),target,target==='account'?user:organization);
   if(ctx.reads[target]!==epoch||!ctx.active||context.current!==ctx||currentScope.current!==ctx.scope)return;
   observePreference(ctx,target,record);
   if(!publish(ctx,{...ctx.snapshot,[target]:{state:'ready',record}}))return;
   return record;
  }catch(error){if(ctx.reads[target]===epoch)publish(ctx,{...ctx.snapshot,[target]:{state:'error',error}});}
 },[scope,user,organization,publish]);
 useLayoutEffect(()=>{
  const ctx:Context={scope,active:true,reads:{account:0,organization:0},snapshot:initial(scope),observed:{}};context.current=ctx;
  setSnapshot(ctx.snapshot);applySavedLocaleDefaults();
  if(session){void read('account');void read('organization');}
  return()=>{ctx.active=false;};
 },[scope,read]);
 const previousRefresh=useRef(refresh);
 useEffect(()=>{if(previousRefresh.current===refresh)return;previousRefresh.current=refresh;if(session){void read('account');void read('organization');}},[refresh,read,scope]);
 const checkReceipt=useCallback((target:Target,record:Preference)=>{
  const ctx=context.current;if(ctx.scope!==scope||!ctx.active||currentScope.current!==scope)return;
  // An operation receipt may be historical. It establishes a version/content
  // constraint for later reads, but never supplies the current display locale.
  observePreference(ctx,target,record,true);
 },[scope]);
 return {snapshot:snapshot.scope===scope?snapshot:initial(scope),read,checkReceipt};
}
type Controller=ReturnType<typeof useLocalePreferenceScope>;
function recoveryStorage():Pick<Storage,'getItem'|'setItem'|'removeItem'>{
 try{return window.sessionStorage;}catch{
  const denied=()=>{throw new Error('sessionStorage unavailable');};
  return {getItem:denied,setItem:denied,removeItem:denied};
 }
}
function PreferenceCard({target,session,resource,controller}:{target:Target;session:Session;resource:Resource;controller:Controller}){
 useLocale();
 const id=target==='account'?session.user.id:session.organization.id,scope=target==='account'?'account:'+session.user.id:'organization:'+session.user.id+':'+session.organization.id;
 const recovery=useMemo(()=>new LocalePreferenceRecovery(scope,target,recoveryStorage()),[scope,target]);
 const [pending,setPending]=useState(recovery.pending),[draft,setDraft]=useState<PreferenceLocale>(()=>recovery.pending?.locale||''),[base,setBase]=useState<{locale:PreferenceLocale;version:number}>(),[busy,setBusy]=useState(false),[reloading,setReloading]=useState(false),[error,setError]=useState<unknown>(),[conflict,setConflict]=useState(false),[saved,setSaved]=useState(false),[receipt,setReceipt]=useState<Preference>();
 const mounted=useRef(true);useEffect(()=>{mounted.current=true;return()=>{mounted.current=false;};},[]);
 const dirty=!!base&&draft!==base.locale,writable=target==='account'||['owner','admin'].includes(session.user.role||'');
 useLeaveGuard(dirty||busy||!!pending,setError);
 useEffect(()=>{
  if(resource.state!=='ready'||!resource.record||busy||pending||dirty)return;
  setBase({locale:resource.record.locale as PreferenceLocale,version:resource.record.version});setDraft(resource.record.locale as PreferenceLocale);
 },[resource,busy,pending,dirty]);
 async function save(restore=false){
  if(busy||reloading||recovery.error||!restore&&(!writable||!base||resource.state!=='ready'||conflict))return;
  const request=restore?recovery.pending:{locale:draft,expected_version:base!.version};
  setBusy(true);setError(undefined);setSaved(false);if(!restore)setReceipt(undefined);
  try{
   const decode=(value:unknown)=>{
    const record=decodeLocalePreference(value,target,id);
    if(!request||record.locale!==request.locale||record.version!==request.expected_version+1)throw new UiError('preferences.protocol');
    controller.checkReceipt(target,record);return record;
   };
   const record=await (restore?recovery.restore(decode):recovery.run(request!.locale,request!.expected_version,decode));
   if(!mounted.current)return;
   setReceipt(record);setConflict(false);
   // Confirming an operation is distinct from reading the current preference:
   // another write may already have advanced it beyond this exact receipt.
   const current=await controller.read(target);
   if(!mounted.current)return;
   if(current){setBase({locale:current.locale as PreferenceLocale,version:current.version});setDraft(current.locale as PreferenceLocale);setSaved(samePreference(current,record));}
   else setBase(undefined);
  }catch(caught){if(mounted.current){setConflict(!recovery.pending&&caught instanceof ApiError&&caught.status===409);setError(caught);}}
  finally{if(mounted.current){setPending(recovery.pending);setBusy(false);}}
 }
 async function reload(){
  const preserveDraft=dirty;
  setReloading(true);setSaved(false);
  try{const record=await controller.read(target);if(record&&mounted.current){setBase({locale:record.locale as PreferenceLocale,version:record.version});if(!preserveDraft)setDraft(record.locale as PreferenceLocale);setConflict(false);setError(undefined);}}
  finally{if(mounted.current)setReloading(false);}
 }
 const locked=busy||!!pending||!!recovery.error||!writable||resource.state!=='ready'||!base;
 const currentSaved=Boolean(saved&&resource.state==='ready'&&resource.record&&receipt&&samePreference(resource.record,receipt));
 const heading=target==='account'?'preferences.account':'preferences.organization';
 return <section className="panel language-preference-card" data-preference-target={target} aria-labelledby={'language-'+target+'-heading'}>
  <div className="panel-heading"><div><h2 id={'language-'+target+'-heading'}>{t(heading)}</h2><p>{t(target==='account'?'preferences.accountHint':'preferences.organizationHint')}</p></div>{target==='account'?<UserRound size={20}/>:<ShieldCheck size={20}/>}</div>
  <div className="language-preference-content">
   <p className="muted">{target==='account'?session.user.name:session.organization.name}</p>
   {resource.state==='loading'&&<p role="status">{t('preferences.loading')}</p>}
   {resource.state==='error'&&<><p>{t('preferences.readFailed')}</p><ErrorNotice error={resource.error}/><button type="button" disabled={busy||reloading} onClick={()=>void reload()}>{t('preferences.retry')}</button></>}
   {resource.state==='ready'&&resource.record&&<small>{t('preferences.version',{version:formatNumber(resource.record.version)})}</small>}
   {!writable&&<p className="muted">{t('preferences.readOnly')}</p>}
   {Boolean(error||recovery.error)&&<ErrorNotice error={error||recovery.error}/>}
   {conflict&&<p className="warning" role="status">{t('preferences.conflict')}</p>}
   {pending&&<div className="language-preference-recovery"><p role="status">{t(receipt?'preferences.confirmedRecoveryHelp':'preferences.pending')}</p><small>{t('preferences.originalScope',{scope})}</small><p>{t('preferences.originalRequest',{locale:t(pending.locale==='zh-CN'?'preferences.locale.zh':pending.locale==='en-US'?'preferences.locale.en':'preferences.inherit'),version:formatNumber(pending.expected_version)})}</p><button type="button" className="primary" disabled={busy||!!recovery.error} onClick={()=>void save(true)}>{t(busy?'preferences.saving':'preferences.restore')}</button></div>}
   <form onSubmit={event=>{event.preventDefault();void save();}}>
    <Field label={t('preferences.locale')}><select name="locale" aria-label={t(target==='account'?'preferences.accountLocale':'preferences.organizationLocale')} value={pending?.locale??draft} disabled={locked} onChange={event=>{setDraft(event.target.value as PreferenceLocale);setSaved(false);setError(undefined);}}><option value="">{t('preferences.inherit')}</option><option value="zh-CN">{t('preferences.locale.zh')}</option><option value="en-US">{t('preferences.locale.en')}</option></select></Field>
    {dirty&&!pending&&<p className="muted" role="status">{t('preferences.unsaved')}</p>}
    {receipt&&!currentSaved&&<p className="success" role="status">{t('preferences.receiptConfirmed',{version:formatNumber(receipt.version)})}</p>}
    {currentSaved&&<p className="success" role="status">{t('preferences.saved')}</p>}
    <div className="language-preference-actions">
     {writable&&<button className="primary" disabled={locked||conflict||reloading||!dirty}>{t(busy?'preferences.saving':target==='account'?'preferences.saveAccount':'preferences.saveOrganization')}</button>}
     {(dirty||pending)&&<button type="button" disabled={busy||!!pending||reloading} onClick={()=>{if(base)setDraft(base.locale);setError(undefined);setSaved(false);}}>{t('preferences.cancel')}</button>}
     {resource.state==='ready'&&<button type="button" disabled={busy||!!pending||reloading} onClick={()=>void reload()}>{t('preferences.reload')}</button>}
    </div>
   </form>
  </div>
 </section>;
}
export function LanguagePreferences({session,controller}:{session:Session;controller:Controller}){
 const locale=useLocale(),[error,setError]=useState<unknown>();
 return <div className="language-preferences">
  <div className="info-strip"><Languages size={20}/><div><h2>{t('preferences.title')}</h2><p>{t('preferences.description')}</p></div></div>
  <section className="panel language-preference-effective"><div><strong>{t('preferences.effective',{locale:t(locale==='zh-CN'?'preferences.locale.zh':'preferences.locale.en'),source:t('preferences.source.'+getLocaleSource())})}</strong><p className="muted">{t('preferences.quickHint')}</p>{Boolean(error)&&<ErrorNotice error={error}/>}</div><button type="button" onClick={()=>{setError(followLocaleDefaults()?undefined:new UiError('preferences.storageFailed'));}}>{t('preferences.follow')}</button></section>
  {/* Views follow the verified organization; account recovery remains user-wide. */}
  <div className="language-preference-grid"><PreferenceCard key={'account:'+session.user.id+':'+session.organization.id} target="account" session={session} resource={controller.snapshot.account} controller={controller}/><PreferenceCard key={'organization:'+session.user.id+':'+session.organization.id} target="organization" session={session} resource={controller.snapshot.organization} controller={controller}/></div>
 </div>;
}
