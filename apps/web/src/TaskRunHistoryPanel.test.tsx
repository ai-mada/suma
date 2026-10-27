// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import {TaskRunHistoryPanel} from './TaskRunHistoryPanel';
import {setLocale} from './i18n';
import type {Session} from './api';
const project='project-a',task='repair-task',stamp='2026-10-27T01:00:00Z',session:Session={user:{id:'user-a',name:'Owner',email:'owner@example.test'},organization:{id:'org-a',name:'Org'},projects:[]};
const observed={task_id:task,run_id:'earlier-run',task_version:null,bound_at:null,observed_at:stamp,binding_kind:'observed',plan_id:'old-plan',current:false},bound={task_id:task,run_id:'current-run',task_version:4,bound_at:stamp,observed_at:stamp,binding_kind:'bound',plan_id:'current-plan',current:true};
let root:Root|undefined,host:HTMLDivElement,reads:string[],refused:boolean;
beforeEach(async()=>{await setLocale('en-US',{persist:false});reads=[];refused=false;host=document.createElement('div');document.body.append(host);vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.stubGlobal('fetch',vi.fn(async(input:string)=>{const path=String(input);reads.push(path);if(refused)return Response.json({error:{code:'forbidden',message:'Current task access denied'}},{status:403});return Response.json(path.includes('before=next')?{items:[bound,observed],total:2,next_cursor:''}:{items:[bound],total:2,next_cursor:'next'});}));});
afterEach(async()=>{if(root)await act(async()=>root!.unmount());root=undefined;host.remove();await setLocale('zh-CN',{persist:false});vi.restoreAllMocks();vi.unstubAllGlobals();});
async function settle(){for(let i=0;i<4;i++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function render(){await act(async()=>{root??=createRoot(host);root.render(<ApiViewProvider session={session}><TaskRunHistoryPanel session={session} project={project} taskID={task}/></ApiViewProvider>);});await settle();}
async function click(selector:string){const node=host.querySelector<HTMLElement>(selector);expect(node,selector).not.toBeNull();await act(async()=>node!.click());await settle();}
it('reads exact run and plan history on demand and keeps observed associations distinct from original binding facts',async()=>{
 await render();expect(reads).toHaveLength(0);await click('[data-task-run-history-toggle]');expect(host.querySelector('[data-task-run-history="current-run"] a')?.getAttribute('href')).toContain('run=current-run');await click('[data-task-run-history-more]');
 expect(host.querySelectorAll('[data-task-run-history]')).toHaveLength(2);const previous=host.querySelector('[data-task-run-history="earlier-run"]')!;expect(previous.textContent).toContain('Original binding time and task version were not recorded');expect(previous.querySelector('[data-task-run-plan]')?.getAttribute('href')).toContain('composition%3Aold-plan');
 await act(async()=>setLocale('zh-CN',{persist:false}));expect(previous.textContent).toContain('原绑定时间和任务版本未记录');refused=true;await click('[data-task-run-history-refresh]');expect(host.querySelector('[data-task-run-history="earlier-run"]')).toBeNull();expect(host.textContent).toContain('Current task access denied');
});
