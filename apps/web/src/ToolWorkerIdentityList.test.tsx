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
const manualID='a'.repeat(32),manualWorker='manual-worker',oneTimeToken='b'.repeat(64);
const rotationPath='/release-workers/'+manualID+'/rotate-credential';
let rotate:((body:Record<string,unknown>)=>Promise<Response>)|undefined;
let canManage:boolean;
let workerRead:(()=>Promise<Response>)|undefined;
let revokeRefused:boolean;
function manualNode(expires_at='2026-11-03T09:00:00Z') {records.push({...identity(manualID,'active',expires_at),worker_id:manualWorker,manual_rotation_allowed:true});}
function rotationReceipt(body:Record<string,unknown>,token?:string) {
 const worker={...records.find(row=>row.id===manualID)!,version:4,expires_at:'2026-11-04T08:00:00Z',profiles:[]};
 records=records.map(row=>row.id===manualID?worker:row);
 return Response.json({operation_id:body.idempotency_key,worker,...(token?{token}:{})});
}
async function settle(){await act(async()=>{for(let index=0;index<12;index++)await Promise.resolve();});}
async function draw(refresh=0){await act(async()=>root.render(<ToolWorkspace project="p" session={session} selected="" onSelect={()=>{}} refresh={refresh} panel=""/>));await settle();}
async function click(selector:string){const button=host.querySelector<HTMLButtonElement>(selector)??document.querySelector<HTMLButtonElement>(selector);expect(button).not.toBeNull();await act(async()=>button!.click());await settle();}
const maintenance=(worker:string)=>'[data-tool-worker-maintenance="'+worker+'"]';
const visibleIDs=()=>[...host.querySelectorAll<HTMLElement>('[data-tool-worker]:not([hidden])')].map(row=>row.dataset.toolWorker);
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.spyOn(Date,'now').mockReturnValue(at);sessionStorage.clear();writes=[];rotate=undefined;canManage=true;workerRead=undefined;revokeRefused=false;
 records=[identity('team-codex-cli'),identity('expired-node','active','2026-11-03T08:00:00Z'),identity('team-codex','revoked'),identity('unknown-node','active','invalid-time')];
 host=document.createElement('div');document.body.append(host);root=createRoot(host);
 vi.stubGlobal('fetch',vi.fn(async(input:string,init:RequestInit={})=>{
  const path=String(input).replace('/api/v1','').split('?')[0];
  if(init.method&&init.method!=='GET'){
   const body=JSON.parse(String(init.body));writes.push({path,method:init.method,body});
   if(path===rotationPath&&rotate)return rotate(body);
   if(revokeRefused)return Response.json({error:{code:'forbidden',message:'Credential access changed'}},{status:403});
   const id=decodeURIComponent(path.split('/')[2]);records=records.map(row=>row.id===id?{...row,status:'revoked',version:row.version+1}:row);
   return Response.json(records.find(row=>row.id===id));
  }
  if(path==='/release-workers')return workerRead?.()??Response.json({items:records,credential_management_allowed:canManage});
  return Response.json(path==='/tool-profiles'?{items:[],total:0}:path==='/project-execution/runners'?{items:[],as_of:'2026-11-03T08:00:00Z'}:{});
 }));
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();sessionStorage.clear();vi.unstubAllGlobals();vi.restoreAllMocks();});

it('keeps only current credentials in the default list and offers expired or unknown identities for cleanup without scanning them',async()=>{
 await draw();expect(visibleIDs()).toEqual(['team-codex-cli']);
 expect(host.querySelector('[data-tool-worker-scan="expired-node"]')).toBeNull();
 await click('[data-tool-worker-filter="attention"]');expect(visibleIDs()).toEqual(['expired-node','unknown-node']);
 expect(host.querySelector('[data-tool-worker-revoke="expired-node"]')).toBeNull();
 await click(maintenance('expired-node'));
 expect(host.querySelector<HTMLButtonElement>('[data-tool-worker-revoke="expired-node"]')?.disabled).toBe(false);
 expect(host.querySelector('[data-tool-worker-counts]')?.textContent).toContain('4');
 await click('[data-tool-worker-filter="revoked"]');expect(visibleIDs()).toEqual(['team-codex']);
 expect(writes).toEqual([]);
});

