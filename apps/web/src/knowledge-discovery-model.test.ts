import {expect,it} from 'vitest';
import {knowledgeDiscoveryPath,readProjectKnowledgeDiscovery} from './knowledge-discovery-model';
const hit={reference:{id:'knowledge-a',revision:2,sha256:'a'.repeat(64)},project_id:'project-a',kind:'skill',title:'Approved skill',summary:'A published summary',snippet:'Literal %_ matches this approved content.',matched_field:'content',source_count:2,published_at:'2026-11-02T00:00:00.123456789Z'};
const scope={project:'project-a',query:'%_',kind:'skill' as const,limit:10};
const page={items:[hit],next_cursor:'opaque:next/?=',mode:'keyword',scope:'current_project_publications',scanned:200,scan_limit:200,exhausted:false};
it('T196 reads only bounded exact publication hits and preserves literal keyword scope',()=>{
 expect(readProjectKnowledgeDiscovery(page,scope)).toEqual(page);
 const path=new URL(knowledgeDiscoveryPath({...scope,cursor:page.next_cursor}),'https://example.test');expect(path.pathname).toBe('/project-knowledge/discovery');expect(Object.fromEntries(path.searchParams)).toEqual({project_id:'project-a',q:'%_',kind:'skill',limit:'10',cursor:page.next_cursor});
 expect(readProjectKnowledgeDiscovery({...page,items:[]},scope).exhausted).toBe(false);
 expect(readProjectKnowledgeDiscovery({...page,items:[{...hit,matched_field:'none'}],exhausted:true,next_cursor:''},{...scope,query:'',kind:''})).toBeDefined();
 for(const patch of [{items:null},{mode:'semantic'},{scope:'all_projects'},{total:99},{score:1},{scan_limit:201},{scanned:201},{scanned:0},{exhausted:true},{next_cursor:''},{items:[hit,{...hit,reference:{...hit.reference,revision:3}}]},{next_cursor:'x'.repeat(2049)}])expect(()=>readProjectKnowledgeDiscovery({...page,...patch},scope)).toThrow();
 for(const patch of [{project_id:'project-b'},{kind:'memory'},{reference:{...hit.reference,sha256:'A'.repeat(64)}},{reference:{...hit.reference,revision:0}},{reference:null},{snippet:'中'.repeat(201)},{source_count:9},{published_at:'2026-02-30T00:00:00Z'},{matched_field:'none'},{draft:'private'}])expect(()=>readProjectKnowledgeDiscovery({...page,items:[{...hit,...patch}]},scope)).toThrow();
 for(const patch of [{query:'中'.repeat(67)},{query:'unsafe\nquery'},{limit:0},{kind:'documents' as never},{cursor:'x'.repeat(2049)}])expect(()=>knowledgeDiscoveryPath({...scope,...patch})).toThrow();
});
