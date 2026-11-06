// @vitest-environment jsdom
import {act,StrictMode} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import type {AdapterDescriptor} from './AdapterConnections';
import {LocalToolsScanPanel} from './LocalToolsScanPanel';
import {setLocale} from './i18n';
import type {ToolInstallationProbeDiagnostic} from './tool-installation-probe-diagnostic-model';

const project='1'.repeat(32),user='2'.repeat(32),tenant='3'.repeat(32);
const session={user:{id:user,name:'Owner',email:'owner@example.test'},organization:{id:tenant,name:'Team'}};
const ids=['codex-jsonl','codebuddy-cli','dsh-multica','claude-code-cli','cursor-cli','qoder-acp','codex-app-server','opencode-sdk','claude-code-sdk'];
const products=['Codex','CodeBuddy Code','DeepSeek Harness / dsh','Claude Code','Cursor','Qoder CLI','Codex','OpenCode','Claude Code'];
const adapter={id:'codex-app-server',product:'Codex',direction:'suma_to_tool',implementation:'implemented',setup_kind:'runner_profile',entrypoint_kind:'cli',adapter:'codex',protocol:'codex-app-server-v1',capabilities:[],limitations:[],source_refs:[]} as AdapterDescriptor;
const cursor={id:'cursor-cli',product:'Cursor',direction:'suma_to_tool',implementation:'planned',setup_kind:'manual_guide',entrypoint_kind:'cli',capabilities:[],limitations:[],source_refs:[]} as AdapterDescriptor;
function report(p=project,cached=false){const now=Date.now();return {status:'observed',report:{protocol:'suma-installed-tools-v1',items:ids.map((id,i)=>({adapter_id:id,product:products[i],installed:false,launch_supported:false,reason_code:'not_installed'}))},scan:{id:'4'.repeat(32),project_id:p,user_id:user,tenant_id:tenant,observed_at:new Date(now-1000).toISOString(),expires_at:new Date(now+59000).toISOString(),cached}};}
let root:Root,host:HTMLDivElement,scan:ReturnType<typeof vi.fn>,select:ReturnType<typeof vi.fn>;
beforeEach(async()=>{await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);scan=vi.fn(async()=>report());select=vi.fn();vi.stubGlobal('sumaDesktop',{platform:'win32',version:'1.0.0',discoverInstalledTools:scan});vi.stubGlobal('fetch',vi.fn(()=>{throw Error('No HTTP from scan panel');}));});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.useRealTimers();vi.unstubAllGlobals();vi.restoreAllMocks();});
async function render(disabled=false,p=project,identity=session,strict=false){const content=<ApiViewProvider session={identity}><LocalToolsScanPanel project={p} adapters={[adapter,cursor]} disabled={disabled} onSelect={select}/></ApiViewProvider>;await act(async()=>root.render(strict?<StrictMode>{content}</StrictMode>:content));}
const rescan=()=>host.querySelector<HTMLButtonElement>('[data-local-tools-rescan]')!;
async function click(element:HTMLButtonElement){await act(async()=>element.click());}

function controlForeground(){
 vi.useFakeTimers({toFake:['Date','setTimeout','clearTimeout','performance']});
 let visible=true,focused=true;
 vi.spyOn(document,'visibilityState','get').mockImplementation(()=>visible?'visible':'hidden');
 vi.spyOn(document,'hidden','get').mockImplementation(()=>!visible);
 vi.spyOn(document,'hasFocus').mockImplementation(()=>focused);
 return {
  focus(value:boolean){focused=value;window.dispatchEvent(new Event(value?'focus':'blur'));},
  show(value:boolean){visible=value;document.dispatchEvent(new Event('visibilitychange'));},
  async wait(ms:number){await act(async()=>vi.advanceTimersByTimeAsync(ms));},
 };
}

