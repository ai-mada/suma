// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {setLocale} from './i18n';
import {cloudRunnerPhase,type CloudRunFacts} from './workbuddy-cloud-task-model';
import {WorkBuddyCloudTaskPanel,type WorkBuddyCloudTaskModel} from './WorkBuddyCloudTaskPanel';
import {AdmissionFacts,AttemptFacts} from './SchedulingFacts';
import type {NodeAdmission} from './scheduling-model';

const execution=(change:Partial<CloudRunFacts>={}):CloudRunFacts=>({runVersion:7,runState:'running',nodeState:'running',reason:'',attempt:1,fence:2,workerID:'formal-worker',...change});
const model=():WorkBuddyCloudTaskModel=>({scopeKey:'owner/org/project/run/cloud',sourceVersion:'version7',state:'ready',task:{title:'Formal cloud task',runLabel:'Run original',nodeLabel:'Cloud original',prompt:'Approved immutable request',requestDigest:'a'.repeat(64),nodeDigest:'b'.repeat(64)},connections:[],actions:{delegate:false,revoke:false,refresh:true},delegation:{id:'delegation',version:2,state:'active',expiresAt:'2030-01-01T00:00:00Z',connectionLabel:'Fixed account',createsConsumed:1},submission:{id:'submission',version:2,state:'task_known',taskID:'provider-original',providerStatus:'completed',observedAt:'2026-10-30T00:00:00Z'},execution:execution({nodeState:'waiting',reason:'cloud.delivery_required'})});

it('describes a claimed node as awaiting submission facts without inventing provider creation',()=>{
 expect(cloudRunnerPhase({execution:execution(),delegation:{state:'active'}})).toBe('awaiting_submission');
 expect(cloudRunnerPhase({execution:execution({attempt:0,fence:0,workerID:''}),delegation:{state:'active'}})).toBe('waiting_qualification');
});
it('keeps prepared and unknown submissions uncertain even while a live node is running',()=>{
 for(const state of ['prepared','unknown'])expect(cloudRunnerPhase({execution:execution(),submission:{state}})).toBe('unknown');
});
it('keeps delivery, disconnected leases and cancellation distinct without a success projection',()=>{
 expect(cloudRunnerPhase({execution:execution({nodeState:'waiting',reason:'cloud.delivery_required'}),submission:{state:'task_known'}})).toBe('delivery_required');
 expect(cloudRunnerPhase({execution:execution({nodeState:'waiting',reason:'cloud.runner_disconnected'}),submission:{state:'task_known'}})).toBe('runner_disconnected');
 expect(cloudRunnerPhase({execution:execution({runState:'cancelling',nodeState:'waiting',reason:'cloud.stop_unconfirmed'}),submission:{state:'task_known'}})).toBe('stop_unconfirmed');
});
it('does not derive platform completion from a provider completed report',()=>{
 expect(cloudRunnerPhase({submission:{state:'task_known',providerStatus:'completed'}})).toBe('known');
});
it('shows a trusted no-creation failure without treating an existing submission as absent',()=>{
 expect(cloudRunnerPhase({execution:execution({nodeState:'failed',reason:'cloud.not_created'}),delegation:{state:'active'}})).toBe('not_created');
 expect(cloudRunnerPhase({execution:execution({nodeState:'failed',reason:'cloud.not_created'}),submission:{state:'prepared'}})).toBe('unknown');
});

let host:HTMLDivElement,root:Root|undefined;
beforeEach(async()=>{await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);});
afterEach(async()=>{if(root)await act(async()=>root?.unmount());root=undefined;host.remove();await setLocale('zh-CN',{persist:false});vi.unstubAllGlobals();});
async function render(value:WorkBuddyCloudTaskModel,onReload=vi.fn(async()=>{})){
 await act(async()=>{root??=createRoot(host);root.render(<WorkBuddyCloudTaskPanel model={value} onDelegate={async()=>({kind:'confirmed'})} onRevoke={async()=>({kind:'confirmed'})} onRefresh={async()=>({kind:'confirmed'})} onReload={onReload}/>);});
}

