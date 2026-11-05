// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiError,type Session} from './api';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import {WorkBuddyConnections} from './WorkBuddyConnections';
import {officialWorkBuddyAuthorizationURL,readWorkBuddyPage,validWorkBuddyRecovery} from './workbuddy-connections-model';
const call=vi.hoisted(()=>vi.fn());
vi.mock('./api',async original=>({...await original<typeof import('./api')>(),api:call}));
const identity:Session={user:{id:'user-a',name:'User',email:'a@example.test'},organization:{id:'org-a',name:'Org'},projects:[]};
const row={id:'connection-a',project_id:'p-a',version:3,owner_id:'user-a',client_id:'app-a',open_id:'account-a',scopes:['user.task.readable'],state:'connected',grant_generation:1,token_generation:2,expires_at:'2026-10-30T03:00:00Z',created_at:'2026-10-30T01:00:00Z',updated_at:'2026-10-30T02:00:00Z',last_error:'',allowed_actions:['authorize','refresh','disconnect']};
const page=(items=[row],project='p-a')=>({project_id:project,configured:true,available_scopes:['user.task.readable','user.task.invokable'],can_authorize:true,items,limit:20,next_cursor:''});
let host:HTMLDivElement,root:Root;
beforeEach(async()=>{vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);await setLocale('en-US',{persist:false});sessionStorage.clear();call.mockReset();call.mockResolvedValue(page());host=document.createElement('div');document.body.append(host);root=createRoot(host);});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();});
async function render(project='p-a',session=identity){await act(async()=>root.render(<ApiViewProvider session={session}><WorkBuddyConnections project={project}/></ApiViewProvider>));}
async function click(text:string){const button=[...host.querySelectorAll('button')].find(node=>node.textContent?.trim()===text);expect(button,text).toBeTruthy();await act(async()=>button!.click());}
async function open(){await act(async()=>host.querySelector<HTMLButtonElement>('.workbuddy-disclosure')!.click());}
async function chooseScope(){const box=host.querySelector<HTMLInputElement>('input[value="user.task.readable"]');expect(box).toBeTruthy();await act(async()=>box!.click());}

it('keeps reauthorization scopes empty until explicit reuse and separates a manually added permission',async()=>{
 await render();await open();await click('Authorize again');
 const form=host.querySelector<HTMLFormElement>('form')!;
 expect([...form.querySelectorAll<HTMLInputElement>('input[type="checkbox"]')].every(input=>!input.checked)).toBe(true);
 expect(form.querySelector<HTMLButtonElement>('button[type="submit"]')?.disabled).toBe(true);
 expect(form.querySelector('.workbuddy-current-scopes')?.textContent).toContain('Read WorkBuddy tasks');
 await click('Reuse current requestable permissions');
 expect(form.querySelector<HTMLInputElement>('input[value="user.task.readable"]')?.checked).toBe(true);
 expect(form.querySelector<HTMLInputElement>('input[value="user.task.invokable"]')?.checked).toBe(false);
 expect(form.querySelector('[aria-label="Requested again"]')?.textContent).toContain('Read WorkBuddy tasks');
 expect(form.querySelector('[aria-label="New requests"]')?.textContent).toContain('None');
 await act(async()=>form.querySelector<HTMLInputElement>('input[value="user.task.invokable"]')!.click());
 expect(form.querySelector('[aria-label="New requests"]')?.textContent).toContain('Create WorkBuddy tasks');
 expect(call.mock.calls.every(([,body])=>!body)).toBe(true);
});

it('explains scope tightening and reuses only the still-requestable part of the original grant',async()=>{
 call.mockResolvedValue({...page([{...row,scopes:['user.task.readable','user.task.invokable']}]),available_scopes:['user.task.readable']});
 await render();await open();await click('Authorize again');
 expect(host.querySelector('form .workbuddy-current-scopes')?.textContent).toContain('Create WorkBuddy tasks');
 expect(host.querySelector('form input[value="user.task.invokable"]')).toBeNull();
 await click('Reuse current requestable permissions');
 expect(host.querySelector<HTMLInputElement>('form input[value="user.task.readable"]')?.checked).toBe(true);
 expect(host.querySelector('form [aria-label="Not requested again"]')?.textContent).toContain('Create WorkBuddy tasks');
 expect(host.querySelector('form .workbuddy-scope-unavailable')?.textContent).toContain('Create WorkBuddy tasks');
 expect(host.textContent).toContain('These are proposed changes');expect(call.mock.calls.every(([,body])=>!body)).toBe(true);
});

