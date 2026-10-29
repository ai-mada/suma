import {useEffect,useState} from 'react';
import {BrainCircuit,RefreshCw,ShieldCheck} from 'lucide-react';
import {useApi} from './api-context';
import type {ModelExecutionOptions} from '../../../packages/contracts/src';
import {ErrorNotice,Empty,Field,Loading} from './ui';
import {useToolList} from './tool-hooks';
import type {ToolProfile} from './tool-model';
import {modelOptionsPath,validModelOptions} from './model-execution-model';
import {validCodexCatalog} from './codex-execution-model';
import {OpenCodeExecutionFacts} from './OpenCodeExecutionFacts';
import {formatDateTime,formatNumber,tr as t,UiError,useLocale} from './i18n';
import './management.css';
export function ModelDirectory({project,refresh}:{project:string;refresh:number}){
 useLocale();const api=useApi(),profiles=useToolList<ToolProfile>(project,'tool-profiles',refresh),[selected,setSelected]=useState(''),[revision,setRevision]=useState(0);
 const profile=!profiles.error?profiles.items?.find(item=>item.id===selected):undefined;
 const scope=profile?{project,profile:profile.id,version:profile.version,sha256:profile.profile.profile_sha256}:undefined;
 const path=scope?modelOptionsPath(scope):'';
 const [read,setRead]=useState<{path:string;value?:ModelExecutionOptions;error?:unknown;loading:boolean}>({path:'',loading:false});
 useEffect(()=>{
  const abort=new AbortController();setRead({path,loading:!!path});if(!scope)return()=>abort.abort();
  void api<ModelExecutionOptions>(path,undefined,'GET',undefined,abort.signal).then(value=>{
   if(!validModelOptions(value,scope)||(value.codex_supported||value.codex_catalog!=null)&&!validCodexCatalog(value.codex_catalog))throw new UiError('model-execution.invalid_options');
   if(!abort.signal.aborted)setRead({path,value,loading:false});
  }).catch(error=>{if(!abort.signal.aborted)setRead({path,error,loading:false});});return()=>abort.abort();
 },[api,path,revision,refresh]);
 const current=read.path===path?read:undefined,page=current?.value;
 const refreshAll=()=>{void profiles.read();setRevision(n=>n+1);};
 return <section className="management-models" aria-label={t('management.model_directory')}>
  <div className="management-heading"><div><h2>{t('management.model_directory')}</h2><p>{t('management.model_source')}</p></div><button onClick={refreshAll} disabled={profiles.loading||current?.loading}><RefreshCw size={15}/>{t('management.refresh')}</button></div>
  <div className="panel management-model-source"><Field label={t('management.model_tool')}><select value={profile?.id||''} onChange={event=>setSelected(event.target.value)}><option value="">{t('management.choose_tool')}</option>{profiles.items?.filter(item=>item.state!=='revoked').map(item=><option key={item.id} value={item.id}>{item.name} · {item.worker_id}</option>)}</select></Field><p>{t('management.model_scope')}</p></div>
  {profiles.loading&&<Loading/>}{profiles.error&&<ErrorNotice error={profiles.error} onRetry={()=>void profiles.read()}/>}
  {!profiles.loading&&!profiles.error&&profiles.items?.length===0&&<Empty title={t('management.models_empty')} description={t('management.models_empty_help')} action={<a href={'?'+new URLSearchParams({project,view:'settings',sub:'tools'})}>{t('management.open_tools')}</a>}/>}
  {profiles.next_cursor&&<button onClick={()=>void profiles.read(true)} disabled={profiles.loading}>{t('development.tools.load_more_configurations')}</button>}
  {current?.loading&&<Loading/>}{!!current?.error&&<ErrorNotice error={current.error} onRetry={()=>setRevision(n=>n+1)}/>}
  {page&&<>
   <p className="management-model-asof">{t('management.model_observed',{at:formatDateTime(page.observed_at,{dateStyle:'short',timeStyle:'medium'})})}</p>
   {page.codex_catalog&&!page.codex_supported&&page.codex_reason&&<div className="warning" role="status"><strong>{t('management.dispatch_restricted')}</strong><p>{page.codex_reason}</p></div>}
   <div className="management-model-grid">{page.items.map(item=><article className="panel management-model-card" key={item.deployment.deployment_sha256}><BrainCircuit size={22}/><h3>{item.deployment.model}</h3><span className="badge">{t('management.platform_model')}</span><dl><dt>{t('management.deployment')}</dt><dd>{item.deployment.deployment_id} · v{formatNumber(item.deployment.revision)}</dd><dt>{t('management.endpoint')}</dt><dd>{item.deployment.endpoint_origin}</dd><dt>{t('management.context')}</dt><dd>{formatNumber(item.deployment.context_window)}</dd><dt>{t('management.pricing')}</dt><dd>{item.cost_state==='priced'?t('management.priced',{currency:item.rate!.currency}):t('management.unpriced')}</dd></dl><details><summary>{t('management.source_basis')}</summary><code>{item.deployment.deployment_sha256}</code></details></article>)}</div>
   {page.codex_catalog&&<div className="management-model-grid">{page.codex_catalog.models.map(model=><article className="panel management-model-card" key={model.model}><BrainCircuit size={22}/><h3>{model.model}</h3><span className="badge">{t('management.tool_catalog')}</span><p>{model.reasoning_efforts.join(' / ')||t('management.fixed_parameters')}</p><small>{t('management.tool_catalog_boundary')}</small></article>)}</div>}
   {page.fixed_opencode&&<div className="panel management-model-card"><OpenCodeExecutionFacts capability={page.fixed_opencode}/></div>}
   {!page.items.length&&!page.codex_catalog&&!page.fixed_opencode&&<Empty title={t('management.no_model_options')} description={t('management.no_model_options_help')}/>}
  </>}
  <div className="management-boundary"><ShieldCheck size={17}/><p>{t('management.model_boundary')}</p><a href={'?'+new URLSearchParams({project,view:'settings',sub:'usage'})}>{t('management.open_billing')}</a></div>
 </section>;
}
