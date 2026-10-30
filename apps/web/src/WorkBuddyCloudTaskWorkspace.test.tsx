// @vitest-environment jsdom
import {webcrypto} from 'node:crypto';
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {CloudDelegation,CloudNodeView,CloudSubmission,WorkBuddyConnection} from '../../../packages/contracts/src';
import {ApiViewProvider} from './api-context';
import type {Session} from './api';
import {setLocale} from './i18n';
import {WorkBuddyCloudTaskWorkspace} from './WorkBuddyCloudTaskWorkspace';
import {cloudRequestDigest} from './workbuddy-cloud-task-model';

const session:Session={user:{id:'user-a',name:'A',email:'a@example.test'},organization:{id:'org-a',name:'Organization'},projects:[]},stamp='2026-10-30T00:00:00Z';
let host:HTMLDivElement,root:Root|undefined,node:CloudNodeView,account:WorkBuddyConnection,calls:{path:string;body?:Record<string,unknown>;headers:Headers}[];
async function fixture(project='project-a',run='run-a',id='node-a'):Promise<CloudNodeView>{const binding={connection_id:'connection-a',grant_generation:1,request:{prompt:'Approved cloud task <original>',name:'Approved feature'}};return {project_id:project,run_id:run,node_id:id,run_version:3,node_spec_sha256:'a'.repeat(64),request_sha256:await cloudRequestDigest(binding.request),binding,allowed_actions:['delegate']};}
function delegation(body?:Record<string,unknown>):CloudDelegation{return {id:'delegation-a',project_id:node.project_id,owner_id:session.user.id,connection_id:'connection-a',version:1,run_id:node.run_id,node_id:node.node_id,node_spec_sha256:node.node_spec_sha256,connection_version_at_issue:3,grant_generation:1,client_binding_sha256:'b'.repeat(64),request_sha256:node.request_sha256,expires_at:String(body?.expires_at||'2030-01-01T00:00:00Z'),max_creates:1,creates_consumed:0,state:'active',created_at:stamp,updated_at:stamp};}
function submission():CloudSubmission{return {id:'submission-a',project_id:node.project_id,delegation_id:'delegation-a',version:2,run_id:node.run_id,node_id:node.node_id,assignment:{run_id:node.run_id,node_id:node.node_id,attempt:1,fence:1,worker_id:'worker-a'},request_sha256:node.request_sha256,state:'task_known',task:{task_id:'vendor-task-a',status:'working'},observed_at:stamp,last_error:'',usage_state:'unreported',created_at:stamp,updated_at:stamp};}
beforeEach(async()=>{
 vi.stubGlobal('crypto',webcrypto);vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);await setLocale('en-US',{persist:false});sessionStorage.clear();calls=[];
 node=await fixture();account={id:'connection-a',project_id:'project-a',owner_id:'user-a',version:3,client_id:'client-a',open_id:'My provider account',scopes:['user.task.invokable','user.task.readable'],state:'connected',grant_generation:1,token_generation:2,expires_at:'2030-01-01T00:00:00Z',created_at:stamp,updated_at:stamp,last_error:'',allowed_actions:['refresh','disconnect']};
 host=document.createElement('div');document.body.append(host);
});
afterEach(async()=>{if(root)await act(async()=>root?.unmount());root=undefined;host.remove();await setLocale('zh-CN',{persist:false});vi.unstubAllGlobals();vi.restoreAllMocks();});
async function settle(){for(let n=0;n<5;n++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function render(project='project-a',run='run-a',id='node-a',user=session,onChanged?:()=>void|Promise<void>,revision?:number){await act(async()=>{root??=createRoot(host);root.render(<ApiViewProvider session={user}><WorkBuddyCloudTaskWorkspace project={project} runID={run} nodeID={id} revision={revision} onChanged={onChanged}/></ApiViewProvider>);});await settle();}
function transport(handler?:(path:string,body?:Record<string,unknown>)=>Promise<Response>){vi.stubGlobal('fetch',vi.fn(async(input:string,options?:RequestInit)=>{const path=String(input).replace(/^\/api\/v1/,'');const body=options?.body?JSON.parse(String(options.body)) as Record<string,unknown>:undefined;calls.push({path,body,headers:new Headers(options?.headers)});if(handler)return handler(path,body);return Response.json(path.startsWith('/workbuddy-cloud-nodes?')?node:account);}));}
async function click(selector:string){const element=host.querySelector<HTMLElement>(selector);expect(element,selector).not.toBeNull();await act(async()=>element!.click());await settle();}
async function choose(selector:string,value:string){const element=host.querySelector<HTMLSelectElement>(selector)!;expect(element,selector).not.toBeNull();await act(async()=>{Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype,'value')!.set!.call(element,value);element.dispatchEvent(new Event('change',{bubbles:true}));});}
async function approve(){await choose('[data-cloud-field="connection"]','connection-a');await choose('[data-cloud-field="duration"]','60');await click('[data-cloud-field="acknowledge"]');}

it('reads exact source then private connection and delegates only the frozen digest with original caller headers',async()=>{
 transport(async(path,body)=>{
  if(path==='/workbuddy-cloud-delegations'){node={...node,delegation:delegation(body),allowed_actions:['revoke']};return Response.json(node.delegation,{status:201});}
  return Response.json(path.startsWith('/workbuddy-cloud-nodes?')?node:account);
 });
 await render();expect(calls.slice(0,2).map(call=>call.path)).toEqual(['/workbuddy-cloud-nodes?project_id=project-a&run_id=run-a&node_id=node-a','/workbuddy-connections/connection-a?project_id=project-a']);
 await approve();await click('[data-cloud-action="delegate"]');
 const write=calls.find(call=>call.body)!;expect(write.path).toBe('/workbuddy-cloud-delegations');expect(write.body).toMatchObject({project_id:'project-a',run_id:'run-a',node_id:'node-a',expected_run_version:3,connection_id:'connection-a',expected_connection_version:3,grant_generation:1,max_creates:1,node_spec_sha256:node.node_spec_sha256,request_sha256:node.request_sha256});
 expect(write.headers.get('X-Suma-Expected-User')).toBe('user-a');expect(write.headers.get('X-Suma-Expected-Organization')).toBe('org-a');
 expect(JSON.stringify(write.body)).not.toContain(node.binding.request.prompt);expect(calls.filter(call=>call.body)).toHaveLength(1);expect(calls.slice(-2).map(call=>call.path)).toEqual(calls.slice(0,2).map(call=>call.path));
 expect(host.textContent).toContain('Delegation recorded');expect(sessionStorage.length).toBe(0);
});

it('restores a lost delegation response after remount with the same body/key and no second logical delegation',async()=>{
 let first=true,delegations=0;transport(async(path,body)=>{
  if(path==='/workbuddy-cloud-delegations'){
   if(first){first=false;delegations++;node={...node,delegation:delegation(body),allowed_actions:['revoke']};throw Error('response lost');}
   return Response.json(node.delegation);
  }
  return Response.json(path.startsWith('/workbuddy-cloud-nodes?')?node:account);
 });
 await render();await approve();await click('[data-cloud-action="delegate"]');
 const original=structuredClone(calls.find(call=>call.body)!.body);expect(host.textContent).toContain('Do not submit again');
 expect([...Array(sessionStorage.length)].map((_,i)=>sessionStorage.getItem(sessionStorage.key(i)!)).join('')).not.toContain(node.binding.request.prompt);
 await act(async()=>root!.unmount());root=undefined;await render();expect(calls.filter(call=>call.body)).toHaveLength(1);
 await click('[data-cloud-action="reconcile"]');expect(calls.filter(call=>call.body).map(call=>call.body)).toEqual([original,original]);expect(delegations).toBe(1);expect(sessionStorage.length).toBe(0);
 expect(calls.every(call=>call.path.startsWith('/workbuddy-cloud-')||call.path.startsWith('/workbuddy-connections/'))).toBe(true);
});

it('does not expose another members connection or discard project-readable cloud progress on its private404',async()=>{
 node={...node,submission:submission(),allowed_actions:[]};transport(async path=>path.startsWith('/workbuddy-connections/')?Response.json({error:{code:'not_found',message:'Not found'}},{status:404}):Response.json(node));
 await render();expect(host.textContent).toContain('vendor-task-a');expect(host.textContent).toContain('working');expect(host.textContent).not.toContain(account.open_id);expect(host.textContent).not.toContain('Access changed');expect(host.querySelector('[data-cloud-action="refresh"]')).toBeNull();
});

it('uses only the human observe endpoint and removes protected source after a current permission refusal',async()=>{
 node={...node,delegation:delegation(),submission:submission(),allowed_actions:['revoke','observe']};
 transport(async(path,body)=>body?Response.json({error:{code:'forbidden',message:'Access changed'}},{status:403}):Response.json(path.startsWith('/workbuddy-cloud-nodes?')?node:account));
 await render();await click('[data-cloud-action="refresh"]');expect(calls.find(call=>call.body)?.path).toBe('/workbuddy-cloud-submissions/submission-a/observe');
 expect(host.textContent).toContain('Access changed');expect(host.textContent).not.toContain('Approved cloud task');expect(host.textContent).not.toContain('vendor-task-a');expect(host.querySelector('[data-cloud-action="delegate"]')).toBeNull();
});

it('retires late reads on a project/run change before fetching a stale private connection',async()=>{
 let resolve!:(response:Response)=>void;const next=await fixture('project-b','run-b','node-b');
 transport(async path=>{
  if(path==='/workbuddy-cloud-nodes?project_id=project-a&run_id=run-a&node_id=node-a')return new Promise<Response>(done=>{resolve=done;});
  return Response.json(path.startsWith('/workbuddy-cloud-nodes?')?next:{...account,project_id:'project-b'});
 });
 await render();await render('project-b','run-b','node-b');await act(async()=>resolve(Response.json(node)));await settle();
 expect(host.textContent).toContain('run-b');expect(host.textContent).not.toContain('run-a');expect(calls.filter(call=>call.path.startsWith('/workbuddy-connections/')).map(call=>call.path)).toEqual(['/workbuddy-connections/connection-a?project_id=project-b']);
});

it('blocks an untracked write when local recovery storage cannot persist it',async()=>{
 transport();await render();const set=vi.spyOn(Storage.prototype,'setItem').mockImplementation(()=>{throw Error('quota');});await approve();await click('[data-cloud-action="delegate"]');
 expect(calls.filter(call=>call.body)).toHaveLength(0);expect(host.textContent).toContain('No new write was sent');set.mockRestore();
});

it('keeps malformed success uncertain and does not lose confirmation when only the follow-up read fails',async()=>{
 let mode:'malformed'|'good'|'read-failed'='malformed';transport(async(path,body)=>{
  if(body){if(mode==='malformed')return Response.json({token:'wrong response'});node={...node,delegation:delegation(body),allowed_actions:['revoke']};mode='read-failed';return Response.json(node.delegation);}
  if(mode==='read-failed'&&path.startsWith('/workbuddy-cloud-nodes?'))return Response.json({error:{code:'unavailable',message:'temporarily unavailable'}},{status:503});
  return Response.json(path.startsWith('/workbuddy-cloud-nodes?')?node:account);
 });
 await render();await approve();await click('[data-cloud-action="delegate"]');const original=structuredClone(calls.find(call=>call.body)!.body);expect(host.textContent).toContain('Do not submit again');
 mode='good';await click('[data-cloud-action="reconcile"]');expect(calls.filter(call=>call.body).map(call=>call.body)).toEqual([original,original]);expect(sessionStorage.length).toBe(0);expect(host.textContent).toContain('Delegation confirmed');expect(host.textContent).toContain('current record could not be read');
});

it('rereads saved source for a changed shared run revision while preserving the draft and original unknown operation',async()=>{
 transport(async(path,body)=>{
  if(body){node={...node,delegation:delegation(body),allowed_actions:['revoke']};throw Error('lost original response');}
  return Response.json(path.startsWith('/workbuddy-cloud-nodes?')?node:account);
 });
 await render('project-a','run-a','node-a',session,undefined,1);await approve();const initialReads=calls.filter(call=>call.path.startsWith('/workbuddy-cloud-nodes?')).length;
 node={...node,run_version:4};await render('project-a','run-a','node-a',session,undefined,2);
 expect(calls.filter(call=>call.path.startsWith('/workbuddy-cloud-nodes?'))).toHaveLength(initialReads+1);
 expect(host.querySelector<HTMLSelectElement>('[data-cloud-field="connection"]')!.value).toBe('connection-a');expect(host.querySelector<HTMLSelectElement>('[data-cloud-field="duration"]')!.value).toBe('60');expect(host.querySelector<HTMLInputElement>('[data-cloud-field="acknowledge"]')!.checked).toBe(false);
 await render('project-a','run-a','node-a',session,undefined,2);expect(calls.filter(call=>call.path.startsWith('/workbuddy-cloud-nodes?'))).toHaveLength(initialReads+1);
 await click('[data-cloud-field="acknowledge"]');await click('[data-cloud-action="delegate"]');const recovery=sessionStorage.getItem(sessionStorage.key(0)!);
 node={...node,submission:submission(),allowed_actions:['revoke','observe']};await render('project-a','run-a','node-a',session,undefined,3);
 expect(host.textContent).toContain('vendor-task-a');expect(host.textContent).toContain('Do not submit again');expect(sessionStorage.getItem(sessionStorage.key(0)!)).toBe(recovery);
 await click('[data-cloud-action="reload"]');expect(calls.filter(call=>call.body)).toHaveLength(1);expect(host.textContent).toContain('Do not submit again');
 expect(calls.filter(call=>call.path.includes('/observe')||call.path.includes('/refresh'))).toHaveLength(0);
});