it('shows safe bilingual reasons and timestamps while leaving unknown and pending connections outside disconnected history',async()=>{
 const unknown={...row,id:'unknown-a',state:'refresh_unknown',last_error:'private-machine-token-C:/private/account',allowed_actions:['authorize','disconnect']};
 const required={...row,id:'required-a',state:'reauthorization_required',last_error:'workbuddy_reauthorization_required',allowed_actions:['authorize','disconnect']};
 const pending={...row,id:'pending-a',state:'authorizing',allowed_actions:['disconnect']};
 const disconnected={...row,id:'disconnected-a',state:'disconnected',allowed_actions:['authorize']};
 call.mockResolvedValue(page([unknown,required,pending,disconnected]));await render();await open();
 const current=host.querySelector('[aria-label="Current authorizations and pending actions"]')!;
 for(const id of ['unknown-a','required-a','pending-a'])expect(current.querySelector('[data-workbuddy-connection-id="'+id+'"]')).toBeTruthy();
 expect(host.querySelector('.workbuddy-history [data-workbuddy-connection-id="unknown-a"]')).toBeNull();
 expect(host.querySelector('[data-workbuddy-connection-id="unknown-a"] .workbuddy-connection-reason')?.textContent).toBe('The connection needs attention. Read its current status and review the currently allowed actions.');
 expect(host.querySelector('[data-workbuddy-connection-id="required-a"] .workbuddy-connection-reason')?.textContent).toContain('The original authorization needs confirmation again');
 expect(host.querySelector('[data-workbuddy-connection-id="required-a"] time')?.getAttribute('datetime')).toBe(row.updated_at);
 expect(host.textContent).not.toContain('private-machine-token');expect(host.textContent).not.toContain('C:/private/account');
 expect(host.textContent).toContain('not every account in the project');expect(call.mock.calls.every(([,body])=>!body)).toBe(true);
 await act(async()=>{await setLocale('zh-CN',{persist:false});});
 expect(host.textContent).toContain('连接出现异常。请读取当前状态');expect(host.textContent).toContain('原授权需要重新确认');expect(host.textContent).toContain('最近更新');
});

it('keeps an explicitly located disconnected account visible and reauthorizable outside the bounded list page',async()=>{
 const target={...row,id:'history-target',open_id:'history-account',state:'disconnected',allowed_actions:['authorize']};
 call.mockImplementation(async(path:string)=>path.startsWith('/workbuddy-connections/')?target:page());
 await act(async()=>root.render(<ApiViewProvider session={identity}><WorkBuddyConnections project="p-a" panel="workbuddy:history-target"/></ApiViewProvider>));
 const card=host.querySelector('.workbuddy-history [data-workbuddy-connection-id="history-target"]');
 expect(card).toBeTruthy();expect(card?.classList.contains('workbuddy-card-target')).toBe(true);expect(card?.textContent).toContain('history-account');
 expect(host.querySelector('.workbuddy-history-toggle')?.getAttribute('aria-expanded')).toBe('true');expect(host.querySelector<HTMLDivElement>('.workbuddy-history-content')?.hidden).toBe(false);
 expect(host.querySelectorAll('[data-workbuddy-connection-id="history-target"]')).toHaveLength(1);
 expect(host.querySelector('[aria-label="Current authorizations and pending actions"] [data-workbuddy-connection-id="connection-a"]')).toBeTruthy();
 const reauthorize=[...card!.querySelectorAll('button')].find(button=>button.textContent==='Authorize again');await act(async()=>reauthorize!.click());
 expect(host.querySelector('form')?.textContent).toContain('history-account');expect(host.querySelector<HTMLInputElement>('form input[type="checkbox"]')?.checked).toBe(false);
 await chooseScope();await act(async()=>host.querySelector<HTMLButtonElement>('.workbuddy-history-toggle')!.click());
 expect(host.querySelector<HTMLDivElement>('.workbuddy-history-content')?.hidden).toBe(true);expect(host.querySelector<HTMLInputElement>('form input[value="user.task.readable"]')?.checked).toBe(true);
 await act(async()=>host.querySelector<HTMLButtonElement>('.workbuddy-history-toggle')!.click());expect(host.querySelector<HTMLInputElement>('form input[value="user.task.readable"]')?.checked).toBe(true);
 expect(call.mock.calls.every(([,body])=>!body)).toBe(true);
 await click('Cancel');call.mockRejectedValue(new ApiError(403,'forbidden','Access denied'));await click('Refresh list');
 expect(host.textContent).not.toContain('history-account');expect(host.querySelector('.workbuddy-history')).toBeNull();
});

