import {useEffect,useMemo,useState} from 'react';
import {ArrowUpRight,BrainCircuit,RefreshCw,ShieldCheck} from 'lucide-react';
import type {ModelExecutionOption,ModelExecutionOptions,ToolProfile} from '../../../packages/contracts/src';
import {ApiError,query} from './api';
import {useApi} from './api-context';
import {validCodexCatalog} from './codex-execution-model';
import {formatDateTime,formatNumber,renderDiagnostic,tr as t,UiError,useLocale} from './i18n';
import {modelMoneyInput,modelOptionsPath,validModelOptions,type ModelScope} from './model-execution-model';
import {useManagedToolProfiles} from './tool-hooks';
import {Empty,ErrorNotice,Field,Loading} from './ui';
import './management.css';

const idPattern=/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/;
const sha256=/^[a-f0-9]{64}$/;
const activeTools={state:'active',query:''} as const;
const number=(value:number)=>formatNumber(value);
const m=(key:string,params?:Record<string,unknown>)=>t('management.'+key,params);
const accessDenied=(error:unknown)=>error instanceof ApiError&&[401,403].includes(error.status);
const denied=(error:unknown)=>accessDenied(error)||error instanceof ApiError&&error.status===404;

function toolLink(project:string,sub:string,record?:string){
 return '?'+new URLSearchParams({project,view:'settings',sub,...(record?{record}:{})});
}

function validSource(value:ToolProfile,project:string,id=value?.id){
 return !!value&&value.id===id&&idPattern.test(id)&&value.project_id===project&&
  ['enabled','paused','revoked'].includes(value.state)&&Number.isSafeInteger(value.version)&&value.version>0&&
  typeof value.name==='string'&&!!value.name&&typeof value.worker_id==='string'&&!!value.worker_id&&
  typeof value.profile?.profile_sha256==='string'&&sha256.test(value.profile.profile_sha256);
}