it('uses the original confirmed revoke command and moves its server-confirmed result to history',async()=>{
 await draw();
 const toggle=host.querySelector<HTMLButtonElement>(maintenance('team-codex-cli'))!;
 expect(toggle.getAttribute('aria-expanded')).toBe('false');
 const details=document.getElementById(toggle.getAttribute('aria-controls')!)!;
 expect(details.hidden).toBe(true);
 await click(maintenance('team-codex-cli'));
 expect(toggle.getAttribute('aria-expanded')).toBe('true');
 expect(details.hidden).toBe(false);
 await click('[data-tool-worker-revoke="team-codex-cli"]');expect(writes).toEqual([]);
 const form=document.querySelector<HTMLFormElement>('[role="dialog"] form')!;expect(form).not.toBeNull();
 await act(async()=>form.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));await settle();
 expect(writes).toHaveLength(1);expect(writes[0]).toMatchObject({path:'/release-workers/team-codex-cli',method:'PATCH',body:{project_id:'p',expected_version:3,status:'revoked'}});
 expect(typeof writes[0].body.idempotency_key).toBe('string');expect(visibleIDs()).toEqual([]);
 await click('[data-tool-worker-filter="revoked"]');expect(visibleIDs()).toEqual(['team-codex-cli','team-codex']);
});

it('uses only the server capability to show credential actions and preserves ordinary node actions',async()=>{
 canManage=false;
 manualNode();
 await draw();
 expect(host.querySelector('[data-tool-worker-register],[data-tool-worker-maintenance]')).toBeNull();
 expect(host.querySelector('.tool-worker-identity-access')?.textContent).toContain('Project administrators');
 expect(host.querySelector('[data-tool-worker-connect="team-codex-cli"]')).not.toBeNull();
 expect(host.querySelector('[data-tool-worker-scan="team-codex-cli"]')).not.toBeNull();
 expect(writes).toEqual([]);
 records=[];
 await draw(1);
 expect(host.querySelector('[data-tool-readiness-state="identity"]')?.textContent).toContain('Ask a project administrator');
 expect(host.querySelector('[data-tool-readiness-next]')).toBeNull();
});

it('keeps the open confirmation while refreshing and withdraws it when the server removes the capability',async()=>{
 await draw();
 await click(maintenance('team-codex-cli'));
 await click('[data-tool-worker-revoke="team-codex-cli"]');
 let finish!:(response:Response)=>void;
 workerRead=()=>new Promise(resolve=>{finish=resolve;});
 await draw(1);
 const form=document.querySelector<HTMLFormElement>('[role="dialog"] form')!;
 expect(form.querySelector<HTMLButtonElement>('.primary')?.disabled).toBe(true);
 await act(async()=>form.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));
 expect(writes).toEqual([]);
 await act(async()=>finish(Response.json({items:records,credential_management_allowed:false})));
 await settle();
 expect(document.querySelector('[role="dialog"]')).toBeNull();
 expect(host.querySelector('[data-tool-worker-maintenance]')).toBeNull();
 expect(writes).toEqual([]);
 workerRead=undefined;
 await draw(2);
 expect(host.querySelector(maintenance('team-codex-cli'))?.getAttribute('aria-expanded')).toBe('false');
});

it('closes the confirmation and checks current authority after the original mutation is refused',async()=>{
 await draw();
 await click(maintenance('team-codex-cli'));
 await click('[data-tool-worker-revoke="team-codex-cli"]');
 canManage=false;
 revokeRefused=true;
 await click('[role="dialog"] form .primary');
 expect(writes).toHaveLength(1);
 expect(document.querySelector('[role="dialog"]')).toBeNull();
 expect(host.querySelector('[data-tool-worker-maintenance]')).toBeNull();
 expect(host.querySelector('.tool-worker-identity-access')).not.toBeNull();
 expect(sessionStorage.getItem('suma-tool-recovery-v1:o:u:p:profiles')).toBeNull();
});

it('retains an original expired-node operation target outside the filter and blocks view changes without replaying it',async()=>{
 sessionStorage.setItem('suma-tool-recovery-v1:o:u:p:profiles',JSON.stringify({path:'/release-workers/expired-node',method:'PATCH',idempotency_key:'original-revoke',body:{project_id:'p',expected_version:3,status:'revoked',idempotency_key:'original-revoke'}}));
 await draw();expect(visibleIDs()).toEqual(['team-codex-cli','expired-node']);
 expect(host.querySelector('[data-tool-worker="expired-node"]')?.hasAttribute('data-tool-worker-context')).toBe(true);
 expect(host.querySelector<HTMLButtonElement>('[data-tool-worker-filter="revoked"]')?.disabled).toBe(true);
 expect(host.querySelector<HTMLInputElement>('[data-tool-worker-search]')?.disabled).toBe(true);
 expect(host.querySelector('[data-tool-worker-scan="expired-node"]')).toBeNull();expect(writes).toEqual([]);
});

