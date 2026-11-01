// @vitest-environment jsdom
import {act,type ReactNode} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {KnowledgeSearch} from './project-views/KnowledgeSearch';
import {TaskDispatchDialog} from './TaskDispatch';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import type {Session,RecordItem,Workflow} from './api';

const session:Session={user:{id:'user-a',name:'Owner',email:'owner@example.test',role:'owner'},organization:{id:'org-a',name:'Organization'},projects:[]};
const pin={id:'knowledge-exact',revision:3,sha256:'d'.repeat(64)},project='project-a';
const hit={reference:pin,project_id:project,kind:'memory',title:'Published deployment memory',summary:'Reviewed instructions',snippet:'Restore the original deployment receipt',matched_field:'none',source_count:0,published_at:'2026-11-02T00:00:00Z'};
const snapshot={revision:3,based_on_revision:2,state:'published',kind:'memory',title:hit.title,summary:hit.summary,content:'EXACT REVIEWED PUBLICATION BODY',sha256:pin.sha256,sources:[],method:'manual',created_by:'user-a',created_at:hit.published_at};
const detail={id:pin.id,project_id:project,version:4,kind:'memory',state:'draft',title:'PRIVATE NEW DRAFT',summary:'Unreviewed draft summary',latest_revision:4,published_revision:3,published_sha256:pin.sha256,published_title:hit.title,published_summary:hit.summary,has_draft:true,published_eligible:true,eligibility:'current',created_at:hit.published_at,updated_at:hit.published_at,latest:{...snapshot,revision:4,based_on_revision:3,state:'draft',title:'PRIVATE NEW DRAFT',content:'UNREVIEWED DRAFT BODY',sha256:'e'.repeat(64)},published:snapshot};
const task={id:'task-a',project_id:project,title:'Restore the original deployment',version:4,description:'Use reviewed project knowledge'} as RecordItem;
const flow:Workflow={id:'flow-a',name:'Original workflow',version:3,nodes:[{id:'develop',name:'Develop',type:'agent'},{id:'independent-test',name:'Independent test',type:'test',command:['junit-test'],config:{candidate_from:'develop'}}],edges:[]};
const profile={id:'profile-a',project_id:project,worker_id:'worker-a',name:'Original tool',version:5,state:'enabled',available:true,profile:{command_id:'command-a',adapter:'codex',protocol:'codex-jsonl-v1',profile_sha256:'a'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'1.0',read_only:false}};
let root:Root|undefined,host:HTMLDivElement,requests:{path:string;method:string;body?:Record<string,unknown>}[];
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});sessionStorage.clear();requests=[];vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);
 vi.stubGlobal('fetch',vi.fn(async(input:string,init:RequestInit={})=>{
  const url=new URL(String(input),location.href),path=url.pathname.replace(/^\/api\/v1/,''),method=init.method||'GET',body=init.body?JSON.parse(String(init.body)):undefined;requests.push({path:path+url.search,method,...(body?{body}:{})});
  if(method!=='GET')return Response.json({error:{code:'fixture_preview_boundary',message:'Synthetic consumer verifies the exact original preview request only.'}},{status:409});
  if(path==='/workflows')return Response.json({items:[flow],total:1});
  if(path==='/tool-profiles')return Response.json({items:[profile],total:1});
  if(path==='/project-knowledge')return Response.json({items:[],total:0});
  if(path==='/project-knowledge/discovery')return Response.json({items:url.searchParams.get('q')==='no-match'?[]:[{...hit,matched_field:url.searchParams.get('q')?'content':'none'}],next_cursor:'',mode:'keyword',scope:'current_project_publications',scanned:1,scan_limit:200,exhausted:true});
  if(path==='/project-knowledge/'+pin.id)return Response.json(detail);
  if(path==='/project-knowledge/'+pin.id+'/revisions/3')return Response.json({project_id:project,knowledge_id:pin.id,snapshot});
  throw Error('Unexpected knowledge consumer route '+path);
 }));
});
afterEach(async()=>{if(root)await act(async()=>root!.unmount());root=undefined;host.remove();document.body.innerHTML='';vi.restoreAllMocks();vi.unstubAllGlobals();});
async function settle(){await act(async()=>{for(let i=0;i<36;i++)await Promise.resolve();});}
async function render(node:ReactNode){await act(async()=>{root??=createRoot(host);root.render(node);});await settle();}
async function click(selector:string){const button=document.querySelector<HTMLButtonElement>(selector);expect(button,selector).not.toBeNull();await act(async()=>button!.click());await settle();}
async function change(element:HTMLInputElement|HTMLSelectElement,value:string){await act(async()=>{Object.getOwnPropertyDescriptor(element instanceof HTMLSelectElement?HTMLSelectElement.prototype:HTMLInputElement.prototype,'value')!.set!.call(element,value);element.dispatchEvent(new Event(element instanceof HTMLSelectElement?'change':'input',{bubbles:true}));});await settle();}

