// @vitest-environment jsdom
import {act,useMemo} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {ToolProfileDiagnostics} from '../../../packages/contracts/src';
import {ProfileDecisionForm} from './ToolProfileDecision';
import {ApiViewProvider,useApi} from './api-context';
import type {Session} from './api';
import {setLocale} from './i18n';
import {useToolAction} from './tool-hooks';
import type {ToolProfile} from './tool-model';
import {ToolWriteRecovery} from './tool-write';

const session:Session={
 user:{id:'owner',name:'Owner',email:'owner@example.test'},
 organization:{id:'org',name:'Organization'},projects:[],
};
const checked='2026-11-06T00:00:00Z';
const tool:ToolProfile={
 id:'original-profile',project_id:'project',worker_id:'runner',name:'Team Codex',version:4,state:'enabled',
 profile:{command_id:'codex-team',adapter:'codex',protocol:'codex-jsonl-v1',profile_sha256:'a'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'1.0',read_only:false},
 enabled_by:'owner',created_at:checked,updated_at:checked,available:false,reason:'',
};
const diagnosticPath='/api/v1/tool-profiles/original-profile/diagnostics?project_id=project';
const commandPath='/api/v1/tool-profiles/original-profile/commands';
function report(source=tool):ToolProfileDiagnostics{
 return {
  tool:source,checked_at:checked,expires_at:'2026-11-06T00:00:01Z',
  checks:(['authorization','worker','inventory','profile'] as const).map(code=>({code,status:'pass',reason:'',next_action:'none'})),
  next_action:'none',
  maintenance_resolution:{
   profile_id:source.id,profile_version:source.version,worker_id:source.worker_id,
   action:'open_worker',reason:'maintenance_unknown',
  },
  impact:{
   scope:'active_project_runs',observed_at:checked,coverage:'complete',runs_examined:1,nodes_examined:1,
   observed_reference_count:1,items_truncated:false,reason:'',
   items:[{run_id:'active-run',node_id:'develop',run_state:'running',node_state:'running',worker_id:'runner',attempt:1,fence:1}],
  },
 };
}
const refusal=(status:number)=>Response.json({error:{code:status===409?'version_conflict':'forbidden',message:'Cannot use the previous configuration'}},{status});
let host:HTMLDivElement,root:Root;
const onSaved=vi.fn();
function View({decision}:{decision:'pause'|'revoke'}){
 const api=useApi();
 const recovery=useMemo(()=>new ToolWriteRecovery('profile-decision-test',sessionStorage,api),[api]);
 const action=useToolAction(recovery);
 return <ProfileDecisionForm project={tool.project_id} value={tool} decision={decision} action={action} onSaved={onSaved} onCancel={()=>{}}/>;
}
function mount(decision:'pause'|'revoke'){
 return act(async()=>root.render(<ApiViewProvider session={session}><View decision={decision}/></ApiViewProvider>));
}
const checkbox=()=>host.querySelector<HTMLInputElement>('[data-tool-decision-confirm]');
const locked=()=>!checkbox()||checkbox()!.disabled;
const impact=()=>host.querySelector('[data-tool-decision-impact] [data-tool-profile-impact]');
async function confirm(){
 const input=checkbox();
 expect(input?.disabled).toBe(false);
 await act(async()=>input!.click());
}
async function refresh(){
 const button=host.querySelector<HTMLButtonElement>('[data-tool-decision-refresh]');
 expect(button?.disabled).toBe(false);
 await act(async()=>button!.click());
}
async function submit(){
 await act(async()=>host.querySelector('form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));
}
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});
 sessionStorage.clear();
 onSaved.mockClear();
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
 host=document.createElement('div');
 document.body.append(host);
 root=createRoot(host);
});
afterEach(async()=>{
 await act(async()=>root.unmount());
 host.remove();
 sessionStorage.clear();
 vi.useRealTimers();
 vi.unstubAllGlobals();
});

