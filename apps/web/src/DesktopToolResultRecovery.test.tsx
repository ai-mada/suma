// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import {DesktopToolConnector} from './DesktopToolConnector';
import {setLocale} from './i18n';

const project='1'.repeat(32),worker='desktop-'+'4'.repeat(32),sha='a'.repeat(64),operation='5'.repeat(32);
const selection={installation_id:'6'.repeat(32),probe_id:'7'.repeat(32)};
const session={user:{id:'2'.repeat(32),name:'Owner',email:'owner@example.test'},organization:{id:'3'.repeat(32),name:'Team'}};
const base={project_id:project,user_id:session.user.id,tenant_id:session.organization.id,adapter_id:'codex-app-server',locale:'en-US'};
const facts={project_id:project,adapter_id:base.adapter_id,worker_id:worker,tool_version:'codex-cli fixture',executable_sha256:'b'.repeat(64),configuration_sha256:sha,catalog_sha256:'c'.repeat(64),credential_expires_at:'2026-10-30T00:00:00Z',credential_version:3};
const response=(extra:Record<string,unknown>={})=>({status:'ok',connection:{...facts,phase:'authorization_required',...extra}});
const recovery=(phase:string,extra:Record<string,unknown>={})=>({operation_id:operation,phase,total:3,recovered:1,remaining:2,...extra});
let root:Root,host:HTMLDivElement,manage:ReturnType<typeof vi.fn>,connected:ReturnType<typeof vi.fn>,blocked:ReturnType<typeof vi.fn>;
beforeEach(async()=>{await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);connected=vi.fn();blocked=vi.fn();manage=vi.fn(async()=>response());vi.stubGlobal('sumaDesktop',{platform:'win32',version:'1.0.0',manageLocalTool:manage});vi.stubGlobal('fetch',vi.fn(()=>{throw Error('Unexpected HTTP');}));});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});
async function render(expected=sha){await act(async()=>root.render(<ApiViewProvider session={session}><DesktopToolConnector project={project} adapterID={base.adapter_id} codexSelection={selection} expectedConfigurationSHA={expected} onConnected={connected} onBlockedChange={blocked}/></ApiViewProvider>));}
async function click(selector:string){const button=host.querySelector<HTMLButtonElement>(selector);expect(button).not.toBeNull();expect(button!.disabled).toBe(false);await act(async()=>button!.click());}