function publicOrigin(value:unknown){
 if(typeof value!=='string'||!/^https?:\/\/[^/?#@\\\s]+$/.test(value))return false;
 try{
  const url=new URL(value);
  return !!url.hostname&&(url.protocol==='https:'||['localhost','127.0.0.1','[::1]'].includes(url.hostname));
 }catch{return false;}
}

function validDirectory(page:ModelExecutionOptions,scope:ModelScope){
 if(!validModelOptions(page,scope)||typeof page.codex_supported!=='boolean'||
  typeof page.reason!=='string'||typeof page.codex_reason!=='string')return false;
 const fixed=[page.fixed_opencode,page.fixed_claude,page.fixed_claude_sdk,page.fixed_acp].filter(Boolean);
 if(fixed.length>1||fixed.length&&page.codex_catalog!==undefined||
  (page.codex_supported||page.codex_catalog!==undefined)&&(!validCodexCatalog(page.codex_catalog)||page.supported||page.items.length>0)||
  !page.supported&&page.items.length>0)return false;
 const seen=new Set<string>();
 return page.items.every(({deployment:d,cost_state,rate})=>{
  if(seen.has(d.deployment_sha256)||!publicOrigin(d.endpoint_origin))return false;
  seen.add(d.deployment_sha256);
  if(cost_state==='unpriced')return rate===undefined;
  return !!rate&&rate.project_id===scope.project&&rate.tool_profile_id===scope.profile&&rate.profile_sha256===scope.sha256&&
   rate.model_deployment_id===d.deployment_id&&rate.model_deployment_revision===d.revision&&rate.model_deployment_sha256===d.deployment_sha256&&
   rate.model===d.model&&rate.endpoint_origin===d.endpoint_origin&&['CNY','USD'].includes(rate.currency)&&
   typeof rate.id==='string'&&!!rate.id&&Number.isSafeInteger(rate.version)&&rate.version>0&&
   [rate.input_rate_micros_per_million,rate.output_rate_micros_per_million].every(value=>Number.isSafeInteger(value)&&value>=0);
 });
}

type SourceRead={profile?:ToolProfile;page?:ModelExecutionOptions;error?:unknown};

export function ModelDirectory({project,refresh,selected,onSelect}:{project:string;refresh:number;selected:string;onSelect:(id:string)=>void}){
 useLocale();
 const api=useApi(),[revision,setRevision]=useState(0);
 const profiles=useManagedToolProfiles<ToolProfile>(project,activeTools,refresh+revision);
 const listError=profiles.error_action==='first'||denied(profiles.failure)?profiles.failure:undefined;
 const request=useMemo(()=>({api,project,selected,refresh,revision,listError}),[api,project,selected,refresh,revision,listError]);
 const [read,setRead]=useState<{request:typeof request;result:SourceRead}>();

 useEffect(()=>{
  const {api,project,selected,listError}=request;
  if(!selected||listError)return;
  const abort=new AbortController();
  const finish=(result:SourceRead)=>{
   if(!abort.signal.aborted)setRead({request,result});
  };
  async function load(){
   let profile:ToolProfile|undefined;
   try{
    if(!idPattern.test(selected))throw new UiError('management.invalid_model_source');
    const value=await api<ToolProfile>('/tool-profiles/'+encodeURIComponent(selected)+query(project),undefined,'GET',undefined,abort.signal);
    if(!validSource(value,project,selected))throw new UiError('management.invalid_model_source');
    if(value.state==='revoked')throw new UiError('management.revoked_model_source');
    profile=value;
    const scope={project,profile:profile.id,version:profile.version,sha256:profile.profile.profile_sha256};
    const page=await api<ModelExecutionOptions>(modelOptionsPath(scope),undefined,'GET',undefined,abort.signal);
    if(!validDirectory(page,scope))throw new UiError('model-execution.invalid_options');
    finish({profile,page});
   }catch(error){finish({profile:denied(error)?undefined:profile,error});}
  }
  void load();
  return()=>abort.abort();
 },[request]);

 const current=read?.request===request?read.result:undefined;
 const checking=!!selected&&!listError&&!current;
 const blocked=accessDenied(current?.error);
 const sources=listError||blocked?[]:(profiles.items??[]).filter(item=>validSource(item,project)&&item.state!=='revoked');
 const source=current?.profile;
 const options=sources.filter(item=>!current?.error||!!source||item.id!==selected).map(item=>item.id===source?.id?source:item);
 if(source&&!options.some(item=>item.id===source.id))options.unshift(source);
 const refreshAll=()=>setRevision(value=>value+1);
 const tools=toolLink(project,'tools');
 const billing=toolLink(project,'usage');
 const exactTool=source&&toolLink(project,'tools',source.id);
 const sourceLabel=(item:ToolProfile)=>item.name+' · '+item.worker_id+(item.state==='paused'?' · '+m('source_paused'):'');
 const initialList=!profiles.items&&!profiles.failure;

 return <section className="management-models" aria-label={m('model_directory')}>
  <div className="management-heading">
   <div><h2>{m('model_directory')}</h2><p>{m('model_source')}</p></div>
   <button type="button" onClick={refreshAll} disabled={profiles.loading||checking}><RefreshCw size={15} aria-hidden="true"/>{m('refresh')}</button>
  </div>
  <div className="panel management-model-source">
   <Field label={m('model_tool')}>
    <select value={selected} disabled={blocked||!!listError||!options.length&&!selected} onChange={event=>onSelect(event.target.value)}>
     <option value="">{m('choose_tool')}</option>
     {selected&&!options.some(item=>item.id===selected)&&<option value={selected}>{m(checking?'checking_source':'source_unavailable')}</option>}
     {options.map(item=><option key={item.id} value={item.id}>{sourceLabel(item)}</option>)}
    </select>
   </Field>
   {profiles.next_cursor&&!listError&&!blocked&&<div className="management-source-more">
    <small>{m('source_count',{loaded:number(sources.length),total:number(profiles.total)})}</small>
    <button type="button" onClick={()=>void profiles.read(true)} disabled={profiles.loading}>{m('more_model_sources')}</button>
   </div>}
   <p>{m('model_scope')}</p>
  </div>
  {(initialList||profiles.loading||checking)&&<Loading/>}
  {!!profiles.failure&&<ErrorNotice error={profiles.failure} onRetry={listError?refreshAll:()=>void profiles.read(true)}/>}
  {!selected&&!initialList&&!profiles.loading&&!profiles.failure&&(options.length>0?
   <Empty title={m('choose_source_title')} description={m('choose_source_help')}/>:
   !profiles.next_cursor&&<Empty title={m('models_empty')} description={m('models_empty_help')} action={<a href={tools}>{m('open_tools')}</a>}/>)}
  {!!current?.error&&<ErrorNotice error={current.error} onRetry={refreshAll}/>}
  {source&&<div className="management-source-summary">
   <div><strong>{source.name}</strong><p>{m('source_version',{worker:source.worker_id,version:number(source.version)})}</p>
    {!!current?.error&&source.reason&&<p>{renderDiagnostic(source.reason_diagnostic??source.reason)}</p>}
   </div>
   <a href={exactTool}>{m('open_source_tool')} <ArrowUpRight size={14} aria-hidden="true"/></a>
  </div>}
  {!!current?.error&&!source&&!denied(current.error)&&<a href={tools}>{m('open_tools')}</a>}
  {current?.page&&<Catalog page={current.page} tool={exactTool!} billing={billing}/>}
  <div className="management-boundary"><ShieldCheck size={17} aria-hidden="true"/><p>{m('model_boundary')}</p><a href={billing}>{m('open_billing')}</a></div>
 </section>;
}

function Catalog({page,tool,billing}:{page:ModelExecutionOptions;tool:string;billing:string}){
 const [search,setSearch]=useState('');
 const codex=page.codex_catalog;
 const fixed=page.fixed_opencode??page.fixed_claude??page.fixed_claude_sdk??page.fixed_acp;
 const fixedName=page.fixed_opencode?page.fixed_opencode.provider_id+'/'+page.fixed_opencode.model:fixed?.model;
 const term=search.trim().toLowerCase();
 const matches=(...fields:string[])=>fields.some(field=>field.toLowerCase().includes(term));
 const deployments=page.items.filter(({deployment:d})=>matches(d.model,d.deployment_id,d.endpoint_origin));
 const models=codex?.models.filter(model=>matches(model.model))??[];
 const total=page.items.length+(codex?.models.length??0)+(fixed?1:0);
 const shown=deployments.length+models.length+(fixed?1:0);

 return <>
  <p className="management-model-asof">{m('model_observed',{at:formatDateTime(page.observed_at,{dateStyle:'short',timeStyle:'medium'})})}</p>
  {codex&&!page.codex_supported&&<div className="warning" role="status">
   <strong>{m('dispatch_restricted')}</strong><p>{renderDiagnostic(page.codex_reason)}</p><a href={billing}>{m('open_billing')}</a>
  </div>}
  {total>1&&<div className="management-model-filter">
   <Field label={m('search_models')}><input type="search" value={search} maxLength={256} onChange={event=>setSearch(event.target.value)}/></Field>
   <p role="status">{m('model_count',{shown:number(shown),total:number(total)})}</p>
  </div>}
  {shown>0&&<div className="management-model-grid">
   {deployments.map(item=><Deployment key={item.deployment.deployment_sha256} item={item}/>)}
   {models.map(model=><article className="panel management-model-card" key={model.model}>
    <ModelHeading name={model.model} kind="tool_catalog"/>
    <dl><dt>{m('reasoning_options')}</dt><dd>{model.reasoning_efforts.join(' / ')||m('fixed_parameters')}</dd>
     {model.default_reasoning_effort&&<><dt>{m('default_reasoning')}</dt><dd>{model.default_reasoning_effort}</dd></>}
    </dl>
    <p>{m('tool_catalog_boundary')}</p>
    <details><summary>{m('source_basis')}</summary><dl>
     <dt>{m('configuration_digest')}</dt><dd><code>{codex?.configuration_sha256}</code></dd>
     <dt>{m('catalog_digest')}</dt><dd><code>{codex?.catalog_sha256}</code></dd>
    </dl></details>
   </article>)}
   {fixed&&fixedName&&<article className="panel management-model-card">
    <ModelHeading name={fixedName} kind="fixed_model"/>
    {page.fixed_opencode&&<dl><dt>{m('endpoint')}</dt><dd>{page.fixed_opencode.endpoint_origin}</dd></dl>}
    <p>{m('fixed_model_help')}</p>
    <details><summary>{m('source_basis')}</summary><dl>
     <dt>{m('configuration_digest')}</dt><dd><code>{fixed.configuration_sha256}</code></dd>
    </dl></details>
   </article>}
  </div>}
  {total>0&&shown===0&&<Empty title={m('no_model_matches')} action={<button type="button" onClick={()=>setSearch('')}>{m('clear_model_search')}</button>}/>}
  {!total&&<Empty title={m(page.supported?'no_model_options':'source_has_no_catalog')} description={m(page.supported?'no_model_options_help':'source_has_no_catalog_help')} action={<a href={tool}>{m('open_source_tool')}</a>}/>}
 </>;
}

function ModelHeading({name,kind}:{name:string;kind:string}){
 return <div className="management-model-card-heading"><BrainCircuit size={20} aria-hidden="true"/><h3>{name}</h3><span className="badge">{m(kind)}</span></div>;
}

function Deployment({item}:{item:ModelExecutionOption}){
 const {deployment:d,rate}=item;
 return <article className="panel management-model-card">
  <ModelHeading name={d.model} kind="platform_model"/>
  <dl>
   <dt>{m('deployment')}</dt><dd>{d.deployment_id} · v{number(d.revision)}</dd>
   <dt>{m('endpoint')}</dt><dd>{d.endpoint_origin}</dd>
   <dt>{m('context')}</dt><dd>{m('token_count',{count:number(d.context_window)})}</dd>
   <dt>{m('pricing')}</dt><dd>{rate?m('priced',{currency:rate.currency}):m('unpriced')}</dd>
   {rate&&<>
    <dt>{m('input_rate')}</dt><dd>{m('per_million',{amount:modelMoneyInput(rate.input_rate_micros_per_million),currency:rate.currency})}</dd>
    <dt>{m('output_rate')}</dt><dd>{m('per_million',{amount:modelMoneyInput(rate.output_rate_micros_per_million),currency:rate.currency})}</dd>
   </>}
  </dl>
  <details><summary>{m('source_basis')}</summary><dl>
   <dt>{m('deployment_digest')}</dt><dd><code>{d.deployment_sha256}</code></dd>
   <dt>{m('configuration_digest')}</dt><dd><code>{d.endpoint_config_sha256}</code></dd>
   {rate&&<><dt>{m('rate_revision')}</dt><dd>{rate.id} · v{number(rate.version)}</dd></>}
  </dl></details>
 </article>;
}
