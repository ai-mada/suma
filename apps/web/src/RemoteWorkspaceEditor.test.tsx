// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {createHash,webcrypto} from 'node:crypto';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {RemoteWorkspaceEditor} from './RemoteWorkspaceEditor';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import {validRemoteWrite} from './remote-workspace-write-model';
import type {WorkspaceObservation,WorkspaceWriteCommand,WorkspaceWriteCreateRequest} from '../../../packages/contracts/src';

vi.mock('@monaco-editor/react',async()=>{
 const {createElement,useEffect,useRef}=await import('react');
 const runtime={editor:{EditorOption:{ariaLabel:4}},KeyMod:{CtrlCmd:2048},KeyCode:{KeyS:49}};
 function MockEditor({value,onChange,onMount,options}:{value:string;onChange?:(v:string)=>void;onMount?:(e:unknown,m:unknown)=>void;options:{ariaLabel?:string;readOnly?:boolean}}){
  const first=useRef(onMount);
  useEffect(()=>{first.current?.({addAction:()=>({dispose(){}})},runtime);},[]);
  return createElement('textarea',{value,readOnly:!!options.readOnly,'aria-label':options.ariaLabel,onChange:(event:{target:{value:string}})=>onChange?.(event.target.value)});
 }
 function MockDiff({original,modified,onMount}:{original:string;modified:string;onMount:(e:unknown,m:unknown)=>void}){
  const text=useRef(modified),changed=useRef<(()=>void)|undefined>(undefined);text.current=modified;
  useEffect(()=>{const editor={getValue:()=>text.current,getOption:()=>'',updateOptions:()=>{},onDidChangeConfiguration:()=>({dispose(){}}),addAction:()=>({dispose(){}}),onDidChangeModelContent:(fn:()=>void)=>{changed.current=fn;return {dispose(){changed.current=undefined;}};}};let pair:unknown={original:{isDisposed:()=>false,dispose(){}},modified:{isDisposed:()=>false,dispose(){}}};onMount({getModel:()=>pair,setModel:(value:unknown)=>{pair=value;},getOriginalEditor:()=>editor,getModifiedEditor:()=>editor},runtime);},[]);
  return createElement('section',null,createElement('textarea',{value:original,readOnly:true,'aria-label':'Current comparison file'}),createElement('textarea',{value:modified,'aria-label':'Merged draft',onChange:(event:{target:{value:string}})=>{text.current=event.target.value;changed.current?.();}}));
 }
 return {default:MockEditor,DiffEditor:MockDiff};
});

