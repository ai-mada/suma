import type {ClaudeSDKExecutionCapability,ToolEvidence} from '../../../packages/contracts/src';
import {Fragment} from 'react';
import {formatNumber,tr,useLocale} from './i18n';
import {validClaudeSDKCapability,validClaudeSDKEvidence} from './claude-sdk-execution-model';
const t=(key:string)=>tr('claude-sdk.'+key);
export function ClaudeSDKExecutionFacts({capability,evidence}:{capability?:ClaudeSDKExecutionCapability|null;evidence?:ToolEvidence}){
 useLocale();const fixed=capability??evidence?.profile.claude_sdk_execution;
 if(!validClaudeSDKCapability(fixed))return null;
 const raw=evidence?.claude_sdk_execution,proof=evidence&&validClaudeSDKEvidence(raw,fixed,evidence)?raw:undefined;
 return <section className="model-execution" data-claude-sdk-execution-facts aria-label={t('title')}>
  <h3>{t('title')}</h3><dl className="model-execution-grid"><dt>{t('configured_model')}</dt><dd><code>{fixed.model}</code></dd><dt>{t('versions')}</dt><dd>SDK {fixed.sdk_version} / CLI {fixed.cli_version}</dd></dl>
  <p className="muted">{t('fixed_boundary')}</p><p className="muted">{t('usage_boundary')}</p>
  {evidence&&!proof?<p className="warning" role="status" data-claude-sdk-evidence="invalid">{t('invalid_evidence')}</p>:!proof?<p className="muted" data-claude-sdk-evidence="unrecorded">{t('unrecorded')}</p>:<div data-claude-sdk-evidence="matched">
   <dl className="model-execution-grid"><dt>{t('acknowledged_model')}</dt><dd>{proof.acknowledged_model?<code>{proof.acknowledged_model}</code>:t('not_confirmed')}</dd>
    {([['turn_ended',proof.turn_ended],['tools_completed',proof.tools_completed],['cancel_requested',proof.cancel_requested],['host_stopped',proof.host_process_stopped],['tree_stopped',proof.host_process_tree_stopped]] as const).map(([key,value])=><Fragment key={key}><dt>{t(key)}</dt><dd>{t(value?'confirmed':'not_confirmed')}</dd></Fragment>)}
    <dt>{t('permissions')}</dt><dd>{formatNumber(proof.permission_requests)}</dd><dt>{t('denied_callbacks')}</dt><dd>{formatNumber(proof.denied_callbacks)}</dd><dt>{t('native_permission_denials')}</dt><dd data-claude-sdk-native-denials>{proof.native_permission_denials===undefined?t('not_recorded'):formatNumber(proof.native_permission_denials)}</dd><dt>{t('questions')}</dt><dd>{formatNumber(proof.user_inputs?.length??0)}</dd>
   </dl><p className="muted">{t('delivery_boundary')}</p>
   <details><summary>{t('source')}</summary><p>{t('session')}: <code data-claude-sdk-session>{proof.session_id||t('not_confirmed')}</code></p><p>{t('configuration')}: <code>{proof.configuration_sha256}</code></p><p>{t('result')}: <code>{proof.result_subtype||t('not_confirmed')}</code></p></details>
  </div>}
  <p className="muted">{t('quality_boundary')}</p>
 </section>;
}