it('refreshes once at the original expiry after returning early, without choosing a newly detected tool',async()=>{
 const foreground=controlForeground(),onAutoSelect=vi.fn(),cli={...adapter,id:'codex-jsonl',protocol:'codex-jsonl-v1'};
 scan.mockImplementationOnce(async()=>report()).mockImplementationOnce(async()=>{
  const value=report();
  value.report.items[0]={...value.report.items[0],installed:true,launch_supported:true,reason_code:'workspace_required',
   executable:'C:/tools/codex.exe',version:'0.141.0'} as typeof value.report.items[number];
  return value;
 });
 await act(async()=>root.render(<ApiViewProvider session={session}><LocalToolsScanPanel project={project}
  adapters={[cli]} compact onSelect={select} onAutoSelect={onAutoSelect}/></ApiViewProvider>));
 const summary=host.querySelector<HTMLElement>('[data-local-tools-results] > summary')!;
 summary.focus();
 await act(async()=>foreground.focus(true));
 await foreground.wait(10000);
 await act(async()=>{foreground.focus(false);foreground.show(false);foreground.focus(true);});
 expect(scan).toHaveBeenCalledOnce();
 await act(async()=>{foreground.show(true);foreground.focus(true);foreground.focus(true);});
 await foreground.wait(48999);
 expect(scan).toHaveBeenCalledOnce();
 await foreground.wait(2);
 expect(scan).toHaveBeenCalledTimes(2);
 expect(scan).toHaveBeenLastCalledWith({action:'scan',project_id:project,user_id:user,tenant_id:tenant,locale:'en-US'});
 expect(host.querySelector('[data-local-tools-shortcut="codex-jsonl"]')).not.toBeNull();
 expect(document.activeElement).toBe(summary);
 expect(onAutoSelect).not.toHaveBeenCalled();
 expect(select).not.toHaveBeenCalled();
 await act(async()=>{foreground.focus(true);foreground.show(true);});
 await foreground.wait(60000);
 expect(scan).toHaveBeenCalledTimes(2);
 expect(fetch).not.toHaveBeenCalled();
});

it('cancels a deferred refresh while hidden and retains stale facts and selection if the return scan fails',async()=>{
 const foreground=controlForeground(),onAutoSelect=vi.fn();
 const value=report();
 value.report.items[6]={...value.report.items[6],installed:true,reason_code:'configuration_required',
  executable:'C:/tools/codex.exe',version:'0.141.0',executable_sha256:'a'.repeat(64)} as typeof value.report.items[number];
 let rejectRefresh!:(reason:Error)=>void;
 scan.mockResolvedValueOnce(value).mockImplementationOnce(()=>new Promise((_,reject)=>{rejectRefresh=reject;}));
 await act(async()=>root.render(<ApiViewProvider session={session}><LocalToolsScanPanel project={project}
  adapters={[adapter,cursor]} selectedAdapterID={adapter.id} onSelect={select} onAutoSelect={onAutoSelect}/></ApiViewProvider>));
 rescan().focus();
 await act(async()=>{foreground.focus(false);foreground.focus(true);foreground.show(false);});
 await foreground.wait(59001);
 expect(scan).toHaveBeenCalledOnce();
 await act(async()=>{foreground.show(true);foreground.focus(true);});
 expect(scan).toHaveBeenCalledTimes(2);
 expect(document.activeElement).toBe(rescan());
 expect(rescan().getAttribute('aria-disabled')).toBe('true');
 await click(rescan());
 expect(scan).toHaveBeenCalledTimes(2);
 await act(async()=>rejectRefresh(new Error('desktop unavailable')));
 const retained=host.querySelector('[data-local-tools-primary] [data-local-tools-adapter="codex-app-server"]');
 expect(retained?.textContent).toContain('0.141.0');
 expect(retained?.querySelector('[data-local-tools-select="codex-app-server"]')?.getAttribute('aria-pressed')).toBe('true');
 expect(host.querySelector('.local-tools-scan-status')?.textContent).toContain('The last scan is shown for reference');
 expect(select).not.toHaveBeenCalled();
 expect(onAutoSelect).not.toHaveBeenCalled();
 expect(rescan().disabled).toBe(false);
 await click(rescan());
 expect(scan).toHaveBeenCalledTimes(3);
 expect(scan).toHaveBeenLastCalledWith({action:'scan',project_id:project,user_id:user,tenant_id:tenant,locale:'en-US',force:true});
 expect(fetch).not.toHaveBeenCalled();
});

