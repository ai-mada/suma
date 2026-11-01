// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {CodexUserInputSelection,ToolInput} from '../../../packages/contracts/src';
import type {ApiCaller,Session} from './api';
import {ApiViewProvider,bindApiView} from './api-context';
import {setLocale} from './i18n';
import {ToolInputPanel,type ToolInputPanelProps} from './ToolInputPanel';
import {toolInputRecord,toolInputScope,toolInputSelections} from './tool-input.test-support';

const callerOverride=vi.hoisted(()=>({current:undefined as ApiCaller|undefined}));
vi.mock('./api-context',async load=>{const actual=await load<typeof import('./api-context')>();return {...actual,useApi:()=>{const caller=actual.useApi();return callerOverride.current??caller;}};});
const identity:Session={user:{id:'user-a',name:'Person',email:'person@example.test'},organization:{id:'org-a',name:'Organization'},projects:[]};
const initial:ToolInputPanelProps={project:toolInputScope.project,runID:toolInputScope.runID,nodeID:toolInputScope.nodeID,attempt:toolInputScope.attempt,fence:toolInputScope.fence,state:'running'};
type Request={path:string;method:string;body?:Record<string,unknown>;headers:Headers};
let host:HTMLDivElement,root:Root,items:ToolInput[],protocol:string,requests:Request[],reply:(request:Request)=>Promise<Response>,saved:Map<string,ToolInput>;
const onChanged=vi.fn();
async function normal(request:Request):Promise<Response>{
 if(request.method==='GET'){
  const url=new URL(request.path,'https://fixture.test'),attempt=Number(url.searchParams.get('attempt')),fence=Number(url.searchParams.get('fence'));
  return Response.json({protocol,items:items.filter(item=>item.attempt===attempt&&item.fence===fence&&url.pathname==='/runs/'+item.run_id+'/nodes/'+item.node_id+'/tool-inputs')});
 }
 const body=request.body!,key=String(body.idempotency_key);let result=saved.get(key);
 if(!result){
  const row=items.find(item=>request.path==='/tool-inputs/'+item.id+'/answer')!;
  result={...row,version:row.version+1,state:'answered',allowed_actions:[],answer:{answers:body.answers as CodexUserInputSelection[],answer_sha256:'6'.repeat(64),actor_id:identity.user.id,answered_at:'2026-11-01T04:01:00Z'}};
  saved.set(key,result);items=items.map(item=>item.id===result!.id?result!:item);
 }
 return Response.json(result);
}
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});sessionStorage.clear();callerOverride.current=undefined;onChanged.mockReset();
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);items=[structuredClone(toolInputRecord)];protocol='codex-user-input-v1';requests=[];saved=new Map();reply=normal;
 host=document.createElement('div');document.body.append(host);root=createRoot(host);
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{
  const body=init?.body?JSON.parse(String(init.body)) as Record<string,unknown>:undefined;
  const request={path:String(input).replace(/^\/api\/v1/,''),method:init?.method||(body?'POST':'GET'),body,headers:new Headers(init?.headers)};
  requests.push(request);return reply(request);
 }));
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();callerOverride.current=undefined;vi.restoreAllMocks();vi.unstubAllGlobals();});
async function settle(){for(let index=0;index<5;index++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function render(props:Partial<ToolInputPanelProps>={},session=identity){
 await act(async()=>root.render(<ApiViewProvider session={session}><p data-public-run>Original run remains available</p><ToolInputPanel {...initial} {...props} onChanged={onChanged}/></ApiViewProvider>));await settle();
}
async function click(selector:string){const button=host.querySelector<HTMLElement>(selector);expect(button,selector).not.toBeNull();await act(async()=>button!.click());await settle();}
async function open(){if(host.querySelector('[data-tool-input-toggle]')?.getAttribute('aria-expanded')!=='true')await click('[data-tool-input-toggle]');}
async function change(selector:string,value:string){
 const field=host.querySelector<HTMLTextAreaElement>(selector);expect(field,selector).not.toBeNull();
 await act(async()=>{Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value')!.set!.call(field,value);field!.dispatchEvent(new Event('input',{bubbles:true}));});await settle();
}
async function chooseAnswers(){
 await click('[data-tool-input-option="approach"][value="Small change"]');
 await change('[data-tool-input-text="approach"]','Keep the public API.');
 await change('[data-tool-input-text="details"]','Use the existing parser.');
 await click('[data-tool-input-other="validation"]');
 await change('[data-tool-input-text="validation"]','Run the relevant checks only.');
}
const writes=()=>requests.filter(request=>request.method==='POST');
function journals(key:string){
 return Array.from({length:sessionStorage.length},(_,index)=>{const name=sessionStorage.key(index)!,raw=sessionStorage.getItem(name)!;let value:Record<string,any>={};try{value=JSON.parse(raw);}catch{/* Other session preferences are not journals. */}return {name,raw,value};})
  .filter(item=>item.value.idempotency_key===key||item.value.request?.idempotency_key===key);
}

it('T187 preserves one explicit original answer and key through an unknown receipt, remount and storage cleanup without repeating a settled HTTP write',async()=>{
 let lost=true;reply=async request=>{const response=await normal(request);if(request.method==='POST'&&lost){lost=false;throw TypeError('Original receipt lost after save');}return response;};
 await render();expect(requests[0].path).toBe('/runs/run-a/nodes/develop/tool-inputs?project_id=project-a&attempt=2&fence=9');
 expect(requests[0].headers.get('X-Suma-Expected-User')).toBe('user-a');await open();await chooseAnswers();
 const submit=host.querySelector<HTMLButtonElement>('[data-tool-input-submit]')!;
 await act(async()=>{submit.click();submit.click();});await settle();
 expect(writes()).toHaveLength(1);const original=writes()[0],key=String(original.body!.idempotency_key);
 expect(original.path).toBe('/tool-inputs/input-a/answer');expect(original.body).toEqual({project_id:'project-a',expected_version:1,idempotency_key:key,answers:toolInputSelections});
 expect(original.headers.get('X-Suma-Expected-Organization')).toBe('org-a');expect(onChanged).not.toHaveBeenCalled();
 const persisted=journals(key);expect(persisted).toHaveLength(2);
 const metadata=persisted.find(item=>item.value.request)!;expect(metadata.value).toMatchObject({input_id:'input-a',source_identity:{codex:{params_sha256:toolInputRecord.request.codex.params_sha256}},request:{body:original.body}});
 expect(metadata.raw).not.toContain(toolInputRecord.request.codex.questions[0].question);expect(metadata.raw).not.toContain('options');
 await click('[data-tool-input-refresh]');expect(onChanged).not.toHaveBeenCalled();expect(journals(key)).toHaveLength(2);
 expect(host.querySelector('[data-tool-input-status="saved"]')).not.toBeNull();expect(host.querySelector('[data-tool-input-status="written"]')).toBeNull();
 items.push({...structuredClone(toolInputRecord),id:'input-next',attempt:3,fence:10,dispatch_id:'dispatch-next'});
 await render({attempt:3,fence:10});expect(host.querySelector('[data-tool-input-card="input-next"]')).toBeNull();expect(journals(key)).toHaveLength(2);
 await act(async()=>root.unmount());root=createRoot(host);await render({attempt:3,fence:10});
 expect(requests.at(-1)?.path).toBe('/runs/run-a/nodes/develop/tool-inputs?project_id=project-a&attempt=2&fence=9');
 const remove=Storage.prototype.removeItem;
 const cleanup=vi.spyOn(Storage.prototype,'removeItem').mockImplementation(function(this:Storage,name:string){if(name.startsWith('suma-write-recovery-v1:'))throw Error('Fixture cleanup unavailable');return remove.call(this,name);});
 await click('[data-tool-input-recover]');expect(writes()).toHaveLength(2);expect(writes()[1].body).toEqual(original.body);expect(journals(key)).toHaveLength(2);expect(onChanged).not.toHaveBeenCalled();
 cleanup.mockRestore();await click('[data-tool-input-recover]');
 expect(writes()).toHaveLength(2);expect(journals(key)).toHaveLength(0);expect(onChanged).toHaveBeenCalledOnce();
 await open();expect(requests.at(-1)?.path).toBe('/runs/run-a/nodes/develop/tool-inputs?project_id=project-a&attempt=3&fence=10');
 expect(host.querySelector('[data-tool-input-card="input-a"]')).toBeNull();expect(host.querySelector('[data-tool-input-card="input-next"]')).not.toBeNull();
 expect(saved.get(key)?.state).toBe('answered');expect(host.querySelector<HTMLButtonElement>('[data-tool-input-submit]')?.disabled).toBe(true);
});

it('T187 isolates late source and caller reads, keeps capability-off or terminal history read-only, and clears current denied private facts',async()=>{
 let finish!:(response:Response)=>void;reply=()=>new Promise(resolve=>{finish=resolve;});await render();
 const late=structuredClone(items);items=[{...toolInputRecord,id:'input-b',node_id:'next-node',attempt:3,fence:10}];reply=normal;
 await render({nodeID:'next-node',attempt:3,fence:10});await act(async()=>finish(Response.json({protocol,items:late})));await settle();await open();
 expect(host.querySelector('[data-tool-input-card="input-a"]')).toBeNull();expect(host.querySelector('[data-tool-input-card="input-b"]')).not.toBeNull();
 reply=()=>new Promise(resolve=>{finish=resolve;});await click('[data-tool-input-refresh]');
 callerOverride.current=bindApiView(identity);items=[];reply=normal;await render({nodeID:'next-node',attempt:3,fence:10});
 await act(async()=>finish(Response.json({protocol,items:[{...toolInputRecord,id:'late-caller-private',node_id:'next-node',attempt:3,fence:10}]})));await settle();await open();
 expect(host.querySelector('[data-tool-input-card]')).toBeNull();
 protocol='';items=[structuredClone(toolInputRecord)];await render({revision:1});await open();
 expect(host.querySelector('[data-tool-input-card="input-a"]')).not.toBeNull();expect(host.querySelector('[data-tool-input-submit]')).toBeNull();
 protocol='codex-user-input-v1';await render({state:'succeeded',revision:2});await open();expect(host.querySelector('[data-tool-input-submit]')).toBeNull();
 reply=async()=>Response.json({error:{code:'forbidden',message:'Current input access denied'}},{status:403});await click('[data-tool-input-refresh]');
 expect(host.querySelector('[data-tool-input-card]')).toBeNull();expect(host.textContent).not.toContain(toolInputRecord.request.codex.questions[0].question);expect(host.querySelector('[data-public-run]')).not.toBeNull();expect(writes()).toHaveLength(0);
 reply=normal;protocol='';items=[];await render({nodeID:'unused-node',revision:3});expect(host.querySelector('[data-tool-input-panel]')).toBeNull();
});

it('T187 blocks unpersisted answers and new input after a mismatched receipt, retaining the original operation even when current access is denied',async()=>{
 await render();await open();await chooseAnswers();
 const store=vi.spyOn(Storage.prototype,'setItem').mockImplementation(()=>{throw Error('Fixture storage unavailable');});await click('[data-tool-input-submit]');expect(writes()).toHaveLength(0);store.mockRestore();
 await click('[data-tool-input-refresh]');
 reply=async request=>{const response=await normal(request);if(request.method!=='POST')return response;const row=await response.json();return Response.json({...row,answer:{...row.answer,actor_id:'another-person'}});};
 await click('[data-tool-input-submit]');expect(writes()).toHaveLength(1);const original=writes()[0],key=String(original.body!.idempotency_key);
 expect(onChanged).not.toHaveBeenCalled();expect(journals(key)).toHaveLength(2);expect(host.querySelector('[data-tool-input-recover]')).not.toBeNull();
 reply=async()=>Response.json({error:{code:'forbidden',message:'Current input access denied'}},{status:403});await click('[data-tool-input-refresh]');
 expect(host.querySelector('[data-tool-input-card]')).toBeNull();expect(host.querySelector('[data-tool-input-recover]')).toBeNull();expect(journals(key)).toHaveLength(2);expect(writes()).toHaveLength(1);
});
