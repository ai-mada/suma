import {test} from 'node:test';
import assert from 'node:assert/strict';
import type {ToolContext} from '@opencode-ai/plugin/tool';
import {ToolHost} from '../src/tool-host.js';

function context(callID='call-one',ask:ToolContext['ask']=async()=>{}){return {sessionID:'ses_a',messageID:'msg_a',callID,abort:new AbortController().signal,ask} as unknown as ToolContext;}
test('custom host forwarding serializes calls and binds operation identities to full SDK context',async()=>{
 const seen:unknown[][]=[];let pending=0,highest=0;const asks:string[]=[];const host=new ToolHost('attempt',async()=>({invoke:async(...args)=>{seen.push(args);pending++;highest=Math.max(highest,pending);await new Promise(resolve=>setTimeout(resolve,5));pending--;return 'receipt';},close:async()=>{}}),async()=>{throw Error('unexpected fatal');});
 await Promise.all([host.invoke('write',{path:'a'},context('call-one',async input=>{asks.push(input.permission);})),host.invoke('check',{check_id:'fixed'},context('call-two',async input=>{asks.push(input.permission);}))]);
 assert.equal(highest,1);assert.deepEqual(asks,['suma_workspace_write','suma_workspace_check']);assert.deepEqual(seen,[['workspace.write','{"path":"a"}',0,'["attempt","ses_a","msg_a","call-one","write"]'],['workspace.check','{"check_id":"fixed"}',0,'["attempt","ses_a","msg_a","call-two","check"]']]);
});
test('missing call identity and unknown host effects permanently close host forwarding',async()=>{
 for(const missing of [true,false]){let calls=0,failures=0,closed=0;const host=new ToolHost('attempt',async()=>({invoke:async()=>{calls++;throw Error('unknown effect');},close:async()=>{closed++;}}),async()=>{failures++;});
  await assert.rejects(host.invoke('write',{},context(missing?'':'call-one')));await assert.rejects(host.invoke('write',{},context('call-two')));assert.equal(calls,missing?0:1);assert.equal(failures,1);assert.equal(closed,missing?0:1);
 }
});
