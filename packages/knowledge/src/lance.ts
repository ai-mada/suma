import * as lancedb from '@lancedb/lancedb';
import type {Backend,Chunk,IndexRequest,SearchRequest} from './types.js';
import {scope} from './types.js';
export class LanceBackend implements Backend {
 readonly kind='lancedb';private constructor(private db:lancedb.Connection,private table:lancedb.Table,private dimensions?:number){}
 static async open(directory:string,profile:string,dimensions?:number):Promise<LanceBackend>{
 const db=await lancedb.connect(directory);const name=`chunks_${profile}`;const names=await db.tableNames();let table:lancedb.Table;
 if(names.includes(name)){table=await db.openTable(name)}else{const seed:Chunk={id:'__schema__',tenant_id:'',project_id:'',source_id:'',revision:0,title:'',content:'schema placeholder',locator:'',...(dimensions?{vector:Array(dimensions).fill(0)}:{})};table=await db.createTable(name,[seed]);}
 await table.createIndex('content',{config:lancedb.Index.fts({baseTokenizer:'ngram',ngramMinLength:1,ngramMaxLength:2,stem:false,removeStopWords:false}),replace:true});return new LanceBackend(db,table,dimensions);
 }
 async replace(source:IndexRequest,rows:Chunk[]):Promise<void>{await this.table.delete(scope(source));if(rows.length)await this.table.add(rows);await this.table.optimize();}
 async search(request:SearchRequest,vector?:number[]):Promise<Chunk[]>{return await this.table.search(vector??new lancedb.MatchQuery(request.query,'content',{operator:lancedb.Operator.And}),vector?'vector':'fts').where(scope(request)).limit(request.limit??10).toArray() as unknown as Chunk[];}
 async health(){const count=await this.table.countRows();await this.table.search('schema','fts').limit(1).toArray();if(this.dimensions)await this.table.search(Array(this.dimensions).fill(0),'vector').limit(1).toArray();return {available:true,verified:true,backend:this.kind,native_rows:count};}
 async close(){this.table.close();this.db.close();}
}
