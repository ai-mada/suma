import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createServer} from 'node:http';
import {mkdir,mkdtemp,rm,stat} from 'node:fs/promises';
import {resolve,join,dirname,sep} from 'node:path';
import {createHash} from 'node:crypto';
import type {ToolContext} from '@opencode-ai/plugin/tool';
import {hostTools} from '../src/tool-host.js';

test('official MCP connections do not retain the completed previous OpenCode stream abort signal',async()=>{
 const requests:{method:string;params?:Record<string,any>}[]=[];
 const fields:Record<string,string[]>={list:['directory','limit','cursor'],read:['path','start_line','max_lines'],write:['path','content','expected_sha256','expected_absent'],remove:['path','expected_sha256'],check:['check_id'],edit:['path','expected_sha256','edits']};
 const server=createServer(async(req,res)=>{res.setHeader('Content-Type','application/json');if(req.method!=='POST'){res.statusCode=405;res.end('{}');return;}let raw='';for await(const chunk of req)raw+=String(chunk);const message=JSON.parse(raw);requests.push({method:message.method,params:message.params});assert.equal(req.headers.authorization,'Bearer fixture-lifetime-token');
  if(message.method==='notifications/initialized'){res.statusCode=202;res.end();return;}
  let result:unknown={};
  if(message.method==='initialize')result={protocolVersion:'2025-11-25',serverInfo:{name:'suma-native-workspace',version:'attempt-workspace-v1'},capabilities:{tools:{}}};
  if(message.method==='tools/list')result={tools:Object.entries(fields).map(([name,names])=>({name:'suma_workspace_'+name,inputSchema:{type:'object',properties:Object.fromEntries(names.map(name=>[name,{}])),additionalProperties:false}}))};
  if(message.method==='tools/call'){const output=message.params.name==='suma_workspace_read'?{path:'fixture.txt',sha256:'a'.repeat(64),content:'original'}:{operation:{id:message.params._meta['suma/operation_id'],state:'committed'},path:'fixture.txt',sha256:'b'.repeat(64)};result={content:[{type:'text',text:JSON.stringify(output)}],structuredContent:output};}
  res.end(JSON.stringify({jsonrpc:'2.0',id:message.id,result}));
 });
 await new Promise<void>(resolve=>server.listen(0,'127.0.0.1',resolve));const address=server.address();if(!address||typeof address==='string')throw Error('fixture address');
 const cache=resolve('.cache/tests');await mkdir(cache,{recursive:true});const root=await mkdtemp(join(cache,'tool-lifetime-'));
 const names=['SUMA_OPENCODE_REQUEST_ID','SUMA_OPENCODE_WORKSPACE_BRIDGE','SUMA_OPENCODE_TOOL_FAILURE'] as const;const previous=Object.fromEntries(names.map(name=>[name,process.env[name]]));
 const readStream=new AbortController(),writeStream=new AbortController();
 const context=(signal:AbortSignal,messageID:string,callID:string)=>({sessionID:'ses_lifetime',messageID,callID,abort:signal,ask:async()=>{}} as unknown as ToolContext);
 try{
  process.env.SUMA_OPENCODE_REQUEST_ID='lifetime-attempt';process.env.SUMA_OPENCODE_WORKSPACE_BRIDGE=JSON.stringify({version:1,endpoint:'http://127.0.0.1:'+address.port+'/mcp',token:'fixture-lifetime-token',policy_sha256:'a'.repeat(64),request_id:'lifetime-attempt'});process.env.SUMA_OPENCODE_TOOL_FAILURE=join(root,'tool-failure.json');
  const host=hostTools();assert.equal(JSON.parse(await host.invoke('read',{path:'fixture.txt'},context(readStream.signal,'msg_read','call_read'))).sha256,'a'.repeat(64));
  // Pinned OpenCode llm.ts scopes each model stream and aborts its signal on
  // release. The next model stream has a new, still-live tool abort signal.
  readStream.abort();assert.equal(writeStream.signal.aborted,false);
  const written=JSON.parse(await host.invoke('write',{path:'fixture.txt',content:'fixed',expected_sha256:'a'.repeat(64)},context(writeStream.signal,'msg_write','call_write')));
  assert.equal(written.operation.state,'committed');
  const calls=requests.filter(item=>item.method==='tools/call');assert.deepEqual(calls.map(item=>item.params?.name),['suma_workspace_read','suma_workspace_write']);assert.equal(requests.filter(item=>item.method==='initialize').length,2);
  const stableIdentity='["lifetime-attempt","ses_lifetime","msg_write","call_write","write"]';assert.equal(calls[1].params?._meta['suma/operation_id'],createHash('sha256').update(JSON.stringify(['lifetime-attempt',0,stableIdentity])).digest('hex'));assert.equal(written.operation.id,calls[1].params?._meta['suma/operation_id']);
  await assert.rejects(stat(join(root,'tool-failure.json')),{code:'ENOENT'});
 }finally{readStream.abort();writeStream.abort();for(const name of names){if(previous[name]===undefined)delete process.env[name];else process.env[name]=previous[name];}server.closeAllConnections();await new Promise<void>(resolve=>server.close(()=>resolve()));if(dirname(resolve(root))!==cache||!root.startsWith(cache+sep))throw Error('unsafe test cleanup');await rm(root,{recursive:true,force:true});}
});
