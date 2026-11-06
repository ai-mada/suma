// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {beforeEach,afterEach,it,expect,vi} from 'vitest';
import {AdapterConnections,readAdapterCatalog,type AdapterDescriptor} from './AdapterConnections';
import {ApiViewProvider} from './api-context';
import {ApiError,type Session} from './api';
import {setLocale} from './i18n';
const call=vi.hoisted(()=>vi.fn());vi.mock('./api',async original=>({...await original<typeof import('./api')>(),api:call}));
let host:HTMLDivElement,root:Root;const setup=vi.fn();
const session:Session={user:{id:'user-a',name:'User',email:'user@example.test'},organization:{id:'org-a',name:'Org'},projects:[]};
const row:AdapterDescriptor={id:'codex-app-server',product:'Codex',direction:'suma_to_tool',entrypoint_kind:'app_server',adapter:'codex',protocol:'codex-app-server-v1',implementation:'implemented',setup_kind:'runner_profile',capabilities:[{key:'task_start',support:'supported',source_refs:[],limitations:[]},{key:'provider_billing',support:'unverified',source_refs:[],limitations:[]}],source_refs:['contracts/codex-app-server.md'],limitations:['implementation_not_connection','observe_only_budget']};
const cloud:AdapterDescriptor={...row,id:'workbuddy-cloud',product:'WorkBuddy',entrypoint_kind:'cloud_api',adapter:undefined,protocol:undefined,implementation:'implemented',setup_kind:'manual_guide',capabilities:[{key:'task_start',support:'supported',source_refs:['contracts/workbuddy-cloud-runner.md'],limitations:[]}],limitations:['workbuddy_not_codebuddy','cloud_fixed_delegation_required','cloud_delivery_not_verified']};
const guide:AdapterDescriptor={...row,id:'workbuddy-connector',product:'WorkBuddy connector',direction:'tool_to_suma',entrypoint_kind:'mcp_client',implementation:'guide_only',setup_kind:'manual_guide',capabilities:[],limitations:['no_execution_adapter']};
const page=(items=[row,cloud,guide],project='p-a')=>({project_id:project,as_of:'2026-10-29T00:00:00Z',items});
beforeEach(async()=>{vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);await setLocale('en-US',{persist:false});call.mockReset();setup.mockClear();call.mockResolvedValue(page());host=document.createElement('div');document.body.append(host);root=createRoot(host);});afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();});
async function render(project='p-a',identity=session){await act(async()=>root.render(<ApiViewProvider session={identity}><AdapterConnections project={project} onSetup={setup}/></ApiViewProvider>));}
async function click(selector:string){await act(async()=>host.querySelector<HTMLButtonElement>(selector)!.click());}
async function textClick(text:string){const b=[...host.querySelectorAll('button')].find(b=>b.textContent?.trim()===text);expect(b,text).toBeTruthy();await act(async()=>b!.click());}
it('separates execution and client directions and routes the exact selected protocol',async()=>{await render();expect(host.querySelectorAll('[data-adapter-id]')).toHaveLength(3);await click('[data-adapter-direction="suma_to_tool"]');expect(host.querySelectorAll('[data-adapter-id]')).toHaveLength(2);await click('[data-adapter-id="codex-app-server"]');await textClick('Configure execution profile');expect(setup).toHaveBeenCalledWith(row);expect(host.textContent).toContain('not prove installation');expect(host.textContent).toContain('Unverified');await click('[data-adapter-direction="tool_to_suma"]');expect(host.querySelectorAll('[data-adapter-id]')).toHaveLength(1);await click('[data-adapter-id="workbuddy-connector"]');await textClick('Read setup guide');expect(setup).toHaveBeenLastCalledWith(guide);});
it('routes protected WorkBuddy cloud setup to account connections without a local profile',async()=>{await render();await click('[data-adapter-id="workbuddy-cloud"]');expect(host.textContent).not.toContain('cannot currently receive tasks');expect(host.textContent).toContain('separate products');expect(host.textContent).toContain('does not verify code delivery');await textClick('Connect WorkBuddy account');expect(setup).toHaveBeenCalledWith(cloud);expect(cloud.adapter).toBeUndefined();expect(cloud.protocol).toBeUndefined();});
it('clears descriptors and action when refresh loses current permission',async()=>{await render();await click('[data-adapter-id="codex-app-server"]');call.mockRejectedValueOnce(new ApiError(403,'forbidden','Access denied'));await textClick('Refresh catalog');expect(host.querySelector('[data-adapter-id]')).toBeNull();expect(host.querySelector('.adapter-connection-next')).toBeNull();expect(host.textContent).toContain('Access denied');});
it('ignores old project responses and clears selection across identities',async()=>{let complete!:(value:unknown)=>void;call.mockImplementationOnce(()=>new Promise(r=>{complete=r;}));await render();call.mockResolvedValue(page([guide],'p-b'));await render('p-b');await act(async()=>complete(page()));expect(host.querySelector('[data-adapter-id="codex-app-server"]')).toBeNull();await click('[data-adapter-direction="tool_to_suma"]');await click('[data-adapter-id="workbuddy-connector"]');call.mockResolvedValue(page([],'p-b'));await render('p-b',{...session,user:{...session.user,id:'user-b'}});expect(host.querySelector('.adapter-connection-next')).toBeNull();});
async function searchFor(value:string){
 const input=host.querySelector<HTMLInputElement>('.adapter-connections-search input')!;
 await act(async()=>{
  Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(input,value);
  input.dispatchEvent(new Event('input',{bubbles:true}));
 });
 return input;
}
function visibleIDs(){
 return [...host.querySelectorAll<HTMLElement>('[data-adapter-id]')].map(button=>button.dataset.adapterId);
}
it('searches both directions with all terms and retains the selected direction',async()=>{
 await render();
 expect(visibleIDs()).toEqual([row.id,cloud.id,guide.id]);
 expect(host.querySelector('[data-adapter-direction="all"]')?.getAttribute('aria-pressed')).toBe('true');
 expect(host.querySelector('[data-adapter-id="workbuddy-connector"] .adapter-connection-direction')?.textContent).toBe('Tools access Suma');
 await searchFor('  WORKBUDDY  mCp  ');
 expect(visibleIDs()).toEqual([guide.id]);
 expect(host.querySelector('.adapter-connections-results')?.getAttribute('role')).toBe('status');
 expect(host.querySelector('.adapter-connections-results')?.textContent).toContain('1');
 await click('[data-adapter-direction="suma_to_tool"]');
 expect(visibleIDs()).toEqual([]);
 const input=await searchFor('CODEX  server-v1');
 expect(visibleIDs()).toEqual([row.id]);
 await click('.adapter-connections-clear-search');
 expect(document.activeElement).toBe(input);
 expect(input.value).toBe('');
 expect(visibleIDs()).toEqual([row.id,cloud.id]);
 expect(call).toHaveBeenCalledTimes(1);
});
it('moves focus to details and restores the card after closing',async()=>{
 await render();
 const card=host.querySelector<HTMLButtonElement>('[data-adapter-id="codex-app-server"]')!;
 card.focus();
 await click('[data-adapter-id="codex-app-server"]');
 const heading=host.querySelector<HTMLElement>('.adapter-connection-detail h4');
 expect(heading?.tabIndex).toBe(-1);
 expect(document.activeElement).toBe(heading);
 await textClick('Close details');
 expect(host.querySelector('.adapter-connection-detail')).toBeNull();
 expect(document.activeElement).toBe(card);
});
it('pins requests to the original view and prevents a delayed same-project view from restoring details',async()=>{
 await render();
 await click('[data-adapter-id="codex-app-server"]');
 let resolvePrevious!:(value:unknown)=>void;
 call.mockImplementationOnce(()=>new Promise(resolve=>{resolvePrevious=resolve;}));
 await textClick('Refresh catalog');
 const oldRequest=call.mock.calls.at(-1)!;
 expect(oldRequest[3]).toEqual({'X-Suma-Expected-User':'user-a','X-Suma-Expected-Organization':'org-a'});
 const identity={...session,user:{...session.user,id:'user-b'},organization:{...session.organization,id:'org-b'}};
 call.mockResolvedValue(page([guide]));
 await render('p-a',identity);
 expect(oldRequest[4].aborted).toBe(true);
 expect(call.mock.calls.at(-1)?.[3]).toEqual({'X-Suma-Expected-User':'user-b','X-Suma-Expected-Organization':'org-b'});
 await act(async()=>resolvePrevious(page([row])));
 expect(visibleIDs()).toEqual([guide.id]);
 expect(host.querySelector('.adapter-connection-detail')).toBeNull();
 expect(setup).not.toHaveBeenCalled();
});
it('rejects duplicate IDs, mismatched projects and unknown actionable states',()=>{expect(readAdapterCatalog(page([row,row]),'p-a')).toBeUndefined();expect(readAdapterCatalog(page(),'p-b')).toBeUndefined();expect(readAdapterCatalog(page([{...row,implementation:'connected'} as unknown as AdapterDescriptor]),'p-a')).toBeUndefined();});
it('renders unknown capability keys explicitly and supports language switching without new reads',async()=>{call.mockResolvedValue(page([{...row,capabilities:[{key:'new_unknown_cap',support:'unverified',source_refs:[],limitations:[]}]}]));await render();await click('[data-adapter-id="codex-app-server"]');expect(host.textContent).toContain('Unrecognized detail: new_unknown_cap');const count=call.mock.calls.length;await act(async()=>setLocale('zh-CN',{persist:false}));expect(host.textContent).toContain('开发工具统一接入');expect(host.textContent).toContain('待验证');expect(call.mock.calls).toHaveLength(count);});
it('does not render source paths as executable links',async()=>{call.mockResolvedValue(page([{...row,source_refs:['javascript:alert(1)','https://unexpected.example']} ]));await render();await click('[data-adapter-id="codex-app-server"]');expect(host.querySelector('.adapter-connection-sources a')).toBeNull();expect(host.querySelector('.adapter-connection-sources')?.textContent).toContain('javascript:alert(1)');});
