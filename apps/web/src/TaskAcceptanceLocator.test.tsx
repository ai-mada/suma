// @vitest-environment jsdom
import {act,StrictMode} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {ApiCaller,Session} from './api';
import * as ApiContext from './api-context';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import {TaskAcceptanceLocator} from './TaskAcceptanceLocator';

const session:Session={user:{id:'user-a',name:'Owner',email:'owner@example.test'},organization:{id:'org-a',name:'Org'},projects:[]};
type Props=Parameters<typeof TaskAcceptanceLocator>[0];
let root:Root|undefined,host:HTMLDivElement,open:ReturnType<typeof vi.fn>,props:Props;
beforeEach(async()=>{await setLocale('en-US',{persist:false});host=document.createElement('div');document.body.append(host);vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.stubGlobal('fetch',vi.fn(()=>{throw Error('Locator must not perform HTTP');}));open=vi.fn();props={session,project:'project-a',taskID:'task-a',taskVersion:3,runID:'run-a',panel:'task-acceptance:3:run-a',onOpenReview:open};});
afterEach(async()=>{if(root)await act(async()=>root!.unmount());root=undefined;host.remove();vi.restoreAllMocks();vi.unstubAllGlobals();await setLocale('zh-CN',{persist:false});});
async function draw(provider=session){await act(async()=>{root??=createRoot(host);root.render(<StrictMode><ApiViewProvider session={provider}><TaskAcceptanceLocator {...props}/></ApiViewProvider></StrictMode>);});}

it('T214 consumes an exact locator once across StrictMode, data refresh and task-version or run ABA without writes',async()=>{
 await draw();expect(open).toHaveBeenCalledOnce();expect(host.querySelector('[data-state=opened]')).not.toBeNull();
 await draw();props={...props,onOpenReview:()=>{open();}};await draw();
 props={...props,taskVersion:4};await draw();expect(host.querySelector('[data-state=basis_changed]')).not.toBeNull();props={...props,taskVersion:3};await draw();
 props={...props,runID:'run-b'};await draw();props={...props,runID:'run-a'};await draw();expect(open).toHaveBeenCalledOnce();
 await act(async()=>{await setLocale('zh-CN',{persist:false});});expect(host.textContent).toContain('没有提交验收决定');expect(fetch).not.toHaveBeenCalled();
});

it('T214 rejects malformed, changed and foreign-view locators with localized explanations and ignores unrelated panels',async()=>{
 props={...props,panel:'task-acceptance:03:run-a'};await draw();expect(host.querySelector('[data-state=invalid]')).not.toBeNull();
 props={...props,panel:'task-acceptance:3:run-a',runID:'run-other'};await draw();expect(host.querySelector('[data-state=basis_changed]')).not.toBeNull();
 props={...props,runID:'run-a'};await draw({...session,user:{...session.user,id:'other'}});expect(host.querySelector('[data-state=scope_changed]')).not.toBeNull();expect(open).not.toHaveBeenCalled();
 props={...props,panel:'dispatch:new'};await draw();expect(host.querySelector('[data-task-acceptance-locator]')).toBeNull();expect(fetch).not.toHaveBeenCalled();
});

it('T214 waits for the original pending operation and retries a refusal only after unblock, never on caller ABA or ordinary renders',async()=>{
 const callerA=vi.fn() as unknown as ApiCaller,callerB=vi.fn() as unknown as ApiCaller;let caller=callerA;vi.spyOn(ApiContext,'useApi').mockImplementation(()=>caller);
 open.mockReturnValue(false);props={...props,blocked:true};await draw();expect(open).not.toHaveBeenCalled();
 props={...props,blocked:false};await draw();expect(open).toHaveBeenCalledOnce();expect(host.querySelector('[data-state=unconfirmed]')).not.toBeNull();
 await draw();caller=callerB;await draw();caller=callerA;await draw();expect(open).toHaveBeenCalledOnce();
 props={...props,blocked:true};await draw();open.mockReturnValue(true);props={...props,blocked:false};await draw();expect(open).toHaveBeenCalledTimes(2);expect(host.querySelector('[data-state=opened]')).not.toBeNull();
 props={...props,blocked:true};await draw();props={...props,blocked:false};await draw();expect(open).toHaveBeenCalledTimes(2);expect(fetch).not.toHaveBeenCalled();
});
