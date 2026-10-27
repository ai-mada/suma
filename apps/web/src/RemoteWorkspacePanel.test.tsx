// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {createHash,webcrypto} from 'node:crypto';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {RemoteWorkspacePanel} from './RemoteWorkspacePanel';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import {remoteScopeKey,validRemoteObservation} from './remote-workspace-model';
import type {WorkspaceObservationCreateRequest,WorkspaceObservationScope,WorkspaceObservationTarget,WorkspaceWriteCreateRequest} from '../../../packages/contracts/src';
vi.mock('@monaco-editor/react',async()=>{const {createElement}=await import('react');return {default:({value,options,onChange}:{value:string;options:{readOnly?:boolean;ariaLabel:string};onChange?:(value:string)=>void})=>createElement('textarea',{value,readOnly:!!options.readOnly,'aria-label':options.ariaLabel,onChange:(event:{target:{value:string}})=>onChange?.(event.target.value)})};});
const project='project-one',scope={run_id:'run-one',node_id:'node-one',attempt:1,fence:2,worker_id:'worker-one',tool_profile_id:'native-one',profile_sha256:'a'.repeat(64)};
const codexScope={...scope,run_id:'codex-run',tool_profile_id:'codex-tool',profile_sha256:'d'.repeat(64)};
const sourceText='export const answer = 42;\n',sha=(text:string)=>createHash('sha256').update(text).digest('hex');
const session={user:{id:'u',name:'Owner',email:'owner@example.test'},organization:{id:'o',name:'Org'}};
let host:HTMLDivElement,root:Root,requests:{path:string;body?:WorkspaceObservationCreateRequest}[],pending:((value:Response)=>void)|undefined,hold=false;
let targets:WorkspaceObservationTarget[],writeReply:((body:WorkspaceWriteCreateRequest)=>Promise<Response>)|undefined;
function response(body:WorkspaceObservationCreateRequest){return {id:'observation-one',project_id:project,version:2,state:'completed',scope:body.scope,operation:body.operation,arguments:body.arguments,expected_sha256:body.expected_sha256,workspace_policy_sha256:'b'.repeat(64),created_at:new Date().toISOString(),expires_at:new Date(Date.now()+60000).toISOString(),observed_at:new Date().toISOString(),result:body.operation==='list'?{list:{entries:[{path:'src/main.ts',kind:'file'}],next_cursor:'',complete:true}}:{read:{path:'src/main.ts',content:sourceText,sha256:sha(sourceText),bytes:26,start_line:1,complete:true}}};}
beforeEach(async()=>{sessionStorage.clear();localStorage.clear();requests=[];pending=undefined;hold=false;writeReply=undefined;targets=[{...scope,run_name:'Run one',node_name:'Develop',can_write:true}];await setLocale('en-US',{persist:false});vi.stubGlobal('crypto',webcrypto);vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{const path=String(input).replace(/^\/api\/v1/,'');const body=init?.body?JSON.parse(String(init.body)):undefined;requests.push({path,body});if(path.startsWith('/workspace/targets'))return Response.json({project_id:project,items:targets,total:targets.length,limit:50,truncated:false,observed_at:new Date().toISOString()});if(path==='/workspace/writes'&&body&&writeReply)return writeReply(body);if(path.includes('metadata=true'))return Response.json({});if(body){if(hold)return new Promise(resolve=>{pending=resolve;});return Response.json(response(body));}throw Error('Unexpected '+path);}));});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();await setLocale('zh-CN',{persist:false});vi.unstubAllGlobals();vi.restoreAllMocks();});
async function settle(){for(let i=0;i<4;i++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function render(user=session){await act(async()=>root.render(<ApiViewProvider session={user}><RemoteWorkspacePanel project={project}/></ApiViewProvider>));await settle();}
async function choose(value:WorkspaceObservationScope=scope){const select=host.querySelector('select')!;await act(async()=>{select.value=remoteScopeKey(value);select.dispatchEvent(new Event('change',{bubbles:true}));});await settle();}
async function click(text:string){const button=[...host.querySelectorAll('button')].find(value=>value.textContent===text);expect(button,text).toBeDefined();await act(async()=>button!.click());await settle();}
async function edit(text:string){const field=host.querySelector<HTMLTextAreaElement>('textarea[aria-label="Execution file draft"]')!;await act(async()=>{Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value')!.set!.call(field,text);field.dispatchEvent(new Event('input',{bubbles:true}));});await settle();}
it('reads the selected exact attempt through control, renders readonly source and switches language without another read',async()=>{await render();await choose();await act(async()=>[...host.querySelectorAll('button')].find(b=>b.textContent==='main.ts')!.click());await settle();const editor=host.querySelector('textarea')!;expect(editor.readOnly).toBe(true);expect(editor.value).toContain('answer = 42');expect(requests.filter(r=>r.body)).toHaveLength(2);expect(requests[2].body).toMatchObject({project_id:project,scope,operation:'read',arguments:{path:'src/main.ts',start_line:1,max_lines:200}});expect(host.textContent).toContain('File basis');expect(host.textContent).toMatch(/Read at [^\n]*\d+:\d+:\d+/);expect(host.querySelector('button[type=submit]')).toBeNull();await act(async()=>setLocale('zh-CN',{persist:false}));expect(host.textContent).toContain('文件依据');expect(host.textContent).toContain('只读');expect(requests).toHaveLength(3);expect(localStorage.length).toBe(0);expect([...Array(sessionStorage.length)].map((_,i)=>sessionStorage.getItem(sessionStorage.key(i)!)).join('')).not.toContain('answer = 42');});
it('hides source synchronously on identity change and ignores an old in-flight result',async()=>{await render();await choose();hold=true;await act(async()=>[...host.querySelectorAll('button')].find(b=>b.textContent==='main.ts')!.click());await settle();const body=requests.at(-1)!.body!;expect(pending).toBeDefined();await render({...session,user:{...session.user,id:'other'}});await act(async()=>pending!(Response.json(response(body))));await settle();expect(host.querySelector('textarea')).toBeNull();expect(host.textContent).not.toContain('answer = 42');});
it('rejects content for a different fence or a changed next-section digest',()=>{const body:WorkspaceObservationCreateRequest={project_id:project,scope,operation:'read',arguments:{path:'src/main.ts',start_line:1,max_lines:200},idempotency_key:'read-one'};const value=response(body);expect(validRemoteObservation(value,project,scope,'read',body.arguments)).toBe(true);expect(validRemoteObservation({...value,scope:{...scope,fence:3}},project,scope,'read',body.arguments)).toBe(false);expect(validRemoteObservation(value,project,scope,'read',body.arguments,'d'.repeat(64))).toBe(false);});
it('accepts a current Codex read target, displays only returned tool and route facts, and never offers editing',async()=>{
 targets=[{...codexScope,run_name:'Codex run',node_name:'Develop',can_write:false,pool_id:'pool-one'}];
 await render();await choose(codexScope);await click('main.ts');
 expect(host.querySelector('textarea')?.readOnly).toBe(true);expect(host.querySelector('.remote-write-editor')).toBeNull();
 expect(host.textContent).toContain('Tool profile: codex-tool');expect(host.textContent).toContain('Execution pool pool-one');
 expect([...host.querySelectorAll('button')].some(button=>button.textContent==='Edit file')).toBe(false);
 expect(requests.filter(request=>request.body).map(request=>request.body?.scope)).toEqual([codexScope,codexScope]);
 await act(async()=>setLocale('zh-CN',{persist:false}));expect(host.textContent).toContain('Codex 工作区只读');expect(host.textContent).toContain('工具档案');expect(host.textContent).toContain('执行池 pool-one');
 expect(requests.some(request=>request.path.startsWith('/workspace/writes'))).toBe(false);
});
it('ignores an earlier native read after selecting a Codex read-only target',async()=>{
 targets.push({...codexScope,run_name:'Codex run',node_name:'Develop',can_write:false});
 await render();await choose();hold=true;await click('main.ts');const original=requests.at(-1)!.body!,release=pending!;hold=false;
 await choose(codexScope);await act(async()=>release(Response.json(response(original))));await settle();await click('main.ts');
 expect(host.querySelector('textarea')?.readOnly).toBe(true);expect(host.querySelector('.remote-write-editor')).toBeNull();
 expect([...host.querySelectorAll('button')].some(button=>button.textContent==='Edit file')).toBe(false);
 expect(requests.at(-1)?.body?.scope).toEqual(codexScope);
});
it('keeps a native draft and its pending save on the original attempt until the editor is explicitly closed',async()=>{
 targets.push({...codexScope,run_name:'Codex run',node_name:'Develop',can_write:false});
 let original:WorkspaceWriteCreateRequest|undefined,release!:(value:Response)=>void;
 writeReply=async body=>{original=body;return new Promise(resolve=>{release=resolve;});};
 await render();await choose();await click('main.ts');await click('Edit file');await edit('submitted native draft\n');await click('Save');await edit('newer native draft\n');
 expect(host.querySelector<HTMLSelectElement>('select')?.disabled).toBe(true);await choose(codexScope);
 expect(host.querySelector('textarea')?.value).toBe('newer native draft\n');expect(original?.scope).toEqual(scope);
 expect(requests.some(request=>request.body?.scope.run_id===codexScope.run_id)).toBe(false);
 const body=original!;await act(async()=>release(Response.json({id:'write-one',project_id:project,version:3,state:'applied',scope:body.scope,workspace_policy_sha256:body.workspace_policy_sha256,path:body.path,expected_sha256:body.expected_sha256,content_sha256:body.content_sha256,request_sha256:'e'.repeat(64),execution_fence:1,created_at:new Date().toISOString(),expires_at:new Date(Date.now()+120000).toISOString(),result:{native_operation_id:'remote-write:write-one',path:body.path,previous_sha256:body.expected_sha256,sha256:body.content_sha256,generation:1}})));await settle();
 expect(host.querySelector('textarea')?.value).toBe('newer native draft\n');expect(host.textContent).toContain('Original save confirmed');
 await click('Discard draft and close');await choose(codexScope);await click('main.ts');
 expect(host.querySelector('textarea')?.readOnly).toBe(true);expect(host.querySelector('.remote-write-editor')).toBeNull();expect(requests.filter(request=>request.path==='/workspace/writes')).toHaveLength(1);
});
