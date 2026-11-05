// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {beforeEach,afterEach,it,expect,vi} from 'vitest';
import {SharedTaskViews,type SharedTaskView,sharedViewPreferences} from './SharedTaskViews';
import {defaultTaskView} from './task-views-model';
import {ApiError} from './api';
import {setLocale} from './i18n';
import {sharedTaskViewRecoveryKey} from './shared-task-view-recovery';
const {call,syncCall}=vi.hoisted(()=>({call:vi.fn(),syncCall:vi.fn()}));
vi.mock('./api',async original=>({...await original<typeof import('./api')>(),api:(path:string,...args:unknown[])=>(path.startsWith('/task-views/sync?')?syncCall:call)(path,...args)}));
let host:HTMLDivElement,root:Root;const change=vi.fn((_value:typeof defaultTaskView)=>true);
const row:SharedTaskView={id:'view-a',project_id:'project-a',owner_id:'user-a',name:'Team backlog',visibility:'project',version:1,state:'active',preferences:{...defaultTaskView,status:'todo'},created_at:'2026-10-29T00:00:00Z',updated_at:'2026-10-29T00:00:00Z',can_edit:true,can_delete:true};
const page=(items=[row])=>({project_id:'project-a',items,total:items.length,limit:20,as_of:'now'});
beforeEach(async()=>{sessionStorage.clear();vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);await setLocale('en-US',{persist:false});call.mockReset();syncCall.mockReset();syncCall.mockResolvedValue(revision());change.mockClear();call.mockResolvedValue(page());host=document.createElement('div');document.body.append(host);root=createRoot(host);});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();sessionStorage.clear();vi.useRealTimers();vi.restoreAllMocks();vi.unstubAllGlobals();});
async function render(user='user-a',denied=false,preferences=defaultTaskView){await act(async()=>root.render(<SharedTaskViews scope={[user,'org-a','project-a']} preferences={preferences} onChange={change} denied={denied}/>));}
async function click(text:string){const b=[...document.querySelectorAll('button')].find(b=>b.textContent?.trim()===text);expect(b,text).toBeTruthy();await act(async()=>b!.click());}
async function input(el:HTMLInputElement|HTMLSelectElement,value:string){await act(async()=>{const proto=el instanceof HTMLInputElement?HTMLInputElement.prototype:HTMLSelectElement.prototype;Object.getOwnPropertyDescriptor(proto,'value')!.set!.call(el,value);el.dispatchEvent(new Event(el instanceof HTMLInputElement?'input':'change',{bubbles:true}));});}
const dialog=()=>document.querySelector('[role=dialog]')!;
const writes=()=>call.mock.calls.filter(request=>request[2]==='POST'||request[2]==='PATCH');
const recoveryKey=sharedTaskViewRecoveryKey(['user-a','org-a','project-a']);
async function remount(user='user-a',preferences=defaultTaskView){await act(async()=>root.unmount());root=createRoot(host);await render(user,false,preferences);}
function receipt(body:Record<string,unknown>,overrides:Partial<SharedTaskView>={}):SharedTaskView{
 return {...row,version:(Number(body.expected_version)||0)+1,name:body.name as string,visibility:body.visibility as SharedTaskView['visibility'],preferences:body.preferences as SharedTaskView['preferences'],...overrides};
}
it('selects formal preferences without granting task access and keeps visibility immutable during editing',async()=>{await render();await input(host.querySelector('select')!,'view-a');expect(change).toHaveBeenCalledWith(expect.objectContaining({status:'todo'}));await click('Update view');expect((dialog().querySelector('.field select') as HTMLSelectElement).disabled).toBe(true);expect(host.textContent).toContain('Revision 1');});
it('projects only filter preferences and never uploads route or task data',()=>{const p=sharedViewPreferences({...defaultTaskView,record:'secret',panel:'edit'} as typeof defaultTaskView);expect(p).not.toHaveProperty('record');expect(p).not.toHaveProperty('panel');});
it('retains a failed save draft and retries an unknown outcome with identical operation and payload',async()=>{await render();await click('Save synced view');await input(dialog().querySelector('input')!,'My queue');call.mockRejectedValueOnce(new ApiError(0,'timeout','Timeout'));await click('Save');const first=call.mock.calls.at(-1)!;expect(dialog().textContent).toContain('not confirmed');expect((dialog().querySelector('input') as HTMLInputElement).value).toBe('My queue');call.mockResolvedValueOnce(receipt(first[1]));await click('Check by retrying');const retry=writes().at(-1)!;expect(retry[0]).toBe(first[0]);expect(retry[1]).toEqual(first[1]);expect(retry[3]).toEqual({'X-Suma-Expected-User':'user-a','X-Suma-Expected-Organization':'org-a'});expect(host.textContent).toContain('Synced view saved');});
it('keeps conflict drafts and creates a new private copy only after explicit action',async()=>{await render();await input(host.querySelector('select')!,'view-a');await click('Update view');await input(dialog().querySelector('input')!,'Draft kept');call.mockRejectedValueOnce(new ApiError(409,'version_conflict','Changed'));await click('Save');expect((dialog().querySelector('input') as HTMLInputElement).value).toBe('Draft kept');const b=[...dialog().querySelectorAll('button')].find(b=>b.textContent==='Save a copy')!;await act(async()=>b.click());call.mockResolvedValueOnce({...row,id:'copy',name:'Draft kept',visibility:'private',preferences:defaultTaskView});await click('Save');expect(call.mock.calls.at(-1)![0]).toBe('/task-views');expect(call.mock.calls.at(-1)![1]).toMatchObject({visibility:'private',name:'Draft kept'});expect(call.mock.calls.at(-1)![1]).not.toHaveProperty('expected_version');});
it('clears server names and pending editor on current authority loss',async()=>{await render();await input(host.querySelector('select')!,'view-a');await click('Update view');await render('user-a',true);expect(host.textContent).not.toContain('Team backlog');expect(document.querySelector('[role=dialog]')).toBeNull();expect(host.textContent).toContain('Access changed');});
it('ignores a previous identity response after changing account',async()=>{let resolve!:(value:unknown)=>void;call.mockImplementationOnce(()=>new Promise(r=>{resolve=r;}));await render();call.mockResolvedValue(page([]));await render('user-b');await act(async()=>resolve(page()));expect(host.textContent).not.toContain('Team backlog');});
it('requires explicit archive confirmation and leaves current filters unchanged',async()=>{await render();await input(host.querySelector('select')!,'view-a');change.mockClear();await click('Delete synced view');expect(call.mock.calls).toHaveLength(1);const b=[...dialog().querySelectorAll('button')].find(b=>b.textContent==='Delete synced view')!;call.mockResolvedValueOnce({...row,state:'archived',version:2,can_edit:false,can_delete:false});await act(async()=>b.click());expect(call.mock.calls.at(-1)![0]).toBe('/task-views/view-a/archive');expect(call.mock.calls.at(-1)![1]).toMatchObject({expected_version:1});expect(change).not.toHaveBeenCalled();expect(host.textContent).not.toContain('Team backlog');});