it('retains an unknown reauthorization and its original recovery when disconnected history is collapsed',async()=>{
 const disconnected={...row,state:'disconnected',allowed_actions:['authorize']};call.mockResolvedValue(page([disconnected]));await render();await open();
 const toggle=host.querySelector<HTMLButtonElement>('.workbuddy-history-toggle')!;
 expect(toggle.getAttribute('aria-expanded')).toBe('false');expect(toggle.textContent).toContain('Disconnected records in this read · 1');
 await act(async()=>toggle.click());await click('Authorize again');await chooseScope();call.mockRejectedValueOnce(new ApiError(0,'network_error','Network unavailable'));
 await act(async()=>host.querySelector<HTMLFormElement>('form')!.requestSubmit());
 const original=call.mock.calls.find(([,body])=>body),stored=Object.values(sessionStorage).join(''),requests=call.mock.calls.length;
 expect(original?.[1]).toMatchObject({project_id:'p-a',connection_id:'connection-a',expected_version:3,scopes:['user.task.readable']});
 await act(async()=>toggle.click());
 expect(host.querySelector<HTMLDivElement>('.workbuddy-history-content')?.hidden).toBe(true);
 const recover=[...host.querySelectorAll('button')].find(button=>button.textContent==='Recover original operation');
 expect(recover).toBeTruthy();expect(recover?.closest('.workbuddy-history-content')).toBeNull();expect(recover?.disabled).toBe(false);
 expect(host.textContent).toContain('The operation outcome is unknown');expect(host.querySelector<HTMLInputElement>('form input[value="user.task.readable"]')?.checked).toBe(true);
 expect(Object.values(sessionStorage).join('')).toBe(stored);expect(stored).toContain((original?.[1] as {idempotency_key:string}).idempotency_key);expect(call).toHaveBeenCalledTimes(requests);
});

