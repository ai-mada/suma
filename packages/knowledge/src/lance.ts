import * as lancedb from '@lancedb/lancedb';
import type {Backend,Chunk,IndexRequest,SearchRequest} from './types.js';
import {scope} from './types.js';
export class LanceBackend implements Backend {
 readonly kind='lancedb';private constructor(private db:lancedb.Connection,private table:lancedb.Table,private dimensions?:number){}
 static async open(directory:string,profile:string,dimensions?:number):Promise<LanceBackend>{
 const db=await lancedb.connect(directory);const name=`chunks_${profile}`;const names=await db.tableNames();let table:lancedb.Table;
 if(names.includes(name)){table=await db.openTable(name)}else{const seed:Chunk={id:'__schema__',tenant_id:'',project_id:'',source_kind:'documents',source_id:'',sequence:0,revision:0,sha256:'',title:'',content:'schema placeholder',start:0,end:1,...(dimensions?{vector:Array(dimensions).fill(0)}:{})};table=await db.createTable(name,[seed]);}
 await table.createIndex('content',{config:lancedb.Index.fts({baseTokenizer:'ngram',ngramMinLength:1,ngramMaxLength:2,stem:false,removeStopWords:false}),replace:true});return new LanceBackend(db,table,dimensions);
 }
 async replace(source:IndexRequest,rows:Chunk[]):Promise<void>{await this.table.delete(scope(source));if(rows.length)await this.table.add(rows);await this.table.optimize();}
 async search(request:SearchRequest,vector?:number[]):Promise<Chunk[]>{
  // Native Lance hybrid performs both lexical and vector retrieval, then RRF.
  // A configured embedding never silently changes this into vector-only recall.
  if(vector){const reranker=await lancedb.rerankers.RRFReranker.create();return await this.table.query().fullTextSearch(request.query).nearestTo(vector).where(scope(request)).rerank(reranker).limit(request.limit).toArray() as unknown as Chunk[]}
  return await this.table.search(new lancedb.MatchQuery(request.query,'content',{operator:lancedb.Operator.And}),'fts').where(scope(request)).limit(request.limit).toArray() as unknown as Chunk[];
 }
 async health(){const count=await this.table.countRows("source_id != ''");await this.table.search('schema','fts').limit(1).toArray();if(this.dimensions)await this.table.search(Array(this.dimensions).fill(0),'vector').limit(1).toArray();return {available:true,verified:true,backend:this.kind,native_rows:count};}
 async close(){this.table.close();this.db.close();}
}
