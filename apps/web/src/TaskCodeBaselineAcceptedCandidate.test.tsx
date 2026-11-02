// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import {TaskCodeBaselinePanel} from './TaskCodeBaselinePanel';
import type {Session} from './api';

const project='project-a',taskID='downstream-a',sha='a'.repeat(64),head='c'.repeat(40);
const session:Session={user:{id:'owner-a',name:'Owner',email:'owner@example.test'},organization:{id:'org-a',name:'Org'},projects:[]};
const source=(node:string,commit=head)=>({schema_version:1,transfer_id:'transfer-'+node.replaceAll('.','-'),project_id:project,state:'sealed',source:{run_id:'accepted-run',node_id:node,attempt:1,fence:2,worker_id:'developer-a'},base_commit:'b'.repeat(40),head_commit:commit,evidence_sha256:sha,bundle_sha256:sha,size_bytes:1234});
const choice=(node:string,commit=head)=>({task_id:'accepted-a',task_version:5,title:'Accepted agent task',source_node_id:node,source:source(node,commit),eligible:false,can_bind_baseline:true,blockers:[] as string[]});
const key=(node:string)=>JSON.stringify(['accepted-a',node]);
type Baseline={project_id:string;task_id:string;version:number;binding?:Record<string,unknown>;can_bind:boolean;blockers:string[]};
let root:Root|undefined,host:HTMLDivElement,target:{id:string;project_id:string;title:string;version:number;status:string},baseline:Baseline,available:ReturnType<typeof choice>[],reads:string[],writes:{path:string;body:Record<string,unknown>;headers:Headers}[],reply:(path:string,init?:RequestInit)=>Promise<Response>;
function defaults(path:string){
 if(path.includes('/code-baseline'))return Response.json(baseline);
 if(path.startsWith('/tasks/'))return Response.json(target);
 if(path.startsWith('/candidate-compositions/options'))return Response.json({project_id:project,task_id:taskID,task_version:target.version,can_create:false,blockers:['Existing baseline prevents creating a composition'],sources:available});
 throw Error('Unexpected read '+path);
}
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});sessionStorage.clear();vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
 target={id:taskID,project_id:project,title:'Unstarted downstream',version:2,status:'todo'};baseline={project_id:project,task_id:taskID,version:1,binding:{protocol:'suma.code-baseline.v1',id:'prior-binding',project_id:project,task_id:taskID,task_version:2,composition_id:'prior-composition',composition_version:2,accepted_basis_sha256:sha,binding_sha256:sha,source:source('prior-agent')},can_bind:true,blockers:[]};available=[choice('agent.one'),choice('agent.two','d'.repeat(40))];reads=[];writes=[];host=document.createElement('div');document.body.append(host);
 reply=async path=>defaults(path);
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{const path=String(input).replace(/^\/api\/v1/,'');if(init?.body)writes.push({path,body:JSON.parse(String(init.body)),headers:new Headers(init.headers)});else reads.push(path);return reply(path,init);}));
});
afterEach(async()=>{if(root)await act(async()=>root?.unmount());root=undefined;host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});
async function settle(){for(let i=0;i<4;i++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function draw(active=session){await act(async()=>{root??=createRoot(host);root.render(<ApiViewProvider session={active}><TaskCodeBaselinePanel session={active} project={project} taskID={taskID} initiallyOpen/></ApiViewProvider>);});await settle();}
async function change(selector:string,value:string){const field=host.querySelector<HTMLSelectElement|HTMLTextAreaElement>(selector)!;expect(field,selector).not.toBeNull();await act(async()=>{Object.getOwnPropertyDescriptor(field instanceof HTMLSelectElement?HTMLSelectElement.prototype:HTMLTextAreaElement.prototype,'value')!.set!.call(field,value);field.dispatchEvent(new Event(field instanceof HTMLSelectElement?'change':'input',{bubbles:true}));});await settle();}
async function click(selector:string){const button=host.querySelector<HTMLElement>(selector)!;expect(button,selector).not.toBeNull();expect((button as HTMLButtonElement).disabled).not.toBe(true);await act(async()=>button.click());await settle();}
async function select(node='agent.one'){await change('[data-task-baseline-source]',key(node));await change('[name=composition_bind_reason]','Reviewed this exact source candidate');}
function accept(body:Record<string,unknown>){target={...target,version:3};baseline={...baseline,version:2,binding:{protocol:'suma.code-baseline.v1',id:'binding-a',project_id:project,task_id:taskID,task_version:3,source_task_id:body.source_task_id,source_task_version:body.expected_source_task_version,accepted_basis_sha256:sha,binding_sha256:sha,source:source(String(body.source_node_id),body.source_node_id==='agent.two'?'d'.repeat(40):head)}};}

it('T222 explicitly binds one of multiple accepted agent producers even when composition creation and eligibility are false',async()=>{
 reply=async(path,init)=>{if(init?.body){accept(JSON.parse(String(init.body)));return Response.json(target);}return defaults(path);};
 await draw();expect(host.querySelector<HTMLSelectElement>('[data-task-baseline-source]')?.value).toBe('');expect(host.querySelector('[data-task-baseline-candidate]')).toBeNull();expect(host.querySelector<HTMLButtonElement>('[data-composition-bind-submit]')?.disabled).toBe(true);expect(writes).toHaveLength(0);
 await select('agent.two');expect(host.querySelector('[data-task-baseline-candidate]')?.textContent).toContain('agent.two');expect(host.querySelector('[data-task-baseline-candidate]')?.textContent).toContain('v5');expect(host.querySelector('[data-task-baseline-candidate]')?.textContent).toContain('d'.repeat(40));
 expect(host.querySelector('[data-task-baseline-candidate] a:last-child')?.getAttribute('href')).toContain('node=agent.two&attempt=1');
 await click('[name=composition_bind_ack]');await click('[data-composition-bind-submit]');expect(writes).toHaveLength(1);expect(writes[0].path).toBe('/tasks/'+taskID+'/code-baseline');
 expect(writes[0].body).toEqual({project_id:project,expected_task_version:2,expected_binding_version:1,source_task_id:'accepted-a',expected_source_task_version:5,source_node_id:'agent.two',acknowledge:true,reason:'Reviewed this exact source candidate',idempotency_key:expect.any(String)});
 expect(host.textContent).toContain('The task baseline was updated');expect(reads.every(path=>!path.startsWith('/candidate-compositions?'))).toBe(true);
});

it('T222 revokes old source confirmation on qualification or candidate changes and discards late account-ABA reads',async()=>{
 await draw();await select();await click('[name=composition_bind_ack]');
 available[0]={...available[0],eligible:true,can_bind_baseline:false};await click('[data-task-baseline-source-refresh]');expect(host.querySelector<HTMLInputElement>('[name=composition_bind_ack]')?.checked).toBe(false);expect(host.querySelector<HTMLButtonElement>('[data-composition-bind-submit]')?.disabled).toBe(true);
 available[0]={...available[0],task_version:6,can_bind_baseline:true,source:source('agent.one','e'.repeat(40))};await click('[data-task-baseline-source-refresh]');expect(host.querySelector<HTMLSelectElement>('[data-task-baseline-source]')?.value).toBe('');expect(host.querySelector('[data-task-baseline-source-changed]')).not.toBeNull();await select();expect(host.querySelector('[data-task-baseline-candidate]')?.textContent).toContain('e'.repeat(40));expect(host.querySelector<HTMLInputElement>('[name=composition_bind_ack]')?.checked).toBe(false);
 let release!:(response:Response)=>void,signal:AbortSignal|undefined;const original=reply;reply=async(path,init)=>{if(path.startsWith('/candidate-compositions/options')&&!signal){signal=init?.signal as AbortSignal;return new Promise<Response>(resolve=>{release=resolve;});}return original(path,init);};
 await click('[data-task-baseline-source-refresh]');await draw({...session,user:{...session.user,id:'owner-b'}});await draw();expect(signal?.aborted).toBe(true);
 await act(async()=>release(Response.json({project_id:project,task_id:taskID,task_version:2,can_create:false,blockers:[],sources:[{...choice('agent.one'),title:'OBSOLETE source'}]})));await settle();expect(host.textContent).not.toContain('OBSOLETE source');expect(host.querySelector('[data-task-baseline-candidate]')).toBeNull();expect(host.querySelector<HTMLSelectElement>('[data-task-baseline-source]')?.value).toBe('');expect(writes).toHaveLength(0);
});

it('T222 reconciles the original uncertain baseline body and key after the source disappears and new binding becomes unavailable',async()=>{
 let lose=true;reply=async(path,init)=>{if(!init?.body)return defaults(path);const body=JSON.parse(String(init.body));if(lose){lose=false;accept(body);baseline={...baseline,can_bind:false};throw TypeError('Committed response was lost');}return Response.json(target);};
 await draw();await select();await click('[name=composition_bind_ack]');await click('[data-composition-bind-submit]');expect(writes).toHaveLength(1);const original=structuredClone(writes[0].body);expect(sessionStorage.length).toBe(1);
 await act(async()=>root!.unmount());root=undefined;available=[];reads=[];await draw();expect(host.querySelector('[data-task-baseline-pending-source]')?.textContent).toContain('accepted-a');expect(host.querySelector('[data-task-baseline-pending-source]')?.textContent).toContain('agent.one');expect(reads.some(path=>path.startsWith('/candidate-compositions/options'))).toBe(false);expect(host.querySelector('[data-composition-bind-submit]')).toBeNull();
 await click('[data-composition-restore]');expect(writes).toHaveLength(2);expect(writes[1].body).toEqual(original);expect(writes[1].path).toBe(writes[0].path);expect(writes[1].headers.get('X-Suma-Expected-User')).toBe(session.user.id);expect(writes[1].headers.get('X-Suma-Expected-Organization')).toBe(session.organization.id);expect(host.textContent).toContain('The task baseline was updated');expect(sessionStorage.length).toBe(0);
});
