// @vitest-environment jsdom
import {act,useState} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import {TaskDispatchDialog} from './TaskDispatch';
import {setLocale,tr} from './i18n';
import type {KnowledgeSummary} from './knowledge-model';
const project='project-a',session={user:{id:'a'.repeat(32),name:'Owner',email:'a@example.test'},organization:{id:'b'.repeat(32),name:'Team'},projects:[]},task={id:'task-a',project_id:project,version:2,title:'Task',description:'Task text'},workflow={id:'flow-a',name:'Flow',version:1,nodes:[{id:'agent',name:'Develop',type:'agent'},{id:'test',name:'Verify',type:'test',command:['test-command'],config:{candidate_from:'agent'}}],edges:[]},profile={id:'tool-a',project_id:project,worker_id:'worker-a',name:'Tool',version:1,state:'enabled',available:true,reason:'',profile:{command_id:'agent-command',adapter:'suma',protocol:'suma-jsonl-v1',profile_sha256:'d'.repeat(64),executable_sha256:'e'.repeat(64),tool_version:'1',read_only:false}};
let root:Root|undefined,host:HTMLDivElement,writes:any[],entry:KnowledgeSummary,plan:any;
function Harness(){const[id,setID]=useState('new');return <ApiViewProvider session={session}><TaskDispatchDialog project={project} session={session} task={task} planID={id} initialWorkflow={workflow} onPlan={next=>setID(next||'new')} onClose={()=>{}} onRun={()=>{}}/></ApiViewProvider>;}
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});localStorage.clear();sessionStorage.clear();vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);writes=[];
 entry={id:'knowledge-a',project_id:project,version:4,kind:'spec',state:'published',title:'UNREVIEWED draft title',summary:'Unreviewed summary',latest_revision:4,published_revision:3,published_sha256:'c'.repeat(64),published_title:'APPROVED title',published_summary:'Approved summary',has_draft:true,published_eligible:true,eligibility:'current',created_at:'',updated_at:''};
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{
  const path=new URL(String(input),location.href).pathname.replace(/^\/api\/v1/,'');
  if(init?.body){const body=JSON.parse(String(init.body));writes.push(body);plan={id:'plan-a',project_id:project,version:1,state:'prepared',task_id:task.id,task_version:task.version,workflow_id:workflow.id,workflow_version:workflow.version,agent_node_id:'agent',tool_profile_id:profile.id,tool_profile_version:profile.version,tool_profile_sha256:profile.profile.profile_sha256,run_id:'',created_at:'2026-10-25T00:00:00Z',expires_at:'2026-10-26T00:00:00Z',context:{task:{...task,acceptance_criteria:[]},knowledge:body.knowledge.map((ref:any)=>({...ref,kind:'spec',title:'IMMUTABLE approved snapshot',summary:'Fixed summary',content:'Frozen <raw> {{text}}',sources:[{kind:'documents',id:'doc-a',version:2,title:'Source',sha256:'f'.repeat(64)}]}))},workflow,valid:true,reason:''};return Response.json(plan);}
  if(path==='/workflows')return Response.json({items:[workflow]});if(path==='/tool-profiles')return Response.json({items:[profile],total:1});if(path==='/project-knowledge')return Response.json({items:[entry],total:1,next_cursor:''});if(path==='/task-dispatch-plans/plan-a')return Response.json(plan);return Response.json({items:[],total:0});
 }));
});
afterEach(async()=>{if(root)await act(async()=>root?.unmount());root=undefined;host.remove();await setLocale('zh-CN',{persist:false});vi.unstubAllGlobals();vi.restoreAllMocks();});
async function settle(){for(let i=0;i<5;i++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function render(){await act(async()=>{root=createRoot(host);root.render(<Harness/>);});await settle();}
async function click(text:string){const button=[...document.querySelectorAll<HTMLButtonElement>('button')].find(value=>value.textContent?.trim()===text)!;expect(button,text).toBeDefined();await act(async()=>button.click());await settle();}
async function chooseTool(){const select=[...document.querySelectorAll<HTMLSelectElement>('select')].find(value=>[...value.options].some(option=>option.value===profile.id))!;await act(async()=>{Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype,'value')!.set!.call(select,profile.id);select.dispatchEvent(new Event('change',{bubbles:true}));});}

it('starts empty, displays published labels instead of draft text, and pins only explicitly chosen exact references',async()=>{
 await render();const checkbox=document.querySelector<HTMLInputElement>('.knowledge-dispatch-choice input')!;expect(checkbox.checked).toBe(false);expect(document.body.textContent).toContain('APPROVED title');expect(document.body.textContent).not.toContain('UNREVIEWED draft title');await chooseTool();await act(async()=>checkbox.click());await click(tr('development.dispatch.fix_and_preview_execution_plan'));
 expect(writes[0].knowledge).toEqual([{id:entry.id,revision:3,sha256:'c'.repeat(64)}]);expect(document.body.textContent).toContain('IMMUTABLE approved snapshot');expect(document.querySelector('.knowledge-fixed summary')?.textContent).toContain('Revision 3');await act(async()=>{const detail=document.querySelector<HTMLDetailsElement>('.knowledge-fixed details')!;detail.open=true;});expect(document.querySelector('.knowledge-fixed .knowledge-content')?.textContent).toBe('Frozen <raw> {{text}}');
});

it('does not silently replace a selected publication when current eligibility changes',async()=>{
 await render();await chooseTool();await act(async()=>document.querySelector<HTMLInputElement>('.knowledge-dispatch-choice input')!.click());entry={...entry,published_revision:5,published_sha256:'9'.repeat(64),published_title:'NEW PUBLICATION'};await click(tr('knowledge.dispatch.refresh'));
 const preview=[...document.querySelectorAll<HTMLButtonElement>('button')].find(value=>value.textContent?.trim()===tr('development.dispatch.fix_and_preview_execution_plan'))!;expect(preview.disabled).toBe(true);expect(document.body.textContent).toContain('APPROVED title');expect(writes).toHaveLength(0);
 await act(async()=>document.querySelector<HTMLInputElement>('.knowledge-dispatch-choice input')!.click());expect(preview.disabled).toBe(false);await click(tr('development.dispatch.fix_and_preview_execution_plan'));expect(writes[0].knowledge[0]).toEqual({id:entry.id,revision:5,sha256:'9'.repeat(64)});
});
