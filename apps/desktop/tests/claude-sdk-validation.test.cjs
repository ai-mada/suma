'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),crypto=require('node:crypto');
test('Claude SDK setup remains closed and the complete configuration uses original GoJSON',()=>{
 const v=require('../src/local-claude-sdk-validation.cjs');
 const setup={executable_sha256:'2'.repeat(64),model:'claude-sonnet-5',credential_env_ref:'OWNED_CLAUDE_KEY'};
 assert.equal(v.validSetup(setup),true);
 for(const credential_env_ref of ['LANG','PATHEXT'])assert.equal(v.validSetup({...setup,credential_env_ref}),false,'OS mechanics cannot carry a credential into a passive probe');
 for(const patch of [{api_key:'secret'},{host:'/untrusted/host'},{credential_env_ref:'CLAUDE_KEY'},{credential_env_ref:'_KEY'},{credential_env_ref:'ANTHROPIC_AUTH_TOKEN'},{model:'other-model'},{model:'claude-name space'}])assert.equal(v.validSetup({...setup,...patch}),false);
 const value={native_cli_executable:'C:\\owned\\a<>&\u2028\u2029\\claude.exe',model:setup.model,credential_env_ref:setup.credential_env_ref,reviewed_cli_version:'2.1.314',node_sha256:'1'.repeat(64),native_cli_sha256:'2'.repeat(64),runtime_manifest_sha256:'3'.repeat(64)};
 assert.equal(v.configurationDigest(value),'bc843c029ba91046dc701cb35dfc7e54724e3d50ad258cf0d086d9376fdd2ff6');
 const cap=v.capability({...value,configuration_sha256:v.configurationDigest(value)});
 assert.equal(v.matchesCapability({...value,configuration_sha256:v.configurationDigest(value)},cap),true);assert.equal(v.matchesCapability(value,{...cap,budget_mode:'enforce'}),false);
 assert.equal(crypto.createHash('sha256').update(v.goJSON({x:'<>&\u2028\u2029'})).digest('hex').length,64);
});