it('compares the latest server revision with the retained draft before an explicit CAS replacement',async()=>{await render();await input(host.querySelector('select')!,'view-a');await click('Update view');await input(dialog().querySelector('input')!,'My retained draft');call.mockRejectedValueOnce(new ApiError(409,'version_conflict','Changed'));await click('Save');call.mockResolvedValueOnce({...row,name:'Changed elsewhere',version:3,preferences:{...defaultTaskView,status:'blocked'}});await click('Load latest revision');expect(dialog().textContent).toContain('Changed elsewhere');expect(dialog().textContent).toContain('My retained draft');expect(dialog().textContent).toContain('Blocked');call.mockResolvedValueOnce({...row,name:'My retained draft',version:4,preferences:defaultTaskView});await click('Update current revision with my draft');expect(call.mock.calls.at(-1)![1]).toMatchObject({expected_version:3,name:'My retained draft'});});


it('loads server-owned iteration criteria independently of local views and preserves exact IDs',async()=>{
 const preferences={...defaultTaskView,iteration_id:'completed-iteration',group_by:'iteration' as const};call.mockResolvedValueOnce(page([{...row,preferences}]));await render();await input(host.querySelector('select')!,'view-a');expect(change).toHaveBeenLastCalledWith(expect.objectContaining({iteration_id:'completed-iteration',group_by:'iteration'}));expect(change.mock.calls.at(-1)?.[0]).not.toHaveProperty('iteration_unset',true);
});

