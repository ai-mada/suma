// @vitest-environment jsdom
import {act, useState} from 'react';
import {createRoot, type Root} from 'react-dom/client';
import {afterEach, beforeEach, expect, it, vi} from 'vitest';
import {ToolWorkspace} from './ToolWorkspace';
import {setLocale} from './i18n';
import type {Session} from './api';
import type {ToolProfile} from './tool-model';

const session:Session={user:{id:'u',name:'Owner',email:'owner@example.test',role:'owner'},organization:{id:'o',name:'Org'},projects:[]};
const definition={command_id:'check',adapter:'command',protocol:'text-v1',profile_sha256:'a'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'1.0',read_only:false} as const;
const profile=(id:string,state:ToolProfile['state'],available:boolean):ToolProfile=>({id,project_id:'p',name:id,worker_id:'runner',version:1,state,profile:definition,enabled_by:'u',created_at:'',updated_at:'',available,reason:available?'':'inventory_stale'});
let root:Root,host:HTMLDivElement,items:ToolProfile[],reads:string[],writes:{path:string;body:any}[],fail:boolean,workers:any[];
function View(){const [selected,setSelected]=useState('');return <ToolWorkspace project="p" session={session} selected={selected} onSelect={setSelected} refresh={0} panel=""/>;}
async function settle(){await act(async()=>{for(let i=0;i<12;i++)await Promise.resolve();});}
async function click(text:string){const button=[...document.querySelectorAll<HTMLButtonElement>('button')].find(b=>b.textContent?.trim()===text);expect(button, text).toBeDefined();await act(async()=>button!.click());await settle();}
async function change(element:HTMLInputElement|HTMLTextAreaElement|HTMLSelectElement,value:string){const proto=element instanceof HTMLSelectElement?HTMLSelectElement.prototype:element instanceof HTMLTextAreaElement?HTMLTextAreaElement.prototype:HTMLInputElement.prototype;await act(async()=>{Object.getOwnPropertyDescriptor(proto,'value')!.set!.call(element,value);element.dispatchEvent(new Event(element instanceof HTMLSelectElement?'change':'input',{bubbles:true}));});await settle();}
beforeEach(async()=>{
 await setLocale('zh-CN',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);items=[];reads=[];writes=[];fail=false;workers=[];sessionStorage.clear();
 vi.stubGlobal('fetch',vi.fn(async(input:string,init:RequestInit)=>{
  const path=String(input).replace('/api/v1','').split('?')[0];
  if(init.method!=='GET'){const body=JSON.parse(String(init.body));writes.push({path,body});const saved={...profile('new','paused',false),name:body.name,profile:body.profile};items.push(saved);return new Response(JSON.stringify(saved),{status:200});}
  reads.push(path);if(fail&&path==='/tool-profiles')return new Response(JSON.stringify({error:{code:'unavailable',message:'Connection unavailable'}}),{status:503});
  const data=path==='/tool-profiles'?{items,total:items.length}:path==='/release-workers'?{items:workers,credential_management_allowed:true}:path==='/project-execution/runners'?{items:[],as_of:'2026-10-29T05:00:00Z'}:items.find(item=>path==='/tool-profiles/'+item.id);
  return new Response(JSON.stringify(data??{}),{status:200});
 }));
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();document.body.innerHTML='';vi.useRealTimers();vi.unstubAllGlobals();});
async function render(){await act(async()=>root.render(<View/>));await settle();}

