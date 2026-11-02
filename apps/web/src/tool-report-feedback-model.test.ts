import {expect,it} from 'vitest';
import type {AdapterDescriptor} from './AdapterConnections';
import {preparationForLocalTool,type ToolPreparationTarget} from './tool-preparation-target';
import {isNewToolReportFeedback,readToolReportFeedback,selectToolReportFeedback,toolReportFeedbackBaseline,toolReportFeedbackRemainingMs,validToolReportFeedbackSelection,validToolReportFeedbackTarget,type ToolReportFeedbackTarget} from './tool-report-feedback-model';

const target:ToolReportFeedbackTarget={project:'project-a',workerID:'worker-a',localTool:'codebuddy-cli'};
const descriptor=(localTool:ToolPreparationTarget['localTool']='codebuddy-cli'):AdapterDescriptor=>{
 const entry=preparationForLocalTool(localTool)!;
 return {id:localTool,product:localTool==='cursor-cli'?'Cursor':'Same product',entrypoint_kind:'cli',direction:'suma_to_tool',adapter:entry.adapter,protocol:entry.protocol,implementation:'implemented',setup_kind:'runner_profile',capabilities:[],source_refs:[],limitations:[]};
};
const catalog=(items=[descriptor()])=>({project_id:target.project,as_of:'2026-11-02T08:00:00Z',items});
const item={profile:{command_id:'codebuddy-team',adapter:'codebuddy',protocol:'codebuddy-jsonl-v1',profile_sha256:'a'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'1.0',read_only:false},available:true,reason:''};
const report=(changes:Record<string,unknown>={})=>({project_id:target.project,worker_id:target.workerID,state:'fresh',as_of:'2026-11-02T08:00:10Z',observed_at:'2026-11-02T08:00:00Z',expires_at:'2026-11-02T08:01:00Z',items:[item],...changes});
const read=(raw=report(),latency=0)=>readToolReportFeedback(catalog(),raw,target,target.project,latency)!;
const precise=(observed:string,asOf:string,state='fresh')=>report({state,observed_at:'2026-11-02T08:00:00.'+observed+'Z',as_of:'2026-11-02T08:00:00.'+asOf+'Z',expires_at:'2026-11-02T08:01:00.'+observed+'Z',items:state==='fresh'?[item]:[]});

it('binds all nine exact preparation tuples to the current project and worker without product aliases',()=>{
 for(const localTool of ['codex-jsonl','codebuddy-cli','dsh-multica','claude-code-cli','qoder-acp','opencode-sdk','cursor-cli','claude-code-sdk','codex-app-server'] as const){
  const current={...target,localTool};
  const view=readToolReportFeedback(catalog([descriptor(localTool)]),{project_id:target.project,worker_id:target.workerID,state:'unreported',as_of:'2026-11-02T08:00:00Z',items:[]},current,target.project,0);
  expect(view?.target.localTool).toBe(localTool);expect(view?.choices).toEqual([]);
 }
 for(const value of [{...target,project:'project-b'},{...target,workerID:''},{...target,localTool:'workbuddy'},{...target,token:'untrusted'},Object.assign(Object.create({}),target)])expect(validToolReportFeedbackTarget(value,target.project)).toBe(false);
 for(const patch of [{protocol:'claude-sdk-v1'},{implementation:'planned'},{setup_kind:'manual_guide'},{direction:'tool_to_suma'},{id:'claude-code-cli'}])expect(readToolReportFeedback(catalog([{...descriptor(),...patch} as AdapterDescriptor]),report(),target,target.project,0)).toBeUndefined();
 for(const raw of [report({project_id:'project-b'}),report({worker_id:'worker-b'}),report({unexpected:true}),report({items:[{...item,profile:{...item.profile,profile_sha256:'bad'}}]})])expect(readToolReportFeedback(catalog(),raw,target,target.project,0)).toBeUndefined();
 expect(readToolReportFeedback({...catalog(),project_id:'project-b'},report(),target,target.project,0)).toBeUndefined();
 expect(readToolReportFeedback(catalog(),report(),target,'project-b',0)).toBeUndefined();
});

it('requires a distinct inventory strictly beyond the fixed server boundary at nanosecond precision',()=>{
 const initial=read(precise('000000001','000000002')),baseline=toolReportFeedbackBaseline(initial);
 expect(isNewToolReportFeedback(read(precise('000000003','000000004')),baseline)).toBe(true);
 for(const raw of [precise('000000002','000000004'),precise('000000001','000000004'),precise('000000000','000000001')])expect(isNewToolReportFeedback(read(raw),baseline)).toBe(false);
 expect(isNewToolReportFeedback(read(precise('000000003','000000004')),{...baseline,target:{...target,workerID:'worker-b'}})).toBe(false);
 // An original future observation becoming fresh after clock recovery remains
 // the same report; neither an old ACK nor wall-clock catch-up counts as new.
 const futureBaseline=toolReportFeedbackBaseline(read(precise('000000003','000000001','stale')));
 expect(isNewToolReportFeedback(read(precise('000000003','000000004')),futureBaseline)).toBe(false);
 expect(isNewToolReportFeedback(read(precise('000000002','000000004')),futureBaseline)).toBe(false);
 const mutable={...target},view=readToolReportFeedback(catalog(),report(),mutable,target.project,0)!,copied=toolReportFeedbackBaseline(view);
 mutable.workerID='worker-b';expect(view.target.workerID).toBe('worker-a');expect(copied.target.workerID).toBe('worker-a');expect(Object.isFrozen(copied.target)).toBe(true);
 expect(isNewToolReportFeedback(initial,{...baseline,asOf:'invalid'})).toBe(false);
});