it('clears previous local observations when the desktop rejects the returning scope',async()=>{
 const foreground=controlForeground();
 scan.mockResolvedValueOnce(report()).mockResolvedValueOnce({status:'failed',code:'discovery_access_changed'});
 await render();
 expect(host.querySelectorAll('[data-local-tools-adapter]')).toHaveLength(9);
 await act(async()=>foreground.focus(false));
 await foreground.wait(59001);
 await act(async()=>foreground.focus(true));
 expect(scan).toHaveBeenCalledTimes(2);
 expect(host.querySelectorAll('[data-local-tools-adapter]')).toHaveLength(0);
 expect(host.querySelector('.local-tools-scan-status')?.textContent).toContain('could not be verified');
 expect(select).not.toHaveBeenCalled();
 expect(fetch).not.toHaveBeenCalled();
});

it('keeps a replayed observation expired after the wall clock moves backwards',async()=>{
 const foreground=controlForeground(),originalWall=Date.now(),value=report();
 scan.mockResolvedValueOnce(value).mockResolvedValueOnce({...value,scan:{...value.scan,cached:true}});
 await render();
 await act(async()=>foreground.focus(false));
 await foreground.wait(59001);
 vi.setSystemTime(originalWall);
 await act(async()=>foreground.focus(true));
 expect(scan).toHaveBeenCalledTimes(2);
 expect(host.querySelector('.local-tools-scan-status')?.textContent).toContain('This scan has expired');
 expect(host.querySelectorAll('[data-local-tools-adapter]')).toHaveLength(9);
 expect(fetch).not.toHaveBeenCalled();
});

it.each(['disabled','stop-unverified'] as const)('does not refresh on return with a %s scan',async(blocked)=>{
 const foreground=controlForeground();
 if(blocked==='stop-unverified'){
  const value=report();
  value.report.items[8]={...value.report.items[8],installed:true,reason_code:'probe_failed',
   diagnostic:{stage:'version',code:'stop_unverified',next_action:'none'}} as typeof value.report.items[number];
  scan.mockResolvedValueOnce(value);
 }
 await render();
 if(blocked==='disabled')await render(true);
 await act(async()=>{foreground.focus(false);foreground.show(false);});
 await foreground.wait(59001);
 await act(async()=>{foreground.show(true);foreground.focus(true);});
 expect(rescan().disabled).toBe(true);
 expect(scan).toHaveBeenCalledOnce();
 expect(select).not.toHaveBeenCalled();
 expect(fetch).not.toHaveBeenCalled();
});

it.each([['unique',true],['ambiguous',false],['expired',false],['unsupported',false],['no exact descriptor',false]] as const)(
 'only suggests a current, uniquely supported catalog entry (%s)',async(state,expected)=>{
  const cli={...adapter,id:'codex-jsonl',protocol:'codex-jsonl-v1'},buddy={...cli,id:'codebuddy-cli',adapter:'codebuddy',protocol:'codebuddy-jsonl-v1'};
  const value=report(),onAutoSelect=vi.fn();
  value.report.items[0]={...value.report.items[0],installed:true,launch_supported:state!=='unsupported',
   reason_code:state==='unsupported'?'unsupported_launcher':'workspace_required',executable:'C:/tools/codex.exe',version:'0.141.0'} as typeof value.report.items[number];
  if(state==='ambiguous')value.report.items[1]={...value.report.items[0],adapter_id:buddy.id,product:'CodeBuddy Code'};
  if(state==='expired'){
   const now=Date.now();value.scan.observed_at=new Date(now-61000).toISOString();value.scan.expires_at=new Date(now-1000).toISOString();
  }
  scan.mockResolvedValueOnce(value);
  await act(async()=>root.render(<ApiViewProvider session={session}><LocalToolsScanPanel project={project}
   adapters={state==='no exact descriptor'?[adapter,buddy]:[cli,adapter,buddy]} compact onSelect={select} onAutoSelect={onAutoSelect}/></ApiViewProvider>));
  if(expected)expect(onAutoSelect).toHaveBeenCalledExactlyOnceWith(cli);
  else expect(onAutoSelect).not.toHaveBeenCalled();
  expect(host.querySelector<HTMLDetailsElement>('[data-local-tools-results]')?.open).toBe(false);
  expect(select).not.toHaveBeenCalled();expect(fetch).not.toHaveBeenCalled();
 });

