// @vitest-environment jsdom
import {act,type ReactNode} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {Tokens} from './Tokens';
import {ApiViewProvider} from './api-context';
import {setLocale,setServerCatalog} from './i18n';
import type {Session} from './api';

const project='project-original';
const session:Session={user:{id:'user-original',name:'Original',email:'owner@example.test',role:'owner'},organization:{id:'org-original',name:'Original org'},projects:[]};
const metadata=(id=project)=>({project_id:id,endpoint_path:'/api/v1/mcp/projects/'+id,endpoint_url:'http://127.0.0.1:4310/api/v1/mcp/projects/'+id,endpoint_source:'listener_loopback',transport:'streamable_http',stateless:true,sdk_name:'github.com/modelcontextprotocol/go-sdk',sdk_version:'v1.8.0',token_environment:'SUMA_MCP_TOKEN',expected_user_id:'user-original',expected_organization_id:'org-original',client_connection_verified:false,tools:[{name:'project.overview',mcp_name:'suma_project_overview',description_key:'application_tool_project_overview',description:'读取当前授权项目的真实状态与活动',parameters:{type:'object',properties:{},additionalProperties:false},read_only:true,credential_scopes:['agent','api']},{name:'project.task_draft_create',mcp_name:'suma_project_task_draft_create',description_key:'application_tool_project_task_draft_create',description:'创建任务草稿',parameters:{type:'object',properties:{},additionalProperties:false},read_only:false,credential_scopes:['api']}]});
let host:HTMLDivElement,root:Root|undefined,reads:string[],writes:string[],copied:string[];
let fixture:(path:string,init?:RequestInit)=>Promise<Response>;
const json=(value:unknown,status=200)=>Response.json(value,{status});
beforeEach(async()=>{
 localStorage.clear();sessionStorage.clear();reads=[];writes=[];copied=[];await setLocale('en-US',{persist:false});
 setServerCatalog({version:'developer-connection-fixture',template_format:'named-dot-v1',default_locale:'zh-CN',supported_locales:['zh-CN','en-US'],messages:[{key:'application_tool_project_overview',parameters:[],texts:{'zh-CN':'读取当前授权项目的真实状态与活动','en-US':'Read current project state and activity'}}]});
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);
 fixture=async(path,init)=>path.startsWith('/developer-connection')?json(metadata(new URL(path,'http://fixture').searchParams.get('project_id')||project)):json({token:'one-time-secret-do-not-inject',expires_at:'2026-10-27T08:00:00Z',project_id:project,scope:JSON.parse(String(init?.body)).scope,label:'Original credential'});
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{const path=String(input).replace(/^\/api\/v1/,'');(init?.body?writes:reads).push(path);return fixture(path,init);}));
 Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:async(value:string)=>{copied.push(value);}}});
});
afterEach(async()=>{if(root)await act(async()=>root?.unmount());root=undefined;host.remove();await setLocale('zh-CN',{persist:false});vi.unstubAllGlobals();vi.restoreAllMocks();});
async function settle(){for(let n=0;n<4;n++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
function view(id=project,identity=session):ReactNode{return <ApiViewProvider session={identity}><Tokens project={id} projectName={'原始项目 '+id} admin/></ApiViewProvider>;}
async function render(node:ReactNode){await act(async()=>{root??=createRoot(host);root.render(node);});await settle();}
async function click(label:string){const button=[...host.querySelectorAll<HTMLButtonElement>('button')].find(item=>item.textContent?.trim()===label||item.getAttribute('aria-label')===label);expect(button,'button '+label).toBeDefined();await act(async()=>button!.click());await settle();}
async function value(input:HTMLInputElement|HTMLSelectElement,text:string){await act(async()=>{Object.getOwnPropertyDescriptor(input instanceof HTMLSelectElement?HTMLSelectElement.prototype:HTMLInputElement.prototype,'value')!.set!.call(input,text);input.dispatchEvent(new Event(input instanceof HTMLSelectElement?'change':'input',{bubbles:true}));});}

it('uses only the declared project endpoint and copies an env-based Codex template without the issued secret',async()=>{
 await render(view());expect(reads).toEqual(['/developer-connection?project_id=project-original']);expect(host.textContent).toContain('Developer connection');expect(host.textContent).toContain('Client connection unverified');
 const config=host.querySelector('pre[data-codex-config]')?.textContent;expect(config).toBe('[mcp_servers.suma]\nurl = "http://127.0.0.1:4310/api/v1/mcp/projects/project-original"\nbearer_token_env_var = "SUMA_MCP_TOKEN"\nhttp_headers = { "X-Suma-Expected-User" = "user-original", "X-Suma-Expected-Organization" = "org-original", "X-Suma-Language" = "en-US" }');
 expect(host.textContent).toContain('suma_project_task_draft_create');expect(host.textContent).toContain('Draft write');
 await value(host.querySelector<HTMLInputElement>('input[name="label"]')!,'Original credential');await value(host.querySelector<HTMLSelectElement>('select')!,'agent');await act(async()=>host.querySelector('form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));await settle();
 expect(host.querySelector('.token-result code')?.textContent).toBe('one-time-secret-do-not-inject');await click('Copy configuration');expect(copied).toEqual([config]);expect(copied[0]).not.toContain('one-time-secret');expect(localStorage.length).toBe(0);expect(sessionStorage.length).toBe(0);expect(writes).toEqual(['/auth/tokens']);
});
it('does not infer a public endpoint from this browser and keeps an unconfigured empty catalog explicit',async()=>{
 fixture=async()=>json({...metadata(),endpoint_url:'',endpoint_source:'unconfigured',tools:[]});await render(view());expect(host.textContent).toContain('Ask an administrator to configure SUMA_PUBLIC_URL');expect(host.textContent).toContain('/api/v1/mcp/projects/project-original');expect(host.textContent).toContain('No tools are declared');expect(host.querySelector('pre[data-codex-config]')).toBeNull();expect([...host.querySelectorAll('button')].some(button=>button.textContent==='Copy endpoint')).toBe(false);expect(copied).toEqual([]);
});
it('keeps the tool catalogue collapsed until its native summary is activated',async()=>{
 await render(view());const catalogue=host.querySelector<HTMLDetailsElement>('details.connection-tools');expect(catalogue).not.toBeNull();expect(catalogue!.open).toBe(false);expect(catalogue!.querySelector('summary')?.textContent).toBe('View tool catalogue (2)');await act(async()=>catalogue!.querySelector('summary')!.click());expect(catalogue!.open).toBe(true);await act(async()=>setLocale('zh-CN',{persist:false}));expect(catalogue!.querySelector('summary')?.textContent).toBe('查看工具目录（2）');expect(catalogue!.open).toBe(true);expect(reads).toHaveLength(1);
});
it('retranslates supported descriptions and the language header without rereading or changing raw identities',async()=>{
 await render(view());expect(host.textContent).toContain('Read current project state and activity');expect(host.textContent).toContain('创建任务草稿');await click('Copy configuration');await act(async()=>setLocale('zh-CN',{persist:false}));await settle();expect(host.textContent).toContain('开发者接入');expect(host.textContent).toContain('读取当前授权项目的真实状态与活动');expect(host.querySelector('pre[data-codex-config]')?.textContent).toContain('"X-Suma-Language" = "zh-CN"');expect(host.querySelector('pre[data-codex-config]')?.textContent).toContain('"X-Suma-Expected-User" = "user-original"');expect([...host.querySelectorAll('button')].some(button=>button.textContent==='复制配置')).toBe(true);expect(reads).toHaveLength(1);expect(copied).toHaveLength(1);
});
it.each([401,403])('clears prior facts after HTTP %i and retries only when requested',async(status)=>{
 let refused=false;fixture=async()=>refused?json({error:{code:'forbidden',message:'Original access denied'}},status):json(metadata());await render(view());refused=true;await click('Refresh connection details');expect(host.textContent).toContain('Original access denied');expect(host.querySelector('pre[data-codex-config]')).toBeNull();expect(host.textContent).not.toContain('suma_project_overview');expect(reads).toHaveLength(2);refused=false;await click('Reload');expect(host.querySelector('pre[data-codex-config]')).not.toBeNull();expect(reads).toHaveLength(3);
});
it('discards late metadata from the previous project instead of borrowing its connection or permissions',async()=>{
 let release!:(value:Response)=>void;fixture=path=>path.endsWith('project-original')?new Promise(resolve=>{release=resolve;}):Promise.resolve(json(metadata('project-next')));await render(view());expect(host.querySelector('pre[data-codex-config]')).toBeNull();await render(view('project-next'));expect(host.querySelector('pre[data-codex-config]')?.textContent).toContain('/projects/project-next');await act(async()=>release(json(metadata())));await settle();expect(host.querySelector('pre[data-codex-config]')?.textContent).not.toContain('/projects/project-original');
});
it('clears previous organization metadata and ignores a late response after the API view changes',async()=>{
 let release!:(value:Response)=>void,requests=0;fixture=async()=>++requests===2?new Promise(resolve=>{release=resolve;}):json(requests===1?metadata():{...metadata(),expected_organization_id:'org-next'});await render(view());await click('Refresh connection details');expect(host.querySelector('pre[data-codex-config]')).toBeNull();await render(view(project,{...session,organization:{id:'org-next',name:'Next org'}}));expect(host.querySelector('pre[data-codex-config]')?.textContent).toContain('org-next');await act(async()=>release(json(metadata())));await settle();expect(host.querySelector('pre[data-codex-config]')?.textContent).not.toContain('org-original');
});
it('reports clipboard failure without sending a tool invocation or storing configuration',async()=>{
 Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:async()=>{throw Error('unavailable');}}});await render(view());await click('Copy configuration');expect(host.textContent).toContain('Cannot access the clipboard');expect(writes).toEqual([]);expect(localStorage.length).toBe(0);expect(sessionStorage.length).toBe(0);
});
it('switches between Codex and Octop without connecting a client or copying an issued credential',async()=>{
 await render(view());const client=host.querySelector<HTMLSelectElement>('[data-connection-client]');expect(client).not.toBeNull();expect(client!.value).toBe('codex');
 const original=host.querySelector('pre[data-codex-config]')!.textContent;await value(client!,'octop');expect(host.querySelector('pre[data-codex-config]')).toBeNull();expect(host.querySelector('pre[data-octop-config]')).not.toBeNull();
 const config=host.querySelector('pre[data-octop-config]')!.textContent!;expect(config).toContain('/projects/project-original');expect(config).toContain('user-original');expect(config).toContain('org-original');expect(config).not.toContain('one-time-secret');expect(host.textContent).toContain('Client connection unverified');
 expect(JSON.parse(config)).toEqual({servers:{'suma_project-original':{transport:'streamable_http',url:'http://127.0.0.1:4310/api/v1/mcp/projects/project-original',headers:{Authorization:'Bearer REPLACE_WITH_PROJECT_API_TOKEN','X-Suma-Expected-User':'user-original','X-Suma-Expected-Organization':'org-original','X-Suma-Language':'en-US'},enabled:true,display_name:'suma',shared:false,default_open:false}}});expect(config).not.toContain('${');expect(config).not.toContain('SUMA_MCP_TOKEN');expect(host.textContent).toContain('Preserve existing servers');
 await click('Copy configuration');expect(copied).toEqual([config]);expect(writes).toEqual([]);expect(localStorage.length).toBe(0);expect(sessionStorage.length).toBe(0);
 await act(async()=>setLocale('zh-CN',{persist:false}));await settle();expect(host.textContent).toContain('接入客户端');expect(host.querySelector('pre[data-octop-config]')?.textContent).toContain('"X-Suma-Language": "zh-CN"');expect(reads).toHaveLength(1);await act(async()=>setLocale('en-US',{persist:false}));
 await value(client!,'codex');expect(host.querySelector('pre[data-codex-config]')?.textContent).toBe(original);expect(host.querySelector('pre[data-octop-config]')).toBeNull();expect(host.querySelector('.connection-copy-status')?.textContent).toBe('');expect(reads).toHaveLength(1);
});
it('does not report a late Codex clipboard completion as an Octop configuration copy',async()=>{
 let finish!:()=>void;Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:()=>new Promise<void>(resolve=>{finish=resolve;})}});await render(view());await click('Copy configuration');await value(host.querySelector<HTMLSelectElement>('[data-connection-client]')!,'octop');await act(async()=>finish());await settle();expect(host.querySelector('pre[data-octop-config]')).not.toBeNull();expect(host.querySelector('.connection-copy-status')?.textContent).toBe('');expect([...host.querySelectorAll('button')].some(button=>button.textContent==='Copied')).toBe(false);expect(writes).toEqual([]);
});
it('clears a one-time credential when the selected usage scope changes',async()=>{
 await render(view());await value(host.querySelector<HTMLInputElement>('input[name="label"]')!,'Original credential');await act(async()=>host.querySelector('form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));await settle();expect(host.querySelector('.token-result')).not.toBeNull();await value(host.querySelector<HTMLSelectElement>('select')!,'api');expect(host.querySelector('.token-result')).toBeNull();expect(writes).toHaveLength(1);
});
it('does not reveal a credential receipt that arrives after the original browser identity changed',async()=>{
 let release!:(value:Response)=>void;fixture=async(path)=>path==='/auth/tokens'?new Promise(resolve=>{release=resolve;}):json(metadata());await render(view());await value(host.querySelector<HTMLInputElement>('input[name="label"]')!,'Original credential');await act(async()=>host.querySelector('form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));await settle();await render(view(project,{...session,user:{...session.user,id:'user-next'}}));await act(async()=>release(json({token:'previous-identity-secret',expires_at:'2026-10-27T08:00:00Z',project_id:project,scope:'runner',label:'Original credential'})));await settle();expect(host.querySelector('.token-result')).toBeNull();expect(host.textContent).not.toContain('previous-identity-secret');expect(writes).toHaveLength(1);
});
