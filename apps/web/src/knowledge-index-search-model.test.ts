import {expect,it} from 'vitest';
import {knowledgeIndexSearchPath,readProjectKnowledgeIndexSearch} from './knowledge-index-search-model';
const scope={project:'project-a',query:'原文 %_',kind:'skill' as const,limit:10};
const hit={reference:{id:'knowledge-a',revision:2,sha256:'a'.repeat(64)},project_id:'project-a',kind:'skill',title:'Published skill',summary:'Approved summary',snippet:'😀准确原文',source_count:2,published_at:'2026-11-02T00:00:00Z',source:{kind:'project_knowledge',id:'knowledge-a',sequence:7,revision:2,sha256:'a'.repeat(64)},locator:{unit:'unicode_codepoints',start:5,end:10}};
const page={items:[hit],mode:'lancedb_hybrid',generation:'generation-a',index_watermark:4,candidates_examined:3,discarded:2,scope:'provider_candidates'};
it('T197 reads exact currently authorized index hits without inventing coverage or conflating source sequence with content revision',()=>{
 expect(readProjectKnowledgeIndexSearch(page,scope)).toEqual(page);
 const url=new URL(knowledgeIndexSearchPath(scope),'https://example.test');expect(url.pathname).toBe('/project-knowledge/index-search');expect(Object.fromEntries(url.searchParams)).toEqual({project_id:'project-a',q:scope.query,kind:'skill',limit:'10'});
 for(const mode of ['lancedb_fts','lancedb_hybrid','oceanbase_fts','oceanbase_hybrid'])expect(readProjectKnowledgeIndexSearch({...page,mode},scope).mode).toBe(mode);
 expect(readProjectKnowledgeIndexSearch({...page,items:[],candidates_examined:0,discarded:0},scope).items).toEqual([]);
 for(const patch of [{items:null},{total:4},{next_cursor:'unknown'},{scope:'full_project'},{mode:'semantic'},{generation:''},{generation:'x'.repeat(129)},{index_watermark:-1},{index_watermark:Number.MAX_SAFE_INTEGER+1},{candidates_examined:101},{candidates_examined:0},{discarded:4}])expect(()=>readProjectKnowledgeIndexSearch({...page,...patch},scope)).toThrow();
 for(const patch of [{project_id:'project-b'},{kind:'memory'},{source:{...hit.source,kind:'documents'}},{source:{...hit.source,id:'other'}},{source:{...hit.source,revision:3}},{source:{...hit.source,sha256:'b'.repeat(64)}},{source:{...hit.source,sequence:0}},{source:{...hit.source,extra:true}},{locator:{unit:'utf16',start:5,end:10}},{locator:{unit:'unicode_codepoints',start:10,end:5}},{locator:{unit:'unicode_codepoints',start:0,end:1048577}},{snippet:'中'.repeat(201)},{score:0.9}])expect(()=>readProjectKnowledgeIndexSearch({...page,items:[{...hit,...patch}]},scope)).toThrow();
 for(const query of ['', '  ', '中'.repeat(67), 'unsafe\nquery'])expect(()=>knowledgeIndexSearchPath({...scope,query})).toThrow();
});
