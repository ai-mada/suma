import {useCallback,useEffect,useRef,useState} from 'react';
import {ChevronDown} from 'lucide-react';
import type {ToolProfile,ToolUsage,UsageAttempt,UsageAttemptPage,UsageBalance,UsageDashboard,UsagePolicy,UsageRateRevision} from '../../../packages/contracts/src';
import {ApiError,query,type ApiCaller,type Session} from './api';
import {useApi} from './api-context';
import {formatDateTime,formatNumber,tr,UiError,useLocale} from './i18n';
import {toolError,toolSessionStorage} from './tool-hooks';
import {ErrorNotice,Field,Loading} from './ui';
import {useWriteAction,WriteButton,WriteFields,WriteNotice,type WriteRecoveryRequest} from './write-action';
import './UsageWorkspace.css';

type Props={session:Session;project:string;refresh:number};
type Page<T>={items:T[];total:number;next_cursor?:string};
type PolicyDraft={scope:string;version:number;mode:string;currency:string;tokenUnlimited:boolean;costUnlimited:boolean;tokens:string;cost:string;reason:string;ack:boolean};
type RateDraft={profile:string;toolVersion:number;version?:number;currency:string;input:string;output:string;inputReservation:string;outputReservation:string;reason:string};
type ConfirmDraft={item:UsageAttempt;values:Record<keyof ToolUsage,string>;source:string;reason:string;ack:boolean};
const t=(key:string,values?:Record<string,unknown>)=>tr('usage-accounting.'+key,values);
const counters=['input_tokens','output_tokens','cached_input_tokens','cache_write_input_tokens','reasoning_output_tokens'] as const;
const states=['reserved','settled','partial','unreported','reconciling','confirmed','unpriced'];
const validInt=(value:unknown):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>=0;
const integer=(value:string)=>/^(0|[1-9]\d*)$/.test(value)&&validInt(Number(value))?Number(value):undefined;
const validID=(value:unknown):value is string=>typeof value==='string'&&!!value&&value.length<=64&&!/[\x00-\x1f\x7f]/.test(value);
const bounded=(value:unknown,max=4000):value is string=>typeof value==='string'&&!!value.trim()&&new TextEncoder().encode(value).length<=max;
const currency=(value:unknown)=>value==='CNY'||value==='USD';
const amount=(raw:string)=>{
 if(!/^(0|[1-9]\d*)(\.\d{1,6})?$/.test(raw))return;
 const [whole,fraction='']=raw.split('.'),value=BigInt(whole)*1000000n+BigInt(fraction.padEnd(6,'0'));
 return value<=BigInt(Number.MAX_SAFE_INTEGER)?Number(value):undefined;
};
const decimal=(value:number)=>{const raw=BigInt(value),fraction=String(raw%1000000n).padStart(6,'0').replace(/0+$/,'');return String(raw/1000000n)+(fraction?'.'+fraction:'');};
function money(value:unknown,unit:string,locale:string){
 if(!validInt(value)||!currency(unit))return t('unknown');
 const [whole,fraction]=decimal(value).split('.'),separator=new Intl.NumberFormat(locale).formatToParts(1.1).find(part=>part.type==='decimal')?.value||'.';
 return unit+' '+formatNumber(Number(whole))+(fraction?separator+fraction:'');
}
const count=(value:unknown)=>validInt(value)?formatNumber(value):t('unknown');
const validUsage=(value:unknown)=>{
 if(!value||typeof value!=='object'||Array.isArray(value))return false;
 const record=value as Record<string,unknown>;
 return Object.keys(record).every(key=>counters.includes(key as keyof ToolUsage)&&validInt(record[key]))&&validInt(record.input_tokens)&&validInt(record.output_tokens);
};
function recoveryValid(request:WriteRecoveryRequest,project:string){
 const b=request.body,keys=Object.keys(b);if(request.method!=='POST'||b.project_id!==project||!validInt(b.expected_version)||!bounded(b.reason))return false;
 if(request.path==='/usage/policies')return keys.every(key=>['project_id','scope','expected_version','mode','currency','token_limit','cost_limit_micros','reason','acknowledge_enforcement','idempotency_key'].includes(key))&&['project','organization'].includes(String(b.scope))&&['observe','enforce'].includes(String(b.mode))&&currency(b.currency)&&(b.token_limit===null||validInt(b.token_limit))&&(b.cost_limit_micros===null||validInt(b.cost_limit_micros))&&typeof b.acknowledge_enforcement==='boolean'&&(b.mode!=='enforce'||b.acknowledge_enforcement===true);
 if(request.path==='/usage/rates')return keys.every(key=>['project_id','tool_profile_id','expected_tool_version','expected_version','currency','input_rate_micros_per_million','output_rate_micros_per_million','input_token_reservation','output_token_reservation','reason','idempotency_key'].includes(key))&&validID(b.tool_profile_id)&&validInt(b.expected_tool_version)&&b.expected_tool_version>0&&currency(b.currency)&&['input_rate_micros_per_million','output_rate_micros_per_million','input_token_reservation','output_token_reservation'].every(key=>validInt(b[key]))&&Number(b.output_token_reservation)>=4096;
 const match=request.path.match(/^\/usage\/attempts\/([^/]+)\/confirm$/);try{return !!match&&validID(decodeURIComponent(match[1]))&&b.expected_version>0&&keys.every(key=>['project_id','expected_version','usage','source_ref','reason','acknowledge_usage','idempotency_key'].includes(key))&&validUsage(b.usage)&&bounded(b.source_ref,2000)&&b.acknowledge_usage===true;}catch{return false;}
}
function useRead<T>(api:ApiCaller,path:string,enabled:boolean,refresh:number,validate:(value:T)=>boolean,onError:(error:unknown)=>void){
 const [state,setState]=useState<{path:string;value?:T;loading:boolean;error?:unknown}>({path,loading:enabled}),epoch=useRef(0),controller=useRef<AbortController|undefined>(undefined);
 const read=useCallback(async()=>{
  if(!enabled)return;const generation=++epoch.current;controller.current?.abort();const abort=new AbortController();controller.current=abort;
  // A same-view refresh must not briefly revoke dependent reads. Other paths
  // never reuse the value, and account/organization/project changes remount.
  setState(previous=>({path,value:previous.path===path?previous.value:undefined,loading:true}));
  try{const value=await api<T>(path,undefined,'GET',undefined,abort.signal);if(abort.signal.aborted||generation!==epoch.current)return;if(!validate(value))throw new UiError('usage-accounting.invalid_read');setState({path,value,loading:false});return value;}
  catch(error){if(abort.signal.aborted||generation!==epoch.current)return;setState({path,error,loading:false});onError(error);}
 },[api,path,enabled,validate,onError]);
 useEffect(()=>{if(enabled)void read();return()=>{epoch.current++;controller.current?.abort();};},[read,enabled,refresh]);
 return {...(state.path===path?state:{path,loading:enabled}),read};
}
function useAttempts(api:ApiCaller,project:string,path:string,refresh:number,onError:(error:unknown)=>void){
 const [state,setState]=useState<{path:string;page?:UsageAttemptPage;loading:boolean;error?:unknown}>({path,loading:true}),current=useRef<UsageAttemptPage|undefined>(undefined),epoch=useRef(0),controller=useRef<AbortController|undefined>(undefined);
 const read=useCallback(async(more=false)=>{
  const previous=current.current,cursor=more?previous?.next_cursor:'';if(more&&!cursor)return;
  const generation=++epoch.current;controller.current?.abort();const abort=new AbortController();controller.current=abort;if(!more)current.current=undefined;setState({path,page:more?previous:undefined,loading:true});
  try{const page=await api<UsageAttemptPage>(path+(cursor?'&before='+encodeURIComponent(cursor):''),undefined,'GET',undefined,abort.signal);if(abort.signal.aborted||generation!==epoch.current)return;
   if(!Array.isArray(page.items)||page.items.length>20||!validInt(page.total)||page.total<page.items.length||typeof page.next_cursor!=='string'||page.items.some(item=>item.project_id!==project||!validID(item.id)||!validInt(item.version)||!Array.isArray(item.allowed_actions)))throw new UiError('usage-accounting.invalid_read');
   const value={...page,items:more?[...new Map([...(previous?.items||[]),...page.items].map(item=>[item.id,item])).values()]:page.items};if(value.items.length>value.total)throw new UiError('usage-accounting.invalid_read');current.current=value;setState({path,page:value,loading:false});return value;
  }catch(error){if(abort.signal.aborted||generation!==epoch.current)return;current.current=undefined;setState({path,error,loading:false});onError(error);}
 },[api,project,path,onError]);
 useEffect(()=>{current.current=undefined;void read();return()=>{epoch.current++;controller.current?.abort();};},[read,refresh]);
 return {...(state.path===path?state:{path,loading:true}),read};
}
const planner=(item:ToolProfile)=>item.state==='enabled'&&item.profile.adapter==='suma'&&item.profile.read_only&&item.profile.planning?.protocol==='suma-schedule-plan-v1'&&!!item.profile.planning.model&&!!item.profile.planning.endpoint_origin;
const policyDraft=(p:UsagePolicy):PolicyDraft=>({scope:p.scope,version:p.version,mode:p.mode,currency:p.currency,tokenUnlimited:p.token_limit===null,costUnlimited:p.cost_limit_micros===null,tokens:validInt(p.token_limit)?String(p.token_limit):'',cost:validInt(p.cost_limit_micros)?decimal(p.cost_limit_micros):'',reason:'',ack:false});
const rateDraft=(profile:ToolProfile,rates:UsageRateRevision[],total:number):RateDraft=>{const current=rates.find(rate=>rate.tool_profile_id===profile.id);return {profile:profile.id,toolVersion:profile.version,version:current?.version??(rates.length===total?0:undefined),currency:current?.currency||'CNY',input:current?decimal(current.input_rate_micros_per_million):'',output:current?decimal(current.output_rate_micros_per_million):'',inputReservation:current?String(current.input_token_reservation):'',outputReservation:current?String(current.output_token_reservation):'4096',reason:''};};

