import type {ACPExecutionCapability,ToolEvidence} from '../../../packages/contracts/src';
import {Fragment} from 'react';
import {formatNumber,tr,useLocale} from './i18n';
import {validACPCapability,validACPEvidence} from './acp-execution-model';

const t=(key:string)=>tr('acp-runtime.facts.'+key);
/** Only the original execution evidence can acknowledge model selection or
 * stopping. A capability describes fixed configuration, never runtime health. */
export function ACPExecutionFacts({capability,evidence}:{capability?:ACPExecutionCapability|null;evidence?:ToolEvidence}){
 useLocale();const fixed=capability??evidence?.profile.acp_execution;
 if(!validACPCapability(fixed))return null;
 const raw=evidence?.acp_execution;
 const proof=evidence&&validACPEvidence(raw,fixed.runtime==='cursor'?'cursor':'qoder',evidence)&&raw.configuration_sha256===fixed.configuration_sha256&&raw.requested_model===fixed.model?raw:undefined;
 return <section className="model-execution" aria-label={t('title')} data-acp-execution-facts>
  <h3>{t('title')}</h3><dl><dt>{t('configured_model')}</dt><dd><code>{fixed.model}</code></dd></dl>
  <p className="muted">{t('configured_boundary')}</p><p className="muted">{t('identity_boundary')}</p><p className="muted">{t('budget_boundary')}</p>
  {raw&&!proof?<p className="warning" role="status">{t('invalid_evidence')}</p>:!proof?<p className="muted">{t('no_evidence')}</p>:<>
   {proof.model_selection_acknowledged?<p>{t('acknowledged_model')}: <code>{proof.selected_model}</code></p>:<p className="warning">{t('acknowledgement_pending')}</p>}
   <dl>{([['prompt_completed',proof.prompt_completed],['tools_complete',proof.tools_complete],['cancel_requested',proof.cancel_requested],['host_stopped',proof.host_process_stopped],['tree_stopped',proof.host_process_tree_stopped]] as const).map(([label,known])=><Fragment key={label}><dt>{t(label)}</dt><dd>{t(known?'confirmed':'not_confirmed')}</dd></Fragment>)}<dt>{t('permissions')}</dt><dd>{formatNumber(proof.permissions_requested)}</dd><dt>{t('permission_denials')}</dt><dd>{formatNumber(proof.permissions_denied)}</dd></dl>
   {proof.host_stopped_after_rpc&&<p>{t('host_stopped_after_rpc')}</p>}
   <details><summary>{t('source')}</summary><code>{proof.configuration_sha256}</code>{proof.session_id&&<p><code>{proof.session_id}</code></p>}{proof.agent_name&&<p>{proof.agent_name}{proof.agent_version?' · '+proof.agent_version:''}</p>}{proof.stop_reason&&<p>{t('stop_reason')}: {t('stop.'+proof.stop_reason)} · <code>{proof.stop_reason}</code></p>}<p>{t('host_exit')}: {proof.host_process_exit_code===undefined?t('exit_unrecorded'):<code data-acp-host-exit>{String(proof.host_process_exit_code)}</code>}</p></details>
  </>}
  <p className="muted">{t('delivery_boundary')}</p>
 </section>;
}
