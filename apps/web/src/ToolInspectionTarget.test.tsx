// @vitest-environment jsdom
import {act,useState} from 'react';
import {webcrypto} from 'node:crypto';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,expect,it,vi} from 'vitest';
import type {ToolInspection,ToolProfile} from '../../../packages/contracts/src';
import {api,type Session} from './api';
import * as ApiContext from './api-context';
import {ApiViewProvider,bindApiView} from './api-context';
import {setLocale} from './i18n';
import {ToolWorkspace} from './ToolWorkspace';
import {toolInspectionSourceDigest} from './tool-inspection-recovery-model';

const session:Session={user:{id:'user-a',name:'Manager',email:'manager@example.test'},organization:{id:'org-a',name:'Organization'},projects:[]};
const maintenance={state:'supported' as const,protocol:'suma-tool-maintenance-v1',instance_id:'original-owner',observed_at:'2026-11-01T00:00:00Z',checked_at:'2026-11-01T00:00:20Z',expires_at:'2026-11-01T00:01:00Z'};
const toolA:ToolProfile={id:'tool-a',project_id:'p',worker_id:'worker-original',name:'Current configuration A',version:4,state:'paused',profile:{command_id:'command-a',adapter:'codex',protocol:'codex-jsonl-v1',profile_sha256:'a'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'1.0',read_only:false},enabled_by:'',created_at:'2026-11-01T00:00:00Z',updated_at:'2026-11-01T00:00:00Z',available:false,reason:'Profile paused',maintenance};
const toolB:ToolProfile={...toolA,id:'tool-b',name:'Original configuration B',version:8,profile:{...toolA.profile,command_id:'command-b',profile_sha256:'c'.repeat(64)}};
const inspection:ToolInspection={id:'inspection-b',project_id:'p',worker_id:'worker-original',tool_profile_id:'tool-b',tool_profile_version:7,profile_sha256:'c'.repeat(64),command_id:'command-b',version:3,fence:1,state:'unknown',claim_id:'claim-original',claimed_at:'2026-11-01T00:00:10Z',lease_until:'2026-11-01T00:01:10Z',requested_by:'user-a',created_at:'2026-11-01T00:00:00Z',expires_at:'2026-11-01T00:05:00Z',result_code:'probe_timeout'};
const report=(tool:ToolProfile)=>({tool,checked_at:'2026-11-01T00:00:20Z',expires_at:'2026-11-01T00:01:00Z',checks:['authorization','worker','inventory','profile'].map(code=>({code,status:'pass',reason:'',next_action:'none'})),next_action:'none',maintenance_resolution:{profile_id:tool.id,profile_version:tool.version,worker_id:tool.worker_id,action:'view_recovery',reason:'recovery_requested',inspection_id:inspection.id,recovery_id:'recovery-original'}});
let root:Root|undefined,host:HTMLDivElement|undefined;
afterEach(async()=>{if(root)await act(async()=>root!.unmount());root=undefined;host?.remove();document.body.innerHTML='';sessionStorage.clear();vi.restoreAllMocks();vi.unstubAllGlobals();});
async function settle(){for(let i=0;i<5;i++)await act(async()=>{await new Promise(done=>setTimeout(done,0));});}
async function click(selector:string){const button=document.querySelector<HTMLButtonElement>(selector);expect(button,selector).not.toBeNull();await act(async()=>button!.click());await settle();}
function View(){const[selected,onSelect]=useState('');return <ApiViewProvider session={session}><ToolWorkspace project="p" session={session} selected={selected} onSelect={onSelect} refresh={0} panel=""/></ApiViewProvider>;}