it('keeps iteration criteria and the original CAS operation through unknown save recovery after current filters change',async()=>{
 const preferences={...defaultTaskView,iteration_id:'completed-iteration',group_by:'iteration' as const};await render('user-a',false,preferences);await input(host.querySelector('select')!,'view-a');await click('Update view');await input(dialog().querySelector('input')!,'Iteration view draft');call.mockRejectedValueOnce(new ApiError(0,'timeout','Timeout'));await click('Save');const original=call.mock.calls.at(-1)!;expect(original[1]).toMatchObject({expected_version:1,preferences:{iteration_id:'completed-iteration',group_by:'iteration'}});
 await render('user-a',false,{...defaultTaskView,iteration_unset:true});expect(dialog().textContent).toContain('completed-iteration');call.mockResolvedValueOnce({...row,name:'Iteration view draft',version:2,preferences});await click('Check by retrying');expect(writes().at(-1)?.[1]).toEqual(original[1]);expect(writes().at(-1)?.[1].preferences).not.toHaveProperty('iteration_unset',true);
});

it('refuses explicit invalid response columns instead of applying a default view or losing the current filter',async()=>{
 const invalidWire:unknown={...page(),items:[{...row,preferences:{...defaultTaskView,status:'todo',columns:null}}]};call.mockResolvedValue(await Response.json(invalidWire).json());await render('user-a',false,{...defaultTaskView,status:'blocked'});expect(host.querySelector('[role=alert]')).not.toBeNull();expect([...host.querySelectorAll('option')].some(option=>option.value==='view-a')).toBe(false);expect(change).not.toHaveBeenCalled();
});
it('clones exact columns into the original unknown operation and retries the same body after the current display changes',async()=>{
 const columns=['title','updated','owner'];await render('user-a',false,{...defaultTaskView,status:'todo',columns});await click('Save synced view');await input(dialog().querySelector('input')!,'Column view');call.mockResolvedValueOnce({...row,preferences:{...defaultTaskView,status:'todo',columns:[]}});await click('Save');const first=call.mock.calls.at(-1)!;expect(first[1]).toMatchObject({preferences:{status:'todo',columns:['title','updated','owner']}});columns.splice(1,1);await render('user-a',false,{...defaultTaskView,status:'blocked',columns:['title']});expect(first[1].preferences.columns).toEqual(['title','updated','owner']);call.mockResolvedValueOnce(receipt(first[1]));await click('Check by retrying');const retry=writes().at(-1)!;expect(retry[1]).toEqual(first[1]);expect(retry[1].idempotency_key).toBe(first[1].idempotency_key);expect(change).not.toHaveBeenCalled();
});

const revision=(digit='a')=>({project_id:'project-a',revision:digit.repeat(64)});
const syncState=()=>host.querySelector('[data-shared-view-sync-state]')?.getAttribute('data-shared-view-sync-state');
const focus=async()=>{await act(async()=>window.dispatchEvent(new Event('focus')));};
const selectView=async(id=row.id)=>{await input(host.querySelector('.shared-task-views-picker select')!,id);};

