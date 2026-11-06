import {describe,expect,it} from 'vitest';
import {locateNotificationSource,locateTaskHandoff,parseWorkbenchRoute,resetInboxScope,taskHandoffID} from './navigation';
import {switchedRoute} from './governance-model';
import {palettePages} from './command-palette-model';
import type {Session} from './api';

describe('current notification reading route',()=>{
 it('restores only declared inbox filters and preserves invalid shared input without widening',()=>{
  for(const filter of ['inbox','unread','archived']){const route=parseWorkbenchRoute('?project=p&view=development&sub=inbox&inbox_filter='+filter);expect(route.inbox_filter).toBe(filter);expect(route.inbox_invalid).toBeUndefined();}
  for(const fields of ['inbox_filter=all','inbox_filter=','inbox_filter=unread&inbox_filter=inbox','inbox_invalid=1'])expect(parseWorkbenchRoute('?project=p&view=development&sub=inbox&'+fields).inbox_invalid).toBe('1');
  const outside=parseWorkbenchRoute('?project=p&view=development&sub=tasks&inbox_filter=unread');expect(outside.inbox_filter).toBeUndefined();
 });
 it('opens only an explicit organization scope and rejects duplicate or unknown scope',()=>{
  const prefix='?project=p&view=development&sub=inbox&';
  expect(parseWorkbenchRoute(prefix+'inbox_scope=organization&inbox_filter=archived')).toMatchObject({inbox_scope:'organization',inbox_filter:'archived'});
  expect(parseWorkbenchRoute(prefix+'inbox_scope=project').inbox_scope).toBeUndefined();
  for(const query of ['inbox_scope=','inbox_scope=tenant','inbox_scope=organization&inbox_scope=project'])expect(parseWorkbenchRoute(prefix+query).inbox_invalid).toBe('1');
  expect(parseWorkbenchRoute('?view=development&sub=tasks&inbox_scope=organization').inbox_scope).toBeUndefined();
 });
 it('moves to the exact project and source together without overriding a leave guard',()=>{
  const start=parseWorkbenchRoute('?project=alpha&view=development&sub=inbox&inbox_scope=organization&inbox_filter=unread');
  let route=start;
  const update=(change:(value:typeof route)=>typeof route)=>{route=change(route);};
  expect(locateNotificationSource('beta','task-b',{kind:'message',id:'message-b'},()=>false,update)).toBe(false);
  expect(locateNotificationSource('../beta','task-b',{kind:'task'},()=>true,update)).toBe(false);
  expect(route).toBe(start);
  expect(locateNotificationSource('beta','task-b',{kind:'message',id:'message-b'},()=>true,update)).toBe(true);
  expect(route).toMatchObject({project:'beta',sub:'ide',panel:'workbench-task:task-b',conversation_message:'message-b',run:'',attempt:''});
  expect(route.inbox_scope).toBeUndefined();
  expect(route.inbox_filter).toBeUndefined();
  route=start;
  expect(locateNotificationSource('beta','task-b',{kind:'review',id:'review-b'},()=>true,update)).toBe(true);
  expect(route).toMatchObject({project:'beta',sub:'tasks',record:'task-b',panel:'task-review:review-b'});
  route=start;
  expect(locateNotificationSource('beta','task-b',{kind:'run',id:'run-b'},()=>false,update)).toBe(false);
  expect(locateNotificationSource('beta','task-b',{kind:'run',id:'../run-b'},()=>true,update)).toBe(false);
  expect(route).toBe(start);
  expect(locateNotificationSource('beta','task-b',{kind:'run',id:'run-b'},()=>true,update)).toBe(true);
  expect(route).toMatchObject({project:'beta',sub:'runs',run:'run-b',record:'',panel:'',node:'',attempt:''});
  expect(route.inbox_scope).toBeUndefined();
 });
 it('opens an automation alert in the originating project without task state',()=>{
  const start=parseWorkbenchRoute('?project=alpha&view=development&sub=inbox&inbox_scope=organization&inbox_filter=unread');
  let route=start;
  const update=(change:(value:typeof route)=>typeof route)=>{route=change(route);};
  expect(locateNotificationSource('beta','',{kind:'automation',id:'rule-b'},()=>false,update)).toBe(false);
  expect(locateNotificationSource('beta','',{kind:'automation',id:'../rule-b'},()=>true,update)).toBe(false);
  expect(route).toBe(start);
  expect(locateNotificationSource('beta','',{kind:'automation',id:'rule-b'},()=>true,update)).toBe(true);
  expect(route).toMatchObject({project:'beta',view:'development',sub:'automations',record:'rule-b',panel:'',run:'',node:'',attempt:'',layout:'list'});
  expect(route.inbox_scope).toBeUndefined();
  expect(route.inbox_filter).toBeUndefined();
 });
 it('opens the original tool input question within its own attempt and honors the leave guard',()=>{
  const start=parseWorkbenchRoute('?project=alpha&view=development&sub=inbox&inbox_scope=organization&inbox_filter=unread');
  let route=start;
  const update=(change:(value:typeof route)=>typeof route)=>{route=change(route);};
  const target={run_id:'run-original',node_id:'implement[2]',attempt:3,input_id:'a'.repeat(32)};
  expect(locateNotificationSource('beta','task-b',{kind:'tool-input',target},()=>false,update)).toBe(false);
  expect(locateNotificationSource('beta','task-b',{kind:'tool-input',target:{...target,attempt:0}},()=>true,update)).toBe(false);
  expect(route).toBe(start);
  expect(locateNotificationSource('beta','task-b',{kind:'tool-input',target},()=>true,update)).toBe(true);
  expect(route).toMatchObject({project:'beta',view:'development',sub:'runs',run:'run-original',node:'implement[2]',attempt:'3',record:'',panel:'tool-input:'+target.input_id,layout:'list'});
  expect(route.inbox_scope).toBeUndefined();
  expect(route.inbox_filter).toBeUndefined();
 });
 it('retains a tool input locator only for an unambiguous execution route',()=>{
  const params=new URLSearchParams({project:'p',view:'development',sub:'runs',run:'run-a',node:'node[2]',attempt:'3',panel:'tool-input:'+'a'.repeat(32)});
  expect(parseWorkbenchRoute('?'+params).panel).toBe(params.get('panel'));
  for(const [key,value] of [['attempt','0'],['node','../other'],['sub','tasks'],['layout','queue'],['record','another'],['panel','tool-input:other']]){
   const invalid=new URLSearchParams(params);invalid.set(key,value);
   expect(parseWorkbenchRoute('?'+invalid).panel).toBe('');
  }
  for(const key of ['project','view','sub','run','node','attempt','panel']){
   const ambiguous=new URLSearchParams(params);ambiguous.append(key,params.get(key)!);
   expect(parseWorkbenchRoute('?'+ambiguous).panel).toBe('');
  }
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
