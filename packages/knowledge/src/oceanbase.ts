import mysql,{type Pool,type RowDataPacket} from 'mysql2/promise';
import type {Backend,Chunk,IndexRequest,SearchRequest} from './types.js';
const columns='id,tenant_id,project_id,source_kind,source_id,sequence,revision,sha256,title,content,start_offset,end_offset';
const row=(value:RowDataPacket)=>({...value,start:Number(value.start_offset),end:Number(value.end_offset)}) as Chunk;
export class OceanBaseBackend implements Backend {
 readonly kind='oceanbase';private constructor(private pool:Pool,private table:string,private dimensions?:number){}
 static async open(uri:string,profile:string,dimensions?:number,tls=false):Promise<OceanBaseBackend>{
  const u=new URL(uri);if(!['localhost','127.0.0.1','[::1]'].includes(u.hostname)&&!tls)throw Error('Remote OceanBase requires TLS');if(dimensions&&(!Number.isInteger(dimensions)||dimensions<1||dimensions>4096))throw Error('OceanBase indexed vector dimensions must be 1–4096');if(!/^[a-f0-9]{40}$/.test(profile))throw Error('Invalid physical generation');
  const pool=mysql.createPool({uri,connectionLimit:4,multipleStatements:false,...(tls?{ssl:{rejectUnauthorized:true}}:{})}),table=`suma_chunks_${profile}`,backend=new OceanBaseBackend(pool,table,dimensions);
  try{const [v]=await pool.query<RowDataPacket[]>('SELECT VERSION() AS version, @@version_comment AS comment');if(!JSON.stringify(v).toLowerCase().includes('oceanbase'))throw Error('Configured server is not verified as OceanBase');
   await pool.query(`CREATE TABLE IF NOT EXISTS ${table} (id VARCHAR(64) PRIMARY KEY, tenant_id VARCHAR(64) NOT NULL, project_id VARCHAR(64) NOT NULL, source_kind VARCHAR(32) NOT NULL, source_id VARCHAR(64) NOT NULL, sequence BIGINT NOT NULL, revision BIGINT NOT NULL, sha256 VARCHAR(64) NOT NULL, title TEXT NOT NULL, content TEXT NOT NULL, start_offset INT NOT NULL, end_offset INT NOT NULL, INDEX scope_idx(tenant_id,project_id,source_kind,source_id), FULLTEXT INDEX content_fts(content) WITH PARSER IK${dimensions?`, vector VECTOR(${dimensions}), VECTOR INDEX embedding_hnsw(vector) WITH (distance=L2,type=hnsw)`:''}) DEFAULT CHARSET=utf8mb4`);await backend.health();return backend;
  }catch(error){await pool.end();throw error}
 }
 async replace(source:IndexRequest,rows:Chunk[]):Promise<void>{const connection=await this.pool.getConnection();try{await connection.beginTransaction();await connection.execute(`DELETE FROM ${this.table} WHERE tenant_id=? AND project_id=? AND source_kind=? AND source_id=?`,[source.tenant_id,source.project_id,source.source.kind,source.source.id]);for(const r of rows){const values:(string|number)[]=[r.id,r.tenant_id,r.project_id,r.source_kind,r.source_id,r.sequence,r.revision,r.sha256,r.title,r.content,r.start,r.end];if(this.dimensions)values.push(JSON.stringify(r.vector));await connection.execute(`INSERT INTO ${this.table} (${columns}${this.dimensions?',vector':''}) VALUES (${values.map(()=>'?').join(',')})`,values)}await connection.commit()}catch(error){await connection.rollback();throw error}finally{connection.release()}}
 async search(request:SearchRequest,vector?:number[]):Promise<Chunk[]>{
  const parameters=[request.tenant_id,request.project_id,request.source_kind],limit=request.limit,filter='tenant_id=? AND project_id=? AND source_kind=?';
  const [lexical]=await this.pool.execute<RowDataPacket[]>(`SELECT ${columns},MATCH(content) AGAINST(?) AS score FROM ${this.table} WHERE ${filter} AND MATCH(content) AGAINST(?)>0 ORDER BY score DESC,id ASC LIMIT ${limit}`,[request.query,...parameters,request.query]);
  if(!vector)return lexical.map(row);
  const [semantic]=await this.pool.execute<RowDataPacket[]>(`SELECT ${columns} FROM ${this.table} WHERE ${filter} ORDER BY l2_distance(vector,?) APPROXIMATE LIMIT ${limit}`,[...parameters,JSON.stringify(vector)]);
  // Both native result sets contribute reciprocal rank; scores from different
  // distance/text domains are never compared directly or relabelled hybrid.
  const merged=new Map<string,{row:Chunk;score:number}>();for(const list of [lexical,semantic])list.forEach((item,index)=>{const prior=merged.get(item.id as string);merged.set(item.id as string,{row:row(item),score:(prior?.score??0)+1/(60+index+1)})});
  return [...merged.values()].sort((a,b)=>b.score-a.score||a.row.id.localeCompare(b.row.id)).slice(0,limit).map(item=>item.row);
 }
 async health(){await this.pool.query(`SELECT COUNT(*) AS n FROM ${this.table}`);await this.pool.query(`SELECT id FROM ${this.table} WHERE MATCH(content) AGAINST('suma-probe')>0 LIMIT 1`);if(this.dimensions)await this.pool.query(`SELECT l2_distance(vector,'${JSON.stringify(Array(this.dimensions).fill(0))}') FROM ${this.table} LIMIT 1`);return {available:true,verified:true,backend:this.kind}}
 async close(){await this.pool.end()}
}
