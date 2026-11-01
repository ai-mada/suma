// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {TaskDiscussionDevelopmentPlan} from '../../../packages/contracts/src';
import {DiscussionDevelopmentPanel} from './DiscussionDevelopmentPanel';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import {codex,harness,native,identity,project,task,page,reply,workflow,options,models,development} from './discussion-developer-tools.test-support';
import {modelBinding} from './model-execution.test-support';

let root:Root,host:HTMLDivElement;
async function settle(){await act(async()=>{for(let i=0;i<22;i++)await Promise.resolve();});}
async function click(selector:string){const element=host.querySelector<HTMLElement>(selector);expect(element).not.toBeNull();await act(async()=>element!.click());await settle();}
async function change(selector:string,value:string){const field=host.querySelector<HTMLInputElement|HTMLSelectElement|HTMLTextAreaElement>(selector);expect(field).not.toBeNull();await act(async()=>{Object.getOwnPropertyDescriptor(field instanceof HTMLSelectElement?HTMLSelectElement.prototype:field instanceof HTMLTextAreaElement?HTMLTextAreaElement.prototype:HTMLInputElement.prototype,'value')!.set!.call(field,value);field!.dispatchEvent(new Event(field instanceof HTMLSelectElement?'change':'input',{bubbles:true}));});await settle();}
beforeEach(async()=>{await setLocale('en-US',{persist:false});sessionStorage.clear();vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();sessionStorage.clear();vi.unstubAllGlobals();vi.restoreAllMocks();});
function transport({wrongCodex=false}:{wrongCodex?:boolean}={}){
 const writes:{path:string;body:Record<string,unknown>}[]=[],reads:string[]=[];let saved:TaskDiscussionDevelopmentPlan|undefined,wrong=wrongCodex;
 vi.stubGlobal('fetch',vi.fn(async(input:string,init:RequestInit={})=>{
  const path=String(input).replace('/api/v1',''),body=init.body?JSON.parse(String(init.body)):undefined;
  if(!body){reads.push(path);if(path.startsWith('/workflows?'))return Response.json({items:[workflow]});if(path.startsWith('/tasks/'+task+'/dispatch-options?'))return Response.json(options);
   if(path.startsWith('/model-execution/options?')){const id=new URL('http://test'+path).searchParams.get('tool_profile_id'),tool=[codex,harness,native].find(item=>item.id===id)!;return Response.json(models(tool));}
   if(path.startsWith('/task-discussion-development-plans/development-plan?')&&saved)return Response.json(saved);
  }else{writes.push({path,body});if(path==='/task-discussion-development-plans'){saved=development(body);if(wrong&&saved.dispatch_plan.codex_execution){wrong=false;const other={...saved.dispatch_plan.codex_execution,model:'other-model'};return Response.json({...saved,dispatch_plan:{...saved.dispatch_plan,codex_execution:other,workflow:{...saved.dispatch_plan.workflow,nodes:saved.dispatch_plan.workflow.nodes.map(node=>node.id==='code'?{...node,config:{...node.config,codex_execution:other}}:node)}}},{status:201});}return Response.json(saved,{status:201});}
   if(path.endsWith('/start')&&saved){saved={...saved,version:2,dispatch_plan:{...saved.dispatch_plan,version:2,state:'started'}};return Response.json({id:'development-run',project_id:project,workflow_id:'managed-development-run',workflow_version:1,state:'running'},{status:201});}
  }return Response.json({error:{code:'unexpected_fixture',message:'Unexpected fixture read'}},{status:400});
 }));return {writes,reads};
}
async function open(onRun=vi.fn()){
 await act(async()=>root.render(<ApiViewProvider session={identity}><DiscussionDevelopmentPanel session={identity} project={project} taskId={task} reply={reply} page={page} disabled={false} accessBlocked={false} onLockChange={()=>{}} onRun={onRun}/></ApiViewProvider>));await settle();await click('[data-development-toggle]');
 await change('[data-development-criteria]','Independently verify the fixed candidate');await change('.discussion-development-fields select','flow@2');return onRun;
}
it('uses a fresh Codex choice, recovers only the original matching preview and starts separately',async()=>{
 const {writes}=transport({wrongCodex:true}),onRun=await open();await click('[data-development-tool="codex-profile"]');
 await change('[data-codex-model]','model-a');await change('[data-codex-effort]','high');await change('[data-codex-notice]','40000');
 await click('.discussion-development-consent input');expect(host.querySelector<HTMLButtonElement>('[data-development-create]')!.disabled).toBe(false);expect(writes).toHaveLength(0);
 await click('[data-development-create]');expect(writes).toHaveLength(1);expect(writes[0].body).toMatchObject({tool_profile_id:codex.id,codex_execution_selection:{model:'model-a',reasoning_effort:'high',token_notice_limit:40000}});expect(writes[0].body).not.toHaveProperty('model_execution_selection');expect(writes[0].body).not.toHaveProperty('knowledge');expect(host.querySelector('[data-development-start]')).toBeNull();expect(onRun).not.toHaveBeenCalled();
 await click('[data-development-restore]');expect(writes).toHaveLength(2);expect(writes[1]).toEqual(writes[0]);expect(host.querySelector<HTMLButtonElement>('[data-development-start]')?.disabled).toBe(true);expect(host.textContent).toContain('model-a');expect(host.textContent).toContain('hard');
 await click('.discussion-development-consent input');await click('[data-development-start]');expect(writes).toHaveLength(3);expect(onRun).toHaveBeenCalledExactlyOnceWith('development-run');expect(writes.every(write=>!write.path.includes('/review'))).toBe(true);
});
it('switches from the platform model to a fixed Harness without borrowing the old selection or budget',async()=>{
 const {writes,reads}=transport();await open();await click('[data-development-tool="native-profile"]');await change('[data-model-select]',modelBinding.deployment.deployment_sha256);await click('.discussion-development-consent input');
 await click('[data-development-tool="harness-profile"]');expect(host.querySelector('[data-model-select]')).toBeNull();expect(host.querySelector('[data-codex-model]')).toBeNull();expect(host.querySelector<HTMLInputElement>('.discussion-development-consent input')!.checked).toBe(false);expect(host.querySelector<HTMLButtonElement>('[data-development-create]')!.disabled).toBe(true);
 await click('.discussion-development-consent input');await click('[data-development-create]');expect(writes).toHaveLength(1);expect(writes[0].body.tool_profile_id).toBe(harness.id);expect(writes[0].body).not.toHaveProperty('model_execution_selection');expect(writes[0].body).not.toHaveProperty('codex_execution_selection');expect(reads.filter(path=>path.startsWith('/model-execution/options?'))).toHaveLength(1);expect(host.querySelector<HTMLButtonElement>('[data-development-start]')?.disabled).toBe(true);
});