it.each(['not_installed','unsupported_launcher'] as const)('shows a next step for the selected entry (%s) and retains it during retry',async(reason)=>{
 const cli={...adapter,id:'codex-jsonl',protocol:'codex-jsonl-v1'},advanced=vi.fn();
 const value=report();
 value.report.items[0]={...value.report.items[0],installed:reason==='unsupported_launcher',reason_code:reason};
 scan.mockResolvedValueOnce(value);
 const target=document.createElement('div');host.before(target);
 try{
  await act(async()=>root.render(<ApiViewProvider session={session}><LocalToolsScanPanel project={project} adapters={[cli]} selectedAdapterID={cli.id} compact feedbackTarget={target} onSelect={select} onAdvanced={advanced}/></ApiViewProvider>));
  expect(target.querySelector('[data-local-tools-next]')?.textContent).toContain(reason==='not_installed'?'Not detected on this computer':'Launch method not supported');
  await click(target.querySelector('[data-local-tools-next-guide]')!);expect(advanced).toHaveBeenCalledExactlyOnceWith(cli);
  let finish!:(response:unknown)=>void;scan.mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve;}));
  const retry=target.querySelector<HTMLButtonElement>('[data-local-tools-next-rescan]')!;retry.focus();await click(retry);
  expect(retry.getAttribute('aria-disabled')).toBe('true');expect(document.activeElement).toBe(retry);
  expect(scan).toHaveBeenLastCalledWith(expect.objectContaining({force:true}));
  await act(async()=>finish(value));
  expect(document.activeElement).toBe(retry);expect(select).not.toHaveBeenCalled();
 }finally{target.remove();}
});

it('offers fresh local shortcuts for distinct protocols and never auto-selects a setup-only entry',async()=>{
 vi.useFakeTimers({toFake:['setTimeout','clearTimeout','performance']});
 const cli={...adapter,id:'codex-jsonl',protocol:'codex-jsonl-v1'};
 const service={...adapter,entrypoint_kind:'app_server'};
 const buddy={...cli,id:'codebuddy-cli',product:'CodeBuddy Code',adapter:'codebuddy',protocol:'codebuddy-jsonl-v1'};
 const value=report(),onAutoSelect=vi.fn();
 const observed={installed:true,launch_supported:false,reason_code:'configuration_required',executable:'C:/tools/codex.exe',version:'0.141.0',executable_sha256:'a'.repeat(64)};
 value.report.items[0]={...value.report.items[0],...observed,launch_supported:true,reason_code:'workspace_required'};
 value.report.items[1]={...value.report.items[1],...observed,reason_code:'unsupported_launcher'};
 value.report.items[6]={...value.report.items[6],...observed};
 scan.mockResolvedValueOnce(value);
 await act(async()=>root.render(<ApiViewProvider session={session}><LocalToolsScanPanel project={project}
  adapters={[cli,service,buddy]} compact onSelect={select} onAutoSelect={onAutoSelect}/></ApiViewProvider>));
 const shortcut=(id:string)=>host.querySelector<HTMLButtonElement>('[data-local-tools-shortcut="'+id+'"]');
 expect(host.querySelector<HTMLDetailsElement>('[data-local-tools-results]')?.open).toBe(false);
 expect(shortcut(cli.id)?.textContent).toContain('Launch method confirmed');
 expect(shortcut(service.id)?.textContent).toContain('Configuration required');
 expect(shortcut(buddy.id)).toBeNull();
 expect(onAutoSelect).toHaveBeenCalledExactlyOnceWith(cli);
 await click(shortcut(service.id)!);expect(select).toHaveBeenCalledExactlyOnceWith(service);
 await act(async()=>vi.advanceTimersByTimeAsync(60001));
 expect(shortcut(cli.id)).toBeNull();expect(shortcut(service.id)).toBeNull();
 expect(scan).toHaveBeenCalledOnce();expect(fetch).not.toHaveBeenCalled();
});

