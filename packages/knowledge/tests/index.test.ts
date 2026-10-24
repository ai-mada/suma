import {test} from 'node:test';
import assert from 'node:assert/strict';
import {mkdtemp,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {createServer} from 'node:http';
import {KnowledgeIndex} from '../src/index.js';

test('native FTS isolates tenants, replaces shorter sources, and tombstones reject replay',async()=>{
 const dir=await mkdtemp(join(tmpdir(),'suma-knowledge-'));const index=await KnowledgeIndex.open({directory:dir});
 try{
 const base={tenant_id:'tenant',project_id:'project',source_id:'doc',revision:1,title:'Release protocol',content:'release rollback database migration '.repeat(160),action:'upsert' as const};
 await index.index(base);await index.index({...base,tenant_id:'other',source_id:'private',content:'private confidential release details'});
 assert.ok((await index.search({tenant_id:'tenant',project_id:'project',query:'rollback',limit:20})).items.length>1);
 assert.equal((await index.search({tenant_id:'tenant',project_id:'project',query:'confidential'})).items.length,0);
 await index.index({...base,revision:2,content:'release summary'});
 assert.equal((await index.search({tenant_id:'tenant',project_id:'project',query:'rollback'})).items.length,0);
 await index.index({...base,revision:3,action:'delete'});await index.index({...base,revision:2});
 assert.equal((await index.search({tenant_id:'tenant',project_id:'project',query:'release'})).items.length,0);
 await index.close();const reopened=await KnowledgeIndex.open({directory:dir});try{await reopened.index({...base,revision:1});assert.equal((await reopened.search({tenant_id:'tenant',project_id:'project',query:'release'})).items.length,0);assert.equal((await reopened.health()).available,true);}finally{await reopened.close()}
 }finally{await index.close();await rm(dir,{recursive:true,force:true})}
});

test('native vectors use profile-specific dimensions and tombstones survive model switches',async()=>{
 const endpoint=createServer(async(req,res)=>{let raw='';for await(const c of req)raw+=c;const body=JSON.parse(raw) as {input:string[]};res.setHeader('Content-Type','application/json');res.end(JSON.stringify({data:body.input.map((text,index)=>({index,embedding:text.includes('cat')?[1,0]:[0,1]}))}))});await new Promise<void>(r=>endpoint.listen(0,'127.0.0.1',r));const addr=endpoint.address();if(!addr || typeof addr==='string')throw Error('No endpoint');const directory=await mkdtemp(join(tmpdir(),'suma-vector-'));const embedding={url:`http://127.0.0.1:${addr.port}`,model:'fixture-embedding',dimensions:2,version:'v1'};
 let index=await KnowledgeIndex.open({directory,embedding});const cat={tenant_id:'t',project_id:'p',source_id:'cat',revision:1,title:'Cat',content:'cat behavior',action:'upsert' as const};
 try{await index.index(cat);await index.index({...cat,source_id:'dog',title:'Dog',content:'dog behavior'});const result=await index.search({tenant_id:'t',project_id:'p',query:'cat',limit:1});assert.equal(result.mode,'lancedb_vector');assert.equal(result.items[0].source_ref,'cat');const profile=(await index.health()).profile;
 await index.close();index=await KnowledgeIndex.open({directory,embedding:{...embedding,version:'v2'}});assert.notEqual((await index.health()).profile,profile);assert.equal((await index.search({tenant_id:'t',project_id:'p',query:'cat'})).items.length,0);await index.index(cat);assert.equal((await index.search({tenant_id:'t',project_id:'p',query:'cat'})).items[0].source_ref,'cat');await index.index({...cat,revision:2,action:'delete'});
 await index.close();index=await KnowledgeIndex.open({directory,embedding});await index.index(cat);assert.ok(!(await index.search({tenant_id:'t',project_id:'p',query:'cat'})).items.some(x=>x.source_ref==='cat'));
 }finally{await index.close();await new Promise<void>(r=>endpoint.close(()=>r()));await rm(directory,{recursive:true,force:true})}
});
