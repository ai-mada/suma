import {expect,it} from 'vitest';
import captured from './test-fixtures/task-run-history.json';
import {validTaskRunHistory} from './task-run-history-model';

// Sanitized actual ListTaskRunProvenance output. Nullable historical binding
// facts are preserved, rather than backfilled from today's task or viewer.
it('accepts actual observed and bound history while rejecting invented binding facts or cross-task associations',()=>{
 const task=captured.items[0].task_id;expect(validTaskRunHistory(captured,task)).toBe(true);
 expect(captured.items.find(item=>item.binding_kind==='observed')?.bound_at).toBeNull();
 expect(validTaskRunHistory(captured,'other-task')).toBe(false);
 const bound=captured.items.find(item=>item.binding_kind==='bound')!;
 for(const invalid of [{...bound,task_version:null},{...bound,bound_at:null},{...bound,current:'true'},{...bound,run_id:'../other-run'}])expect(validTaskRunHistory({...captured,items:[invalid]},task)).toBe(false);
});
