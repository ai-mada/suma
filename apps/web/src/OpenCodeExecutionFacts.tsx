import {ArrowUpRight} from 'lucide-react';
import type {OpenCodeExecutionCapability} from '../../../packages/contracts/src';
import {formatDateTime,formatNumber,tr,useLocale} from './i18n';
import {validOpenCodeCapability} from './opencode-execution-model';
import type {OpenCodeRunFacts,OpenCodeRunTest} from './opencode-run-model';
import {Badge} from './ui';

const t=(key:string,values?:Record<string,unknown>)=>tr('opencode-execution.'+key,values);
/** Reuses the model facts treatment. Settings are fixed by the approved tool;
 * evidence, human acceptance and production readiness remain separate facts. */
export function OpenCodeExecutionFacts({capability,run,tests}:{capability?:OpenCodeExecutionCapability|null;run?:OpenCodeRunFacts;tests?:OpenCodeRunTest[]}) {
 useLocale();const fixed=run?.capability??capability;if(!validOpenCodeCapability(fixed)&&!run)return null;
 const validFixed=validOpenCodeCapability(fixed)?fixed:undefined,proof=run?.proof;
 const status=run?.phase==='unrecorded'?t('not_recorded'):run?t('run_'+run.phase):'';
 const tone=run?.phase==='completed'?'succeeded':run?.phase==='unknown'?'reconciling':run?.phase==='stopped'?'cancelled':run?.phase;
 const model=(provider?:string,model?:string)=>provider&&model?provider+'/'+model:t('not_recorded');
 return <section key={run?.identity??validFixed?.configuration_sha256} className={'model-execution'+(run?' opencode-run':'')} aria-label={t(run?'run_title':'title')} data-opencode-run-state={run?.phase}>
  {run?<div className="model-execution-heading opencode-run-heading"><h5>{t('run_title')}</h5><span role="status" className={'badge state-'+tone}><i/>{status}</span></div>:<h3>{t('title')}</h3>}
  {validFixed?<strong>{validFixed.provider_id}/{validFixed.model}</strong>:<p className="muted">{t('requested_model')}: {t('not_recorded')}</p>}
  {!run&&<><p className="muted">{t('fixed')}</p><p className="muted">{t('allowance')}</p></>}
  {run?.notice&&<p className={run.phase==='unknown'||run.notice==='run_platform_failed'?'warning':'muted'}>{t(run.notice)}</p>}
  {run&&<details className="opencode-run-evidence"><summary>{t('run_details')}</summary><dl>
   <dt>{tr('development.facts.tool_report')}</dt><dd><code>{run.terminal||t('not_recorded')}</code></dd>
   <dt>{t('requested_model')}</dt><dd>{proof?model(proof.requested_provider,proof.requested_model):model(validFixed?.provider_id,validFixed?.model)}</dd>
   <dt>{t('acknowledged_model')}</dt><dd>{model(proof?.acknowledged_provider,proof?.acknowledged_model)}</dd>
   <dt>{tr('development.facts.external_session_id')}</dt><dd><code>{run.session||t('not_recorded')}</code></dd>
   <dt>{t('user_message')}</dt><dd><code>{proof?.user_message_id||t('not_recorded')}</code></dd>
   <dt>{t('assistant_message')}</dt><dd><code>{proof?.assistant_message_id||t('not_recorded')}</code></dd>
   <dt>{t('finish')}</dt><dd><code>{proof?.finish||t('not_recorded')}</code></dd>
   <dt>{t('completed_at')}</dt><dd>{proof?.completed_at?formatDateTime(proof.completed_at,{dateStyle:'short',timeStyle:'medium'}):t('not_recorded')}</dd>
   <dt>{t('abort_acknowledged')}</dt><dd>{proof?t(proof.abort_acknowledged?'abort_yes':'abort_no'):t('not_recorded')}</dd>
  </dl>{proof&&<p className={run.positiveStop?'muted':'warning'}>{t(run.positiveStop?'stopped':'stop_unknown')}</p>}
  {run.session&&<p className="muted">{tr('development.facts.the_session_id_is_for_traceability_and')}</p>}
  </details>}
  {validFixed&&<details className="opencode-run-configuration"><summary>{t('basis')}</summary><dl>
   <dt>{t('runtime')}</dt><dd>OpenCode {validFixed.server_version} · SDK {validFixed.sdk_version}</dd>
   <dt>{t('endpoint')}</dt><dd>{validFixed.endpoint_origin}</dd>
   <dt>{t('configuration')}</dt><dd><code>{validFixed.configuration_sha256}</code></dd>
  </dl>{run&&<><p className="muted">{t('fixed')}</p><p className="muted">{t('allowance')}</p></>}<p className="muted">{t('identity')}</p>
  </details>}
  {run&&<details className="opencode-run-verification"><summary>{t('follow_up')}</summary><p className="muted">{t('verification_boundary')}</p>
   {!!tests?.length&&<><h5>{t('run_tests')}</h5>{tests.length>20&&<p className="muted">{t('run_tests_scope',{shown:formatNumber(20),total:formatNumber(tests.length)})}</p>}<div className="tool-event-list opencode-run-tests">{tests.slice(0,20).map(test=><button type="button" className="full" key={test.id} onClick={test.onOpen}><span>{test.name||test.id}</span><Badge state={test.state}/><ArrowUpRight size={14}/></button>)}</div></>}
  </details>}
 </section>;
}
