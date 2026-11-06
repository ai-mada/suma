// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {beforeEach,afterEach,it,expect,vi} from 'vitest';
import {ToolWorkspace} from './ToolWorkspace';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import type {AdapterDescriptor} from './AdapterConnections';
const session={user:{id:'user-a',name:'Owner',email:'owner@example.test'},organization:{id:'org-a',name:'Org'},projects:[]};
const row:AdapterDescriptor={id:'codex-app-server',product:'Codex app-server',entrypoint_kind:'app_server',direction:'suma_to_tool',adapter:'codex',protocol:'codex-app-server-v1',implementation:'implemented',setup_kind:'runner_profile',capabilities:[],source_refs:[],limitations:[]};
const cloud:AdapterDescriptor={id:'workbuddy-cloud',product:'WorkBuddy',entrypoint_kind:'cloud_api',direction:'suma_to_tool',implementation:'implemented',setup_kind:'manual_guide',capabilities:[],source_refs:['contracts/workbuddy-cloud-runner.md'],limitations:['cloud_fixed_delegation_required']};
const clients:AdapterDescriptor[]=[{...row,id:'qoder-client',product:'Qoder',direction:'tool_to_suma',setup_kind:'manual_guide',implementation:'guide_only'},{...row,id:'codex-client',product:'Codex client',direction:'tool_to_suma',setup_kind:'client_export'}];
const jsonlProfile={command_id:'jsonl-only',adapter:'codex',protocol:'codex-jsonl-v1',profile_sha256:'a'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'1.0',read_only:false};
let host:HTMLDivElement,root:Root,writes:string[];
beforeEach(async()=>{await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);writes=[];vi.stubGlobal('fetch',vi.fn(async(url:string,init:RequestInit)=>{const request=new URL(String(url),location.href),path=request.pathname.replace('/api/v1','');if(init.method!=='GET'){writes.push(path);return new Response('{}',{status:400});}const data=path==='/adapter-catalog'?{project_id:'p',as_of:'2026-10-29T00:00:00Z',items:[row,cloud,...clients,{...row,id:'harness-guide',product:'Harness guide',implementation:'guide_only',setup_kind:'manual_guide'}]}:path==='/release-workers'?{items:[{id:'w',worker_id:'runner',status:'active',version:1}]}:path==='/tool-discovery'?{project_id:'p',worker_id:'runner',state:'fresh',as_of:'2026-10-29T00:00:10Z',observed_at:'2026-10-29T00:00:00Z',expires_at:'2026-10-29T00:01:00Z',items:[{profile:jsonlProfile,available:true,reason:''}]}:path==='/developer-connection'?{project_id:'p',endpoint_path:'/api/v1/mcp/projects/p',endpoint_url:'https://suma.example/api/v1/mcp/projects/p',endpoint_source:'configured',transport:'streamable_http',stateless:true,sdk_name:'github.com/modelcontextprotocol/go-sdk',sdk_version:'v1.8.0',token_environment:'SUMA_MCP_TOKEN',expected_user_id:'user-a',expected_organization_id:'org-a',client_connection_verified:false,tools:[]}:path==='/workbuddy-connections'?{project_id:request.searchParams.get('project_id'),configured:true,available_scopes:['user.task.readable','user.task.invokable'],can_authorize:true,items:[],limit:20,next_cursor:''}:{items:[],total:0,as_of:'2026-10-29T00:00:00Z'};return new Response(JSON.stringify(data),{status:200});}));});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();});
async function render(openCatalog=true){await act(async()=>root.render(<ApiViewProvider session={session}><ToolWorkspace project="p" session={session} selected="" onSelect={()=>{}} refresh={0} panel=""/></ApiViewProvider>));if(openCatalog){await click('[data-tool-workspace-more-toggle]');await click('.adapter-catalog-disclosure > summary');}}
async function click(selector:string){await act(async()=>document.querySelector<HTMLButtonElement>(selector)!.click());}
async function choose(id:string){await click('[data-adapter-id="'+id+'"]');await click('.adapter-connection-next');}
function workBuddyPanel(){
 const id=host.querySelector('.workbuddy-disclosure')?.getAttribute('aria-controls');
 return id?document.getElementById(id):null;
}
it('keeps the catalog closed by default and preserves the selected protocol through collapse and reopen',async()=>{await render(false);
 expect(host.querySelector<HTMLDetailsElement>('[data-tool-workspace-more]')?.open).toBe(false);
 expect(host.querySelector('.adapter-catalog-disclosure')).toBeNull();
 await click('[data-tool-workspace-more-toggle]');
 const catalog=host.querySelector<HTMLDetailsElement>('.adapter-catalog-disclosure')!;expect(catalog.open).toBe(false);expect(catalog.querySelector('summary')?.textContent).toContain('Browse catalog');expect(document.querySelector('.tool-columns')).not.toBeNull();await click('.adapter-catalog-disclosure > summary');expect(catalog.open).toBe(true);await click('[data-adapter-id="codex-app-server"]');await click('.adapter-catalog-disclosure > summary');expect(catalog.open).toBe(false);await click('.adapter-catalog-disclosure > summary');expect(catalog.open).toBe(true);await click('.adapter-connection-next');expect(document.querySelector<HTMLSelectElement>('[data-quick-tool]')?.value).toBe('codex-app-server');expect(writes).toHaveLength(0);});
