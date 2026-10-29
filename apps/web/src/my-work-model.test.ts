import {expect,it} from 'vitest';
import {appendMyWorkPage,myWorkPath,myWorkScopeKey,myWorkStatus,validMyWorkPage,validMyWorkQuery} from './my-work-model';
import type {MyWorkPage} from '../../../packages/contracts/src';

const scope={project:'project-a',userID:'user-a',organizationID:'org-a',q:'',status:'' as const};
const task=(patch:Record<string,unknown>={})=>({id:'task-a',project_id:'project-a',version:1,title:'Assigned work',updated_at:'2026-10-29T01:00:00Z',status:'todo',owner_id:'human-resource-a',...patch});
const page=(patch:Record<string,unknown>={})=>({project_id:'project-a',user_id:'user-a',resource_id:'human-resource-a',resource_name:'Human A',resource_status:'active',items:[task()],total:1,limit:20,next_cursor:'',status_counts:[{status:'todo',total:1}],as_of:'2026-10-29T02:00:00Z',...patch});

it('preserves literal search text and binds cursor navigation to the selected filter',()=>{
 const params=new URLSearchParams(myWorkPath({...scope,q:' 50%_中文 + ',status:'blocked'},'opaque+/=').split('?')[1]);
 expect(params.get('q')).toBe(' 50%_中文 + ');expect(params.get('status')).toBe('blocked');expect(params.get('cursor')).toBe('opaque+/=');expect(params.get('limit')).toBe('20');
 expect(validMyWorkQuery('😀'.repeat(200))).toBe(true);expect(validMyWorkQuery('😀'.repeat(201))).toBe(false);expect(validMyWorkQuery('a\0b')).toBe(false);expect(myWorkStatus('all')).toBe('');expect(myWorkStatus('draft')).toBe('draft');expect(myWorkStatus('running')).toBe('');
 expect(myWorkScopeKey({...scope,userID:'user-b'})).not.toBe(myWorkScopeKey(scope));expect(myWorkScopeKey({...scope,organizationID:'org-b'})).not.toBe(myWorkScopeKey(scope));
});

it('accepts only the server human resource and rejects cross-user, owner, project or status facts',()=>{
 expect(validMyWorkPage(page(),scope)).toBe(true);
 for(const patch of [{user_id:'user-b'},{project_id:'project-b'},{items:[task({owner_id:'user-a'})]},{items:[task({project_id:'project-b'})]},{items:[task({status:'archived'})]},{items:[task(),task()]},{total:2},{status_counts:[{status:'todo',total:1},{status:'todo',total:0}]}])expect(validMyWorkPage(page(patch),scope)).toBe(false);
 expect(validMyWorkPage(page({items:[],total:0,status_counts:[{status:'todo',total:1}]}),{...scope,status:'blocked'})).toBe(true);
});

it('keeps assigned drafts and current custom business states in the complete queue and counts',()=>{
 const mixed=page({items:[task({status:'draft'}),task({id:'task-b',status:'waiting_for_customer'})],total:3,status_counts:[{status:'draft',total:1},{status:'waiting_for_customer',total:2}]});expect(validMyWorkPage(mixed,scope)).toBe(true);
 expect(validMyWorkPage(page({items:[task({status:'draft'})],status_counts:[{status:'draft',total:1}]}),{...scope,status:'draft'})).toBe(true);
 expect(validMyWorkPage(page({status_counts:[{status:'archived',total:1}]}),scope)).toBe(false);expect(validMyWorkPage(page({items:[task({status:'x'.repeat(201)})]}),scope)).toBe(false);
});

it('allows unbound and inactive empty reads while rejecting concealed task facts',()=>{
 for(const resource_status of ['unbound','inactive']){
  const empty={resource_status,items:[],total:0,status_counts:[],next_cursor:''};expect(validMyWorkPage(page(empty),scope)).toBe(true);expect(validMyWorkPage(page({...empty,items:[task()]}),scope)).toBe(false);expect(validMyWorkPage(page({...empty,status_counts:[{status:'todo',total:1}]}),scope)).toBe(false);
 }
});

it('merges live pages by identity and rejects resource changes or shrinking totals',()=>{
 const initial=page({total:2,next_cursor:'next',status_counts:[{status:'todo',total:2}]}) as unknown as MyWorkPage;
 const next=page({items:[task({title:'Updated task'}),task({id:'task-b'})],total:2,status_counts:[{status:'todo',total:2}]}) as unknown as MyWorkPage;
 expect(appendMyWorkPage(initial,next)?.items.map(row=>row.title)).toEqual(['Updated task','Assigned work']);expect(appendMyWorkPage(initial,{...next,resource_id:'other-resource'})).toBeUndefined();expect(appendMyWorkPage(initial,{...next,total:1})).toBeUndefined();
});