it('checks the directory only when its scoped marker changes and keeps the applied revision until explicit review',async()=>{
 vi.useFakeTimers();
 await render();await selectView();
 expect(syncCall.mock.calls[0]?.[0]).toBe('/task-views/sync?project_id=project-a');
 expect(syncCall.mock.invocationCallOrder[0]).toBeLessThan(call.mock.invocationCallOrder[0]!);
 expect(syncState()).toBe('current');
 await act(async()=>vi.advanceTimersByTimeAsync(30000));
 expect(syncCall).toHaveBeenCalledTimes(3);expect(call).toHaveBeenCalledTimes(1);
 const latest={...row,version:2,name:'Team in progress',preferences:{...row.preferences,status:'in_progress'}};
 syncCall.mockResolvedValue(revision('b'));call.mockResolvedValue(page([latest]));
 await focus();
 expect(host.querySelector('[data-shared-view-update-notice]')?.textContent).toContain('Revision 2 is available');
 expect(host.querySelector('.shared-task-views-meta')?.textContent).toContain('Revision 1');
 expect(change).toHaveBeenCalledTimes(1);
 expect(host.querySelector('[data-shared-view-update-notice]')?.textContent).toContain('In progress');
 syncCall.mockRejectedValueOnce(new ApiError(0,'timeout','Transport interrupted'));
 await click('Refresh views');
 expect(syncState()).toBe('stale');
 expect((host.querySelector('[data-shared-view-apply-update]') as HTMLButtonElement).disabled).toBe(true);
 expect(host.textContent).toContain('Transport interrupted');
 await focus();
 expect(syncState()).toBe('current');
 expect(host.textContent).not.toContain('Transport interrupted');
 await click('Apply new revision');
 expect(change).toHaveBeenLastCalledWith(expect.objectContaining({status:'in_progress'}));
 expect(host.querySelector('[data-shared-view-update-notice]')).toBeNull();
 expect(host.querySelector('.shared-task-views-meta')?.textContent).toContain('Revision 2');
});

it('serializes wakeups and rechecks the directory after a hidden or offline period',async()=>{
 vi.useFakeTimers();
 const visible=vi.spyOn(document,'visibilityState','get').mockReturnValue('visible');
 const online=vi.spyOn(window.navigator,'onLine','get').mockReturnValue(true);
 await render();
 let complete!:(value:unknown)=>void;
 syncCall.mockImplementationOnce(()=>new Promise(resolve=>{complete=resolve;}));
 await focus();await focus();
 expect(syncCall).toHaveBeenCalledTimes(2);
 await act(async()=>complete(revision()));
 visible.mockReturnValue('hidden');
 await act(async()=>document.dispatchEvent(new Event('visibilitychange')));
 await act(async()=>vi.advanceTimersByTimeAsync(45000));await focus();
 expect(syncCall).toHaveBeenCalledTimes(2);
 online.mockReturnValue(false);visible.mockReturnValue('visible');
 await act(async()=>window.dispatchEvent(new Event('offline')));
 await act(async()=>vi.advanceTimersByTimeAsync(45000));
 expect(syncState()).toBe('offline');expect(syncCall).toHaveBeenCalledTimes(2);
 online.mockReturnValue(true);
 await act(async()=>window.dispatchEvent(new Event('online')));
 expect(syncState()).toBe('current');expect(syncCall).toHaveBeenCalledTimes(3);
 expect(call).toHaveBeenCalledTimes(1);
});

it('checks the selected view beyond the first page and isolates a missing view from the current project',async()=>{
 const paged={...row,id:'view-next',name:'Another saved view',preferences:{...defaultTaskView,status:'blocked'}};
 call.mockResolvedValueOnce({...page(),next_cursor:'next'}).mockResolvedValueOnce(page([paged]));
 await render();await click('Load more views');await selectView(paged.id);change.mockClear();
 syncCall.mockResolvedValue(revision('b'));
 call.mockResolvedValueOnce({...page(),next_cursor:'next'}).mockRejectedValueOnce(new ApiError(404,'not_found','Missing'));
 await focus();
 expect(call.mock.calls.at(-1)?.[0]).toBe('/task-views/view-next?project_id=project-a');
 expect(host.textContent).toContain('Team backlog');
 expect(host.textContent).not.toContain('Another saved view');
 expect(host.querySelector('[data-shared-view-unavailable]')?.textContent).toContain('current filters are still active');
 expect(change).not.toHaveBeenCalled();expect(syncState()).toBe('current');
});

