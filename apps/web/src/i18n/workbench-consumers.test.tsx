// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {App} from '../App';
import {setLocale,setServerCatalog} from './index';

const A='project-workbench-a',B='project-workbench-b';
const projects=[{id:A,title:'原始项目 <code>A</code>',description:'用户项目说明 <strong>原件</strong>',version:7},{id:B,title:'原始项目 B',description:'另一个项目原文',version:9}];
const task={id:'task-original',project_id:A,title:'用户任务 <code>原件</code>',status:'running',priority:'high',version:3};
let host:HTMLDivElement,root:Root|undefined,writes:{path:string;body:any}[],finishedOnly:boolean,missingVersion:boolean;
let capabilities:(project:string)=>Promise<unknown>,search:(body:any)=>Promise<unknown>;
let errorEndpoint:string|undefined,errorMethod:string|undefined;
let nonJsonEndpoint:string|undefined;
const parameterScope='原始范围 <code>raw</code>';
beforeEach(async()=>{
 localStorage.clear();sessionStorage.clear();await setLocale('en-US',{persist:false});
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);writes=[];finishedOnly=false;missingVersion=false;errorEndpoint=undefined;errorMethod=undefined;nonJsonEndpoint=undefined;
 setServerCatalog({version:'workbench-parameter-fixture',template_format:'named-dot-v1',default_locale:'zh-CN',supported_locales:['zh-CN','en-US'],messages:[{key:'fixture.workbench.scope',parameters:[{name:'scope',kind:'text'},{name:'limit',kind:'integer'}],texts:{'en-US':'Scope {{.scope}} has limit {{.limit}}.','zh-CN':'范围{{.scope}}的额度为{{.limit}}。'}}]});
 capabilities=async project=>({items:[{id:project,name:project===A?'原始模型 A':'原始模型 B',configured:true,available:false,verified:'configured'}]});
 search=async()=>({items:[{id:'source-original',type:'documents',title:'原始知识 <code>原件</code>',snippet:'原始段落 <strong>内容</strong>',source_ref:'source-original',revision:4,locator:{line:17}}],mode:'keyword',index_watermark:0});
 vi.stubGlobal('fetch',vi.fn(async(input:string,options?:RequestInit)=>{
  const url=new URL(String(input),location.href),path=url.pathname.replace(/^\/api\/v1/,''),project=url.searchParams.get('project_id')||A;
  const body=options?.body?JSON.parse(String(options.body)):undefined;if(body)writes.push({path,body});
  if(path===nonJsonEndpoint)return new Response('<html>upstream unavailable</html>',{status:503,headers:{'Content-Type':'text/html'}});
  if(path===errorEndpoint&&(!errorMethod||options?.method===errorMethod))return new Response(JSON.stringify({error:{code:'scope_blocked',message:'Scope '+parameterScope+' has limit 17.',message_key:'fixture.workbench.scope',message_params:{scope:parameterScope,limit:17},locale:'en-US',localized:true}}),{status:403,headers:{'Content-Type':'application/json'}});
  let data:unknown;
  if(path==='/auth/session')data={user:{id:'a'.repeat(32),name:'原始负责人',email:'fixture@example.test',role:'owner'},organization:{id:'b'.repeat(32),name:'原始组织'},projects:projects.map(p=>missingVersion?{...p,version:undefined}:p)};
  else if(path==='/account/preferences')data={user_id:'a'.repeat(32),locale:'',version:0,updated_at:''};
  else if(path==='/organization/preferences')data={organization_id:'b'.repeat(32),locale:'',version:0,updated_at:''};
  else if(path==='/overview')data={project:projects.find(p=>p.id===project),counts:{requirements:2,tasks:1000,runs:1,issues:0},delivery:{scope:'whole_project',total_tasks:1000,scope_tasks:1000,accepted_tasks:340,cancelled_tasks:0,running_tasks:1,waiting_tasks:0,review_tasks:finishedOnly?0:1,blocked_tasks:0,reconciling_tasks:0},attention:{review:{kind:'review',items:finishedOnly?[]:[{...task,status:'review',owner_id:'',owner_name:'',run_id:'',run_state:'',updated_at:'2026-10-29T00:00:00Z',due_date:''}],total:finishedOnly?0:1,next_cursor:'',as_of:'2026-10-29T00:00:00Z',date_basis:'UTC'}},activity:[{id:'activity-original',title:'用户活动原文 <code>操作</code>',actor_name:'原始成员',at:'2026-10-24T00:00:00Z'},{id:'conversation-original',kind:'task_conversation.posted',title:'Task conversation message',created_at:'2026-10-29T00:00:00Z'}]};
  else if(path==='/tasks')data={items:[{...task,status:finishedOnly?'accepted':'running'}],total:1};
  else if(path==='/runs')data={items:[{id:'run-original',workflow_name:'用户流程 <strong>原件</strong>',workflow_version:4,version:8,state:'succeeded',created_at:'2026-10-24T00:00:00Z'}],total:1};
  else if(path==='/capabilities')data=await capabilities(project);
  else if(path==='/search')data=await search(body);
  else if(['/members','/organizations','/requirements','/teams'].includes(path))data={items:[],total:0};
  else throw Error('Unexpected workbench fixture '+path);
  return new Response(JSON.stringify(data),{headers:{'Content-Type':'application/json'}});
 }));
});
afterEach(async()=>{if(root)await act(async()=>root?.unmount());root=undefined;host.remove();await setLocale('zh-CN',{persist:false});vi.unstubAllGlobals();vi.restoreAllMocks();});
async function settle(){for(let n=0;n<8;n++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function render(view:string,sub=''){history.replaceState(null,'','/?'+new URLSearchParams({view,sub,project:A}));await act(async()=>{root=createRoot(host);root.render(<App/>);});await settle();}
async function language(value:'en-US'|'zh-CN'){await act(async()=>{await setLocale(value,{persist:false});});await settle();}
async function chooseProject(project:string){const select=host.querySelector<HTMLSelectElement>('select[aria-label="Current project"]')!;expect(select).not.toBeNull();await act(async()=>{select.value=project;select.dispatchEvent(new Event('change',{bubbles:true}));});await settle();}
async function submitSearch(value:string){const input=host.querySelector<HTMLInputElement>('.knowledge-search input')!;await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(input,value);input.dispatchEvent(new Event('input',{bubbles:true}));});await act(async()=>{input.form!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true}));});await settle();}

