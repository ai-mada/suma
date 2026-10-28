import {test} from 'node:test';
import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {mkdir,mkdtemp,rm,readFile} from 'node:fs/promises';
import {resolve,join,dirname,sep} from 'node:path';
import {createHash} from 'node:crypto';
const entry=resolve('dist/suma-opencode.bundle.mjs');
async function cli(args:string[],env:NodeJS.ProcessEnv,input?:unknown){return await new Promise<{code:number|null;out:string;err:string}>((resolve,reject)=>{const child=spawn(process.execPath,[entry,...args],{env:{...process.env,...env},windowsHide:true,stdio:[input===undefined?'ignore':'pipe','pipe','pipe']});let out='',err='';child.stdout!.on('data',chunk=>out+=String(chunk));child.stderr!.on('data',chunk=>err+=String(chunk));if(input!==undefined)child.stdin!.end(JSON.stringify(input)+'\n');child.once('error',reject);child.once('close',code=>resolve({code,out,err}));});}
test('closed bundle version/help and invalid configuration are bilingual without credentials or model access',async()=>{
 const version=await cli(['--version'],{SUMA_OPENCODE_CONFIG:'invalid'});assert.deepEqual(version,{code:0,out:'suma-opencode-runtime 1.0.0\n',err:''});
 for(const language of ['zh-CN','en-US']){const help=await cli(['--help'],{SUMA_LANGUAGE:language});assert.equal(help.code,0);assert.ok(help.out.includes('opencode-sdk-v1'));const failure=await cli(['--probe'],{SUMA_LANGUAGE:language,SUMA_OPENCODE_CONFIG:'{"secret":"must-not-echo"}'});assert.equal(failure.code,2);assert.equal(failure.out,'');assert.ok(!failure.err.includes('must-not-echo'));assert.match(failure.err,language==='zh-CN'?/配置无效/:/configuration is invalid/);}
});
test('offline probe rejects a real local executable with a foreign version and never emits a capability ACK',async()=>{
 const cache=resolve('.cache/tests');await mkdir(cache,{recursive:true});const root=await mkdtemp(join(cache,'probe-negative-'));
 try{const config={executable:process.execPath,executable_sha256:createHash('sha256').update(await readFile(process.execPath)).digest('hex'),home:join(root,'home'),source:join(root,'source'),provider_id:'fixed',model:'fixed-model',base_url:'http://127.0.0.1:1/v1',configuration_sha256:'a'.repeat(64),tool_entry:resolve('dist/suma-workspace.tool.mjs'),plugin_package_root:resolve('dist/opencode-plugin')};await mkdir(config.source);const result=await cli(['--probe'],{SUMA_LANGUAGE:'en-US',SUMA_OPENCODE_CONFIG:JSON.stringify(config)});assert.equal(result.code,1);assert.equal(result.out,'');assert.equal(result.err,'OpenCode runtime probe failed.\n');}
 finally{if(dirname(resolve(root))!==cache||!root.startsWith(cache+sep))throw Error('unsafe test cleanup');await rm(root,{recursive:true,force:true});}
});
test('startup failure emits only the private parent-close acknowledgement and no tree-proof alias',async()=>{
 const cache=resolve('.cache/tests');await mkdir(cache,{recursive:true});const root=await mkdtemp(join(cache,'startup-parent-'));
 try{const config={executable:process.execPath,executable_sha256:'0'.repeat(64),home:join(root,'home'),source:join(root,'source'),provider_id:'fixed',model:'fixed-model',base_url:'http://127.0.0.1:1/v1',configuration_sha256:'a'.repeat(64),tool_entry:resolve('dist/suma-workspace.tool.mjs'),plugin_package_root:resolve('dist/opencode-plugin')};await mkdir(config.source);
  const input={v:1,type:'execute',request_id:'startup-parent',goal:'fixture',workspace_bridge:{version:1,endpoint:'http://127.0.0.1:1/mcp',token:'fixture-bridge-token',policy_sha256:'a'.repeat(64),request_id:'startup-parent'}};
  const result=await cli([],{SUMA_LANGUAGE:'en-US',SUMA_OPENCODE_CONFIG:JSON.stringify(config)},input);assert.equal(result.code,1);const frames=result.out.trim().split('\n').map(line=>JSON.parse(line));assert.equal(frames.length,1);assert.equal(frames[0].type,'failed');assert.equal(frames[0].request_id,'startup-parent');assert.equal(frames[0].server_parent_stopped,true);assert.equal(Object.hasOwn(frames[0],'server_stopped'),false);
 }finally{if(dirname(resolve(root))!==cache||!root.startsWith(cache+sep))throw Error('unsafe test cleanup');await rm(root,{recursive:true,force:true});}
});
