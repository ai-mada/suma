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
  const data=path==='/tool-profiles'?{items,total:items.length}:path==='/release-workers'?{items:workers}:path==='/project-execution/runners'?{items:[],as_of:'2026-10-29T05:00:00Z'}:items.find(item=>path==='/tool-profiles/'+item.id);
  return new Response(JSON.stringify(data??{}),{status:200});
 }));
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();document.body.innerHTML='';vi.useRealTimers();vi.unstubAllGlobals();});
async function render(){await act(async()=>root.render(<View/>));await settle();}

it('offers the first actionable setup step directly from an empty workspace',async()=>{
 await render();expect(host.textContent).toContain('下一步');await click('登记执行节点');expect(document.querySelector('[role=dialog] input[pattern]')).not.toBeNull();expect(writes).toHaveLength(0);
});
it('filters by actual dispatch eligibility and keeps name/adapter search combined',async()=>{
 items=[profile('ready','enabled',true),profile('review','paused',false),profile('offline','enabled',false),profile('retired','revoked',false)];await render();
 await click('可派发 1');expect([...host.querySelectorAll('.tool-resource strong')].map(e=>e.textContent)).toEqual(['ready']);
 await click('待处理 1');expect([...host.querySelectorAll('.tool-resource strong')].map(e=>e.textContent)).toEqual(['offline']);
 await click('全部 4');await change(host.querySelector('.filter-toolbar input')!,'command');expect(host.querySelectorAll('.tool-resource')).toHaveLength(4);
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
