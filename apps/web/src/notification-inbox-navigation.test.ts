import {describe,expect,it} from 'vitest';
import {parseWorkbenchRoute,resetInboxScope} from './navigation';
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
});
