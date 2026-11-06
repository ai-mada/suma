// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {CodexUserInputSelection,CursorInputDecision,ClaudeInputDecision,ToolInput} from '../../../packages/contracts/src';
import type {ApiCaller,Session} from './api';
import {ApiViewProvider,bindApiView} from './api-context';
import {setLocale} from './i18n';
import {ToolInputPanel,type ToolInputPanelProps} from './ToolInputPanel';
import {toolInputRecord,toolInputScope,toolInputSelections} from './tool-input.test-support';
import {cursorToolInputScope,cursorToolInputRecord,cursorToolInputPlanRecord,cursorToolInputDecision} from './tool-input.test-support';
import {buildToolInputAnswerBody,buildToolInputIntent,toolInputRecoveryScope} from './tool-input-model';
import {claudeSDKInputScope,claudeSDKInputRecord,claudeSDKInputDecision} from './claude-sdk.test-support';

const callerOverride=vi.hoisted(()=>({current:undefined as ApiCaller|undefined}));
vi.mock('./api-context',async load=>{const actual=await load<typeof import('./api-context')>();return {...actual,useApi:()=>{const caller=actual.useApi();return callerOverride.current??caller;}};});
const identity:Session={user:{id:'user-a',name:'Person',email:'person@example.test'},organization:{id:'org-a',name:'Organization'},projects:[]};
const initial:ToolInputPanelProps={project:toolInputScope.project,runID:toolInputScope.runID,nodeID:toolInputScope.nodeID,attempt:toolInputScope.attempt,fence:toolInputScope.fence,protocol:'codex-app-server-v1',state:'running'};
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
  result={...row,version:row.version+1,state:'answered',allowed_actions:[],answer:{...(Object.hasOwn(body,'claude')?{claude:body.claude as ClaudeInputDecision}:Object.hasOwn(body,'cursor')?{cursor:body.cursor as CursorInputDecision}:{answers:body.answers as CodexUserInputSelection[]}),answer_sha256:'6'.repeat(64),actor_id:identity.user.id,answered_at:'2026-11-01T04:01:00Z'}};
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

it('T265 opens and focuses only the linked question once without changing either answer',async()=>{
 const linked='b'.repeat(32);
 items.push({...structuredClone(toolInputRecord),id:linked,state:'closed',allowed_actions:[]});
 await render({requestedInputID:linked});

 const target=host.querySelector<HTMLElement>('[data-tool-input-target]');
 expect(target?.dataset.toolInputTarget).toBe(linked);
 expect(target?.querySelector('[data-tool-input-card]')?.getAttribute('data-tool-input-state')).toBe('closed');
 expect(host.querySelectorAll('[data-tool-input-target]')).toHaveLength(1);
 expect(host.querySelector('[data-tool-input-toggle]')?.getAttribute('aria-expanded')).toBe('true');
 expect(document.activeElement).toBe(target?.querySelector('h4'));
 expect(host.querySelectorAll('input:checked')).toHaveLength(0);
 expect(writes()).toHaveLength(0);

 const toggle=host.querySelector<HTMLElement>('[data-tool-input-toggle]')!;
 toggle.focus();
 await click('[data-tool-input-refresh]');
 expect(document.activeElement).toBe(toggle);
 await click('[data-tool-input-toggle]');
 await render({requestedInputID:linked,revision:1});
 expect(toggle.getAttribute('aria-expanded')).toBe('false');
 expect(writes()).toHaveLength(0);
});