it('offers the first actionable setup step directly from an empty workspace',async()=>{
 await render();expect(host.querySelector('[data-tool-readiness-state="identity"]')?.textContent).toContain('先登记一个执行节点');await click('登记执行节点');expect(document.querySelector('[role=dialog] input[pattern]')).not.toBeNull();expect(writes).toHaveLength(0);
});
it('clears node facts on an actor switch while preserving the old uncertain operation without replay',async()=>{
 workers=[{id:'old-node',worker_id:'old-scope-runner',status:'active',version:1}];
 const key='suma-tool-recovery-v1:o:u:p:profiles',pending=JSON.stringify({path:'/tool-profiles',method:'POST',idempotency_key:'original-operation',body:{project_id:'p',worker_id:'old-scope-runner',name:'Original',profile:definition}});
 sessionStorage.setItem(key,pending);await render();expect(host.textContent).toContain('old-scope-runner');
 const base=vi.mocked(fetch).getMockImplementation()!,pendingReads:((response:Response)=>void)[]=[];
 vi.mocked(fetch).mockImplementation(async(input,init)=>/\/(?:release-workers|project-execution\/runners)\?/.test(String(input))?new Promise(resolve=>pendingReads.push(resolve)):base(input,init));
 await act(async()=>root.render(<ToolWorkspace project="p" session={{...session,user:{...session.user,id:'other-user'}}} selected="" onSelect={()=>{}} refresh={0} panel=""/>));await settle();
 expect(host.textContent).not.toContain('old-scope-runner');expect(sessionStorage.getItem(key)).toBe(pending);expect(writes).toHaveLength(0);
 await act(async()=>pendingReads.forEach(resolve=>resolve(Response.json({items:[],as_of:'2026-10-29T05:00:00Z'}))));await settle();expect(host.textContent).not.toContain('old-scope-runner');expect(writes).toHaveLength(0);
});
it('filters actual dispatch eligibility within the loaded window and submits public search explicitly',async()=>{
 items=[profile('ready','enabled',true),profile('review','paused',false),profile('offline','enabled',false)];await render();
 const select=host.querySelector<HTMLSelectElement>('[data-tools-window-filter]')!;
 await change(select,'available');expect([...host.querySelectorAll('.tool-resource strong')].map(e=>e.textContent)).toEqual(['ready']);
 await change(select,'attention');expect([...host.querySelectorAll('.tool-resource strong')].map(e=>e.textContent)).toEqual(['offline']);
 await change(select,'all');await change(host.querySelector('.filter-toolbar input')!,'command');expect(host.querySelectorAll('.tool-resource')).toHaveLength(3);
 await act(async()=>host.querySelector<HTMLFormElement>('[data-tools-search]')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));await settle();expect(new URLSearchParams(window.location.search).get('tools_query')).toBe('command');
});
it('never presents retained availability as current after a failed refresh',async()=>{
 items=[profile('ready','enabled',true)];await render();fail=true;
 await act(async()=>host.querySelector<HTMLButtonElement>('[aria-label="刷新工具配置"]')!.click());await settle();
 expect(host.querySelector('.tool-readiness')?.textContent).toContain('状态待确认');expect(host.querySelector('.tool-readiness')?.textContent).not.toContain('可派发 1');
 expect(host.querySelector('.tool-resource')?.textContent).not.toContain('最近读取时可派发');
});
it('keeps old availability unconfirmed while retrying a failed read',async()=>{
 items=[profile('ready','enabled',true)];await render();fail=true;
 await act(async()=>host.querySelector<HTMLButtonElement>('[aria-label="刷新工具配置"]')!.click());await settle();fail=false;
 const base=vi.mocked(fetch).getMockImplementation()!;let release!:(response:Response)=>void;
 vi.mocked(fetch).mockImplementation(async(input,init)=>String(input).includes('/tool-profiles?')?new Promise(resolve=>{release=resolve;}):base(input,init));
 await act(async()=>host.querySelector<HTMLButtonElement>('[aria-label="刷新工具配置"]')!.click());await settle();
 expect(host.querySelector('.tool-readiness')?.textContent).toContain('状态待确认');expect(host.querySelector('.tool-resource')?.textContent).not.toContain('本次读取可派发');
 await act(async()=>release(new Response(JSON.stringify({items,total:1}),{status:200})));await settle();expect(host.querySelector('.tool-resource')?.textContent).toContain('本次读取可派发');
});
it('retains user-authored names and requires a choice when multiple inventory entries are available',async()=>{
 await render();await click('登记配置');const textarea=document.querySelector<HTMLTextAreaElement>('.tool-form textarea')!;
 const input=document.querySelector<HTMLInputElement>('.tool-form input[maxlength="200"]')!;await change(input,'My checks');
 await change(textarea,JSON.stringify({items:[{profile:definition,available:true,reason:''},{profile:{...definition,command_id:'other'},available:true,reason:''}]}));
 expect(input.value).toBe('My checks');expect(document.querySelector('.tool-form .tool-context')).toBeNull();expect(document.querySelector<HTMLButtonElement>('.dialog-actions .primary')!.disabled).toBe(true);expect(writes).toHaveLength(0);
});
it('switches language while preserving filters and an unsaved registration draft',async()=>{
 await render();await click('登记配置');const input=document.querySelector<HTMLInputElement>('.tool-form input[maxlength="200"]')!;await change(input,'Team owned name');
 await act(async()=>setLocale('en-US',{persist:false}));expect(input.value).toBe('Team owned name');expect(document.querySelector('[role=dialog]')?.textContent).toContain('Save as paused configuration');expect(writes).toHaveLength(0);
});
it('refreshes profiles, identities and the selected detail together',async()=>{
 items=[profile('ready','enabled',true)];await render();await act(async()=>host.querySelector<HTMLButtonElement>('.tool-resource')!.click());await settle();reads=[];
 await act(async()=>host.querySelector<HTMLButtonElement>('[aria-label="刷新工具配置"]')!.click());await settle();
 expect(reads).toEqual(expect.arrayContaining(['/tool-profiles','/release-workers','/project-execution/runners','/tool-profiles/ready']));
});
it('pauses and resumes all periodic reads without changing any configuration',async()=>{
 vi.useFakeTimers({toFake:['setInterval','clearInterval']});items=[profile('ready','enabled',true)];await render();await act(async()=>host.querySelector<HTMLButtonElement>('.tool-resource')!.click());await settle();await click('暂停自动刷新');reads=[];
 await act(async()=>{await vi.advanceTimersByTimeAsync(20000);});expect(reads).toEqual([]);
 await click('恢复自动刷新');reads=[];await act(async()=>{await vi.advanceTimersByTimeAsync(10000);});await settle();expect(reads).toContain('/release-workers');expect(reads).toContain('/tool-profiles/ready');expect(writes).toHaveLength(0);
});
it('validates the inventory on paste, selects its sole available entry and suggests an editable name without writing',async()=>{
 workers=[{id:'w',worker_id:'runner',status:'active',version:1}];await render();await click('登记配置');
 const textarea=document.querySelector<HTMLTextAreaElement>('.tool-form textarea')!;
 await change(textarea,JSON.stringify({items:[{profile:definition,available:true,reason:''}]}));
 const form=document.querySelector('.tool-form')!;const input=form.querySelector<HTMLInputElement>('input[maxlength="200"]')!;
 expect(input.value).toBe('check');expect(form.textContent).toContain('已识别 1 项配置');expect(writes).toHaveLength(0);
 const selects=form.querySelectorAll('select');expect([...selects].some(s=>s.value==='check')).toBe(true);
 await change(selects[0],'runner');await change(input,'Team checks');
 await act(async()=>form.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));await settle();expect(writes).toHaveLength(1);expect(writes[0].body).toMatchObject({project_id:'p',worker_id:'runner',name:'Team checks',profile:definition});
});
it('invalidates the parsed selection when inventory content becomes invalid',async()=>{
 await render();await click('登记配置');const textarea=document.querySelector<HTMLTextAreaElement>('.tool-form textarea')!;
 await change(textarea,JSON.stringify({items:[{profile:definition,available:true,reason:''}]}));await change(textarea,'{"items":');
 expect(document.querySelector('.tool-form .tool-context')).toBeNull();expect(document.querySelector<HTMLButtonElement>('.dialog-actions .primary')!.disabled).toBe(true);expect(writes).toHaveLength(0);
});
it('imports a bounded local JSON file without uploading it and shows the paused-save receipt',async()=>{
 workers=[{id:'w',worker_id:'runner',status:'active',version:1}];await render();await click('登记配置');
 const file=new File(['inventory'],'tools.json',{type:'application/json'});Object.defineProperty(file,'text',{value:async()=>JSON.stringify({items:[{profile:definition,available:true,reason:''}]})});
 const input=document.querySelector<HTMLInputElement>('[data-tool-inventory-file]');expect(input).not.toBeNull();Object.defineProperty(input!,'files',{configurable:true,value:[file]});await act(async()=>input!.dispatchEvent(new Event('change',{bubbles:true})));await settle();
 expect(document.querySelector<HTMLInputElement>('.tool-form input[maxlength="200"]')!.value).toBe('check');expect(writes).toHaveLength(0);
 await change(document.querySelector<HTMLSelectElement>('.tool-form select')!,'runner');await act(async()=>document.querySelector('.tool-form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));await settle();
 expect(host.querySelector('[data-tool-receipt]')?.textContent).toContain('已登记配置');expect(host.querySelector('[data-tool-receipt]')?.textContent).toContain('暂停');expect(writes).toHaveLength(1);
});
it('keeps newer typed content when a previous file read finishes late',async()=>{
 await render();await click('登记配置');let finish!:(text:string)=>void;const file=new File(['inventory'],'tools.json');Object.defineProperty(file,'text',{value:()=>new Promise(resolve=>{finish=resolve;})});
 const input=document.querySelector<HTMLInputElement>('[data-tool-inventory-file]');expect(input).not.toBeNull();Object.defineProperty(input!,'files',{configurable:true,value:[file]});await act(async()=>input!.dispatchEvent(new Event('change',{bubbles:true})));
 const text=JSON.stringify({items:[{profile:{...definition,command_id:'typed'},available:true,reason:''}]});await change(document.querySelector<HTMLTextAreaElement>('.tool-form textarea')!,text);await act(async()=>finish(JSON.stringify({items:[{profile:definition,available:true,reason:''}]})));await settle();
 expect(document.querySelector<HTMLTextAreaElement>('.tool-form textarea')!.value).toBe(text);expect(document.querySelector<HTMLInputElement>('.tool-form input[maxlength="200"]')!.value).toBe('typed');expect(writes).toHaveLength(0);
});
it('rejects oversized files before reading them and explains the next missing registration step',async()=>{
 await render();await click('登记配置');const file=new File(['x'],'tools.json');Object.defineProperty(file,'size',{value:262145});const read=vi.fn();Object.defineProperty(file,'text',{value:read});
 const input=document.querySelector<HTMLInputElement>('[data-tool-inventory-file]');expect(input).not.toBeNull();Object.defineProperty(input!,'files',{configurable:true,value:[file]});await act(async()=>input!.dispatchEvent(new Event('change',{bubbles:true})));await settle();
 expect(read).not.toHaveBeenCalled();expect(document.querySelector('[data-tool-import-error]')?.textContent).toContain('256');expect(document.querySelector('[data-tool-form-next]')?.textContent).toContain('执行节点');expect(writes).toHaveLength(0);
});
it('presents current dispatch state before technical fingerprints and collapses setup steps for configured projects',async()=>{
 items=[{...profile('ready','enabled',true),reason:'reported'}];await render();await act(async()=>host.querySelector<HTMLButtonElement>('.tool-resource')!.click());await settle();
 const technical=host.querySelector<HTMLDetailsElement>('[data-tool-technical]');expect(technical).not.toBeNull();expect(technical!.open).toBe(false);expect(technical!.textContent).toContain('a'.repeat(64));
 expect(host.querySelector('.tool-facts .tool-dispatch-status')?.className).toContain('available');expect(host.querySelector('.tool-facts .warning')).toBeNull();expect(host.querySelector<HTMLDetailsElement>('.tool-journey-disclosure')?.open).toBe(false);
});
it('blocks saving the previous profile during a file read and preserves it on a local read failure',async()=>{
 workers=[{id:'w',worker_id:'runner',status:'active',version:1}];await render();await click('登记配置');await change(document.querySelector<HTMLSelectElement>('.tool-form select')!,'runner');
 const original=JSON.stringify({items:[{profile:definition,available:true,reason:''}]});await change(document.querySelector<HTMLTextAreaElement>('.tool-form textarea')!,original);
 let reject!:(error:Error)=>void;const file=new File(['x'],'tools.json');Object.defineProperty(file,'text',{value:()=>new Promise((_resolve,fail)=>{reject=fail;})});const input=document.querySelector<HTMLInputElement>('[data-tool-inventory-file]')!;Object.defineProperty(input,'files',{configurable:true,value:[file]});await act(async()=>input.dispatchEvent(new Event('change',{bubbles:true})));
 expect(document.querySelector<HTMLButtonElement>('.dialog-actions .primary')!.disabled).toBe(true);await act(async()=>document.querySelector('.tool-form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));expect(writes).toHaveLength(0);
 await act(async()=>reject(Error('local read failed')));await settle();expect(document.querySelector<HTMLTextAreaElement>('.tool-form textarea')!.value).toBe(original);expect(document.querySelector('[data-tool-import-error]')?.textContent).toContain('原有草稿已保留');
});
it('shows inline invalid-file diagnostics and clears them when the inventory is corrected',async()=>{
 await render();await click('登记配置');const textarea=document.querySelector<HTMLTextAreaElement>('.tool-form textarea')!;
 await change(textarea,'{"items":');expect(textarea.getAttribute('aria-invalid')).toBe('true');const errorID=textarea.getAttribute('aria-describedby')!.split(' ').at(-1)!;expect(document.getElementById(errorID)?.getAttribute('role')).toBe('alert');
 await change(textarea,JSON.stringify({items:[{profile:definition,available:false,reason:'missing local executable'}]}));expect(textarea.getAttribute('aria-invalid')).toBe('false');expect(document.querySelector('.tool-import-result.needs-attention')).not.toBeNull();expect(document.querySelector<HTMLButtonElement>('.dialog-actions .primary')!.disabled).toBe(true);
});
it('hides an operation receipt from a different project scope',async()=>{
 workers=[{id:'w',worker_id:'runner',status:'active',version:1}];await render();await click('登记配置');await change(document.querySelector<HTMLSelectElement>('.tool-form select')!,'runner');await change(document.querySelector<HTMLTextAreaElement>('.tool-form textarea')!,JSON.stringify({items:[{profile:definition,available:true,reason:''}]}));await act(async()=>document.querySelector('.tool-form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));await settle();expect(host.querySelector('[data-tool-receipt]')).not.toBeNull();
 await act(async()=>root.render(<ToolWorkspace project="other" session={session} selected="" onSelect={()=>{}} refresh={0} panel=""/>));await settle();expect(host.querySelector('[data-tool-receipt]')).toBeNull();
});
it('preserves a name edited while an inventory file is being read',async()=>{
 await render();await click('登记配置');let finish!:(text:string)=>void;const file=new File(['inventory'],'tools.json');Object.defineProperty(file,'text',{value:()=>new Promise(resolve=>{finish=resolve;})});
 const input=document.querySelector<HTMLInputElement>('[data-tool-inventory-file]')!;Object.defineProperty(input,'files',{configurable:true,value:[file]});await act(async()=>input.dispatchEvent(new Event('change',{bubbles:true})));
 await change(document.querySelector<HTMLInputElement>('.tool-form input[maxlength="200"]')!,'Team chosen name');await act(async()=>finish(JSON.stringify({items:[{profile:definition,available:true,reason:''}]})));await settle();expect(document.querySelector<HTMLInputElement>('.tool-form input[maxlength="200"]')!.value).toBe('Team chosen name');
});
it('shows the latest file failure instead of an older JSON parse error',async()=>{
 await render();await click('登记配置');await change(document.querySelector<HTMLTextAreaElement>('.tool-form textarea')!,'{"items":');
 const file=new File(['x'],'tools.json');Object.defineProperty(file,'size',{value:262145});const input=document.querySelector<HTMLInputElement>('[data-tool-inventory-file]')!;Object.defineProperty(input,'files',{configurable:true,value:[file]});await act(async()=>input.dispatchEvent(new Event('change',{bubbles:true})));await settle();expect(document.querySelector('[data-tool-import-error]')?.textContent).toContain('256 KiB');
});

function reported(worker:string,command='detected') {return {project_id:'p',worker_id:worker,state:'fresh',as_of:'2026-10-29T08:00:00Z',observed_at:'2026-10-29T08:00:00Z',expires_at:'2026-10-29T08:01:00Z',items:[{profile:{...definition,command_id:command},available:true,reason:''}]};}
function withDiscovery(read:(worker:string)=>Promise<Response>){const base=vi.mocked(fetch).getMockImplementation()!;vi.mocked(fetch).mockImplementation((input,init)=>{const url=new URL(String(input),'http://local.test');return url.pathname.endsWith('/tool-discovery')?read(url.searchParams.get('worker_id')!):base(input,init);});}
it('automatically reads the selected node and registers its actual sole tool paused without manual JSON',async()=>{
 workers=[{id:'w',worker_id:'runner',status:'active',version:1}];withDiscovery(async worker=>new Response(JSON.stringify(reported(worker)),{status:200}));
 await render();await click('登记配置');await change(document.querySelector<HTMLSelectElement>('.tool-form select')!,'runner');
 expect(document.querySelector<HTMLInputElement>('.tool-form input[maxlength="200"]')!.value).toBe('detected');expect(document.querySelector('.tool-form .tool-context')).not.toBeNull();expect(writes).toHaveLength(0);
 await act(async()=>document.querySelector('.tool-form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));await settle();expect(writes).toHaveLength(1);expect(writes[0].body).toMatchObject({worker_id:'runner',profile:{...definition,command_id:'detected'}});expect(host.querySelector('[data-tool-receipt]')?.textContent).toContain('暂停');
});
it('clears a detected configuration on node change but preserves a manual draft and name until explicit adoption',async()=>{
 workers=[{id:'w',worker_id:'runner',status:'active',version:1},{id:'w2',worker_id:'runner2',status:'active',version:1}];let finish!:(response:Response)=>void;
 withDiscovery(async worker=>worker==='runner'?new Response(JSON.stringify(reported(worker)),{status:200}):new Promise(resolve=>{finish=resolve;}));
 await render();await click('登记配置');const node=document.querySelector<HTMLSelectElement>('.tool-form select')!;await change(node,'runner');expect(document.querySelector('.tool-form .tool-context')).not.toBeNull();
 await change(node,'runner2');expect(document.querySelector('.tool-form .tool-context')).toBeNull();
 const manual=JSON.stringify({items:[{profile:{...definition,command_id:'manual'},available:true,reason:''}]});await change(document.querySelector<HTMLTextAreaElement>('.tool-form textarea')!,manual);await change(document.querySelector<HTMLInputElement>('.tool-form input[maxlength="200"]')!,'My tool');
 await act(async()=>finish(new Response(JSON.stringify(reported('runner2','node-two')),{status:200})));await settle();expect(document.querySelector<HTMLTextAreaElement>('.tool-form textarea')!.value).toBe(manual);expect(document.querySelector<HTMLInputElement>('.tool-form input[maxlength="200"]')!.value).toBe('My tool');
 await act(async()=>document.querySelector<HTMLButtonElement>('[data-tool-discovery-adopt]')!.click());await settle();expect(document.querySelector<HTMLTextAreaElement>('.tool-form textarea')!.value).toContain('node-two');expect(document.querySelector<HTMLInputElement>('.tool-form input[maxlength="200"]')!.value).toBe('My tool');expect(writes).toHaveLength(0);
});

it('blocks registration when adopted discovery expires and preserves the name through an explicit fresh read',async()=>{
 vi.useFakeTimers({toFake:['setTimeout','clearTimeout']});let now=1000;vi.spyOn(performance,'now').mockImplementation(()=>now);
 workers=[{id:'w',worker_id:'runner',status:'active',version:1}];let discoveryReads=0;
 withDiscovery(async worker=>{discoveryReads++;return Response.json(discoveryReads===1?reported(worker):{...reported(worker,'fresh-tool'),as_of:'2026-10-29T08:01:01Z',observed_at:'2026-10-29T08:01:00Z',expires_at:'2026-10-29T08:02:00Z'});});
 await render();await click('登记配置');await change(document.querySelector<HTMLSelectElement>('.tool-form select')!,'runner');
 await change(document.querySelector<HTMLInputElement>('.tool-form input[maxlength="200"]')!,'Team coding tool');
 expect(document.querySelector<HTMLButtonElement>('.tool-form button.primary')!.disabled).toBe(false);
 // An immediate submit must check the original deadline even before the timer renders.
 now=61000;
 await act(async()=>document.querySelector('.tool-form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));await settle();
 expect(writes).toHaveLength(0);
 await act(async()=>vi.advanceTimersByTime(60000));await settle();
 expect(document.querySelector<HTMLButtonElement>('.tool-form button.primary')!.disabled).toBe(true);
 expect(document.querySelector('[data-preflight-check="inventory"]')?.getAttribute('data-preflight-state')).toBe('attention');
 expect(document.querySelector<HTMLInputElement>('.tool-form input[maxlength="200"]')!.value).toBe('Team coding tool');
 expect(discoveryReads).toBe(1);
 // A failed file read must retain the adopted source as well as its text.
 const file=new File(['x'],'replacement.json');Object.defineProperty(file,'text',{value:async()=>{throw Error('cannot read local file');}});
 const input=document.querySelector<HTMLInputElement>('[data-tool-inventory-file]')!;Object.defineProperty(input,'files',{configurable:true,value:[file]});
 await act(async()=>input.dispatchEvent(new Event('change',{bubbles:true})));await settle();
 await act(async()=>document.querySelector('.tool-form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));await settle();
 expect(writes).toHaveLength(0);
 await act(async()=>document.querySelector<HTMLButtonElement>('[data-preflight-review]')!.click());await settle();
 expect(discoveryReads).toBe(2);expect(writes).toHaveLength(0);
 await act(async()=>document.querySelector<HTMLButtonElement>('[data-tool-discovery-adopt]')!.click());await settle();
 expect(document.querySelector<HTMLInputElement>('.tool-form input[maxlength="200"]')!.value).toBe('Team coding tool');
 expect(document.querySelector<HTMLButtonElement>('.tool-form button.primary')!.disabled).toBe(false);
 await act(async()=>document.querySelector('.tool-form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));await settle();
 expect(writes).toHaveLength(1);expect(writes[0].body).toMatchObject({worker_id:'runner',name:'Team coding tool',profile:{...definition,command_id:'fresh-tool'}});
});
