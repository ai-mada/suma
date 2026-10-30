import type {ClaudeExecutionCapability,ToolEvidence} from '../../../packages/contracts/src';
import {tr,useLocale,formatNumber} from './i18n';
import {validClaudeCapability} from './claude-cli-model';
export function ClaudeExecutionFacts({capability,evidence}:{capability?:ClaudeExecutionCapability|null;evidence?:ToolEvidence}){
 useLocale();const value=capability??evidence?.profile.claude_execution;
 if(!validClaudeCapability(value))return null;
 const proof=evidence?.claude_execution;
 return <section className="model-execution" aria-label={tr('claude-cli.title')}>
  <h3>{tr('claude-cli.title')}</h3>
  <dl><dt>{tr('claude-cli.configured_model')}</dt><dd><code>{value.model}</code></dd>{proof?.acknowledged_model&&<><dt>{tr('claude-cli.acknowledged_model')}</dt><dd><code>{proof.acknowledged_model}</code></dd></>}</dl>
  <p className="muted">{tr('claude-cli.fixed_policy')}</p>
  {proof&&proof.permission_denial_count>0&&<p className="warning">{tr('claude-cli.permission_denied',{count:formatNumber(proof.permission_denial_count)})}</p>}
  {proof&&(proof.deferred_tool_use||(proof.queued_turn_count??0)>0||proof.result_terminal_reason&&proof.result_terminal_reason!=='completed')&&<p className="warning">{tr('claude-cli.incomplete')}</p>}
 </section>;
}
