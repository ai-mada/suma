// @vitest-environment jsdom
import {act,StrictMode} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import type {AdapterDescriptor} from './AdapterConnections';
import {LocalToolsScanPanel} from './LocalToolsScanPanel';
import {setLocale} from './i18n';

const project='1'.repeat(32),user='2'.repeat(32),tenant='3'.repeat(32);
const session={user:{id:user,name:'Owner',email:'owner@example.test'},organization:{id:tenant,name:'Team'}};
const ids=['codex-jsonl','codebuddy-cli','dsh-multica','claude-code-cli','cursor-cli','qoder-acp','codex-app-server'];
const products=['Codex','CodeBuddy Code','DeepSeek Harness / dsh','Claude Code','Cursor','Qoder CLI','Codex'];
const adapter={id:'codex-app-server',product:'Codex',direction:'suma_to_tool',implementation:'implemented',setup_kind:'runner_profile',entrypoint_kind:'cli',adapter:'codex',protocol:'codex-app-server-v1',capabilities:[],limitations:[],source_refs:[]} as AdapterDescriptor;
const cursor={id:'cursor-cli',product:'Cursor',direction:'suma_to_tool',implementation:'planned',setup_kind:'manual_guide',entrypoint_kind:'cli',capabilities:[],limitations:[],source_refs:[]} as AdapterDescriptor;
function report(p=project,cached=false){const now=Date.now();return {status:'observed',report:{protocol:'suma-installed-tools-v1',items:ids.map((id,i)=>({adapter_id:id,product:products[i],installed:false,launch_supported:false,reason_code:'not_installed'}))},scan:{id:'4'.repeat(32),project_id:p,user_id:user,tenant_id:tenant,observed_at:new Date(now-1000).toISOString(),expires_at:new Date(now+59000).toISOString(),cached}};}
let root:Root,host:HTMLDivElement,scan:ReturnType<typeof vi.fn>,select:ReturnType<typeof vi.fn>;
beforeEach(async()=>{await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);scan=vi.fn(async()=>report());select=vi.fn();vi.stubGlobal('sumaDesktop',{platform:'win32',version:'1.0.0',discoverInstalledTools:scan});vi.stubGlobal('fetch',vi.fn(()=>{throw Error('No HTTP from scan panel');}));});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.useRealTimers();vi.unstubAllGlobals();vi.restoreAllMocks();});
async function render(disabled=false,p=project,identity=session,strict=false){const content=<ApiViewProvider session={identity}><LocalToolsScanPanel project={p} adapters={[adapter,cursor]} disabled={disabled} onSelect={select}/></ApiViewProvider>;await act(async()=>root.render(strict?<StrictMode>{content}</StrictMode>:content));}
const rescan=()=>host.querySelector<HTMLButtonElement>('[data-local-tools-rescan]')!;
async function click(element:HTMLButtonElement){await act(async()=>element.click());}

it('automatically scans once and only forces an explicit refresh, retaining locale and seven protocol facts without connecting',async()=>{
 await render(false,project,session,true);expect(scan).toHaveBeenCalledExactlyOnceWith({action:'scan',project_id:project,user_id:user,tenant_id:tenant,locale:'en-US'});expect(host.querySelectorAll('[data-local-tools-adapter]')).toHaveLength(7);expect(select).not.toHaveBeenCalled();
 await act(async()=>setLocale('zh-CN',{persist:false}));await render(false,project,session,true);expect(scan).toHaveBeenCalledOnce();expect(host.textContent).toContain('本机开发工具');
 vi.useFakeTimers({toFake:['setTimeout','clearTimeout']});const cached=report(project,true),now=Date.now();cached.scan.observed_at=new Date(now-59900).toISOString();cached.scan.expires_at=new Date(now+100).toISOString();scan.mockResolvedValueOnce(cached);await click(rescan());expect(scan).toHaveBeenLastCalledWith({action:'scan',project_id:project,user_id:user,tenant_id:tenant,locale:'zh-CN',force:true});expect(host.textContent).toContain('缓存');await act(async()=>vi.advanceTimersByTimeAsync(101));expect(host.textContent).toContain('已过期');expect(host.querySelectorAll('[data-local-tools-adapter]')).toHaveLength(7);
 await click(host.querySelector<HTMLButtonElement>('[data-local-tools-select=cursor-cli]')!);expect(select).toHaveBeenCalledExactlyOnceWith(cursor);expect(scan).toHaveBeenCalledTimes(2);expect(fetch).not.toHaveBeenCalled();
});
it('defers disabled auto-scan, locks selection, drops late scope results and rejects invalid reports',async()=>{
 await render(true);expect(scan).not.toHaveBeenCalled();await render(false);expect(scan).toHaveBeenCalledOnce();await render(true);expect(rescan().disabled).toBe(true);expect(host.querySelector<HTMLButtonElement>('[data-local-tools-select=codex-app-server]')!.disabled).toBe(true);
 await render(false);let finish!:(value:unknown)=>void;scan.mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve;}));await click(rescan());expect(rescan().disabled).toBe(true);
 const next='9'.repeat(32);scan.mockResolvedValueOnce(report(next));await render(false,next);await act(async()=>finish({...report(),report:{...report().report,items:report().report.items.map(item=>({...item,product:'Late private result'}))}}));expect(host.textContent).not.toContain('Late private result');expect(host.querySelectorAll('[data-local-tools-adapter]')).toHaveLength(7);
 scan.mockResolvedValueOnce({...report(next),scan:{...report(next).scan,tenant_id:'8'.repeat(32)}});await click(rescan());expect(host.querySelectorAll('[data-local-tools-adapter]')).toHaveLength(0);expect(host.textContent).toContain('could not be verified');expect(select).not.toHaveBeenCalled();expect(fetch).not.toHaveBeenCalled();
});
