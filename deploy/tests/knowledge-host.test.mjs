import {test} from 'node:test';
import assert from 'node:assert/strict';
import {mkdtemp,mkdir,rm} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {createServer} from 'node:net';
import {spawn} from 'node:child_process';
import {once} from 'node:events';

async function unusedPort(){const socket=createServer();await new Promise((ok,fail)=>socket.once('error',fail).listen(0,'127.0.0.1',ok));const address=socket.address();assert.ok(address&&typeof address!=='string');await new Promise(ok=>socket.close(ok));return address.port;}

for(const host of [undefined,'0.0.0.0'])test(`knowledge server starts with ${host??'default loopback'} binding`,async t=>{
 const root=resolve('.cache/deploy-knowledge-tests');await mkdir(root,{recursive:true});const directory=await mkdtemp(join(root,'index-'));const port=await unusedPort();
 const env={...process.env,SUMA_KNOWLEDGE_TOKEN:'deployment-test-fixture',SUMA_KNOWLEDGE_DIR:directory,SUMA_KNOWLEDGE_PORT:String(port),SUMA_KNOWLEDGE_BACKEND:'lancedb'};
 for(const key of ['SUMA_KNOWLEDGE_HOST','SUMA_KNOWLEDGE_OB_URL','SUMA_EMBEDDING_URL','SUMA_EMBEDDING_API_KEY'])delete env[key];if(host)env.SUMA_KNOWLEDGE_HOST=host;
 const child=spawn(process.execPath,[resolve('packages/knowledge/dist/src/server.js')],{env,stdio:['ignore','ignore','pipe']});let error='';child.stderr.setEncoding('utf8');child.stderr.on('data',chunk=>error+=chunk);
 t.after(async()=>{if(child.exitCode===null){child.kill('SIGTERM');await once(child,'exit');}await rm(directory,{recursive:true,force:true});});
 const origin=`http://${host?'127.0.0.2':'127.0.0.1'}:${port}`;let response;
 for(let attempt=0;attempt<100;attempt++){
  if(child.exitCode!==null)break;
  try{response=await fetch(origin+'/health',{signal:AbortSignal.timeout(200)});if(response.ok)break;}catch{}
  await new Promise(ok=>setTimeout(ok,50));
 }
 assert.equal(response?.status,200,`Knowledge service did not accept connections on expected address: ${error}`);
 const value=await response.json();assert.equal(value.available,true);
});
