import {expect,it} from 'vitest';
import {desktopCodexBridge,validDesktopCodexScope,validDesktopCodexConfiguration,validDesktopCodexSelection,validDesktopCodexRequest,makeDesktopCodexRequest,desktopCodexRecoveryScope,readDesktopCodexResult,type DesktopCodexScope} from './desktop-codex-connector-model';
const id=(c:string)=>c.repeat(32),hash=(c:string)=>c.repeat(64);
const scope:DesktopCodexScope={project:id('1'),userID:id('2'),organizationID:id('3'),adapterID:'codex-app-server'};
const configuration={provider_id:'teamProvider',base_url:'https://provider.example/v1',api_key_env_ref:'TEAM_API_KEY',windows_sandbox:'unelevated' as const};
const options={locale:'en-US' as const,installation_id:id('4'),operation_id:id('5'),configuration};
function response(){const now=Date.now();return {status:'ok',observation:{project_id:scope.project,user_id:scope.userID,tenant_id:scope.organizationID,adapter_id:scope.adapterID,phase:'ready',installations:[{installation_id:id('4'),label:'Codex',kind:'native',tool_version:'codex-cli 0.134.0',executable_sha256:hash('a')}],probe:{id:id('6'),operation_id:id('5'),installation_id:id('4'),source_sha256:hash('b'),created_at:new Date(now-1000).toISOString(),expires_at:new Date(now+59000).toISOString(),discovery:{protocol:'suma-codex-model-discovery-v1',state:'ready',executable_sha256:hash('a'),tool_version:'codex-cli 0.134.0',configuration_sha256:hash('c'),catalogue:{protocol:'codex-app-server-v1',configuration_sha256:hash('c'),catalog_sha256:hash('d'),models:[{model:'model-a',reasoning_efforts:['high'],default_reasoning_effort:'high'}]}}}}};}
it('uses the existing bridge and closes each Codex request around current caller and original operation',()=>{
 expect(desktopCodexBridge({platform:'win32',version:'1.0.0',manageLocalTool:async()=>({})})).toBeDefined();
 expect(desktopCodexBridge({platform:'win32',version:'1.0.0',inspectLocalACP:async()=>({})})).toBeUndefined();
 expect(validDesktopCodexScope(scope)).toBe(true);expect(validDesktopCodexScope({...scope,adapterID:'codex-jsonl'})).toBe(false);
 const request=makeDesktopCodexRequest('models',scope,options,'win32');expect(validDesktopCodexRequest(request,scope,'win32')).toBe(true);
 expect(makeDesktopCodexRequest('model-status',scope,{locale:'en-US',operation_id:options.operation_id})).toEqual({project_id:scope.project,user_id:scope.userID,tenant_id:scope.organizationID,adapter_id:scope.adapterID,locale:'en-US',action:'model-status',operation_id:options.operation_id});
 for(const patch of [{user_id:id('8')},{locale:['en-US']},{action:'status'},{operation_id:'new'},{model:'guess'},{configuration:{...configuration,argv:['run']}}])expect(validDesktopCodexRequest({...request,...patch},scope,'win32')).toBe(false);
 expect(validDesktopCodexRequest({...request,action:'detect'},scope,'win32')).toBe(false);
 expect(desktopCodexRecoveryScope(scope)).not.toBe(desktopCodexRecoveryScope({...scope,userID:id('8')}));
 expect(validDesktopCodexSelection({installation_id:id('4'),probe_id:id('6')})).toBe(true);expect(validDesktopCodexSelection({installation_id:id('4'),probe_id:id('6'),model_id:'model-a'})).toBe(false);
});
it('permits only safe provider drafts and explicit Windows sandbox without accepting secrets or process configuration',()=>{
 expect(validDesktopCodexConfiguration(configuration,'win32')).toBe(true);
 expect(validDesktopCodexConfiguration({...configuration,enable_user_input:true},'win32')).toBe(true);expect(validDesktopCodexConfiguration({...configuration,enable_user_input:false},'win32')).toBe(true);
 for(const enable_user_input of [null,'true',1])expect(validDesktopCodexConfiguration({...configuration,enable_user_input},'win32')).toBe(false);
 const {windows_sandbox:_,...portable}=configuration;expect(validDesktopCodexConfiguration(portable,'linux')).toBe(true);expect(validDesktopCodexConfiguration(portable,'win32')).toBe(false);
 for(const patch of [{base_url:'http://remote.example/v1'},{base_url:'https://user:secret@provider.example/v1'},{base_url:'https://provider.example/v1?'},{base_url:'https://provider.example/v1#'},{base_url:'https://provider.example/\u2028v1'},{api_key:'secret'},{executable:'/private/codex'},{api_key_env_ref:'SUMA_RUNNER_TOKEN'},{api_key_env_ref:'NODE_OPTIONS'},{windows_sandbox:'disabled'},{windows_sandbox:['unelevated']},{provider_id:'bad.name'}])expect(validDesktopCodexConfiguration({...configuration,...patch},'win32')).toBe(false);
 expect(validDesktopCodexConfiguration({...configuration,base_url:'http://127.0.0.1:1234/v1'},'win32')).toBe(true);
});
it('binds a ready catalogue to exact installation configuration caller operation and live probe',()=>{
 const value=response(),o=value.observation,p=o.probe,d=p.discovery;
 expect(readDesktopCodexResult(value,scope,id('5'))).toEqual(value);
 expect(readDesktopCodexResult(value,{...scope,organizationID:id('9')},id('5'))).toBeUndefined();expect(readDesktopCodexResult(value,scope,id('9'))).toBeUndefined();
 for(const patch of [{installation_id:id('9')},{expires_at:new Date(Date.now()-1).toISOString()},{expires_at:new Date(Date.parse(p.created_at)+60001).toISOString()},{source_sha256:'bad'},{token:'secret'}])expect(readDesktopCodexResult({...value,observation:{...o,probe:{...p,...patch}}},scope)).toBeUndefined();
 for(const patch of [{configuration_sha256:hash('e')},{executable_sha256:hash('e')},{tool_version:'changed'},{runtime:'qoder'},{catalogue:null}])expect(readDesktopCodexResult({...value,observation:{...o,probe:{...p,discovery:{...d,...patch}}}},scope)).toBeUndefined();
 expect(readDesktopCodexResult({...value,observation:{...o,installations:[{...o.installations[0],kind:'wrapper_only'}]}},scope)).toBeUndefined();
});
it('keeps unknown expired and unavailable observations unusable without discarding original source locators',()=>{
 const value=response(),o=value.observation,p=o.probe,{catalogue:_,...discovery}=p.discovery;
 const unknown={...value,observation:{...o,phase:'unknown',probe:{...p,discovery:{...discovery,state:'unknown',error_code:'codex_discovery_unknown'}}}};
 expect(readDesktopCodexResult(unknown,scope,id('5'))).toEqual(unknown);
 expect(readDesktopCodexResult({...unknown,observation:{...unknown.observation,probe:{...unknown.observation.probe,discovery:{...unknown.observation.probe.discovery,catalogue:p.discovery.catalogue}}}},scope)).toBeUndefined();
 const expired={...value,observation:{...o,phase:'expired',probe:{...p,created_at:new Date(Date.now()-61000).toISOString(),expires_at:new Date(Date.now()-1000).toISOString()}}};expect(readDesktopCodexResult(expired,scope)).toEqual(expired);
 const noInstall={status:'ok',observation:{project_id:scope.project,user_id:scope.userID,tenant_id:scope.organizationID,adapter_id:scope.adapterID,phase:'not_installed',installations:[]}};expect(readDesktopCodexResult(noInstall,scope)).toEqual(noInstall);
 expect(readDesktopCodexResult({status:'failed',code:'codex_discovery_authorization_changed'},scope)).toEqual({status:'failed',code:'codex_discovery_authorization_changed'});
});
