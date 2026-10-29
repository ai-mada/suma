// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import type {Session} from './api';
import {setLocale} from './i18n';
import {DeveloperClientConnections} from './DeveloperClientConnections';
import {developerClientArtifact} from './developer-client-connections-model';
import type {DeveloperConnectionMetadata} from './developer-connection-model';

const project='project-original';
const session={user:{id:'user-original'},organization:{id:'org-original'}} as Session;
const metadata=(id=project):DeveloperConnectionMetadata=>({project_id:id,endpoint_path:'/api/v1/mcp/projects/'+id,endpoint_url:'https://suma.example/api/v1/mcp/projects/'+id,endpoint_source:'configured',transport:'streamable_http',stateless:true,sdk_name:'github.com/modelcontextprotocol/go-sdk',sdk_version:'v1.8.0',token_environment:'SUMA_MCP_TOKEN',expected_user_id:'user-original',expected_organization_id:'org-original',client_connection_verified:false,tools:[]});
let root:Root,host:HTMLDivElement,reads:string[],respond:(url:string)=>Promise<Response>,clipboard:PropertyDescriptor|undefined,createURL:PropertyDescriptor|undefined,revokeURL:PropertyDescriptor|undefined;
const json=(body:unknown,status=200)=>new Response(JSON.stringify(body),{status,headers:{'Content-Type':'application/json'}});
beforeEach(async()=>{await setLocale('en-US',{persist:false});host=document.createElement('div');document.body.append(host);root=createRoot(host);reads=[];respond=async()=>json(metadata());clipboard=Object.getOwnPropertyDescriptor(navigator,'clipboard');createURL=Object.getOwnPropertyDescriptor(URL,'createObjectURL');revokeURL=Object.getOwnPropertyDescriptor(URL,'revokeObjectURL');vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.stubGlobal('fetch',vi.fn(async(input:RequestInfo|URL)=>{const url=String(input);reads.push(url);return respond(url);}));Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:vi.fn(async()=>{})}});});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();for(const [target,key,descriptor] of [[navigator,'clipboard',clipboard],[URL,'createObjectURL',createURL],[URL,'revokeObjectURL',revokeURL]] as const){if(descriptor)Object.defineProperty(target,key,descriptor);else Reflect.deleteProperty(target,key);}vi.unstubAllGlobals();vi.restoreAllMocks();});
async function render(id=project,current=session){await act(async()=>root.render(<ApiViewProvider session={current}><DeveloperClientConnections project={id}/></ApiViewProvider>));}
async function open(){await act(async()=>{const detail=host.querySelector<HTMLDetailsElement>('[data-developer-clients]')!;detail.open=true;detail.dispatchEvent(new Event('toggle'));});}
async function choose(value:string){await act(async()=>{const select=host.querySelector('select')!;Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype,'value')!.set!.call(select,value);select.dispatchEvent(new Event('change',{bubbles:true}));});}

it('exports each documented environment syntax with a project key and rejects unsafe or mismatched endpoints',()=>{
 const item=metadata(),expected={'cursor':'Bearer ${env:SUMA_MCP_TOKEN}','claude-code':'Bearer ${SUMA_MCP_TOKEN}','opencode':'Bearer {env:SUMA_MCP_TOKEN}'};
 for(const client of ['codex','cursor','claude-code','opencode'] as const){const artifact=developerClientArtifact(client,item,'en-US')!;expect(artifact.content).toContain('suma_project-original');expect(artifact.content).toContain('user-original');expect(artifact.content).toContain('org-original');expect(artifact.content).toContain('SUMA_MCP_TOKEN');if(client!=='codex')expect(artifact.content).toContain(expected[client]);}
 expect(JSON.parse(developerClientArtifact('opencode',item,'en-US')!.content).mcp['suma_project-original'].oauth).toBe(false);
 for(const endpoint of ['http://suma.example'+item.endpoint_path,'https://user:secret@suma.example'+item.endpoint_path,item.endpoint_url+'?token=secret','https://suma.example/api/v1/mcp/projects/other','https://suma.example/a/../'+item.endpoint_path.slice(1)])expect(developerClientArtifact('cursor',{...item,endpoint_url:endpoint},'en-US')).toBeUndefined();
 expect(developerClientArtifact('cursor',{...item,endpoint_url:'http://127.0.0.1:4310'+item.endpoint_path,endpoint_source:'listener_loopback'},'zh-CN')).toBeDefined();
 expect(developerClientArtifact('cursor',{...item,endpoint_url:'https://suma.example:443'+item.endpoint_path},'en-US')).toBeDefined();
 for(const client of ['qoder','zed','workbuddy','harness'] as const)expect(developerClientArtifact(client,item,'en-US')).toBeUndefined();
});

it('loads only when opened, exports no credential and keeps guide-only clients honest in both languages',async()=>{
 await render();expect(reads).toHaveLength(0);await open();expect(reads).toHaveLength(1);expect(host.textContent).toContain('Connection unverified');
 await choose('cursor');expect(host.querySelector('[data-client-config]')?.textContent).toContain('Bearer ${env:SUMA_MCP_TOKEN}');expect(host.textContent).toContain('.cursor/mcp.json');
 await choose('qoder');expect(host.querySelector('[data-client-copy]')).toBeNull();expect(host.querySelector('[data-client-config]')).toBeNull();expect(host.textContent).toContain('Manual setup guide');
 await act(async()=>setLocale('zh-CN',{persist:false}));expect(host.textContent).toContain('手动接入指南');expect(reads).toHaveLength(1);expect(host.querySelector('a[href^="https://docs.qoder.com/"]')).not.toBeNull();
});

it('provides selectable manual fallback on copy failure and downloads only the reviewed secret-free fragment',async()=>{
 Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:vi.fn(async()=>{throw Error('denied');})}});await render();await open();await choose('claude-code');
 await act(async()=>host.querySelector<HTMLButtonElement>('[data-client-copy]')!.click());const fallback=host.querySelector('textarea')!;expect(fallback.readOnly).toBe(true);expect(fallback.value).toBe(host.querySelector('[data-client-config]')!.textContent);expect(fallback.value).toContain('Bearer ${SUMA_MCP_TOKEN}');expect(host.textContent).toContain('Select and copy');
 const create=vi.fn(()=> 'blob:suma-template'),revoke=vi.fn();Object.defineProperty(URL,'createObjectURL',{configurable:true,value:create});Object.defineProperty(URL,'revokeObjectURL',{configurable:true,value:revoke});const click=vi.spyOn(HTMLAnchorElement.prototype,'click').mockImplementation(function(this:HTMLAnchorElement){expect(this.download).toBe('suma-claude-code-project-original.json');});
 await act(async()=>host.querySelector<HTMLButtonElement>('[data-client-download]')!.click());expect(create).toHaveBeenCalledOnce();expect(click).toHaveBeenCalledOnce();expect(reads).toHaveLength(1);
});

it('drops old project and account exports, ignores late reads and fails closed on view identity mismatch',async()=>{
 let complete!:(value:Response)=>void;respond=async url=>url.includes('project-original')?new Promise(resolve=>{complete=resolve;}):json(metadata('project-next'));
 await render();await open();await render('project-next');expect(host.querySelector('[data-client-config]')?.textContent).toContain('project-next');await act(async()=>complete(json(metadata())));expect(host.querySelector('[data-client-config]')?.textContent).not.toContain('project-original');
 await render('project-next',{...session,user:{...session.user,id:'user-next'}});expect(host.querySelector('[data-client-config]')).toBeNull();expect(host.textContent).toContain('Connection details do not match');expect(host.querySelector('[data-client-copy]')).toBeNull();
});