it('T265 preserves newer focus during a linked read and reports when the exact attempt has no linked question',async()=>{
 const linked='b'.repeat(32);
 items.push({...structuredClone(toolInputRecord),id:linked});
 let complete!:(response:Response)=>void;
 reply=()=>new Promise(resolve=>{complete=resolve;});
 await render({requestedInputID:linked});
 const outside=host.querySelector<HTMLElement>('[data-public-run]')!;
 outside.tabIndex=-1;
 outside.focus();
 await act(async()=>complete(Response.json({protocol,items})));
 await settle();
 expect(document.activeElement).toBe(outside);
 await click('.tool-input-panel-jump');
 expect(document.activeElement).toBe(host.querySelector('[data-tool-input-target] h4'));

 reply=normal;
 items.push({...structuredClone(toolInputRecord),id:'next-input',attempt:3,fence:10});
 await render({requestedInputID:linked,attempt:3,fence:10});
 expect(host.querySelector('[data-tool-input-target]')).toBeNull();
 expect(host.querySelector('[data-tool-input-card="'+linked+'"]')).toBeNull();
 expect(host.querySelector('[data-tool-input-card="next-input"]')).not.toBeNull();
 expect(host.textContent).toContain('The linked input is unavailable in this execution attempt.');
 expect(writes()).toHaveLength(0);
});

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
 const metadata=persisted.find(item=>item.value.request)!;expect(metadata.value).toMatchObject({input_id:'input-a',source_identity:{codex:{params_sha256:toolInputRecord.request.codex!.params_sha256}},request:{body:original.body}});
 expect(metadata.raw).not.toContain(toolInputRecord.request.codex!.questions[0].question);expect(metadata.raw).not.toContain('options');
 await click('[data-tool-input-refresh]');expect(onChanged).not.toHaveBeenCalled();expect(journals(key)).toHaveLength(2);
 expect(host.querySelector('[data-tool-input-status="saved"]')).not.toBeNull();expect(host.querySelector('[data-tool-input-status="written"]')).toBeNull();
 const nextInput='c'.repeat(32);
 items.push({...structuredClone(toolInputRecord),id:nextInput,attempt:3,fence:10,dispatch_id:'dispatch-next'});
 await render({attempt:3,fence:10,requestedInputID:nextInput});expect(host.querySelector('[data-tool-input-card="'+nextInput+'"]')).toBeNull();expect(host.querySelector('[data-tool-input-target]')).toBeNull();expect(journals(key)).toHaveLength(2);
 await act(async()=>root.unmount());root=createRoot(host);await render({attempt:3,fence:10,requestedInputID:nextInput});
 expect(requests.at(-1)?.path).toBe('/runs/run-a/nodes/develop/tool-inputs?project_id=project-a&attempt=2&fence=9');
 const remove=Storage.prototype.removeItem;
 const cleanup=vi.spyOn(Storage.prototype,'removeItem').mockImplementation(function(this:Storage,name:string){if(name.startsWith('suma-write-recovery-v1:'))throw Error('Fixture cleanup unavailable');return remove.call(this,name);});
 await click('[data-tool-input-recover]');expect(writes()).toHaveLength(2);expect(writes()[1].body).toEqual(original.body);expect(journals(key)).toHaveLength(2);expect(onChanged).not.toHaveBeenCalled();
 cleanup.mockRestore();await click('[data-tool-input-recover]');
 expect(writes()).toHaveLength(2);expect(journals(key)).toHaveLength(0);expect(onChanged).toHaveBeenCalledOnce();
 await open();expect(requests.at(-1)?.path).toBe('/runs/run-a/nodes/develop/tool-inputs?project_id=project-a&attempt=3&fence=10');
 expect(host.querySelector('[data-tool-input-card="input-a"]')).toBeNull();expect(host.querySelector('[data-tool-input-target="'+nextInput+'"] [data-tool-input-card="'+nextInput+'"]')).not.toBeNull();
 expect(saved.get(key)?.state).toBe('answered');expect(host.querySelector<HTMLButtonElement>('[data-tool-input-submit]')?.disabled).toBe(true);
});