it('keeps platform pause visible beside an existing provider task without replacing a critical reason',async()=>{
 const value=model();value.execution=execution({runState:'paused',nodeState:'waiting'});
 for(const [reason,phase] of [['','known'],['cloud.runner_disconnected','runner_disconnected']]){
  value.execution={...value.execution!,reason};await render(value);
  expect(host.querySelector('[data-cloud-runner-phase]')?.getAttribute('data-cloud-runner-phase')).toBe(phase);
  const pause=host.querySelector('[data-cloud-platform-paused]');expect(pause).not.toBeNull();expect(pause?.closest('details:not([open])')).toBeNull();expect(pause?.textContent).toContain('Original run paused');
  const provider=host.querySelector('.cloud-task-provider-state');expect(provider?.textContent).toBe('completed');expect(provider?.closest('details:not([open])')).toBeNull();
  expect(host.querySelector<HTMLDetailsElement>('[data-cloud-execution-details]')?.open).toBe(false);
  expect(host.querySelectorAll('button')).toHaveLength(2);
 }
});
it('shows one next action with collapsed exact execution facts and unreported cost',async()=>{
 await render(model());expect(host.querySelector('[data-cloud-runner-phase]')?.textContent).toContain('Platform delivery evidence required');
 expect(host.querySelector<HTMLDetailsElement>('[data-cloud-execution-details]')?.open).toBe(false);
 expect(host.querySelectorAll('button.primary')).toHaveLength(1);expect(host.querySelector('[data-cloud-action="refresh"]')).not.toBeNull();
 expect(host.textContent).toContain('provider-original');expect(host.textContent).toContain('completed');expect(host.textContent).toContain('Not reported');expect(host.textContent).not.toMatch(/0\s*(tokens|USD|CNY|cost)/);
 expect(host.querySelector('[data-cloud-action="delegate"]')).toBeNull();
});
it('keeps a machine creation unknown separate from browser operation recovery',async()=>{
 const value=model();value.submission={id:'submission',version:1,state:'prepared'};value.execution=execution();value.actions={delegate:false,revoke:false,refresh:false};
 const onReload=vi.fn(async()=>{});await render(value,onReload);
 expect(host.querySelector('[data-cloud-runner-phase]')?.textContent).toContain('Creation outcome needs checking');expect(host.textContent).not.toContain('Creating cloud task');
 expect(host.querySelector('[data-cloud-action="reconcile"]')).toBeNull();expect(host.querySelector('[data-cloud-action="delegate"]')).toBeNull();
 await act(async()=>host.querySelector<HTMLButtonElement>('[data-cloud-action="reload"]')!.click());expect(onReload).toHaveBeenCalledTimes(1);
});
it('renders Chinese stop uncertainty with keyboard-operable details and the original attempt',async()=>{
 await setLocale('zh-CN',{persist:false});const value=model();value.execution=execution({runState:'cancelling',nodeState:'waiting',reason:'cloud.stop_unconfirmed'});await render(value);
 expect(host.querySelector('[data-cloud-runner-phase]')?.textContent).toContain('远端停止未确认');
 const summary=host.querySelector<HTMLElement>('[data-cloud-execution-details] > summary')!;summary.focus();expect(document.activeElement).toBe(summary);
 expect(host.textContent).toContain('formal-worker');expect(host.textContent).toContain('cloud.stop_unconfirmed');expect(host.querySelector('[data-cloud-runner-phase]')?.textContent).toBe('远端停止未确认');expect(host.textContent).not.toContain('等待审批');
});
it('labels waiting execution facts without implying approval unless an approval blocker exists',async()=>{
 const snapshot:NodeAdmission={run_id:'run',node_id:'cloud',run_version:7,node_state:'waiting',attempt:1,fence:2,policy_version:1,evaluated_at:'2026-10-30T00:00:00Z',status:'blocked',blockers:[{code:'node_waiting',source:'node',message:''}],requests:[],reservations:[]};
 await act(async()=>{root=createRoot(host);root.render(<><AdmissionFacts snapshot={snapshot}/><AttemptFacts attempt={{run_id:'run',node_id:'cloud',attempt:1,fence:2,worker_id:'formal-worker',state:'waiting',started_at:'2026-10-30T00:00:00Z',completed_at:'',result_source:'unreported'}}/></>);});
 expect(host.querySelector('.scheduling-facts > p')?.textContent).toContain('Node waiting');
 expect(host.textContent).not.toMatch(/approval/i);
 snapshot.blockers=[{code:'approval_required',source:'node',message:''}];
 await act(async()=>root!.render(<AdmissionFacts snapshot={snapshot}/>));
 expect(host.querySelector('.scheduling-facts > p')?.textContent).toContain('Node waiting');
 expect(host.querySelector('.scheduling-blockers')?.textContent).toContain('Awaiting human approval');
});
