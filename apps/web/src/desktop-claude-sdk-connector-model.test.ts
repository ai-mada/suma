import {expect,it} from 'vitest';
import {makeDesktopClaudeSDKSetup,matchesDesktopClaudeSDKSetup,validDesktopClaudeSDKConfiguration,validDesktopClaudeSDKSetup} from './desktop-claude-sdk-connector-model';

const configuration={model:'claude-sonnet-4-6',credential_env_ref:'TEAM_ANTHROPIC_KEY'},sha='a'.repeat(64),configSHA='b'.repeat(64);
const capability={sdk_version:'0.3.314',cli_version:'2.1.314',model:configuration.model,configuration_sha256:configSHA,model_source:'administrator_sdk_option',budget_mode:'observe_only',identity_evidence:'sdk_init_model_ack_only',auth_strategy:'api_key_env_ref'};
const connection={project_id:'1'.repeat(32),adapter_id:'claude-code-sdk',phase:'connected',worker_id:'desktop-'+'4'.repeat(32),model:configuration.model,credential_env_ref:configuration.credential_env_ref,tool_version:'2.1.314',executable_sha256:sha,runtime_executable_sha256:'c'.repeat(64),configuration_sha256:configSHA,claude_sdk_execution:capability};

it('T201 binds only public Claude SDK setup to native bytes and strict host capability without fabricating a digest',()=>{
 const setup={...configuration,executable_sha256:sha};
 expect(validDesktopClaudeSDKConfiguration(configuration)).toBe(true);
 expect(makeDesktopClaudeSDKSetup(configuration,sha)).toEqual(setup);
 expect(validDesktopClaudeSDKSetup(setup)).toBe(true);
 expect(matchesDesktopClaudeSDKSetup(setup,connection)).toBe(true);
 for(const phase of ['prepared','starting']){expect(matchesDesktopClaudeSDKSetup(setup,{...connection,phase})).toBe(false);expect(matchesDesktopClaudeSDKSetup(setup,{...connection,phase},false)).toBe(true);}
 for(const phase of ['stopped','failed','issuance_unknown'])expect(matchesDesktopClaudeSDKSetup(setup,{...connection,phase},false)).toBe(false);
 expect(matchesDesktopClaudeSDKSetup(setup,{...connection,phase:'starting',worker_id:undefined},false)).toBe(false);
 for(const patch of [{model:'model-one'},{model:'claude-unsafe\n4'},{credential_env_ref:'ANTHROPIC_AUTH_TOKEN'},{credential_env_ref:'NODE_OPTIONS'},{credential_env_ref:'SUMA_TOKEN'},{credential_env_ref:'CLAUDE_CONFIG_DIR'},{credential_env_ref:'COMSPEC'},{credential_env_ref:'sk-secret-value'},{api_key:'secret'},{native_cli_executable:'D:/cli.exe'},{configuration_sha256:configSHA},{argv:[]}])expect(validDesktopClaudeSDKConfiguration({...configuration,...patch}),JSON.stringify(Object.keys(patch))).toBe(false);
 for(const credential_env_ref of ['LANG','LC_ALL','PATHEXT','SYSTEMDRIVE','ALLUSERSPROFILE','PROGRAMFILES','PROGRAMW6432','COMMONPROGRAMFILES','COMMONPROGRAMW6432','HOMEDRIVE','HOMEPATH','PUBLIC','OS','LC_MEASUREMENT','LD_PRELOAD','DYLD_LIBRARY_PATH'])expect(validDesktopClaudeSDKConfiguration({...configuration,credential_env_ref}),credential_env_ref).toBe(false);
 for(const patch of [{executable_sha256:sha.toUpperCase()},{executable_sha256:''},{runtime_manifest:'private'},{claude_sdk_execution:capability}])expect(validDesktopClaudeSDKSetup({...setup,...patch})).toBe(false);
 for(const patch of [{phase:'stopped'},{adapter_id:'claude-code-cli'},{model:'claude-opus-4-6'},{tool_version:'2.1.315'},{executable_sha256:'d'.repeat(64)},{runtime_executable_sha256:undefined},{configuration_sha256:'d'.repeat(64)},{claude_sdk_execution:{...capability,budget_mode:'hard'}}])expect(matchesDesktopClaudeSDKSetup(setup,{...connection,...patch})).toBe(false);
 expect(matchesDesktopClaudeSDKSetup({...setup,credential_env_ref:'OTHER_KEY'},connection)).toBe(false);
 for(const credential_env_ref of [undefined,null,'','NODE_OPTIONS','sk-secret-value'])expect(matchesDesktopClaudeSDKSetup(setup,{...connection,credential_env_ref})).toBe(false);
});