it('T193 keeps nine scan facts separate from registration and offers only explicit existing-profile or SDK configuration navigation',async()=>{
 const sdk={...adapter,id:'claude-code-sdk',product:'Claude Agent SDK',adapter:'claude',protocol:'claude-sdk-v1'} as AdapterDescriptor,onViewExisting=vi.fn();
 async function draw(disabled=false){await act(async()=>root.render(<ApiViewProvider session={session}><LocalToolsScanPanel project={project} adapters={[adapter,sdk]} disabled={disabled} onSelect={select} onViewExisting={onViewExisting}/></ApiViewProvider>));}
 await draw();expect(host.querySelectorAll('[data-local-tools-adapter]')).toHaveLength(9);expect(host.querySelector('[data-local-tools-adapter="claude-code-sdk"]')).not.toBeNull();expect(host.querySelector('[data-local-tools-manual-support]')).toBeNull();expect(host.querySelector('[data-local-tools-select="claude-code-sdk"]')).not.toBeNull();
 const basis=host.querySelector<HTMLDetailsElement>('[data-local-tools-basis="codex-app-server"]');expect(basis).not.toBeNull();expect(basis!.open).toBe(false);expect(basis!.textContent).toContain(adapter.protocol);
 const existing=host.querySelector<HTMLButtonElement>('[data-local-tools-existing="codex-app-server"]');expect(existing).not.toBeNull();expect(onViewExisting).not.toHaveBeenCalled();await click(existing!);expect(onViewExisting).toHaveBeenCalledExactlyOnceWith(adapter);expect(select).not.toHaveBeenCalled();
 await click(host.querySelector<HTMLButtonElement>('[data-local-tools-select="claude-code-sdk"]')!);expect(select).toHaveBeenCalledExactlyOnceWith(sdk);expect(onViewExisting).toHaveBeenCalledOnce();
 await draw(true);expect(host.querySelector<HTMLButtonElement>('[data-local-tools-existing="codex-app-server"]')!.disabled).toBe(true);await click(host.querySelector<HTMLButtonElement>('[data-local-tools-existing="codex-app-server"]')!);expect(onViewExisting).toHaveBeenCalledOnce();expect(scan).toHaveBeenCalledOnce();expect(fetch).not.toHaveBeenCalled();
});

