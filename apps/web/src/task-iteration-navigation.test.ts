import {expect,it} from 'vitest';
import {parseWorkbenchRoute,resetTaskCollectionScope} from './navigation';
import {switchedRoute} from './governance-model';

const base='?project=project-a&view=development&sub=tasks&';
it('restores exact iteration and grouping, and clears both when leaving the project',()=>{
 const route=parseWorkbenchRoute(base+'task_iteration=iteration-a&task_group=iteration');
 expect(route).toMatchObject({task_iteration:'iteration-a',task_group:'iteration'});
 expect(route.task_invalid).toBeUndefined();
 const unset=parseWorkbenchRoute(base+'task_iteration_unset=true');
 expect(unset.task_iteration_unset).toBe(true);expect(unset.task_invalid).toBeUndefined();
 const reset=resetTaskCollectionScope({...route,task_iteration_unset:false});
 expect(reset.task_iteration).toBeUndefined();expect(reset.task_iteration_unset).toBeUndefined();expect(reset.task_group).toBeUndefined();
 const next=switchedRoute(route,{user:{id:'u',name:'U',email:'u@example.test'},organization:{id:'org',name:'Org'},projects:[{id:'project-b',version:1}]});
 expect(next.project).toBe('project-b');expect(next.task_iteration).toBeUndefined();
 expect(parseWorkbenchRoute('?view=development&sub=my-work&task_iteration=iteration-a&task_iteration_unset=true').task_iteration).toBeUndefined();
});

it.each([
 'task_iteration=../other','task_iteration='+('a'.repeat(65)),
 'task_iteration=one&task_iteration=two','task_iteration_unset=true&task_iteration_unset=true',
 'task_iteration_unset=1','task_iteration_unset=',
 'task_iteration=iteration-a&task_iteration_unset=true',
])('rejects ambiguous iteration route without showing all tasks: %s',fields=>{
 expect(parseWorkbenchRoute(base+fields).task_invalid).toBe('1');
});