it('keeps connections folded and reads only a bounded page after expansion',async()=>{await render();expect(call).not.toHaveBeenCalled();await open();expect(call.mock.calls[0][0]).toBe('/workbuddy-connections?project_id=p-a&limit=20');expect(host.textContent).toContain('account-a');expect(host.textContent).toContain('permissions and execution eligibility checked separately');expect(host.textContent).not.toContain('Cloud task dispatch is not available');expect(host.querySelector('details')).toBeTruthy();});
it('prepares only explicitly selected scopes and keeps the official URL out of storage',async()=>{await render();await open();await click('Connect WorkBuddy account');expect(host.querySelector<HTMLButtonElement>('button[type="submit"]')?.disabled).toBe(true);await chooseScope();let stored='';const authorization={id:'authorization-a',version:1,connection:{...row,state:'authorizing',allowed_actions:['disconnect']},authorization_url:'https://www.workbuddy.cn/openapi/v2/authorize?state=private-state',expires_at:'2099-01-01T00:00:00Z',state:'pending'};call.mockImplementation(async(path:string,body?:unknown)=>{if(body){stored=Object.values(sessionStorage).join('');return authorization;}return page([authorization.connection]);});await act(async()=>host.querySelector<HTMLFormElement>('form')!.requestSubmit());const link=host.querySelector<HTMLAnchorElement>('a[target="_blank"]');expect(link?.href).toBe(authorization.authorization_url);expect(link?.rel).toBe('noopener noreferrer');expect(stored).toContain('user.task.readable');expect(stored).not.toContain('private-state');expect(Object.values(sessionStorage).join('')).not.toContain('private-state');const body=call.mock.calls.find(([,body])=>body)?.[1];expect(body).toMatchObject({project_id:'p-a',scopes:['user.task.readable']});expect(body).not.toHaveProperty('code');});
it('gates writes by current server actions and supports read-only viewers',async()=>{call.mockResolvedValue({...page([{...row,allowed_actions:[]}]),can_authorize:false});await render();await open();expect(host.textContent).toContain('account-a');expect(host.textContent).not.toContain('Connect WorkBuddy account');expect([...host.querySelectorAll('button')].some(b=>b.textContent==='Refresh authorization')).toBe(false);});
it('clears protected summaries and URL on access loss',async()=>{await render();await open();call.mockRejectedValueOnce(new ApiError(403,'forbidden','Access denied'));await click('Refresh list');expect(host.textContent).not.toContain('account-a');expect(host.querySelector('a[target="_blank"]')).toBeNull();expect(host.textContent).toContain('Access denied');});
it('ignores a late response after switching project and identity',async()=>{let complete!:(value:unknown)=>void;call.mockImplementationOnce(()=>new Promise(resolve=>{complete=resolve;}));await render();await open();await render('p-b',{...identity,user:{...identity.user,id:'user-b'}});expect(host.textContent).not.toContain('account-a');call.mockResolvedValue(page([],'p-b'));await open();await act(async()=>complete(page()));expect(host.textContent).not.toContain('account-a');expect(host.textContent).toContain('No WorkBuddy accounts connected');});
it('preserves the original refresh key and version after an unknown outcome',async()=>{await render();await open();call.mockRejectedValueOnce(new ApiError(0,'network_error','Network unavailable'));await click('Refresh authorization');const first=call.mock.calls.find(([,body])=>body);expect(first?.[1]).toMatchObject({project_id:'p-a',expected_version:3});expect(host.textContent).toContain('The operation outcome is unknown');expect(host.querySelector('button[type="submit"]')).toBeNull();call.mockResolvedValueOnce({...row,version:4});await click('Recover original operation');const writes=call.mock.calls.filter(([,body])=>body);expect(writes).toHaveLength(2);expect(writes[1].slice(0,3)).toEqual(writes[0].slice(0,3));});
it('requires an explicit local-only disconnect confirmation and moves the record only after its receipt',async()=>{
 await render();await open();await click('Disconnect locally');expect(host.textContent).toContain('does not revoke access at WorkBuddy or stop remote tasks');expect(call.mock.calls.filter(([,body])=>body)).toHaveLength(0);
 const box=host.querySelector<HTMLInputElement>('input[name="confirm-disconnect"]');await act(async()=>box!.click());
 const disconnected={...row,state:'disconnected',version:4,allowed_actions:['authorize']};let complete!:(value:unknown)=>void;
 call.mockImplementation((path:string,body?:unknown)=>body?new Promise(resolve=>{complete=resolve;}):Promise.resolve(page([disconnected])));
 await act(async()=>host.querySelector<HTMLFormElement>('form')!.requestSubmit());
 const write=call.mock.calls.find(([,body])=>body);expect(write?.[0]).toBe('/workbuddy-connections/connection-a/disconnect');expect(write?.[1]).toMatchObject({project_id:'p-a',expected_version:3,reason:'user_requested_local_disconnect'});expect((write?.[1] as {idempotency_key:string}).idempotency_key).toBeTruthy();
 expect(host.querySelector('.workbuddy-history')).toBeNull();expect(host.querySelector('[aria-label="Current authorizations and pending actions"] [data-workbuddy-connection-id="connection-a"]')).toBeTruthy();
 await act(async()=>complete(disconnected));
 expect(host.querySelector('.workbuddy-history [data-workbuddy-connection-id="connection-a"]')).toBeTruthy();expect(host.querySelector('[aria-label="Current authorizations and pending actions"]')).toBeNull();
 const toggle=host.querySelector<HTMLButtonElement>('.workbuddy-history-toggle')!;expect(toggle.getAttribute('aria-expanded')).toBe('false');expect(toggle.textContent).toContain('Disconnected records in this read · 1');expect(host.querySelector<HTMLDivElement>('.workbuddy-history-content')?.hidden).toBe(true);
 await act(async()=>toggle.click());expect(host.querySelector<HTMLDivElement>('.workbuddy-history-content')?.hidden).toBe(false);
 expect(host.textContent).toContain('retain their records and related task and run evidence');expect(host.textContent).toContain('Remote access and remote tasks were not revoked or stopped');
});
it('does not accept cross-project, duplicate or unbounded pages and secret-bearing recovery bodies',()=>{expect(readWorkBuddyPage(page(),'p-b')).toBeUndefined();expect(readWorkBuddyPage(page([row,row]),'p-a')).toBeUndefined();expect(readWorkBuddyPage({...page(),limit:100},'p-a')).toBeUndefined();expect(validWorkBuddyRecovery({path:'/workbuddy-authorizations',body:{project_id:'p-a',scopes:['user.task.readable'],code:'secret',idempotency_key:'operation-a'},method:'POST',idempotency_key:'operation-a'},'p-a')).toBe(false);});
it('uses bounded server cursors and restores the previous page without accumulating rows',async()=>{call.mockResolvedValueOnce({...page(),next_cursor:'opaque/page-2'});await render();await open();call.mockResolvedValueOnce(page([{...row,id:'connection-b',open_id:'account-b'}]));await click('Next page');expect(call.mock.calls.at(-1)?.[0]).toBe('/workbuddy-connections?project_id=p-a&limit=20&cursor=opaque%2Fpage-2');expect(host.textContent).toContain('account-b');expect(host.textContent).not.toContain('account-a');call.mockResolvedValueOnce({...page(),next_cursor:'opaque/page-2'});await click('Previous page');expect(call.mock.calls.at(-1)?.[0]).toBe('/workbuddy-connections?project_id=p-a&limit=20');expect(host.textContent).toContain('account-a');expect(host.textContent).not.toContain('account-b');});
it('retains the original start after timeout and refuses writes when the reread becomes read-only',async()=>{await render();await open();await click('Connect WorkBuddy account');await chooseScope();call.mockRejectedValueOnce(new ApiError(0,'network_error','Network unavailable'));await act(async()=>host.querySelector<HTMLFormElement>('form')!.requestSubmit());const first=call.mock.calls.find(([,body])=>body);expect(first?.[0]).toBe('/workbuddy-authorizations');call.mockResolvedValueOnce({...page([{...row,allowed_actions:[]}]),can_authorize:false});await click('Read current status');expect(host.textContent).not.toContain('Recover original operation');expect(host.textContent).not.toContain('Connect WorkBuddy account');expect(call.mock.calls.filter(([,body])=>body)).toHaveLength(1);expect(Object.values(sessionStorage).join('')).toContain((first?.[1] as {idempotency_key:string}).idempotency_key);});
it('hides a late authorization URL and leaves its original safe recovery after an identity change',async()=>{await render();await open();await click('Connect WorkBuddy account');await chooseScope();let complete!:(value:unknown)=>void;call.mockImplementationOnce(()=>new Promise(resolve=>{complete=resolve;}));await act(async()=>host.querySelector<HTMLFormElement>('form')!.requestSubmit());call.mockResolvedValue(page([],'p-b'));await render('p-b',{...identity,user:{...identity.user,id:'user-b'}});await act(async()=>complete({id:'authorization-a',version:1,connection:{...row,state:'authorizing'},authorization_url:'https://www.workbuddy.cn/openapi/v2/authorize?state=private-late-state',expires_at:'2099-01-01T00:00:00Z',state:'pending'}));await open();expect(host.querySelector('a[target="_blank"]')).toBeNull();expect(host.textContent).not.toContain('account-a');const recovery=Object.values(sessionStorage).join('');expect(recovery).toContain('p-a');expect(recovery).not.toContain('private-late-state');expect(call.mock.calls.filter(([,body])=>body)).toHaveLength(1);});
it('protects a selected permission draft until explicit cancellation',async()=>{await render();await open();await click('Connect WorkBuddy account');await chooseScope();let allowed=true;await act(async()=>{allowed=window.dispatchEvent(new Event('suma:navigate',{cancelable:true}));});expect(allowed).toBe(false);await click('Cancel');expect(window.dispatchEvent(new Event('suma:navigate',{cancelable:true}))).toBe(true);});
it('permits only an official fixed HTTPS authorization page',()=>{expect(officialWorkBuddyAuthorizationURL('https://www.workbuddy.cn/openapi/v2/authorize?state=private')).toBe(true);for(const url of ['javascript:alert(1)','https://evil.example/openapi/v2/authorize','https://www.workbuddy.cn.evil.example/openapi/v2/authorize','https://user@www.workbuddy.cn/openapi/v2/authorize','http://www.workbuddy.cn/openapi/v2/authorize','https://www.workbuddy.cn/openapi/v2/tasks','https://www.workbuddy.cn/openapi/v2/authorize#fragment'])expect(officialWorkBuddyAuthorizationURL(url),url).toBe(false);});
it('keeps an in-memory link while authorization is still pending and clears it on 404',async()=>{
 await render();await open();await click('Connect WorkBuddy account');await chooseScope();
 const pending={...row,state:'authorizing',allowed_actions:['disconnect']},receipt={id:'authorization-a',version:1,connection:pending,authorization_url:'https://www.workbuddy.cn/openapi/v2/authorize?state=private-pending-state',expires_at:'2099-01-01T00:00:00Z',state:'pending'};
 call.mockImplementation(async(path:string,body?:unknown)=>body?receipt:path.startsWith('/workbuddy-connections/')?pending:page([pending]));
 await act(async()=>host.querySelector<HTMLFormElement>('form')!.requestSubmit());await click('Check authorization result');
 expect(call.mock.calls.some(([path])=>path==='/workbuddy-connections/connection-a?project_id=p-a')).toBe(true);
 expect(host.querySelector<HTMLAnchorElement>('a[target="_blank"]')?.href).toBe(receipt.authorization_url);
 call.mockRejectedValueOnce(new ApiError(404,'not_found','Not found'));await click('Check authorization result');
 expect(host.querySelector('a[target="_blank"]')).toBeNull();expect(host.textContent).not.toContain('account-a');
 call.mockResolvedValueOnce(page([pending]));await click('Refresh list');expect(host.querySelector('a[target="_blank"]')).toBeNull();expect(Object.values(sessionStorage).join('')).not.toContain('private-pending-state');
});
it('retains and checks a new authorization whose connection is absent from the bounded page',async()=>{
 call.mockResolvedValueOnce({...page(),next_cursor:'page-two'});await render();await open();
 call.mockResolvedValueOnce(page([{...row,id:'connection-b',open_id:'account-b'}]));await click('Next page');await click('Connect WorkBuddy account');await chooseScope();
 const receipt={id:'authorization-new',version:1,connection:{...row,id:'connection-new',state:'authorizing',allowed_actions:['disconnect']},authorization_url:'https://www.workbuddy.cn/openapi/v2/authorize?state=private-new-state',expires_at:'2099-01-01T00:00:00Z',state:'pending'};
 call.mockImplementation(async(path:string,body?:unknown)=>body?receipt:path.startsWith('/workbuddy-connections/')?{...receipt.connection,state:'connected',version:4}:page([{...row,id:'connection-b',open_id:'account-b'}]));
 await act(async()=>host.querySelector<HTMLFormElement>('form')!.requestSubmit());expect(host.querySelector<HTMLAnchorElement>('a[target="_blank"]')?.href).toBe(receipt.authorization_url);
 await click('Check authorization result');expect(call.mock.calls.some(([path])=>path==='/workbuddy-connections/connection-new?project_id=p-a')).toBe(true);expect(host.textContent).toContain('WorkBuddy account authorization was saved');expect(host.querySelector('a[target="_blank"]')).toBeNull();
});
it('recovers the same unknown operation after a refused recovery and a successful authorized reread',async()=>{
 await render();await open();call.mockRejectedValueOnce(new ApiError(0,'network_error','Network unavailable'));await click('Refresh authorization');
 const original=call.mock.calls.find(([,body])=>body);call.mockRejectedValueOnce(new ApiError(403,'forbidden','Access denied'));await click('Recover original operation');
 expect(host.textContent).not.toContain('account-a');expect(host.textContent).not.toContain('Recover original operation');
 call.mockResolvedValueOnce(page());await click('Refresh list');expect(host.textContent).toContain('account-a');expect(host.textContent).toContain('Recover original operation');
 call.mockImplementation(async(path:string,body?:unknown)=>body?{...row,version:4}:page());await click('Recover original operation');
 const writes=call.mock.calls.filter(([,body])=>body);expect(writes).toHaveLength(3);for(const write of writes)expect(write.slice(0,3)).toEqual(original?.slice(0,3));
});
it('clears an existing private link when the same account becomes a read-only viewer',async()=>{
 await render();await open();await click('Connect WorkBuddy account');await chooseScope();
 const pending={...row,state:'authorizing',allowed_actions:['disconnect']},receipt={id:'authorization-a',version:1,connection:pending,authorization_url:'https://www.workbuddy.cn/openapi/v2/authorize?state=private-viewer-state',expires_at:'2099-01-01T00:00:00Z',state:'pending'};
 call.mockImplementation(async(path:string,body?:unknown)=>body?receipt:page([pending]));await act(async()=>host.querySelector<HTMLFormElement>('form')!.requestSubmit());expect(host.querySelector('a[target="_blank"]')).toBeTruthy();
 call.mockResolvedValueOnce({...page([{...pending,allowed_actions:[]}]),can_authorize:false});await click('Refresh list');expect(host.querySelector('a[target="_blank"]')).toBeNull();
 call.mockResolvedValueOnce(page([pending]));await click('Refresh list');expect(host.querySelector('a[target="_blank"]')).toBeNull();
});
it('opens the clean callback route and reads its exact current-authorized connection outside the list page',async()=>{
 const target={...row,id:'callback-connection',open_id:'callback-account'};
 call.mockImplementation(async(path:string)=>path.startsWith('/workbuddy-connections/')?target:page());
 await act(async()=>root.render(<ApiViewProvider session={identity}><WorkBuddyConnections project="p-a" panel="workbuddy:callback-connection"/></ApiViewProvider>));
 expect(host.querySelector('.workbuddy-disclosure')?.getAttribute('aria-expanded')).toBe('true');
 expect(call.mock.calls.some(([path])=>path==='/workbuddy-connections/callback-connection?project_id=p-a')).toBe(true);
 expect(host.querySelector('[data-workbuddy-connection-id="callback-connection"]')?.textContent).toContain('callback-account');
 expect(host.textContent).toContain('account-a');expect(call.mock.calls.every(([,body])=>!body)).toBe(true);
 await click('Authorize again');await chooseScope();
 const pending={...target,state:'authorizing',version:4,allowed_actions:['disconnect']},receipt={id:'callback-authorization',version:1,connection:pending,authorization_url:'https://www.workbuddy.cn/openapi/v2/authorize?state=private-callback-state',expires_at:'2099-01-01T00:00:00Z',state:'pending'};
 call.mockImplementation(async(path:string,body?:unknown)=>body?receipt:path.startsWith('/workbuddy-connections/')?pending:page());
 await act(async()=>host.querySelector<HTMLFormElement>('form')!.requestSubmit());expect(host.querySelector('a[target="_blank"]')).toBeTruthy();
 let rejectDetail!:(error:unknown)=>void;
 call.mockImplementation((path:string)=>path.startsWith('/workbuddy-connections/')?new Promise((resolve,reject)=>{rejectDetail=reject;}):Promise.reject(new ApiError(500,'server_error','Unavailable')));
 await click('Refresh list');await act(async()=>rejectDetail(new ApiError(403,'forbidden','Access denied')));
 call.mockImplementation(async(path:string)=>path.startsWith('/workbuddy-connections/')?pending:page());await click('Refresh list');expect(host.querySelector('a[target="_blank"]')).toBeNull();
 call.mockImplementation(async(path:string)=>{if(path.startsWith('/workbuddy-connections/'))throw new ApiError(404,'not_found','Not found');return page();});
 await click('Refresh list');expect(host.textContent).not.toContain('callback-account');expect(host.textContent).not.toContain('account-a');
});
it('guides the desktop shell to a clean browser address without starting or exposing OAuth authorization',async()=>{
 vi.stubGlobal('sumaDesktop',{platform:'win32',version:'1.0.0',chooseProjectDirectory:vi.fn()});
 const previousClipboard=Object.getOwnPropertyDescriptor(navigator,'clipboard'),previousURL=location.href,copy=vi.fn().mockResolvedValue(undefined),privateURL='https://www.workbuddy.cn/openapi/v2/authorize?state=private-desktop-state';
 Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:copy}});
 const key='desktop-original-start',request={path:'/workbuddy-authorizations',method:'POST',idempotency_key:key,body:{project_id:'p-a',scopes:['user.task.readable'],idempotency_key:key}};
 sessionStorage.setItem('suma-write-recovery-v1:workbuddy-connections:'+JSON.stringify(['org-a','user-a','p-a']),JSON.stringify(request));
 try{
  history.replaceState({},'','/?code=private-code&state=private-state');call.mockResolvedValue({...page(),authorization_url:privateURL});await render();await open();
  expect(host.textContent).toContain('system browser');expect(host.textContent).not.toContain('Connect WorkBuddy account');expect(host.textContent).not.toContain('Authorize again');expect(host.textContent).not.toContain('Recover original operation');
  expect(host.querySelector('a[target="_blank"]')).toBeNull();expect(host.textContent).not.toContain('private-desktop-state');expect(host.textContent).toContain('Refresh authorization');expect(host.textContent).toContain('Disconnect locally');
  const address=host.querySelector<HTMLTextAreaElement>('[data-workbuddy-browser-address]'),expected=location.origin+'/?project=p-a&view=settings&sub=tools';expect(address?.readOnly).toBe(true);expect(address?.value).toBe(expected);
  await act(async()=>host.querySelector<HTMLButtonElement>('[data-workbuddy-browser-copy]')!.click());expect(copy).toHaveBeenCalledWith(expected);expect(copy.mock.calls.flat().join(' ')).not.toContain('private-');
  copy.mockRejectedValueOnce(new Error('unavailable'));await act(async()=>host.querySelector<HTMLButtonElement>('[data-workbuddy-browser-copy]')!.click());expect(document.activeElement).toBe(address);expect(address?.selectionEnd).toBe(expected.length);
  expect(call.mock.calls.every(([,body])=>!body)).toBe(true);expect(sessionStorage.getItem('suma-write-recovery-v1:workbuddy-connections:'+JSON.stringify(['org-a','user-a','p-a']))).toContain(key);
 }finally{history.replaceState({},'',previousURL);if(previousClipboard)Object.defineProperty(navigator,'clipboard',previousClipboard);else Reflect.deleteProperty(navigator,'clipboard');}
});

