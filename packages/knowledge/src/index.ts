import {mkdir,open,readFile,rm} from 'node:fs/promises';
import {join} from 'node:path';
import {Catalog,durable,readJSON,type Head,type Rebuild} from './catalog.js';
import {LanceBackend} from './lance.js';
import {OceanBaseBackend} from './oceanbase.js';
import {canonical,digest,IndexError,protocol,rowSource,sourceKey,validID,validateIndex,validateSearch,type Backend,type Chunk,type EmbeddingConfig,type IndexOptions,type IndexRequest,type SearchRequest} from './types.js';
import type {KnowledgeIndexReceipt,KnowledgeIndexSearchResult} from './contract.js';

export class KnowledgeIndex {
 private queue:Promise<unknown>=Promise.resolve();private queued=0;private closed=false;private closing=false;private embeddingVerified=false;private rebuildTask:Promise<void>=Promise.resolve();
 private constructor(private options:IndexOptions,private catalog:Catalog,private backend:Backend|undefined,private profile:string,private unlock:()=>Promise<void>){}
 static async open(options:IndexOptions):Promise<KnowledgeIndex>{
  if(options.maxQueuedOperations!==undefined&&(!Number.isInteger(options.maxQueuedOperations)||options.maxQueuedOperations<1||options.maxQueuedOperations>4096))throw Error('Queued operation limit must be 1–4096');
  const embedding=options.embedding;if(embedding&&(!validID(embedding.model,256)||!validID(embedding.version,128)||!Number.isInteger(embedding.dimensions)||embedding.dimensions<1||embedding.dimensions>4096))throw Error('Embedding model/version and dimensions 1–4096 required');
  if(embedding)embeddingURL(embedding.url);
  const rebuild=options.rebuild;if(rebuild&&(!validID(rebuild.operationID,128)||!Number.isInteger(rebuild.maxSources)||rebuild.maxSources<1||rebuild.maxSources>10000000||!Number.isInteger(rebuild.maxEmbeddingRequests)||rebuild.maxEmbeddingRequests<0||rebuild.maxEmbeddingRequests>10000000||!Number.isInteger(rebuild.maxDurationMS)||rebuild.maxDurationMS<1||rebuild.maxDurationMS>604800000))throw Error('Explicit bounded rebuild configuration required');
  const profile=digest(JSON.stringify({protocol,backend:options.backend??'lancedb',endpoint:options.backend==='oceanbase'?safeBackendIdentity(options.oceanbaseURL??''):'local',model:embedding?.model??'none',embedding_endpoint:embedding?.url??'',dimensions:embedding?.dimensions??0,version:embedding?.version??'fts-ngram-v1',chunker:'unicode-codepoints-1500-overlap150-v1'}));
  await mkdir(options.directory,{recursive:true});const lock=join(options.directory,'service.lock');let handle;
  try{handle=await open(lock,'wx',0o600)}catch(error){if((error as NodeJS.ErrnoException).code!=='EEXIST')throw error;const pid=Number(await readFile(lock,'utf8'));if(!Number.isInteger(pid)||pid<=0)throw Error('Knowledge lock invalid; operator inspection required');try{process.kill(pid,0);throw Error('Knowledge directory already owned by a process')}catch(check){if((check as NodeJS.ErrnoException).code!=='ESRCH')throw check}await rm(lock);handle=await open(lock,'wx',0o600)}
  await handle.writeFile(String(process.pid));await handle.sync();const unlock=async()=>{await handle.close();await rm(lock,{force:true})};
  try{
   const catalog=await Catalog.open(options.directory,profile);const state=catalog.state;
   const sameOperation=rebuild&&state.rebuild?.operation_id===rebuild.operationID;
   if(sameOperation&&(state.processing_profile!==profile||state.rebuild!.max_sources!==rebuild.maxSources||state.rebuild!.max_embedding_requests!==rebuild.maxEmbeddingRequests))throw Error('Existing rebuild approval cannot be changed in place');
   if(rebuild&&!sameOperation){state.processing_profile=profile;state.generation=digest(profile+':'+rebuild.operationID);state.phase='rebuilding';state.rebuild={operation_id:rebuild.operationID,scanned_sources:0,replaced_sources:0,skipped_sources:0,embedding_requests:0,max_sources:rebuild.maxSources,max_embedding_requests:rebuild.maxEmbeddingRequests,deadline_at:new Date(Date.now()+rebuild.maxDurationMS).toISOString(),reason_code:'',cursor:''};await catalog.save()}
   else if(state.processing_profile!==profile){state.phase='rebuild_required';await catalog.save()}
   let backend:Backend|undefined;if(state.phase!=='rebuild_required')backend=await openBackend(options,state.generation);
   const index=new KnowledgeIndex(options,catalog,backend,profile,unlock);
   if(state.phase==='rebuilding'){index.rebuildTask=index.serialized(()=>index.rebuild()).catch(()=>undefined)}
   return index;
  }catch(error){await unlock();throw error}
 }
 private serialized<T>(fn:()=>Promise<T>,signal?:AbortSignal):Promise<T>{
  if(this.queued>=(this.options.maxQueuedOperations??64))return Promise.reject(new IndexError('index_busy',503));
  this.queued++;const next=this.queue.then(()=>{checkCancelled(signal);if(this.closed)throw Error('Knowledge index closed');return fn()});const settled=next.finally(()=>{this.queued--});this.queue=settled.catch(()=>undefined);return settled;
 }
 private requireReady(){const phase=this.catalog.state.phase;if(phase!=='ready')throw new IndexError('index_'+phase,503);if(this.closing)throw new IndexError('index_unavailable',503)}
 async ready():Promise<void>{await this.rebuildTask;this.requireReady()}
 private async rebuild(){
  const state=this.catalog.state,rebuild=state.rebuild!;
  try{for await(const head of this.catalog.heads(rebuild.cursor)){
   if(this.closing)throw new IndexError('index_blocked',503,'closing');
   this.checkRebuild(rebuild);if(rebuild.scanned_sources>=rebuild.max_sources)throw new IndexError('index_blocked',503,'source_budget_exhausted');
   const request=await this.catalog.request(head);
   await this.project(head,request,rebuild);rebuild.scanned_sources++;rebuild.replaced_sources++;rebuild.cursor=head.key;await this.catalog.save();
  }
  // Completion means every durable current head has reached this generation.
  if(rebuild.scanned_sources!==state.source_count||state.pending_count!==0)throw new IndexError('index_blocked',503,'source_catalog_incomplete');
  state.phase='ready';state.active_generation=state.generation;rebuild.reason_code='';await this.catalog.save();
  }catch(error){state.phase='blocked';rebuild.reason_code=error instanceof IndexError?error.message:'native_operation_failed';await this.catalog.save();throw error}
 }
 private checkRebuild(rebuild:Rebuild){if(Date.now()>=Date.parse(rebuild.deadline_at))throw new IndexError('index_blocked',503,'deadline_exhausted')}
 private receipt(request:IndexRequest,raw:Uint8Array,disposition:KnowledgeIndexReceipt['disposition']):KnowledgeIndexReceipt{return {protocol,event_id:request.event_id,tenant_id:request.tenant_id,project_id:request.project_id,source:{...request.source},action:request.action,disposition,generation:this.catalog.state.generation,index_watermark:this.catalog.state.watermark,request_sha256:digest(raw)}}
 async index(value:IndexRequest,raw?:Uint8Array,signal?:AbortSignal):Promise<KnowledgeIndexReceipt>{
  validateIndex(value);this.requireReady();const request=JSON.parse(canonical(value)) as IndexRequest;const original=raw??Buffer.from(canonical(request));
  return this.serialized(async()=>{this.requireReady();const key=sourceKey(request),previous=await this.catalog.get(key),sequence=request.source.sequence,fingerprint=digest(canonical(request));
   if(previous){
    if(sequence<previous.sequence||(sequence===previous.sequence&&previous.action==='delete'&&request.action==='upsert'))return this.receipt(request,original,'obsolete');
    if(sequence===previous.sequence&&request.action===previous.action){if(previous.snapshot!==fingerprint)throw new IndexError('source_conflict',409,'Source sequence content conflict');if(previous.phase==='active'&&previous.generation===this.catalog.state.generation)return this.receipt(request,original,'duplicate')}
    else if(sequence===previous.sequence&&request.action!=='delete')throw new IndexError('source_conflict',409,'Source sequence action conflict');
   }
   const snapshot=await this.catalog.snapshot(request);const head:Head=previous&&previous.snapshot===snapshot?{...previous}:{key,snapshot,sequence,action:request.action,phase:'pending',generation:this.catalog.state.generation,embedding_profile:this.profile,embedding_attempts:0};head.phase='pending';head.generation=this.catalog.state.generation;
   await this.catalog.put(head,!(previous&&previous.snapshot===snapshot));
   await this.project(head,request,undefined,signal);return this.receipt(request,original,'applied');
  },signal);
 }
 private async project(head:Head,request:IndexRequest,rebuild?:Rebuild,signal?:AbortSignal){
  checkCancelled(signal);
  if(!this.backend)throw Error('Backend not opened');
  if(head.phase!=='pending'||head.generation!==this.catalog.state.generation){head.phase='pending';head.generation=this.catalog.state.generation;await this.catalog.put(head)}
  const rows:Chunk[]=[];
  if(request.action==='upsert'){const points=Array.from(request.content);for(let start=0;start<points.length;start+=1350){const end=Math.min(start+1500,points.length);rows.push({id:digest(head.key+':'+head.sequence+':'+start),tenant_id:request.tenant_id,project_id:request.project_id,source_kind:request.source.kind,source_id:request.source.id,sequence:request.source.sequence,revision:request.source.revision,sha256:request.source.sha256,title:request.title,content:points.slice(start,end).join(''),start,end})}}
  if(this.options.embedding&&rows.length){for(let offset=0;offset<rows.length;offset+=32){
   checkCancelled(signal);if(rebuild)this.checkRebuild(rebuild);const input=rows.slice(offset,offset+32).map(row=>row.title+'\n'+row.content),inputSHA=digest(JSON.stringify(input)),path=this.catalog.vectorPath(this.profile,head.snapshot,offset/32);let vectors:number[][];
   try{const cached=await readJSON<{input_sha256:string;vectors:number[][]}>(path,8<<20);if(cached.input_sha256!==inputSHA)throw Error('Vector cache input mismatch');vectors=validVectors(cached.vectors,input.length,this.options.embedding.dimensions)}catch(error){
    if((error as NodeJS.ErrnoException).code!=='ENOENT')throw error;
    if(rebuild){if(rebuild.embedding_requests>=rebuild.max_embedding_requests)throw new IndexError('index_blocked',503,'embedding_budget_exhausted');rebuild.embedding_requests++;await this.catalog.save()}
    // Normal delivery has a small persistent retry ceiling. An explicitly
    // approved rebuild has its own durable total budget, so it can recover a
    // source which exhausted normal retries without secretly resetting them.
    if(!rebuild){if(head.embedding_profile!==this.profile){head.embedding_profile=this.profile;head.embedding_attempts=0}if(head.embedding_attempts>=32)throw new IndexError('index_unavailable',503,'source_embedding_attempt_budget_exhausted');head.embedding_attempts++;await this.catalog.put(head)}
    const remaining=rebuild?Math.max(1,Date.parse(rebuild.deadline_at)-Date.now()):30000;
    checkCancelled(signal);vectors=await embed(input,this.options.embedding,Math.min(30000,remaining),signal);this.embeddingVerified=true;await durable(path,{input_sha256:inputSHA,vectors});
   }
   for(let j=0;j<vectors.length;j++)rows[offset+j].vector=vectors[j];
  }}
  checkCancelled(signal);if(rebuild)this.checkRebuild(rebuild);await this.backend.replace(request,rows);head.phase='active';head.generation=this.catalog.state.generation;await this.catalog.put(head);
 }
 async search(value:SearchRequest,signal?:AbortSignal):Promise<KnowledgeIndexSearchResult>{
  validateSearch(value);this.requireReady();const request={...value};return this.serialized(async()=>{this.requireReady();
   checkCancelled(signal);const vector=this.options.embedding?(await embed([request.query],this.options.embedding,30000,signal))[0]:undefined;if(vector)this.embeddingVerified=true;
   checkCancelled(signal);const rows=await this.backend!.search({...request,limit:Math.min(100,request.limit*5)},vector);checkCancelled(signal);const items:KnowledgeIndexSearchResult['items']=[],seen=new Set<string>();
   for(const row of rows){if(row.tenant_id!==request.tenant_id||row.project_id!==request.project_id||row.source_kind!==request.source_kind)continue;const key=digest(JSON.stringify([row.tenant_id,row.project_id,row.source_kind,row.source_id])),head=await this.catalog.get(key);if(!head||head.action!=='upsert'||head.phase!=='active'||head.generation!==this.catalog.state.generation||head.sequence!==Number(row.sequence))continue;
    const original=await this.catalog.request(head),source=rowSource(row),start=Number(row.start),end=Number(row.end);if(JSON.stringify(source)!==JSON.stringify(original.source)||!Number.isInteger(start)||!Number.isInteger(end)||start<0||end<=start||end>Array.from(original.content).length)continue;const identity=JSON.stringify([source,start,end]);if(seen.has(identity))continue;seen.add(identity);items.push({source,locator:{unit:'unicode_codepoints',start,end}});if(items.length>=request.limit)break;
   }
   return {protocol,items,mode:`${this.backend!.kind}_${vector?'hybrid':'fts'}`,generation:this.catalog.state.generation,index_watermark:this.catalog.state.watermark};
  },signal);
 }
 async health(){
  // Deliberately outside the long rebuild queue: progress is observable while
  // the native backend is being repopulated; it never pretends to be ready.
  const state=this.catalog.state;let native:{available:boolean;verified:boolean;[key:string]:unknown}={available:false,verified:false};if(state.phase==='ready'&&this.backend)native=await this.backend.health();
  const rebuild=state.rebuild?{...state.rebuild}:undefined;if(rebuild)delete (rebuild as Partial<Rebuild>).cursor;
  return {...native,phase:state.phase,profile:this.profile,generation:state.generation,active_generation:state.active_generation,mode:`${this.options.backend??'lancedb'}_${this.options.embedding?'hybrid':'fts'}`,index_watermark:state.watermark,watermark_kind:'provider_local_projection_position',source_count:state.source_count,pending_source_count:state.pending_count,rebuild,embedding_configured:Boolean(this.options.embedding),embedding_verified:this.embeddingVerified};
 }
 async close(){if(this.closed)return;this.closing=true;await this.queue;this.closed=true;await this.backend?.close();await this.unlock()}
}
async function openBackend(options:IndexOptions,generation:string){return options.backend==='oceanbase'?OceanBaseBackend.open(options.oceanbaseURL??'',generation.slice(0,40),options.embedding?.dimensions,options.oceanbaseTLS):LanceBackend.open(join(options.directory,'lance'),generation.slice(0,40),options.embedding?.dimensions)}
function safeBackendIdentity(uri:string){const url=new URL(uri);url.username='';url.password='';return url.toString()}
function embeddingURL(raw:string){const url=new URL(raw);if(url.username||url.password||url.hash||(url.protocol!=='https:'&&!(url.protocol==='http:'&&['localhost','127.0.0.1','[::1]'].includes(url.hostname))))throw Error('Embedding endpoint must use HTTPS or loopback');return url}
function validVectors(value:unknown,count:number,dimensions:number):number[][]{if(!Array.isArray(value)||value.length!==count||value.some(row=>!Array.isArray(row)||row.length!==dimensions||row.some(x=>typeof x!=='number'||!Number.isFinite(x))))throw Error('Embedding dimensions/model profile mismatch');return value as number[][]}
function checkCancelled(signal?:AbortSignal){if(signal?.aborted)throw new IndexError('request_cancelled',503)}
async function embed(input:string[],config:EmbeddingConfig,timeout=30000,signal?:AbortSignal):Promise<number[][]>{
 checkCancelled(signal);const timeoutSignal=AbortSignal.timeout(timeout);const response=await fetch(embeddingURL(config.url),{method:'POST',headers:{'Content-Type':'application/json',...(config.apiKey?{Authorization:`Bearer ${config.apiKey}`}:{})},body:JSON.stringify({model:config.model,input,dimensions:config.dimensions}),signal:signal?AbortSignal.any([signal,timeoutSignal]):timeoutSignal,redirect:'error'});if(!response.ok)throw Error(`Embedding endpoint HTTP ${response.status}`);const reader=response.body?.getReader();if(!reader)throw Error('Embedding response missing');let bytes=0;const chunks:Uint8Array[]=[];try{for(;;){const part=await reader.read();if(part.done)break;bytes+=part.value.length;if(bytes>8*1024*1024){await reader.cancel();throw Error('Embedding response too large')}chunks.push(part.value)}}finally{reader.releaseLock()}
 const body=JSON.parse(Buffer.concat(chunks).toString('utf8')) as {data?:{index:number;embedding:number[]}[]};if(!Array.isArray(body.data)||body.data.length!==input.length)throw Error('Invalid embedding response');const sorted=body.data.sort((a,b)=>a.index-b.index);if(sorted.some((row,i)=>row.index!==i))throw Error('Invalid embedding response ordering');return validVectors(sorted.map(row=>row.embedding),input.length,config.dimensions);
}
