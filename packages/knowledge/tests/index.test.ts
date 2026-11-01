import {test} from 'node:test';
import assert from 'node:assert/strict';
import {mkdir,mkdtemp} from 'node:fs/promises';
import {join,resolve} from 'node:path';
import {KnowledgeIndex} from '../src/index.js';
import {digest,protocol,type IndexRequest,type SearchRequest} from '../src/types.js';
const document=(sequence:number,content:string,tenant_id='tenant'):IndexRequest=>({protocol,event_id:`document-${tenant_id}-${sequence}`,tenant_id,project_id:'project',source:{kind:'documents',id:'doc',sequence,revision:sequence,sha256:digest(content)},action:'upsert',title:'Release protocol',content});
const query=(text:string):SearchRequest=>({protocol,tenant_id:'tenant',project_id:'project',source_kind:'documents',query:text,limit:20});
test('native FTS isolates tenants, replaces shorter sources, and tombstones reject replay',async()=>{
 const parent=resolve(process.env.SUMA_KNOWLEDGE_TEST_TMP??'.cache/knowledge-projection-197/projection/native');await mkdir(parent,{recursive:true});const directory=await mkdtemp(join(parent,'replace-'));let index=await KnowledgeIndex.open({directory});
 try{const base=document(1,'release rollback database migration '.repeat(160));await index.index(base);await index.index(document(1,'private confidential release details','other'));
  assert.ok((await index.search(query('rollback'))).items.length>1);assert.equal((await index.search(query('confidential'))).items.length,0);
  await index.index(document(2,'release summary'));assert.equal((await index.search(query('rollback'))).items.length,0);
  await index.index({...document(3,''),action:'delete',title:'',content:''});await index.index(base);assert.equal((await index.search(query('release'))).items.length,0);
  await index.close();index=await KnowledgeIndex.open({directory});await index.index(base);assert.equal((await index.search(query('release'))).items.length,0);assert.equal((await index.health()).available,true);
 }finally{await index.close()}
});
