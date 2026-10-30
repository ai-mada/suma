import type {ModelBinding,CodexBinding,OpenCodeExecutionCapability,ToolEvidence} from '../../../packages/contracts/src';
import {ModelExecutionFacts} from './ModelExecutionPanel';
import {CodexExecutionFacts} from './CodexModelExecutionPanel';
import {OpenCodeExecutionFacts} from './OpenCodeExecutionFacts';
import {OctopExecutionFacts} from './OctopExecutionFacts';
import {ClaudeExecutionFacts} from './ClaudeExecutionFacts';
import type {ClaudeExecutionCapability} from '../../../packages/contracts/src';
import {ModelGatewayFacts} from './ModelGatewayFacts';
import type {OpenCodeRunFacts,OpenCodeRunTest} from './opencode-run-model';
import type {OctopExecutionCapability} from './octop-execution-model';
type Props={binding?:ModelBinding|null;codex?:CodexBinding|null;openCode?:OpenCodeExecutionCapability|null;octop?:OctopExecutionCapability|null;claude?:ClaudeExecutionCapability|null;evidence?:ToolEvidence;state?:string;openCodeRun?:OpenCodeRunFacts;tests?:OpenCodeRunTest[];project?:string};
/** Shared mounting across tasks, expert plans and repair plans. Each driver
 * keeps its own receipt semantics; all platform models share one binding. */
export function ExecutionModelFacts({binding,codex,openCode,octop,claude,evidence,state,openCodeRun,tests,project}:Props){
 return <><ModelExecutionFacts binding={binding}/><CodexExecutionFacts binding={codex} evidence={evidence?.codex_execution}/><OpenCodeExecutionFacts capability={openCode} run={openCodeRun} tests={tests}/><ClaudeExecutionFacts capability={claude} evidence={evidence}/><ModelGatewayFacts binding={binding} evidence={evidence?.model_execution} project={project} required={!!binding&&state!==undefined}/>{(octop||evidence?.profile.adapter==='octop')&&<OctopExecutionFacts capability={octop??evidence?.profile.octop_execution} evidence={evidence} state={state}/>}</>;
}
