// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ToolWorkspace} from './ToolWorkspace';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import type {Session} from './api';
import type {AdapterDescriptor} from './AdapterConnections';
import type {ToolConnectDraft} from './ToolQuickConnect';
import type {ToolProfile} from './tool-model';
import {createPoolToolSetupPin,serializePoolToolSetupPin,type PoolToolSetupPin} from './execution-pools/pool-tool-setup-model';

// Downstream widgets are separate consumers. Here the production Workspace,
// public draft, original preparation selection and API ownership remain real.
vi.mock('./ToolQuickConnect',()=>({ToolQuickConnect:({draft,replacementSource,onAdvanced}:{draft?:ToolConnectDraft;replacementSource?:ToolProfile;onAdvanced:(adapter?:AdapterDescriptor,worker?:string)=>void})=><div data-test-quick data-worker={draft?.workerID} data-adapter={draft?.adapterID} data-source={replacementSource?.id}><button data-test-advanced onClick={()=>onAdvanced(adapter,'pool-machine')}>Prepare current shared tool</button></div>}));
vi.mock('./ToolSetupGuide',()=>({ToolSetupGuide:({expanded,expectedWorkerID,onPrepareSharedPool}:{expanded?:boolean;expectedWorkerID?:string;onPrepareSharedPool?:(tool:'codex-jsonl',current:()=>boolean)=>void})=>expanded?<div data-test-shared-guide data-worker={expectedWorkerID}><button data-test-owner onClick={()=>onPrepareSharedPool?.('codex-jsonl',()=>guideCurrent)}>Continue to current pool</button></div>:null}));
const session:Session={user:{id:'u',name:'Owner',email:'owner@example.test',role:'owner'},organization:{id:'o',name:'Org'},projects:[]};
const at='2026-11-03T00:00:00Z';
const adapter:AdapterDescriptor={id:'codex-jsonl',product:'Codex',entrypoint_kind:'cli',direction:'suma_to_tool',implementation:'implemented',setup_kind:'runner_profile',adapter:'codex',protocol:'codex-jsonl-v1',capabilities:[],source_refs:[],limitations:[]};
const sharedPage={items:[{id:'machine',pool_id:'team-pool',worker_id:'pool-machine',status:'active',name:'Shared machine',credential_expires_at:'2026-12-03T00:00:00Z'}],as_of:at};
let root:Root,host:HTMLDivElement,pins:PoolToolSetupPin[]=[],writes:string[]=[],reads:string[]=[],guideCurrent=true,sharedFailure=false,deferredCatalog=false,protocol='codex-jsonl-v1',release:((value:Response)=>void)|undefined;
async function settle(){await act(async()=>{for(let i=0;i<12;i++)await Promise.resolve();await new Promise(r=>setTimeout(r,20));});}
async function draw(panel=''){await act(async()=>root.render(<ApiViewProvider session={session}><ToolWorkspace session={session} project="p" selected="" onSelect={()=>{}} refresh={0} panel={panel} onPoolSetup={pin=>pins.push(pin)}/></ApiViewProvider>));await settle();}
async function click(selector:string){const button=document.querySelector<HTMLButtonElement>(selector)!;expect(button,selector).not.toBeNull();expect(button.disabled).toBe(false);await act(async()=>button.click());await settle();}
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});sessionStorage.clear();vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
 vi.stubGlobal('requestAnimationFrame',(callback:FrameRequestCallback)=>setTimeout(()=>callback(performance.now()),0));vi.stubGlobal('cancelAnimationFrame',clearTimeout);
 vi.spyOn(HTMLElement.prototype,'getClientRects').mockImplementation(()=>[{width:100,height:44}] as unknown as DOMRectList);
 host=document.createElement('div');host.id='main';host.tabIndex=-1;document.body.append(host);root=createRoot(host);
 pins=[];writes=[];reads=[];guideCurrent=true;sharedFailure=false;deferredCatalog=false;protocol=adapter.protocol!;release=undefined;
 vi.stubGlobal('fetch',vi.fn(async(input:string,init:RequestInit={})=>{
  const path=new URL(String(input),'http://local.test').pathname.replace('/api/v1','');
  if((init.method||'GET')!=='GET'){writes.push(path);return Response.json({error:{code:'unexpected_write',message:'No writes in navigation fixture'}},{status:409});}
  reads.push(path);
  if(path==='/tool-profiles')return Response.json({items:[],total:0});
  if(path==='/release-workers')return Response.json({items:[{id:'ordinary',project_id:'p',worker_id:'ordinary',status:'active',version:1,expires_at:'2026-12-03T00:00:00Z'}]});
  if(path==='/project-execution/runners')return sharedFailure?Response.json({error:{code:'unavailable',message:'Shared source unavailable'}},{status:503}):Response.json(sharedPage);
  if(path==='/adapter-catalog'){if(deferredCatalog)return new Promise<Response>(resolve=>{release=resolve;});return Response.json({project_id:'p',as_of:at,items:[{...adapter,protocol}]});}
  return Response.json({error:{code:'unavailable',message:'No current fixture source'}},{status:503});
 }));
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();sessionStorage.clear();vi.unstubAllGlobals();vi.restoreAllMocks();});
it('T219 opens new shared setup on the exact adapter and worker without profile or replacement authority',async()=>{
 const pin=createPoolToolSetupPin(adapter,'pool-machine','o','p',sharedPage)!;await draw(serializePoolToolSetupPin(pin));
 const quick=document.querySelector<HTMLElement>('[data-test-quick]');expect(quick?.dataset.adapter).toBe('codex-jsonl');expect(quick?.dataset.worker).toBe('pool-machine');expect(quick?.dataset.source).toBeUndefined();
 expect(reads.some(p=>p.startsWith('/tool-profiles/'))).toBe(false);expect(writes).toEqual([]);
 await act(async()=>root.unmount());root=createRoot(host);protocol='another-protocol';await draw(serializePoolToolSetupPin(pin));
 expect(document.querySelector('[data-test-quick]')).toBeNull();expect(writes).toEqual([]);
});
it('T219 carries a shared Guide choice to its current owner and rejects retired or refreshed async sources',async()=>{
 await draw();await click('[data-tools-add]');await click('[data-test-advanced]');expect(host.querySelector<HTMLElement>('[data-test-shared-guide]')?.dataset.worker).toBe('pool-machine');
 await click('[data-test-owner]');expect(pins).toEqual([createPoolToolSetupPin(adapter,'pool-machine','o','p',sharedPage)]);
 deferredCatalog=true;await click('[data-test-owner]');expect(release).toBeDefined();guideCurrent=false;
 await act(async()=>release!(Response.json({project_id:'p',as_of:at,items:[adapter]})));await settle();expect(pins).toHaveLength(1);
 guideCurrent=true;release=undefined;await click('[data-test-owner]');expect(release).toBeDefined();
 sharedFailure=true;await click('button[aria-label="Refresh tool configurations"]');
 await act(async()=>release!(Response.json({project_id:'p',as_of:at,items:[adapter]})));await settle();expect(pins).toHaveLength(1);expect(writes).toEqual([]);
});