it('rotates the stopped manual node, presents the one-time secret and copies only on request',async()=>{
 manualNode();rotate=async body=>rotationReceipt(body,oneTimeToken);
 const writeText=vi.fn().mockResolvedValue(undefined);Object.defineProperty(navigator,'clipboard',{value:{writeText},configurable:true});
 await draw();await click(maintenance('team-codex-cli'));
 expect(host.querySelector('[data-tool-worker-rotate="team-codex-cli"]')).toBeNull();
 await click(maintenance(manualWorker));
 expect(host.querySelector(maintenance('team-codex-cli'))?.getAttribute('aria-expanded')).toBe('false');
 await click('[data-tool-worker-rotate="manual-worker"]');
 expect(document.querySelector<HTMLButtonElement>('[data-worker-rotation-submit]')?.disabled).toBe(true);
 expect(writes).toEqual([]);
 await click('[data-worker-rotation-stopped]');await click('[data-worker-rotation-submit]');
 expect(writes).toEqual([{path:rotationPath,method:'POST',body:{project_id:'p',expected_version:3,acknowledge_stopped:true,idempotency_key:expect.any(String)}}]);
 const field=document.querySelector<HTMLInputElement>('[data-worker-rotation-credential] input[type=password]');
 expect(field?.value).toBe(oneTimeToken);expect(writeText).not.toHaveBeenCalled();expect(JSON.stringify(sessionStorage)).not.toContain(oneTimeToken);
 await click('[data-worker-rotation-copy]');expect(writeText).toHaveBeenCalledExactlyOnceWith(oneTimeToken);
 await click('[data-worker-rotation-close]');expect(document.querySelector('[data-worker-rotation-credential]')).toBeNull();
});

it.each([false,true])('keeps the original operation after a lost response, first issue on recovery: %s',async(firstIssueOnRecovery)=>{
 manualNode('2026-11-03T08:00:00Z');rotate=async body=>{if(writes.length===1)throw new TypeError('Response lost');return rotationReceipt(body,firstIssueOnRecovery?oneTimeToken:undefined);};
 await draw();await click('[data-tool-worker-filter="attention"]');await click(maintenance(manualWorker));await click('[data-tool-worker-rotate="manual-worker"]');
 await click('[data-worker-rotation-stopped]');await click('[data-worker-rotation-submit]');
 expect(writes).toHaveLength(1);const raw=sessionStorage.getItem('suma-tool-recovery-v1:o:u:p:profiles');expect(raw).toContain(String(writes[0].body.idempotency_key));
 await act(async()=>root.unmount());root=createRoot(host);await draw();expect(writes).toHaveLength(1);
 expect(host.querySelector('[data-tool-worker="manual-worker"]')?.hasAttribute('data-tool-worker-context')).toBe(true);
 await click('.tool-recovery .primary');expect(writes).toHaveLength(2);expect(writes[1]).toEqual(writes[0]);
 if(firstIssueOnRecovery){
  expect(document.querySelector<HTMLInputElement>('[data-worker-rotation-credential] input[type=password]')?.value).toBe(oneTimeToken);
 }else{
  expect(document.querySelector('[data-worker-rotation-credential]')?.textContent).toContain('one-time credential was not recovered');
  expect(document.querySelector('[data-worker-rotation-copy]')).toBeNull();
 }
 expect(sessionStorage.getItem('suma-tool-recovery-v1:o:u:p:profiles')).toBeNull();
});

it('blocks a pending execution conflict until a current row is reopened and explicitly acknowledged',async()=>{
 manualNode();rotate=async body=>writes.length===1?Response.json({error:{code:'worker_rotation_pending',message:'Pending work'}},{status:409}):rotationReceipt(body);
 await draw();await click(maintenance(manualWorker));await click('[data-tool-worker-rotate="manual-worker"]');await click('[data-worker-rotation-stopped]');await click('[data-worker-rotation-submit]');
 expect(document.querySelector('[data-worker-rotation-submit]')).toBeNull();
 await click('[data-worker-rotation-refresh]');expect(writes).toHaveLength(1);
 await click('[data-tool-worker-rotate="manual-worker"]');expect(document.querySelector<HTMLButtonElement>('[data-worker-rotation-submit]')?.disabled).toBe(true);
 await click('[data-worker-rotation-stopped]');await click('[data-worker-rotation-submit]');
 expect(writes).toHaveLength(2);expect(writes[1].body.idempotency_key).not.toBe(writes[0].body.idempotency_key);
});

it('clears a recovered definitive refusal without starting another command',async()=>{
 manualNode();rotate=async()=>Response.json({error:{code:'worker_rotation_pending',message:'Pending work'}},{status:409});
 sessionStorage.setItem('suma-tool-recovery-v1:o:u:p:profiles',JSON.stringify({path:rotationPath,method:'POST',idempotency_key:'original-rotate',body:{project_id:'p',expected_version:3,acknowledge_stopped:true}}));
 await draw();await click('.tool-recovery .primary');
 expect(writes).toEqual([{path:rotationPath,method:'POST',body:{project_id:'p',expected_version:3,acknowledge_stopped:true,idempotency_key:'original-rotate'}}]);
 expect(sessionStorage.getItem('suma-tool-recovery-v1:o:u:p:profiles')).toBeNull();
 expect(document.querySelector('[data-worker-rotation-credential]')).toBeNull();
});