it('keeps the selected app-server protocol through quick and advanced setup and rejects otherwise valid JSONL inventory',async()=>{
 await render();
 await choose('codex-app-server');
 expect(document.querySelector<HTMLSelectElement>('[data-quick-tool]')?.value).toBe('codex-app-server');
 expect(document.querySelector('[role=dialog]')?.textContent).toContain('This node has not reported this tool entrypoint');
 expect(document.querySelector<HTMLButtonElement>('[data-quick-register]')?.disabled).toBe(true);
 await click('[data-quick-advanced]');
 expect(document.querySelector('[role=dialog]')).toBeNull();
 expect(host.querySelector<HTMLDetailsElement>('.tool-cli-setup')?.open).toBe(true);
 expect(host.querySelector<HTMLSelectElement>('[data-tool-setup-preset]')?.value).toBe('codex-app-server');
 expect(host.querySelector('[data-codex-app-server-setup-guide]')).not.toBeNull();
 expect(writes).toHaveLength(0);
});
it('opens the selected manual client and then a different export client without showing an unrelated execution template',async()=>{await render();await click('[data-adapter-direction="tool_to_suma"]');await choose('qoder-client');expect((document.querySelector('.developer-clients-chooser select') as HTMLSelectElement).value).toBe('qoder');expect(document.querySelector('[data-client-copy]')).toBeNull();expect(document.querySelector('.tool-cli-setup')).toBeNull();await choose('codex-client');expect((document.querySelector('.developer-clients-chooser select') as HTMLSelectElement).value).toBe('codex');expect(document.querySelector('[data-client-config]')?.textContent).toContain('mcp_servers');expect(writes).toHaveLength(0);});
it('routes the limited Harness manual guide to its existing explicit preset',async()=>{await render();await choose('harness-guide');expect((document.querySelector('.tool-cli-setup-select select') as HTMLSelectElement).value).toBe('harness');expect(document.querySelector('.tool-cli-setup pre')?.textContent).toContain('dsh-multica-jsonl-v1');expect(writes).toHaveLength(0);});
it('opens the current project cloud account panel without remounting it or bypassing draft protection',async()=>{
 vi.stubGlobal('requestAnimationFrame',(callback:FrameRequestCallback)=>{callback(0);return 1;});vi.stubGlobal('cancelAnimationFrame',()=>{});
 await render();const block=(event:Event)=>event.preventDefault();window.addEventListener('suma:navigate',block);
 await choose('workbuddy-cloud');expect(host.querySelector('.workbuddy-disclosure')?.getAttribute('aria-expanded')).toBe('false');window.removeEventListener('suma:navigate',block);
 await click('.adapter-connection-next');expect(host.querySelector('.workbuddy-disclosure')?.getAttribute('aria-expanded')).toBe('true');
 expect(vi.mocked(fetch).mock.calls.some(([url])=>String(url).includes('/workbuddy-connections?project_id=p&'))).toBe(true);
 const accounts=workBuddyPanel();expect(accounts).not.toBeNull();const reads=vi.mocked(fetch).mock.calls.length;
 await click('.adapter-connection-next');expect(workBuddyPanel()).toBe(accounts);expect(vi.mocked(fetch).mock.calls).toHaveLength(reads);expect(host.querySelector('[role=dialog]')).toBeNull();expect(writes).toHaveLength(0);
 await act(async()=>root.render(<ApiViewProvider session={session}><ToolWorkspace project="another-project" session={session} selected="" onSelect={()=>{}} refresh={0} panel=""/></ApiViewProvider>));
 expect(host.querySelector('.workbuddy-disclosure')?.getAttribute('aria-expanded')).toBe('false');expect(workBuddyPanel()).toBeNull();expect(writes).toHaveLength(0);
 await click('.workbuddy-disclosure');await click('.workbuddy-disclosure');
 await act(async()=>root.render(<ApiViewProvider session={session}><ToolWorkspace project="another-project" session={session} selected="" onSelect={()=>{}} refresh={0} panel="workbuddy"/></ApiViewProvider>));
 expect(host.querySelector('.workbuddy-disclosure')?.getAttribute('aria-expanded')).toBe('true');expect(vi.mocked(fetch).mock.calls.some(([url])=>String(url).includes('/workbuddy-connections?project_id=another-project&'))).toBe(true);expect(writes).toHaveLength(0);
});