const nativeReceipt=()=>({id:'authorization-native',version:1,connection:{...row,state:'authorizing',version:4,allowed_actions:['disconnect']},authorization_url:'https://www.workbuddy.cn/openapi/v2/authorize?state=private-native-state',expires_at:'2099-01-01T00:00:00Z',state:'pending'});
function nativeBridge(openAuthorization=vi.fn().mockResolvedValue({status:'opened'})){
 vi.stubGlobal('sumaDesktop',{platform:'win32',version:'1.0.0',chooseProjectDirectory:vi.fn(),openWorkBuddyAuthorization:openAuthorization});return openAuthorization;
}
async function prepareNative(reauthorize=false){
 await render();await open();expect(host.querySelector('[data-workbuddy-browser-address]')).toBeNull();
 await click(reauthorize?'Authorize again':'Connect WorkBuddy account');await chooseScope();
 const receipt=nativeReceipt();call.mockImplementation(async(path:string,body?:unknown)=>body?receipt:path.startsWith('/workbuddy-connections/')?receipt.connection:page([receipt.connection]));
 await act(async()=>host.querySelector<HTMLFormElement>('form')!.requestSubmit());return receipt;
}

it('native desktop prepares a scoped start and sends only four IDs, with duplicate opening blocked',async()=>{
 let resolve!:(value:unknown)=>void;const native=nativeBridge(vi.fn(()=>new Promise(done=>{resolve=done;})));
 const receipt=await prepareNative();expect(host.querySelector('a[target="_blank"]')).toBeNull();expect(native).not.toHaveBeenCalled();
 const write=call.mock.calls.find(([,body])=>body);expect(write?.[0]).toBe('/workbuddy-authorizations');expect(write?.[1]).toMatchObject({project_id:'p-a',scopes:['user.task.readable']});
 await click('Authorize in desktop window');expect(native).toHaveBeenCalledExactlyOnceWith({project_id:'p-a',authorization_id:receipt.id,user_id:'user-a',tenant_id:'org-a'});
 const opening=[...host.querySelectorAll('button')].find(button=>button.textContent?.includes('Opening authorization window'));expect(opening?.disabled).toBe(true);
 await act(async()=>opening!.click());expect(native).toHaveBeenCalledTimes(1);const reads=call.mock.calls.length;await act(async()=>window.dispatchEvent(new Event('focus')));expect(call).toHaveBeenCalledTimes(reads);
 await act(async()=>resolve({status:'opened'}));expect(host.textContent).toContain('The authorization window is open');expect(host.textContent).not.toContain('WorkBuddy account authorization was saved');expect(JSON.stringify(native.mock.calls)).not.toContain('private-native-state');expect(Object.values(sessionStorage).join('')).not.toContain('private-native-state');
});

