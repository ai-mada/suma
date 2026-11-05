// @vitest-environment jsdom
import {createHash,webcrypto} from 'node:crypto';
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {CloudCollaborationSource,CloudTurn,CloudTurnSource,CloudTurnView,CloudTurnEntry,CloudTurnPermission,CloudTurnCommand,CloudTurnPage} from '../../../packages/contracts/src';
import type {ApiCaller,Session} from './api';
import {ApiViewProvider,bindApiView} from './api-context';
import {setLocale} from './i18n';
import {WorkBuddyCloudTurns} from './WorkBuddyCloudTurns';

const callerOverride=vi.hoisted(()=>({current:undefined as ApiCaller|undefined}));
vi.mock('./api-context',async importOriginal=>{
 const actual=await importOriginal<typeof import('./api-context')>();
 return {...actual,useApi:()=>{const caller=actual.useApi();return callerOverride.current??caller;}};
});
const session:Session={user:{id:'user-a',name:'Owner',email:'owner@example.test'},organization:{id:'org-a',name:'Org'},projects:[]};
const stamp='2026-10-31T12:00:00Z',prompt='Continue the exact original engineering task';
const initialSource=():CloudCollaborationSource=>({project_id:'project-a',run_id:'run-a',node_id:'node-a',submission_id:'submission-a',submission_version:1,task_id:'provider-task-a',node_spec_sha256:'a'.repeat(64),request_sha256:'b'.repeat(64)});
const digest=(text:string)=>createHash('sha256').update(text,'utf8').digest('hex');
type Request={path:string;method:string;body?:Record<string,unknown>;headers:Headers};
type Reply=(request:Request)=>Promise<Response>;
let root:Root,host:HTMLDivElement,source:CloudCollaborationSource,turn:CloudTurn|undefined,head:number,entries:CloudTurnEntry[],permissions:CloudTurnPermission[],commands:CloudTurnCommand[],requests:Request[],reply:Reply,promptReceipts:Map<string,CloudTurn>,commandReceipts:Map<string,CloudTurnCommand>,onChanged:ReturnType<typeof vi.fn>;
const turnSource=():CloudTurnSource=>({collaboration:{...source},attempt:1,fence:2,dispatch_id:'dispatch-a',assignment_sha256:'c'.repeat(64)});
function makeTurn(patch:Partial<CloudTurn>={}):CloudTurn{return {id:'turn-a',version:1,operation_id:'original-prompt-operation',head_version_at_start:0,source:turnSource(),owner_id:session.user.id,state:'running',prompt_sha256:digest(prompt),started_at:stamp,deadline_at:'2026-10-31T12:15:00Z',overdue:false,usage_state:'unreported',...patch};}
function view():CloudTurnView{return {source:turnSource(),version:head,...(turn?{latest_turn:turn}:{}),allowed_actions:!turn||turn.state==='completed'?['prompt']:['cancel','reconcile']};}
function page(after=0):CloudTurnPage{const items=entries.slice(after,after+50);return {turn:turn!,items,permissions,commands,next_after:items.at(-1)?.sequence??after,has_more:after+items.length<entries.length};}
const viewPath=()=>'/workbuddy-cloud-submissions/'+source.submission_id+'/turns?project_id='+source.project_id;
const promptPath=()=>'/workbuddy-cloud-submissions/'+source.submission_id+'/turns';
const pagePath=(after=0)=>'/workbuddy-cloud-turns/turn-a?project_id=project-a&after='+after+'&limit=50';
function permission(id:string,approve_allowed=true):CloudTurnPermission{return {id,turn_id:'turn-a',version:1,tool_call_id:'tool-'+id,title:'Review the original tool action',details:'Write the approved project file',details_sha256:digest('Write the approved project file'),approve_allowed,state:'pending',options:[{id:'allow-one',name:'Allow this call',kind:'allow_once'},{id:'deny-one',name:'Reject this call',kind:'reject_once'},{id:'allow-always',name:'Always allow',kind:'allow_always'}]};}
async function normal(request:Request):Promise<Response>{
 if(request.method==='GET'&&request.path.includes('/turns?'))return Response.json(view());
 if(request.method==='GET'&&request.path.startsWith('/workbuddy-cloud-turns/'))return Response.json(page(Number(new URL(request.path,'https://suma.test').searchParams.get('after')||0)));
 if(request.path===promptPath()&&request.body){
  const body=request.body,key=String(body.idempotency_key);let saved=promptReceipts.get(key);
  if(!saved){saved=makeTurn({operation_id:key,head_version_at_start:Number(body.expected_version),prompt_sha256:digest(String(body.prompt)),source:{...turnSource(),collaboration:{...source,submission_version:Number(body.expected_submission_version)}}});promptReceipts.set(key,saved);turn=saved;head++;entries=[{sequence:1,kind:'assistant_text',text:'Persisted reply <script>not executable</script>'}];}
  return Response.json(saved,{status:202});
 }
 if(request.body&&request.path.startsWith('/workbuddy-cloud-turns/')){
  const body=request.body,key=String(body.idempotency_key);let command=commandReceipts.get(key);
  if(!command){const permissionID=/\/permissions\/([^/]+)\/reply$/.exec(request.path)?.[1],kind=permissionID?'permission':request.path.endsWith('/cancel')?'cancel':'reconcile';command={id:'command-'+(commandReceipts.size+1),turn_id:'turn-a',operation_id:key,kind,state:'prepared',...(permissionID?{permission_id:permissionID,option_id:String(body.option_id)}:{})};commandReceipts.set(key,command);commands.push(command);if(permissionID)permissions=permissions.map(item=>item.id===permissionID?{...item,state:'prepared',version:item.version+1}:item);}
  return Response.json(command,{status:request.path.endsWith('/reconcile')?200:202});
 }
 throw new Error('Unexpected platform request: '+request.path);
}
beforeEach(async()=>{
 callerOverride.current=undefined;await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.stubGlobal('crypto',webcrypto);sessionStorage.clear();source=initialSource();turn=undefined;head=0;entries=[];permissions=[];commands=[];requests=[];promptReceipts=new Map();commandReceipts=new Map();reply=normal;onChanged=vi.fn();host=document.createElement('div');document.body.append(host);root=createRoot(host);
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{const path=String(input).replace(/^\/api\/v1/,''),body:Record<string,unknown>|undefined=init?.body?JSON.parse(String(init.body)):undefined,request={path,body,method:init?.method||(body?'POST':'GET'),headers:new Headers(init?.headers)};requests.push(request);return reply(request);}));
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();callerOverride.current=undefined;vi.unstubAllGlobals();vi.restoreAllMocks();});
async function settle(){for(let n=0;n<6;n++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function render(identity=session,revision=0){await act(async()=>root.render(<ApiViewProvider session={identity}><p data-public-cloud-node>Public original node remains available</p><WorkBuddyCloudTurns source={source} revision={revision} runVersion={7} onChanged={onChanged}/></ApiViewProvider>));await settle();}
const selector=(name:string)=>'[data-cloud-turns-'+name+']';
function element<T extends HTMLElement=HTMLButtonElement>(name:string){const found=host.querySelector<T>(selector(name));expect(found,selector(name)).not.toBeNull();return found!;}
async function click(name:string){await act(async()=>element(name).click());await settle();}
async function open(){if(element('toggle').getAttribute('aria-expanded')!=='true')await click('toggle');}
async function compose(text=prompt){const input=element<HTMLTextAreaElement>('prompt');await act(async()=>{Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value')!.set!.call(input,text);input.dispatchEvent(new Event('input',{bubbles:true}));});await settle();}
const writes=()=>requests.filter(request=>request.method==='POST');
const state=()=>element('state').getAttribute('data-state');
function journalFor(operation:string){for(let index=0;index<sessionStorage.length;index++){const key=sessionStorage.key(index)!,value=sessionStorage.getItem(key)!;try{if(JSON.parse(value).idempotency_key===operation)return {key,value};}catch{/* Other UI preferences are not operation records. */}}throw new Error('Original command was not persisted');}
function option(permissionID:string,optionID:string){return host.querySelector<HTMLButtonElement>('[data-cloud-turns-permission="'+permissionID+'"][data-cloud-turns-option="'+optionID+'"]');}

it('only reads stored data on expansion and sends one explicit scoped prompt before displaying persisted text',async()=>{
 await render();expect(element('toggle').getAttribute('aria-expanded')).toBe('false');expect(requests).toHaveLength(0);await open();expect(requests.map(request=>request.method)).toEqual(['GET']);expect(requests[0].path).toBe(viewPath());
 await compose();expect(writes()).toHaveLength(0);await click('send');expect(writes()).toHaveLength(1);
 expect(writes()[0].path).toBe(promptPath());expect(writes()[0].body).toEqual({project_id:'project-a',expected_submission_version:1,expected_run_version:7,expected_version:0,prompt,idempotency_key:expect.any(String)});
 expect(writes()[0].headers.get('X-Suma-Expected-User')).toBe(session.user.id);expect(host.textContent).toContain('Persisted reply <script>not executable</script>');expect(host.querySelector('script')).toBeNull();
 expect(requests.some(request=>request.path===pagePath())).toBe(true);expect(requests.every(request=>request.path.startsWith('/workbuddy-cloud-'))).toBe(true);
});

it('renders stored artifact changes in sequence with literal previews and no new operation',async()=>{
 const preview='<a href="https://example.invalid/artifact">Read</a>\n<script>ignored()</script>';
 turn=makeTurn();
 head=1;
 entries=[
  {sequence:1,kind:'assistant_text',text:'Continuing the original task'},
  {sequence:2,kind:'artifact',artifact_type:'plan',artifact_event:'updated',text:preview},
  {sequence:3,kind:'artifact',artifact_type:'media',artifact_event:'deleted',text:''},
 ];
 await render();
 await open();
 const rows=host.querySelectorAll('.cloud-turns-entries>li');
 expect(rows).toHaveLength(3);
 expect(rows[0].textContent).toContain(entries[0].text);
 expect(rows[1].querySelector('small')?.textContent).toBe('2');
 expect(rows[1].textContent).toContain('Plan · Updated');
 expect(rows[1].querySelector('p')?.textContent).toBe(preview);
 expect(rows[1].querySelector('a,script,iframe,img')).toBeNull();
 expect(rows[2].textContent).toContain('Media · Removed');
 expect(rows[2].textContent).toContain('No text preview available.');
 await act(async()=>{await setLocale('zh-CN',{persist:false});});
 expect(rows[1].textContent).toContain('计划 · 已更新');
 expect(rows[2].textContent).toContain('媒体 · 已移除');
 expect(requests.some(request=>request.path===pagePath())).toBe(true);
 expect(requests.every(request=>request.method==='GET'&&[viewPath(),pagePath()].includes(request.path))).toBe(true);
 expect(state()).toBe('running');
});

it('offers only the real one-time permission choices and keeps redacted approvals and always choices unavailable',async()=>{
 const original=permission('permission-a'),specific={id:'allow-specific',name:'Allow only the reviewed file change',kind:'allow_once'};original.options.splice(1,0,specific);
 turn=makeTurn({state:'waiting_permission'});head=1;permissions=[original,permission('permission-b',false)];await render();await open();
 const allow=option('permission-a','allow-one');expect(allow).not.toBeNull();expect(allow!.disabled).toBe(false);
 expect(allow!.textContent).toContain(original.options[0].name);const distinct=option('permission-a',specific.id);expect(distinct).not.toBeNull();expect(distinct!.textContent).toContain(specific.name);expect(distinct!.disabled).toBe(false);
 for(const id of ['permission-a','permission-b']){const always=option(id,'allow-always');expect(!always||always.disabled).toBe(true);}
 const blocked=option('permission-b','allow-one');expect(!blocked||blocked.disabled).toBe(true);expect(option('permission-b','deny-one')?.disabled).toBe(false);
 await act(async()=>{allow!.click();allow!.click();});await settle();expect(writes()).toHaveLength(1);expect(writes()[0].path).toBe('/workbuddy-cloud-turns/turn-a/permissions/permission-a/reply');expect(writes()[0].body).toMatchObject({project_id:'project-a',expected_version:1,option_id:'allow-one'});
 const deny=option('permission-b','deny-one')!;await act(async()=>deny.click());await settle();expect(writes()).toHaveLength(2);expect(writes()[1].body).toMatchObject({option_id:'deny-one'});expect(writes().every(request=>!request.path.endsWith('/turns'))).toBe(true);
});

it('records a prepared cancel request without presenting it as a completed turn or stopped task',async()=>{
 turn=makeTurn();head=1;await render();await open();await click('cancel');
 expect(writes()).toHaveLength(1);expect(writes()[0].path).toBe('/workbuddy-cloud-turns/turn-a/cancel');expect(writes()[0].body).toEqual({project_id:'project-a',expected_version:1,idempotency_key:expect.any(String)});
 expect(commandReceipts.values().next().value?.state).toBe('prepared');expect(state()).toBe('running');expect(host.querySelector('[data-public-cloud-node]')?.textContent).toBe('Public original node remains available');
});

it('recovers original prompt and control keys across collapse, growing source versions and remount without treating GET as acknowledgement',async()=>{
 let losePrompt=true,loseCancel=true;reply=async request=>{if(request.method==='POST'&&request.path.endsWith('/turns')&&losePrompt){losePrompt=false;await normal(request);throw new Error('Prompt response lost after commit');}if(request.method==='POST'&&request.path.endsWith('/cancel')&&loseCancel){loseCancel=false;await normal(request);throw new Error('Cancel response lost after commit');}return normal(request);};
 await render();await open();await compose();await click('send');const original=writes()[0],journal=journalFor(String(original.body!.idempotency_key));await click('toggle');
 source={...source,submission_version:2};head=2;await render(session,1);expect(sessionStorage.getItem(journal.key)).toBe(journal.value);expect(writes()).toHaveLength(1);expect(host.querySelector(selector('recover'))).not.toBeNull();
 await act(async()=>root.unmount());root=createRoot(host);await render(session,2);await open();await click('refresh');expect(writes()).toHaveLength(1);expect(host.querySelector(selector('recover'))).not.toBeNull();
 await click('recover');expect(writes()).toHaveLength(2);expect(writes()[1].body).toEqual(original.body);expect(promptReceipts.size).toBe(1);expect(host.querySelector(selector('recover'))).toBeNull();
 await click('cancel');const control=writes().at(-1)!,controlJournal=journalFor(String(control.body!.idempotency_key));source={...source,submission_version:3};head=3;await render(session,3);await click('refresh');
 expect(sessionStorage.getItem(controlJournal.key)).toBe(controlJournal.value);expect(host.querySelector(selector('recover'))).not.toBeNull();await click('recover');
 const controls=writes().filter(request=>request.path.endsWith('/cancel'));expect(controls).toHaveLength(2);expect(controls[1].body).toEqual(control.body);expect(commandReceipts.size).toBe(1);expect(state()).toBe('running');
});

it('clears only private turn data on refusal and ignores a late response after the current private scope changes',async()=>{
 turn=makeTurn();head=1;entries=[{sequence:1,kind:'assistant_text',text:'Original private turn content'}];await render();await open();expect(host.textContent).toContain(entries[0].text);
 reply=async()=>Response.json({error:{code:'forbidden',message:'Private access changed'}},{status:403});await click('refresh');expect(host.textContent).not.toContain('Original private turn content');expect(host.querySelector('[data-public-cloud-node]')).not.toBeNull();
 const oldView=view();let finish!:(value:Response)=>void;reply=()=>new Promise(resolve=>{finish=resolve;});await click('refresh');const stale=finish;
 source={...source,task_id:'different-provider-task',request_sha256:'d'.repeat(64)};turn=undefined;head=0;entries=[];permissions=[];commands=[];const next={...session,user:{...session.user,id:'user-b'},organization:{...session.organization,id:'org-b'}};callerOverride.current=bindApiView(next);reply=normal;
 await render(next,1);await open();await act(async()=>stale(Response.json(oldView)));await settle();expect(host.textContent).not.toContain('Original private turn content');expect(writes()).toHaveLength(0);expect(host.querySelector('[data-public-cloud-node]')?.textContent).toBe('Public original node remains available');
});

it('does not send a prompt that cannot first be saved to recovery storage',async()=>{
 await render();await open();await compose();vi.spyOn(Storage.prototype,'setItem').mockImplementation(()=>{throw new Error('Storage full');});await click('send');
 expect(writes()).toHaveLength(0);expect(element<HTMLTextAreaElement>('prompt').value).toBe(prompt);expect(host.querySelector('[role="alert"]')).not.toBeNull();
});

it('pages the same persisted turn without repeating entries or creating another prompt',async()=>{
 vi.spyOn(document,'visibilityState','get').mockReturnValue('visible');
 turn=makeTurn();head=1;entries=Array.from({length:55},(_,index)=>({sequence:index+1,kind:'assistant_text',text:'archived-entry-'+(index+1)+'-end'}));await render();await open();
 expect(host.textContent).toContain('archived-entry-1-end');expect(host.textContent).not.toContain('archived-entry-55-end');
 await act(async()=>{await new Promise(resolve=>setTimeout(resolve,1100));});await settle();
 expect(host.textContent).toContain('archived-entry-55-end');expect(host.textContent?.split('archived-entry-1-end').length).toBe(2);expect(requests.some(request=>request.path===pagePath(50))).toBe(true);expect(turn.version).toBe(1);expect(writes()).toHaveLength(0);
 entries.push({sequence:56,kind:'assistant_text',text:'archived-entry-56-end'});turn={...turn,version:2,state:'completed',finished_at:'2026-10-31T12:01:00Z',stop_reason:'end_turn'};head=2;
 await act(async()=>{await new Promise(resolve=>setTimeout(resolve,1100));});await settle();
 expect(host.textContent).toContain('archived-entry-56-end');expect(state()).toBe('completed');const readCount=requests.filter(request=>request.method==='GET').length;
 await act(async()=>{await new Promise(resolve=>setTimeout(resolve,1100));});await settle();expect(requests.filter(request=>request.method==='GET')).toHaveLength(readCount);expect(writes()).toHaveLength(0);
});

it('keeps the exact journal pending after a mismatched success receipt even when a later GET shows the stored turn',async()=>{
 reply=async request=>{const response=await normal(request);if(request.method==='POST'&&request.path.endsWith('/turns'))return Response.json({...await response.json(),operation_id:'different-original-operation'},{status:202});return response;};
 await render();await open();await compose();await click('send');const original=writes()[0],journal=journalFor(String(original.body!.idempotency_key));expect(host.querySelector(selector('recover'))).not.toBeNull();expect(onChanged).not.toHaveBeenCalled();
 await click('refresh');expect(writes()).toHaveLength(1);expect(element<HTMLButtonElement>('send').disabled).toBe(true);expect(host.querySelector(selector('recover'))).not.toBeNull();expect(sessionStorage.getItem(journal.key)).toBe(journal.value);
});