it('T199 opens the exact cross-profile maintenance record and isolates late callers, current refusal and an original local receipt',async()=>{
 await setLocale('en-US',{persist:false});sessionStorage.clear();vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.stubGlobal('crypto',webcrypto);
 let caller=bindApiView(session,api),hold=false,deny=false,finish!:(response:Response)=>void;
 vi.spyOn(ApiContext,'useApi').mockImplementation(()=>caller);
 const requests:{path:string;method:string}[]=[],sourceSHA=await toolInspectionSourceDigest(inspection);
 vi.stubGlobal('fetch',vi.fn(async(input:string,init:RequestInit={})=>{
  const path=new URL(String(input),'http://local.test').pathname.replace('/api/v1',''),method=init.method||'GET';requests.push({path,method});
  if(method!=='GET')return Response.json({error:{code:'unexpected_write',message:'Navigation creates no operation'}},{status:400});
  if(path==='/tool-profiles')return Response.json({items:[toolA],total:1});
  if(path==='/tool-profiles/tool-a'||path==='/tool-profiles/tool-b')return Response.json(path.endsWith('tool-a')?toolA:toolB);
  if(path.endsWith('/diagnostics'))return Response.json(report(path.includes('/tool-a/')?toolA:toolB));
  if(path==='/release-workers')return Response.json({items:[{id:'worker-row',worker_id:toolA.worker_id,status:'active',version:1}]});
  if(path==='/project-execution/runners')return Response.json({items:[],as_of:'2026-11-01T00:00:20Z'});
  if(path==='/tool-inspections/inspection-b'){
   if(hold)return new Promise<Response>(done=>{finish=done;});
   if(deny)return Response.json({error:{code:'forbidden',message:'Original record access denied'}},{status:403});
   return Response.json(inspection);
  }
  if(path==='/tool-inspection-recoveries/recovery-original')return Response.json({id:'recovery-original',project_id:'p',worker_id:inspection.worker_id,inspection:{inspection_id:inspection.id,claim_id:inspection.claim_id,fence:inspection.fence},inspection_version:inspection.version,source_sha256:sourceSHA,version:1,state:'requested',requested_by:'user-a',created_at:'2026-11-01T00:02:00Z',expires_at:'2026-11-01T00:07:00Z'});
  return Response.json({items:[]});
 }));
 host=document.createElement('div');document.body.append(host);root=createRoot(host);const render=async()=>{await act(async()=>root!.render(<View/>));await settle();};
 await render();await click('.tool-resource');await click('[data-tool-doctor-maintenance-action="view_recovery"]');
 expect(requests.some(request=>request.path==='/tool-inspections/inspection-b')).toBe(true);expect(requests.some(request=>request.path==='/tool-profiles/tool-b/diagnostics')).toBe(true);
 expect(document.querySelector('[data-tool-reinspection-source]')?.textContent).toContain('tool-b');expect(document.querySelector('[data-tool-reinspection-state]')?.getAttribute('data-tool-reinspection-state')).toBe('unknown');
 expect(document.querySelector('[data-tool-recovery-state]')?.getAttribute('data-tool-recovery-state')).toBe('requested');expect(document.querySelector('.tool-detail')?.textContent).toContain(toolB.name);expect(requests.every(request=>request.method==='GET')).toBe(true);
 await click('[data-tool-reinspection-defer]');await click('.tool-resource');hold=true;await click('[data-tool-doctor-maintenance-action="view_recovery"]');
 caller=bindApiView(session,api);await render();hold=false;await act(async()=>finish(Response.json(inspection)));await settle();expect(document.querySelector('[data-tool-reinspection-panel]')).toBeNull();
 deny=true;await click('[data-tool-doctor-maintenance-action="view_recovery"]');expect(document.querySelector('[data-tool-reinspection-panel]')).toBeNull();expect(document.querySelector('.tool-detail')?.textContent).not.toContain(toolB.name);
 const base=JSON.stringify(['org-a','user-a','p','tool-a']),originalTool={...toolA,version:3},request={path:'/tool-profiles/tool-a/inspections',method:'POST',idempotency_key:'original-local-199',body:{project_id:'p',expected_version:3,idempotency_key:'original-local-199'}},metadata=JSON.stringify({scope:{project:'p',toolID:'tool-a',userID:'user-a',organizationID:'org-a'},tool:originalTool,request});
 sessionStorage.setItem('suma-tool-reinspection-intent:'+base,metadata);sessionStorage.setItem('suma-write-recovery-v1:tool-reinspection:'+base,JSON.stringify(request));const lookupCount=requests.filter(item=>item.path==='/tool-inspections/inspection-b').length;
 await click('[data-tool-doctor-maintenance-action="view_recovery"]');expect(document.querySelector('[data-tool-reinspection-restore]')).not.toBeNull();expect(document.querySelector('[data-tool-reinspection-source]')?.textContent).toContain('tool-a');
 expect(requests.filter(item=>item.path==='/tool-inspections/inspection-b')).toHaveLength(lookupCount);expect(sessionStorage.getItem('suma-tool-reinspection-intent:'+base)).toBe(metadata);expect(requests.every(item=>item.method==='GET')).toBe(true);
});