it('native desktop supports reauthorization using the original connection version',async()=>{
 const native=nativeBridge();await prepareNative(true);expect(call.mock.calls.find(([,body])=>body)?.[1]).toMatchObject({connection_id:'connection-a',expected_version:3,scopes:['user.task.readable']});await click('Authorize in desktop window');expect(native).toHaveBeenCalledExactlyOnceWith({project_id:'p-a',authorization_id:'authorization-native',user_id:'user-a',tenant_id:'org-a'});
});

it('native desktop return focus reads the exact connection before showing authorization success',async()=>{
 nativeBridge();const receipt=await prepareNative();await click('Authorize in desktop window');const before=call.mock.calls.length;
 const connected={...receipt.connection,state:'connected',version:5,allowed_actions:['authorize','refresh','disconnect']};call.mockImplementation(async(path:string)=>path.startsWith('/workbuddy-connections/')?connected:page([connected]));
 await act(async()=>window.dispatchEvent(new Event('focus')));const requests=call.mock.calls.slice(before);expect(requests.some(([path])=>path==='/workbuddy-connections/connection-a?project_id=p-a')).toBe(true);expect(requests.every(([,body])=>!body)).toBe(true);expect(host.textContent).toContain('WorkBuddy account authorization was saved');expect(call.mock.calls.filter(([,body])=>body)).toHaveLength(1);
});

