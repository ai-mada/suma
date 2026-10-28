import type {ToolEvidence} from './tool-model';
import {octopRunFacts,validOctopCapability,type OctopExecutionCapability} from './octop-execution-model';
import {tr,useLocale,formatDateTime} from './i18n';
const t=(key:string,values?:Record<string,unknown>)=>tr('octop-execution.'+key,values);
/** Compact configuration and execution facts; no control, retry or inference
 * of provider identity is performed by this presentation component. */
export function OctopExecutionFacts({capability,evidence,state}:{capability?:OctopExecutionCapability|null;evidence?:ToolEvidence;state?:string}){
 useLocale();const run=typeof state==='string'?octopRunFacts(state,evidence):undefined,fixed=run?.capability??(validOctopCapability(capability)?capability:undefined);
 if(!fixed&&!run)return null;
 const proof=run?.proof,tone=run?.phase==='completed'?'succeeded':run?.phase==='stopped'?'cancelled':run?.phase==='failed'?'failed':'reconciling';
 return <section className="model-execution" aria-label={t(run?'run_title':'title')} data-octop-run-state={run?.phase}>
  <div className="model-execution-heading"><h4>{t(run?'run_title':'title')}</h4>{run&&<span className={'badge state-'+tone} role="status">{t('phase_'+run.phase)}</span>}</div>
  {fixed&&<strong>{fixed.provider_id}/{fixed.model}</strong>}
  <p className="muted">{t('configuration_only')}</p>
  <p className="muted">{t('observe_only')}</p>
  {run&&<dl><dt>{t('protocol_stop')}</dt><dd><code>{proof?.stop_reason||t('not_recorded')}</code></dd><dt>{t('process_stop')}</dt><dd>{proof?t(proof.process_stopped?'process_stopped':'process_unknown'):t('not_recorded')}</dd><dt>{t('cancel_request')}</dt><dd>{proof?t(proof.cancel_requested?'cancel_requested':'cancel_not_requested'):t('not_recorded')}</dd></dl>}
  <details><summary>{t(run?'details':'configuration_details')}</summary><dl>
   {fixed&&<><dt>{t('runtime')}</dt><dd>{t('runtime_versions',{harness:fixed.harness_version,sdk:fixed.acp_sdk_version})}</dd><dt>{t('endpoint')}</dt><dd>{fixed.endpoint_origin}</dd><dt>{t('configuration')}</dt><dd><code>{fixed.configuration_sha256}</code></dd></>}
   {run&&<><dt>{tr('development.facts.tool_report')}</dt><dd><code>{evidence?.terminal||t('not_recorded')}</code></dd><dt>{t('requested_model')}</dt><dd>{proof?proof.requested_provider+'/'+proof.requested_model:t('not_recorded')}</dd><dt>{t('acknowledged_model')}</dt><dd>{proof?.acknowledged_provider&&proof?.acknowledged_model?proof.acknowledged_provider+'/'+proof.acknowledged_model:t('not_recorded')}</dd><dt>{t('session')}</dt><dd><code>{proof?.session_id||t('not_recorded')}</code></dd><dt>{t('completed_at')}</dt><dd>{proof?.completed_at?formatDateTime(proof.completed_at,{dateStyle:'short',timeStyle:'medium'}):t('not_recorded')}</dd></>}
  </dl><p className="muted">{t('workspace_boundary')}</p><p className="muted">{t('stop_boundary')}</p></details>
 </section>;
}
