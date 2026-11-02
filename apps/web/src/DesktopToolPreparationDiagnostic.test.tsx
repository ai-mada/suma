// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import {DesktopToolConnector} from './DesktopToolConnector';
import {ClaudeSDKLocalConnector} from './ClaudeSDKLocalConnector';
import {OpenCodeLocalConnector} from './OpenCodeLocalConnector';
import {setLocale} from './i18n';

const project='1'.repeat(32),session={user:{id:'2'.repeat(32),name:'Owner',email:'owner@example.test'},organization:{id:'3'.repeat(32),name:'Team'}};
let host:HTMLDivElement,root:Root,manage:ReturnType<typeof vi.fn>,detect:ReturnType<typeof vi.fn>,guide:ReturnType<typeof vi.fn>,configure:ReturnType<typeof vi.fn>;
const unavailable=(adapter_id='claude-code-sdk',preparation_reason?:string)=>({status:'ok',connection:{project_id:project,adapter_id,phase:'unavailable',...(preparation_reason?{preparation_reason}:{})}});
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);sessionStorage.clear();host=document.createElement('div');document.body.append(host);root=createRoot(host);guide=vi.fn();configure=vi.fn();
 manage=vi.fn(async()=>unavailable('claude-code-sdk','sdk_assets_missing'));
 detect=vi.fn(async(request:{adapter_id:string})=>({status:'observed',report:{protocol:'suma-installed-tools-v1',items:[{adapter_id:request.adapter_id,product:request.adapter_id==='opencode-sdk'?'OpenCode':'Claude Code',installed:true,launch_supported:false,executable:'C:/tools/'+(request.adapter_id==='opencode-sdk'?'opencode.exe':'claude.exe'),executable_sha256:'a'.repeat(64),version:request.adapter_id==='opencode-sdk'?'1.19.4':'2.1.314',reason_code:'configuration_required'}]}}));
 vi.stubGlobal('sumaDesktop',{platform:'win32',version:'1.0.0',manageLocalTool:manage,discoverInstalledTools:detect});vi.stubGlobal('fetch',vi.fn(()=>{throw Error('No HTTP expected');}));
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});
async function renderPlain(showGuide=true){await act(async()=>root.render(<ApiViewProvider session={session}><DesktopToolConnector project={project} adapterID="claude-code-sdk" onConnected={vi.fn()} onGuide={showGuide?guide:undefined} onConfigure={configure}/></ApiViewProvider>));}
async function click(selector:string){const button=host.querySelector<HTMLButtonElement>(selector);expect(button).not.toBeNull();expect(button!.disabled).toBe(false);await act(async()=>button!.click());}

it('T203 renders only the classified SDK cause, keeps absent callbacks hidden and reconciles unknown original reads without a new setup',async()=>{
 await renderPlain();expect(host.querySelector('[data-tool-connection-reason="sdk_assets_missing"]')).not.toBeNull();
 expect(host.querySelector('.desktop-tool-connector')?.getAttribute('data-phase')).toBe('unavailable');expect(host.querySelector('.desktop-tool-state')).toBeNull();
 expect(host.textContent).toContain('SDK resources are missing');expect(detect).not.toHaveBeenCalled();
 await click('[data-tool-connection-action="guide"]');expect(guide).toHaveBeenCalledOnce();expect(manage).toHaveBeenCalledOnce();
 await renderPlain(false);expect(host.querySelector('[data-tool-connection-action="guide"]')).toBeNull();
 await renderPlain();manage.mockResolvedValueOnce(unavailable());await click('[data-local-tool-status]');expect(host.querySelector('[data-tool-connection-diagnostic]')).toBeNull();expect(host.querySelector('.desktop-tool-state')?.textContent).toContain('Check the Git workspace');
 await click('[data-local-tool-status]');expect(host.querySelector('[data-tool-connection-reason="sdk_assets_missing"]')).not.toBeNull();
 manage.mockRejectedValueOnce(Error('untrusted stderr token=fixture-private203'));await click('[data-local-tool-status]');
 expect(host.querySelector('[data-tool-connection-reason="original_operation"]')).not.toBeNull();
 expect(host.querySelector('[data-tool-connection-action="guide"]')).toBeNull();expect(host.querySelector('[data-tool-connection-action="configure"]')).toBeNull();expect(host.querySelector('[data-tool-connection-action="detect"]')).toBeNull();
 expect(host.textContent).not.toContain('fixture-private203');await click('[data-tool-connection-action="reconcile"]');
 expect(manage.mock.calls.every(([request])=>request.action==='status'&&!Object.hasOwn(request,'claude_sdk_setup'))).toBe(true);expect(detect).not.toHaveBeenCalled();expect(fetch).not.toHaveBeenCalled();
});

it('T203 focuses the public credential reference only for its verified cause in both SDK forms and preserves other configuration targets',async()=>{
 for(const adapterID of ['claude-code-sdk','opencode-sdk']){
  manage.mockImplementation(async()=>unavailable(adapterID,'credential_reference_unavailable'));
  const connected=vi.fn();await act(async()=>root.render(<ApiViewProvider session={session}>{adapterID==='claude-code-sdk'?<ClaudeSDKLocalConnector project={project} onConnected={connected}/>:<OpenCodeLocalConnector project={project} onConnected={connected}/>}</ApiViewProvider>));
  await click('.desktop-tool-connector [data-tool-connection-action="configure"]');
  expect(document.activeElement).toBe(host.querySelector(adapterID==='claude-code-sdk'?'[name=credential_env_ref]':'[name=api_key_env_ref]'));
  await click(adapterID==='claude-code-sdk'?'.claude-sdk-local > .tool-connection-diagnostic [data-tool-connection-action="configure"]':'.opencode-local > .tool-connection-diagnostic [data-tool-connection-action="configure"]');
  expect(document.activeElement).toBe(host.querySelector(adapterID==='claude-code-sdk'?'[name=model]':'[name=provider_id]'));
  expect(connected).not.toHaveBeenCalled();expect(manage.mock.calls.every(([request])=>request.action==='status')).toBe(true);
 }
 expect(fetch).not.toHaveBeenCalled();
});