const sha=(text:string)=>createHash('sha256').update(text).digest('hex');
const project='project-one',scope={run_id:'run-one',node_id:'code',attempt:1,fence:2,worker_id:'worker-one',tool_profile_id:'native',profile_sha256:'a'.repeat(64)};
const initial='initial file\n';
const source:WorkspaceObservation={id:'observation-one',project_id:project,version:2,state:'completed',scope,operation:'read',arguments:{path:'source.txt',full_file:true},workspace_policy_sha256:'b'.repeat(64),created_at:'2026-10-27T04:00:00Z',observed_at:'2026-10-27T04:00:00Z',expires_at:'2026-10-27T04:01:00Z',result:{read:{path:'source.txt',content:initial,sha256:sha(initial),bytes:Buffer.byteLength(initial),start_line:1,complete:true}}};
const session={user:{id:'user-one',name:'Owner',email:'owner@example.test'},organization:{id:'org-one',name:'Org'}};
let root:Root,host:HTMLDivElement,writes:WorkspaceWriteCreateRequest[],fixture:(path:string,body?:WorkspaceWriteCreateRequest)=>Promise<Response>;
function command(body:WorkspaceWriteCreateRequest,id='command-one'):WorkspaceWriteCommand{return {id,project_id:project,version:3,state:'applied',scope,workspace_policy_sha256:body.workspace_policy_sha256,path:body.path,expected_sha256:body.expected_sha256,content_sha256:body.content_sha256,request_sha256:'d'.repeat(64),execution_fence:1,created_at:'2026-10-27T04:00:00Z',expires_at:'2026-10-27T04:02:00Z',result:{native_operation_id:'remote-write:'+id,path:body.path,previous_sha256:body.expected_sha256,sha256:body.content_sha256,generation:1}};}
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});localStorage.clear();sessionStorage.clear();writes=[];vi.stubGlobal('crypto',webcrypto);vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
 fixture=async(path,body)=>body?Response.json(command(body)):Response.json({...source,state:'expired',result:undefined});
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{const path=String(input).replace(/^\/api\/v1/,'');const body=init?.body?JSON.parse(String(init.body)):undefined;if(path==='/workspace/writes'&&body)writes.push(body);return fixture(path,body);}));
 host=document.createElement('div');document.body.append(host);root=createRoot(host);
 await act(async()=>root.render(<ApiViewProvider session={session}><RemoteWorkspaceEditor project={project} source={source} active onClose={()=>{}} onAccessLost={()=>{}} onRecorded={()=>{}}/></ApiViewProvider>));await settle();
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();await setLocale('zh-CN',{persist:false});vi.unstubAllGlobals();vi.restoreAllMocks();});
async function settle(){for(let i=0;i<4;i++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function edit(text:string,label='Execution file draft'){
 const field=host.querySelector<HTMLTextAreaElement>('textarea[aria-label="'+label+'"]')!;
 await act(async()=>{Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value')!.set!.call(field,text);field.dispatchEvent(new Event('input',{bubbles:true}));});await settle();
}
async function click(text:string){const button=[...host.querySelectorAll('button')].find(item=>item.textContent===text);expect(button,text).toBeDefined();await act(async()=>button!.click());await settle();}

it('preserves a newer draft when the original save ACK arrives, then uses the confirmed file digest for the next explicit save',async()=>{
 let release!:(v:Response)=>void;
 fixture=async(path,body)=>body?new Promise(resolve=>{release=resolve;}):Response.json({...source,state:'expired',result:undefined});
 await edit('submitted\n');await click('Save');expect(writes).toHaveLength(1);
 await edit('newer unsaved work\n');await act(async()=>release(Response.json(command(writes[0]))));await settle();
 expect(host.querySelector('textarea')?.value).toBe('newer unsaved work\n');expect(host.textContent).toContain('Original save confirmed');expect(host.textContent).toContain('Unsaved changes');
 fixture=async(path,body)=>Response.json(command(body!,'command-two'));await click('Save');expect(writes).toHaveLength(2);expect(writes[1].expected_sha256).toBe(sha('submitted\n'));expect(writes[1].content).toBe('newer unsaved work\n');
 expect(localStorage.length).toBe(0);expect(sessionStorage.length).toBe(0);
});

it('recovers a lost create response with the exact original body/key rather than submitting a later draft',async()=>{
 let first=true;
 fixture=async(path,body)=>{if(body){if(first){first=false;throw Error('owned lost receipt');}return Response.json(command(body));}return Response.json({...source,state:'expired',result:undefined});};
 await edit('original request\n');await click('Save');await edit('later draft\n');await click('Check original save');
 expect(writes).toHaveLength(2);expect(writes[1]).toEqual(writes[0]);expect(host.querySelector('textarea')?.value).toBe('later draft\n');expect(host.textContent).toContain('Original save confirmed');
});

it('keeps a conflict draft and requires an explicit reviewed comparison basis before a new CAS save',async()=>{
 const current='model changed this file\n';let attempt=0;
 fixture=async(path,body)=>{
  if(path==='/workspace/writes'&&body){const value=command(body);if(++attempt===1)return Response.json({...value,state:'not_applied',result:undefined,error_code:'file_changed'});return Response.json(value);}
  if(path==='/workspace/observations'&&body)return Response.json({...source,id:'comparison-one',arguments:{path:'source.txt',full_file:true},result:{read:{path:'source.txt',content:current,sha256:sha(current),bytes:Buffer.byteLength(current),start_line:1,complete:true}}});
  return Response.json({...source,state:'expired',result:undefined});
 };
 await edit('human draft\n');await click('Save');expect(host.textContent).toContain('The file changed');await click('Read current file and compare');expect(host.querySelector<HTMLTextAreaElement>('textarea[aria-label="Current comparison file"]')?.value).toBe(current);
 const adopt=[...host.querySelectorAll('button')].find(item=>item.textContent==='Continue with this comparison basis')!;expect(adopt.disabled).toBe(true);
 await edit('reviewed merged result\n','Merged draft');await act(async()=>host.querySelector<HTMLInputElement>('input[type=checkbox]')!.click());await click('Continue with this comparison basis');await click('Save');
 expect(writes).toHaveLength(2);expect(writes[1].expected_sha256).toBe(sha(current));expect(writes[1].content).toBe('reviewed merged result\n');
});

it('rejects a legal-looking applied receipt for the wrong file transition',()=>{
 const body={project_id:project,scope,workspace_policy_sha256:'b'.repeat(64),path:'source.txt',expected_sha256:sha(initial),content:'saved\n',content_sha256:sha('saved\n'),idempotency_key:'save-one'};
 const value=command(body);expect(validRemoteWrite(value,body)).toBe(true);value.result!.previous_sha256='e'.repeat(64);expect(validRemoteWrite(value,body)).toBe(false);
});

it('keeps the draft but releases a create identity after a first definite permission refusal',async()=>{
 fixture=async(path,body)=>body?Response.json({error:{code:'forbidden',message:'No write permission'}},{status:403}):Response.json({...source,state:'expired',result:undefined});
 await edit('kept after refusal\n');await click('Save');expect(host.querySelector('textarea')?.value).toBe('kept after refusal\n');expect([...host.querySelectorAll('button')].find(button=>button.textContent==='Discard draft and close')?.disabled).toBe(false);expect(host.textContent).not.toContain('Close draft; keep original save');
});

it('requires an explicit inspection and preserves its original key after an inspection reply is lost',async()=>{
 const inspections:unknown[]=[];let submitted:WorkspaceWriteCreateRequest|undefined,readInspected=false;
 fixture=async(path,body)=>{
  if(path==='/workspace/writes'&&body){submitted=body;return Response.json({...command(body),state:'reconciling',result:undefined,error_code:'operation_unknown'});}
  if(path.endsWith('/inspect')){inspections.push(body);if(inspections.length===1)throw Error('owned inspection reply lost');readInspected=true;return Response.json({...command(submitted!),version:4,state:'reconciling',result:undefined,error_code:'operation_unknown',inspection_id:'inspect-one',inspection_state:'requested'});}
  if(path.startsWith('/workspace/writes/'))return Response.json({...command(submitted!),version:readInspected?5:3,state:readInspected?'not_applied':'reconciling',result:undefined,error_code:readInspected?'not_started':'operation_unknown',inspection_state:readInspected?'completed':''});
  return Response.json({...source,state:'expired',result:undefined});
 };
 await edit('uncertain original write\n');await click('Save');expect(inspections).toHaveLength(0);expect(writes).toHaveLength(1);
 await click('Check original save');await edit('later draft kept\n');await click('Check original save');expect(inspections).toHaveLength(2);expect(inspections[1]).toEqual(inspections[0]);expect(writes).toHaveLength(1);expect(host.textContent).toContain('Confirmed not applied');expect(host.querySelector('textarea')?.value).toBe('later draft kept\n');
});
