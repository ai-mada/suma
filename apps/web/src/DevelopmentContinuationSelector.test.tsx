// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import type {Session} from './api';
import type {TaskDevelopmentContinuationOptions} from '../../../packages/contracts/src';
import {DevelopmentContinuationSelector,type DevelopmentContinuationSelectorProps} from './DevelopmentContinuationSelector';
import {continuationCandidate,continuationOptions,continuationSource} from './development-continuation.test-support';
import {setLocale} from './i18n';

const session:Session={user:{id:'user',name:'Owner',email:'owner@example.test'},organization:{id:'org',name:'Team'},projects:[]};
let root:Root|undefined,host:HTMLDivElement|undefined;
async function settle(){await act(async()=>{for(let i=0;i<20;i++)await Promise.resolve();});}
afterEach(async()=>{if(root)await act(async()=>root!.unmount());host?.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});
async function mount(onChange:DevelopmentContinuationSelectorProps['onChange'],onAccessDenied=vi.fn()){
 await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);
 const render=async(patch:Partial<DevelopmentContinuationSelectorProps>={},view=session)=>{await act(async()=>root!.render(<ApiViewProvider session={view}><DevelopmentContinuationSelector project="project-a" taskID="task-a" taskVersion={7} onChange={onChange} onAccessDenied={onAccessDenied} {...patch}/></ApiViewProvider>));await settle();};
 await render();return render;
}
async function click(selector:string){const target=host!.querySelector<HTMLElement>(selector);expect(target,selector).not.toBeNull();await act(async()=>target!.click());await settle();}

it('reads only after opt-in, requires an explicit loaded candidate and rejects a changed paginated source before a fresh task-version choice',async()=>{
 const second={selection:{...continuationSource,node_id:'second'},candidate:{...continuationCandidate,transfer_id:'transfer-two',head_commit:'6'.repeat(40),source:{...continuationCandidate.source,node_id:'second'}},node_name:'Second successful producer'};
 const first={...continuationOptions,total:2,complete:false,next_cursor:'source-page-two'},next={...continuationOptions,items:[second],total:2,complete:false,next_cursor:'source-page-three'};
 const drift={...continuationOptions,source_run_version:9,items:[{...second,selection:{...second.selection,expected_run_version:9}}],total:2};
 const fresh={...continuationOptions,task_version:8,source_run_version:10,items:[{...continuationOptions.items[0],selection:{...continuationSource,expected_run_version:10}}]};
 const unavailable:TaskDevelopmentContinuationOptions={...continuationOptions,task_version:8,source_task_run_id:'',source_run_version:0,source_run_state:'',items:[],total:0,complete:true,reason:'no_run'};
 const pages=[first,next,drift,fresh,unavailable],reads:string[]=[],changed=vi.fn();vi.stubGlobal('fetch',vi.fn(async(input:string,init:RequestInit={})=>{expect(init.method||'GET').toBe('GET');reads.push(String(input));return Response.json(pages.shift());}));
 const render=await mount(changed);expect(reads).toEqual([]);expect(changed.mock.lastCall?.[0]).toEqual({ready:true});
 await click('[data-continuation-toggle]');expect(reads).toHaveLength(1);expect(host!.querySelector<HTMLInputElement>('[data-continuation-source="develop"]')?.checked).toBe(false);expect(changed.mock.lastCall?.[0]).toEqual({ready:false});
 await click('[data-continuation-source="develop"]');expect(changed.mock.lastCall?.[0]).toEqual({source:continuationSource,ready:true});
 await click('[data-continuation-more]');expect(reads[1]).toContain('before=source-page-two');await click('[data-continuation-source="second"]');expect(changed.mock.lastCall?.[0]).toEqual({source:second.selection,ready:true});
 await click('[data-continuation-more]');expect(reads[2]).toContain('before=source-page-three');expect(host!.querySelectorAll('[data-continuation-source]')).toHaveLength(0);expect(changed.mock.lastCall?.[0]).toEqual({ready:false});expect(host!.querySelector('[role="alert"]')).not.toBeNull();
 await render({taskVersion:8});expect(host!.querySelector<HTMLInputElement>('[data-continuation-toggle]')?.checked).toBe(true);expect(host!.querySelector<HTMLInputElement>('[data-continuation-source="develop"]')?.checked).toBe(false);expect(changed.mock.lastCall?.[0]).toEqual({ready:false});await click('[data-continuation-source="develop"]');expect(changed.mock.lastCall?.[0]).toEqual({source:fresh.items[0].selection,ready:true});
 await click('[data-continuation-toggle]');await click('[data-continuation-toggle]');expect(host!.querySelectorAll('[data-continuation-source]')).toHaveLength(0);expect(changed.mock.lastCall?.[0]).toEqual({ready:false});expect(host!.querySelector('[role="alert"]')?.textContent).toContain('This task has no previous run to continue.');expect(host!.textContent).not.toContain('no_run');
});

it('preserves a pending source without callbacks, ignores late reads after opt-out and caller change, and clears protected candidates on current denial',async()=>{
 const changed=vi.fn(),denied=vi.fn();let resolveFirst!:(value:Response)=>void,resolveThird!:(value:Response)=>void,calls=0;
 vi.stubGlobal('fetch',vi.fn(async(_input:string,init:RequestInit={})=>{expect(init.method||'GET').toBe('GET');calls++;if(calls===1)return new Promise<Response>(resolve=>{resolveFirst=resolve;});if(calls===2)return Response.json(continuationOptions);if(calls===3)return new Promise<Response>(resolve=>{resolveThird=resolve;});return Response.json({error:{code:'current_access_denied',message:'Current access denied'}},{status:403});}));
 const render=await mount(changed,denied);await click('[data-continuation-toggle]');await click('[data-continuation-toggle]');await act(async()=>resolveFirst(Response.json(continuationOptions)));await settle();expect(host!.querySelectorAll('[data-continuation-source]')).toHaveLength(0);expect(changed.mock.lastCall?.[0]).toEqual({ready:true});
 await click('[data-continuation-toggle]');await click('[data-continuation-source="develop"]');const original=changed.mock.lastCall?.[0],count=changed.mock.calls.length;
 await render({disabled:true});await render({disabled:true,taskVersion:8});expect(changed.mock.calls).toHaveLength(count);expect(changed.mock.lastCall?.[0]).toEqual(original);expect(calls).toBe(2);expect(host!.querySelector<HTMLInputElement>('[data-continuation-toggle]')?.disabled).toBe(true);
 await render({taskVersion:8});expect(calls).toBe(3);const other={...session,organization:{id:'another-org',name:'Another team'}};await render({taskVersion:8},other);await act(async()=>resolveThird(Response.json({...continuationOptions,task_version:8})));await settle();expect(host!.querySelectorAll('[data-continuation-source]')).toHaveLength(0);expect(changed.mock.lastCall?.[0]).toEqual({ready:true});
 await click('[data-continuation-toggle]');expect(denied).toHaveBeenCalledTimes(1);expect(host!.querySelectorAll('[data-continuation-source]')).toHaveLength(0);expect(changed.mock.lastCall?.[0]).toEqual({ready:false});expect(host!.querySelector<HTMLInputElement>('[data-continuation-toggle]')?.disabled).toBe(true);
});
