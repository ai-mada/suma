import {test} from 'node:test';
import assert from 'node:assert/strict';
import {mkdir,mkdtemp,readFile,rm} from 'node:fs/promises';
import {join,relative,resolve} from 'node:path';
import {KnowledgeIndex} from '../src/index.js';
import {LanceBackend} from '../src/lance.js';
import type {Backend,Chunk,IndexRequest} from '../src/types.js';

test('index revisions reject coercible aliases before changing the manifest or backend',async t=>{
 const base=resolve(process.env.SUMA_KNOWLEDGE_TEST_TMP??'.cache/e2e/baseline-100-web/knowledge-fixtures');
 await mkdir(base,{recursive:true});const directory=await mkdtemp(join(base,'index-'));
 let rows:Chunk[]=[],writes=0;
 const backend:Backend={kind:'fixture',replace:async(_source,items)=>{writes++;rows=items},search:async()=>rows,health:async()=>({available:true,verified:true}),close:async()=>{}};
 t.mock.method(LanceBackend,'open',async()=>backend as LanceBackend);
 const index=await KnowledgeIndex.open({directory});
 const source={tenant_id:'tenant',project_id:'project',source_id:'source',revision:1,title:'Title',content:'Content',action:'upsert' as const};
 try{
  for(const revision of ['1',' 1 ',true,null,[],0,-1,1.5,Infinity,Number.MAX_SAFE_INTEGER+1])
   await assert.rejects(()=>index.index({...source,revision} as IndexRequest),/Positive integer revision/);
  assert.equal(writes,0);
  await assert.rejects(()=>readFile(join(directory,'manifest.json')),{code:'ENOENT'});
  assert.deepEqual(await index.index(source),{applied:true,index_watermark:1});
  assert.deepEqual(await index.index({...source,revision:2,action:'delete'}),{applied:true,index_watermark:2});
  assert.deepEqual(await index.index(source),{applied:false,index_watermark:2});
  assert.equal(writes,2);
  assert.equal((await index.search({tenant_id:'tenant',project_id:'project',query:'Content'})).items.length,0);
 }finally{
  await index.close();
  const child=relative(base,resolve(directory));
  if(!child||child.startsWith('..')||child.includes('/')||child.includes('\\')||!child.startsWith('index-'))throw Error('Refusing cleanup outside exact revision fixture');
  await rm(directory,{recursive:true,force:true});
 }
});
