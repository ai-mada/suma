import {test} from 'node:test';
import assert from 'node:assert/strict';
import {resolve} from 'node:path';
import {parseConfig,generatedConfig,childEnvironment} from '../src/config.js';
import {assertEffectiveConfig} from '../src/session.js';
const root=resolve('.cache','fixture');
const base={executable:resolve(root,'opencode.exe'),executable_sha256:'a'.repeat(64),home:resolve(root,'home'),source:resolve(root,'source'),provider_id:'fixed',model:'fixed-model',base_url:'http://127.0.0.1:1234/v1',configuration_sha256:'b'.repeat(64),tool_entry:resolve(root,'dist/suma-workspace.tool.mjs'),plugin_package_root:resolve(root,'dist/opencode-plugin')};
test('generated configuration fixes the model and suppresses all repository, builtin and auxiliary authority',()=>{
 const config=parseConfig(base),generated=generatedConfig(config),permission=generated.permission as Record<string,string>;
 assert.equal(permission['*'],'deny');assert.equal(permission.suma_workspace_write,'allow');assert.equal(permission.bash,undefined);assert.equal(generated.model,'fixed/fixed-model');assert.deepEqual(generated.mcp,{});assert.deepEqual(generated.plugin,[]);assert.deepEqual((generated.agent as any).title,{disable:true,options:{},permission:{'*':'deny'}});assert.deepEqual(generated.compaction,{auto:false,prune:false});
 const env=childEnvironment(config,generated,undefined,{AWS_SECRET_ACCESS_KEY:'private',NODE_OPTIONS:'bad',OPENCODE_CONFIG:'foreign',npm_config_registry:'foreign',HTTP_PROXY:'foreign',SUMA_SECRET:'private',PATH:'foreign'});
 for(const name of ['AWS_SECRET_ACCESS_KEY','NODE_OPTIONS','OPENCODE_CONFIG','npm_config_registry','HTTP_PROXY','SUMA_SECRET'])assert.equal(env[name],undefined);
 assert.equal(env.OPENCODE_PURE,'true');assert.equal(env.OPENCODE_DISABLE_DEFAULT_PLUGINS,'true');assert.equal(env.npm_config_offline,'true');assert.equal(env.OPENCODE_CONFIG_DIR,resolve(config.home,'config/opencode'));
});
test('closed local configuration rejects user endpoint credentials and source-contained homes',()=>{
 for(const patch of [{base_url:'https://user:password@example.test/v1'},{base_url:'http://example.test/v1'},{home:resolve(root,'source','home')},{arbitrary_config:{}},{model:'foreign model'},{api_key_env_ref:'bad\nkey'}])assert.throws(()=>parseConfig({...base,...patch}));
 const configured=parseConfig({...base,api_key_env_ref:'APPROVED_MODEL_KEY'});assert.throws(()=>generatedConfig(configured,{}));assert.ok(JSON.stringify(generatedConfig(configured,{APPROVED_MODEL_KEY:'secret'})).includes('secret'));assert.equal(childEnvironment(configured,{},undefined,{APPROVED_MODEL_KEY:'secret'}).APPROVED_MODEL_KEY,undefined);
});
test('the pinned server agent normal form passes exact configuration checks without allowing extra authority',()=>{
 const expected=generatedConfig(parseConfig(base));const actual=structuredClone(expected) as Record<string,any>;
 // 1.19.4 ConfigAgentV1.Info.decode always adds options and permission,
 // including on disabled agents; /config returns this decoded value.
 for(const agent of Object.values(actual.agent) as Record<string,unknown>[]){agent.options={...agent.options as Record<string,unknown>};agent.permission={...agent.permission as Record<string,unknown>};}
 actual.mode={};actual.plugin_origins=[];
 assert.doesNotThrow(()=>assertEffectiveConfig(actual,expected));
 for(const mutate of [(value:any)=>{value.agent.suma.permission.bash='allow';},(value:any)=>{value.agent.title.disable=false;},(value:any)=>{value.agent.foreign={mode:'primary',permission:{'*':'allow'}};},(value:any)=>{value.provider.fixed.options.baseURL='https://foreign.test/v1';},(value:any)=>{value.mcp.foreign={type:'remote',url:'https://foreign.test/mcp'};},(value:any)=>{value.mode.foreign={permission:{'*':'allow'}};},(value:any)=>{value.plugin_origins.push({spec:'foreign-plugin'});}]){const altered=structuredClone(actual);mutate(altered);assert.throws(()=>assertEffectiveConfig(altered,expected));}
});
