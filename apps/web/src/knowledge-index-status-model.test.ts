import {expect,it} from 'vitest';
import {knowledgeIndexStatusPath,readKnowledgeIndexStatus} from './knowledge-index-status-model';

const source={kind:'project_knowledge',id:'knowledge-a',sequence:7,revision:3,sha256:'a'.repeat(64)};
const acknowledgement={protocol:'suma-knowledge-index-v1',event_id:'event-older',tenant_id:'org-a',project_id:'project-a',source,action:'upsert',disposition:'obsolete',generation:'provider-generation-a',index_watermark:4,request_sha256:'b'.repeat(64)};
const status={domain:'project_knowledge',as_of:'2026-11-02T00:00:00Z',configured:false,coverage:'outbox_delivery_only',pending:2,indexed:1,failed:1,total:4,current_publications:2,untracked_publications:1,latest_event:{event_id:'event-newer',source:{...source,sequence:9},action:'delete',state:'pending',created_at:'2026-11-02T00:00:00Z'},last_ack:acknowledgement};
it('T197 reads closed scoped delivery facts without equating different events, watermarks or an unpublished tombstone with coverage',()=>{
 const scope={project:'project-a',organizationID:'org-a'};
 expect(knowledgeIndexStatusPath(scope.project)).toBe('/project-knowledge/index-status?project_id=project-a');expect(readKnowledgeIndexStatus(status,scope)).toEqual(status);
 const empty={...status,pending:0,indexed:0,failed:0,total:0,current_publications:0,untracked_publications:0,configured:true,latest_event:null,last_ack:null};expect(readKnowledgeIndexStatus(empty,scope)).toEqual(empty);
 const tombstone={...source,sequence:10,revision:0,sha256:''};const deleted={...status,latest_event:{...status.latest_event,source:tombstone},last_ack:{...acknowledgement,source:tombstone,action:'delete'}};expect(readKnowledgeIndexStatus(deleted,scope)).toEqual(deleted);
 const {last_ack:_,...missing}=status;
 for(const invalid of [missing,{...status,content:'private source body'},{...status,last_ack:{...acknowledgement,project_id:'another-project'}},{...status,last_ack:{...acknowledgement,source:{...source,kind:'documents'}}},{...status,last_ack:{...acknowledgement,request_sha256:'unverified'}},{...deleted,last_ack:{...deleted.last_ack,action:'upsert'}},{...status,pending:-1},{...status,latest_event:{...status.latest_event,state:'healthy'}}])expect(()=>readKnowledgeIndexStatus(invalid,scope)).toThrow();
});
