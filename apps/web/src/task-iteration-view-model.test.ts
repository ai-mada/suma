import {expect,it} from 'vitest';
import {defaultTaskView,taskQueryKey,taskViewFromRoute,taskViewToRoute,taskWorkspacePath} from './task-views-model';
import {sharedViewPreferences} from './SharedTaskViews';

it('round-trips exact and unlinked iteration views through route, query identity and shared preferences',()=>{
 for(const criteria of [{iteration_id:'unset'},{iteration_unset:true}] as const){
  const view={...defaultTaskView,...criteria,group_by:'iteration' as const,layout:'board' as const};
  const route=taskViewToRoute(view);expect(taskViewFromRoute(route)).toEqual(view);expect(taskQueryKey(view)).not.toBe(taskQueryKey(defaultTaskView));
  const query=new URLSearchParams(taskWorkspacePath('project-a',view).split('?')[1]);expect(query.get('iteration_id')).toBe('iteration_id' in criteria?criteria.iteration_id:null);expect(query.get('iteration_unset')).toBe('iteration_unset' in criteria?'true':null);
  const routeView={...view,record:'private-task'};const projected=sharedViewPreferences(routeView);expect(projected).toMatchObject(criteria);expect(projected.group_by).toBe('iteration');expect(projected).not.toHaveProperty('record');
 }
 const all=taskViewToRoute(defaultTaskView);expect(all.task_iteration).toBeUndefined();expect(all.task_iteration_unset).toBeUndefined();
});
