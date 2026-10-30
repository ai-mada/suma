import {describe,it,expect} from 'vitest';
import {parseWorkbenchRoute,resetTaskCollectionScope} from './navigation';
import {defaultTaskView,taskViewFromRoute,taskViewToRoute,taskWorkspacePath,taskQueryKey} from './task-views-model';
import {groupTaskRows} from './multica-task-view';

describe('task view route and grouped projections',()=>{
 it('round trips exact custom all status and member filters without changing generic status semantics',()=>{
  const view={...defaultTaskView,status:'all',priority:'custom-priority',owner_id:'member-a',sort:'title_asc' as const,layout:'board' as const,group_by:'owner' as const};
  const fields=taskViewToRoute(view),url=new URLSearchParams({view:'development',sub:'tasks'});
  for(const [key,value] of Object.entries(fields))if(value!==undefined)url.set(key,String(value));
  expect(taskViewFromRoute(parseWorkbenchRoute('?'+url))).toEqual(view);
  expect(taskViewFromRoute(parseWorkbenchRoute('?view=development&status=todo')).status).toBe('');
  expect(parseWorkbenchRoute('?view=quality&sub=issues&status=open&task_owner=member-a').task_owner).toBeUndefined();
 });
 it.each(['task_status='+encodeURIComponent('字'.repeat(201)),'task_owner=../other','task_sort=arbitrary','task_group=run','q='+('a'.repeat(201))])('fails closed for invalid task links: %s',value=>{
  expect(parseWorkbenchRoute('?view=development&sub=tasks&'+value).task_invalid).toBe('1');
 });
 it('keeps literal search and cursor characters in the server query',()=>{
  const path=taskWorkspacePath('project-a',{...defaultTaskView,q:'  %_<script>&  ',owner_id:'__unassigned__'},'opaque+&=');
  const params=new URL(path,'http://test').searchParams;
  expect(params.get('q')).toBe('  %_<script>&  ');expect(params.get('owner_id')).toBe('__unassigned__');expect(params.get('cursor')).toBe('opaque+&=');
 });
 it('preserves a 200-code-point astral query without splitting UTF-16 pairs',()=>{
  const q='😀'.repeat(200),route=parseWorkbenchRoute('?view=development&sub=tasks&q='+encodeURIComponent(q));
  expect(route.task_invalid).toBeUndefined();expect(taskViewFromRoute(route).q).toBe(q);
  expect(new URL(taskWorkspacePath('project-a',taskViewFromRoute(route)),'http://test').searchParams.get('q')).toBe(q);
  expect(parseWorkbenchRoute('?view=development&sub=tasks&q='+encodeURIComponent(q+'😀')).task_invalid).toBe('1');
  const personal=parseWorkbenchRoute('?view=development&task_owner=member-a');
  expect(personal.sub).toBe('my-work');expect(personal.task_owner).toBeUndefined();
 });
 it('groups the same loaded membership without mutating or reordering server results',()=>{
  const rows=[{id:'b',owner:'x'},{id:'a',owner:''},{id:'c',owner:'x'}];
  const groups=groupTaskRows(rows,['','x'],row=>row.owner);
  expect(groups.map(group=>group.rows.map(row=>row.id))).toEqual([['a'],['b','c']]);
  expect(rows.map(row=>row.id)).toEqual(['b','a','c']);
 });
});

it('round trips display columns separately from task membership and retains all filters for malformed column links',()=>{
 const selected={...defaultTaskView,q:'exact',status:'todo',owner_id:'member-a',columns:['title','updated','status']};const route=taskViewToRoute(selected);const url=new URLSearchParams({view:'development',sub:'tasks'});for(const [key,value]of Object.entries(route))if(value!==undefined)url.set(key,String(value));expect(taskViewFromRoute(parseWorkbenchRoute('?'+url))).toEqual(selected);expect(taskQueryKey(selected)).toBe(taskQueryKey({...selected,columns:['title']}));expect(taskWorkspacePath('project-a',selected)).not.toContain('columns');
 const invalid=taskViewFromRoute(parseWorkbenchRoute('?view=development&sub=tasks&task_columns=title,unknown&task_owner=member-a&task_status=todo&q=exact'));expect(invalid).toMatchObject({q:'exact',status:'todo',owner_id:'member-a',columns:['title','unknown']});
 const duplicate=taskViewFromRoute(parseWorkbenchRoute('?view=development&sub=tasks&task_columns=title,status&task_columns=title,owner&task_status=todo'));expect(duplicate.status).toBe('todo');expect(duplicate.columns).toEqual(['title','status','title','owner']);expect(resetTaskCollectionScope(parseWorkbenchRoute('?view=development&sub=tasks&task_columns=title,owner')).task_columns).toBeUndefined();
});