it('native desktop focus preserves an edited scope draft and an unknown write without automatic requests',async()=>{
 nativeBridge();await prepareNative();await click('Authorize in desktop window');await click('Connect WorkBuddy account');await chooseScope();
 const before=call.mock.calls.length;await act(async()=>window.dispatchEvent(new Event('focus')));expect(call).toHaveBeenCalledTimes(before);expect(host.querySelector<HTMLInputElement>('input[value="user.task.readable"]')?.checked).toBe(true);
 call.mockRejectedValueOnce(new ApiError(0,'network_error','Network unavailable'));await act(async()=>host.querySelector<HTMLFormElement>('form')!.requestSubmit());expect(host.textContent).toContain('The operation outcome is unknown');const pending=call.mock.calls.length;
 await act(async()=>window.dispatchEvent(new Event('focus')));expect(call).toHaveBeenCalledTimes(pending);expect(host.textContent).toContain('The operation outcome is unknown');
});

it('native desktop ignores a late opening result after project and identity change',async()=>{
 let resolve!:(value:unknown)=>void;nativeBridge(vi.fn(()=>new Promise(done=>{resolve=done;})));await prepareNative();await click('Authorize in desktop window');
 call.mockResolvedValue(page([],'p-b'));await render('p-b',{...identity,user:{...identity.user,id:'user-b'}});await open();const before=call.mock.calls.length;
 await act(async()=>resolve({status:'opened'}));await act(async()=>window.dispatchEvent(new Event('focus')));expect(call).toHaveBeenCalledTimes(before);expect(host.textContent).not.toContain('The authorization window is open');expect(host.textContent).not.toContain('account-a');expect(host.querySelector('a[target="_blank"]')).toBeNull();
});

