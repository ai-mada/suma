import {describe,expect,it} from 'vitest';
import {locateTaskHandoff,parseWorkbenchRoute,resetInboxScope,taskHandoffID} from './navigation';
import {switchedRoute} from './governance-model';
import {palettePages} from './command-palette-model';
import type {Session} from './api';

describe('current notification reading route',()=>{
 it('restores only declared inbox filters and preserves invalid shared input without widening',()=>{
  for(const filter of ['inbox','unread','archived']){const route=parseWorkbenchRoute('?project=p&view=development&sub=inbox&inbox_filter='+filter);expect(route.inbox_filter).toBe(filter);expect(route.inbox_invalid).toBeUndefined();}
  for(const fields of ['inbox_filter=all','inbox_filter=','inbox_filter=unread&inbox_filter=inbox','inbox_invalid=1'])expect(parseWorkbenchRoute('?project=p&view=development&sub=inbox&'+fields).inbox_invalid).toBe('1');
  const outside=parseWorkbenchRoute('?project=p&view=development&sub=tasks&inbox_filter=unread');expect(outside.inbox_filter).toBeUndefined();
 });
 it('clears the private filter on a project fallback and keeps an accessible same-scope saved route',()=>{
  const current=parseWorkbenchRoute('?project=old&view=development&sub=inbox&inbox_filter=unread&inbox_invalid=1');
  expect(resetInboxScope(current).inbox_filter).toBeUndefined();
  const session:Session={user:{id:'u',name:'U',email:'u@example.test'},organization:{id:'o',name:'O'},projects:[{id:'next',version:1}]};
  const changed=switchedRoute(current,session);expect(changed.project).toBe('next');expect(changed.inbox_filter).toBeUndefined();expect(changed.inbox_invalid).toBeUndefined();
  const saved={...current,project:'next'};expect(switchedRoute(current,session,saved)).toEqual(saved);
 });
 it('offers one project-scoped command destination without a guessed receiver',()=>{
  const entries=palettePages.filter(page=>page.id==='inbox');expect(entries).toHaveLength(1);expect(entries[0]).toMatchObject({projectRequired:true,target:{view:'development',sub:'inbox'}});
 });
 it('restores an exact handoff only inside an unambiguous task detail scope',()=>{
  const prefix='?project=p&view=development&sub=tasks&record=task-a&';
  expect(taskHandoffID(parseWorkbenchRoute(prefix+'panel=task-handoff:handoff-a').panel)).toBe('handoff-a');
  for(const fields of ['panel=task-handoff:..','panel=task-handoff:a%2Fb','panel=task-handoff:a&panel=task-handoff:b','panel=task-handoff:a&record=task-b'])expect(parseWorkbenchRoute(prefix+fields).panel).toBe('');
  expect(parseWorkbenchRoute('?project=p&view=development&sub=inbox&record=task-a&panel=task-handoff:a').panel).toBe('');
  expect(parseWorkbenchRoute('?project=p&view=development&sub=tasks&panel=task-handoff:a').panel).toBe('');
 });
 it('keeps an unsaved current scope until explicit navigation succeeds',()=>{
  let route=parseWorkbenchRoute('?project=p&view=development&sub=inbox&inbox_filter=unread');
  const before=route,update=(change:(value:typeof route)=>typeof route)=>{route=change(route);};
  expect(locateTaskHandoff('task-a','handoff-a',()=>false,update)).toBe(false);expect(route).toBe(before);
  expect(locateTaskHandoff('task-a','../other',()=>true,update)).toBe(false);expect(route).toBe(before);
  expect(locateTaskHandoff('task-a','handoff-a',()=>true,update)).toBe(true);
  expect(route).toMatchObject({project:'p',view:'development',sub:'tasks',record:'task-a',panel:'task-handoff:handoff-a',run:'',node:'',attempt:'',layout:'list'});
  expect(route.inbox_filter).toBeUndefined();expect(route.conversation_message).toBeUndefined();
 });
});