it('automatically scans once and only forces an explicit refresh, retaining locale and nine protocol facts without connecting',async()=>{
 await render(false,project,session,true);expect(scan).toHaveBeenCalledExactlyOnceWith({action:'scan',project_id:project,user_id:user,tenant_id:tenant,locale:'en-US'});expect(host.querySelectorAll('[data-local-tools-adapter]')).toHaveLength(9);expect(select).not.toHaveBeenCalled();
 await act(async()=>setLocale('zh-CN',{persist:false}));await render(false,project,session,true);expect(scan).toHaveBeenCalledOnce();expect(host.textContent).toContain('本机开发工具');
 vi.useFakeTimers({toFake:['setTimeout','clearTimeout']});const cached=report(project,true),now=Date.now();cached.scan.observed_at=new Date(now-59900).toISOString();cached.scan.expires_at=new Date(now+100).toISOString();scan.mockResolvedValueOnce(cached);await click(rescan());expect(scan).toHaveBeenLastCalledWith({action:'scan',project_id:project,user_id:user,tenant_id:tenant,locale:'zh-CN',force:true});expect(host.textContent).toContain('缓存');await act(async()=>vi.advanceTimersByTimeAsync(101));expect(host.textContent).toContain('已过期');expect(host.querySelectorAll('[data-local-tools-adapter]')).toHaveLength(9);
 await click(host.querySelector<HTMLButtonElement>('[data-local-tools-select=cursor-cli]')!);expect(select).toHaveBeenCalledExactlyOnceWith(cursor);expect(scan).toHaveBeenCalledTimes(2);expect(fetch).not.toHaveBeenCalled();
});
it('defers disabled auto-scan, locks selection, drops late scope results and rejects invalid reports',async()=>{
 await render(true);expect(scan).not.toHaveBeenCalled();await render(false);expect(scan).toHaveBeenCalledOnce();await render(true);expect(rescan().disabled).toBe(true);expect(host.querySelector<HTMLButtonElement>('[data-local-tools-select=codex-app-server]')!.disabled).toBe(true);
 await render(false);let finish!:(value:unknown)=>void;scan.mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve;}));await click(rescan());expect(rescan().disabled).toBe(true);
 const next='9'.repeat(32);scan.mockResolvedValueOnce(report(next));await render(false,next);await act(async()=>finish({...report(),report:{...report().report,items:report().report.items.map(item=>({...item,product:'Late private result'}))}}));expect(host.textContent).not.toContain('Late private result');expect(host.querySelectorAll('[data-local-tools-adapter]')).toHaveLength(9);
 scan.mockResolvedValueOnce({...report(next),scan:{...report(next).scan,tenant_id:'8'.repeat(32)}});await click(rescan());expect(host.querySelectorAll('[data-local-tools-adapter]')).toHaveLength(0);expect(host.textContent).toContain('could not be verified');expect(select).not.toHaveBeenCalled();expect(fetch).not.toHaveBeenCalled();
});
it('offers the reviewed Cursor Node package as configuration only and selects the precise ACP descriptor without connecting',async()=>{
 const current={...cursor,implementation:'implemented',setup_kind:'runner_profile',adapter:'cursor',protocol:'acp-stdio-v1'} as AdapterDescriptor,value=report();
 value.report.items[4]={...value.report.items[4],installed:true,reason_code:'acp_configuration_required',executable:'C:/tools/cursor-agent/node.exe',version:'Cursor fixture version',executable_sha256:'a'.repeat(64)} as typeof value.report.items[number];scan.mockResolvedValueOnce(value);
 await act(async()=>root.render(<ApiViewProvider session={session}><LocalToolsScanPanel project={project} adapters={[current]} onSelect={select}/></ApiViewProvider>));
 const row=host.querySelector('[data-local-tools-adapter="cursor-cli"]')!;expect(row).not.toBeNull();expect(row.textContent).toContain('Complete the connection configuration');expect(row.textContent).toContain('Cursor fixture version');
 const choose=row.querySelector<HTMLButtonElement>('[data-local-tools-select="cursor-cli"]')!;expect(choose.textContent).toBe('Configure integration');await click(choose);expect(select).toHaveBeenCalledExactlyOnceWith(current);expect(scan).toHaveBeenCalledOnce();expect(fetch).not.toHaveBeenCalled();
});

it('T202 keeps installed, exceptional and selected facts primary while folding only other missing tools with their original actions and protocols',async()=>{
 const value=report(),sdk={...adapter,id:'claude-code-sdk',product:'Claude Agent SDK',adapter:'claude',protocol:'claude-sdk-v1'} as AdapterDescriptor,onViewExisting=vi.fn();
 value.report.items[5]={...value.report.items[5],reason_code:'probe_failed'};
 value.report.items[6]={...value.report.items[6],installed:true,reason_code:'configuration_required',executable:'C:/tools/codex.exe',version:'0.141.0',executable_sha256:'a'.repeat(64)} as typeof value.report.items[number];
 value.report.items[8]={...value.report.items[8],installed:true,reason_code:'version_unsupported',executable:'C:/tools/claude.exe',version:'2.1.313',executable_sha256:'b'.repeat(64)} as typeof value.report.items[number];scan.mockResolvedValueOnce(value);
 await act(async()=>root.render(<ApiViewProvider session={session}><LocalToolsScanPanel project={project} adapters={[adapter,cursor,sdk]} selectedAdapterID={cursor.id} onSelect={select} onViewExisting={onViewExisting}/></ApiViewProvider>));
 const primary=host.querySelector('[data-local-tools-primary]')!,other=host.querySelector<HTMLDetailsElement>('[data-local-tools-other]')!;
 expect(primary).not.toBeNull();expect(other).not.toBeNull();expect(other.open).toBe(false);
 expect([...primary.querySelectorAll<HTMLElement>('[data-local-tools-adapter]')].map(row=>row.dataset.localToolsAdapter)).toEqual(['cursor-cli','qoder-acp','codex-app-server','claude-code-sdk']);
 expect([...other.querySelectorAll<HTMLElement>('[data-local-tools-adapter]')].map(row=>row.dataset.localToolsAdapter)).toEqual(['codex-jsonl','codebuddy-cli','dsh-multica','claude-code-cli','opencode-sdk']);
 expect(host.querySelector('[data-local-tools-installation-summary]')!.textContent).toBe('Installations detected: 2 · Checks needing attention: 1');
 const sdkRow=primary.querySelector('[data-local-tools-adapter="claude-code-sdk"]')!;expect(sdkRow.textContent).toContain('Prepare Claude Code 2.1.314');expect(sdkRow.textContent).toContain('2.1.313');expect(sdkRow.textContent).toContain('claude-sdk-v1');
 const existing=primary.querySelector<HTMLButtonElement>('[data-local-tools-existing="codex-app-server"]')!;await click(existing);expect(onViewExisting).toHaveBeenCalledExactlyOnceWith(adapter);
 expect(primary.querySelector('[data-local-tools-adapter="qoder-acp"] button')).toBeNull();expect(host.querySelectorAll('[data-local-tools-adapter]')).toHaveLength(9);expect(scan).toHaveBeenCalledOnce();expect(select).not.toHaveBeenCalled();expect(fetch).not.toHaveBeenCalled();
});

