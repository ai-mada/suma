// @vitest-environment jsdom
import {act,type ComponentProps} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {setLocale} from './i18n';
import {WorkBuddyCloudTaskPanel,type WorkBuddyCloudTaskModel,type WorkBuddyCloudActionResult,type WorkBuddyCloudDelegateDraft} from './WorkBuddyCloudTaskPanel';

let host:HTMLDivElement,root:Root|undefined;
const confirmed=():Promise<WorkBuddyCloudActionResult>=>Promise.resolve({kind:'confirmed'});
const base=():WorkBuddyCloudTaskModel=>({
 scopeKey:'user/org/project/run/node',sourceVersion:'run7-node-a-connection3',state:'ready',
 task:{title:'Build approved changes',runLabel:'Iteration 7',nodeLabel:'Implement feature',prompt:'Original <script>request</script> {{name}}',name:'Feature implementation',requestDigest:'a'.repeat(64),nodeDigest:'b'.repeat(64)},
 connections:[{id:'connection-a',label:'My WorkBuddy account',version:3,grantGeneration:1,eligible:true},{id:'connection-b',label:'Another account',version:4,grantGeneration:2,eligible:false,unavailableReason:'node_mismatch'}],
 actions:{delegate:true,revoke:false,refresh:false},
});
beforeEach(async()=>{await setLocale('en-US',{persist:false});host=document.createElement('div');document.body.append(host);vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);});
afterEach(async()=>{if(root)await act(async()=>root?.unmount());root=undefined;host.remove();await setLocale('zh-CN',{persist:false});vi.restoreAllMocks();vi.unstubAllGlobals();});
async function settle(){for(let n=0;n<3;n++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function render(model=base(),extra:Partial<ComponentProps<typeof WorkBuddyCloudTaskPanel>>={}){await act(async()=>{root??=createRoot(host);root.render(<WorkBuddyCloudTaskPanel model={model} onDelegate={confirmed} onRevoke={confirmed} onRefresh={confirmed} onReload={async()=>{}} {...extra}/>);});await settle();}
async function click(selector:string){const element=host.querySelector<HTMLElement>(selector);expect(element,selector).not.toBeNull();await act(async()=>element!.click());await settle();}
async function change(selector:string,value:string){const element=host.querySelector<HTMLSelectElement>(selector)!;expect(element,selector).not.toBeNull();await act(async()=>{Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype,'value')!.set!.call(element,value);element.dispatchEvent(new Event('change',{bubbles:true}));});}
async function approve(){await change('[data-cloud-field="connection"]','connection-a');await change('[data-cloud-field="duration"]','60');await click('[data-cloud-field="acknowledge"]');}

it('requires an eligible exact-node connection, explicit duration and acknowledgement before delegation',async()=>{
 const onDelegate=vi.fn((_draft:WorkBuddyCloudDelegateDraft)=>confirmed());await render(base(),{onDelegate});
 expect(onDelegate).not.toHaveBeenCalled();expect(host.querySelector<HTMLButtonElement>('[data-cloud-action="delegate"]')!.disabled).toBe(true);
 expect(host.querySelector<HTMLOptionElement>('option[value="connection-b"]')!.disabled).toBe(true);
 await approve();expect(host.querySelector<HTMLButtonElement>('[data-cloud-action="delegate"]')!.disabled).toBe(false);
 await click('[data-cloud-action="delegate"]');
 expect(onDelegate).toHaveBeenCalledTimes(1);expect(onDelegate.mock.calls[0][0]).toMatchObject({connectionID:'connection-a',connectionVersion:3,grantGeneration:1,sourceVersion:'run7-node-a-connection3'});
 const expiry=new Date(onDelegate.mock.calls[0][0].expiresAt).getTime();expect(expiry-Date.now()).toBeGreaterThan(59*60_000);expect(expiry-Date.now()).toBeLessThanOrEqual(60*60_000);
 expect(host.textContent).toContain('Delegation confirmed');expect(host.textContent).not.toContain('Task completed');
});

it('retains the draft after a known failure and clears approval when the original source changes',async()=>{
 const onDelegate=vi.fn(async()=>({kind:'failed' as const}));await render(base(),{onDelegate});await approve();await click('[data-cloud-action="delegate"]');
 expect(host.querySelector<HTMLSelectElement>('[data-cloud-field="connection"]')!.value).toBe('connection-a');
 expect(host.querySelector<HTMLSelectElement>('[data-cloud-field="duration"]')!.value).toBe('60');expect(host.querySelector('[role="alert"]')).not.toBeNull();
 await render({...base(),sourceVersion:'run8-node-a-connection3'},{onDelegate});
 expect(host.querySelector<HTMLInputElement>('[data-cloud-field="acknowledge"]')!.checked).toBe(false);expect(host.textContent).toContain('Review the updated request');
 expect(host.querySelector<HTMLSelectElement>('[data-cloud-field="connection"]')!.value).toBe('connection-a');
});

it('locks an unknown operation and only offers read or original-operation reconciliation',async()=>{
 const onDelegate=vi.fn(async()=>({kind:'unknown' as const})),onReload=vi.fn(async()=>{}),onReconcile=vi.fn(confirmed);
 await render(base(),{onDelegate,onReload,onReconcile});await approve();await click('[data-cloud-action="delegate"]');
 expect(host.querySelector<HTMLButtonElement>('[data-cloud-action="delegate"]')!.disabled).toBe(true);expect(host.textContent).toContain('Do not submit again');
 await click('[data-cloud-action="reload"]');expect(onReload).toHaveBeenCalledTimes(1);expect(onDelegate).toHaveBeenCalledTimes(1);
 await click('[data-cloud-action="reconcile"]');expect(onReconcile).toHaveBeenCalledTimes(1);expect(onDelegate).toHaveBeenCalledTimes(1);
});

it('treats a thrown command as unknown and keeps the original lock after failed reconciliation',async()=>{
 const onDelegate=vi.fn(async()=>{throw Error('private transport payload');}),onReconcile=vi.fn(async()=>({kind:'failed' as const}));
 await render(base(),{onDelegate,onReconcile});await approve();await click('[data-cloud-action="delegate"]');
 expect(host.textContent).toContain('Do not submit again');expect(host.textContent).not.toContain('private transport payload');
 await click('[data-cloud-action="reconcile"]');
 expect(host.textContent).toContain('Do not submit again');expect(host.querySelector<HTMLButtonElement>('[data-cloud-action="delegate"]')!.disabled).toBe(true);
 expect(onDelegate).toHaveBeenCalledTimes(1);expect(host.textContent).not.toContain('The operation was not applied');
 const model={...base(),sourceVersion:'new-read',recovery:{action:'delegate' as const,state:'unknown' as const,canReconcile:false}};
 await render(model,{onDelegate,onReconcile});expect(host.querySelector('[data-cloud-action="reconcile"]')).toBeNull();
 expect(host.querySelector<HTMLButtonElement>('[data-cloud-action="delegate"]')!.disabled).toBe(true);
});

it('prevents duplicate pending commands and ignores late confirmation after a scope switch',async()=>{
 let resolve!:(value:WorkBuddyCloudActionResult)=>void;const onDelegate=vi.fn(()=>new Promise<WorkBuddyCloudActionResult>(done=>{resolve=done;}));
 await render(base(),{onDelegate});await approve();
 const button=host.querySelector<HTMLButtonElement>('[data-cloud-action="delegate"]')!;await act(async()=>{button.click();button.click();});
 expect(onDelegate).toHaveBeenCalledTimes(1);expect(host.querySelector('[aria-busy="true"]')).not.toBeNull();
 const next={...base(),scopeKey:'other-user/org/project/run/node'};await render(next,{onDelegate});
 await act(async()=>resolve({kind:'confirmed'}));await settle();
 expect(host.textContent).not.toContain('Delegation confirmed');expect(host.querySelector<HTMLSelectElement>('[data-cloud-field="connection"]')!.value).toBe('');
});

it('shows persisted cloud facts without treating provider completion or missing usage as acceptance or zero cost',async()=>{
 const model=base();model.delegation={id:'delegate-a',version:2,state:'active',expiresAt:'2030-01-01T12:00:00Z',connectionLabel:'My WorkBuddy account',createsConsumed:1};
 model.submission={id:'submission-a',version:3,state:'task_known',taskID:'vendor-task-a',providerStatus:'completed',observedAt:'2026-10-30T12:00:00Z'};model.actions={delegate:false,revoke:true,refresh:true};
 const onRefresh=vi.fn(confirmed),onRevoke=vi.fn(confirmed);await render(model,{onRefresh,onRevoke});
 expect(host.textContent).toContain('completed');expect(host.textContent).toContain('Not reported');expect(host.textContent).not.toMatch(/0\s*(tokens|USD|CNY|cost)/);
 expect(host.querySelector('[data-cloud-action="delegate"]')).toBeNull();await click('[data-cloud-action="refresh"]');expect(onRefresh).toHaveBeenCalledTimes(1);
 await click('[data-cloud-action="revoke"]');expect(onRevoke).toHaveBeenCalledTimes(1);expect(host.textContent).toContain('does not stop an existing cloud task');
});

it('never offers a new create for prepared or unknown records and escapes original content',async()=>{
 const model=base();model.submission={id:'submission-a',version:1,state:'prepared'};model.actions={delegate:true,revoke:false,refresh:false};await render(model);
 expect(host.querySelector('[data-cloud-action="delegate"]')).toBeNull();expect(host.textContent).toContain('Creation outcome needs checking');
 expect(host.querySelector('script')).toBeNull();expect(host.textContent).toContain('Original <script>request</script> {{name}}');
 expect(host.querySelector('[data-cloud-action="refresh"]')).toBeNull();
});

it('clears protected content on access denial and does not let late results restore it',async()=>{
 let resolve!:(value:WorkBuddyCloudActionResult)=>void;const onDelegate=vi.fn(()=>new Promise<WorkBuddyCloudActionResult>(done=>{resolve=done;})),onReload=vi.fn(async()=>{});
 await render(base(),{onDelegate,onReload});await approve();await act(async()=>host.querySelector<HTMLButtonElement>('[data-cloud-action="delegate"]')!.click());
 const model={...base(),state:'denied' as const};await render(model,{onDelegate,onReload});
 expect(host.textContent).not.toContain('Original <script>');expect(host.querySelector('form')).toBeNull();expect(host.textContent).toContain('Access changed');
 await act(async()=>resolve({kind:'confirmed'}));await settle();expect(host.textContent).not.toContain('Original <script>');expect(host.textContent).not.toContain('Delegation confirmed');
 await click('[data-cloud-action="reload"]');expect(onReload).toHaveBeenCalledTimes(1);
});

it('renders loading, unavailable and Chinese states with labelled keyboard-operable controls',async()=>{
 await render({...base(),state:'loading'});expect(host.querySelector('[role="status"]')).not.toBeNull();expect(host.querySelector('form')).toBeNull();
 await render({...base(),state:'unavailable',unavailableReason:'source_missing'});expect(host.textContent).toContain('original run and node');expect(host.querySelector('form')).toBeNull();
 await act(async()=>setLocale('zh-CN',{persist:false}));await render();expect(host.textContent).toContain('WorkBuddy 云任务');
 expect(host.querySelector('label[for]')).not.toBeNull();expect(host.querySelector('select')?.getAttribute('id')).toBeTruthy();expect(host.querySelector('details > summary')).not.toBeNull();
});

it.each(['conflict','denied'] as const)('reopens only an explicitly checked current source after %s without losing selected fields unnecessarily',async kind=>{
 const onDelegate=vi.fn(async()=>({kind})),onReload=vi.fn(async()=>{});await render(base(),{onDelegate,onReload});await approve();await click('[data-cloud-action="delegate"]');
 await click('[data-cloud-action="reload"]');expect(onReload).toHaveBeenCalledTimes(1);expect(host.querySelector('form')).not.toBeNull();
 expect(host.querySelector<HTMLInputElement>('[data-cloud-field="acknowledge"]')!.checked).toBe(false);
 if(kind==='conflict'){
  expect(host.querySelector<HTMLSelectElement>('[data-cloud-field="connection"]')!.value).toBe('connection-a');
  await click('[data-cloud-field="acknowledge"]');expect(host.querySelector<HTMLButtonElement>('[data-cloud-action="delegate"]')!.disabled).toBe(false);
 }else expect(host.querySelector<HTMLSelectElement>('[data-cloud-field="connection"]')!.value).toBe('');
 expect(onDelegate).toHaveBeenCalledTimes(1);
});

it('reads saved platform facts in the normal ready view without a command and never releases an unknown operation',async()=>{
 const onReload=vi.fn(async()=>{}),onDelegate=vi.fn(async()=>({kind:'unknown' as const}));await render(base(),{onReload,onDelegate});await approve();
 expect(host.querySelectorAll('[data-cloud-action="reload"]')).toHaveLength(1);await click('[data-cloud-action="reload"]');
 expect(onReload).toHaveBeenCalledTimes(1);expect(onDelegate).not.toHaveBeenCalled();expect(host.querySelector<HTMLSelectElement>('[data-cloud-field="duration"]')!.value).toBe('60');
 await click('[data-cloud-action="delegate"]');expect(host.textContent).toContain('Do not submit again');
 expect(host.querySelectorAll('[data-cloud-action="reload"]')).toHaveLength(1);await click('[data-cloud-action="reload"]');
 expect(onReload).toHaveBeenCalledTimes(2);expect(onDelegate).toHaveBeenCalledTimes(1);expect(host.querySelector<HTMLButtonElement>('[data-cloud-action="delegate"]')!.disabled).toBe(true);
});
