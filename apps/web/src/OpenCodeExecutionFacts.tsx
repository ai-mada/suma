import type {OpenCodeExecutionCapability,OpenCodeExecutionEvidence} from '../../../packages/contracts/src';
import {formatDateTime,tr,useLocale} from './i18n';
import {validOpenCodeCapability} from './opencode-execution-model';

const t=(key:string)=>tr('opencode-execution.'+key);
/** Reuses the model facts treatment. Settings are fixed by the approved tool;
 * evidence, human acceptance and production readiness remain separate facts. */
export function OpenCodeExecutionFacts({capability,evidence}:{capability?:OpenCodeExecutionCapability|null;evidence?:OpenCodeExecutionEvidence|null}) {
 useLocale();if(!validOpenCodeCapability(capability))return null;
 return <section className="model-execution" aria-label={t('title')}>
  <h3>{t('title')}</h3><strong>{capability.provider_id}/{capability.model}</strong>
  <p className="muted">{t('fixed')}</p><p className="muted">{t('allowance')}</p>
  <details><summary>{t('basis')}</summary><dl>
   <dt>{t('runtime')}</dt><dd>OpenCode {capability.server_version} · SDK {capability.sdk_version}</dd>
   <dt>{t('endpoint')}</dt><dd>{capability.endpoint_origin}</dd>
   <dt>{t('configuration')}</dt><dd><code>{capability.configuration_sha256}</code></dd>
  </dl><p>{t('identity')}</p>
  {evidence&&<><p>{evidence.server_stopped?t('stopped'):t('stop_unknown')}</p>{evidence.completed_at&&<p>{formatDateTime(evidence.completed_at,{dateStyle:'short',timeStyle:'medium'})}</p>}<code>{evidence.session_id}</code></>}
  </details>
 </section>;
}
