// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {beforeEach,afterEach,it,expect,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import {ModelExecutionPanel} from './ModelExecutionPanel';
import {CodexExecutionFacts} from './CodexModelExecutionPanel';
import type {ModelExecutionOptions,CodexBinding} from '../../../packages/contracts/src';
const scope={project:'project-a',profile:'tool-a',version:2,sha256:'a'.repeat(64)};
const session={user:{id:'user-a',name:'Owner',email:'owner@example.test'},organization:{id:'org-a',name:'Org'},projects:[]};
const catalog={protocol:'codex-app-server-v1',configuration_sha256:'b'.repeat(64),catalog_sha256:'c'.repeat(64),models:[{model:'model-a',reasoning_efforts:['high','low'],default_reasoning_effort:'low'}]};
function options():ModelExecutionOptions{return {project_id:scope.project,tool_profile_id:scope.profile,tool_profile_version:2,tool_profile_sha256:scope.sha256,observed_at:'2026-10-27T09:00:00Z',supported:false,reason:'',items:[],codex_supported:true,codex_catalog:catalog,codex_reason:''};}
let root:Root|undefined,host:HTMLDivElement;
beforeEach(async()=>{await setLocale('en-US',{persist:false});host=document.createElement('div');document.body.append(host);vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);});
afterEach(async()=>{if(root)await act(async()=>root!.unmount());root=undefined;host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();await setLocale('zh-CN',{persist:false});});
const settle=async()=>act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});
async function set(selector:string,value:string){const field=host.querySelector<HTMLInputElement|HTMLSelectElement>(selector)!;await act(async()=>{Object.getOwnPropertyDescriptor(field.tagName==='SELECT'?HTMLSelectElement.prototype:HTMLInputElement.prototype,'value')!.set!.call(field,value);field.dispatchEvent(new Event(field.tagName==='SELECT'?'change':'input',{bubbles:true}));});}
it('uses GET-only scoped options and preserves explicit effort and notice across languages',async()=>{
 const changed=vi.fn(),fetcher=vi.fn(async(_input:RequestInfo|URL,_init?:RequestInit)=>Response.json(options()));vi.stubGlobal('fetch',fetcher);
 await act(async()=>{root=createRoot(host);root.render(<ApiViewProvider session={session}><ModelExecutionPanel scope={scope} supported={false} codex disabled={false} onChange={changed}/></ApiViewProvider>);});await settle();
 expect(changed.mock.lastCall?.[0].ready).toBe(false);await set('[data-codex-model]','model-a');await set('[data-codex-effort]','high');await set('[data-codex-notice]','40');
 expect(changed.mock.lastCall?.[0]).toEqual({ready:true,codexSelection:{catalog_sha256:catalog.catalog_sha256,model:'model-a',reasoning_effort:'high',token_notice_limit:40}});
 await act(async()=>setLocale('zh-CN',{persist:false}));expect((host.querySelector('[data-codex-notice]') as HTMLInputElement).value).toBe('40');expect(host.textContent).toContain('不支持硬额度');expect(host.querySelector('[data-codex-effort] option:checked')?.textContent).toBe('高');expect(fetcher).toHaveBeenCalledTimes(1);expect(fetcher.mock.calls[0][1]?.method).toBe('GET');
 await set('[data-codex-notice]','1e2');expect(changed.mock.lastCall?.[0].ready).toBe(false);
});
it('blocks an unrelated catalog and requires a real Codex choice after discovery',async()=>{
 const changed=vi.fn();vi.stubGlobal('fetch',vi.fn(async()=>Response.json({...options(),project_id:'other-project'})));
 await act(async()=>{root=createRoot(host);root.render(<ApiViewProvider session={session}><ModelExecutionPanel scope={scope} supported={false} codex disabled={false} onChange={changed}/></ApiViewProvider>);});await settle();expect(changed.mock.lastCall?.[0].ready).toBe(false);expect(host.querySelector('[data-codex-model]')).toBeNull();
 vi.stubGlobal('fetch',vi.fn(async()=>Response.json(options())));
 await act(async()=>root!.render(<ApiViewProvider session={session}><ModelExecutionPanel scope={scope} supported={false} discover disabled={false} onChange={changed}/></ApiViewProvider>));await settle();expect(changed.mock.lastCall?.[0].ready).toBe(false);expect(host.querySelector('[data-codex-model]')).not.toBeNull();
 await act(async()=>root!.render(<ApiViewProvider session={session}><ModelExecutionPanel scope={scope} supported={false} discover allowCodex={false} disabled={false} onChange={changed}/></ApiViewProvider>));await settle();expect(changed.mock.lastCall?.[0].ready).toBe(false);expect(host.querySelector('[data-codex-model]')).toBeNull();expect(host.textContent).toContain('does not support Codex');expect(host.textContent).not.toContain('configures its own model');
});
it('keeps configuration acknowledgement separate from provider identity and hard budgets',async()=>{
 const binding={protocol:'codex-app-server-v1',configuration_sha256:'b'.repeat(64),catalog_sha256:'c'.repeat(64),model:'model-a',reasoning_effort:'high',token_notice_limit:40,budget_mode:'observe_only',identity_evidence:'configuration_ack_only',binding_sha256:'d'.repeat(64)} satisfies CodexBinding;
 await act(async()=>{root=createRoot(host);root.render(<CodexExecutionFacts binding={binding} evidence={{binding_sha256:binding.binding_sha256,requested_model:'model-a',requested_reasoning_effort:'high',acknowledged_model:'model-a',acknowledged_reasoning_effort:'high',acknowledged_provider:'owned',configuration_acknowledged:true,interrupt_requested:false,interrupt_acknowledged:false,token_notice_exceeded:true,thread_id:'thread-1',turn_id:'turn-1'}}/>);});
 expect(host.textContent).toContain('CLI acknowledged');expect(host.textContent).toContain('does not attest');expect(host.textContent).toContain('exceeded');expect(host.textContent).toContain('thread-1');
});
