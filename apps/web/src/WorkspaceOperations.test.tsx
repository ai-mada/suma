// @vitest-environment jsdom
// Validate the host projection and the actual existing history consumer once;
// no SDK, subprocess, model or secondary event endpoint is involved.
import {act,type ReactNode} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {beforeEach,afterEach,expect,it,vi} from 'vitest';
import type {WorkspaceOperationEvent,NativeWorkspaceEvidence,Event as SavedEvent,ExecutionAttempt} from '../../../packages/contracts/src';
import {workspaceOperationsForAttempt,groupWorkspaceOperations,workspaceOperationFailed,validWorkspaceOperation,savedHostOperationMatchesSource} from './workspace-operations-model';
import {nativeCheckForOperation} from './NativeWorkspaceFacts';
import {ToolExecutionFacts} from './ToolFacts';
import {WorkspaceOperations} from './WorkspaceOperations';
import {AttemptToolEventHistory} from './AttemptToolEventHistory';
import {savedToolDisplayMatchesSource,type AttemptToolScope} from './attempt-tool-history';
import {toolFactsForAttempt} from './tool-model';
import {setLocale} from './i18n';
import type {WorkspaceEvidence} from './api';

const scope:AttemptToolScope={project_id:'p',run_id:'r',node_id:'n',attempt:1,fence:7,worker_id:'w'},profile='a'.repeat(64),policy='b'.repeat(64),start='2026-10-29T01:00:00Z',end='2026-10-29T01:00:01Z';
function operation(change:Partial<WorkspaceOperationEvent>={}):WorkspaceOperationEvent{return {protocol:'suma-workspace-operation-v1',source:'go_workspace_host',schema_version:1,event_id:'c'.repeat(64),sequence:1,operation_id:'host-1',revision:1,action:'read',phase:'started',generation_before:2,generation_after:2,started_at:start,recorded_at:start,request_sha256:'d'.repeat(64),policy_sha256:policy,profile_sha256:profile,...change};}
function terminal(change:Partial<WorkspaceOperationEvent>={}):WorkspaceOperationEvent{return operation({event_id:'e'.repeat(64),sequence:2,revision:2,phase:'completed',completed_at:end,recorded_at:end,result_sha256:'f'.repeat(64),...change});}
function saved(op:WorkspaceOperationEvent,sequence=op.sequence):SavedEvent{const data={attempt:scope.attempt,fence:scope.fence,worker_id:scope.worker_id,host_operation:op};return {sequence,run_id:scope.run_id,node_id:scope.node_id,type:'tool_progress',at:'2026-10-29T01:00:02Z',data,raw_data:JSON.stringify(data)};}
const project=(events:SavedEvent[],identity=scope)=>workspaceOperationsForAttempt(events,identity,profile);
function evidence():WorkspaceEvidence{const native:NativeWorkspaceEvidence={schema_version:1,policy_sha256:policy,generation:2,state:'closed',files:[],checks:[{operation_id:'effect-check',check_id:'unit',profile_sha256:'1'.repeat(64),generation:2,workspace_sha256:'2'.repeat(64),exit_code:1,stopped:false,outcome_unknown:false,output:'<script>private output {{literal}}</script>',output_truncated:false}],operation_trace:{invocation_count:3,record_count:6,pending_count:0,max_invocations:2048,max_records:4096,state:'complete'}};return {native_workspace:native};}
let host:HTMLDivElement,root:Root;
beforeEach(async()=>{await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.stubGlobal('fetch',vi.fn());host=document.createElement('div');document.body.append(host);root=createRoot(host);});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();await setLocale('zh-CN',{persist:false});vi.unstubAllGlobals();});
async function render(node:ReactNode){await act(async()=>root.render(node));}
async function click(selector:string){const element=host.querySelector<HTMLButtonElement>(selector);expect(element).not.toBeNull();await act(async()=>element!.click());}
async function settle(){for(let i=0;i<4;i++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}

it('accepts only the original exclusive host source in an exact attempt and keeps SDK siblings raw-only',()=>{
 const good=saved(operation()),wrongScope={...good,data:{...good.data,fence:8}},mixed={...good,data:{...good.data,tool_event:{schema_version:1,kind:'message',command_id:'codex',profile_sha256:profile,protocol:'suma-jsonl-v1',message:'SDK text'}}};mixed.raw_data=JSON.stringify(mixed.data);
 const mismatch={...good,raw_data:JSON.stringify({...good.data,host_operation:operation({path:'other.ts'})})};
 const duplicate={...good,raw_data:good.raw_data!.replace('"host_operation":','"host_operation":{},"host_operation":')};
 const sourceScope={...good,raw_data:good.raw_data!.replace('"fence":7','"fence":8')};
 const projection=project([good,wrongScope,mixed,mismatch,duplicate,sourceScope]);expect(projection.facts.map(item=>item.operation)).toEqual([good.data.host_operation]);expect(projection.invalid).toBe(4);
 expect(savedHostOperationMatchesSource(mixed)).toBe(false);expect(savedToolDisplayMatchesSource(mixed)).toBe(false);expect(toolFactsForAttempt([mixed],{id:'n',...scope})).toEqual([]);
 expect(validWorkspaceOperation({...operation(),path:'../secret'})).toBe(false);expect(validWorkspaceOperation({...operation(),command:'hidden'})).toBe(false);expect(validWorkspaceOperation({...terminal(),revision:1})).toBe(false);
 expect(project([good],{...scope,attempt:2}).facts).toEqual([]);expect(workspaceOperationsForAttempt([good],scope,'9'.repeat(64)).invalid).toBe(1);
});

it('orders revisions independently from arrival or content generation and refuses contradictory completion claims',()=>{
 const first=saved(operation()),last=saved(terminal({path:'src/main.ts',effect_operation_id:'effect-1'})),group=groupWorkspaceOperations(project([last,first,last]).facts)[0];expect(group.facts).toHaveLength(2);expect(group.latest.operation.revision).toBe(2);expect(group.inconsistent).toBe(false);expect(group.missingStart).toBe(false);expect(group.pendingEnd).toBe(false);
 expect(groupWorkspaceOperations(project([last]).facts)[0].missingStart).toBe(true);
 const conflicting=saved(terminal({path:'src/other.ts'}),4),invalid=groupWorkspaceOperations(project([first,last,conflicting]).facts)[0];expect(invalid.inconsistent).toBe(true);expect(workspaceOperationFailed(invalid)).toBe(false);
 const changed=saved(terminal({request_sha256:'0'.repeat(64)}));expect(groupWorkspaceOperations(project([first,changed]).facts)[0].inconsistent).toBe(true);
 const failure=groupWorkspaceOperations(project([saved(terminal({action:'check',exit_code:3,stopped:false}))]).facts)[0];expect(failure.latest.operation.phase).toBe('completed');expect(workspaceOperationFailed(failure)).toBe(true);
});

it('keeps current facts folded and scoped, filters real outcomes and preserves disclosure/focus on language-only changes',async()=>{
 const rows=[saved(operation()),saved(terminal({operation_id:'failed-check',action:'check',phase:'failed',error_code:'native_workspace_check_failed',check_id:'unit',exit_code:1,stopped:false})),saved(terminal({operation_id:'unknown-remove',action:'remove',phase:'unknown',completed_at:undefined,error_code:'future_reason'}),3)];
 await render(<ToolExecutionFacts state="running" workspaceOperations={{scope,events:rows,evidence:evidence(),profileSHA:profile}}/>);expect(host.querySelector('.wo-row')).toBeNull();expect(host.querySelector('[data-wo-toggle]')?.getAttribute('aria-expanded')).toBe('false');await click('[data-wo-toggle]');expect(host.querySelectorAll('.wo-row')).toHaveLength(3);expect(host.textContent).toContain('This page or live window may contain only part of the history');
 const select=host.querySelector<HTMLSelectElement>('[data-wo-filter]')!;await act(async()=>{select.value='failed';select.dispatchEvent(new Event('change',{bubbles:true}));});expect(host.querySelectorAll('.wo-row')).toHaveLength(1);const row=host.querySelector<HTMLDetailsElement>('.wo-row')!,summary=row.querySelector('summary')!;row.open=true;summary.focus();expect(row.textContent).toContain('nonzero exit code');
 await act(async()=>setLocale('zh-CN',{persist:false}));expect(row.isConnected).toBe(true);expect(row.open).toBe(true);expect(document.activeElement).toBe(summary);expect(host.textContent).toContain('开发检查返回非零退出码');expect(fetch).not.toHaveBeenCalled();
 const nextScope={...scope,attempt:2};await render(<ToolExecutionFacts state="running" workspaceOperations={{scope:nextScope,events:rows,evidence:evidence(),profileSHA:profile}}/>);expect(row.isConnected).toBe(false);expect(host.querySelector('[data-wo-toggle]')?.getAttribute('aria-expanded')).toBe('false');await click('[data-wo-toggle]');expect(host.querySelectorAll('.wo-row')).toHaveLength(0);expect(host.textContent).toContain('不证明服务端已接收确认');
});

it('opens only the exact effect generation and check snapshot, while keeping parent and check profiles distinct',async()=>{
 const value=evidence(),op=terminal({action:'check',check_id:'unit',effect_operation_id:'effect-check',phase:'failed',error_code:'native_workspace_check_failed',exit_code:1,stopped:false});
 expect(nativeCheckForOperation(value,op)?.profile_sha256).toBe('1'.repeat(64));
 for(const change of [{effect_operation_id:'other-effect'},{generation_after:1},{policy_sha256:'9'.repeat(64)},{check_id:'other-check'},{stopped:true},{revision:1}])expect(nativeCheckForOperation(value,{...op,...change})).toBeUndefined();
 await render(<WorkspaceOperations scope={scope} projection={project([saved(op)])} workspaceEvidence={value} context="current"/>);await click('[data-wo-toggle]');const row=host.querySelector<HTMLDetailsElement>('.wo-row')!;row.open=true;expect(row.textContent).toContain('Check command profile SHA-256');expect(row.textContent).toContain('Developer tool profile SHA-256');expect(row.textContent).toContain('Independent tests and acceptance remain separate.');
 const output=row.querySelector<HTMLDetailsElement>('.native-workspace-output')!;await act(async()=>{output.open=true;output.dispatchEvent(new Event('toggle'));});expect(output.querySelector('pre')?.textContent).toBe('<script>private output {{literal}}</script>');expect(host.querySelector('script')).toBeNull();
 const native=value.native_workspace as NativeWorkspaceEvidence;native.checks[0].generation=1;await render(<WorkspaceOperations scope={scope} projection={project([saved(op)])} workspaceEvidence={value} context="current"/>);expect(host.querySelector('.native-workspace-output')).toBeNull();expect(host.textContent).toContain('Another result with the same check name is not substituted');
});

it('reuses exact persisted history and source links without extra reads, then removes host facts on access loss',async()=>{
 const attempt:ExecutionAttempt={run_id:'r',node_id:'n',attempt:1,fence:7,worker_id:'w',state:'running',started_at:start,completed_at:'',result_source:'none'},items=[saved(operation(),41),saved(terminal(),42)],body={schema_version:1,...scope,items,watermark:100,scanned_through:100,has_more:false,page_cursor:'first-window'};
 let status=200;vi.mocked(fetch).mockImplementation(async()=>new Response(JSON.stringify(status===200?body:{error:{code:'forbidden',message:'grant lost'}}),{status,headers:{'Content-Type':'application/json'}}));
 await render(<AttemptToolEventHistory project="p" attempt={attempt}/>);await settle();expect(fetch).toHaveBeenCalledTimes(1);expect(String(vi.mocked(fetch).mock.calls[0][0])).toContain('/attempts/1/tool-events?project_id=p');await click('[data-wo-toggle]');expect(host.textContent).toContain('Historical attempt 1');expect(host.querySelectorAll('.wo-row')).toHaveLength(1);
 const source=[...host.querySelectorAll<HTMLButtonElement>('.wo-events button')][0];await act(async()=>source.click());const original=host.querySelector<HTMLDetailsElement>('[data-saved-tool-event="41"] [data-saved-raw]')!;expect(original.open).toBe(true);expect(document.activeElement).toBe(original.querySelector('summary'));expect(host.querySelector('[data-saved-tool-event="41"] [data-raw-event-data]')?.textContent).toBe(items[0].raw_data);
 await act(async()=>setLocale('zh-CN',{persist:false}));expect(fetch).toHaveBeenCalledTimes(1);expect(original.open).toBe(true);expect(document.activeElement).toBe(original.querySelector('summary'));await act(async()=>setLocale('en-US',{persist:false}));status=403;const refresh=[...host.querySelectorAll<HTMLButtonElement>('button')].find(button=>button.textContent==='Read latest saved facts')!;await act(async()=>refresh.click());await settle();expect(host.querySelector('.workspace-operations')).toBeNull();expect(host.querySelectorAll('[data-saved-tool-event]')).toHaveLength(0);expect(host.textContent).toContain('Access to saved facts was lost');
});
