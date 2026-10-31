import {expect,it,vi} from 'vitest';
import {webcrypto} from 'node:crypto';
import {validOpenCodeCapability} from './opencode-execution-model';
import {validDesktopOpenCodeConfiguration,validDesktopOpenCodeSetup,makeDesktopOpenCodeSetup,openCodeConfigurationSHA} from './desktop-opencode-connector-model';
const config={provider_id:'provider-one',model:'vendor/model-one',base_url:'https://provider.example/v1',api_key_env_ref:'PROVIDER_API_KEY'};
it('reproduces the Go OpenCode configuration digest with original ordered fields and HTML escaping',async()=>{
 vi.stubGlobal('crypto',webcrypto);
 try{
  const path='C:/tools/OpenCode <>&/opencode.exe',expected='c35514a91c9b68a3810da809b403ffad8244b2cb76589c464258b846fd195cb2';
  expect(await openCodeConfigurationSHA(config,path)).toBe(expected);
  expect(await openCodeConfigurationSHA(config,'C:/工具/<>&/opencode.exe')).toBe('42afdd7217c7e12d8e9b8eebab9ffb91506679a8790a783d0496c41210661035');
  expect(await openCodeConfigurationSHA({...config,api_key_env_ref:'OTHER_API_KEY'},path)).not.toBe(expected);
  expect(await openCodeConfigurationSHA({...config,base_url:'https://Provider.Example:443/v1'},path)).toMatch(/^[a-f0-9]{64}$/);
  expect(await openCodeConfigurationSHA({...config,base_url:'https://Provider.Example:443/v1'},path)).not.toBe(expected);
  expect(await openCodeConfigurationSHA({...config,base_url:'HTTPS://Provider.Example:443/v1'},path)).toBeUndefined();
  expect(await openCodeConfigurationSHA(config,'C:/other/opencode.exe')).not.toBe(expected);
  expect(await openCodeConfigurationSHA({...config,model:''},path)).toBeUndefined();
  for(const invalid of ['opencode.exe','C:relative.exe','C:/bad\ud800.exe','C:/bad\n.exe'])expect(await openCodeConfigurationSHA(config,invalid)).toBeUndefined();
  vi.stubGlobal('crypto',{});expect(await openCodeConfigurationSHA(config,path)).toBeUndefined();
 }finally{vi.unstubAllGlobals();}
});
it('validates fixed OpenCode configuration without accepting paths credentials or transport overrides',()=>{
 expect(validDesktopOpenCodeConfiguration(config)).toBe(true);
 expect(validDesktopOpenCodeConfiguration({...config,base_url:'https://Provider.Example:443/v1'})).toBe(true);
 expect(validDesktopOpenCodeConfiguration({...config,base_url:'HTTPS://Provider.Example:443/v1'})).toBe(false);
 const capability={sdk_version:'1.19.4',server_version:'1.19.4',provider_id:config.provider_id,model:config.model,endpoint_origin:'https://Provider.Example:443',configuration_sha256:'a'.repeat(64),budget_mode:'observe_only',identity_evidence:'configuration_ack_only'};
 expect(validOpenCodeCapability(capability)).toBe(true);
 for(const endpoint_origin of ['https://Provider.Example:443/','HTTPS://Provider.Example:443','http://LOCALHOST:8080'])expect(validOpenCodeCapability({...capability,endpoint_origin})).toBe(false);
 for(const base_url of ['http://127.0.0.1:2345/v1','http://[::1]:2345/v1'])expect(validDesktopOpenCodeConfiguration({...config,base_url})).toBe(true);
 for(const patch of [{provider_id:''},{model:'bad model'},{base_url:'http://remote.example/v1'},{base_url:'https://u:p@host/v1'},{base_url:'https://host/v1?'},{base_url:'https://host/v1#x'},{api_key_env_ref:'SUMA_TOKEN'},{api_key_env_ref:'OPENCODE_KEY'},{api_key_env_ref:'NODE_OPTIONS'},{api_key_env_ref:'NPM_TOKEN'},{api_key_env_ref:'HOME'},{api_key_env_ref:'secret-value'},{executable:'private.exe'},{args:[]},{api_key:'secret'}])expect(validDesktopOpenCodeConfiguration({...config,...patch}),JSON.stringify(Object.keys(patch))).toBe(false);
});
it('binds OpenCode setup to the observed native digest without guessing a configuration digest',()=>{
 const setup={...config,executable_sha256:'a'.repeat(64)};
 expect(makeDesktopOpenCodeSetup(config,setup.executable_sha256)).toEqual(setup);expect(validDesktopOpenCodeSetup(setup)).toBe(true);
 for(const patch of [{executable_sha256:'A'.repeat(64)},{executable_sha256:''},{configuration_sha256:'b'.repeat(64)},{model:'bad\nmodel'},{command:['run']},{provider_id:'bad\ud800'}])expect(validDesktopOpenCodeSetup({...setup,...patch})).toBe(false);
 expect(makeDesktopOpenCodeSetup({...config,model:''},setup.executable_sha256)).toBeUndefined();
});
