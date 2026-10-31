import {useEffect, useId, useState} from 'react';
import {RefreshCw} from 'lucide-react';
import type {CodexBinding, CodexCatalog, CodexEvidence, ExecutionSkillBundle, ModelExecutionOptions} from '../../../packages/contracts/src';
import {CodexNativeSkillFacts} from './CodexNativeSkillFacts';
import {useApi} from './api-context';
import {formatNumber, tr, UiError, useLocale} from './i18n';
import {ErrorNotice, Field, Loading} from './ui';
import type {ModelChoice} from './ModelExecutionPanel';
import {modelOptionsPath, validModelOptions, type ModelScope} from './model-execution-model';
import {codexBindingPresent, codexSelection, validCodexCatalog, codexEffortLabel} from './codex-execution-model';
const t=(key:string,values?:Record<string,unknown>)=>tr('codex-execution.'+key,values);

type Props={scope:ModelScope;disabled:boolean;onChange:(choice:ModelChoice)=>void};
/** A fresh scoped inventory read. Selecting values neither starts the CLI nor
 * borrows its account configuration, workspace permissions or pricing. */
export function CodexModelExecutionPanel({scope,disabled,onChange}:Props) {
 useLocale();const api=useApi(),path=modelOptionsPath(scope);
 const [read,setRead]=useState<{api:typeof api;path:string;value:ModelExecutionOptions}>(),[error,setError]=useState<unknown>(),[loading,setLoading]=useState(false),[refresh,setRefresh]=useState(0);
 const page=read?.api===api&&read.path===path?read.value:undefined;
 useEffect(()=>{
  const abort=new AbortController();setRead(undefined);setError(undefined);setLoading(true);
  void api<ModelExecutionOptions>(path,undefined,'GET',undefined,abort.signal).then(value=>{
   if(abort.signal.aborted)return;
   if(!validModelOptions(value,scope)||value.supported||value.codex_supported!==true||!validCodexCatalog(value.codex_catalog))throw new UiError('codex-execution.invalid_options');
   setRead({api,path,value});
  }).catch(reason=>{if(!abort.signal.aborted)setError(reason);}).finally(()=>{if(!abort.signal.aborted)setLoading(false);});
  return()=>abort.abort();
 },[api,path,refresh]);
 return <CodexModelExecutionSelector key={path+':'+refresh} catalog={page?.codex_catalog} disabled={disabled} loading={loading} error={error} onChange={onChange} onRefresh={()=>setRefresh(n=>n+1)}/>;
}
/** Reuse an already validated discovery response so role discovery reads once. */
export function CodexModelExecutionSelector({catalog,disabled,loading=false,error,onChange,onRefresh}:{catalog?:CodexCatalog|null;disabled:boolean;loading?:boolean;error?:unknown;onChange:(choice:ModelChoice)=>void;onRefresh:()=>void}) {
 useLocale();const id=useId(),[draft,setDraft]=useState({model:'',effort:'',threshold:''});
 const selection=catalog?codexSelection(catalog,draft.model,draft.effort,draft.threshold):undefined;
 useEffect(()=>onChange({ready:!loading&&!error&&!!selection,codexSelection:selection}),[catalog,draft,loading,error,onChange]);
 const selected=catalog?.models.find(item=>item.model===draft.model);
 return <section className="model-execution" aria-labelledby={id} aria-busy={loading}>
  <div className="model-execution-heading"><h3 id={id}>{t('title')}</h3><button type="button" aria-label={t('refresh')} disabled={disabled||loading} onClick={onRefresh}><RefreshCw size={14}/></button></div>
  <p className="muted">{t('intro')}</p>{loading&&<Loading/>}{!!error&&<ErrorNotice error={error}/>}
  {catalog&&<><Field label={t('model')}><select data-codex-model value={draft.model} disabled={disabled||loading} onChange={event=>{const model=catalog.models.find(item=>item.model===event.target.value);setDraft({model:event.target.value,effort:model?.default_reasoning_effort||'',threshold:''});}}><option value="">{t('choose')}</option>{catalog.models.map(item=><option key={item.model} value={item.model}>{item.model}</option>)}</select></Field>
   {selected&&<><Field label={t('effort')}><select data-codex-effort value={draft.effort} disabled={disabled||loading||!selected.reasoning_efforts.length} onChange={event=>setDraft({...draft,effort:event.target.value})}>{selected.reasoning_efforts.length?selected.reasoning_efforts.map(effort=><option key={effort} value={effort}>{codexEffortLabel(effort)}</option>):<option value="">{t('default_effort')}</option>}</select></Field>
    <details><summary>{t('notice')}</summary><Field label={t('token_notice')} hint={t('notice_hint')}><input data-codex-notice inputMode="numeric" value={draft.threshold} disabled={disabled||loading} onChange={event=>setDraft({...draft,threshold:event.target.value})}/></Field>{!selection&&<p role="status">{t('invalid_notice')}</p>}</details>
   </>}
   <p className="muted">{t('boundary')}</p><details><summary>{t('basis')}</summary><p>{t('config_digest')}</p><code>{catalog.configuration_sha256}</code><p>{t('catalog_digest')}</p><code>{catalog.catalog_sha256}</code></details>
  </>}
 </section>;
}
export function CodexExecutionFacts({binding,evidence,skills}:{binding?:CodexBinding|null;evidence?:CodexEvidence|null;skills?:ExecutionSkillBundle|null}) {
 useLocale();if(!codexBindingPresent(binding)&&!evidence)return null;
 return <section className="model-execution" aria-label={t('frozen')}><h3>{t('frozen')}</h3>
  {binding&&<><strong>{binding.model}</strong><p>{t('effort')}: {binding.reasoning_effort?codexEffortLabel(binding.reasoning_effort):t('default_effort')}</p>{binding.token_notice_limit!=null&&<p>{t('notice_value',{count:formatNumber(binding.token_notice_limit)})}</p>}</>}
  {evidence&&<><p>{t('requested')}: {evidence.requested_model} · {evidence.requested_reasoning_effort?codexEffortLabel(evidence.requested_reasoning_effort):t('default_effort')}</p><p>{evidence.configuration_acknowledged?t('acknowledged',{model:evidence.acknowledged_model,effort:evidence.acknowledged_reasoning_effort?codexEffortLabel(evidence.acknowledged_reasoning_effort):t('default_effort')}):t('not_acknowledged')}</p>{evidence.token_notice_exceeded&&<p role="status" className="warning">{t('notice_exceeded')}</p>}{!!evidence.rerouted_models?.length&&<p>{t('rerouted')}: {evidence.rerouted_models.join(', ')}</p>}</>}
  <p className="muted">{t('boundary')}</p><details><summary>{t('basis')}</summary>{binding&&<code>{binding.binding_sha256}</code>}{evidence&&<p>{t('thread')}: {evidence.thread_id||'—'} · {t('turn')}: {evidence.turn_id||'—'}</p>}</details>
  {evidence&&<CodexNativeSkillFacts mode="attempt" bundle={skills} evidence={evidence.skills}/>}
 </section>;
}
