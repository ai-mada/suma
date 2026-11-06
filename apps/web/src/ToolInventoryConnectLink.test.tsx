// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ToolInventoryConnectLink} from './ToolInventoryConnectLink';
import {ToolWorkspace} from './ToolWorkspace';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import {parseWorkbenchRoute} from './navigation';
import type {Session} from './api';
import type {AdapterDescriptor} from './AdapterConnections';

const project='p-team',worker='codex-runner',command='codex-team';
const observed='2026-11-03T08:00:00Z',latest='2026-11-03T08:00:10Z';
const digest='a'.repeat(64),changedDigest='c'.repeat(64);
const profile={command_id:command,adapter:'codex',protocol:'codex-jsonl-v1',profile_sha256:digest,executable_sha256:'b'.repeat(64),tool_version:'1.0',read_only:false};
const adapter:AdapterDescriptor={
 id:'codex-jsonl',product:'Codex',entrypoint_kind:'cli',direction:'suma_to_tool',
 implementation:'implemented',setup_kind:'runner_profile',adapter:'codex',protocol:'codex-jsonl-v1',
 capabilities:[],source_refs:[],limitations:[],
};
const session:Session={user:{id:'u',name:'Owner',email:'owner@example.test',role:'owner'},organization:{id:'o',name:'Org'},projects:[]};
const panel=(scope=project)=>`tool-inventory-connect:${scope}:${worker}:${command}:${digest}:${observed}`;
it('keeps the inventory locator only on an unambiguous tools route',()=>{
 const route=new URLSearchParams({project,view:'settings',sub:'tools',panel:panel()});
 expect(parseWorkbenchRoute('?'+route).panel).toBe(panel());
 for(const key of ['project','view','sub','panel']){
  const duplicate=new URLSearchParams(route);
  duplicate.append(key,route.get(key)!);
  expect(parseWorkbenchRoute('?'+duplicate).panel).toBe('');
 }
 for(const [key,value] of [['sub','organization'],['record','existing-profile']]){
  const different=new URLSearchParams(route);
  different.set(key,value);
  expect(parseWorkbenchRoute('?'+different).panel).toBe('');
 }
});
const buttonSelector='[data-tool-inventory-connect-check]';
const observation=(hash=digest)=>({available:true,reason:'',profile:{...profile,profile_sha256:hash}});
function fresh(hash=digest){
 return {project_id:project,worker_id:worker,state:'fresh',as_of:'2026-11-03T08:00:30Z',observed_at:latest,
  expires_at:'2026-11-03T08:01:10Z',items:[observation(hash)]};
}
function stale(){
 return {...fresh(),state:'stale',as_of:'2026-11-03T08:01:10Z',items:[]};
}
type RequestRecord={path:string;params:URLSearchParams;method:string};
let root:Root,host:HTMLDivElement,requests:RequestRecord[],inventory:()=>ReturnType<typeof fresh>;
const onReview=vi.fn();
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
 host=document.createElement('div');document.body.append(host);root=createRoot(host);
 onReview.mockReset();requests=[];inventory=()=>fresh();
 vi.spyOn(Date,'now').mockReturnValue(Date.parse('2026-11-03T08:00:30Z'));
 vi.stubGlobal('fetch',vi.fn(async(input:string,init:RequestInit={})=>{
  const url=new URL(input,'http://suma.test'),path=url.pathname.replace(/^\/api\/v1/,'');
  requests.push({path,params:url.searchParams,method:init.method??'GET'});
  if(path==='/tool-discovery')return Response.json(inventory());
  if(path==='/adapter-catalog')return Response.json({project_id:project,as_of:latest,items:[adapter]});
  if(path==='/tool-profiles')return Response.json({items:[],total:0});
  if(path==='/release-workers')return Response.json({
   credential_management_allowed:true,
   items:[{id:'worker-identity',project_id:project,worker_id:worker,version:1,status:'active',deployment:false,created_at:observed,expires_at:'2026-11-04T08:00:00Z'}],
  });
  if(path==='/project-execution/runners')return Response.json({items:[],as_of:latest});
  throw new Error('Unexpected request '+path);
 }));
});
afterEach(async()=>{
 await act(async()=>root.unmount());
 host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();
});
async function renderLink(value=panel()){
 await act(async()=>root.render(<ApiViewProvider session={session}><ToolInventoryConnectLink project={project} panel={value} disabled={false} onReview={onReview}/></ApiViewProvider>));
}
async function checkLink(){
 const button=host.querySelector<HTMLButtonElement>(buttonSelector);
 expect(button?.disabled).toBe(false);
 await act(async()=>button?.click());
}
const exactReads=()=>requests.filter(read=>read.path==='/tool-discovery'||read.path==='/adapter-catalog');

it('hands off the exact current available configuration only after explicit review',async()=>{
 await renderLink();
 expect(exactReads()).toEqual([]);
 expect(host.querySelector(buttonSelector)?.textContent).toContain('Review this tool');
 await checkLink();
 expect(exactReads().map(({path,params,method})=>[path,params.get('project_id'),params.get('worker_id'),method])).toEqual([
  ['/tool-discovery',project,worker,'GET'],['/adapter-catalog',project,null,'GET'],
 ]);
 expect(onReview).toHaveBeenCalledExactlyOnceWith({
  adapter,
  draft:{adapterID:adapter.id,workerID:worker,commandID:command,name:'',nameEdited:false,observationPin:{profileSHA256:digest,observedAt:latest}},
 });
 expect(requests).toHaveLength(2);
});

it('rejects foreign project, expired evidence and a changed configuration without any write or fallback',async()=>{
 await renderLink(panel('p-other'));
 expect(host.querySelector('[role="alert"]')).not.toBeNull();
 expect(host.querySelector(buttonSelector)).toBeNull();
 expect(exactReads()).toEqual([]);

 inventory=stale;
 await renderLink();
 await checkLink();
 expect(host.querySelector('[role="alert"]')).not.toBeNull();
 expect(onReview).not.toHaveBeenCalled();

 inventory=()=>fresh(changedDigest);
 await checkLink();
 expect(host.querySelector('[role="alert"]')).not.toBeNull();
 expect(onReview).not.toHaveBeenCalled();
 expect(exactReads().filter(read=>read.path==='/tool-discovery').every(read=>read.params.get('worker_id')===worker)).toBe(true);
 expect(requests.every(read=>read.method==='GET')).toBe(true);
});

it('rechecks the same pin in Quick Connect before allowing paused registration',async()=>{
 let inventoryReads=0;
 inventory=()=>fresh(++inventoryReads===1?digest:changedDigest);
 await act(async()=>root.render(
  <ApiViewProvider session={session}>
   <ToolWorkspace project={project} session={session} selected="" panel={panel()} onSelect={()=>{}} refresh={0}/>
  </ApiViewProvider>
 ));
 expect(exactReads()).toEqual([]);
 await checkLink();
 const dialog=document.querySelector('[role="dialog"]');
 expect(dialog?.querySelector<HTMLSelectElement>('[data-quick-tool]')?.value).toBe(adapter.id);
 expect(dialog?.querySelector('[data-quick-observation-changed]')).not.toBeNull();
 expect(dialog?.querySelector<HTMLButtonElement>('[data-quick-register]')?.disabled).toBe(true);
 expect(exactReads().filter(read=>read.path==='/tool-discovery').map(read=>read.params.get('worker_id'))).toEqual([worker,worker]);
 expect(requests.every(read=>read.method==='GET')).toBe(true);
});
