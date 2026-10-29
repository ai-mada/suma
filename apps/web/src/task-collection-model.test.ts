import {expect,it,vi} from 'vitest';
import {appendTaskCollectionPage,reconcileTaskCollectionPages,TaskCollectionReadQueue,taskCollectionColumnScope,taskCollectionPath,taskCollectionScopeKey,validTaskCollectionFilter,validTaskCollectionPage,validTaskCollectionSummary,type TaskCollectionPageView} from './task-collection-model';

const scope={project:'project-a',userID:'user-a',organizationID:'org-a',q:'',status:'all',layout:'list'};
const task=(patch:Record<string,unknown>={})=>({id:'task-a',project_id:'project-a',version:1,title:'Current task',status:'todo',owner_id:'human-a',priority:'high',requirement_id:'req-a',updated_at:'2026-10-29T02:00:00Z',...patch});
const page=(patch:Record<string,unknown>={})=>({project_id:'project-a',as_of:'2026-10-29T03:00:00Z',items:[task()],total:1,limit:20,next_cursor:'',...patch});

it('uses literal full-scope filters, and distinguishes all tasks from the literal all state',()=>{
 const filtered={...scope,q:' 50%_中 + ',task_owner:'human-a',task_priority:'high',task_requirement:'req-a'};
 const summary=new URLSearchParams(taskCollectionPath(filtered,'summary','draft','ignored').split('?')[1]);expect(summary.get('q')).toBe(' 50%_中 + ');expect(summary.get('owner_id')).toBe('human-a');expect(summary.get('priority')).toBe('high');expect(summary.get('requirement_id')).toBe('req-a');expect(summary.has('status')).toBe(false);expect(summary.has('limit')).toBe(false);expect(summary.has('cursor')).toBe(false);
 expect(new URLSearchParams(taskCollectionPath(scope,'items').split('?')[1]).has('status')).toBe(false);expect(new URLSearchParams(taskCollectionPath(scope,'items','all','opaque+/=').split('?')[1]).get('status')).toBe('all');
 expect(new URLSearchParams(taskCollectionPath({...scope,task_unassigned:true},'items').split('?')[1]).get('unassigned')).toBe('true');
 expect(validTaskCollectionFilter({...scope,q:'😀'.repeat(200)})).toBe(true);expect(validTaskCollectionFilter({...scope,q:'😀'.repeat(201)})).toBe(false);expect(validTaskCollectionFilter({...scope,task_owner:'human-a',task_unassigned:true})).toBe(false);expect(validTaskCollectionFilter({...scope,task_status:'all',task_priority:'archived'})).toBe(true);
 expect(taskCollectionScopeKey(scope)).not.toBe(taskCollectionScopeKey({...scope,task_status:'all'}));expect(taskCollectionScopeKey(scope)).not.toBe(taskCollectionScopeKey({...scope,organizationID:'org-b'}));
});

it('validates real full-filter summary counts while preserving draft and custom groups',()=>{
 const summary={project_id:'project-a',group_by:'status',groups_total:3,groups_limit:20,next_group_cursor:'',as_of:'2026-10-29T03:00:00Z',total:4,groups:[{key:'draft',total:1},{key:'waiting_for_customer',total:2},{key:'all',total:1}]};expect(validTaskCollectionSummary(summary,'project-a')).toBe(true);
 expect(validTaskCollectionSummary({...summary,total:5},'project-a')).toBe(false);expect(validTaskCollectionSummary(summary,'project-b')).toBe(false);expect(validTaskCollectionSummary({...summary,groups:[{key:'bad\nstate',total:4}]},'project-a')).toBe(false);
});

it('rejects wrong project, owner, requirement, priority and status page facts',()=>{
 const filtered={...scope,task_owner:'human-a',task_priority:'high',task_requirement:'req-a'};expect(validTaskCollectionPage(page(),filtered,'todo')).toBe(true);
 for(const patch of [{project_id:'project-b'},{items:[task({owner_id:'other'})]},{items:[task({priority:'low'})]},{items:[task({requirement_id:'other'})]},{items:[task({status:'draft'})]},{items:[task(),task()]},{items:[task({status:'bad\nstate'})]}])expect(validTaskCollectionPage(page(patch),filtered,'todo')).toBe(false);
 expect(validTaskCollectionPage(page({items:[task({status:'all'})]}),scope,'all')).toBe(true);expect(validTaskCollectionPage(page(),scope,'all')).toBe(false);expect(validTaskCollectionPage(page({items:[task({owner_id:''})]}),{...scope,task_unassigned:true},'')).toBe(true);
});

