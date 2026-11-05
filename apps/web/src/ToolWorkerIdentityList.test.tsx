// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ToolWorkspace} from './ToolWorkspace';
import {setLocale} from './i18n';
import type {Session} from './api';
import type {ExecutionWorker} from './tool-model';

const session:Session={user:{id:'u',name:'Owner',email:'owner@example.test',role:'owner'},organization:{id:'o',name:'Org'},projects:[]};
const at=Date.parse('2026-11-03T08:00:00Z');
const identity=(id:string,status='active',expires_at='2026-11-03T09:00:00Z'):ExecutionWorker=>({id,project_id:'p',worker_id:id,version:3,status,deployment:false,created_at:'2026-11-03T07:00:00Z',expires_at});
let root:Root,host:HTMLDivElement,records:ExecutionWorker[],writes:{path:string;method:string;body:Record<string,unknown>}[];
async function settle(){await act(async()=>{for(let index=0;index<12;index++)await Promise.resolve();});}
async function draw(){await act(async()=>root.render(<ToolWorkspace project="p" session={session} selected="" onSelect={()=>{}} refresh={0} panel=""/>));await settle();}
async function click(selector:string){const button=host.querySelector<HTMLButtonElement>(selector)??document.querySelector<HTMLButtonElement>(selector);expect(button).not.toBeNull();await act(async()=>button!.click());await settle();}
const visibleIDs=()=>[...host.querySelectorAll<HTMLElement>('[data-tool-worker]:not([hidden])')].map(row=>row.dataset.toolWorker);
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.spyOn(Date,'now').mockReturnValue(at);sessionStorage.clear();writes=[];
 records=[identity('team-codex-cli'),identity('expired-node','active','2026-11-03T08:00:00Z'),identity('team-codex','revoked'),identity('unknown-node','active','invalid-time')];
 host=document.createElement('div');document.body.append(host);root=createRoot(host);
 vi.stubGlobal('fetch',vi.fn(async(input:string,init:RequestInit={})=>{
  const path=String(input).replace('/api/v1','').split('?')[0];
  if(init.method&&init.method!=='GET'){
   const body=JSON.parse(String(init.body));writes.push({path,method:init.method,body});
   const id=decodeURIComponent(path.split('/')[2]);records=records.map(row=>row.id===id?{...row,status:'revoked',version:row.version+1}:row);
   return Response.json(records.find(row=>row.id===id));
  }
  return Response.json(path==='/release-workers'?{items:records}:path==='/tool-profiles'?{items:[],total:0}:path==='/project-execution/runners'?{items:[],as_of:'2026-11-03T08:00:00Z'}:{});
 }));
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();sessionStorage.clear();vi.unstubAllGlobals();vi.restoreAllMocks();});

it('keeps only current credentials in the default list and offers expired or unknown identities for cleanup without scanning them',async()=>{
 await draw();expect(visibleIDs()).toEqual(['team-codex-cli']);
 expect(host.querySelector('[data-tool-worker-scan="expired-node"]')).toBeNull();
 await click('[data-tool-worker-filter="attention"]');expect(visibleIDs()).toEqual(['expired-node','unknown-node']);
 expect(host.querySelector<HTMLButtonElement>('[data-tool-worker-revoke="expired-node"]')?.disabled).toBe(false);
 expect(host.querySelector('[data-tool-worker-counts]')?.textContent).toContain('4');
 await click('[data-tool-worker-filter="revoked"]');expect(visibleIDs()).toEqual(['team-codex']);
 expect(writes).toEqual([]);
});

it('uses the original confirmed revoke command and moves its server-confirmed result to history',async()=>{
 await draw();await click('[data-tool-worker-revoke="team-codex-cli"]');expect(writes).toEqual([]);
 const form=document.querySelector<HTMLFormElement>('[role="dialog"] form')!;expect(form).not.toBeNull();
 await act(async()=>form.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));await settle();
 expect(writes).toHaveLength(1);expect(writes[0]).toMatchObject({path:'/release-workers/team-codex-cli',method:'PATCH',body:{project_id:'p',expected_version:3,status:'revoked'}});
 expect(typeof writes[0].body.idempotency_key).toBe('string');expect(visibleIDs()).toEqual([]);
 await click('[data-tool-worker-filter="revoked"]');expect(visibleIDs()).toEqual(['team-codex-cli','team-codex']);
});

it('retains an original expired-node operation target outside the filter and blocks view changes without replaying it',async()=>{
 sessionStorage.setItem('suma-tool-recovery-v1:o:u:p:profiles',JSON.stringify({path:'/release-workers/expired-node',method:'PATCH',idempotency_key:'original-revoke',body:{project_id:'p',expected_version:3,status:'revoked',idempotency_key:'original-revoke'}}));
 await draw();expect(visibleIDs()).toEqual(['team-codex-cli','expired-node']);
 expect(host.querySelector('[data-tool-worker="expired-node"]')?.hasAttribute('data-tool-worker-context')).toBe(true);
 expect(host.querySelector<HTMLButtonElement>('[data-tool-worker-filter="revoked"]')?.disabled).toBe(true);
 expect(host.querySelector<HTMLInputElement>('[data-tool-worker-search]')?.disabled).toBe(true);
 expect(host.querySelector('[data-tool-worker-scan="expired-node"]')).toBeNull();expect(writes).toEqual([]);
});
