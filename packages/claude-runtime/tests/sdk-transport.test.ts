import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile,writeFile} from 'node:fs/promises';
import {fileURLToPath} from 'node:url';
import {join} from 'node:path';
import {query,type Query} from '@anthropic-ai/claude-agent-sdk/core';
import {ClaudeSession,type QueryFactory} from '../src/runtime.js';
import {sha256,SDK_VERSION,CLI_VERSION,type Frame,type PermissionSource,type InputRequest,type RuntimeConfig} from '../src/contract.js';
import {mkdir,mkdtemp} from 'node:fs/promises';
import {resolve} from 'node:path';

test('T192 actual pinned SDK transport carries native permission and question replies', {timeout:20000},async()=>{
 const cache=resolve('.cache/claude-runtime-tests');await mkdir(cache,{recursive:true});const root=await mkdtemp(join(cache,'sdk-transport-')),source=join(root,'workspace'),home=join(root,'home');await mkdir(source);
 const fixture=fileURLToPath(new URL('../../tests/native-fixture.js',import.meta.url)),log=join(root,'native-inbox.ndjson');await writeFile(log,'');
 const config:RuntimeConfig={native_cli_executable:fixture,native_cli_sha256:sha256(await readFile(fixture)),home,source,model:'claude-sonnet-5',credential_env_ref:'TEST_CLAUDE_KEY',configuration_sha256:'a'.repeat(64),sdk_version:SDK_VERSION,cli_version:CLI_VERSION};
 const request={v:1 as const,type:'execute' as const,request_id:'sdk-operation',goal:'Owned fixture only; no model.',configuration_sha256:config.configuration_sha256};
 const frames:Frame[]=[];let session:ClaudeSession;
 const actualSDK:QueryFactory=params=>{let actual:Query;try{actual=query({...params,options:{...params.options,executable:'node',env:{...params.options!.env,SUMA_CLAUDE_FIXTURE_LOG:log}}});}catch(error){void writeFile(log+'.sdk-startup-error.txt',String(error).replaceAll('fixture-no-network-key','[redacted]'));throw error;}return{async *[Symbol.asyncIterator](){try{yield*actual;}catch(error){await writeFile(log+'.sdk-error.txt',String(error).replaceAll('fixture-no-network-key','[redacted]'));throw error;}},close:()=>actual.close()} as Query;};
 session=new ClaudeSession(config,request,frame=>{frames.push(frame);if(frame.type==='permission_request'){const original=frame.source as PermissionSource;assert.equal(original.request_id,'native-permission-id');assert.equal(original.tool_use_id,'tool-permission');queueMicrotask(()=>session.handle({v:1,type:'permission_reply',request_id:request.request_id,source:original,decision:{outcome:'allow_once'}}));}if(frame.type==='input_request'){const original=frame.source as InputRequest;assert.equal(original.request_id,'native-question-id');assert.equal(original.tool_use_id,'tool-question');queueMicrotask(()=>session.handle({v:1,type:'input_reply',request_id:request.request_id,source:original,answer:{outcome:'answered',answers:[{question_id:'q1',option_ids:['o2','o1']}]}}));}},actualSDK,{TEST_CLAUDE_KEY:'fixture-no-network-key',PATH:process.env.PATH,SystemRoot:process.env.SystemRoot});
 const final=await session.run();assert.equal(final.type,'completed',JSON.stringify(final));const writes=(await readFile(log,'utf8')).trim().split(/\r?\n/).map(line=>JSON.parse(line));const replies=writes.filter(v=>v.type==='control_response');assert.equal(replies.length,2);assert.deepEqual(replies.map(v=>v.response.request_id),['native-permission-id','native-question-id']);assert.ok(writes.some(v=>v.type==='control_request'&&v.request.subtype==='initialize'));assert.ok(!writes.some(v=>v.fixture_error));assert.equal(frames.filter(v=>v.type==='initialized').length,1);assert.equal(frames.filter(v=>v.type==='permission_request').length,1);assert.equal(frames.filter(v=>v.type==='input_request').length,1);assert.equal((final.evidence as Record<string,unknown>).returned_count,2);
});
