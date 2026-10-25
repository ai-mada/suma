// @vitest-environment jsdom
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { beforeEach, afterEach, expect, it, vi } from 'vitest';
import { ToolProfileFacts, toolReason } from './ToolFacts';
import { ToolBindingFacts } from './ToolBindingStatus';
import { setLocale, setServerCatalog } from './i18n';
import {ErrorNotice} from './ui';
import {ApiError} from './api';
import type { ToolProfile } from './tool-model';
import {ToolWorkspace} from './ToolWorkspace';

const catalog = {version:'display-fixture',template_format:'named-dot-v1',default_locale:'zh-CN',supported_locales:['zh-CN','en-US'],messages:[
  {key:'display.tool.profile_paused',parameters:[],texts:{'zh-CN':'工具档案已暂停，尚未授权派发','en-US':'Tool profile is paused and is not authorized for dispatch'}},
  {key:'display.tool.reported_unavailable',parameters:[{name:'reason',kind:'text'}],texts:{'zh-CN':'执行节点报告工具不可用：{{.reason}}','en-US':'Worker reported the tool unavailable: {{.reason}}'}},
]};
const profile: ToolProfile = {
  id:'profile', project_id:'p', worker_id:'worker', name:'User tool name',version:1,state:'paused',
  profile:{command_id:'coder',adapter:'suma',protocol:'suma-jsonl-v1',profile_sha256:'a'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'1.0',read_only:false},
  enabled_by:'u',created_at:'',updated_at:'',available:false,reason:'工具档案已暂停，尚未授权派发',
  ...{reason_diagnostic:{schema_version:1,message_key:'display.tool.profile_paused'}},
};
let host:HTMLDivElement, root:Root;
beforeEach(async()=>{vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);await setLocale('en-US',{persist:false});setServerCatalog(catalog);host=document.createElement('div');document.body.append(host);root=createRoot(host);});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();});

