// @vitest-environment jsdom
import {webcrypto} from 'node:crypto';
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {CloudCollaborationSource,CloudCollaborationView,CloudHistoryLoad,CloudHistoryEntry,CloudHistoryPage} from '../../../packages/contracts/src';
import type {Session} from './api';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import {WorkBuddyCloudCollaboration} from './WorkBuddyCloudCollaboration';
import {WorkBuddyCloudTaskPanel,type WorkBuddyCloudTaskModel} from './WorkBuddyCloudTaskPanel';

const session:Session={user:{id:'user-a',name:'A',email:'a@example.test'},organization:{id:'org-a',name:'Organization'},projects:[]};
const initialSource=():CloudCollaborationSource=>({project_id:'project-a',run_id:'run-a',node_id:'node-a',submission_id:'submission-a',submission_version:1,task_id:'task-a',node_spec_sha256:'a'.repeat(64),request_sha256:'b'.repeat(64)});
let root:Root|undefined,host:HTMLDivElement,source:CloudCollaborationSource,view:CloudCollaborationView,loads:Map<string,CloudHistoryLoad>,entries:Map<string,CloudHistoryEntry[]>,calls:{path:string;body?:Record<string,unknown>;headers:Headers}[];
const makeLoad=(id:string,items:CloudHistoryEntry[]=[],patch:Partial<CloudHistoryLoad>={}):CloudHistoryLoad=>({id,version:2,operation_id:'operation-'+id,head_version_at_start:0,source:{...source},state:'loaded',coverage:'unverified',started_at:'2026-10-31T00:00:00Z',deadline_at:'2026-10-31T00:00:30Z',overdue:false,finished_at:'2026-10-31T00:00:05Z',entry_count:items.length,unsupported_count:items.filter(item=>item.kind==='unsupported').length,text_bytes:items.reduce((sum,item)=>sum+new TextEncoder().encode(item.text||'').length,0),...patch});
function saved(load:CloudHistoryLoad,items:CloudHistoryEntry[]=[]){loads.set(load.id,load);entries.set(load.id,items);}
function pageFor(load:CloudHistoryLoad,after=0):CloudHistoryPage{const items=(entries.get(load.id)||[]).slice(after,after+50),next=items.at(-1)?.sequence||after;return {load,items,total:load.entry_count,next_after:next,has_more:next<load.entry_count};}
async function settle(){for(let n=0;n<6;n++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function render(account=session,revision=0){await act(async()=>{root??=createRoot(host);root.render(<ApiViewProvider session={account}><p>Public cloud progress</p><WorkBuddyCloudCollaboration source={source} revision={revision}/></ApiViewProvider>);});await settle();}
async function click(action:string){const button=host.querySelector<HTMLButtonElement>('[data-collaboration-action="'+action+'"]');expect(button,action).not.toBeNull();await act(async()=>button!.click());await settle();}
function transport(handler?:(path:string,body?:Record<string,unknown>)=>Promise<Response|undefined>){vi.stubGlobal('fetch',vi.fn(async(input:string,options?:RequestInit)=>{const path=String(input).replace(/^\/api\/v1/,''),body=options?.body?JSON.parse(String(options.body)) as Record<string,unknown>:undefined;calls.push({path,body,headers:new Headers(options?.headers)});if(handler){const response=await handler(path,body);if(response)return response;}if(path.includes('/collaboration?'))return Response.json(view);if(path.startsWith('/workbuddy-cloud-history-loads/')){const url=new URL(path,'http://suma.test'),load=loads.get(decodeURIComponent(url.pathname.split('/').at(-1)!));if(load)return Response.json(pageFor(load,Number(url.searchParams.get('after')||0)));}return Response.json({error:{code:'missing',message:'unavailable'}},{status:404});}));}
beforeEach(async()=>{await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.stubGlobal('crypto',webcrypto);sessionStorage.clear();host=document.createElement('div');document.body.append(host);source=initialSource();view={source:{...source},version:0,allowed_actions:['load_history']};loads=new Map();entries=new Map();calls=[];});
afterEach(async()=>{if(root)await act(async()=>root!.unmount());root=undefined;host.remove();sessionStorage.clear();vi.unstubAllGlobals();vi.restoreAllMocks();await setLocale('zh-CN',{persist:false});});

it('only reads stored projections until an explicit load sends current source and an explicit zero head version',async()=>{
 const text:CloudHistoryEntry[]=[{sequence:1,kind:'assistant_text',text:'Original <script>request</script> [link](https://example.test)'}];
 transport(async(path,body)=>{if(body){const loaded=makeLoad('load-new',text,{operation_id:String(body.idempotency_key),head_version_at_start:Number(body.expected_version)});saved(loaded,text);view={source:{...source},version:2,latest_load:loaded,last_loaded:loaded,allowed_actions:['load_history']};return Response.json(loaded);}return undefined;});
 await render();expect(calls.filter(call=>call.body)).toHaveLength(0);expect(host.textContent).toContain('No session load has been recorded');
 expect(calls[0].headers.get('X-Suma-Expected-User')).toBe('user-a');expect(calls[0].headers.get('X-Suma-Expected-Organization')).toBe('org-a');
 await click('read');expect(calls.filter(call=>call.body)).toHaveLength(0);
 await click('load');const command=calls.find(call=>call.body)!;expect(command.path).toBe('/workbuddy-cloud-submissions/submission-a/history-loads');expect(command.body).toMatchObject({project_id:'project-a',expected_submission_version:1,expected_version:0});expect(Object.keys(command.body!).sort()).toEqual(['expected_submission_version','expected_version','idempotency_key','project_id']);
 expect(host.querySelector('.cloud-collaboration-entries')?.textContent).toContain(text[0].text);expect(host.querySelector('.cloud-collaboration-entries script')).toBeNull();expect(host.querySelector('.cloud-collaboration-entries a')).toBeNull();expect(host.querySelector<HTMLDetailsElement>('.cloud-collaboration-source')?.open).toBe(false);expect(calls.some(call=>call.path.includes('workbuddy-connections'))).toBe(false);
});

it('replays a saved artifact as metadata while keeping only the text entry shareable',async()=>{
 const items:CloudHistoryEntry[]=[{sequence:1,kind:'artifact',artifact_type:'plan',artifact_event:'created'},
  {sequence:2,kind:'assistant_text',text:'Original session reply'}];
 const loaded=makeLoad('artifact-load',items);saved(loaded,items);
 view={source:{...source},version:2,latest_load:loaded,last_loaded:loaded,allowed_actions:[]};
 const onShareSelection=vi.fn();transport();
 await act(async()=>{root=createRoot(host);root.render(<ApiViewProvider session={session}><WorkBuddyCloudCollaboration source={source} onShareSelection={onShareSelection}/></ApiViewProvider>);});await settle();

 const artifact=host.querySelector('.cloud-collaboration-entries li[value="1"]');
 expect(artifact).not.toBeNull();
 expect(artifact?.textContent).toContain('Plan · Added');
 expect(artifact?.textContent).toContain('Only the artifact type and change were saved.');
 expect(artifact?.querySelector('a, button, input, pre')).toBeNull();
 expect(host.querySelectorAll('[data-history-share-select]')).toHaveLength(1);
 expect(host.querySelector('[data-history-share-select="2"]')).not.toBeNull();
 expect(host.textContent).toContain('Coverage is unverified');
 expect(onShareSelection.mock.calls.every(([value])=>value===undefined)).toBe(true);
 expect(calls.filter(call=>call.body)).toHaveLength(0);
});

it('retains an older confirmed observation beside a limited attempt and pages only the local archive',async()=>{
 const oldSource={...source},oldItems:CloudHistoryEntry[]=Array.from({length:51},(_,n)=>({sequence:n+1,kind:'assistant_text',text:'saved entry '+(n+1)})),old=makeLoad('old-loaded',oldItems,{source:oldSource});saved(old,oldItems);
 source={...source,submission_version:3};const prefix:CloudHistoryEntry[]=[{sequence:1,kind:'user_text',text:'latest partial prefix'}],latest=makeLoad('new-limited',prefix,{source:{...source},head_version_at_start:2,state:'limited',error_code:'workbuddy_cloud_history_limited',started_at:'2026-10-31T00:01:00Z',deadline_at:'2026-10-31T00:01:30Z',finished_at:'2026-10-31T00:01:05Z'});saved(latest,prefix);view={source:{...source},version:4,latest_load:latest,last_loaded:old,allowed_actions:[]};transport();
 await render();expect(host.textContent).toContain('This load reached a limit');expect(host.textContent).toContain('previous confirmed observation is retained');expect(host.querySelectorAll('.cloud-collaboration-entries li')).toHaveLength(50);expect(host.querySelector('[data-collaboration-action="load"]')).toBeNull();
 await click('more');expect(host.querySelectorAll('.cloud-collaboration-entries li')).toHaveLength(51);expect(calls.some(call=>call.path.includes('after=50&limit=50'))).toBe(true);
 await click('latest');expect(host.querySelector('.cloud-collaboration-entries')?.textContent).toContain('latest partial prefix');expect(host.querySelector('.cloud-collaboration-entries')?.textContent).not.toContain('saved entry 1');expect(calls.filter(call=>call.body)).toHaveLength(0);
});

it('keeps a mismatched receipt pending and recovers its original body and key after an observation version advances',async()=>{
 let original:CloudHistoryLoad|undefined,posts=0;transport(async(path,body)=>{if(body){posts++;if(!original){original=makeLoad('original-load',[],{operation_id:String(body.idempotency_key),head_version_at_start:Number(body.expected_version)});saved(original);view={source:{...source,submission_version:2},version:2,latest_load:original,last_loaded:original,allowed_actions:['load_history']};return Response.json({...original,operation_id:'different-operation'});}return Response.json(original);}return undefined;});
 await render();await click('load');expect(host.textContent).toContain('Original session load needs checking');expect(sessionStorage.length).toBe(1);const first=structuredClone(calls.find(call=>call.body)!.body);
 await click('read');expect(sessionStorage.length).toBe(1);expect(host.querySelector<HTMLButtonElement>('[data-collaboration-action="load"]')?.disabled).toBe(true);
 await click('recover');expect(posts).toBe(2);expect(calls.filter(call=>call.body).map(call=>call.body)).toEqual([first,first]);expect(sessionStorage.length).toBe(0);expect(host.textContent).toContain('This load was confirmed');
});

it('clears private session content after a current refusal while public node facts remain',async()=>{
 const items:CloudHistoryEntry[]=[{sequence:1,kind:'user_text',text:'private original session text'}],loaded=makeLoad('private-load',items);saved(loaded,items);view={source:{...source},version:2,latest_load:loaded,last_loaded:loaded,allowed_actions:[]};let deny=false;transport(async()=>deny?Response.json({error:{code:'forbidden',message:'no access'}},{status:403}):undefined);
 await render();expect(host.textContent).toContain('private original session text');deny=true;await click('read');expect(host.textContent).not.toContain('private original session text');expect(host.textContent).toContain('Session observations are unavailable');expect(host.textContent).toContain('Public cloud progress');expect(host.querySelector('.cloud-collaboration-source')).toBeNull();
});

it('clears only private records when a saved-read source changes without treating a POST head conflict as source loss',async()=>{
 const items:CloudHistoryEntry[]=Array.from({length:51},(_,n)=>({sequence:n+1,kind:'user_text',text:'source-protected-entry-'+(n+1)})),loaded=makeLoad('source-bound-load',items);saved(loaded,items);view={source:{...source},version:2,latest_load:loaded,last_loaded:loaded,allowed_actions:['load_history']};
 let fail:''|'post'|'metadata'|'page'='';transport(async(path,body)=>{if(fail==='post'&&body)return Response.json({error:{code:'workbuddy_cloud_conflict',message:'head changed'}},{status:409});if(fail==='metadata'&&path.includes('/collaboration?')||fail==='page'&&path.startsWith('/workbuddy-cloud-history-loads/'))return Response.json({error:{code:'workbuddy_cloud_changed',message:'original source changed'}},{status:409});return undefined;});
 const publicModel:WorkBuddyCloudTaskModel={scopeKey:'public-original',sourceVersion:'original',state:'ready',task:{title:'Public original cloud task',runLabel:'Original run',nodeLabel:'Original node',prompt:'Public fixed request',requestDigest:source.request_sha256,nodeDigest:source.node_spec_sha256},connections:[],actions:{delegate:false,revoke:false,refresh:false},submission:{id:source.submission_id,version:1,state:'task_known',taskID:source.task_id,providerStatus:'waiting_input'}};
 await act(async()=>{root=createRoot(host);root.render(<ApiViewProvider session={session}><WorkBuddyCloudTaskPanel model={publicModel} onDelegate={async()=>({kind:'confirmed'})} onRevoke={async()=>({kind:'confirmed'})} onRefresh={async()=>({kind:'confirmed'})} onReload={async()=>{}}/><WorkBuddyCloudCollaboration source={source}/></ApiViewProvider>);});await settle();
 fail='post';await click('load');expect(host.querySelector('.cloud-collaboration-entries')?.textContent).toContain('source-protected-entry-1');
 fail='';await click('read');fail='metadata';await click('read');expect(host.textContent).not.toContain('source-protected-entry-1');expect(host.querySelector('.workbuddy-cloud-task')?.textContent).toContain('Public original cloud task');
 fail='';await click('read');expect(host.querySelector('.cloud-collaboration-entries')?.textContent).toContain('source-protected-entry-1');fail='page';await click('more');expect(host.textContent).not.toContain('source-protected-entry-1');expect(host.querySelector('.workbuddy-cloud-task')?.textContent).toContain('waiting_input');expect(host.querySelector('.cloud-collaboration-source')).toBeNull();
});

it('does not let a late page from another source populate the current session scope',async()=>{
 let release!:(response:Response)=>void;const held=new Promise<Response>(resolve=>{release=resolve;});const items:CloudHistoryEntry[]=[{sequence:1,kind:'user_text',text:'old protected entry'}],old=makeLoad('old-pending',items);saved(old,items);view={source:{...source},version:2,latest_load:old,last_loaded:old,allowed_actions:[]};const oldPage=pageFor(old);
 transport(async(path)=>path.startsWith('/workbuddy-cloud-history-loads/old-pending?')?held:undefined);await render();
 source={...initialSource(),task_id:'different-task',request_sha256:'c'.repeat(64)};view={source:{...source},version:0,allowed_actions:[]};await render();await act(async()=>release(Response.json(oldPage)));await settle();expect(host.textContent).not.toContain('old protected entry');expect(host.textContent).toContain('No session load has been recorded');
});

it('preserves saved text and its time on a transient read failure and blocks a new external load',async()=>{
 const items:CloudHistoryEntry[]=[{sequence:1,kind:'assistant_text',text:'retained original observation'}],loaded=makeLoad('retained',items);saved(loaded,items);view={source:{...source},version:2,latest_load:loaded,last_loaded:loaded,allowed_actions:['load_history']};let fail=false;transport(async(path)=>fail&&path.includes('/collaboration?')?Response.json({error:{code:'unavailable',message:'read failed'}},{status:503}):undefined);
 await render();fail=true;await click('read');expect(host.textContent).toContain('retained original observation');expect(host.textContent).toContain('Existing observations retain their original content and time');expect(host.querySelector('time[datetime="2026-10-31T00:00:05Z"]')).not.toBeNull();expect(host.querySelector<HTMLButtonElement>('[data-collaboration-action="load"]')?.disabled).toBe(true);expect(calls.filter(call=>call.body)).toHaveLength(0);
});

it('distinguishes an acknowledged zero-entry load from a session never loaded or a pending-request inventory',async()=>{
 const loaded=makeLoad('loaded-zero');saved(loaded);view={source:{...source},version:2,latest_load:loaded,last_loaded:loaded,allowed_actions:[]};transport();await render();
 expect(host.textContent).toContain('This load was confirmed without saved displayable entries');expect(host.textContent).not.toContain('No session load has been recorded');expect(host.textContent).toContain('does not establish that no requests are pending');expect(host.textContent).toContain('Coverage is unverified');expect(calls.filter(call=>call.body)).toHaveLength(0);
});

it('uses the server overdue fact without expiring a loading record from the browser clock or reconnecting',async()=>{
 const loading=makeLoad('loading-original',[],{version:1,state:'loading',started_at:'2020-01-01T00:00:00Z',deadline_at:'2020-01-01T00:00:30Z',finished_at:undefined,overdue:false});saved(loading);view={source:{...source},version:1,latest_load:loading,allowed_actions:[]};transport();await render();expect(host.textContent).toContain('This load is in progress');expect(host.textContent).not.toContain('Deadline passed; outcome needs checking');
 const overdue={...loading,overdue:true};loads.set(overdue.id,overdue);view={...view,latest_load:overdue};await render(session,1);expect(host.textContent).toContain('Deadline passed; outcome needs checking');expect(calls.filter(call=>call.body)).toHaveLength(0);expect(host.querySelector('[data-collaboration-action="load"]')).toBeNull();
});