it('retains the higher record version across keyset pages and refuses shrinking live totals',()=>{
 const before=page({items:[task({version:3})],total:2}) as unknown as TaskCollectionPageView,next=page({items:[task({version:2}),task({id:'task-b'})],total:2}) as unknown as TaskCollectionPageView;
 expect(appendTaskCollectionPage(before,next)?.items[0].version).toBe(3);expect(appendTaskCollectionPage(before,{...next,total:1})).toBeUndefined();
});

it('shows a moved task once at its highest loaded version and marks both live columns changed',()=>{
 const groups=[{key:'todo'},{key:'in_progress'}],pages={todo:{page:{items:[task({version:2})]}},in_progress:{page:{items:[task({version:3,status:'in_progress'})]}}};
 const reconciled=reconcileTaskCollectionPages(groups,pages);expect(reconciled.rows).toHaveLength(1);expect(reconciled.rows[0].version).toBe(3);expect(reconciled.rowsByGroup.get('todo')).toEqual([]);expect([...reconciled.changed].sort()).toEqual(['in_progress','todo']);
 const ambiguous=reconcileTaskCollectionPages(groups,{...pages,todo:{page:{items:[task({version:3})]}}});expect(ambiguous.rows).toEqual([]);expect(ambiguous.changed.size).toBe(2);
});

it('bounds concurrent requests and never sends an aborted queued request',async()=>{
 const queue=new TaskCollectionReadQueue(2),controllers=Array.from({length:3},()=>new AbortController()),release:(()=>void)[]=[],started=vi.fn();
 const jobs=controllers.map((controller,index)=>queue.run(()=>new Promise<number>(resolve=>{started(index);release.push(()=>resolve(index));}),controller.signal).catch(error=>error));
 await Promise.resolve();expect(started).toHaveBeenCalledTimes(2);controllers[2].abort();release.forEach(done=>done());await Promise.all(jobs);expect(started.mock.calls.map(row=>row[0])).toEqual([0,1]);
});

it('keeps global criteria and sort when paging owner, priority or empty-status groups',()=>{
 const base={...scope,task_status:'draft',task_sort:'title_asc',task_requirement:'req-a'};
 const owner=new URLSearchParams(taskCollectionPath(taskCollectionColumnScope({...base,task_group:'owner'},''),'items').split('?')[1]);expect(owner.get('unassigned')).toBe('true');expect(owner.has('owner_id')).toBe(false);expect(owner.get('status')).toBe('draft');expect(owner.get('sort')).toBe('title_asc');expect(owner.get('requirement_id')).toBe('req-a');
 const priority=new URLSearchParams(taskCollectionPath(taskCollectionColumnScope({...base,task_group:'priority'},''),'items').split('?')[1]);expect(priority.get('priority_unset')).toBe('true');expect(priority.has('priority')).toBe(false);
 const emptyStatus=new URLSearchParams(taskCollectionPath(taskCollectionColumnScope({...base,task_group:'status'},''),'items').split('?')[1]);expect(emptyStatus.get('status_unset')).toBe('true');expect(emptyStatus.has('status')).toBe(false);
 expect(validTaskCollectionFilter({...scope,task_priority:'high',task_priority_unset:true})).toBe(false);expect(validTaskCollectionFilter({...scope,task_status:'todo',task_status_unset:true})).toBe(false);
});

it('accepts a bounded group-header page without equating its counts to the full project scope',()=>{
 const partial={project_id:'project-a',group_by:'owner',as_of:'2026-10-29T03:00:00Z',total:1000,groups:[{key:'human-a',total:3}],groups_total:120,groups_limit:20,next_group_cursor:'next-groups'};expect(validTaskCollectionSummary(partial,'project-a','owner')).toBe(true);expect(validTaskCollectionSummary({...partial,groups_total:0},'project-a','owner')).toBe(false);expect(validTaskCollectionSummary({...partial,groups:[{key:'human-a',total:1001}]},'project-a','owner')).toBe(false);
});
