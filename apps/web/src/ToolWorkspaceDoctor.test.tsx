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
const tool:ToolProfile={id:'existing',project_id:'p',worker_id:'runner',name:'Original team tool',version:4,state:'enabled',profile:{command_id:'codex-team',adapter:'codex',protocol:'codex-jsonl-v1',profile_sha256:'a'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'1.0',read_only:false},enabled_by:'u',created_at:'2026-10-30T08:00:00Z',updated_at:'2026-10-30T08:00:00Z',available:false,reason:'Original authorization requires review'};
let root:Root|undefined,host:HTMLDivElement|undefined;
function View(){const [selected,onSelect]=useState('');return <ApiViewProvider session={session}><ToolWorkspace project="p" session={session} selected={selected} onSelect={onSelect} refresh={0} panel=""/></ApiViewProvider>;}
async function settle(){await act(async()=>{for(let i=0;i<24;i++)await Promise.resolve();});}
afterEach(async()=>{if(root)await act(async()=>root!.unmount());host?.remove();document.body.innerHTML='';vi.unstubAllGlobals();vi.restoreAllMocks();sessionStorage.clear();});

it('opens original versioned review for an enabled tool and locates only its exact existing worker without writing',async()=>{
 await setLocale('en-US',{persist:false});sessionStorage.clear();vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
 const reads:string[]=[],writes:string[]=[];let workerIssue=false;
 vi.stubGlobal('fetch',vi.fn(async(input:string,init:RequestInit={})=>{
  const path=new URL(String(input),'http://local.test').pathname.replace('/api/v1','');
  if((init.method||'GET')!=='GET'){writes.push(path);return Response.json({error:{code:'unexpected_write',message:'Unexpected write'}},{status:400});}
  reads.push(path);
  if(path==='/tool-profiles')return Response.json({items:[tool],total:1});
  if(path==='/tool-profiles/existing')return Response.json(tool);
  if(path==='/tool-profiles/existing/diagnostics')return Response.json({tool,checked_at:'2026-10-30T08:00:20Z',expires_at:'2026-10-30T08:01:00Z',checks:[
   {code:'authorization',status:workerIssue?'pass':'blocked',reason:workerIssue?'':tool.reason,next_action:workerIssue?'none':'review_profile'},
   {code:'worker',status:workerIssue?'blocked':'pass',reason:workerIssue?'Worker credential needs review':'',next_action:workerIssue?'open_worker':'none'},
   {code:'inventory',status:'pass',reason:'',next_action:'none'},
   {code:'profile',status:'pass',reason:'',next_action:'none'}],next_action:workerIssue?'open_worker':'review_profile'});
  if(path==='/release-workers')return Response.json({items:['other-runner','runner'].map((worker_id,i)=>({id:'worker'+i,worker_id,status:'active',version:1,expires_at:'2026-10-31T08:00:00Z'}))});
  if(path==='/project-execution/runners')return Response.json({items:[],as_of:'2026-10-30T08:00:20Z'});
  if(path==='/adapter-catalog')return Response.json({project_id:'p',as_of:'2026-10-30T08:00:20Z',items:[]});
  return Response.json({items:[]});
 }));
 host=document.createElement('div');document.body.append(host);root=createRoot(host);await act(async()=>root!.render(<View/>));await settle();
 await act(async()=>host!.querySelector<HTMLButtonElement>('.tool-resource')!.click());await settle();
 expect(reads).toContain('/tool-profiles/existing/diagnostics');
 const review=host.querySelector<HTMLButtonElement>('[data-tool-doctor-action="review_profile"]');expect(review).not.toBeNull();await act(async()=>review!.click());await settle();
 const dialog=document.querySelector('[role="dialog"]');expect(dialog?.textContent).toContain('configuration v4');
 const checkbox=dialog?.querySelector<HTMLInputElement>('input[type="checkbox"]');expect(checkbox?.checked).toBe(false);expect(dialog?.querySelector<HTMLButtonElement>('.dialog-actions button.primary')?.disabled).toBe(true);expect(writes).toEqual([]);
 const cancel=[...dialog!.querySelectorAll<HTMLButtonElement>('button')].find(button=>button.textContent?.trim()==='Cancel');expect(cancel).toBeDefined();await act(async()=>cancel!.click());await settle();
 workerIssue=true;const refresh=host.querySelector<HTMLButtonElement>('[data-tool-doctor-refresh]');expect(refresh).not.toBeNull();await act(async()=>refresh!.click());await settle();
 const worker=host.querySelector<HTMLButtonElement>('[data-tool-doctor-action="open_worker"]');expect(worker).not.toBeNull();await act(async()=>worker!.click());
 expect(document.activeElement).toBe(host.querySelector('[data-tool-worker="runner"]'));expect(document.activeElement).not.toBe(host.querySelector('[data-tool-worker="other-runner"]'));
 expect(document.querySelector('[role="dialog"]')).toBeNull();expect(writes).toEqual([]);
});
