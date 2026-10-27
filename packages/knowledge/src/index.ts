import {createHash,randomUUID} from 'node:crypto';
import {mkdir,open,readFile,rename,rm} from 'node:fs/promises';
import {join} from 'node:path';
import {LanceBackend} from './lance.js';
import {OceanBaseBackend} from './oceanbase.js';
import {validID,type Backend,type Chunk,type EmbeddingConfig,type IndexOptions,type IndexRequest,type SearchRequest} from './types.js';
interface SourceState {revision:number;action:'upsert'|'delete';phase:'pending'|'active';profile:string}
interface Manifest {format:1;watermark:number;sources:Record<string,SourceState>}
const sourceKey=(r:{tenant_id:string;project_id:string;source_id:string})=>JSON.stringify([r.tenant_id,r.project_id,r.source_id]);
export class KnowledgeIndex {
 private queue:Promise<unknown>=Promise.resolve();private closed=false;private embeddingVerified=false;
 private constructor(private options:IndexOptions,private backend:Backend,private profile:string,private manifest:Manifest,private unlock:()=>Promise<void>){}
 static async open(options:IndexOptions):Promise<KnowledgeIndex>{
 await mkdir(options.directory,{recursive:true});const lock=join(options.directory,'service.lock');let handle;
 try{handle=await open(lock,'wx',0o600)}catch(e){if((e as NodeJS.ErrnoException).code!=='EEXIST')throw e;const pid=Number(await readFile(lock,'utf8'));if(!Number.isInteger(pid) || pid<=0)throw Error('Knowledge lock invalid; operator inspection required');try{process.kill(pid,0);throw Error('Knowledge directory already owned by a process')}catch(check){if((check as NodeJS.ErrnoException).code!=='ESRCH')throw check;}await rm(lock);handle=await open(lock,'wx',0o600)}
 await handle.writeFile(String(process.pid));await handle.sync();const unlock=async()=>{await handle.close();await rm(lock,{force:true})};
 try{const embedding=options.embedding;if(embedding && (!embedding.model || !embedding.version || !Number.isInteger(embedding.dimensions) || embedding.dimensions<1 || embedding.dimensions>4096))throw Error('Embedding model/version and dimensions 1–4096 required');
 const profile=createHash('sha256').update(JSON.stringify({backend:options.backend??'lancedb',model:embedding?.model??'none',dimensions:embedding?.dimensions??0,version:embedding?.version??'fts-ngram-v1',chunker:'chars1500-overlap150-v1'})).digest('hex').slice(0,24);
 const backend=options.backend==='oceanbase'?await OceanBaseBackend.open(options.oceanbaseURL??'',profile,embedding?.dimensions,options.oceanbaseTLS):await LanceBackend.open(join(options.directory,'lance'),profile,embedding?.dimensions);
 let manifest:Manifest={format:1,watermark:0,sources:{}};try{manifest=JSON.parse(await readFile(join(options.directory,'manifest.json'),'utf8'));if(manifest.format!==1 || typeof manifest.sources!=='object' || !manifest.sources || !Number.isSafeInteger(manifest.watermark))throw Error('invalid manifest')}catch(e){if((e as NodeJS.ErrnoException).code!=='ENOENT'){await backend.close();throw e}}
 return new KnowledgeIndex(options,backend,profile,manifest,unlock);
 }catch(e){await unlock();throw e}
 }
 private serialized<T>(fn:()=>Promise<T>):Promise<T>{const next=this.queue.then(()=>{if(this.closed)throw Error('Knowledge index closed');return fn()});this.queue=next.catch(()=>undefined);return next}
 private async persist(){const target=join(this.options.directory,'manifest.json');const temp=target+'.'+randomUUID()+'.tmp';const f=await open(temp,'wx',0o600);try{await f.writeFile(JSON.stringify(this.manifest));await f.sync()}finally{await f.close()};try{await rename(temp,target);if(process.platform!=='win32'){const dir=await open(this.options.directory,'r');try{await dir.sync()}finally{await dir.close()}}}finally{await rm(temp,{force:true})}}
 async index(request:IndexRequest){return this.serialized(async()=>{
 if(!validID(request.tenant_id) || !validID(request.project_id) || !validID(request.source_id))throw Error('Invalid source scope');const revision=request.revision;if(!Number.isSafeInteger(revision) || revision<1 || !['upsert','delete'].includes(request.action))throw Error('Positive integer revision and upsert/delete action required');
 if(request.action==='upsert' && (typeof request.title!=='string' || request.title.length>1024 || typeof request.content!=='string' || Buffer.byteLength(request.content)>1024*1024))throw Error('Title/content invalid or source exceeds 1 MiB');
 const key=sourceKey(request);const previous=this.manifest.sources[key];if(previous && (revision<previous.revision || (revision===previous.revision && previous.action==='delete' && (request.action==='upsert' || previous.phase==='active')) || (revision===previous.revision && previous.phase==='active' && previous.profile===this.profile && request.action===previous.action)))return {applied:false,index_watermark:this.manifest.watermark};
 this.manifest.sources[key]={revision,action:request.action,phase:'pending',profile:this.profile};await this.persist();
 const rows:Chunk[]=[];if(request.action==='upsert'){for(let start=0;start<request.content.length;start+=1350){const content=request.content.slice(start,start+1500);rows.push({id:createHash('sha256').update(key+':'+revision+':'+start).digest('hex'),tenant_id:request.tenant_id,project_id:request.project_id,source_id:request.source_id,revision,title:request.title,content,locator:`chars:${start}-${start+content.length}`})}}
 if(this.options.embedding && rows.length){for(let i=0;i<rows.length;i+=32){const vectors=await embed(rows.slice(i,i+32).map(r=>r.title+'\n'+r.content),this.options.embedding);this.embeddingVerified=true;for(let j=0;j<vectors.length;j++)rows[i+j].vector=vectors[j]}}
 await this.backend.replace(request,rows);this.manifest.sources[key]={revision,action:request.action,phase:'active',profile:this.profile};this.manifest.watermark++;await this.persist();return {applied:true,index_watermark:this.manifest.watermark};
 })}
 async search(request:SearchRequest){return this.serialized(async()=>{if(!validID(request.tenant_id) || !validID(request.project_id) || typeof request.query!=='string' || !request.query.trim() || request.query.length>4096)throw Error('Invalid search scope or query');const limit=Number.isInteger(request.limit)?Math.max(1,Math.min(50,request.limit!)):10;
 const vector=this.options.embedding?(await embed([request.query],this.options.embedding))[0]:undefined;if(vector)this.embeddingVerified=true;const rows=await this.backend.search({...request,limit:Math.min(500,limit*10)},vector);
 const items=rows.filter(r=>{const s=this.manifest.sources[sourceKey(r)];return s?.action==='upsert' && s.phase==='active' && s.profile===this.profile && s.revision===Number(r.revision)}).slice(0,limit).map(r=>({id:r.id,title:r.title,snippet:r.content.slice(0,600),source_ref:r.source_id,revision:Number(r.revision),locator:r.locator}));return {items,mode:`${this.backend.kind}_${vector?'vector':'fts'}`,index_watermark:this.manifest.watermark};
 })}
 async health(){return this.serialized(async()=>({...await this.backend.health(),profile:this.profile,mode:`${this.backend.kind}_${this.options.embedding?'vector':'fts'}`,index_watermark:this.manifest.watermark,embedding_configured:Boolean(this.options.embedding),embedding_verified:this.embeddingVerified}))}
 async close(){await this.queue;if(this.closed)return;this.closed=true;await this.backend.close();await this.unlock()}
}
async function embed(input:string[],config:EmbeddingConfig):Promise<number[][]>{
 const url=new URL(config.url);if(url.username || url.password || (url.protocol!=='https:' && !(url.protocol==='http:' && ['localhost','127.0.0.1','[::1]'].includes(url.hostname))))throw Error('Embedding endpoint must use HTTPS or loopback');
 const r=await fetch(url,{method:'POST',headers:{'Content-Type':'application/json',...(config.apiKey?{Authorization:`Bearer ${config.apiKey}`}:{})},body:JSON.stringify({model:config.model,input,dimensions:config.dimensions}),signal:AbortSignal.timeout(30000),redirect:'error'});if(!r.ok)throw Error(`Embedding endpoint HTTP ${r.status}`);const reader=r.body?.getReader();if(!reader)throw Error('Embedding response missing');let bytes=0;const chunks:Uint8Array[]=[];try{for(;;){const part=await reader.read();if(part.done)break;bytes+=part.value.length;if(bytes>8*1024*1024){await reader.cancel();throw Error('Embedding response too large')};chunks.push(part.value)}}finally{reader.releaseLock()};const response=JSON.parse(Buffer.concat(chunks).toString('utf8')) as {data?:{index:number;embedding:number[]}[]};if(!Array.isArray(response.data) || response.data.length!==input.length)throw Error('Invalid embedding response');const sorted=response.data.sort((a,b)=>a.index-b.index);return sorted.map((row,i)=>{if(row.index!==i || !Array.isArray(row.embedding) || row.embedding.length!==config.dimensions || row.embedding.some(x=>typeof x!=='number' || !Number.isFinite(x)))throw Error('Embedding dimensions/model profile mismatch');return row.embedding})
}
