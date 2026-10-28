import {useLocale,tr,formatNumber} from './i18n';
import type {ModelBinding,ModelEvidence} from '../../../packages/contracts/src';
import {gatewayFacts,usageCounters} from './model-gateway-model';
const t=(key:string,values?:Record<string,unknown>)=>tr('model-gateway.'+key,values);
/** Current-attempt gateway evidence only. The view never infers ledger
 * settlement, releases a hold or treats cancellation as zero consumption. */
export function ModelGatewayFacts({evidence,binding,project,required=false}:{evidence?:ModelEvidence|null;binding?:ModelBinding|null;project?:string;required?:boolean}){
 const locale=useLocale(),facts=gatewayFacts(evidence,binding),proof=facts.evidence;if(!evidence&&!required)return null;
 return <section className="model-execution" aria-label={t('title')} data-model-gateway-state={facts.coverage}>
  <div className="model-execution-heading"><h4>{t('title')}</h4><span className={'badge state-'+(facts.coverage==='complete'?'succeeded':'reconciling')} role="status">{t('coverage_'+facts.coverage)}</span></div>
  {proof&&<><strong>{proof.model}</strong><p>{t(proof.gateway_closed?'gateway_closed':'gateway_open')} · {t('request_count',{count:formatNumber(proof.requests.length)})}</p><p className="muted">{t('usage_'+facts.usageState)}</p>{facts.modelChanged&&<p className="warning">{t('model_changed')}</p>}
   <details><summary>{t('requests')}</summary>{proof.requests.length?<ul className="tool-event-list">{proof.requests.map(item=><li key={item.step}><strong>{t('step',{step:formatNumber(item.step+1)})} · {t('request_'+item.state)}</strong><dl><dt>{t('returned_model')}</dt><dd>{item.provider_model||t('not_reported')}</dd><dt>{t('provider_request')}</dt><dd><code>{item.provider_request_id||t('not_reported')}</code></dd><dt>{t('allowance')}</dt><dd>{formatNumber(item.output_allowance)}</dd><dt>{t('request_digest')}</dt><dd><code>{item.request_sha256}</code></dd><dt>{t('response_digest')}</dt><dd><code>{item.response_sha256||t('not_reported')}</code></dd>{usageCounters.map(key=><div key={key}><dt>{t('counter_'+key)}</dt><dd>{item.usage?.[key]===undefined?t('not_reported'):formatNumber(item.usage[key]!)}</dd></div>)}</dl></li>)}</ul>:<p className="muted">{t('no_requests')}</p>}</details>
   <details><summary>{t('basis')}</summary><dl><dt>{t('binding')}</dt><dd><code>{proof.binding_sha256}</code></dd><dt>{t('deployment')}</dt><dd><code>{proof.deployment_sha256}</code></dd><dt>{t('parameters')}</dt><dd><code>{proof.parameters_sha256}</code></dd>{usageCounters.map(key=><div key={key}><dt>{t('counter_'+key)}</dt><dd>{facts.totals[key]===undefined?t('not_reported'):new Intl.NumberFormat(locale).format(facts.totals[key]!)}</dd></div>)}</dl><p className="muted">{t('total_scope')}</p></details>
  </>}
  <p className="muted">{t('accounting_boundary')}</p>{project&&<a href={'?'+new URLSearchParams({project,view:'settings',sub:'usage'})} onClick={event=>{if(!window.dispatchEvent(new Event('suma:navigate',{cancelable:true})))event.preventDefault();}}>{t('open_usage')}</a>}
 </section>;
}
