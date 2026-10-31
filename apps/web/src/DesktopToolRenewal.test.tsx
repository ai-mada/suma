// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import {DesktopToolConnector} from './DesktopToolConnector';
import {setLocale} from './i18n';
const project='1'.repeat(32),worker='desktop-'+'4'.repeat(32),sha='a'.repeat(64),selection={installation_id:'5'.repeat(32),probe_id:'6'.repeat(32)};
const session={user:{id:'2'.repeat(32),name:'Owner',email:'owner@example.test'},organization:{id:'3'.repeat(32),name:'Team'}};
const base={project_id:project,user_id:session.user.id,tenant_id:session.organization.id,adapter_id:'codex-app-server',locale:'en-US'};
const facts={project_id:project,adapter_id:base.adapter_id,worker_id:worker,tool_version:'codex-cli fixture',executable_sha256:'b'.repeat(64),configuration_sha256:sha,catalog_sha256:'c'.repeat(64),expires_at:new Date(Date.now()+60000).toISOString(),credential_expires_at:'2026-10-30T00:00:00Z',credential_version:3};
const response=(phase:string,extra:Record<string,unknown>={})=>({status:'ok',connection:{...facts,phase,...extra}});
let root:Root,host:HTMLDivElement,manage:ReturnType<typeof vi.fn>,connected:ReturnType<typeof vi.fn>;
beforeEach(async()=>{await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);connected=vi.fn();manage=vi.fn(async()=>response('stopped'));vi.stubGlobal('sumaDesktop',{platform:'win32',version:'1.0.0',manageLocalTool:manage});vi.stubGlobal('fetch',vi.fn(()=>{throw Error('Unexpected HTTP');}));});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});
async function render(expected=sha){await act(async()=>root.render(<ApiViewProvider session={session}><DesktopToolConnector project={project} adapterID={base.adapter_id} codexSelection={selection} expectedConfigurationSHA={expected} onConnected={connected}/></ApiViewProvider>));}
async function click(selector:string){const button=host.querySelector<HTMLButtonElement>(selector);expect(button).not.toBeNull();expect(button!.disabled).toBe(false);await act(async()=>button!.click());}
it('shows distinct credential and inventory expiries and renews without connecting until an explicit original reconnect',async()=>{
 await render();expect(host.querySelector('[data-desktop-credential-expiry] time')?.getAttribute('datetime')).toBe(facts.credential_expires_at);expect(host.querySelector('[data-desktop-inventory-expiry] time')?.getAttribute('datetime')).toBe(facts.expires_at);expect(host.querySelector('[data-desktop-credential-version]')?.textContent).toBe('3');
 manage.mockImplementation(async request=>response(request.action==='renew'?'prepared':'connected',{credential_version:4,credential_expires_at:'2090-01-02T00:00:00Z'}));await click('[data-desktop-credential-renew]');
 expect(manage.mock.calls.map(call=>call[0])).toEqual([{...base,action:'status'},{...base,action:'renew'}]);expect(connected).not.toHaveBeenCalled();expect(host.querySelector('[data-desktop-credential-version]')?.textContent).toBe('4');
 await click('[data-local-tool-connect]');expect(manage.mock.calls.at(-1)![0]).toEqual({...base,action:'connect'});expect(connected).toHaveBeenCalledExactlyOnceWith(worker);expect(fetch).not.toHaveBeenCalled();
});
it('keeps an unknown renewal on the exact renew action while an edited draft cannot turn it into a new connection',async()=>{
 await render();manage.mockImplementationOnce(async()=>{throw Error('lost renewal response');});await click('[data-desktop-credential-renew]');
 expect(host.querySelector('[data-local-tool-connect]')).toBeNull();expect(host.querySelector('[data-desktop-credential-result]')?.textContent).toContain('original renewal');
 await render('d'.repeat(64));manage.mockImplementation(async()=>response('renewal_unknown',{renewal_reason:'execution_pending'}));await click('[data-desktop-credential-renew]');
 expect(manage.mock.calls.at(-1)![0]).toEqual({...base,action:'renew'});expect(host.querySelector('[data-local-tool-connect]')).toBeNull();
 expect(host.querySelector('[data-desktop-credential-reason]')?.textContent).toBeTruthy();
 manage.mockImplementationOnce(async()=>{throw Error('status unavailable');});await click('[data-local-tool-status]');expect(host.querySelector('p[role="status"].desktop-tool-warning')?.textContent).toContain('renewal');expect(host.querySelector('[data-local-tool-connect]')).toBeNull();
 manage.mockImplementation(async()=>response('prepared',{credential_version:4}));await click('[data-desktop-credential-renew]');expect(manage.mock.calls.filter(call=>call[0].action==='connect')).toHaveLength(0);expect(connected).not.toHaveBeenCalled();
});
it('presents a known drain refusal without pretending a renewal is pending or an identity was issued',async()=>{
 await render();manage.mockImplementationOnce(async()=>({status:'failed',code:'managed_runner_renewal_blocked'}));await click('[data-desktop-credential-renew]');
 expect(host.querySelector('[data-tool-connection-reason]')?.getAttribute('data-tool-connection-reason')).toBe('renewal_blocked');expect(host.querySelector('[data-desktop-credential-result]')).toBeNull();expect(host.querySelector('[data-desktop-credential-version]')?.textContent).toBe('3');
 expect(connected).not.toHaveBeenCalled();expect(manage.mock.calls.at(-1)![0]).toEqual({...base,action:'renew'});
});