it('translates overview facts and actions without changing user content or refetching on language changes',async()=>{
 await render('overview');expect(host.textContent).toContain('Task delivery');expect(host.textContent).toContain('Task discussion message recorded');
 expect(host.textContent).toContain(projects[0].description);expect(host.textContent).toContain(task.title);
 expect(host.querySelector('[data-overview-delivery]')?.textContent).toContain('1,000');expect(host.textContent).toContain('用户活动原文 <code>操作</code>');
 const requests=vi.mocked(fetch).mock.calls.length;await language('zh-CN');expect(host.textContent).toContain('任务交付');expect(host.textContent).toContain('已记录任务讨论消息');expect(host.textContent).not.toContain('PROJECT HOME');expect(host.querySelector('.po-header h1')?.textContent).toBe(projects[0].title);expect(host.textContent).toContain(task.title);expect(vi.mocked(fetch).mock.calls).toHaveLength(requests);expect(writes).toHaveLength(0);
});
it('keeps an empty review queue distinct from the full-project delivery result',async()=>{
 finishedOnly=true;await render('overview');expect(host.textContent).toContain('No tasks pending review');expect(host.textContent).toContain('This is the current project queue, not an overall delivery result.');expect(host.querySelector('[data-overview-delivery]')?.textContent).toContain('1,000');expect(host.querySelector('[role=progressbar]')?.getAttribute('aria-valuenow')).toBe('34');
});
it('uses complete execution summary labels and keeps raw workflow titles and request identity on locale changes',async()=>{
 await render('development','runs');expect(host.textContent).toContain('Loaded executions: 1 of 1');expect(host.textContent).toContain('Nodes and output are read separately in run details.');expect(host.textContent).toContain('用户流程 <strong>原件</strong>');
 const requests=vi.mocked(fetch).mock.calls.length;await language('zh-CN');expect(host.textContent).toContain('已加载执行：1 / 1');expect(location.search).toContain('sub=runs');expect(vi.mocked(fetch).mock.calls).toHaveLength(requests);expect(writes).toHaveLength(0);
});
it('keeps false and unknown capability observations separate and does not promote a nonboolean declaration to verified',async()=>{
 await render('settings','capabilities');expect(host.textContent).toContain('Configuration, availability, and verification are separate facts.');
 const verified=host.querySelector('[data-capability-state="verified"]');expect(verified?.textContent).toContain('Not reported');expect(verified?.querySelector('.yes')).toBeNull();
 expect(host.querySelector('[data-capability-state="available"]')?.textContent).toContain('No');
 const requests=vi.mocked(fetch).mock.calls.length;await language('zh-CN');expect(host.textContent).toContain('原始模型 A');expect(host.querySelector('[data-capability-state="verified"]')?.textContent).toContain('未上报');expect(vi.mocked(fetch).mock.calls).toHaveLength(requests);
});
it('discards a late capability response from the old project',async()=>{
 let resolveOld!:(value:unknown)=>void;capabilities=project=>project===A?new Promise(resolve=>{resolveOld=resolve;}):Promise.resolve({items:[{name:'当前B模型',verified:false}]});
 await render('settings','capabilities');await chooseProject(B);expect(host.textContent).toContain('当前B模型');
 await act(async()=>{resolveOld({items:[{name:'旧A模型',verified:true}]});});await settle();expect(host.textContent).not.toContain('旧A模型');expect(host.textContent).toContain('当前B模型');
});
it('shows project identity in both languages without fabricating a version when missing',async()=>{
 missingVersion=true;await render('settings','project');expect(host.textContent).toContain('Project identity and context');expect(host.textContent).toContain(projects[0].title);const versions=[...host.querySelectorAll('dt')].find(e=>e.textContent==='Record version')?.nextElementSibling;expect(versions?.textContent).toBe('Not reported');await language('zh-CN');expect(host.textContent).toContain('当前项目的持久身份与上下文');
});
it('keeps the original search query, raw results and an explicitly reported zero watermark while switching languages',async()=>{
 await render('knowledge','search');expect(host.querySelector('input[placeholder="Search requirements, tasks, and documents…"]')).not.toBeNull();await submitSearch('原始查询 <code>query</code>');
 expect(writes).toEqual([{path:'/search',body:{project_id:A,query:'原始查询 <code>query</code>',types:[]}}]);
 expect(host.textContent).toContain('Index watermark: 0');expect(host.textContent).toContain('原始知识 <code>原件</code>');expect(host.querySelector('.type-chip')?.textContent).toBe('Documents');expect(host.textContent).toContain('Results for: 原始查询 <code>query</code>');const requests=vi.mocked(fetch).mock.calls.length;
 await language('zh-CN');expect(host.textContent).toContain('索引水位：0');expect(host.querySelector('.type-chip')?.textContent).toBe('文档');expect(host.textContent).toContain('原始知识 <code>原件</code>');expect((host.querySelector('.knowledge-search input') as HTMLInputElement).value).toBe('原始查询 <code>query</code>');expect(vi.mocked(fetch).mock.calls).toHaveLength(requests);
});
it('does not present a late knowledge response under another project',async()=>{
 let resolveOld!:(value:unknown)=>void;search=()=>new Promise(resolve=>{resolveOld=resolve;});await render('knowledge','search');await submitSearch('原始查询');await chooseProject(B);
 await act(async()=>{resolveOld({items:[{id:'old',title:'旧项目私有结果',snippet:'原始片段'}],mode:'keyword',index_watermark:1});});await settle();expect(host.textContent).not.toContain('旧项目私有结果');expect(location.search).toContain(B);
});
it('relocalizes a parameterized run-list failure without reloading the original request',async()=>{
 errorEndpoint='/runs';await render('development','runs');expect(host.textContent).toContain('Scope '+parameterScope+' has limit 17.');const requests=vi.mocked(fetch).mock.calls.length;
 await language('zh-CN');expect(host.textContent).toContain('范围'+parameterScope+'的额度为17。');expect(vi.mocked(fetch).mock.calls).toHaveLength(requests);expect(writes).toHaveLength(0);
});
it('retains typed write failures and the unchanged requirement draft while switching languages',async()=>{
 await render('requirements');const create=[...host.querySelectorAll<HTMLButtonElement>('button')].find(button=>button.textContent?.trim()==='New requirement')!;expect(create).toBeDefined();await act(async()=>create.click());await settle();
 const title=document.querySelector<HTMLInputElement>('[role=dialog] input[name=title]')!;expect(title).not.toBeNull();await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(title,'原始需求 <code>draft</code>');title.dispatchEvent(new Event('input',{bubbles:true}));});
 errorEndpoint='/requirements';errorMethod='POST';await act(async()=>{title.form!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true}));});await settle();expect(document.querySelector('[role=dialog]')?.textContent).toContain('Scope '+parameterScope+' has limit 17.');const requests=vi.mocked(fetch).mock.calls.length,request=JSON.stringify(writes);
 await language('zh-CN');expect(document.querySelector('[role=dialog]')?.textContent).toContain('范围'+parameterScope+'的额度为17。');expect(title.value).toBe('原始需求 <code>draft</code>');expect(vi.mocked(fetch).mock.calls).toHaveLength(requests);expect(JSON.stringify(writes)).toBe(request);
});
it('relocalizes local HTTP fallback diagnostics with their original status without rereading',async()=>{
 nonJsonEndpoint='/runs';await render('development','runs');expect(host.textContent).toContain('Request failed (503); its error receipt was incomplete.');const requests=vi.mocked(fetch).mock.calls.length;
 await language('zh-CN');expect(host.textContent).toContain('请求失败（503），错误回执未完整读取。');expect(vi.mocked(fetch).mock.calls).toHaveLength(requests);expect(writes).toHaveLength(0);
});
it('keeps precise parameterized team-write refusals rather than replacing them with generic prose',async()=>{
 await render('settings','teams');const create=host.querySelector<HTMLButtonElement>('button[aria-label="Create team"]')!;expect(create).not.toBeNull();await act(async()=>create.click());await settle();
 const name=document.querySelector<HTMLInputElement>('[role=dialog] input[name=name]')!;expect(name).not.toBeNull();await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(name,'原始团队 <code>draft</code>');name.dispatchEvent(new Event('input',{bubbles:true}));});
 errorEndpoint='/teams';errorMethod='POST';await act(async()=>{name.form!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true}));});await settle();expect(document.querySelector('[role=dialog]')?.textContent).toContain('Scope '+parameterScope+' has limit 17.');const requests=vi.mocked(fetch).mock.calls.length,body=JSON.stringify(writes);
 await language('zh-CN');expect(document.querySelector('[role=dialog]')?.textContent).toContain('范围'+parameterScope+'的额度为17。');expect(name.value).toBe('原始团队 <code>draft</code>');expect(vi.mocked(fetch).mock.calls).toHaveLength(requests);expect(JSON.stringify(writes)).toBe(body);
});