it('T192 saves exact SDK choices once, restores the original callback body and tuple before terminal history, and removes currently denied questions',async()=>{
 protocol='claude-sdk-input-v1';items=[structuredClone(claudeSDKInputRecord)];const scope=claudeSDKInputScope;let lost=true;
 reply=async request=>{const response=await normal(request);if(request.method==='POST'&&lost){lost=false;throw TypeError('Original SDK answer saved; response lost');}return response;};
 await render(scope);await open();expect(writes()).toHaveLength(0);expect(host.querySelectorAll('input:checked')).toHaveLength(0);expect(host.textContent).not.toContain('Experimental input capability');
 for(const selection of claudeSDKInputDecision.answers!)for(const option of selection.option_ids)await click('[data-tool-input-claude-option="'+selection.question_id+'"][value="'+option+'"]');
 const submit=host.querySelector<HTMLButtonElement>('[data-tool-input-submit]')!;await act(async()=>{submit.click();submit.click();});await settle();expect(writes()).toHaveLength(1);
 const original=writes()[0],key=String(original.body!.idempotency_key);expect(original.path).toBe('/tool-inputs/'+claudeSDKInputRecord.id+'/answer');expect(original.body).toEqual({project_id:scope.project,expected_version:1,idempotency_key:key,claude:claudeSDKInputDecision});expect(original.body).not.toHaveProperty('answers');expect(original.body).not.toHaveProperty('cursor');expect(original.headers.get('X-Suma-Expected-User')).toBe(scope.userID);expect(onChanged).not.toHaveBeenCalled();
 const persisted=journals(key);expect(persisted).toHaveLength(2);const metadata=persisted.find(item=>item.value.request)!;expect(metadata.value).toMatchObject({source_identity:{claude:{callback_id:claudeSDKInputRecord.request.claude.callback_id,request_id:claudeSDKInputRecord.request.claude.request_id,params_sha256:claudeSDKInputRecord.request.claude.params_sha256}},request:{body:original.body}});expect(metadata.raw).not.toContain('input_json');expect(metadata.raw).not.toContain(claudeSDKInputRecord.request.claude.questions[0].prompt);
 await click('[data-tool-input-refresh]');expect(host.querySelector('[data-tool-input-status="saved"]')).not.toBeNull();expect(host.querySelector('[data-tool-input-status="written"]')).toBeNull();expect(host.querySelector('[data-tool-input-status="resolved"]')).toBeNull();expect(journals(key)).toHaveLength(2);
 items.push({...structuredClone(claudeSDKInputRecord),id:'sdk-input-next',attempt:3,fence:10,dispatch_id:'sdk-dispatch-next',request:{...claudeSDKInputRecord.request,idempotency_key:'original-sdk-input-next',claude:{...claudeSDKInputRecord.request.claude,session_id:'sdk-session-next',tool_use_id:'sdk-tool-next',request_id:'sdk-request-next',callback_id:'sdk-callback-next'}}});
 await render({...scope,attempt:3,fence:10,state:'succeeded'});expect(host.querySelector('[data-tool-input-card="sdk-input-next"]')).toBeNull();await act(async()=>root.unmount());root=createRoot(host);await render({...scope,attempt:3,fence:10,state:'succeeded'});expect(requests.at(-1)?.path).toBe('/runs/'+scope.runID+'/nodes/'+scope.nodeID+'/tool-inputs?project_id='+scope.project+'&attempt=2&fence=9');
 expect(host.querySelector<HTMLButtonElement>('[data-tool-input-recover]')?.disabled).toBe(false);await click('[data-tool-input-recover]');expect(writes()).toHaveLength(2);expect(writes()[1].body).toEqual(original.body);expect(writes()[1].headers.get('X-Suma-Expected-Organization')).toBe(scope.organizationID);expect(saved.size).toBe(1);expect(journals(key)).toHaveLength(0);expect(onChanged).toHaveBeenCalledOnce();await open();
 expect(requests.at(-1)?.path).toBe('/runs/'+scope.runID+'/nodes/'+scope.nodeID+'/tool-inputs?project_id='+scope.project+'&attempt=3&fence=10');expect(host.querySelector('[data-tool-input-card="'+claudeSDKInputRecord.id+'"]')).toBeNull();expect(host.querySelector('[data-tool-input-card="sdk-input-next"]')).not.toBeNull();expect(host.querySelector('[data-tool-input-submit]')).toBeNull();expect(writes()).toHaveLength(2);
 reply=async()=>Response.json({error:{code:'forbidden',message:'Current SDK input access denied'}},{status:403});await click('[data-tool-input-refresh]');expect(host.querySelector('[data-tool-input-card]')).toBeNull();expect(host.textContent).not.toContain(claudeSDKInputRecord.request.claude.questions[0].prompt);expect(host.querySelector('[data-public-run]')).not.toBeNull();expect(writes()).toHaveLength(2);
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
 expect(host.querySelector('[data-tool-input-card]')).toBeNull();expect(host.textContent).not.toContain(toolInputRecord.request.codex!.questions[0].question);expect(host.querySelector('[data-public-run]')).not.toBeNull();expect(writes()).toHaveLength(0);
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

it('T190 answers exact Cursor question IDs and restores a legal large selection under its original ACP scope and key',async()=>{
 protocol='cursor-acp-input-v1';items=[structuredClone(cursorToolInputRecord)];await render({protocol:'acp-stdio-v1'});await open();
 expect(host.querySelector<HTMLButtonElement>('[data-tool-input-submit]')?.disabled).toBe(true);expect(writes()).toHaveLength(0);
 for(const answer of cursorToolInputDecision.answers||[])for(const option of answer.option_ids)await click('[data-tool-input-cursor-option="'+answer.question_id+'"][value="'+option+'"]');
 expect(writes()).toHaveLength(0);await click('[data-tool-input-submit]');
 expect(writes()).toHaveLength(1);expect(writes()[0].body).toEqual({project_id:'project-a',expected_version:1,idempotency_key:expect.any(String),cursor:cursorToolInputDecision});expect(writes()[0].body).not.toHaveProperty('answers');
 expect(host.querySelector('[data-tool-input-status="saved"]')).not.toBeNull();expect(host.querySelector('[data-tool-input-status="written"]')).toBeNull();
 await act(async()=>root.unmount());root=createRoot(host);saved.clear();
 const large=structuredClone(cursorToolInputRecord);large.id='cursor-large-input';large.request.idempotency_key='large-native-question';large.request.cursor!.request_id={kind:'number',value:'99'};
 const answerIDs:NonNullable<CursorInputDecision['answers']>=[];
 large.request.cursor!.questions!.questions=Array.from({length:16},(_,index)=>{
  const prefix='question-'+index+'-',id=prefix+'q'.repeat(256-prefix.length),options=Array.from({length:32},(_,offset)=>{const head='option-'+index+'-'+offset+'-';return {id:head+'x'.repeat(256-head.length),label:'Option '+index+'/'+offset};});
  answerIDs.push({question_id:id,option_ids:options.map(option=>option.id)});return {id,prompt:'Original large question '+index+' must not enter recovery metadata.',allow_multiple:true,options};
 });
 const decision:CursorInputDecision={outcome:'answered',answers:answerIDs},key='large-cursor-answer-original-key',body=buildToolInputAnswerBody(large,cursorToolInputScope,{cursor:decision},key);expect(body).toBeDefined();
 const request={path:'/tool-inputs/'+large.id+'/answer',method:'POST',idempotency_key:key,body:body!},intent=buildToolInputIntent(large,cursorToolInputScope,request);expect(intent).toBeDefined();
 const raw=JSON.stringify(intent);expect(new TextEncoder().encode(raw).length).toBeGreaterThan(65536);expect(raw).not.toContain('must not enter recovery metadata');
 sessionStorage.setItem('suma-tool-input-intent:'+toolInputRecoveryScope(cursorToolInputScope),raw);sessionStorage.setItem('suma-write-recovery-v1:tool-inputs:'+toolInputRecoveryScope(cursorToolInputScope),JSON.stringify(request));
 const receipt:ToolInput={...large,version:2,state:'answered',allowed_actions:[],answer:{cursor:decision,answer_sha256:'6'.repeat(64),actor_id:identity.user.id,answered_at:'2026-11-01T04:01:00Z'}};
 saved.set(key,receipt);items=[receipt,{...structuredClone(cursorToolInputRecord),id:'cursor-next-attempt',attempt:3,fence:10}];
 await render({protocol:'acp-stdio-v1',attempt:3,fence:10,state:'succeeded'});expect(requests.at(-1)?.path).toContain('attempt=2&fence=9');expect(journals(key)).toHaveLength(2);
 await click('[data-tool-input-recover]');expect(writes()).toHaveLength(2);expect(writes()[1].body).toEqual(body);expect(journals(key)).toHaveLength(0);expect(requests.at(-1)?.path).toContain('attempt=3&fence=10');
 await open();expect(host.querySelector('[data-tool-input-card="cursor-next-attempt"]')).not.toBeNull();expect(host.querySelector('[data-tool-input-submit]')).toBeNull();
});

it('T190 previews the complete Cursor plan and saves only an explicit typed decision without granting permissions or caching plan text',async()=>{
 protocol='cursor-acp-input-v1';const plan=structuredClone(cursorToolInputPlanRecord);plan.request.cursor!.plan!.plan+='\nUnique controller plan body <script>inert()</script>';items=[plan];
 const persisted:string[]=[],setItem=Storage.prototype.setItem;vi.spyOn(Storage.prototype,'setItem').mockImplementation(function(this:Storage,key:string,value:string){persisted.push(value);return setItem.call(this,key,value);});
 await render({protocol:'acp-stdio-v1'});await open();expect(host.querySelector('[data-tool-input-plan]')?.textContent).toBe(plan.request.cursor!.plan!.plan);expect(host.querySelector('script')).toBeNull();
 expect(host.querySelector<HTMLInputElement>('[data-tool-input-decision="accepted"]')?.checked).toBe(false);expect(host.querySelector<HTMLButtonElement>('[data-tool-input-submit]')?.disabled).toBe(true);
 await click('[data-tool-input-decision="accepted"]');expect(writes()).toHaveLength(0);await click('[data-tool-input-submit]');expect(writes()).toHaveLength(1);
 expect(writes()[0].body).toEqual({project_id:'project-a',expected_version:1,idempotency_key:expect.any(String),cursor:{outcome:'accepted'}});expect(writes()[0].body).not.toHaveProperty('answers');expect(writes()[0].body).not.toHaveProperty('planUri');
 expect(persisted.some(value=>value.includes('Unique controller plan body'))).toBe(false);expect(requests.every(request=>request.path.includes('/tool-inputs'))).toBe(true);expect(onChanged).toHaveBeenCalledOnce();
 const answered=items[0];items=[{...answered,version:3,observation:{sequence:1,params_sha256:answered.request.cursor!.params_sha256,answer_sha256:answered.answer!.answer_sha256,answer_observed:true,reply_started:true,reply_written:false,request_resolved:false,execution_closed:true,failure_code:'response_write_unknown'}}];
 await click('[data-tool-input-refresh]');expect(host.querySelector('[data-tool-input-status="execution_closed"]')).not.toBeNull();expect(host.querySelector('[data-tool-input-status="written"]')).toBeNull();expect(host.querySelector('[data-tool-input-status="unknown"]')).not.toBeNull();expect(writes()).toHaveLength(1);
 protocol='';items=[];await render({protocol:'acp-stdio-v1',nodeID:'qoder-without-inputs',revision:1});expect(host.querySelector('[data-tool-input-panel]')).toBeNull();expect(writes()).toHaveLength(1);
});
