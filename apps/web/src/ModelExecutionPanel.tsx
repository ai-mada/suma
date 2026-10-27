import {useEffect, useId, useState} from 'react';
import {RefreshCw} from 'lucide-react';
import type {ModelBinding, ModelExecutionOptions, ModelSelection, CodexSelection} from '../../../packages/contracts/src';
import {useApi} from './api-context';
import {formatNumber, tr, UiError, useLocale} from './i18n';
import {ErrorNotice, Field, Loading} from './ui';
import {bindingPresent, modelOptionsPath, modelSelection, validModelOptions, type ModelDraft, type ModelScope} from './model-execution-model';
import './ModelExecutionPanel.css';
import {CodexModelExecutionPanel} from './CodexModelExecutionPanel';

const t = (key: string, values?: Record<string, unknown>) => tr('model-execution.' + key, values);
export type ModelChoice = {selection?: ModelSelection; codexSelection?: CodexSelection; ready: boolean};
type Props = {scope: ModelScope; supported: boolean; discover?: boolean; codex?: boolean; disabled: boolean; onChange: (value: ModelChoice) => void};
/** A current scoped read, never a model health probe or a dispatch action. */
export function ModelExecutionPanel(props: Props) {
  return props.codex ? <CodexModelExecutionPanel scope={props.scope} disabled={props.disabled} onChange={props.onChange}/> : <NativeModelExecutionPanel {...props}/>;
}
function NativeModelExecutionPanel({scope, supported, discover = false, disabled, onChange}: Props) {
  const api = useApi(); useLocale(); const id = useId();
  const [read, setRead] = useState<{api:typeof api;path:string;value:ModelExecutionOptions}>(), [error, setError] = useState<unknown>(), [loading, setLoading] = useState(false), [refresh, setRefresh] = useState(0);
  const [draft, setDraft] = useState<ModelDraft>({deployment: '', steps: '8', output: '4096', tokens: '', cost: ''});
  const path = modelOptionsPath(scope);
  const page=read?.api===api&&read.path===path?read.value:undefined;
  useEffect(() => {
    const abort = new AbortController(); setRead(undefined); setError(undefined);
    setDraft({deployment: '', steps: '8', output: '4096', tokens: '', cost: ''});
    if (!supported && !discover) {setLoading(false); return () => abort.abort();}
    setLoading(true);
    void api<ModelExecutionOptions>(path, undefined, 'GET', undefined, abort.signal).then(value => {
      if (abort.signal.aborted) return;
      if (!validModelOptions(value, scope)) throw new UiError('model-execution.invalid_options');
      setRead({api,path,value});
    }).catch(reason => {if (!abort.signal.aborted) setError(reason);}).finally(() => {if (!abort.signal.aborted) setLoading(false);});
    return () => abort.abort();
  }, [api, path, supported, discover, refresh]);
  const selection = page && modelSelection(page, draft);
  useEffect(() => {onChange({selection, ready: !supported && !discover || !loading && !error && (!!selection || !supported && discover && page?.supported === false && !page.codex_supported)});}, [supported, discover, page, draft, loading, error, onChange]);
  if (discover && page?.codex_supported) return <p className="muted">{tr('codex-execution.team_pending')}</p>;
  if (!supported && (!discover || page?.supported === false)) return <p className="muted">{t('fixed_tool')}</p>;
  const item = page?.items.find(item => item.deployment.deployment_sha256 === draft.deployment);
  return <section className="model-execution" aria-labelledby={id} aria-busy={loading}>
    <div className="model-execution-heading"><h3 id={id}>{t('title')}</h3><button type="button" aria-label={t('refresh')} disabled={disabled || loading} onClick={() => setRefresh(n => n + 1)}><RefreshCw size={14}/></button></div>
    <p className="muted">{t('intro')}</p>
    {loading && <Loading/>}{!!error && <ErrorNotice error={error}/>}
    {page && !page.items.length && <p>{t('empty')}</p>}
    {!!page?.items.length && <>
      <Field label={t('model')}><select data-model-select value={draft.deployment} disabled={disabled || loading} onChange={event => {
        const selected = page.items.find(option => option.deployment.deployment_sha256 === event.target.value);
        setDraft({deployment: event.target.value, steps: String(selected?.default_parameters.max_steps || 8), output: String(selected?.default_parameters.max_output_tokens || 4096), tokens: '', cost: ''});
      }}><option value="">{t('choose')}</option>{page.items.map(option => <option key={option.deployment.deployment_sha256} value={option.deployment.deployment_sha256}>{option.deployment.model} · {option.deployment.deployment_id} · v{option.deployment.revision}</option>)}</select></Field>
      {item && <>
        <div className="model-execution-grid"><Field label={t('steps')} hint={t('steps_hint')}><input data-model-steps type="number" min={1} max={16} step={1} value={draft.steps} disabled={disabled} onChange={event => setDraft({...draft, steps: event.target.value})}/></Field><Field label={t('output')} hint={t('output_hint')}><input data-model-output type="number" min={1} max={32768} step={1} value={draft.output} disabled={disabled} onChange={event => setDraft({...draft, output: event.target.value})}/></Field></div>
        <p className="muted">{item.cost_state === 'priced' ? t('priced', {currency: item.rate!.currency, version: formatNumber(item.rate!.version)}) : t('unpriced')}</p>
        <details><summary>{t('budget')}</summary><p className="muted">{t('budget_hint')}</p><div className="model-execution-grid"><Field label={t('tokens')} hint={t('optional')}><input data-model-token-budget inputMode="numeric" value={draft.tokens} disabled={disabled} onChange={event => setDraft({...draft, tokens: event.target.value})}/></Field><Field label={t('cost', {currency: item.rate?.currency || ''})} hint={item.rate ? t('optional') : t('price_first')}><input data-model-cost-budget inputMode="decimal" value={draft.cost} disabled={disabled || !item.rate} onChange={event => setDraft({...draft, cost: event.target.value})}/></Field></div><p className="muted">{t('policy')}</p></details>
        <details><summary>{t('basis')}</summary><dl><dt>{t('endpoint')}</dt><dd>{item.deployment.endpoint_origin}</dd><dt>{t('revision')}</dt><dd>{item.deployment.deployment_sha256}</dd></dl><p className="muted">{t('boundary')}</p></details>
        {!selection && <p role="status">{t('invalid_parameters')}</p>}
      </>}
    </>}
  </section>;
}
export function ModelExecutionFacts({binding}: {binding?: ModelBinding | null}) {
  useLocale(); if (!bindingPresent(binding)) return null;
  return <section className="model-execution" aria-label={t('frozen')}><h3>{t('frozen')}</h3><strong>{binding.deployment.model}</strong><p>{t('parameters', {steps: formatNumber(binding.parameters.max_steps), output: formatNumber(binding.parameters.max_output_tokens)})}</p><p className="muted">{binding.rate ? t('priced', {currency: binding.rate.currency, version: formatNumber(binding.rate.version)}) : t('unpriced')}</p>{binding.budget&&<p>{t('fixed_budget',{tokens:binding.budget.token_limit==null?t('inherited'):formatNumber(binding.budget.token_limit),cost:binding.budget.cost_limit_micros==null?t('inherited'):binding.budget.currency+' '+modelBudgetMoney(binding.budget.cost_limit_micros)})}</p>}<details><summary>{t('basis')}</summary><p>{binding.deployment.deployment_id} · v{binding.deployment.revision}</p><code>{binding.binding_sha256}</code><p>{t('reservation', {input: formatNumber(binding.input_token_reservation), output: formatNumber(binding.output_token_reservation)})}</p><p className="muted">{t('boundary')}</p></details></section>;
}

// Money remains an exact decimal string; budget arithmetic never uses floats.
function modelBudgetMoney(value:number){
 const micros=BigInt(value),fraction=String(micros%1000000n).padStart(6,'0').replace(/0+$/,'');
 return formatNumber(Number(micros/1000000n))+(fraction?'.'+fraction:'');
}