it('keeps the editor draft on remote revision changes and removes an archived view without changing filters',async()=>{
 await render();await selectView();await click('Update view');
 await input(dialog().querySelector('input')!,'My local revision');
 syncCall.mockResolvedValue(revision('b'));
 const latest={...row,version:2,name:'Latest team view',preferences:{...row.preferences,status:'blocked'}};
 call.mockResolvedValue(page([latest]));
 await focus();
 expect(dialog().textContent).toContain('Latest team view');
 expect(dialog().textContent).toContain('Blocked');
 expect((dialog().querySelector('input') as HTMLInputElement).value).toBe('My local revision');
 expect((dialog().querySelector('button:not([type])') as HTMLButtonElement).disabled).toBe(true);
 expect(change).toHaveBeenCalledTimes(1);
 syncCall.mockResolvedValue(revision('c'));
 call.mockResolvedValueOnce(page([])).mockResolvedValueOnce({...latest,state:'archived',version:3});
 await focus();
 expect(host.querySelector('[data-shared-view-unavailable]')).not.toBeNull();
 expect(host.textContent).not.toContain('Latest team view');
 expect(dialog().textContent).toContain('personal copy');
 expect((dialog().querySelector('input') as HTMLInputElement).value).toBe('My local revision');
 expect(change).toHaveBeenCalledTimes(1);
});

it('marks a failed marker stale and clears private material when live authority is revoked',async()=>{
 await render();await selectView();await click('Update view');await input(dialog().querySelector('input')!,'Private unsaved name');
 syncCall.mockResolvedValueOnce({project_id:'wrong-project',revision:'b'.repeat(64)});
 await focus();
 expect(syncState()).toBe('stale');expect(host.textContent).toContain('Team backlog');
 expect(call).toHaveBeenCalledTimes(1);
 syncCall.mockRejectedValueOnce(new ApiError(403,'forbidden','Forbidden'));
 await focus();
 expect(syncState()).toBe('denied');expect(host.textContent).not.toContain('Team backlog');
 expect(document.querySelector('[role=dialog]')).toBeNull();
 expect(change).toHaveBeenCalledTimes(1);
 const reads=syncCall.mock.calls.length;await focus();expect(syncCall).toHaveBeenCalledTimes(reads);
 syncCall.mockResolvedValue(revision('b'));await click('Refresh views');
 expect(syncState()).toBe('current');expect(host.textContent).toContain('Team backlog');
});

