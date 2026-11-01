import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {mkdir,mkdtemp,readFile,writeFile,rename} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {createServer} from 'node:http';
import {KnowledgeIndex} from '../src/index.js';
import {sourceKey,type IndexRequest} from '../src/types.js';

const protocol='suma-knowledge-index-v1';
const digest=(x:string)=>createHash('sha256').update(x).digest('hex');
const source=(kind:'documents'|'project_knowledge',id:string,sequence:number,revision:number,content:string)=>({protocol,event_id:`event-${kind}-${id}-${sequence}`,tenant_id:'tenant197',project_id:'project197',source:{kind,id,sequence,revision,sha256:kind==='documents'?digest(content):digest('published:'+id+':'+revision)},action:'upsert' as const,title:'Typed publication',content});
const query=(kind:'documents'|'project_knowledge',text:string,limit=10)=>({protocol,tenant_id:'tenant197',project_id:'project197',source_kind:kind,query:text,limit});
async function directory(name:string){const parent=resolve(process.env.SUMA_KNOWLEDGE_TEST_TMP??'.cache/knowledge-projection-197/projection/native');await mkdir(parent,{recursive:true});return mkdtemp(join(parent,name+'-'));}

test('typed native sources isolate domains and durable tombstones reject old publication replay',async()=>{
 const dir=await directory('typed');let index=await KnowledgeIndex.open({directory:dir});
 try{
  const doc=source('documents','same-id',1,1,'📚 shared keyword document');const knowledge=source('project_knowledge','same-id',2,1,'📚 shared keyword publication');
  const ack=await index.index(doc as any);assert.equal((ack as any).protocol,protocol);assert.deepEqual((ack as any).source,doc.source);
  await index.index(knowledge as any);
  assert.equal((await index.search(query('documents','keyword') as any)).items.length,1);
  assert.equal((await index.search(query('project_knowledge','keyword') as any)).items.length,1);
  await assert.rejects(()=>index.index({...knowledge,content:'changed bytes at same sequence'} as any),/conflict/i);
  const deleted={...knowledge,event_id:'retire-known197',source:{...knowledge.source,sequence:4},action:'delete',title:'',content:''};
  await index.index(deleted as any);const obsolete=await index.index(knowledge as any);assert.equal((obsolete as any).disposition,'obsolete');
  assert.equal((await index.search(query('project_knowledge','keyword') as any)).items.length,0);
  assert.equal((await index.search(query('documents','keyword') as any)).items.length,1);
  await index.close();index=await KnowledgeIndex.open({directory:dir});
  await index.index(knowledge as any);assert.equal((await index.search(query('project_knowledge','keyword') as any)).items.length,0);
  const unpublished={...knowledge,event_id:'retire-draft197',source:{kind:'project_knowledge',id:'draft-only',sequence:2,revision:0,sha256:''},action:'delete',title:'',content:''};
  await index.index(unpublished as any);
 }finally{await index.close()}
});

