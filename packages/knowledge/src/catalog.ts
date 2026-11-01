import {mkdir,open,readFile,readdir,rename,rm,stat} from 'node:fs/promises';
import {dirname,join} from 'node:path';
import {randomUUID} from 'node:crypto';
import {canonical,digest,sourceKey,validateIndex,type IndexRequest} from './types.js';

// One small state file and one independently durable head per typed source.
// Full source snapshots are immutable and content addressed; accepting a new
// source never rewrites every other source or all of their original content.
export interface Head {key:string;snapshot:string;sequence:number;action:'upsert'|'delete';phase:'pending'|'active';generation:string;embedding_profile:string;embedding_attempts:number}
export interface Rebuild {operation_id:string;scanned_sources:number;replaced_sources:number;skipped_sources:number;embedding_requests:number;max_sources:number;max_embedding_requests:number;deadline_at:string;reason_code:string;cursor:string}
export interface State {format:2;processing_profile:string;generation:string;active_generation:string;phase:'ready'|'rebuild_required'|'rebuilding'|'blocked';watermark:number;source_count:number;pending_count:number;rebuild?:Rebuild;inflight?:Head}
const hash=/^[a-f0-9]{64}$/;
export async function readJSON<T>(path:string,maximum:number):Promise<T>{const info=await stat(path);if(!info.isFile()||info.size>maximum)throw Error('Invalid bounded index metadata');return JSON.parse(await readFile(path,'utf8')) as T}
export async function durable(path:string,value:unknown):Promise<void>{
 await mkdir(dirname(path),{recursive:true});const temporary=path+'.'+randomUUID()+'.tmp';const file=await open(temporary,'wx',0o600);
 try{await file.writeFile(JSON.stringify(value));await file.sync()}finally{await file.close()}
 try{await rename(temporary,path);if(process.platform!=='win32'){const directory=await open(dirname(path),'r');try{await directory.sync()}finally{await directory.close()}}}finally{await rm(temporary,{force:true})}
}
export class Catalog {
 private constructor(readonly directory:string,readonly state:State){}
 static async open(directory:string,profile:string):Promise<Catalog>{
  // The former untyped format is deliberately not migrated or silently reset.
  try{await stat(join(directory,'manifest.json'));throw Error('Unknown index catalog schema; explicit export/rebuild required')}catch(error){if((error as NodeJS.ErrnoException).code!=='ENOENT')throw error}
  let state:State;try{state=await readJSON<State>(join(directory,'catalog.json'),16384)}catch(error){if((error as NodeJS.ErrnoException).code!=='ENOENT')throw error;state={format:2,processing_profile:profile,generation:profile,active_generation:profile,phase:'ready',watermark:0,source_count:0,pending_count:0};await durable(join(directory,'catalog.json'),state)}
  if(state.format!==2||!hash.test(state.processing_profile)||!hash.test(state.generation)||!hash.test(state.active_generation)||!['ready','rebuild_required','rebuilding','blocked'].includes(state.phase)||![state.watermark,state.source_count,state.pending_count].every(n=>Number.isSafeInteger(n)&&n>=0)||state.pending_count>state.source_count)throw Error('Invalid current index catalog');
  if(state.rebuild){const r=state.rebuild;if(typeof r.operation_id!=='string'||!r.operation_id.trim()||r.operation_id.length>128||!['scanned_sources','replaced_sources','skipped_sources','embedding_requests','max_sources','max_embedding_requests'].every(key=>Number.isSafeInteger(r[key as keyof Rebuild])&&(r[key as keyof Rebuild] as number)>=0)||r.max_sources<1||r.scanned_sources>r.max_sources||r.embedding_requests>r.max_embedding_requests||!Number.isFinite(Date.parse(r.deadline_at))||typeof r.reason_code!=='string'||(r.cursor!==''&&!hash.test(r.cursor)))throw Error('Invalid durable rebuild approval')}
  const catalog=new Catalog(directory,state);if(state.inflight){catalog.validateHead(state.inflight);await durable(catalog.headPath(state.inflight.key),state.inflight);delete state.inflight;await catalog.save()}
  return catalog;
 }
 private headPath(key:string){if(!hash.test(key))throw Error('Invalid index head key');return join(this.directory,'heads',key.slice(0,2),key+'.json')}
 private validateHead(head:Head){if(!head||!hash.test(head.key)||!hash.test(head.snapshot)||!hash.test(head.generation)||!hash.test(head.embedding_profile)||!Number.isInteger(head.sequence)||head.sequence<1||head.sequence>2147483647||!['upsert','delete'].includes(head.action)||!['pending','active'].includes(head.phase)||!Number.isInteger(head.embedding_attempts)||head.embedding_attempts<0||head.embedding_attempts>32)throw Error('Invalid index source head')}
 async get(key:string):Promise<Head|undefined>{try{const head=await readJSON<Head>(this.headPath(key),4096);this.validateHead(head);if(head.key!==key)throw Error('Index head identity mismatch');return head}catch(error){if((error as NodeJS.ErrnoException).code==='ENOENT')return;throw error}}
 async save(){await durable(join(this.directory,'catalog.json'),this.state)}
 async put(head:Head,advance=false){
  if(this.state.inflight)throw Error('Uncommitted index head requires catalog recovery');this.validateHead(head);const prior=await this.get(head.key);if(!prior)this.state.source_count++;
  this.state.pending_count+=(head.phase==='pending'?1:0)-(prior?.phase==='pending'?1:0);
  if(advance){if(this.state.watermark>=Number.MAX_SAFE_INTEGER)throw Error('Index watermark exhausted');this.state.watermark++}
  // Persist the exact intended head together with updated counters first.
  // A crash between the two files is completed idempotently on next open.
  this.state.inflight=head;await this.save();await durable(this.headPath(head.key),head);delete this.state.inflight;await this.save();
 }
 async snapshot(request:IndexRequest):Promise<string>{const raw=canonical(request),key=digest(raw);const path=join(this.directory,'snapshots',key.slice(0,2),key+'.json');try{const existing=await readJSON<IndexRequest>(path,8<<20);if(canonical(existing)!==raw)throw Error('Immutable source digest conflict')}catch(error){if((error as NodeJS.ErrnoException).code!=='ENOENT')throw error;await durable(path,request)}return key}
 async request(head:Head):Promise<IndexRequest>{const request=await readJSON<IndexRequest>(join(this.directory,'snapshots',head.snapshot.slice(0,2),head.snapshot+'.json'),8<<20);validateIndex(request);if(digest(canonical(request))!==head.snapshot||sourceKey(request)!==head.key||request.source.sequence!==head.sequence||request.action!==head.action)throw Error('Immutable source snapshot mismatch');return request}
 async *heads(after=''):AsyncGenerator<Head>{
  // Bound sorting memory to a single one-of-256 shard, with a stable cursor.
  for(let i=0;i<256;i++){const shard=i.toString(16).padStart(2,'0');if(after&&shard<after.slice(0,2))continue;let entries:string[];try{entries=await readdir(join(this.directory,'heads',shard))}catch(error){if((error as NodeJS.ErrnoException).code==='ENOENT')continue;throw error}
   for(const name of entries.filter(n=>/^[a-f0-9]{64}\.json$/.test(n)).sort()){const key=name.slice(0,-5);if(key<=after)continue;const head=await this.get(key);if(!head)throw Error('Source head disappeared during rebuild');yield head}
  }
 }
 vectorPath(profile:string,snapshot:string,batch:number){if(!hash.test(profile)||!hash.test(snapshot)||!Number.isInteger(batch)||batch<0)throw Error('Invalid vector snapshot key');return join(this.directory,'vectors',profile,snapshot.slice(0,2),snapshot+'-'+batch+'.json')}
}