it('rechecks exact impact and resets consent before a new versioned removal after a conflict',async()=>{
 vi.useFakeTimers({toFake:['setTimeout','clearTimeout','performance']});
 const writes:Record<string,unknown>[]=[];
 let resolveInitial!:(response:Response)=>void;
 const firstRead=new Promise<Response>(resolve=>{resolveInitial=resolve;});
 const latest={...tool,version:5};
 const removed:ToolProfile={...tool,version:6,state:'revoked'};
 const fetcher=vi.fn(async(input:string,init:RequestInit)=>{
  if(init.method==='GET'){
   expect(input).toBe(diagnosticPath);
   expect(init.body).toBeUndefined();
   return writes.length?Response.json(report(latest)):firstRead;
  }
  expect(input).toBe(commandPath);
  expect(init.method).toBe('POST');
  writes.push(JSON.parse(String(init.body)));
  return writes.length===1?refusal(409):Response.json(removed);
 });
 vi.stubGlobal('fetch',fetcher);

 await mount('revoke');
 expect(fetcher).toHaveBeenCalledOnce();
 expect(checkbox()?.disabled).toBe(true);
 await submit();
 expect(writes).toEqual([]);
 await act(async()=>resolveInitial(Response.json(report())));
 expect(impact()?.textContent??'').toContain('1 reference');
 expect(checkbox()?.parentElement?.textContent).toContain('v4');

 await confirm();
 await submit();
 expect(onSaved).not.toHaveBeenCalled();
 expect(writes).toEqual([{project_id:'project',action:'revoke',expected_version:4,idempotency_key:expect.any(String)}]);
 await refresh();
 expect(checkbox()?.checked).toBe(false);
 expect(checkbox()?.parentElement?.textContent).toContain('v5');
 await submit();
 expect(writes).toHaveLength(1);

 await confirm();
 await submit();
 expect(writes).toHaveLength(2);
 expect(writes[1]).toEqual({project_id:'project',action:'revoke',expected_version:5,idempotency_key:expect.any(String)});
 expect(writes[1].idempotency_key).not.toBe(writes[0].idempotency_key);
 expect(onSaved).toHaveBeenCalledExactlyOnceWith(removed);
 expect(fetcher.mock.calls.filter(([,init])=>init.method==='GET')).toHaveLength(2);
});

it('expires consent and keeps pause locked after denial or mismatched evidence',async()=>{
 vi.useFakeTimers({toFake:['setTimeout','clearTimeout','performance']});
 let result=()=>Response.json(report());
 const fetcher=vi.fn(async(input:string,init:RequestInit)=>{
  expect(input).toBe(diagnosticPath);
  expect(init.method).toBe('GET');
  return result();
 });
 vi.stubGlobal('fetch',fetcher);
 await mount('pause');
 expect(impact()?.getAttribute('data-coverage')).toBe('complete');
 await confirm();

 await act(async()=>vi.advanceTimersByTimeAsync(1001));
 expect(locked()).toBe(true);
 await submit();
 expect(fetcher).toHaveBeenCalledOnce();

 result=()=>refusal(403);
 await refresh();
 expect(locked()).toBe(true);
 expect(impact()?.textContent??'').not.toContain('active-run');

 result=()=>Response.json(report({...tool,worker_id:'other-runner'}));
 await refresh();
 expect(locked()).toBe(true);
 expect(impact()?.textContent??'').not.toContain('active-run');
 await submit();

 result=()=>Response.json(report({...tool,version:5,state:'paused'}));
 await refresh();
 expect(locked()).toBe(true);
 await submit();
 expect(fetcher).toHaveBeenCalledTimes(4);
 expect(onSaved).not.toHaveBeenCalled();
});

it('hides reviewed impact and prevents a second command when access changes at submission',async()=>{
 vi.useFakeTimers({toFake:['setTimeout','clearTimeout','performance']});
 const writes:Record<string,unknown>[]=[];
 let allowed=true;
 const fetcher=vi.fn(async(input:string,init:RequestInit)=>{
  if(init.method==='GET'){
   expect(input).toBe(diagnosticPath);
   return allowed?Response.json(report()):refusal(403);
  }
  expect(input).toBe(commandPath);
  expect(init.method).toBe('POST');
  writes.push(JSON.parse(String(init.body)));
  return refusal(403);
 });
 vi.stubGlobal('fetch',fetcher);
 await mount('revoke');
 await confirm();
 allowed=false;

 await submit();
 expect(writes).toEqual([{project_id:'project',action:'revoke',expected_version:4,idempotency_key:expect.any(String)}]);
 expect(impact()).toBeNull();
 expect(locked()).toBe(true);
 await submit();
 expect(writes).toHaveLength(1);

 await refresh();
 expect(impact()).toBeNull();
 expect(locked()).toBe(true);
 allowed=true;
 await refresh();
 expect(impact()?.getAttribute('data-coverage')).toBe('complete');
 expect(checkbox()?.checked).toBe(false);
 await submit();
 expect(writes).toHaveLength(1);
 expect(onSaved).not.toHaveBeenCalled();
});
