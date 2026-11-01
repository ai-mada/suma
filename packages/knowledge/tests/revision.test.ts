import {test} from 'node:test';
import assert from 'node:assert/strict';
import {mkdir,mkdtemp,readFile} from 'node:fs/promises';
import {join,resolve} from 'node:path';
import {KnowledgeIndex} from '../src/index.js';
import {LanceBackend} from '../src/lance.js';
import {digest,protocol,type Backend,type Chunk,type IndexRequest} from '../src/types.js';
test('index revisions reject coercible aliases before changing the catalog or backend',async t=>{
 const parent=resolve(process.env.SUMA_KNOWLEDGE_TEST_TMP??'.cache/knowledge-projection-197/projection/native');await mkdir(parent,{recursive:true});const directory=await mkdtemp(join(parent,'revision-'));let rows:Chunk[]=[],writes=0;
 const backend:Backend={kind:'lancedb',replace:async(_source,items)=>{writes++;rows=items},search:async()=>rows,health:async()=>({available:true,verified:true}),close:async()=>{}};t.mock.method(LanceBackend,'open',async()=>backend as LanceBackend);
 const index=await KnowledgeIndex.open({directory}),source:IndexRequest={protocol,event_id:'event',tenant_id:'tenant',project_id:'project',source:{kind:'documents',id:'source',sequence:1,revision:1,sha256:digest('Content')},title:'Title',content:'Content',action:'upsert'};
 try{const before=await readFile(join(directory,'catalog.json'),'utf8');for(const sequence of ['1',' 1 ',true,null,[],0,-1,1.5,Infinity,2147483648])await assert.rejects(()=>index.index({...source,source:{...source.source,sequence}} as IndexRequest),/Invalid typed source/);
  assert.equal(writes,0);assert.equal(await readFile(join(directory,'catalog.json'),'utf8'),before);assert.equal((await index.index(source)).disposition,'applied');
  const tombstone={...source,event_id:'retire',source:{...source.source,sequence:2,revision:2},action:'delete' as const,title:'',content:''};assert.equal((await index.index(tombstone)).disposition,'applied');assert.equal((await index.index(source)).disposition,'obsolete');assert.equal(writes,2);
 }finally{await index.close()}
});
