// @vitest-environment jsdom
import {act,useState} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ToolWorkspace} from './ToolWorkspace';
import {serializePoolToolRecoveryPin,createPoolToolRecoveryPin,type PoolToolRecoveryPin} from './execution-pools/pool-tool-recovery-model';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import type {Session} from './api';
import type {ToolProfile} from './tool-model';
import type {ToolConnectDraft} from './ToolQuickConnect';
import type {ToolReportFeedbackSelection} from './tool-report-feedback-model';

// Only the downstream boundary is mocked here. Production Workspace, Doctor,
// current API parsing, public drafts and original write recovery stay real.
vi.mock('./ToolQuickConnect',()=>({ToolQuickConnect:({draft,replacementSource}:{draft?:ToolConnectDraft;replacementSource?:ToolProfile})=><div data-test-quick data-worker={draft?.workerID} data-command={draft?.commandID} data-source={replacementSource?.id}/> }));
vi.mock('./ToolSetupGuide',()=>({ToolSetupGuide:({id,expanded,expectedWorkerID,onReportSelection}:{id?:string;expanded?:boolean;expectedWorkerID?:string;onReportSelection?:(value:ToolReportFeedbackSelection)=>void})=><details id={id} className="tool-cli-setup" open={expanded}><summary tabIndex={0}>Original guide</summary><details data-remote-runner-setup><summary tabIndex={0}>Original node setup</summary><input name="remote-runner-worker" value={expectedWorkerID??''} readOnly/></details><button data-test-wrong-report onClick={()=>onReportSelection?.({...selection,workerID:'decoy'})}>Wrong report target</button><button data-test-report onClick={()=>onReportSelection?.(selection)}>Use exact report</button></details>}));

const session:Session={user:{id:'u',name:'Owner',email:'owner@example.test',role:'owner'},organization:{id:'o',name:'Org'},projects:[]};
const at='2026-11-03T00:00:00Z';
const tool:ToolProfile={id:'existing',project_id:'p',worker_id:'pool-node-a',name:'Current Codex tool',version:4,state:'enabled',profile:{command_id:'codex-team',adapter:'codex',protocol:'codex-jsonl-v1',profile_sha256:'a'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'reviewed',read_only:false},enabled_by:'u',created_at:at,updated_at:at,available:false,reason:'inventory_missing'};
const sharedPage={items:[{id:'node-a',pool_id:'shared-pool-a',worker_id:'pool-node-a',status:'active',name:'Shared node A',credential_expires_at:'2026-12-03T00:00:00Z'}],as_of:at};
const adapter={id:'codex-jsonl',product:'Codex',entrypoint_kind:'cli',direction:'suma_to_tool',implementation:'implemented',setup_kind:'runner_profile',adapter:'codex',protocol:'codex-jsonl-v1',capabilities:[],source_refs:[],limitations:[]};
const selection:ToolReportFeedbackSelection={project:'p',workerID:'runner',adapterID:'codex-jsonl',commandID:'codex-team',profileSHA256:'c'.repeat(64),observedAt:at};
let root:Root,host:HTMLDivElement,reads:string[],writes:string[],pins:PoolToolRecoveryPin[]=[],sourceVersion=4,workerBlocked:boolean,inspectionUnknown:boolean,sharedFailure:boolean,deferredCatalog:boolean,catalogRelease:((value:Response)=>void)|undefined;

function diagnostic(){return {tool,checked_at:at,expires_at:'2026-11-03T00:00:40Z',maintenance_resolution:{profile_id:tool.id,profile_version:tool.version,worker_id:tool.worker_id,...(inspectionUnknown?{action:'view_inspection',reason:'inspection_unknown',inspection_id:'inspection-original'}:{action:'open_worker',reason:workerBlocked?'worker_unavailable':'maintenance_unknown'})},checks:[{code:'authorization',status:'pass',reason:'Authorization current',next_action:'none'},{code:'worker',status:workerBlocked?'blocked':'pass',reason:workerBlocked?'Worker credential revoked':'Worker current',next_action:workerBlocked?'open_worker':'none'},{code:'inventory',status:'blocked',reason:'inventory_missing',next_action:'open_worker'},{code:'profile',status:'not_checked',reason:'Await current inventory',next_action:'none'}],next_action:'open_worker'};}
function View({active=session,selected='existing',panel=''}:{active?:Session;selected?:string;panel?:string}){const[value,onSelect]=useState(selected);return <ApiViewProvider session={active}><ToolWorkspace project="p" session={active} selected={selected||value} onSelect={onSelect} refresh={0} panel={panel} onPoolRecovery={pin=>pins.push(pin)}/></ApiViewProvider>;}
async function settle(){await act(async()=>{for(let i=0;i<16;i++)await Promise.resolve();await new Promise(resolve=>setTimeout(resolve,25));});}
async function draw(active=session,selected='existing',panel=''){await act(async()=>root.render(<View active={active} selected={selected} panel={panel}/>));await settle();}
async function click(selector:string){const element=document.querySelector<HTMLButtonElement>(selector)!;expect(element,selector).not.toBeNull();expect(element.disabled).not.toBe(true);await act(async()=>{element.focus();element.click();});await settle();}

