// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {Session} from './api';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import {KnowledgeIndexStatusPanel} from './KnowledgeIndexStatusPanel';

const source={kind:'project_knowledge',id:'knowledge-a',sequence:7,revision:3,sha256:'a'.repeat(64)};
const status={domain:'project_knowledge',as_of:'2026-11-02T00:00:00Z',configured:false,coverage:'outbox_delivery_only',pending:2,indexed:1,failed:1,total:4,current_publications:2,untracked_publications:1,latest_event:{event_id:'event-newer',source:{...source,sequence:9},action:'delete',state:'pending',created_at:'2026-11-02T00:00:00Z'},last_ack:{protocol:'suma-knowledge-index-v1',event_id:'event-older',tenant_id:'org-a',project_id:'project-a',source,action:'upsert',disposition:'obsolete',generation:'provider-generation-a',index_watermark:4,request_sha256:'b'.repeat(64)}};
let root:Root,host:HTMLDivElement,session:Session;
type Props=Parameters<typeof KnowledgeIndexStatusPanel>[0];
async function settle(){await act(async()=>{for(let i=0;i<20;i++)await Promise.resolve();});}
async function render(props:Partial<Props>={}){await act(async()=>root.render(<ApiViewProvider session={session}><KnowledgeIndexStatusPanel project="project-a" {...props}/></ApiViewProvider>));await settle();}
async function open(){const details=host.querySelector<HTMLDetailsElement>('[data-knowledge-index-status]');expect(details).not.toBeNull();await act(async()=>{details!.open=true;details!.dispatchEvent(new Event('toggle'));});await settle();}
async function refresh(){const button=host.querySelector<HTMLButtonElement>('[data-knowledge-index-status-refresh]');expect(button).not.toBeNull();await act(async()=>button!.click());await settle();}
beforeEach(async()=>{await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);session={user:{id:'user-a',name:'Owner',email:'owner@example.test'},organization:{id:'org-a',name:'Organization'},projects:[]};});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});
it('T197 reads only after explicit expansion, keeps stale delivery facts read-only and clears refusal or late foreign-scope results',async()=>{
 let code=200,value=status;const denied=vi.fn(),fetcher=vi.fn(async(_path:string,_init:RequestInit)=>Response.json(code===200?value:{error:{code:'unavailable',message:'Synthetic read failed'}},{status:code}));vi.stubGlobal('fetch',fetcher);
 await render({onAccessDenied:denied});expect(fetcher).not.toHaveBeenCalled();expect(host.querySelector<HTMLDetailsElement>('[data-knowledge-index-status]')?.open).toBe(false);
 await open();expect(fetcher).toHaveBeenCalledOnce();expect(host.textContent).toContain('No index service is configured');expect(host.textContent).toContain('event-newer');expect(host.textContent).toContain('event-older');expect(host.textContent).toContain('do not prove complete provider coverage');
 const [path,init]=fetcher.mock.calls[0];expect(path).toBe('/api/v1/project-knowledge/index-status?project_id=project-a');expect(init.headers).toMatchObject({'X-Suma-Expected-User':'user-a','X-Suma-Expected-Organization':'org-a'});
 code=503;await refresh();expect(host.querySelector('[data-knowledge-index-status-state="stale"]')).not.toBeNull();expect(host.textContent).toContain('event-older');expect(denied).not.toHaveBeenCalled();
 code=403;await refresh();expect(host.querySelector('[data-knowledge-index-status-state="denied"]')).not.toBeNull();expect(host.textContent).not.toContain('event-older');expect(host.textContent).not.toContain('provider-generation-a');expect(denied).toHaveBeenCalledOnce();
 code=200;value={...status,configured:true,pending:0,indexed:0,failed:0,total:0,current_publications:0,untracked_publications:0,latest_event:null,last_ack:null} as unknown as typeof status;await refresh();expect(host.textContent).toContain('This does not verify that it can answer a query');expect(host.textContent).toContain('No acknowledgement');expect(host.textContent).not.toContain('healthy');
 let finish!:(value:Response)=>void;fetcher.mockImplementationOnce(()=>new Promise<Response>(resolve=>{finish=resolve;}));await refresh();const before=fetcher.mock.calls.length;session={...session,user:{...session.user,id:'other'},organization:{id:'other-org',name:'Other organization'}};await render({project:'other-project',onAccessDenied:denied});expect(fetcher).toHaveBeenCalledTimes(before);expect(host.textContent).not.toContain('event-newer');
 await act(async()=>finish(Response.json({error:{code:'forbidden',message:'Old source denied'}},{status:403})));await settle();expect(denied).toHaveBeenCalledOnce();await open();expect(fetcher.mock.calls.at(-1)?.[0]).toBe('/api/v1/project-knowledge/index-status?project_id=other-project');
 await render({project:'other-project',accessBlocked:true,onAccessDenied:denied});expect(host.textContent).not.toContain('provider-generation-a');expect(host.querySelector<HTMLButtonElement>('[data-knowledge-index-status-refresh]')?.disabled).toBe(true);
 expect(fetcher.mock.calls.every(([,options])=>options.method==='GET'&&options.body===undefined)).toBe(true);
});
