import {createHash} from 'node:crypto';
import type {KnowledgeIndexRequest,KnowledgeIndexSearchRequest,KnowledgeIndexSource} from './contract.js';
export type IndexRequest=KnowledgeIndexRequest;
export type SearchRequest=KnowledgeIndexSearchRequest;
export type Source=KnowledgeIndexSource;
export interface Chunk extends Record<string,unknown> {id:string;tenant_id:string;project_id:string;source_kind:Source['kind'];source_id:string;sequence:number;revision:number;sha256:string;title:string;content:string;start:number;end:number;vector?:number[]}
export interface Backend {kind:'lancedb'|'oceanbase';replace(source:IndexRequest,rows:Chunk[]):Promise<void>;search(request:SearchRequest,vector?:number[]):Promise<Chunk[]>;health():Promise<{available:boolean;verified:boolean;[key:string]:unknown}>;close():Promise<void>}
export interface EmbeddingConfig {url:string;model:string;dimensions:number;version:string;apiKey?:string}
export interface RebuildOptions {operationID:string;maxSources:number;maxEmbeddingRequests:number;maxDurationMS:number}
export interface IndexOptions {directory:string;backend?:'lancedb'|'oceanbase';oceanbaseURL?:string;oceanbaseTLS?:boolean;embedding?:EmbeddingConfig;rebuild?:RebuildOptions;maxQueuedOperations?:number}
export const protocol='suma-knowledge-index-v1';
export const digest=(value:string|Uint8Array)=>createHash('sha256').update(value).digest('hex');
export class IndexError extends Error {constructor(readonly code:string,readonly status:number,message=code){super(message)}}
export function invalid(message:string):never {throw new IndexError('invalid_request',400,message)}
export function exact(value:unknown,keys:string[]):value is Record<string,unknown>{return value!==null&&typeof value==='object'&&!Array.isArray(value)&&Object.keys(value).length===keys.length&&keys.every(key=>Object.hasOwn(value,key))}
export function text(value:unknown,maximum:number):value is string{return typeof value==='string'&&Buffer.byteLength(value)<=maximum&&!/[\u0000\uD800-\uDFFF]/u.test(value)}
export function validID(value:unknown,maximum=64):value is string{return text(value,maximum)&&value.length>0&&value.trim()===value&&!/\p{Cc}/u.test(value)}
export function sourceValid(value:unknown,action:string):value is Source {
 if(!exact(value,['kind','id','sequence','revision','sha256'])||!['documents','project_knowledge'].includes(String(value.kind))||!validID(value.id)||!Number.isInteger(value.sequence)||(value.sequence as number)<1||(value.sequence as number)>2147483647)return false;
 if(value.kind==='project_knowledge'&&action==='delete'&&value.revision===0&&value.sha256==='')return true;
 return Number.isInteger(value.revision)&&(value.revision as number)>0&&(value.revision as number)<=2147483647&&typeof value.sha256==='string'&&/^[a-f0-9]{64}$/.test(value.sha256)&&(value.kind!=='documents'||value.revision===value.sequence);
}
export function validateIndex(value:unknown):asserts value is IndexRequest {
 if(!exact(value,['protocol','event_id','tenant_id','project_id','source','action','title','content'])||value.protocol!==protocol||!validID(value.event_id)||!validID(value.tenant_id)||!validID(value.project_id)||!['upsert','delete'].includes(String(value.action))||!sourceValid(value.source,String(value.action)))invalid('Invalid typed source');
 if(value.action==='delete'){if(value.title!==''||value.content!=='')invalid('Invalid tombstone content');return}
 if(!text(value.title,4096)||!text(value.content,1<<20))invalid('Invalid bounded title/content');
 if(value.source.kind==='documents'&&value.source.sha256!==digest(value.content))invalid('Invalid document content digest');
}
export function validateSearch(value:unknown):asserts value is SearchRequest {
 if(!exact(value,['protocol','tenant_id','project_id','source_kind','query','limit'])||value.protocol!==protocol||!validID(value.tenant_id)||!validID(value.project_id)||!['documents','project_knowledge'].includes(String(value.source_kind))||!text(value.query,500)||!value.query.trim()||/\p{Cc}/u.test(value.query)||!Number.isInteger(value.limit)||(value.limit as number)<1||(value.limit as number)>100)invalid('Invalid typed search');
}
export function canonical(request:IndexRequest):string{return JSON.stringify({protocol:request.protocol,event_id:request.event_id,tenant_id:request.tenant_id,project_id:request.project_id,source:{kind:request.source.kind,id:request.source.id,sequence:request.source.sequence,revision:request.source.revision,sha256:request.source.sha256},action:request.action,title:request.title,content:request.content})}
export const sourceKey=(r:IndexRequest)=>digest(JSON.stringify([r.tenant_id,r.project_id,r.source.kind,r.source.id]));
export function quoted(s:string):string{return "'"+s.replaceAll("'","''")+"'"}
export function scope(r:SearchRequest|IndexRequest):string{return `tenant_id=${quoted(r.tenant_id)} AND project_id=${quoted(r.project_id)} AND source_kind=${quoted('source_kind' in r?r.source_kind:r.source.kind)}`+('source' in r?` AND source_id=${quoted(r.source.id)}`:'')}
export function rowSource(r:Chunk):Source{return {kind:r.source_kind,id:r.source_id,sequence:Number(r.sequence),revision:Number(r.revision),sha256:r.sha256}}