it('charges request and monotonic latency and allows only an explicit live available selection',()=>{
 const unavailable={...item,profile:{...item.profile,command_id:'offline'},available:false,reason:'not available'};
 const view=read(report({items:[item,unavailable]}),125);
 expect(view.choices.map(choice=>choice.profile.command_id)).toEqual(['codebuddy-team','offline']);expect(view.remainingMs).toBe(49875);
 expect(toolReportFeedbackRemainingMs(view,4875)).toBe(45000);
 const selection=selectToolReportFeedback(view,'codebuddy-team',4875)!;
 expect(selection).toEqual({project:target.project,workerID:target.workerID,adapterID:'codebuddy-cli',commandID:'codebuddy-team',profileSHA256:'a'.repeat(64),observedAt:'2026-11-02T08:00:00Z'});
 expect(validToolReportFeedbackSelection(selection,target)).toBe(true);
 expect(selectToolReportFeedback(view,'offline',0)).toBeUndefined();expect(selectToolReportFeedback(view,'missing',0)).toBeUndefined();expect(selectToolReportFeedback(view,'codebuddy-team',49875)).toBeUndefined();
 for(const invalid of [-1,NaN,Infinity]){expect(readToolReportFeedback(catalog(),report(),target,target.project,invalid)).toBeUndefined();expect(toolReportFeedbackRemainingMs(view,invalid)).toBe(0);}
 expect(read(report(),50001).remainingMs).toBe(0);
 const newer=read(precise('000000003','000000004'));
 expect(isNewToolReportFeedback(newer,toolReportFeedbackBaseline(read(precise('000000001','000000002'))),newer.remainingMs)).toBe(false);
 for(const patch of [{project:'project-b'},{workerID:'worker-b'},{adapterID:'claude-code-cli'},{profileSHA256:'A'.repeat(64)},{observedAt:'2026-02-30T08:00:00Z'},{commandID:''},{secret:'untrusted'}])expect(validToolReportFeedbackSelection({...selection,...patch},target)).toBe(false);
 const stale=read(report({state:'stale',as_of:'2026-11-02T08:01:00Z',items:[]}));expect(stale.choices).toEqual([]);expect(selectToolReportFeedback(stale,'codebuddy-team',0)).toBeUndefined();
});

it('reuses complete CLI and SDK capability gates instead of merging the same Claude product',()=>{
 const cliCapability={model:'claude-sonnet-4-6',model_source:'administrator_cli_argument',configuration_sha256:'c'.repeat(64),budget_mode:'observe_only',identity_evidence:'configuration_ack_only',auth_strategy:'existing_cli_login'};
 const sdkCapability={sdk_version:'0.3.314',cli_version:'2.1.314',model:'claude-sonnet-4-6',configuration_sha256:'d'.repeat(64),model_source:'administrator_sdk_option',budget_mode:'observe_only',identity_evidence:'sdk_init_model_ack_only',auth_strategy:'api_key_env_ref'};
 const cli={...item,profile:{...item.profile,command_id:'claude-cli',adapter:'claude',protocol:'claude-jsonl-v1',tool_version:'2.1.314',claude_execution:cliCapability}};
 const sdk={...item,profile:{...item.profile,command_id:'claude-sdk',adapter:'claude',protocol:'claude-sdk-v1',tool_version:'2.1.314',user_input_protocol:'claude-sdk-input-v1',claude_sdk_execution:sdkCapability}};
 for(const [localTool,commandID] of [['claude-code-cli','claude-cli'],['claude-code-sdk','claude-sdk']] as const){
  const view=readToolReportFeedback(catalog([descriptor(localTool)]),report({items:[cli,sdk]}),{...target,localTool},target.project,0)!;
  expect(view?.choices.map(choice=>choice.profile.command_id)).toEqual([commandID]);expect(selectToolReportFeedback(view,commandID,0)?.adapterID).toBe(localTool);
 }
 for(const patch of [{tool_version:'suma-claude-runtime 1.0.0'},{read_only:true},{claude_sdk_execution:undefined},{claude_sdk_execution:{...sdkCapability,sdk_version:'0.3.313'}},{user_input_protocol:'claude-jsonl-input-v1'}])expect(readToolReportFeedback(catalog([descriptor('claude-code-sdk')]),report({items:[{...sdk,profile:{...sdk.profile,...patch}}]}),{...target,localTool:'claude-code-sdk'},target.project,0)).toBeUndefined();
 expect(readToolReportFeedback(catalog([descriptor('claude-code-sdk')]),report(),{...target,localTool:'claude-code-sdk'},target.project,0)?.choices).toEqual([]);
});