it('updates same-revision edit rights and keeps a personal draft copy without writing the shared view',async()=>{
 await render();await selectView();await click('Update view');await input(dialog().querySelector('input')!,'Local draft after role change');
 syncCall.mockResolvedValue(revision('b'));call.mockResolvedValue(page([{...row,can_edit:false,can_delete:false}]));
 await focus();
 expect(dialog().textContent).toContain('Editing access changed');
 expect((dialog().querySelector('button:not([type])') as HTMLButtonElement).disabled).toBe(true);
 await act(async()=>dialog().querySelector('form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));
 expect(call.mock.calls.some(request=>request[2]==='PATCH')).toBe(false);
 const copy=[...dialog().querySelectorAll('button')].find(button=>button.textContent==='Save a copy')!;
 await act(async()=>copy.click());
 expect((dialog().querySelector('input') as HTMLInputElement).value).toBe('Local draft after role change');
 expect((dialog().querySelector('.field select') as HTMLSelectElement).value).toBe('private');
 expect((dialog().querySelector('button:not([type])') as HTMLButtonElement).disabled).toBe(false);
});

it('persists the request before sending, restores it on remount, and treats the old receipt as history',async()=>{
 await render();await click('Save synced view');await input(dialog().querySelector('input')!,'Original name');
 let submitted!:Record<string,unknown>;
 call.mockImplementationOnce((_path,body)=>{
  submitted=body;
  expect(JSON.parse(sessionStorage.getItem(recoveryKey)!).command.body).toEqual(JSON.parse(JSON.stringify(body)));
  return new Promise(()=>{});
 });
 await click('Save');expect(writes()).toHaveLength(1);
 const newer={...row,id:'saved-once',version:2,name:'Name from another device'};
 call.mockResolvedValue(page([newer]));
 await remount('user-a',{...defaultTaskView,status:'blocked'});
 expect(host.querySelector('[data-shared-view-recovery]')).not.toBeNull();
 expect(writes()).toHaveLength(1);
 expect(host.textContent).not.toContain('No synced views yet');
 expect(document.querySelector('[role=dialog]')).toBeNull();
 call.mockResolvedValueOnce(receipt(submitted,{id:newer.id,can_edit:false,can_delete:false}));
 await click('Check by retrying');
 expect(writes()).toHaveLength(2);expect(writes().at(-1)?.[1]).toEqual(JSON.parse(JSON.stringify(submitted)));
 expect(sessionStorage.getItem(recoveryKey)).toBeNull();
 expect(host.textContent).toContain(newer.name);
 expect(host.textContent).not.toContain('Original name');
 expect(host.textContent).toContain('Synced view saved');
 expect(change).not.toHaveBeenCalled();
});

it('keeps a scoped unknown request out of another account and clears it on confirmed project denial',async()=>{
 await render();await click('Save synced view');await input(dialog().querySelector('input')!,'Private request');
 call.mockRejectedValueOnce(new ApiError(0,'network_error','Lost response'));await click('Save');
 await click('Check later');
 expect(host.querySelector('[data-shared-view-recovery]')).not.toBeNull();
 expect(document.querySelector('[role=dialog]')).toBeNull();
 await remount('user-b');
 expect(host.querySelector('[data-shared-view-recovery]')).toBeNull();
 expect(sessionStorage.getItem(recoveryKey)).not.toBeNull();
 syncCall.mockRejectedValueOnce(new ApiError(403,'forbidden','Forbidden'));
 await remount();
 expect(host.querySelector('[data-shared-view-recovery]')).toBeNull();
 expect(sessionStorage.getItem(recoveryKey)).toBeNull();
 expect(writes()).toHaveLength(1);
});

it('blocks the request if browser storage rejects the original record',async()=>{
 await render();await click('Save synced view');await input(dialog().querySelector('input')!,'Unsaved view');
 vi.spyOn(Storage.prototype,'setItem').mockImplementation(()=>{throw new DOMException('Quota','QuotaExceededError');});
 await click('Save');
 expect(writes()).toHaveLength(0);
 expect(dialog().textContent).toContain('couldn’t access');
 expect((dialog().querySelector('input') as HTMLInputElement).value).toBe('Unsaved view');
});

it('resolves a superseded replay without claiming it never saved and offers a draft without writing',async()=>{
 await render();await selectView();await click('Update view');await input(dialog().querySelector('input')!,'Recovered draft');
 call.mockRejectedValueOnce(new ApiError(0,'network_error','Lost response'));await click('Save');
 const original=writes()[0];
 await remount();
 call.mockRejectedValueOnce(new ApiError(409,'version_conflict','Changed'));
 await click('Check by retrying');
 expect(writes()[1][1]).toEqual(JSON.parse(JSON.stringify(original[1])));
 expect(sessionStorage.getItem(recoveryKey)).toBeNull();
 expect(host.textContent).toContain('original request could not be confirmed');
 await click('Open as a personal draft');
 expect((dialog().querySelector('input') as HTMLInputElement).value).toBe('Recovered draft');
 expect((dialog().querySelector('.field select') as HTMLSelectElement).value).toBe('private');
 expect(writes()).toHaveLength(2);
});

it('keeps readable views after a restored write is refused and project read still succeeds',async()=>{
 await render();await click('Save synced view');await input(dialog().querySelector('input')!,'Original request');
 call.mockRejectedValueOnce(new ApiError(0,'network_error','Lost response'));await click('Save');
 await remount();
 call.mockRejectedValueOnce(new ApiError(403,'forbidden','Editing is restricted'));
 await click('Check by retrying');
 expect(host.textContent).toContain('Team backlog');
 expect(host.textContent).toContain('original request could not be confirmed');
 expect(sessionStorage.getItem(recoveryKey)).toBeNull();
 expect(writes()).toHaveLength(2);
});
