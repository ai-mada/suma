// @vitest-environment jsdom
import {act,useState} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {TaskViews} from './TaskViews';
import {defaultTaskView} from './task-views-model';
import {readTaskViews,writeTaskViews,taskViewsKey,type TaskViewScope,type TaskViewPreferences} from './task-views-storage';
import {setLocale} from './i18n';
import type {RecordItem} from './api';
import type {Iteration} from './planning-model';
let host:HTMLDivElement,root:Root;
const scope:TaskViewScope=['user-a','org-a','project-a'];
const rows=[{id:'task-a',project_id:'project-a',title:'Original <script> task',version:1,status:'todo',priority:'high',owner_id:'member-outside-first-page',created_at:'2026-10-29T00:00:00Z'},{id:'task-b',project_id:'project-a',title:'Review task',version:1,status:'all',priority:'custom',created_at:'2026-10-29T00:00:00Z'}] as RecordItem[];
const changed=vi.fn(),open=vi.fn();
function Harness({initial=defaultTaskView,identity=scope,iterations=[]}:{initial?:TaskViewPreferences;identity?:TaskViewScope;iterations?:Iteration[]}){const [view,setView]=useState(initial);return <TaskViews scope={identity} preferences={view} onChange={next=>{changed(next);setView(next);return true;}} page={{items:rows,task_plans:rows.map(row=>({task_id:row.id,iteration_id:'',plan_version:0})),total:205,next_cursor:'next',as_of:'2026-10-29T00:00:00Z',loading:false,error:null,action:'first',reload:async()=>{},more:async()=>{}}} members={[]} iterations={iterations} moreMembers lookupBusy={false} lookupError={undefined} onMoreMembers={()=>{}} selected="" onSelect={open} selection={[]} onToggle={()=>{}} onCreate={()=>{}} onDependencies={()=>{}} onArrange={()=>{}}/>;}
beforeEach(async()=>{await setLocale('en-US',{persist:false});localStorage.clear();changed.mockClear();open.mockClear();vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();});
async function render(node=<Harness/>){await act(async()=>root.render(node));}
async function click(text:string,container:ParentNode=document){const target=[...container.querySelectorAll('button')].find(button=>button.textContent?.trim()===text);expect(target,text).toBeTruthy();await act(async()=>target!.click());}
async function change(target:HTMLInputElement|HTMLSelectElement,value:string){await act(async()=>{const proto=target instanceof HTMLInputElement?HTMLInputElement.prototype:HTMLSelectElement.prototype;Object.getOwnPropertyDescriptor(proto,'value')!.set!.call(target,value);target.dispatchEvent(new Event(target instanceof HTMLInputElement?'input':'change',{bubbles:true}));});}
it('projects identical task IDs in list and board, exposes loaded counts, and retains unknown assignee IDs',async()=>{
 await render(<Harness initial={{...defaultTaskView,owner_id:'member-outside-first-page'}}/>);
 expect(host.textContent).toContain('Loaded 2 of 205');expect(host.textContent).toContain('member-outside-first-page');
 expect(host.querySelectorAll('[data-task-id]')).toHaveLength(2);expect(host.querySelector('script')).toBeNull();
 await click('Board view',host);expect(host.querySelectorAll('[data-task-id]')).toHaveLength(2);
 expect(host.querySelector('.task-views-card')?.textContent).toContain('Original <script> task');
 const group=[...host.querySelectorAll('select')].find(select=>select.parentElement?.textContent?.startsWith('Group by'))!;
 await change(group,'owner');expect(host.querySelectorAll('[data-task-id]')).toHaveLength(2);
 await click('Original <script> task',host);expect(open).toHaveBeenCalledWith('task-a');
});
it('saves only preferences, restores custom status, and requires confirmation before deletion',async()=>{
 await render(<Harness initial={{...defaultTaskView,status:'all'}}/>);await click('Save view',host);
 const modal=document.querySelector('[role=dialog]')!;await change(modal.querySelector('input')!,'My <b> view');await click('Save',modal);
 const saved=readTaskViews(localStorage,scope);expect(saved.collection?.views[0].preferences.status).toBe('all');expect(saved.raw).not.toContain('Original');
 const remove=[...host.querySelectorAll<HTMLButtonElement>('button[aria-label]')].find(button=>button.getAttribute('aria-label')?.startsWith('Delete view'))!;expect(remove,host.innerHTML).toBeTruthy();await act(async()=>remove.click());
 expect(readTaskViews(localStorage,scope).collection?.views).toHaveLength(1);await click('Cancel',document.querySelector('[role=dialog]')!);
 await act(async()=>remove.click());await click('Delete view',document.querySelector('[role=dialog]')!);
 expect(readTaskViews(localStorage,scope).collection?.views).toHaveLength(0);expect(changed).not.toHaveBeenCalled();
});
it('keeps an in-progress save name and current filters when another tab changes the collection',async()=>{
 await render();await click('Save view',host);let modal=document.querySelector('[role=dialog]')!;await change(modal.querySelector('input')!,'Unfinished name');
 writeTaskViews(localStorage,scope,[{id:'other',name:'Other tab',preferences:{...defaultTaskView,status:'accepted'}}],null);
 await act(async()=>window.dispatchEvent(new StorageEvent('storage',{key:taskViewsKey(scope),newValue:localStorage.getItem(taskViewsKey(scope))})));
 expect(modal.textContent).toContain('Another tab changed');expect(changed).not.toHaveBeenCalled();await click('Load latest saved views',modal);
 expect((modal.querySelector('input') as HTMLInputElement).value).toBe('Unfinished name');await click('Save',modal);
 expect(readTaskViews(localStorage,scope).collection?.views.map(view=>view.name)).toEqual(['Other tab','Unfinished name']);
});
it('shows corruption without deleting the original and isolates another account',async()=>{
 localStorage.setItem(taskViewsKey(scope),'broken');await render();expect(host.textContent).toContain('original data is retained');expect(localStorage.getItem(taskViewsKey(scope))).toBe('broken');
 await render(<Harness identity={['user-b','org-a','project-a']}/>);expect(host.textContent).not.toContain('Saved views are invalid');
 expect(localStorage.getItem(taskViewsKey(scope))).toBe('broken');
});