it('starts an explicit scope-only receipt recovery and keeps renewal and connection independent',async()=>{
 await render();expect(host.querySelector('[data-desktop-recovery-total]')).toBeNull();expect(manage).toHaveBeenCalledExactlyOnceWith({...base,action:'status'});
 manage.mockImplementation(async request=>response(request.action==='renew'?{phase:'prepared',credential_version:4}:{recovery:recovery('complete',{recovered:3,remaining:0})}));
 await click('[data-desktop-recovery-action]');expect(manage.mock.calls.at(-1)![0]).toEqual({...base,action:'recover'});
 expect(host.querySelector('[data-desktop-recovery-state]')?.getAttribute('data-desktop-recovery-state')).toBe('complete');expect(host.querySelector('[data-desktop-recovery-total]')?.textContent).toBe('3');expect(host.querySelector('[data-desktop-recovery-remaining]')?.textContent).toBe('0');
 expect(host.querySelector('[data-desktop-credential-version]')?.textContent).toBe('3');expect(host.querySelector('[data-local-tool-connect]')).toBeNull();expect(connected).not.toHaveBeenCalled();expect(fetch).not.toHaveBeenCalled();
 await click('[data-desktop-credential-renew]');expect(manage.mock.calls.map(call=>call[0].action)).toEqual(['status','recover','renew']);expect(host.querySelector('[data-desktop-credential-version]')?.textContent).toBe('4');expect(connected).not.toHaveBeenCalled();
});
it('keeps lost receipt recovery on its original action and permits read-only status without a new plan',async()=>{
 await render();manage.mockImplementationOnce(async()=>{throw Error('lost recovery response');});await click('[data-desktop-recovery-action]');
 expect(host.querySelector('[data-desktop-recovery-state]')?.getAttribute('data-desktop-recovery-state')).toBe('unknown');expect(blocked).toHaveBeenLastCalledWith(true);expect(host.querySelector('[data-local-tool-connect]')).toBeNull();expect(host.querySelector('[data-desktop-credential-renew]')).toBeNull();
 await render('d'.repeat(64));manage.mockImplementationOnce(async()=>response({recovery:recovery('unknown',{reason_code:'result_unknown'})}));await click('[data-desktop-recovery-read]');expect(manage.mock.calls.at(-1)![0]).toEqual({...base,action:'recovery-status'});expect(manage.mock.calls.filter(call=>call[0].action==='recover')).toHaveLength(1);
 manage.mockImplementationOnce(async()=>{throw Error('status transport unavailable');});await click('[data-desktop-recovery-read]');expect(host.querySelector('[data-desktop-recovery-state]')?.getAttribute('data-desktop-recovery-state')).toBe('unknown');expect(host.querySelector('[data-desktop-recovery-recovered]')?.textContent).toBe('1');expect(host.querySelector('.desktop-tool-connector > p[role="status"]')?.textContent).toContain('credential');
 manage.mockImplementationOnce(async()=>response({recovery:recovery('complete',{recovered:3,remaining:0})}));await click('[data-desktop-recovery-action]');expect(manage.mock.calls.filter(call=>call[0].action==='recover').map(call=>call[0])).toEqual([{...base,action:'recover'},{...base,action:'recover'}]);expect(connected).not.toHaveBeenCalled();
});
it('preserves the original unknown renewal ahead of receipt recovery',async()=>{
 manage.mockImplementation(async()=>response({phase:'renewal_unknown',recovery:recovery('unknown',{reason_code:'result_unknown'})}));await render();
 expect(host.querySelector('[data-desktop-recovery-action]')).toBeNull();expect(host.querySelector('[data-desktop-recovery-read]')).toBeNull();expect(host.querySelector('[data-local-tool-connect]')).toBeNull();
 manage.mockImplementationOnce(async()=>response({phase:'prepared',credential_version:4}));await click('[data-desktop-credential-renew]');expect(manage.mock.calls.map(call=>call[0].action)).toEqual(['status','renew']);expect(connected).not.toHaveBeenCalled();
});
it('retains partial and blocked evidence without reporting a connected worker or a cleared batch',async()=>{
 manage.mockImplementation(async()=>response({recovery:recovery('partial',{reason_code:'receipt_missing'})}));await render();
 expect(host.querySelector('[data-desktop-recovery-state]')?.getAttribute('data-desktop-recovery-state')).toBe('partial');expect(host.querySelector('[data-desktop-recovery-remaining]')?.textContent).toBe('2');expect(host.querySelector('[data-desktop-recovery-reason]')?.textContent).toContain('receipt');expect(blocked).toHaveBeenLastCalledWith(true);
 manage.mockImplementationOnce(async()=>response({recovery:recovery('blocked',{reason_code:'unsupported_pending'})}));await click('[data-desktop-recovery-read]');
 expect(host.querySelector('[data-desktop-recovery-state]')?.getAttribute('data-desktop-recovery-state')).toBe('blocked');expect(host.querySelector('[data-desktop-recovery-recovered]')?.textContent).toBe('1');expect(host.querySelector('[data-desktop-credential-renew]')).toBeNull();expect(connected).not.toHaveBeenCalled();expect(fetch).not.toHaveBeenCalled();
});
it('honors explicit access and binding refusals from a read-only recovery check while retaining the original batch',async()=>{
 manage.mockImplementation(async()=>response({phase:'stopped',recovery:recovery('partial',{reason_code:'receipt_missing'})}));await render();
 manage.mockImplementationOnce(async()=>({status:'failed',code:'managed_runner_access_changed'}));await click('[data-desktop-recovery-read]');
 expect(host.querySelector('.desktop-tool-connector > p[role="status"]')?.textContent).toContain('credential expiry');expect(host.querySelector('[data-desktop-recovery-action]')).toBeNull();expect(host.querySelector('[data-desktop-credential-renew]')).toBeNull();expect(host.querySelector('[data-desktop-recovery-recovered]')?.textContent).toBe('1');
 await click('[data-local-tool-status]');expect(host.querySelector('[data-desktop-recovery-action]')).not.toBeNull();
 manage.mockImplementationOnce(async()=>({status:'failed',code:'binding_changed'}));await click('[data-desktop-recovery-read]');
 expect(host.querySelector('.desktop-tool-connector > p[role="status"]')?.textContent).toContain('folder binding changed');expect(host.querySelector('[data-desktop-recovery-action]')).toBeNull();expect(host.querySelector('[data-desktop-recovery-remaining]')?.textContent).toBe('2');expect(connected).not.toHaveBeenCalled();
});