it('T196 discovers and previews the exact publication in search without exposing its newer draft or retaining another caller content',async()=>{
 await render(<ApiViewProvider session={session}><KnowledgeSearch project={project}/></ApiViewProvider>);
 await click('[data-knowledge-search-mode="published"]');await click('[data-knowledge-discovery-preview="knowledge-exact"]');
 expect(host.textContent).toContain(snapshot.content);expect(host.textContent).not.toContain(detail.latest.content);expect(host.textContent).not.toContain(detail.latest.title);
 expect(document.querySelector('[data-knowledge-preview-choose]')).toBeNull();
 await click('[data-knowledge-preview-close]');expect(document.querySelector('[data-knowledge-discovery-preview="knowledge-exact"]')).not.toBeNull();
 const other={...session,user:{...session.user,id:'user-b'}};
 await render(<ApiViewProvider session={other}><KnowledgeSearch project={project}/></ApiViewProvider>);
 expect(host.textContent).not.toContain(snapshot.content);expect(document.querySelector('[data-knowledge-published-preview]')).toBeNull();expect(requests.every(row=>row.method==='GET')).toBe(true);
});

it('T196 keeps an explicitly chosen exact publication independent of discovery pages and sends it only in the original dispatch preview',async()=>{
 await render(<ApiViewProvider session={session}><TaskDispatchDialog project={project} session={session} task={task} planID="new" initialWorkflow={flow} onPlan={()=>{}} onClose={()=>{}} onRun={()=>{}}/></ApiViewProvider>);
 await change(document.querySelector<HTMLSelectElement>('[data-dispatch-manual-tool]')!,profile.id);
 await click('[data-knowledge-toggle]');await click('[data-knowledge-discovery-preview="knowledge-exact"]');
 expect(requests.filter(row=>row.method!=='GET')).toEqual([]);expect(document.querySelector('[data-knowledge-selected="knowledge-exact"]')).toBeNull();
 await click('[data-knowledge-preview-choose]');expect(document.querySelector('[data-knowledge-selected="knowledge-exact"]')).not.toBeNull();
 await change(document.querySelector<HTMLInputElement>('[data-knowledge-discovery-query]')!,'no-match');await click('[data-knowledge-discovery-search]');
 expect(document.querySelector('[data-knowledge-discovery-hit="knowledge-exact"]')).toBeNull();expect(document.querySelector('[data-knowledge-selected="knowledge-exact"]')).not.toBeNull();
 expect(document.body.textContent).not.toContain(detail.latest.content);expect(requests.filter(row=>row.method!=='GET')).toEqual([]);
 const submit=document.querySelector<HTMLButtonElement>('.tool-dispatch .dialog-actions button.primary')!;expect(submit.disabled).toBe(false);
 await act(async()=>document.querySelector<HTMLFormElement>('.tool-dispatch form.tool-form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));await settle();
 const writes=requests.filter(row=>row.method!=='GET');expect(writes).toHaveLength(1);expect(writes[0].path).toBe('/task-dispatch-plans');expect(writes[0].body?.knowledge).toEqual([pin]);expect(writes[0].body).not.toHaveProperty('content');expect(writes.some(row=>row.path.endsWith('/start'))).toBe(false);
});

it('T196 keeps dispatch access hidden when a task refresh succeeds but the same recovery batch observes a refused tool read',async()=>{
 const original=vi.mocked(fetch).getMockImplementation()!;let restoring=false,denyProfiles=true,holdTask=false,releaseTask:((response:Response)=>void)|undefined;
 vi.mocked(fetch).mockImplementation(async(input,init={})=>{
  const path=new URL(String(input),location.href).pathname.replace(/^\/api\/v1/,'');
  if(path==='/project-knowledge/discovery')return Response.json({error:{code:'refused',message:'Current project access changed'}},{status:403});
  if(path==='/tasks/'+task.id){if(holdTask)return new Promise<Response>(resolve=>{releaseTask=resolve;});return Response.json(task);}
  if(restoring&&path==='/tool-profiles'&&denyProfiles)return Response.json({error:{code:'refused',message:'Current tool access changed'}},{status:403});
  return original(input,init);
 });
 await render(<ApiViewProvider session={session}><TaskDispatchDialog project={project} session={session} task={task} planID="new" initialWorkflow={flow} onPlan={()=>{}} onClose={()=>{}} onRun={()=>{}}/></ApiViewProvider>);
 await click('[data-knowledge-toggle]');expect(document.querySelector('.tool-dispatch form.tool-form')).toBeNull();expect(document.body.textContent).not.toContain(task.title);
 const recheck=()=>[...document.querySelectorAll<HTMLButtonElement>('.tool-dispatch button')].find(button=>button.textContent==='Recheck project access')!;
 restoring=true;expect(recheck()).toBeDefined();await act(async()=>recheck().click());await settle();
 expect(document.querySelector('.tool-dispatch form.tool-form')).toBeNull();expect(document.body.textContent).not.toContain(task.title);
 denyProfiles=false;holdTask=true;await act(async()=>recheck().click());await settle();expect(recheck().disabled).toBe(true);
 releaseTask!(Response.json({error:{code:'refused',message:'A newer current access refusal'}},{status:403}));await settle();
 expect(document.querySelector('.tool-dispatch form.tool-form')).toBeNull();expect(recheck().disabled).toBe(false);
 holdTask=false;await act(async()=>recheck().click());await settle();expect(document.querySelector('.tool-dispatch form.tool-form')).not.toBeNull();expect(document.body.textContent).toContain(task.title);expect(requests.filter(row=>row.method!=='GET')).toEqual([]);
});
