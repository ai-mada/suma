export interface IndexRequest {tenant_id:string;project_id:string;source_id:string;revision:number;title:string;content:string;action:'upsert'|'delete'}
export interface SearchRequest {tenant_id:string;project_id:string;query:string;limit?:number}
export interface Chunk extends Record<string,unknown> {id:string;tenant_id:string;project_id:string;source_id:string;revision:number;title:string;content:string;locator:string;vector?:number[]}
export interface SearchItem {id:string;title:string;snippet:string;source_ref:string;revision:number;locator:string}
export interface Backend {kind:string;replace(source:IndexRequest,rows:Chunk[]):Promise<void>;search(request:SearchRequest,vector?:number[]):Promise<Chunk[]>;health():Promise<{available:boolean;verified:boolean;[key:string]:unknown}>;close():Promise<void>}
export interface EmbeddingConfig {url:string;model:string;dimensions:number;version:string;apiKey?:string}
export interface IndexOptions {directory:string;backend?:'lancedb'|'oceanbase';oceanbaseURL?:string;oceanbaseTLS?:boolean;embedding?:EmbeddingConfig}
export function quoted(s:string):string{return "'"+s.replaceAll("'","''")+"'"}
export function scope(r:{tenant_id:string;project_id:string;source_id?:string}):string{return `tenant_id=${quoted(r.tenant_id)} AND project_id=${quoted(r.project_id)}`+(r.source_id!==undefined?` AND source_id=${quoted(r.source_id)}`:'')}
export function validID(x:unknown):x is string{return typeof x==='string' && x.length>0 && x.length<=192 && !/[\x00-\x1f]/.test(x)}
