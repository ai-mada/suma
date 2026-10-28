import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createServer,type IncomingMessage} from 'node:http';
import {runSession} from '../src/session.js';
import {generatedConfig} from '../src/config.js';
import type {Frame,RuntimeConfig,ExecuteRequest} from '../src/contract.js';

async function fixture(options:{badParent?:boolean;error?:boolean;pending?:boolean;disconnect?:boolean;hold?:boolean;stopFail?:boolean;holdCreate?:boolean;completedAt?:number;tokens?:unknown;effectiveConfig?:unknown;toolErrorProgress?:boolean}={}){
 const calls:string[]=[];let active:any;let stopped=false;
 const server=createServer(async(req,res)=>{let raw='';for await(const chunk of req)raw+=String(chunk);const url=new URL(req.url!,'http://localhost');calls.push(req.method+' '+url.pathname);res.setHeader('Content-Type','application/json');
  assert.equal(req.headers.authorization,'Basic fixture');
  if(url.pathname==='/global/health'){res.end(JSON.stringify({healthy:true,version:'1.19.4'}));return;}
  if(url.pathname==='/config'){res.end(JSON.stringify(options.effectiveConfig??{}));return;}
  if(url.pathname==='/session'&&req.method==='POST'){if(options.holdCreate)return;res.end(JSON.stringify({id:'ses_fixture'}));return;}
  if(url.pathname==='/session/ses_fixture/abort'){res.end('true');active?.end(JSON.stringify({info:{error:{name:'MessageAbortedError'}}}));return;}
  if(url.pathname==='/session/ses_fixture/message'&&req.method==='POST'){
   const input=JSON.parse(raw);assert.equal(input.agent,'suma');assert.deepEqual(input.model,{providerID:'fixed',modelID:'fixed-model'});assert.deepEqual(input.parts,[{type:'text',text:'fix'}]);
   if(options.disconnect){req.socket.destroy();return;}if(options.hold){active=res;return;}if(options.toolErrorProgress)await new Promise(resolve=>setTimeout(resolve,50));
   res.end(JSON.stringify({info:{id:'msg_assistant',role:'assistant',sessionID:'ses_fixture',parentID:options.badParent?'foreign':input.messageID,providerID:'fixed',modelID:'fixed-model',time:{created:1,completed:options.completedAt??2},finish:'stop',...(options.error?{error:{name:'APIError'}}:{}),...(options.tokens?{tokens:options.tokens}:{})},parts:options.pending?[{type:'tool',state:{status:'pending'}}]:[{type:'text',text:'done'}]}));return;
  }
  if(url.pathname==='/event'){res.writeHead(200,{'Content-Type':'text/event-stream'});res.write('data: '+JSON.stringify({type:'session.status',properties:{sessionID:'ses_fixture',status:{type:'idle'}}})+'\n\n');if(options.toolErrorProgress)res.write('data: '+JSON.stringify({type:'message.part.updated',properties:{part:{type:'tool',sessionID:'ses_fixture',messageID:'msg_tool',callID:'call_write',tool:'suma_workspace_write',state:{status:'error',error:'private fixture error must not leak'}}}})+'\n\n');return;}
  res.statusCode=404;res.end('{}');
 });
 await new Promise<void>(resolve=>server.listen(0,'127.0.0.1',resolve));const address=server.address();if(!address||typeof address==='string')throw Error('address');
 return {calls,url:'http://127.0.0.1:'+address.port,get stopped(){return stopped;},async stop(){if(options.stopFail)throw Error('not stopped');server.closeAllConnections();await new Promise<void>(resolve=>server.close(()=>resolve()));stopped=true;},async close(){server.closeAllConnections();await new Promise<void>(resolve=>server.close(()=>resolve()));}};
}
const config={provider_id:'fixed',model:'fixed-model',configuration_sha256:'a'.repeat(64),source:'D:/fixture',home:'D:/fixture'} as RuntimeConfig;
const request={v:1,type:'execute',request_id:'attempt',goal:'fix',workspace_bridge:{}} as ExecuteRequest;
test('actual SDK prompt completion emits terminal only after owned server stops',async()=>{
 const server=await fixture();const frames:Frame[]=[];try{await runSession(request,config,{...server,authorization:'Basic fixture'}, {},frame=>{if(frame.type==='completed')assert.equal(server.stopped,true);frames.push(frame);},new AbortController().signal);assert.deepEqual(frames.map(frame=>frame.type),['ready','started','progress','progress','completed']);assert.equal(frames.at(-1)?.server_parent_stopped,true);assert.equal(frames.at(-1)?.usage_state,'unreported');assert.equal(server.calls.filter(call=>call==='POST /session/ses_fixture/message').length,1);}finally{await server.close();}
});
test('private terminal acknowledges parent close without publishing a process-tree stop fact',async()=>{
 const server=await fixture(),frames:Frame[]=[];try{await runSession(request,config,{...server,authorization:'Basic fixture'}, {},frame=>frames.push(frame),new AbortController().signal);const final=frames.at(-1)!;assert.equal(final.type,'completed');assert.equal(final.server_parent_stopped,true);assert.equal(Object.hasOwn(final,'server_stopped'),false);}finally{await server.close();}
});
test('a failed tool observation remains failed progress without determining the final turn outcome',async()=>{
 const server=await fixture({toolErrorProgress:true}),frames:Frame[]=[];try{await runSession(request,config,{...server,authorization:'Basic fixture'}, {},frame=>frames.push(frame),new AbortController().signal);const progress=frames.filter(frame=>frame.type==='progress'&&frame.kind==='tool');assert.deepEqual(progress,[{v:1,request_id:'attempt',type:'progress',session_id:'ses_fixture',kind:'tool',status:'failed',tool:'write'}]);assert.equal(frames.at(-1)?.type,'completed');assert.ok(!JSON.stringify(frames).includes('private fixture error'));}finally{await server.close();}
});
test('actual SDK accepts the pinned decoded configuration and refuses authority drift before session creation',async()=>{
 const runtime={...config,base_url:'http://127.0.0.1:1234/v1'};const expected=generatedConfig(runtime);
 for(const drift of [false,true]){const actual=structuredClone(expected) as Record<string,any>;for(const agent of Object.values(actual.agent) as Record<string,unknown>[]){agent.options={...agent.options as Record<string,unknown>};agent.permission={...agent.permission as Record<string,unknown>};}actual.mode={};actual.plugin_origins=[];if(drift)actual.agent.suma.permission.bash='allow';
  const server=await fixture({effectiveConfig:actual}),frames:Frame[]=[];try{await runSession(request,runtime,{...server,authorization:'Basic fixture'},expected,frame=>frames.push(frame),new AbortController().signal);if(drift){assert.equal(frames.at(-1)?.type,'failed');assert.equal(frames.at(-1)?.code,'configuration_mismatch');assert.equal(server.calls.filter(call=>call==='POST /session').length,0);assert.equal(server.calls.filter(call=>call==='POST /session/ses_fixture/message').length,0);}else{assert.equal(frames.at(-1)?.type,'completed');assert.equal(server.calls.filter(call=>call==='POST /session/ses_fixture/message').length,1);}}finally{await server.close();}
 }
});
test('upstream default zero counters remain unreported and actual positive counters remain partial',async()=>{
 for(const tokens of [{input:0,output:0,reasoning:0,cache:{read:0,write:0}},{input:10,output:2,reasoning:0,cache:{read:0,write:0}}]){const server=await fixture({tokens}),frames:Frame[]=[];try{await runSession(request,config,{...server,authorization:'Basic fixture'}, {},frame=>frames.push(frame),new AbortController().signal);const final=frames.at(-1)!;assert.equal(final.type,'completed');if(tokens.input===0){assert.equal(final.usage,undefined);assert.equal(final.usage_state,'unreported');}else{assert.deepEqual(final.usage,{input_tokens:10,output_tokens:2});assert.equal(final.usage_state,'partial');}}finally{await server.close();}}
});
test('zero millisecond terminal timestamp is canonical and an accepted create with a lost response remains unknown',async()=>{
 const completed=await fixture({completedAt:1000}),frames:Frame[]=[];try{await runSession(request,config,{...completed,authorization:'Basic fixture'}, {},frame=>frames.push(frame),new AbortController().signal);assert.equal(frames.at(-1)?.completed_at,'1970-01-01T00:00:01Z');}finally{await completed.close();}
 const server=await fixture({holdCreate:true}),controller=new AbortController(),cancelled:Frame[]=[];try{const running=runSession(request,config,{...server,authorization:'Basic fixture'}, {},frame=>cancelled.push(frame),controller.signal);const deadline=Date.now()+1000;while(!server.calls.includes('POST /session')&&Date.now()<deadline)await new Promise(resolve=>setTimeout(resolve,5));assert.ok(server.calls.includes('POST /session'));controller.abort();await running;assert.equal(cancelled.at(-1)?.type,'failed');assert.equal(cancelled.at(-1)?.code,'cancel_unconfirmed');assert.equal(cancelled.at(-1)?.server_parent_stopped,true);assert.equal(server.calls.filter(call=>call==='POST /session').length,1);}finally{await server.close();}
});
test('mismatched, errored or incomplete assistant never succeeds even when HTTP returns 200',async()=>{
 for(const options of [{badParent:true},{error:true},{pending:true},{stopFail:true},{disconnect:true}]){const server=await fixture(options);const frames:Frame[]=[];try{await runSession(request,config,{...server,authorization:'Basic fixture'}, {},frame=>frames.push(frame),new AbortController().signal);assert.equal(frames.at(-1)?.type,'failed');assert.ok(!frames.some(frame=>frame.type==='completed'));assert.equal(server.calls.filter(call=>call==='POST /session/ses_fixture/message').length,1);}finally{await server.close();}}
});
test('cancel acknowledgement and confirmed server stop are separate required facts',async()=>{
 const server=await fixture({hold:true}),controller=new AbortController(),frames:Frame[]=[];try{const running=runSession(request,config,{...server,authorization:'Basic fixture'}, {},frame=>frames.push(frame),controller.signal);const deadline=Date.now()+1000;while(!server.calls.includes('POST /session/ses_fixture/message')&&Date.now()<deadline)await new Promise(resolve=>setTimeout(resolve,5));assert.ok(server.calls.includes('POST /session/ses_fixture/message'));controller.abort();await running;assert.equal(frames.at(-1)?.type,'cancelled');assert.equal(frames.at(-1)?.abort_acknowledged,true);assert.equal(frames.at(-1)?.server_parent_stopped,true);assert.equal(server.calls.filter(call=>call==='POST /session/ses_fixture/abort').length,1);}finally{await server.close();}
});
