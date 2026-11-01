import {test} from 'node:test';
import assert from 'node:assert/strict';
import {mkdir,mkdtemp} from 'node:fs/promises';
import {join,resolve} from 'node:path';
import {KnowledgeIndex} from '../src/index.js';
import {createKnowledgeServer} from '../src/server.js';
import {digest,protocol} from '../src/types.js';

test('internal indexing requires bearer auth and invalid search cannot query another tenant',async()=>{
 const parent=resolve(process.env.SUMA_KNOWLEDGE_TEST_TMP??'.cache/knowledge-projection-197/projection/native');await mkdir(parent,{recursive:true});const directory=await mkdtemp(join(parent,'http-'));const index=await KnowledgeIndex.open({directory});const server=createKnowledgeServer(index,'internal-token');await new Promise<void>(r=>server.listen(0,'127.0.0.1',r));const addr=server.address();if(!addr || typeof addr==='string')throw Error('No port');const origin=`http://127.0.0.1:${addr.port}`;
 try{const publicHealth=await fetch(origin+'/health');assert.equal(publicHealth.status,200);assert.equal(Object.hasOwn(await publicHealth.json() as object,'source_count'),false);assert.equal((await fetch(origin+'/index',{method:'POST',headers:{'Content-Type':'application/json'},body:'{}'})).status,401);const response=await fetch(origin+'/search',{method:'POST',headers:{'Content-Type':'application/json',Authorization:'Bearer internal-token'},body:JSON.stringify({tenant_id:'',project_id:'p',query:'secret'})});assert.equal(response.status,400);const result=await response.json() as {error:{code:string}};assert.equal(result.error.code,'invalid_request');
 const source={protocol,event_id:'http-original',tenant_id:'t',project_id:'p',source:{kind:'documents',id:'doc',sequence:1,revision:1,sha256:digest('📚 original')},action:'upsert',title:'Title',content:'📚 original'};
 const raw=JSON.stringify(source,null,2)+'\n',indexed=await fetch(origin+'/index',{method:'POST',headers:{'Content-Type':'application/json',Authorization:'Bearer internal-token'},body:raw});assert.equal(indexed.status,200);const ack=await indexed.json() as {request_sha256:string;source:unknown};assert.equal(ack.request_sha256,digest(raw));assert.deepEqual(ack.source,source.source);
 }finally{await new Promise<void>(r=>server.close(()=>r()));await index.close()}
});
