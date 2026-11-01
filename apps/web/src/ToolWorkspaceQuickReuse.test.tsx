// @vitest-environment jsdom
import {act,useState} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,expect,it,vi} from 'vitest';
import {ToolWorkspace} from './ToolWorkspace';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import type {Session} from './api';
import type {ToolProfile} from './tool-model';

const session:Session={user:{id:'u',name:'Owner',email:'owner@example.test',role:'owner'},organization:{id:'o',name:'Org'},projects:[]};
const definition={command_id:'codex-team',adapter:'codex',protocol:'codex-jsonl-v1',profile_sha256:'a'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'1.0',read_only:false};
const existing:ToolProfile={id:'existing',project_id:'p',worker_id:'runner',name:'Original team tool',version:4,state:'enabled',profile:definition,enabled_by:'u',created_at:'2026-10-30T08:00:00Z',updated_at:'2026-10-30T08:00:00Z',available:false,reason:'inventory_stale'};
const adapter={id:'codex-jsonl',product:'Codex',entrypoint_kind:'cli',direction:'suma_to_tool',implementation:'implemented',setup_kind:'runner_profile',adapter:'codex',protocol:'codex-jsonl-v1',capabilities:[],source_refs:[],limitations:[]};
let root:Root|undefined,host:HTMLDivElement|undefined;
async function settle(){await act(async()=>{for(let i=0;i<24;i++)await Promise.resolve();});}
function View(){const [selected,onSelect]=useState('');return <ApiViewProvider session={session}><ToolWorkspace project="p" session={session} selected={selected} onSelect={onSelect} refresh={0} panel=""/></ApiViewProvider>;}
afterEach(async()=>{if(root)await act(async()=>root!.unmount());host?.remove();document.body.innerHTML='';vi.unstubAllGlobals();vi.restoreAllMocks();sessionStorage.clear();});

it('opens an exact enabled profile from quick connect in the original detail without a registration receipt or business write',async()=>{
 await setLocale('en-US',{persist:false});sessionStorage.clear();vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
 const reads:string[]=[],queries:unknown[]=[],writes:string[]=[];
 vi.stubGlobal('fetch',vi.fn(async(input:string,init:RequestInit={})=>{
  const url=new URL(String(input),'http://local.test'),path=url.pathname.replace('/api/v1',''),method=init.method||'GET';
  if(path==='/tool-profiles/matches'&&method==='POST'){
   queries.push(JSON.parse(String(init.body)));
   return Response.json({project_id:'p',worker_id:'runner',profile:definition,items:[existing],total:1});
  }
  if(method!=='GET'){writes.push(method+' '+path);return Response.json({error:{code:'unexpected_write',message:'Unexpected business write'}},{status:400});}
  reads.push(path);
  if(path==='/tool-profiles')return Response.json({items:[existing],total:1});
  if(path==='/tool-profiles/existing')return Response.json(existing);
  if(path==='/release-workers')return Response.json({items:[{id:'w',worker_id:'runner',status:'active',version:1}]});
  if(path==='/project-execution/runners')return Response.json({items:[],as_of:'2026-10-30T08:00:20Z'});
  if(path==='/adapter-catalog')return Response.json({project_id:'p',as_of:'2026-10-30T08:00:20Z',items:[adapter]});
  if(path==='/tool-discovery')return Response.json({project_id:'p',worker_id:'runner',state:'fresh',as_of:'2026-10-30T08:00:20Z',observed_at:'2026-10-30T08:00:00Z',expires_at:'2026-10-30T08:01:00Z',items:[{profile:definition,available:true,reason:''}]});
  return Response.json({items:[]});
 }));
 host=document.createElement('div');document.body.append(host);root=createRoot(host);await act(async()=>root!.render(<View/>));await settle();
 const quick=host.querySelector<HTMLButtonElement>('.tool-list .list-toolbar button.primary');expect(quick).not.toBeNull();await act(async()=>quick!.click());await settle();
 const select=document.querySelector<HTMLSelectElement>('[data-quick-tool]');expect(select).not.toBeNull();await act(async()=>{Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype,'value')!.set!.call(select,'codex-jsonl');select!.dispatchEvent(new Event('change',{bubbles:true}));});await settle();
 const open=document.querySelector<HTMLButtonElement>('[data-existing-tool-open="existing"]');expect(open).not.toBeNull();expect(open!.disabled).toBe(false);
 expect(queries).toEqual([{project_id:'p',worker_id:'runner',profile:definition}]);expect(writes).toEqual([]);
 expect(document.querySelector('[data-quick-register]')).toBeNull();expect(reads).not.toContain('/tool-profiles/existing');
 await act(async()=>open!.click());await settle();
 expect(document.querySelector('[role="dialog"]')).toBeNull();expect(host.querySelector('.tool-resource.selected')?.textContent).toContain(existing.name);
 expect(reads).toContain('/tool-profiles/existing');expect(host.querySelector('.tool-detail')?.textContent).toContain(existing.name);
 expect(host.querySelector('[data-tool-receipt]')).toBeNull();expect(writes).toEqual([]);
});