beforeEach(async()=>{
 await setLocale('en-US',{persist:false});sessionStorage.clear();vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
 vi.stubGlobal('requestAnimationFrame',(callback:FrameRequestCallback)=>setTimeout(()=>callback(performance.now()),0));vi.stubGlobal('cancelAnimationFrame',clearTimeout);
 vi.spyOn(HTMLElement.prototype,'getClientRects').mockImplementation(()=>[{width:100,height:44}] as unknown as DOMRectList);
 host=document.createElement('div');host.id='main';host.tabIndex=-1;document.body.append(host);root=createRoot(host);
 reads=[];writes=[];pins=[];sourceVersion=4;workerBlocked=false;inspectionUnknown=false;sharedFailure=false;deferredCatalog=false;catalogRelease=undefined;
 vi.stubGlobal('fetch',vi.fn(async(input:string,init:RequestInit={})=>{
  const path=new URL(String(input),'http://local.test').pathname.replace('/api/v1','');
  if((init.method||'GET')!=='GET'){writes.push(path);return Response.json({error:{code:'unexpected_write',message:'No mutation authorized in this navigation fixture'}},{status:409});}
  reads.push(path);
  if(path==='/tool-profiles')return Response.json({items:[tool],total:1});
  if(path==='/tool-profiles/existing')return Response.json({...tool,version:sourceVersion});
  if(path==='/tool-profiles/other')return Response.json({...tool,id:'other',worker_id:'decoy'});
  if(path==='/tool-profiles/existing/diagnostics')return Response.json(diagnostic());
  if(path.endsWith('/replacements'))return Response.json({project_id:'p',profile_id:'existing',direction:'outgoing',items:[]});
  if(path==='/release-workers')return Response.json({items:['runner','decoy'].map((worker_id,index)=>({id:'identity-'+index,project_id:'p',worker_id,status:'active',version:1,expires_at:'2026-12-03T00:00:00Z'}))});
  if(path==='/project-execution/runners')return sharedFailure?Response.json({error:{code:'unavailable',message:'Shared source unavailable'}},{status:503}):Response.json(sharedPage);
  if(path==='/adapter-catalog'){if(deferredCatalog)return new Promise<Response>(resolve=>{catalogRelease=resolve;});return Response.json({project_id:'p',as_of:at,items:[adapter]});}
  return Response.json({error:{code:'unavailable',message:'No current fixture source'}},{status:503});
 }));
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();sessionStorage.clear();vi.unstubAllGlobals();vi.restoreAllMocks();});


it('T218 carries the selected shared profile to its exact owner locator without ordinary pairing or mutations',async()=>{
 await draw();await click('[data-tool-doctor-maintenance-action="open_worker"]');
 const manage=host.querySelector<HTMLButtonElement>('[data-pool-tool-recovery-open="pool-node-a"]');expect(manage).not.toBeNull();
 await click('[data-pool-tool-recovery-open="pool-node-a"]');
 expect(pins).toEqual([createPoolToolRecoveryPin(tool,'o',sharedPage)]);
 expect(document.querySelector('[data-remote-runner-setup]')).toBeNull();expect(writes).toEqual([]);
});
it('T218 returns to the exact old profile source and blocks changed or expired metadata rather than rebinding',async()=>{
 const pin=createPoolToolRecoveryPin(tool,'o',sharedPage)!,panel=serializePoolToolRecoveryPin(pin);
 await draw(session,'existing',panel);
 const quick=document.querySelector<HTMLElement>('[data-test-quick]');expect(quick?.dataset.source).toBe(tool.id);expect(quick?.dataset.worker).toBe(tool.worker_id);expect(quick?.dataset.command).toBe(tool.profile.command_id);
 await act(async()=>root.unmount());root=createRoot(host);sourceVersion=5;await draw(session,'existing',panel);
 expect(document.querySelector('[data-test-quick]')).toBeNull();
 await act(async()=>root.unmount());root=createRoot(host);sourceVersion=4;deferredCatalog=true;
 const clock=vi.spyOn(performance,'now');clock.mockReturnValue(1000);await draw(session,'existing',panel);expect(catalogRelease).toBeDefined();
 clock.mockReturnValue(42000);await act(async()=>catalogRelease!(Response.json({project_id:'p',as_of:at,items:[adapter]})));await settle();clock.mockRestore();
 expect(document.querySelector('[data-test-quick]')).toBeNull();
 // Catalog completion must not outlive a newer failed shared-source read.
 await act(async()=>root.unmount());root=createRoot(host);clock.mockRestore();
 deferredCatalog=true;sharedFailure=false;catalogRelease=undefined;await draw(session,'existing',panel);expect(catalogRelease).toBeDefined();
 sharedFailure=true;await click('button[aria-label="Refresh tool configurations"]');
 await act(async()=>catalogRelease!(Response.json({project_id:'p',as_of:at,items:[adapter]})));await settle();
 expect(document.querySelector('[data-test-quick]')).toBeNull();expect(writes).toEqual([]);
});