export function UsageWorkspace(props:Props){return <UsageView key={JSON.stringify([props.session.user.id,props.session.organization.id,props.project])} {...props}/>;}
function UsageView({session,project,refresh}:Props){
 const locale=useLocale(),api=useApi(),[month,setMonth]=useState(()=>new Date().toISOString().slice(0,7)),[filter,setFilter]=useState(''),[revision,setRevision]=useState(0),[forbidden,setForbidden]=useState<unknown>(),[notice,setNotice]=useState(false),alive=useRef(true);
 const [policy,setPolicy]=useState<PolicyDraft>(),[rateOpen,setRateOpen]=useState(false),[rate,setRate]=useState<RateDraft>(),[confirm,setConfirm]=useState<ConfirmDraft>();
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;};},[]);
 const onError=useCallback((error:unknown)=>{if(error instanceof ApiError&&[401,403].includes(error.status))setForbidden(error);},[]);
 const validateDashboard=useCallback((value:UsageDashboard)=>value.project_id===project&&!!value.project_policy&&value.project_policy.scope==='project'&&value.project_policy.project_id===project&&Array.isArray(value.balances)&&Array.isArray(value.rates)&&value.rates.length<=100&&validInt(value.rate_total)&&value.rate_total>=value.rates.length&&value.rates.every(rate=>rate.project_id===project)&&typeof value.can_manage_project==='boolean'&&typeof value.can_manage_organization==='boolean',[project]);
 const dashboard=useRead<UsageDashboard>(api,'/usage'+query(project)+'&month='+month,true,refresh+revision,validateDashboard,onError),data=dashboard.value;
 const attempts=useAttempts(api,project,'/usage/attempts'+query(project)+'&month='+month+'&limit=20'+(filter?'&state='+filter:''),refresh+revision,onError);
 const validateProfiles=useCallback((value:Page<ToolProfile>)=>Array.isArray(value.items)&&value.items.length<=100&&validInt(value.total)&&value.total>=value.items.length&&value.items.every(item=>item.project_id===project),[project]);
 const profiles=useRead<Page<ToolProfile>>(api,'/tool-profiles'+query(project)+'&limit=100',rateOpen&&!!data?.can_manage_rates,refresh+revision,validateProfiles,onError);
 const action=useWriteAction({scope:JSON.stringify(['usage-accounting',session.user.id,session.organization.id,project]),storage:toolSessionStorage(),validateRequest:request=>recoveryValid(request,project)}),locked=action.busy||!!action.pending||action.conflict||action.persistenceBlocked;
 const chosenProfile=profiles.value?.items.find(item=>item.id===rate?.profile&&planner(item));
 const policySource=data&&(policy?.scope==='organization'?data.organization_policy:data.project_policy),policyAllowed=!!policySource?.allowed_actions.includes('write')&&(policy?.scope==='organization'?data?.can_manage_organization:data?.can_manage_project);
 const tokens=policy?.tokenUnlimited?null:policy?integer(policy.tokens):undefined,cost=policy?.costUnlimited?null:policy?amount(policy.cost):undefined;
 const canPolicy=policyAllowed&&policy&&tokens!==undefined&&cost!==undefined&&bounded(policy.reason)&&(policy.mode!=='enforce'||policy.ack)&&!locked;
 const rateInput=rate?amount(rate.input):undefined,rateOutput=rate?amount(rate.output):undefined,reserveInput=rate?integer(rate.inputReservation):undefined,reserveOutput=rate?integer(rate.outputReservation):undefined;
 const canRate=data?.can_manage_rates&&rate&&chosenProfile&&rate.version!==undefined&&rateInput!==undefined&&rateOutput!==undefined&&reserveInput!==undefined&&reserveOutput!==undefined&&reserveOutput>=4096&&BigInt(reserveInput)+BigInt(reserveOutput)<=BigInt(Number.MAX_SAFE_INTEGER)&&bounded(rate.reason)&&!locked;
 const confirmedUsage:ToolUsage={};let usageValid=!!confirm;
 if(confirm)for(const key of counters){const raw=confirm.values[key],value=raw===''?undefined:integer(raw),known=confirm.item.usage?.[key];if(raw!==''&&value===undefined||['input_tokens','output_tokens'].includes(key)&&value===undefined||known!==undefined&&known!==null&&(value===undefined||value<known))usageValid=false;if(value!==undefined)confirmedUsage[key]=value;}
 const canConfirm=!!confirm&&confirm.item.allowed_actions.includes('confirm')&&usageValid&&bounded(confirm.source,2000)&&bounded(confirm.reason)&&confirm.ack&&!locked;
 async function completed(value:unknown){if(value===undefined||!alive.current)return;setNotice(true);setConfirm(undefined);setPolicy(undefined);setRate(undefined);setRevision(n=>n+1);}
 async function savePolicy(){if(!canPolicy||!policy)return;setNotice(false);await completed(await action.run<UsagePolicy>('/usage/policies',{project_id:project,scope:policy.scope,expected_version:policy.version,mode:policy.mode,currency:policy.currency,token_limit:tokens,cost_limit_micros:cost,reason:policy.reason,acknowledge_enforcement:policy.ack}));}
 async function saveRate(){if(!canRate||!rate)return;setNotice(false);await completed(await action.run<UsageRateRevision>('/usage/rates',{project_id:project,tool_profile_id:rate.profile,expected_tool_version:rate.toolVersion,expected_version:rate.version,currency:rate.currency,input_rate_micros_per_million:rateInput,output_rate_micros_per_million:rateOutput,input_token_reservation:reserveInput,output_token_reservation:reserveOutput,reason:rate.reason}));}
 async function saveConfirm(){if(!canConfirm||!confirm)return;setNotice(false);await completed(await action.run<UsageAttempt>('/usage/attempts/'+encodeURIComponent(confirm.item.id)+'/confirm',{project_id:project,expected_version:confirm.item.version,usage:confirmedUsage,source_ref:confirm.source,reason:confirm.reason,acknowledge_usage:true}));}
 async function rebase(){
  const [latest,page,tools]=await Promise.all([dashboard.read(),attempts.read(),rateOpen?profiles.read():Promise.resolve(undefined)]);if(!latest||!page)throw new UiError('usage-accounting.refresh_failed');
  if(policy){const next=policy.scope==='organization'?latest.organization_policy:latest.project_policy;if(!next||!next.allowed_actions.includes('write'))throw new UiError('usage-accounting.access_changed');setPolicy({...policy,version:next.version,ack:false});}
  if(rate){const profile=tools?.items.find(item=>item.id===rate.profile&&planner(item));if(!profile)throw new UiError('usage-accounting.refresh_failed');const next=rateDraft(profile,latest.rates,latest.rate_total);setRate({...rate,version:next.version,toolVersion:profile.version});}
  if(confirm){const item=page.items.find(item=>item.id===confirm.item.id);if(!item||!item.allowed_actions.includes('confirm'))throw new UiError('usage-accounting.confirm_changed');setConfirm({...confirm,item,ack:false});}
 }
 if(forbidden)return <section className="panel usage-workspace"><h2>{t('title')}</h2><ErrorNotice error={toolError(forbidden)} onRetry={()=>{setForbidden(undefined);setPolicy(undefined);setRate(undefined);setConfirm(undefined);setRevision(n=>n+1);}}/></section>;
 const profileOptions=profiles.value?.items.filter(planner)||[];
 const openConfirm=(item:UsageAttempt)=>setConfirm({item,values:Object.fromEntries(counters.map(key=>[key,validInt(item.usage?.[key])?String(item.usage?.[key]):''])) as ConfirmDraft['values'],source:'',reason:'',ack:false});
 return <section className="usage-workspace" aria-label={t('title')}>
  <div className="ua-heading"><div><h1>{t('title')}</h1><p className="ua-muted">{t('intro')}</p></div><Field label={t('month')}><input name="usage_month" type="month" value={month} disabled={locked} onChange={event=>{if(/^\d{4}-(0[1-9]|1[0-2])$/.test(event.target.value)){setMonth(event.target.value);setConfirm(undefined);setNotice(false);}}}/></Field></div>
  <WriteNotice action={action} onRebase={rebase}/>
  {action.pending&&<div className="ua-callout" role="status"><strong>{t('original_operation')}</strong><p>{t('restore_help')}</p><RecoveryFacts request={action.pending} locale={locale}/><p className="ua-raw">{String(action.pending.body.reason||'')}</p><button data-usage-restore type="button" disabled={action.busy} onClick={()=>{const request=action.pending;if(request)void action.run(request.path,request.body,request.method).then(completed);}}>{t('restore')}</button></div>}
  {notice&&<p className="ua-success" role="status">{t('saved')}</p>}
  {dashboard.loading&&<Loading/>}{Boolean(dashboard.error)&&<ErrorNotice error={toolError(dashboard.error)} onRetry={()=>void dashboard.read()}/>}
  {data&&<>
   <div className="ua-balances"><Balance balance={data.balances.find(value=>value.scope==='project'&&value.project_id===project)} policy={data.project_policy} locale={locale}/>{data.can_manage_organization&&data.organization_policy&&<div data-usage-organization><Balance balance={data.balances.find(value=>value.scope==='organization')} policy={data.organization_policy} locale={locale}/></div>}</div>
   <p className="ua-muted">{t('priced_portion')}</p>
   {!data.can_manage_organization&&<p className="ua-muted">{data.organization_enforced?t('organization_enforced'):t('organization_observe')}</p>}
   <p className="ua-muted">{t('admission_only')}{' '}{t('as_of',{value:formatDateTime(data.as_of)})}</p>
   {(data.can_manage_project||data.can_manage_organization)&&<div className="panel ua-management">
    <button data-usage-policy-toggle className="ua-toggle" type="button" aria-expanded={!!policy} disabled={locked} onClick={()=>setPolicy(value=>value?undefined:policyDraft(data.project_policy))}><span>{t('edit_policy')}</span><ChevronDown size={16} aria-hidden="true"/></button>
    {policy&&<form data-usage-policy-form onSubmit={event=>{event.preventDefault();void savePolicy();}}><WriteFields action={action}>
     <div className="ua-grid"><Field label={t('scope')}><select name="policy_scope" value={policy.scope} disabled={locked} onChange={event=>{const source=event.target.value==='organization'?data.organization_policy:data.project_policy;if(source)setPolicy(policyDraft(source));}}>{data.can_manage_project&&<option value="project">{t('project')}</option>}{data.can_manage_organization&&data.organization_policy&&<option value="organization">{t('organization')}</option>}</select></Field><Field label={t('mode')}><select name="policy_mode" value={policy.mode} onChange={event=>setPolicy({...policy,mode:event.target.value,ack:false})}><option value="observe">{t('observe')}</option><option value="enforce">{t('enforce')}</option></select></Field><Field label={t('currency')}><select name="policy_currency" value={policy.currency} onChange={event=>setPolicy({...policy,currency:event.target.value,ack:false})}><option>CNY</option><option>USD</option></select></Field></div>
     <div className="ua-grid"><div><label className="ua-check"><input name="token_unlimited" type="checkbox" checked={policy.tokenUnlimited} onChange={event=>setPolicy({...policy,tokenUnlimited:event.target.checked})}/>{t('unlimited_tokens')}</label><Field label={t('token_limit')}><input name="token_limit" type="text" inputMode="numeric" disabled={policy.tokenUnlimited} value={policy.tokens} onChange={event=>setPolicy({...policy,tokens:event.target.value})}/></Field></div><div><label className="ua-check"><input name="cost_unlimited" type="checkbox" checked={policy.costUnlimited} onChange={event=>setPolicy({...policy,costUnlimited:event.target.checked})}/>{t('unlimited_cost')}</label><Field label={t('cost_limit',{currency:policy.currency})} hint={t('decimal_help')}><input name="cost_limit" type="text" inputMode="decimal" disabled={policy.costUnlimited} value={policy.cost} onChange={event=>setPolicy({...policy,cost:event.target.value})}/></Field></div></div>
     <p className="ua-muted">{t('zero_limit')}{' '}{t('version',{value:formatNumber(policy.version)})}</p><Field label={t('reason')}><textarea name="policy_reason" rows={2} maxLength={4000} value={policy.reason} onChange={event=>setPolicy({...policy,reason:event.target.value})}/></Field>
     {policy.mode==='enforce'&&<label className="ua-check"><input name="acknowledge_enforcement" type="checkbox" checked={policy.ack} onChange={event=>setPolicy({...policy,ack:event.target.checked})}/>{t('enforce_ack')}</label>}<WriteButton action={action} disabled={!canPolicy}>{t('save_policy')}</WriteButton>
    </WriteFields></form>}
    {data.can_manage_rates&&<><button data-usage-rate-toggle className="ua-toggle" type="button" aria-expanded={rateOpen} disabled={locked} onClick={()=>setRateOpen(value=>!value)}><span>{t('edit_rates')}</span><ChevronDown size={16} aria-hidden="true"/></button>
    {rateOpen&&<div className="ua-rate-body"><p className="ua-muted">{t('rate_help')}{' '}{t('rate_coverage',{loaded:formatNumber(data.rates.length),total:formatNumber(data.rate_total)})}</p>
     {profiles.loading&&<Loading/>}{Boolean(profiles.error)&&<ErrorNotice error={toolError(profiles.error)} onRetry={()=>void profiles.read()}/>}
     {profiles.value&&!profileOptions.length&&<div className="ua-callout"><p>{t('no_profiles')}</p><a href={'?'+new URLSearchParams({project,view:'settings',sub:'tools'})}>{t('open_tools')}</a></div>}
     <form data-usage-rate-form onSubmit={event=>{event.preventDefault();void saveRate();}}><WriteFields action={action}>
      <Field label={t('profile')}><select name="rate_profile" value={rate?.profile||''} onChange={event=>{const profile=profileOptions.find(value=>value.id===event.target.value);setRate(profile?rateDraft(profile,data.rates,data.rate_total):undefined);}}><option value="">{t('choose_profile')}</option>{profileOptions.map(value=><option key={value.id} value={value.id}>{value.name}</option>)}</select></Field>
      {profiles.value?.next_cursor&&<p className="ua-muted">{t('profile_coverage')} <a href={'?'+new URLSearchParams({project,view:'settings',sub:'tools'})}>{t('open_tools')}</a></p>}
      {rate&&chosenProfile&&<><p className="ua-raw ua-muted">{chosenProfile.profile.planning?.model}{' · '}{chosenProfile.profile.planning?.endpoint_origin}</p><p className="ua-muted">{t('not_health')}</p>{rate.version===undefined&&<p className="ua-warning">{t('rate_not_loaded')}</p>}
       <div className="ua-grid"><Field label={t('currency')}><select name="rate_currency" value={rate.currency} onChange={event=>setRate({...rate,currency:event.target.value})}><option>CNY</option><option>USD</option></select></Field><Field label={t('input_rate',{currency:rate.currency})} hint={t('decimal_help')}><input name="input_rate" inputMode="decimal" value={rate.input} onChange={event=>setRate({...rate,input:event.target.value})}/></Field><Field label={t('output_rate',{currency:rate.currency})} hint={t('decimal_help')}><input name="output_rate" inputMode="decimal" value={rate.output} onChange={event=>setRate({...rate,output:event.target.value})}/></Field></div>
       <div className="ua-grid"><Field label={t('input_reservation')}><input name="input_reservation" inputMode="numeric" value={rate.inputReservation} onChange={event=>setRate({...rate,inputReservation:event.target.value})}/></Field><Field label={t('output_reservation')} hint={t('output_floor')}><input name="output_reservation" inputMode="numeric" value={rate.outputReservation} onChange={event=>setRate({...rate,outputReservation:event.target.value})}/></Field></div><p className="ua-muted">{t('reservation_help')}{' '}{t('version',{value:rate.version===undefined?t('unknown'):formatNumber(rate.version)})}</p>
       <Field label={t('reason')}><textarea name="rate_reason" rows={2} maxLength={4000} value={rate.reason} onChange={event=>setRate({...rate,reason:event.target.value})}/></Field><WriteButton action={action} disabled={!canRate}>{t('save_rate')}</WriteButton>
      </>}
     </WriteFields></form>
     {!!data.rates.length&&<details className="ua-disclosure"><summary>{t('current_rates')}</summary><div className="ua-rates">{data.rates.map(value=><div key={value.id}><strong>{value.model} · {value.currency} · v{formatNumber(value.version)}</strong><p>{t('rate_pair',{input:money(value.input_rate_micros_per_million,value.currency,locale),output:money(value.output_rate_micros_per_million,value.currency,locale)})}</p><code>{value.profile_sha256}</code></div>)}</div></details>}
    </div>}</>}
   </div>}
  </>}
  <section className="panel ua-attempts"><div className="ua-heading"><h2>{t('attempts')}</h2><Field label={t('state_filter')}><select name="usage_state" value={filter} disabled={locked} onChange={event=>{setFilter(event.target.value);setConfirm(undefined);}}><option value="">{t('all_states')}</option>{states.map(state=><option key={state} value={state}>{t('state.'+state)}</option>)}</select></Field></div><p className="ua-muted">{t('counter_help')}</p>
   {attempts.loading&&<Loading/>}{Boolean(attempts.error)&&<ErrorNotice error={toolError(attempts.error)} onRetry={()=>void attempts.read()}/>}
   {attempts.page&&<><p className="ua-muted">{t('attempt_coverage',{loaded:formatNumber(attempts.page.items.length),total:formatNumber(attempts.page.total)})}</p>{!attempts.page.items.length&&<p className="ua-empty">{t('no_attempts')}</p>}
    {attempts.page.items.map(item=><article className="ua-attempt" data-usage-attempt key={item.id}>
     <div className="ua-heading"><div><strong>{item.model||t('unattributed_model')}</strong><p className="ua-muted">{t('attempt_identity',{attempt:formatNumber(item.attempt),fence:formatNumber(item.fence)})}{' · '}{t('state.'+item.state)}</p></div><a data-usage-run href={'?'+new URLSearchParams({project,view:'development',sub:'runs',run:item.run_id,node:item.node_id,attempt:String(item.attempt)})}>{t('open_run')}</a></div>
     <dl className="ua-facts"><div data-attempt-total><dt>{t('reported_tokens')}</dt><dd>{count(item.reported_tokens)}</dd></div><div><dt>{t('estimated_cost')}</dt><dd>{money(item.estimated_cost_micros,item.currency,locale)}</dd></div><div><dt>{t('held_tokens')}</dt><dd>{count(item.held_tokens)}</dd></div><div><dt>{t('held_cost')}</dt><dd>{money(item.held_cost_micros,item.currency,locale)}</dd></div></dl>
     {item.overrun&&<p className="ua-warning">{t('overrun')}</p>}<p className="ua-muted">{t('source.'+item.usage_source)}</p>
     <details className="ua-disclosure"><summary data-usage-attempt-details>{t('source_details')}</summary><dl className="ua-facts">{counters.map(key=><div data-counter={key} key={key}><dt>{t('counter.'+key)}</dt><dd>{count(item.usage?.[key])}</dd></div>)}</dl><p>{t('usage_state')}: {['complete','partial','unreported'].includes(item.usage_state)?t('coverage.'+item.usage_state):item.usage_state}</p><p>{t('reserved_tokens')}: {count(item.reserved_tokens)} · {t('reserved_cost')}: {money(item.reserved_cost_micros,item.currency,locale)}</p><p className="ua-raw">{item.tool_profile_id} · {item.worker_id}</p><code>{item.profile_sha256}</code><code>{item.source_result_sha256}</code>{item.rate&&<p>{t('frozen_rate',{version:formatNumber(item.rate.version),currency:item.rate.currency,model:item.rate.model})}</p>}{item.source_ref&&<p className="ua-raw">{item.source_ref}</p>}{item.decision_reason&&<p className="ua-raw">{item.decision_reason}</p>}{item.decision_by&&<p>{item.decision_by} · {formatDateTime(item.decision_at)}</p>}</details>
     {item.allowed_actions.includes('confirm')&&<button data-usage-confirm type="button" disabled={locked} onClick={()=>openConfirm(item)}>{t('confirm')}</button>}
    </article>)}
    {attempts.page.next_cursor&&<button data-usage-more type="button" disabled={attempts.loading||locked} onClick={()=>void attempts.read(true)}>{t('more')}</button>}
   </>}
   {confirm&&<form className="ua-confirm" data-usage-confirm-form onSubmit={event=>{event.preventDefault();void saveConfirm();}}><h3>{t('confirm')}</h3><p>{t('confirm_help')}</p><p className="ua-raw">{confirm.item.run_id} · {confirm.item.node_id} · {t('attempt_identity',{attempt:formatNumber(confirm.item.attempt),fence:formatNumber(confirm.item.fence)})}</p><WriteFields action={action}><div className="ua-grid">{counters.map(key=><Field key={key} label={t('counter.'+key)} hint={confirm.item.usage?.[key]!==undefined?t('minimum',{value:count(confirm.item.usage[key])}):['input_tokens','output_tokens'].includes(key)?t('required_counter'):t('optional_counter')}><input name={'confirm_'+key} inputMode="numeric" value={confirm.values[key]} onChange={event=>setConfirm({...confirm,values:{...confirm.values,[key]:event.target.value},ack:false})}/></Field>)}</div><Field label={t('source_ref')}><input name="source_ref" maxLength={2000} value={confirm.source} onChange={event=>setConfirm({...confirm,source:event.target.value})}/></Field><Field label={t('reason')}><textarea name="confirm_reason" maxLength={4000} rows={2} value={confirm.reason} onChange={event=>setConfirm({...confirm,reason:event.target.value})}/></Field><label className="ua-check"><input name="acknowledge_usage" type="checkbox" checked={confirm.ack} onChange={event=>setConfirm({...confirm,ack:event.target.checked})}/>{t('confirm_ack')}</label><div className="ua-actions"><WriteButton action={action} disabled={!canConfirm}>{t('save_confirmation')}</WriteButton><button type="button" disabled={locked} onClick={()=>setConfirm(undefined)}>{t('cancel')}</button></div></WriteFields></form>}
  </section>
 </section>;
}
function Balance({balance,policy,locale}:{balance?:UsageBalance;policy:UsagePolicy;locale:string}){
 return <section className="panel ua-balance"><div className="ua-heading"><h2>{t(policy.scope==='organization'?'organization':'project')}</h2><span className={'ua-mode ua-mode-'+policy.mode}>{t(policy.mode)}</span></div><dl className="ua-facts"><div><dt>{t('reported_tokens')}</dt><dd>{count(balance?.reported_tokens)}</dd></div><div data-usage-reserved-tokens><dt>{t('reserved_tokens')}</dt><dd>{count(balance?.reserved_tokens)}</dd></div><div><dt>{t('unresolved')}</dt><dd>{count(balance?.unresolved_attempts)}</dd></div><div><dt>{t('token_limit')}</dt><dd>{policy.token_limit===null?t('unlimited'):count(policy.token_limit)}</dd></div></dl><div className="ua-money">{balance?.costs.map(cost=><div key={cost.currency}><strong>{cost.currency}</strong><dl><div><dt>{t('estimated_cost')}</dt><dd>{money(cost.spent_micros,cost.currency,locale)}</dd></div><div><dt>{t('reserved_cost')}</dt><dd>{money(cost.reserved_micros,cost.currency,locale)}</dd></div></dl></div>)}</div><p className="ua-muted">{t('policy_cost_limit')}: {policy.cost_limit_micros===null?t('unlimited'):money(policy.cost_limit_micros,policy.currency,locale)}</p></section>;
}
function RecoveryFacts({request,locale}:{request:WriteRecoveryRequest;locale:string}){
 const b=request.body;
 return <div className="ua-recovery-facts"><p>{t('version',{value:count(b.expected_version)})}</p>{request.path==='/usage/policies'?<><p>{t(b.scope==='organization'?'organization':'project')} · {t(b.mode==='enforce'?'enforce':'observe')}</p><p>{t('token_limit')}: {b.token_limit===null?t('unlimited'):count(b.token_limit)}</p><p>{t('policy_cost_limit')}: {b.cost_limit_micros===null?t('unlimited'):money(b.cost_limit_micros,String(b.currency),locale)}</p></>:request.path==='/usage/rates'?<><p className="ua-raw">{String(b.tool_profile_id)}</p><p>{t('rate_pair',{input:money(b.input_rate_micros_per_million,String(b.currency),locale),output:money(b.output_rate_micros_per_million,String(b.currency),locale)})}</p><p>{t('input_reservation')}: {count(b.input_token_reservation)} · {t('output_reservation')}: {count(b.output_token_reservation)}</p></>:<><p>{t('confirm')}</p><dl className="ua-facts">{counters.map(key=><div key={key}><dt>{t('counter.'+key)}</dt><dd>{count((b.usage as ToolUsage|undefined)?.[key])}</dd></div>)}</dl><p className="ua-raw">{String(b.source_ref)}</p></>}</div>;
}
