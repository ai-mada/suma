import {describe,it,expect} from 'vitest';
import {consumeTaskAcceptancePanel,isWorkBuddyProgressTaskLocator,parseTaskAcceptancePanel,taskAcceptancePanel,taskDeliveryHref,workBuddyProgressTaskHref} from './task-delivery-navigation-model';
import {clearWorkBuddyProgressScope,parseWorkbenchRoute,type WorkbenchRoute} from './navigation';

describe('task delivery navigation targets',()=>{
  it('opens an exact WorkBuddy task scope and rejects ambiguous navigation without widening the read',()=>{
    const href=workBuddyProgressTaskHref('project-a','task-a','attention')!;
    expect(isWorkBuddyProgressTaskLocator(new URLSearchParams(href))).toBe(true);
    const exact=parseWorkbenchRoute(href);
    expect(exact).toMatchObject({project:'project-a',view:'development',sub:'runs',layout:'workbuddy',workbuddy_task:'task-a',workbuddy_filter:'attention'});
    expect(clearWorkBuddyProgressScope(exact)).toMatchObject({workbuddy_task:undefined,workbuddy_filter:undefined});
    for(const filter of ['delivery','input'] as const){
      const scoped=workBuddyProgressTaskHref('project-a','task-a',filter)!;
      expect(isWorkBuddyProgressTaskLocator(new URLSearchParams(scoped))).toBe(true);
      expect(parseWorkbenchRoute(scoped)).toMatchObject({workbuddy_task:'task-a',workbuddy_filter:filter});
      expect(isWorkBuddyProgressTaskLocator(new URLSearchParams(scoped+'&workbuddy_filter=attention'))).toBe(false);
      expect(parseWorkbenchRoute(scoped+'&workbuddy_filter=attention')).toMatchObject({workbuddy_filter_invalid:'1'});
    }
    for(const suffix of ['&workbuddy_task=other','&project=other','&run=','&node=node-a','&record=task-a','&panel=other']){
      const conflicting=href+suffix;
      expect(isWorkBuddyProgressTaskLocator(new URLSearchParams(conflicting))).toBe(false);
      expect(parseWorkbenchRoute(conflicting)).toMatchObject({workbuddy_task_invalid:'1'});
    }
    for(const id of ['','.','..','task/other','a'.repeat(65)]){
      expect(workBuddyProgressTaskHref('project-a',id)).toBeUndefined();
      expect(parseWorkbenchRoute(href.replace('task-a',encodeURIComponent(id))).workbuddy_task_invalid).toBe('1');
    }
  });

  it('round-trips only canonical three-part acceptance locators with exact bounded public identifiers',()=>{
    for(const taskVersion of [1,27,2147483646]){
      for(const runID of ['run-Exact_1.2','a'.repeat(64)]){
        const panel=taskAcceptancePanel(taskVersion,runID);
        expect(panel).toBe('task-acceptance:'+taskVersion+':'+runID);
        expect(parseTaskAcceptancePanel(panel!)).toEqual({taskVersion,runID});
      }
    }
    const invalidIDs=['','.','..','../run','run/child','run\\child','run:child','run?next=other','run#other','run&project=other','%2e%2e',' run','run ','run\n','run\r\n','run\0','a'.repeat(65)];
    for(const runID of invalidIDs){
      expect(taskAcceptancePanel(3,runID),JSON.stringify(runID)).toBeUndefined();
      expect(parseTaskAcceptancePanel('task-acceptance:3:'+runID),JSON.stringify(runID)).toBeUndefined();
    }
    for(const taskVersion of [0,-1,1.5,2147483647,Number.MAX_SAFE_INTEGER,NaN,Infinity]){
      expect(taskAcceptancePanel(taskVersion,'run-exact')).toBeUndefined();
    }
    for(const panel of ['','task-acceptance:1','task-acceptance:1:run:extra','other:1:run','task-acceptance:01:run','task-acceptance:+1:run','task-acceptance:1.0:run','task-acceptance:1e1:run','task-acceptance: 1:run','task-acceptance:0:run','task-acceptance:2147483647:run','task-acceptance:9007199254740993:run']){
      expect(parseTaskAcceptancePanel(panel),panel).toBeUndefined();
    }
  });

  it('links each exact project reference to the original page without carrying execution or acceptance authority',()=>{
    const project='project-Exact_1.2',id='record-Exact_1.2';
    const targets=[
      {target:{kind:'issue' as const,id},fields:{project,view:'quality',sub:'issues',record:id,layout:'list'}},
      {target:{kind:'requirement' as const,id},fields:{project,view:'requirements',sub:'',record:id,layout:'list'}},
      {target:{kind:'team' as const,id},fields:{project,view:'development',sub:'runs',layout:'team',panel:'expert-team:'+id}},
      {target:{kind:'delivery' as const,id},fields:{project,view:'development',sub:'tasks',record:id,layout:'list',panel:'task-delivery'}},
      {target:{kind:'review' as const,id,taskVersion:7,runID:'run-Exact_7'},fields:{project,view:'development',sub:'tasks',record:id,layout:'list',panel:'task-acceptance:7:run-Exact_7'}},
    ];
    for(const {target,fields} of targets){
      const href=taskDeliveryHref(project,target)!;
      expect(href.startsWith('?')).toBe(true);
      expect(Object.fromEntries(new URLSearchParams(href))).toEqual(fields);
      for(const invalid of ['','.','..','../other','other/child','other\\child','other:child','other&project=foreign','other?record=foreign','other#fragment','other\n','a'.repeat(65)]){
        expect(taskDeliveryHref(invalid,target),JSON.stringify(invalid)).toBeUndefined();
        expect(taskDeliveryHref(project,{...target,id:invalid}),JSON.stringify(invalid)).toBeUndefined();
      }
    }
    // A navigation target is not a quality decision or a fallback to an
    // arbitrary task: review always retains its original version and run.
    for(const target of [{kind:'review' as const,id},{kind:'review' as const,id,taskVersion:0,runID:'run'},{kind:'review' as const,id,taskVersion:7,runID:'../other'}]){
      expect(taskDeliveryHref(project,target)).toBeUndefined();
    }
  });

  it('keeps a delivery locator only for one exact task detail without a competing scope',()=>{
    const params=new URLSearchParams(taskDeliveryHref('project-Exact_1.2',{kind:'delivery',id:'task-Exact_1.2'}));
    const parse=(p:URLSearchParams)=>parseWorkbenchRoute('?'+p.toString());
    expect(parse(params)).toMatchObject({project:'project-Exact_1.2',view:'development',sub:'tasks',record:'task-Exact_1.2',layout:'list',panel:'task-delivery',run:'',node:'',attempt:''});
    const defaultLayout=new URLSearchParams(params);
    defaultLayout.delete('layout');
    expect(parse(defaultLayout).panel).toBe('task-delivery');
    for(const key of ['project','record','view','sub','layout','panel']){
      const ambiguous=new URLSearchParams(params);
      ambiguous.append(key,ambiguous.get(key)!);
      expect(parse(ambiguous).panel,key).toBe('');
    }
    for(const [key,value] of [['project',''],['project','foreign/project'],['record',''],['record','..'],['record','task\n'],['view','quality'],['sub','runs'],['sub',''],['layout','dependencies'],['layout','board'],['run','run-a'],['node','node-a'],['attempt','0']]){
      const conflicting=new URLSearchParams(params);
      conflicting.set(key,value);
      expect(parse(conflicting).panel,key+'='+value).toBe('');
    }
  });

  it('round-trips an exact original run without exposing an ambiguous task route',()=>{
    const project='project-a',id='task-a',runID='run-Exact_1.2';
    const href=taskDeliveryHref(project,{kind:'delivery',id,runID})!;
    const params=new URLSearchParams(href);
    expect(params.get('panel')).toBe('task-delivery:'+runID);
    expect(params.has('run')).toBe(false);
    expect(parseWorkbenchRoute(href)).toMatchObject({record:id,run:'',panel:'task-delivery:'+runID});
    for(const invalid of ['','.','..','run/extra','run:extra','run\n','r'.repeat(65)]){
      expect(taskDeliveryHref(project,{kind:'delivery',id,runID:invalid})).toBeUndefined();
      params.set('panel','task-delivery:'+invalid);
      expect(parseWorkbenchRoute('?'+params).panel).toBe('');
    }
    params.set('panel','task-delivery:'+runID);
    for(const key of ['run','node','attempt']){
      const conflicting=new URLSearchParams(params);
      conflicting.set(key,'');
      expect(parseWorkbenchRoute('?'+conflicting).panel).toBe('');
    }
    params.append('panel','task-delivery');
    expect(parseWorkbenchRoute('?'+params).panel).toBe('');
  });

  it('consumes only the matching originating route and preserves newer account, scope or panel objects',()=>{
    const route:WorkbenchRoute={project:'project-a',view:'development',sub:'tasks',record:'task-a',panel:'task-acceptance:3:run-a',run:'',node:'',attempt:'',q:'keep this filter',status:'all',layout:'list',week:'2026-11-02',task_owner:'member-a'};
    const owner={userID:'user-a',organizationID:'org-a'};
    const expected={...owner,project:'project-a',taskID:'task-a',panel:'task-acceptance:3:run-a'};
    const consumed=consumeTaskAcceptancePanel(route,owner,expected);
    expect(consumed).not.toBe(route);
    expect(consumed).toEqual({...route,panel:''});
    expect(consumeTaskAcceptancePanel(consumed,owner,expected)).toBe(consumed);
    for(const changedOwner of [{...owner,userID:'user-b'},{...owner,organizationID:'org-b'},{userID:undefined,organizationID:undefined}]){
      expect(consumeTaskAcceptancePanel(route,changedOwner,expected)).toBe(route);
    }
    for(const patch of [{project:'project-b'},{view:'quality' as const},{sub:'runs'},{record:'task-b'},{panel:'task-acceptance:4:run-a'},{panel:'task-acceptance:3:run-b'},{panel:'dispatch:new'},{panel:''}]){
      const newer={...route,...patch};
      expect(consumeTaskAcceptancePanel(newer,owner,expected),JSON.stringify(patch)).toBe(newer);
    }
    for(const patch of [{project:'project-b'},{taskID:'task-b'},{panel:'task-acceptance:03:run-a'},{panel:'task-acceptance:3:run-a:extra'}]){
      expect(consumeTaskAcceptancePanel(route,owner,{...expected,...patch}),JSON.stringify(patch)).toBe(route);
    }
  });
});
