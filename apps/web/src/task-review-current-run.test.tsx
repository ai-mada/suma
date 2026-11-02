// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,expect,it,vi} from 'vitest';
import {App} from './App';
import {setLocale} from './i18n';

let root:Root|undefined,host:HTMLDivElement|undefined;
afterEach(async()=>{if(root)await act(async()=>root!.unmount());root=undefined;host?.remove();vi.restoreAllMocks();vi.unstubAllGlobals();});

// Opening acceptance is a fresh read, while an uncertain acceptance remains
// the original operation. Neither event changes the reviewer's selected run.
it('refreshes a completed linked run on opening review and retains an uncertain review verbatim',async()=>{
 const project='review-current-project',taskID='review-current-task',runID='review-current-run',olderID='review-earlier-run';
 const task={id:taskID,project_id:project,version:3,title:'Fixed review task',description:'Original task scope',status:'in_progress',run_id:runID,data:{},acceptance_criteria:['Independent candidate checks']};
 let state='running',linkedReads=0,failWrite=true;
 const writes:{path:string;body:Record<string,unknown>}[]=[];
 const run=(id:string,runState:string)=>({id,project_id:project,task_id:taskID,workflow_name:'Independent candidate workflow',state:runState,nodes:[],version:4});
 await setLocale('en-US',{persist:false});localStorage.clear();sessionStorage.clear();
 history.replaceState(null,'','/?'+new URLSearchParams({project,view:'development',sub:'tasks',record:taskID}));
 host=document.createElement('div');document.body.append(host);vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{
  const path=new URL(String(input),location.href).pathname.replace(/^\/api\/v1/,'');
  let value:unknown;
  if(init?.body){writes.push({path,body:JSON.parse(String(init.body))});if(failWrite){failWrite=false;return Response.json({error:{code:'uncertain',message:'Original review acknowledgement unavailable'}},{status:500});}value={...task,status:'accepted',version:4};}
  else if(path==='/auth/session')value={user:{id:'review-user',name:'Reviewer',email:'review@example.test',role:'owner'},organization:{id:'review-org',name:'Review organization'},projects:[{id:project,version:1,title:'Review project'}]};
  else if(path==='/overview')value={counts:{tasks:1},activity:[]};
  else if(path==='/tasks')value={items:[task],total:1};
  else if(path==='/task-workspace')value={items:[task],task_plans:[{task_id:task.id,iteration_id:'',plan_version:0}],total:1,as_of:'2026-10-29T00:00:00Z'};
  else if(path==='/tasks/'+taskID)value=task;
  else if(path==='/tasks/'+taskID+'/delivery'){
   const nodes={total:0,waiting:0,running:0,succeeded:0,failed:0,skipped:0,reconciling:0,other:0,limit:20,complete:true,items:[]};
   value={project_id:project,task_id:taskID,task_version:task.version,task_status:task.status,run_basis_state:'current',formal_acceptance:false,run:{run_id:runID,version:4,state,nodes,tests:nodes},expert_plans:{scope:'current_run',total:0,limit:20,complete:true,items:[]},issues:{scope:'current_run',total:0,limit:20,complete:true,items:[]}};
  }
  else if(path==='/runs/'+runID){linkedReads++;value=run(runID,state);}
  else if(path==='/runs')value={items:[run(runID,state),run(olderID,'succeeded')],total:2};
  else if(['/iterations','/requirements','/members'].includes(path))value={items:[],total:0};
  else throw Error('Unexpected request '+path);
  return Response.json(value);
 }));
 async function settle(ready:()=>boolean){for(let i=0;i<50;i++){await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});if(ready())return;}expect(ready()).toBe(true);}
 await act(async()=>{root=createRoot(host!);root.render(<App/>);});
 await settle(()=>linkedReads===1&&!!host?.querySelector('aside.detail-pane'));
 state='succeeded';
 await settle(()=>!!host!.querySelector('[data-task-delivery-toggle]'));
 await act(async()=>host!.querySelector<HTMLButtonElement>('[data-task-delivery-toggle]')!.click());
 await settle(()=>!!host!.querySelector('[data-task-delivery-review]'));
 const open=host!.querySelector<HTMLElement>('[data-task-delivery-review]');expect(open).not.toBeNull();
 await act(async()=>open!.click());
 await settle(()=>linkedReads===2);
 const select=document.querySelector<HTMLSelectElement>('[role=dialog] select[name=run_id]')!;
 expect(select).not.toBeNull();expect(select.value).toBe(runID);
 expect(select.options.namedItem(runID)?.textContent??[...select.options].find(option=>option.value===runID)?.textContent).toContain('Succeeded');
 await act(async()=>{select.value=olderID;select.dispatchEvent(new Event('change',{bubbles:true}));const reason=document.querySelector<HTMLTextAreaElement>('[role=dialog] textarea[name=reason]')!;reason.value='Independent tests reviewed; preserve this exact selection.';reason.dispatchEvent(new Event('input',{bubbles:true}));document.querySelector('[role=dialog] form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true}));});
 await settle(()=>writes.length===1&&!!document.querySelector('[role=dialog] fieldset[disabled]'));
 expect(writes[0].body.run_id).toBe(olderID);expect(writes[0].path).toBe('/tasks/'+taskID+'/review');
 state='failed';
 await act(async()=>document.querySelector('[role=dialog] form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));
 await settle(()=>writes.length===2);
 expect(writes[1]).toEqual(writes[0]);expect(writes[1].body.idempotency_key).toBeTruthy();
});

it('T214 opens the exact acceptance locator in the real App, consumes its URL and does not reopen after an out-of-page task remount',async()=>{
 const project='locator-project',taskID='locator-task',runID='locator-run';
 const task={id:taskID,project_id:project,version:3,title:'Located review task',description:'Current record',status:'in_progress',run_id:runID,data:{},acceptance_criteria:['Independent evidence']};
 const run={id:runID,project_id:project,workflow_name:'Fixed workflow',version:2,state:'succeeded',nodes:[]};
 let writes=0;
 await setLocale('en-US',{persist:false});localStorage.clear();sessionStorage.clear();
 history.replaceState(null,'','/?'+new URLSearchParams({project,view:'development',sub:'tasks',record:taskID,panel:'task-acceptance:3:'+runID}));
 host=document.createElement('div');document.body.append(host);vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{
  const path=new URL(String(input),location.href).pathname.replace(/^\/api\/v1/,'');
  if(init?.body){writes++;return Response.json({error:{code:'unexpected_write',message:'Navigation must not submit Review'}},{status:400});}
  if(path==='/auth/session')return Response.json({user:{id:'locator-user',name:'Reviewer',email:'review@example.test',role:'owner'},organization:{id:'locator-org',name:'Organization'},projects:[{id:project,version:1,title:'Project'}]});
  if(path==='/tasks/'+taskID)return Response.json(task);
  if(path==='/runs/'+runID)return Response.json(run);
  if(path==='/runs')return Response.json({items:[run],total:1});
  if(path==='/task-workspace')return Response.json({items:[],task_plans:[],total:0,as_of:'2026-11-02T00:00:00Z'});
  if(path==='/overview')return Response.json({counts:{tasks:1},activity:[]});
  return Response.json({items:[],total:0});
 }));
 async function settle(ready:()=>boolean){for(let i=0;i<60;i++){await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});if(ready())return;}expect(ready()).toBe(true);}
 await act(async()=>{root=createRoot(host!);root.render(<App/>);});
 await settle(()=>document.querySelector<HTMLSelectElement>('[role=dialog] select[name=run_id]')?.value===runID);
 expect(document.querySelector<HTMLSelectElement>('[role=dialog] select[name=run_id]')?.value).toBe(runID);
 await settle(()=>!new URLSearchParams(location.search).has('panel'));
 expect(new URLSearchParams(location.search).get('record')).toBe(taskID);expect(writes).toBe(0);
 // A real parent/component remount loses local consumer Sets. The consumed
 // route is therefore the authority that prevents the same intent reopening.
 await act(async()=>root!.unmount());root=undefined;
 await act(async()=>{root=createRoot(host!);root.render(<App/>);});
 await settle(()=>!!host?.querySelector('aside.detail-pane'));
 for(let i=0;i<5;i++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});
 expect(document.querySelector('[role=dialog] select[name=run_id]')).toBeNull();expect(writes).toBe(0);
});
