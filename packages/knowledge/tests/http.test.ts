import {test} from 'node:test';
import assert from 'node:assert/strict';
import {mkdtemp,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {KnowledgeIndex} from '../src/index.js';
import {createKnowledgeServer} from '../src/server.js';

test('internal indexing requires bearer auth and invalid search cannot query another tenant',async()=>{
 const directory=await mkdtemp(join(tmpdir(),'suma-knowledge-http-'));const index=await KnowledgeIndex.open({directory});const server=createKnowledgeServer(index,'internal-token');await new Promise<void>(r=>server.listen(0,'127.0.0.1',r));const addr=server.address();if(!addr || typeof addr==='string')throw Error('No port');const origin=`http://127.0.0.1:${addr.port}`;
 try{assert.equal((await fetch(origin+'/health')).status,200);assert.equal((await fetch(origin+'/index',{method:'POST',headers:{'Content-Type':'application/json'},body:'{}'})).status,401);const response=await fetch(origin+'/search',{method:'POST',headers:{'Content-Type':'application/json',Authorization:'Bearer internal-token'},body:JSON.stringify({tenant_id:'',project_id:'p',query:'secret'})});assert.equal(response.status,400);const result=await response.json() as {error:{code:string}};assert.equal(result.error.code,'invalid_request');
 }finally{await new Promise<void>(r=>server.close(()=>r()));await index.close();await rm(directory,{recursive:true,force:true})}
});