it('uses source-bound eligibility in the profile and pinned binding while retaining raw disclosure',async()=>{
 await act(async()=>root.render(<><ToolProfileFacts profile={profile}/><ToolBindingFacts binding={{profile_id:'profile',profile_sha256:'a'.repeat(64),command:'coder',run_id:'run',node_id:'agent'}} project="p" state="succeeded" profile={profile}/></>));
 const paragraphs=[...host.querySelectorAll('p')].map(item=>item.textContent);
 expect(paragraphs.filter(text=>text==='Tool profile is paused and is not authorized for dispatch')).toHaveLength(2);
 expect([...host.querySelectorAll('details code')].filter(item=>item.textContent===profile.reason)).toHaveLength(2);
});
it.each([
 {message:'External tool returned an unrelated original error',message_key:'display.tool.profile_paused'},
 {message:'External tool returned an unrelated original error',schema_version:1,message_key:'display.tool.profile_paused'},
 {message:'工具档案已暂停，尚未授权派发',schema_version:1,message_key:'display.tool.profile_paused'},
 {message:'External tool returned an unrelated original error',message_key:'display.tool.reported_unavailable',message_params:{reason:'inventory_stale'}},
 {message:'执行节点报告工具不可用：inventory_stale',schema_version:1,message_key:'display.tool.reported_unavailable',message_params:{reason:'inventory_stale'}},
 {message:'External original error',message_key:'display.tool.reported_unavailable',message_params:{reason:'display.tool.profile_paused',status:429}},
])('keeps generic ErrorNotice raw for display identity without source-bound consumer proof: %j',async(value)=>{
 await act(async()=>root.render(<ErrorNotice error={value}/>));expect(host.querySelector('[role="alert"]')?.textContent).toBe(value.message);
});
it('does not turn a no-hint raw ErrorNotice into an ambient display alias when its catalog arrives',async()=>{
 setServerCatalog({...catalog,messages:[]});await act(async()=>root.render(<ErrorNotice error={profile.reason}/>));
 expect(host.textContent).toBe(profile.reason);
 await act(async()=>setServerCatalog(catalog));expect(host.textContent).toBe(profile.reason);
 await act(async()=>{await setLocale('zh-CN',{persist:false});});
 await act(async()=>root.render(<ErrorNotice error="Tool profile is paused and is not authorized for dispatch"/>));
 expect(host.textContent).toBe('Tool profile is paused and is not authorized for dispatch');
});
it('keeps unknown future display-domain keys and reverse sources raw when a newer catalog arrives',async()=>{
 const source='未来工具资格说明原文';
 setServerCatalog({...catalog,messages:[...catalog.messages,{key:'display.tool.future',parameters:[],texts:{'zh-CN':source,'en-US':'Future tool qualification translation'}}]});
 const message='Unrelated original future error';
 await act(async()=>root.render(<><ErrorNotice error={{message,message_key:'display.tool.future'}}/><ErrorNotice error={source}/></>));
 expect([...host.querySelectorAll('[role="alert"]')].map(alert=>alert.textContent)).toEqual([message,source]);
});
it('retains legacy generic APIError key and raw-alias behavior alongside bound display consumers',async()=>{
 setServerCatalog({...catalog,messages:[...catalog.messages,
  {key:'pool.limit',parameters:[{name:'limit',kind:'integer'}],texts:{'zh-CN':'上限{{.limit}}','en-US':'Limit {{.limit}}'}},
  {key:'pool.paused',parameters:[],texts:{'zh-CN':'共享池已暂停','en-US':'Pool paused'}},
 ]});
 await act(async()=>root.render(<><ErrorNotice error={new ApiError(409,'limit_reached','Original API reason',undefined,{message_key:'pool.limit',message_params:{limit:0}})}/><ErrorNotice error="共享池已暂停"/><ToolProfileFacts profile={profile}/></>));
 expect([...host.querySelectorAll('[role="alert"]')].map(alert=>alert.textContent)).toEqual(['Limit 0','Pool paused']);
 expect(host.querySelector('.warning p')?.textContent).toBe('Tool profile is paused and is not authorized for dispatch');expect(host.querySelector('details code')?.textContent).toBe(profile.reason);
});
it('does not swallow unknown English raw reasons behind a generic warning',()=>{
 expect(toolReason('external credential broker denied scope')).toBe('external credential broker denied scope');
 expect(toolReason('constructor')).toBe('constructor');expect(toolReason('__proto__')).toBe('__proto__');
});
it('keeps the existing disclosure, focus, raw data and requests unchanged across locale and late catalog',async()=>{
 const fetch=vi.fn();vi.stubGlobal('fetch',fetch);setServerCatalog({...catalog,messages:[]});
 const before=JSON.stringify(profile);await act(async()=>root.render(<ToolProfileFacts profile={profile}/>));
 const details=host.querySelector('details')!,summary=details.querySelector('summary')!;details.open=true;summary.focus();
 expect(host.querySelector('.warning p')?.textContent).toBe(profile.reason);
 await act(async()=>setServerCatalog(catalog));
 expect(host.querySelector('.warning p')?.textContent).toBe('Tool profile is paused and is not authorized for dispatch');
 await act(async()=>{await setLocale('zh-CN',{persist:false});});
 expect(host.querySelector('details')).toBe(details);expect(details.open).toBe(true);expect(document.activeElement).toBe(summary);
 expect(details.querySelector('code')?.textContent).toBe(profile.reason);expect(JSON.stringify(profile)).toBe(before);expect(fetch).not.toHaveBeenCalled();
});
it.each(['enable','pause'] as const)('preserves the real %s dialog confirmation, raw disclosure and focus without locale requests',async(decision)=>{
 const value={...profile,state:decision==='enable'?'paused':'enabled'} as ToolProfile;
 const fetch=vi.fn(async(input:unknown)=>{
  const path=new URL(String(input),'http://local').pathname;
  return new Response(JSON.stringify(path.endsWith('/tool-profiles/profile')?value:path.endsWith('/tool-profiles')?{items:[value],total:1}:{items:[],total:0}),{headers:{'Content-Type':'application/json'}});
 });vi.stubGlobal('fetch',fetch);
 await act(async()=>root.render(<ToolWorkspace project="p" selected="profile" panel="" refresh={0} onSelect={()=>{}} session={{user:{id:'u',name:'Fixture',email:'fixture@example.test',role:'owner'},organization:{id:'org',name:'Fixture'},projects:[]}}/>));
 const launch=[...host.querySelectorAll('button')].find(button=>button.textContent===(decision==='enable'?'Review and enable':'Pause configuration'));
 expect(launch).toBeTruthy();await act(async()=>launch!.click());
 const dialog=document.querySelector<HTMLElement>('[role="dialog"]')!;expect(dialog).not.toBeNull();
 expect(dialog.querySelector('.warning p')?.textContent).toBe('Tool profile is paused and is not authorized for dispatch');
 const checkbox=dialog.querySelector<HTMLInputElement>('input[type="checkbox"]')!;await act(async()=>checkbox.click());expect(checkbox.checked).toBe(true);
 const details=dialog.querySelector('details')!,summary=details.querySelector('summary')!;details.open=true;summary.focus();const count=fetch.mock.calls.length;
 await act(async()=>{await setLocale('zh-CN',{persist:false});});
 expect(document.querySelector('[role="dialog"]')).toBe(dialog);expect(dialog.querySelector('input[type="checkbox"]')).toBe(checkbox);expect(checkbox.checked).toBe(true);
 expect(dialog.querySelector('details')).toBe(details);expect(details.open).toBe(true);expect(document.activeElement).toBe(summary);expect(details.querySelector('code')?.textContent).toBe(profile.reason);expect(fetch.mock.calls).toHaveLength(count);
});