it('T202 uses native disclosure and keeps a newly selected missing tool visible while a scope remount resets disclosure without extra scans',async()=>{
 async function draw(p=project,selectedAdapterID?:string){await act(async()=>root.render(<ApiViewProvider session={session}><LocalToolsScanPanel project={p} adapters={[adapter,cursor]} selectedAdapterID={selectedAdapterID} onSelect={select}/></ApiViewProvider>));}
 await draw();const other=host.querySelector<HTMLDetailsElement>('[data-local-tools-other]')!,summary=other?.querySelector('summary');expect(other).not.toBeNull();expect(other.open).toBe(false);expect(summary?.tagName).toBe('SUMMARY');
 summary!.focus();expect(document.activeElement).toBe(summary);expect(summary!.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',bubbles:true,cancelable:true}))).toBe(true);
 // The native summary owns activation; no custom key handler or scan is needed.
 await act(async()=>summary!.click());expect(other.open).toBe(true);expect(scan).toHaveBeenCalledOnce();
 const choose=other.querySelector<HTMLButtonElement>('[data-local-tools-select="cursor-cli"]')!;choose.focus();await click(choose);expect(select).toHaveBeenCalledExactlyOnceWith(cursor);await draw(project,cursor.id);
 expect(host.querySelector('[data-local-tools-primary] [data-local-tools-adapter="cursor-cli"]')).not.toBeNull();expect(host.querySelector('[data-local-tools-other] [data-local-tools-adapter="cursor-cli"]')).toBeNull();expect(scan).toHaveBeenCalledOnce();
 expect(document.activeElement).toBe(host.querySelector('[data-local-tools-primary] [data-local-tools-select="cursor-cli"]'));
 rescan().focus();await draw(project,adapter.id);expect(document.activeElement).toBe(rescan());expect(select).toHaveBeenCalledOnce();
 const next='9'.repeat(32);scan.mockResolvedValueOnce(report(next));await draw(next);expect(host.querySelector<HTMLDetailsElement>('[data-local-tools-other]')!.open).toBe(false);expect(scan).toHaveBeenCalledTimes(2);expect(fetch).not.toHaveBeenCalled();
});

it('T202 gives the empty primary list its existing node entry and localizes folded observations without implicit rechecking',async()=>{
 const onWorker=vi.fn();async function draw(disabled=false){await act(async()=>root.render(<ApiViewProvider session={session}><LocalToolsScanPanel project={project} adapters={[adapter,cursor]} disabled={disabled} onSelect={select} onWorker={onWorker}/></ApiViewProvider>));}
 await draw();expect(host.querySelector('[data-local-tools-primary]')).toBeNull();expect(host.querySelector('[data-local-tools-empty]')!.textContent).toContain('No installation detected');
 let worker=host.querySelector<HTMLButtonElement>('[data-local-tools-worker]')!;await click(worker);expect(onWorker).toHaveBeenCalledOnce();expect(scan).toHaveBeenCalledOnce();
 await act(async()=>setLocale('zh-CN',{persist:false}));expect(host.querySelector('[data-local-tools-other] > summary')!.textContent).toContain('其他未发现工具（9）');expect(host.querySelector('[data-local-tools-installation-summary]')!.textContent).toBe('检测到安装：0 · 需要处理的检测：0');expect(scan).toHaveBeenCalledOnce();
 await click(rescan());expect(scan).toHaveBeenCalledTimes(2);expect(scan).toHaveBeenLastCalledWith({action:'scan',project_id:project,user_id:user,tenant_id:tenant,locale:'zh-CN',force:true});await draw(true);worker=host.querySelector<HTMLButtonElement>('[data-local-tools-worker]')!;expect(worker.disabled).toBe(true);await click(worker);expect(onWorker).toHaveBeenCalledOnce();expect(select).not.toHaveBeenCalled();expect(fetch).not.toHaveBeenCalled();
});

it('T226 opens the precise failed tool guide without changing selection and only rechecks locally on an explicit safe action',async()=>{
 const sdk={...adapter,id:'claude-code-sdk',product:'Claude Code',adapter:'claude',protocol:'claude-sdk-v1'} as AdapterDescriptor,onAdvanced=vi.fn();
 function failed(diagnostic:ToolInstallationProbeDiagnostic){const value=report();value.report.items[8]={...value.report.items[8],installed:true,reason_code:'probe_failed',diagnostic} as typeof value.report.items[number];return value;}
 scan.mockResolvedValueOnce(failed({stage:'help',code:'help_contract_mismatch',next_action:'guide'}));
 async function draw(advanced=false,disabled=false){await act(async()=>root.render(<ApiViewProvider session={session}><LocalToolsScanPanel project={project} adapters={[adapter,sdk]} selectedAdapterID={adapter.id} disabled={disabled} onSelect={select} {...(advanced?{onAdvanced}:{})}/></ApiViewProvider>));}
 await draw();let row=host.querySelector('[data-local-tools-adapter="claude-code-sdk"]')!;
 expect(row.querySelector('[data-tool-probe-diagnostic="help_contract_mismatch"]')).not.toBeNull();expect(row.textContent).toContain('Required command arguments were not confirmed');expect(row.querySelector('[data-local-tools-select]')).toBeNull();expect(row.querySelector('[data-local-tools-diagnostic-action]')).toBeNull();expect(scan).toHaveBeenCalledOnce();
 await draw(true);await click(host.querySelector<HTMLButtonElement>('[data-local-tools-diagnostic-action="guide"]')!);expect(onAdvanced).toHaveBeenCalledExactlyOnceWith(sdk);expect(select).not.toHaveBeenCalled();expect(scan).toHaveBeenCalledOnce();
 await act(async()=>setLocale('zh-CN',{persist:false}));expect(host.textContent).toContain('未能确认所需命令参数');
 scan.mockResolvedValueOnce(failed({stage:'version',code:'probe_timeout',next_action:'scan'}));await click(rescan());expect(scan).toHaveBeenCalledTimes(2);expect(host.querySelector('[data-local-tools-diagnostic-action="guide"]')).toBeNull();
 await draw(true,true);expect(host.querySelector<HTMLButtonElement>('[data-local-tools-diagnostic-action="scan"]')!.disabled).toBe(true);await click(host.querySelector<HTMLButtonElement>('[data-local-tools-diagnostic-action="scan"]')!);expect(scan).toHaveBeenCalledTimes(2);
 await draw(true);scan.mockResolvedValueOnce(failed({stage:'version',code:'stop_unverified',next_action:'none'}));await click(host.querySelector<HTMLButtonElement>('[data-local-tools-diagnostic-action="scan"]')!);expect(scan).toHaveBeenCalledTimes(3);expect(scan).toHaveBeenLastCalledWith({action:'scan',project_id:project,user_id:user,tenant_id:tenant,locale:'zh-CN',force:true});
 row=host.querySelector('[data-local-tools-adapter="claude-code-sdk"]')!;expect(row.textContent).toContain('尚未确认检测进程已停止');expect(row.querySelector('[data-local-tools-diagnostic-action]')).toBeNull();expect(rescan().disabled).toBe(true);await click(rescan());expect(scan).toHaveBeenCalledTimes(3);expect(fetch).not.toHaveBeenCalled();
});
