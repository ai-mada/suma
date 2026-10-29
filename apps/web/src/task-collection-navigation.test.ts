import {describe,expect,it} from 'vitest';
import {parseWorkbenchRoute,resetTaskCollectionScope} from './navigation';
import {switchedRoute} from './governance-model';
import type {Session} from './api';
import {stateLabel} from './api';

describe('task collection route scope',()=>{
 it('keeps custom states that match Object prototype names as literal labels',()=>{
  expect(stateLabel('constructor')).toBe('constructor');expect(stateLabel('__proto__')).toBe('__proto__');
 });
 it('restores literal all state and project-owned filters from a shared URL',()=>{
  const route=parseWorkbenchRoute('?project=p&view=development&sub=tasks&task_status=all&task_owner=human&task_priority=urgent&task_requirement=req&task_unassigned=false&q=%25_!');
  expect(route).toMatchObject({project:'p',task_status:'all',task_owner:'human',task_priority:'urgent',task_requirement:'req',task_unassigned:false,q:'%_!'});
  expect(route.task_invalid).toBeUndefined();
  const unassigned=parseWorkbenchRoute('?project=p&view=development&sub=tasks&task_unassigned=true');
  expect(unassigned.task_unassigned).toBe(true);expect(unassigned.task_invalid).toBeUndefined();
 });
 it('clears all scoped filters on a project fallback but preserves an accessible saved view',()=>{
  const route=parseWorkbenchRoute('?project=old&view=development&sub=tasks&task_status=all&task_owner=human&task_priority=urgent&task_requirement=req&task_unassigned=false&task_sort=updated_desc&task_group=status');
  const reset=resetTaskCollectionScope(route);
  for(const key of ['task_invalid','task_status','task_owner','task_priority','task_requirement','task_unassigned','task_sort','task_group'] as const)expect(reset[key]).toBeUndefined();
  const session:Session={user:{id:'u',name:'User',email:'u@example.test'},organization:{id:'o',name:'Org'},projects:[{id:'next',version:1}]};
  const next=switchedRoute(route,session);
  expect(next.project).toBe('next');expect(next.task_owner).toBeUndefined();expect(next.task_status).toBeUndefined();
  const saved={...route,project:'next'};expect(switchedRoute(route,session,saved)).toEqual(saved);
 });
 it.each(['task_unassigned=1','task_unassigned=','task_requirement=../other','task_requirement='+('a'.repeat(65)),'task_unassigned=true&task_owner=human','task_status=todo&task_status_unset=true','task_priority=high&task_priority_unset=true','task_sort=unknown','task_group=unknown','task_status='+encodeURIComponent('字'.repeat(201)),'task_status=%20%20','task_status=todo&task_status=todo','q=%09'])('refuses invalid task filters without widening the query: %s',fields=>{
  expect(parseWorkbenchRoute('?project=p&view=development&sub=tasks&'+fields).task_invalid).toBe('1');
 });
 it('retains current literal values, all four sorts, and explicit unset filters',()=>{
  for(const fields of ['task_sort=title_asc&task_group=owner','task_status=archived','task_owner=__unassigned__','task_requirement=req','task_status_unset=true&task_priority_unset=true','task_status='+encodeURIComponent('字'.repeat(100))])expect(parseWorkbenchRoute('?view=development&sub=tasks&'+fields).task_invalid).toBeUndefined();
  const route=parseWorkbenchRoute('?view=development&sub=tasks&q=%20literal%20&task_status=all&task_priority_unset=true');
  expect(route.q).toBe(' literal ');expect(route.task_status).toBe('all');expect(route.task_priority_unset).toBe(true);
  const reset=resetTaskCollectionScope(route);expect(reset.task_priority_unset).toBeUndefined();expect(reset.task_status_unset).toBeUndefined();
 });
 it('retains the default personal workspace and drops task-only fields outside tasks',()=>{
  const personal=parseWorkbenchRoute('?project=p&view=development&task_requirement=req&task_unassigned=true');
  expect(personal.sub).toBe('my-work');expect(personal.task_requirement).toBeUndefined();expect(personal.task_unassigned).toBeUndefined();
 });
});
