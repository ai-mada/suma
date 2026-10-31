// @vitest-environment jsdom
import {createHash,webcrypto} from 'node:crypto';
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import {OpenCodeLocalConnector} from './OpenCodeLocalConnector';
import {setLocale} from './i18n';
const project='1'.repeat(32),worker='desktop-'+'4'.repeat(32),nativeSHA='a'.repeat(64),executable='C:/tools/opencode.exe';
const session={user:{id:'2'.repeat(32),name:'Owner',email:'owner@example.test'},organization:{id:'3'.repeat(32),name:'Team'}};
let host:HTMLDivElement,root:Root;
beforeEach(async()=>{await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.stubGlobal('crypto',webcrypto);sessionStorage.clear();host=document.createElement('div');document.body.append(host);root=createRoot(host);vi.stubGlobal('fetch',vi.fn(()=>{throw Error('Unexpected HTTP');}));});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();sessionStorage.clear();vi.unstubAllGlobals();vi.restoreAllMocks();});
async function settle(){await act(async()=>{for(let i=0;i<4;i++)await new Promise(resolve=>setTimeout(resolve,0));});}
async function click(action:string){const button=host.querySelector<HTMLButtonElement>('[data-local-tool-'+action+']');expect(button).not.toBeNull();expect(button!.disabled).toBe(false);await act(async()=>button!.click());await settle();}
async function change(name:string,value:string){const input=host.querySelector<HTMLInputElement>('[name="'+name+'"]')!;await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(input,value);input.dispatchEvent(new Event('input',{bubbles:true}));});await settle();}
it('recovers an exact lost connection through real component status and applies a changed same-origin setup only after confirmed stop',async()=>{
 const connected=vi.fn(),ready=vi.fn(),requests:Record<string,unknown>[]=[];let phase='unconfigured',configSHA='',loseResponse=true;
 let config={provider_id:'approved',model:'fixed-model',base_url:'https://models.example.test/v1',api_key_env_ref:'MODEL_PROVIDER_KEY'};
 const manageLocalTool=vi.fn(async(request:Record<string,unknown>)=>{requests.push(request);if(request.action==='connect'){
  if(request.opencode_setup){const {executable_sha256:_,...submitted}=request.opencode_setup as Record<string,string>;config=submitted as typeof config;configSHA=createHash('sha256').update(JSON.stringify({policy_version:1,sdk_version:'1.19.4',provider:{executable,provider_id:config.provider_id,model:config.model,base_url:config.base_url,api_key_env_ref:config.api_key_env_ref}})).digest('hex');}
  phase='connected';if(loseResponse){loseResponse=false;throw Error('lost response');}
 }else if(request.action==='stop')phase='stopped';
 return {status:'ok',connection:{project_id:project,adapter_id:'opencode-sdk',phase,...(phase==='unconfigured'?{}:{worker_id:worker,tool_version:'1.19.4',executable_sha256:nativeSHA,model:config.model,provider_id:config.provider_id,endpoint_origin:'https://models.example.test',configuration_sha256:configSHA})}};});
 const discoverInstalledTools=vi.fn(async()=>({status:'observed',report:{protocol:'suma-installed-tools-v1',items:[{adapter_id:'opencode-sdk',product:'OpenCode',installed:true,launch_supported:false,reason_code:'configuration_required',executable,executable_sha256:nativeSHA,version:'1.19.4'}]}}));
 vi.stubGlobal('sumaDesktop',{platform:'win32',version:'1.0.0',manageLocalTool,discoverInstalledTools});
 await act(async()=>root.render(<ApiViewProvider session={session}><OpenCodeLocalConnector project={project} onConnected={connected} onRegistrationReadyChange={ready}/></ApiViewProvider>));await settle();
 for(const [name,value] of Object.entries(config))await change(name,value);
 expect(discoverInstalledTools).toHaveBeenCalledOnce();await click('connect');expect(connected).not.toHaveBeenCalled();
 const originalSetup=requests.find(request=>request.action==='connect')!.opencode_setup;expect(originalSetup).toEqual({...config,executable_sha256:nativeSHA});
 await click('status');expect(requests.at(-1)).toMatchObject({action:'status'});expect(requests.at(-1)).not.toHaveProperty('opencode_setup');expect(connected).toHaveBeenCalledExactlyOnceWith({worker_id:worker,model:'fixed-model',provider_id:'approved',endpoint_origin:'https://models.example.test',tool_version:'1.19.4',executable_sha256:nativeSHA,configuration_sha256:configSHA});
 const originalSHA=configSHA;await change('base_url','https://models.example.test/other');expect(ready).toHaveBeenLastCalledWith(false);await click('status');expect(connected).toHaveBeenCalledOnce();
 await click('stop');await click('connect');expect(requests.at(-1)).toMatchObject({action:'connect',opencode_setup:{base_url:'https://models.example.test/other'}});expect(configSHA).not.toBe(originalSHA);expect(connected).toHaveBeenCalledTimes(2);expect(ready).toHaveBeenLastCalledWith(true);
 await click('stop');await click('connect');expect(connected).toHaveBeenCalledTimes(3);await click('status');expect(connected).toHaveBeenCalledTimes(3);expect(fetch).not.toHaveBeenCalled();
});