it('shows the original numeric saved-view envelope as invalid without applying or overwriting its owner meaning',async()=>{
 const original=JSON.stringify({version:1,views:[{id:'original-view',name:'Original unassigned view',preferences:{...defaultTaskView,owner_id:'__unassigned__'}}]});localStorage.setItem(taskViewsKey(scope),original);await render();expect(host.textContent).toContain('original data is retained');expect(host.querySelector('.task-views-saved select')?.textContent).not.toContain('Original unassigned view');expect(localStorage.getItem(taskViewsKey(scope))).toBe(original);expect(changed).not.toHaveBeenCalled();
});


it('offers one iteration filter with completed and unknown exact choices and saves it with iteration grouping',async()=>{
 const iterations=[{id:'done',version:2,title:'Delivery Q4',status:'completed',description:'',start_date:'',end_date:'',goal:''},{id:'unset',version:1,title:'Literal unset iteration',status:'active',description:'',start_date:'',end_date:'',goal:''}];
 await render(<Harness initial={{...defaultTaskView,iteration_id:'missing'}} iterations={iterations}/>);
 expect(host.querySelectorAll('[aria-labelledby=task-filter-iteration]')).toHaveLength(1);const select=host.querySelector<HTMLSelectElement>('[aria-labelledby=task-filter-iteration]')!;expect(select.value).toBe('value:missing');expect(select.selectedOptions[0].textContent).toBe('missing · Needs review');expect(select.textContent).toContain('Delivery Q4 · Completed');
 await change(select,'value:unset');expect(changed).toHaveBeenLastCalledWith(expect.objectContaining({iteration_id:'unset',iteration_unset:undefined}));await change(select,'unset');expect(changed).toHaveBeenLastCalledWith(expect.objectContaining({iteration_id:undefined,iteration_unset:true}));await change(select,'value:done');
 await change(host.querySelector<HTMLSelectElement>('[aria-labelledby=task-filter-group_by]')!,'iteration');await click('Save view',host);const modal=document.querySelector('[role=dialog]')!;await change(modal.querySelector('input')!,'Completed iteration lane');await click('Save',modal);
 const saved=readTaskViews(localStorage,scope).collection?.views[0].preferences;expect(saved).toMatchObject({iteration_id:'done',group_by:'iteration'});expect(saved?.iteration_unset).toBeUndefined();
});