it('native desktop failure remains a safe hint with explicit current-status recovery',async()=>{
 nativeBridge(vi.fn().mockResolvedValue({status:'failed',code:'private-native-state provider secret'}));await prepareNative();await click('Authorize in desktop window');expect(host.textContent).toContain('The authorization window could not open');expect(host.textContent).not.toContain('provider secret');expect(host.textContent).not.toContain('WorkBuddy account authorization was saved');
 await click('Check authorization result');expect(call.mock.calls.some(([path])=>path==='/workbuddy-connections/connection-a?project_id=p-a')).toBe(true);expect(call.mock.calls.filter(([,body])=>body)).toHaveLength(1);
});

it('native desktop coalesces return-focus reads and refuses another connection as success',async()=>{
 nativeBridge();const receipt=await prepareNative();await click('Authorize in desktop window');let resolve!:(value:unknown)=>void;
 call.mockImplementation((path:string)=>path.startsWith('/workbuddy-connections/')?new Promise(done=>{resolve=done;}):Promise.resolve(page([receipt.connection])));
 const before=call.mock.calls.length;await act(async()=>{window.dispatchEvent(new Event('focus'));window.dispatchEvent(new Event('focus'));});
 const reads=call.mock.calls.slice(before).filter(([path])=>path==='/workbuddy-connections/connection-a?project_id=p-a');expect(reads).toHaveLength(1);
 await act(async()=>resolve({...row,id:'connection-other',version:5,state:'connected'}));expect(host.textContent).not.toContain('WorkBuddy account authorization was saved');expect(host.textContent).toContain('does not match');
});
