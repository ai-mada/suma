// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {ExpertTeamRole,ModelExecutionOptions} from '../../../packages/contracts/src';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import {ExpertModelSelections,expertModelScope,type ExpertModelTask} from './ExpertModelSelections';
const sha='a'.repeat(64),first='b'.repeat(64),second='c'.repeat(64);
const role:ExpertTeamRole={key:'developer',title:'Coding expert',resource_id:'resource-a',resource_version:1,resource_kind:'agent',resource_name:'Agent',membership_id:'member-a',membership_version:1,workflow_id:'workflow-a',workflow_version:1,agent_node_id:'code',tool_profile_id:'tool-a',tool_profile_version:2,tool_profile_sha256:sha,tool_name:'Native tool',knowledge:[]};
const tasks:ExpertModelTask[]=[{taskID:'task-a',title:'Task A',role},{taskID:'task-b',title:'Task B',role}];
const session={user:{id:'user-a',name:'Owner',email:'owner@example.test'},organization:{id:'org-a',name:'Org'},projects:[]};
function options():ModelExecutionOptions{return {project_id:'project-a',tool_profile_id:'tool-a',tool_profile_version:2,tool_profile_sha256:sha,observed_at:'2026-10-27T00:00:00Z',supported:true,reason:'',items:[first,second].map((hash,i)=>({deployment:{deployment_id:'model-'+i,revision:1,deployment_sha256:hash,protocol:'openai-chat-completions-v1',model:'Model '+i,endpoint_origin:'http://127.0.0.1:1234',endpoint_config_sha256:'d'.repeat(64),context_window:8192},cost_state:'unpriced',default_parameters:{max_steps:2,max_output_tokens:100},input_token_reservation:16384,output_token_reservation:100}))};}
let root:Root|undefined,host:HTMLDivElement;const changed=vi.fn();
beforeEach(async()=>{await setLocale('en-US',{persist:false});changed.mockClear();host=document.createElement('div');document.body.append(host);vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.stubGlobal('fetch',vi.fn(async()=>Response.json(options())));});
afterEach(async()=>{if(root)await act(async()=>root!.unmount());root=undefined;host.remove();vi.unstubAllGlobals();await setLocale('zh-CN',{persist:false});});
async function settle(){for(let i=0;i<3;i++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function render(values=tasks){await act(async()=>{root??=createRoot(host);root.render(<ApiViewProvider session={session}><ExpertModelSelections project="project-a" tasks={values} disabled={false} onChange={changed}/></ApiViewProvider>);});await settle();}
async function choose(index:number,hash:string){const node=host.querySelectorAll<HTMLSelectElement>('[data-model-select]')[index];await act(async()=>{node.value=hash;node.dispatchEvent(new Event('change',{bubbles:true}));});await settle();}
it('reads once per role, binds separate task requests and supports an explicit task override',async()=>{
 await render();expect(fetch).toHaveBeenCalledTimes(1);expect(changed.mock.lastCall?.[0].ready).toBe(false);
 await choose(0,first);let result=changed.mock.lastCall?.[0];expect(result.scope).toBe(expertModelScope('project-a',tasks));expect(result.ready).toBe(true);expect(result.selections['task-a'].deployment_sha256).toBe(first);expect(result.selections['task-b'].deployment_sha256).toBe(first);
 const override=host.querySelector<HTMLInputElement>('[data-expert-model-task="task-b"] input')!;
 await act(async()=>override.click());await settle();expect(fetch).toHaveBeenCalledTimes(2);expect(changed.mock.lastCall?.[0].ready).toBe(false);
 await choose(1,second);result=changed.mock.lastCall?.[0];expect(result.ready).toBe(true);expect(result.selections['task-a'].deployment_sha256).toBe(first);expect(result.selections['task-b'].deployment_sha256).toBe(second);
 await act(async()=>override.click());await settle();expect(changed.mock.lastCall?.[0].selections['task-b'].deployment_sha256).toBe(first);
});
it('does not inherit a choice across changed role/tool versions and preserves language-neutral selections',async()=>{
 await render();await choose(0,first);await act(async()=>setLocale('zh-CN',{persist:false}));expect(host.textContent).toContain('模型与本次额度');expect(changed.mock.lastCall?.[0].ready).toBe(true);
 await render(tasks.map(task=>({...task,role:{...role,tool_profile_version:3}})));expect(changed.mock.lastCall?.[0].ready).toBe(false);expect(host.querySelector('[data-model-select]')).toBeNull();
});