test('native hybrid uses lexical and vector signals and bounded rebuild resumes without resetting approval',async()=>{
 let calls=0;let hold:Promise<void>|undefined;let entered:(()=>void)|undefined;
 const server=createServer(async(req,res)=>{let raw='';for await(const chunk of req)raw+=chunk;const body=JSON.parse(raw) as {input:string[]};calls++;entered?.();if(hold)await hold;res.setHeader('Content-Type','application/json');res.end(JSON.stringify({data:body.input.map((text,index)=>({index,embedding:text.includes('lexical')?[1,0]:[0,1]}))}));});
 await new Promise<void>(done=>server.listen(0,'127.0.0.1',done));const address=server.address();if(!address||typeof address==='string')throw Error('fixture listener');
 const dir=await directory('hybrid');const embedding={url:`http://127.0.0.1:${address.port}`,model:'owned-fixture',dimensions:2,version:'fixture-v1'};
 let index=await KnowledgeIndex.open({directory:dir,embedding,maxQueuedOperations:2});
 try{
  await index.index(source('project_knowledge','lexical',1,1,'rocket lexical knowledge') as any);
  await index.index(source('project_knowledge','semantic',1,1,'ocean semantic knowledge') as any);
  const result=await index.search(query('project_knowledge','rocket',2) as any);
  assert.equal(result.mode,'lancedb_hybrid');assert.equal(result.items.length,2);
  // RRF ties/native ngram scores do not promise a lexical-first order. Both
  // independently meaningful candidates must survive genuine native fusion.
  assert.ok(result.items.some(hit=>(hit as any).source.id==='lexical'));assert.ok(result.items.some(hit=>(hit as any).source.id==='semantic'));
  let release!:()=>void;hold=new Promise<void>(done=>{release=done});const started=new Promise<void>(done=>{entered=done});const beforeQueue=calls;
  const running=index.search(query('project_knowledge','rocket',2) as any);await started;
  const cancel=new AbortController();const queued=index.search(query('project_knowledge','queued-cancel',2) as any,cancel.signal);cancel.abort();
  await assert.rejects(()=>index.search(query('project_knowledge','queue-overflow',2) as any),/index_busy/);
  release();hold=undefined;entered=undefined;await running;await assert.rejects(()=>queued,/request_cancelled/);assert.equal(calls,beforeQueue+1,'cancelled queued work must not call embedding');
  await index.close();
  // This is an exact durable retry-budget fixture, not 32 invented vendor
  // failures. Preserve the generated vector bytes rather than deleting them.
  const key=sourceKey(source('project_knowledge','lexical',1,1,'rocket lexical knowledge') as IndexRequest),headPath=join(dir,'heads',key.slice(0,2),key+'.json');
  const head=JSON.parse(await readFile(headPath,'utf8')) as {embedding_attempts:number;embedding_profile:string;snapshot:string};head.embedding_attempts=32;await writeFile(headPath,JSON.stringify(head));
  const vectorPath=join(dir,'vectors',head.embedding_profile,head.snapshot.slice(0,2),head.snapshot+'-0.json');await rename(vectorPath,vectorPath+'.withheld-fixture');
  const beforeRecovery=calls;index=await KnowledgeIndex.open({directory:dir,embedding,rebuild:{operationID:'recover-exhausted-source197',maxSources:10,maxEmbeddingRequests:10,maxDurationMS:10000}});await index.ready();assert.equal(calls,beforeRecovery+1);assert.equal((JSON.parse(await readFile(headPath,'utf8')) as {embedding_attempts:number}).embedding_attempts,32,'explicit approval must not reset normal retry history');await index.close();
  const next={...embedding,version:'fixture-v2'};
  const rebuild={operationID:'approved-rebuild197',maxSources:1,maxEmbeddingRequests:1,maxDurationMS:10000};
  index=await KnowledgeIndex.open({directory:dir,embedding:next,rebuild} as any);
  if(typeof(index as any).ready==='function')await assert.rejects(()=>(index as any).ready());
  const blocked=await index.health() as any;assert.equal(blocked.phase,'blocked');assert.equal(blocked.rebuild.scanned_sources,1);
  const beforeRestart=calls;await index.close();index=await KnowledgeIndex.open({directory:dir,embedding:next,rebuild} as any);
  if(typeof(index as any).ready==='function')await assert.rejects(()=>(index as any).ready());
  assert.equal(calls,beforeRestart,'same operation must not reset a consumed rebuild budget');
  await index.close();index=await KnowledgeIndex.open({directory:dir,embedding:next,rebuild:{...rebuild,operationID:'new-explicit-rebuild197',maxSources:10,maxEmbeddingRequests:10}} as any);
  if(typeof(index as any).ready==='function')await(index as any).ready();
  const ready=await index.health() as any;assert.equal(ready.phase,'ready');assert.equal(ready.generation,ready.active_generation);
  assert.equal((await index.search(query('project_knowledge','rocket',2) as any)).items.length,2);
 }finally{await index.close();await new Promise<void>(done=>server.close(()=>done()))}
});
