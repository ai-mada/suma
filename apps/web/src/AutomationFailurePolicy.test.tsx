// @vitest-environment jsdom
import {act} from 'react';
import {createRoot} from 'react-dom/client';
import {expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import type {Session} from './api';
import {AutomationWorkspace} from './AutomationWorkspace';
import {automationFailurePauseReason,type AutomationRule} from './automation-model';
import {setLocale} from './i18n';

const project='project-automation';
const session:Session={
 user:{id:'reviewer',name:'Reviewer',email:'reviewer@example.test'},
 organization:{id:'team',name:'Team'},
 projects:[{id:project,version:1,role:'reviewer'}],
};
const stamp='2026-11-06T01:00:00Z';
const rule:AutomationRule={
 id:'rule-fixed',project_id:project,version:2,revision:1,name:'Nightly check',
 workflow_id:'workflow-fixed',workflow_version:1,trigger:'manual',cron:'',timezone:'UTC',
 misfire:'skip',catch_up_limit:1,overlap:'forbid',failure_pause_threshold:3,
 state:'enabled',activated_by:'reviewer',next_at:'',last_error:'',created_at:stamp,updated_at:stamp,
};
const stoppedTrigger={
 id:'trigger-stopped',rule_id:rule.id,revision:rule.revision,source:'cron',
 scheduled_at:stamp,state:'blocked',reason:automationFailurePauseReason,
 run_id:'',created_at:stamp,updated_at:stamp,
};

it('shows the failure-pause reason and requires a new policy revision before reviewed enablement',async()=>{
 const host=document.createElement('div');document.body.append(host);
 const root=createRoot(host);
 const writes:Record<string,unknown>[]=[];
 let current=rule,occurrences:typeof stoppedTrigger[]=[],detailReads=0;
 sessionStorage.clear();
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
 vi.stubGlobal('fetch',vi.fn(async(input:string,options?:RequestInit)=>{
  const url=new URL(input,location.origin);
  if(url.pathname.endsWith('/occurrences'))return Response.json({items:occurrences,total:occurrences.length});
  if(url.pathname.endsWith('/automations/'+rule.id+'/commands')){
   expect(options?.method).toBe('POST');
   const request=JSON.parse(String(options?.body)) as Record<string,unknown>;
   writes.push(request);
   expect(request.expected_version).toBe(current.version);
   if(request.action==='revise'){
    expect(request.spec).toMatchObject({failure_pause_threshold:4});
    current={...current,last_error:'',failure_pause_threshold:4,revision:current.revision+1,version:current.version+1};
   }else{
    expect(request.action).toBe('enable');
    current={...current,state:'enabled',version:current.version+1};
   }
   return Response.json({rule:current});
  }
  if(url.pathname.endsWith('/automations/'+rule.id)){detailReads++;return Response.json(current);}
  if(url.pathname.endsWith('/automations'))return Response.json({items:[current],total:1});
  if(url.pathname.endsWith('/workflows'))return Response.json({items:[{id:rule.workflow_id,version:1,name:'Checks'}],total:1});
  return Response.json({items:[],total:0});
 }));
 function button(container:ParentNode,label:string){
  return Array.from(container.querySelectorAll('button')).find(value=>value.textContent?.trim()===label);
 }
 async function click(element:HTMLElement){await act(async()=>{element.click();});}
 try{
  await setLocale('en-US',{persist:false});
  await act(async()=>root.render(<ApiViewProvider session={session}>
   <AutomationWorkspace project={project} session={session} selected={rule.id} search="" onSearch={()=>{}} refresh={0} onSelect={()=>{}}/>
  </ApiViewProvider>));
  expect(host.textContent).toContain('Pause after 3 failures');
  expect(detailReads).toBe(1);

  current={...current,state:'paused',last_error:automationFailurePauseReason,version:current.version+1};
  occurrences=[stoppedTrigger];
  await click(host.querySelector<HTMLButtonElement>('[aria-label="Refresh trigger records"]')!);
  const paused=host.querySelector('[data-automation-failure-paused]');
  expect(paused?.textContent).toContain('save a new policy revision');
  expect(host.querySelector('.automation-occurrences')?.textContent).toContain('blocked this trigger');
  expect(detailReads).toBe(2);
  expect(button(host,'Review and enable')).toBeUndefined();
  expect(writes).toHaveLength(0);

  await click(button(host,'Edit policy')!);
  const editor=document.querySelector('[role="dialog"]')!;
  const selector=editor.querySelector<HTMLSelectElement>('[name="failure_pause_threshold"]')!;
  expect(selector.value).toBe('3');
  expect([...selector.options].map(option=>option.value)).toEqual(['0','2','3','4','5','6','7','8','9','10']);
  await act(async()=>{
   selector.value='4';
   selector.dispatchEvent(new Event('change',{bubbles:true}));
  });
  await click(button(editor,'Save and pause rule')!);
  expect(host.querySelector('[data-automation-failure-paused]')).toBeNull();
  expect(writes).toHaveLength(1);
  await click(button(host,'Review and enable')!);
  const approval=document.querySelector('[role="dialog"]')!;
  expect(approval.textContent).toContain('Pause after 4 failures');
  expect(button(approval,'Enable this rule')?.disabled).toBe(true);
  await click(approval.querySelector<HTMLInputElement>('input[type="checkbox"]')!);
  await click(button(approval,'Enable this rule')!);
  expect(writes.map(request=>request.action)).toEqual(['revise','enable']);
  expect(button(host,'Trigger once')).toBeDefined();
 }finally{
  await act(async()=>root.unmount());host.remove();
  vi.unstubAllGlobals();sessionStorage.clear();
  await setLocale('zh-CN',{persist:false});
 }
});
