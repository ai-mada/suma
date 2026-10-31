import {expect,it} from 'vitest';
import type {ProjectKnowledgeSource} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';
import {deliveryKnowledgeSourceInput,validDeliveryKnowledgeSourceView,validProjectKnowledgeSource,validProjectKnowledgeContextSource} from './delivery-knowledge-model';
import {intelligenceRecovery,intelligenceSource} from './knowledge-intelligence-model';
import {knowledgeSourceInputs} from './knowledge-model';
import {validKnowledgeContext,validKnowledgeRevisionDetail} from './task-conversation-knowledge-model';

const sha='a'.repeat(64),basisSHA='b'.repeat(64);
function source():ProjectKnowledgeSource{return {kind:'accepted_delivery',id:'task-a',version:4,title:'Reviewed task',quote:'  Exact accepted work\n{{literal}}  ',sha256:sha,delivery:{protocol:'suma-accepted-delivery-v1',task:{id:'task-a',version:4,run_id:'run-a',review_id:'review-a',sha256:sha},run_version:8,tests:[{run_id:'run-a',node_id:'test-a',attempt:1,fence:2,worker_id:'independent-worker',result_sha256:sha}],candidates:[],sha256:basisSHA}};}
function recovery(input:unknown):WriteRecoveryRequest{return {path:'/knowledge-intelligence/analyses',method:'POST',idempotency_key:'original-analysis-key',body:{project_id:'project-a',goal:'Extract verified lessons',kind:'skill',locale:'en-US',sources:[input],tool_profile_id:'extractor-a',expected_tool_version:2,max_steps:1,max_output_tokens:2048,acknowledge_model_use:true,idempotency_key:'original-analysis-key'}};}

it('preserves the accepted delivery basis through editable sources and original analysis recovery',()=>{
 const original=source(),input={kind:original.kind,id:original.id,version:original.version,quote:original.quote,basis_sha256:basisSHA};
 expect(knowledgeSourceInputs([original as Parameters<typeof knowledgeSourceInputs>[0][number]])).toEqual([input]);
 expect(intelligenceSource(input)).toBe(true);expect(intelligenceRecovery(recovery(input),'project-a')).toBe(true);
 expect(deliveryKnowledgeSourceInput(original)).toEqual(input);expect(original.delivery?.sha256).toBe(basisSHA);
 for(const bad of [{...input,basis_sha256:undefined},{...input,basis_sha256:'bad'},{...input,delivery:original.delivery},{...input,kind:'tasks'},{...input,extra:'proof'}])expect(intelligenceRecovery(recovery(bad),'project-a')).toBe(false);
});

it('reads exact accepted task provenance without inventing a code candidate',()=>{
 const original=source(),view={project_id:'project-a',source:original};
 expect(validDeliveryKnowledgeSourceView(view,'project-a','task-a')).toBe(true);
 expect(validDeliveryKnowledgeSourceView(view,'other','task-a')).toBe(false);expect(validDeliveryKnowledgeSourceView(view,'project-a','other')).toBe(false);
 expect(validDeliveryKnowledgeSourceView({...view,token:'private'},'project-a','task-a')).toBe(false);
 for(const patch of [{delivery:undefined},{delivery:null},{version:5},{id:'other'},{basis_sha256:basisSHA},{quote:123},{kind:'tasks'}])expect(validDeliveryKnowledgeSourceView({...view,source:{...original,...patch}},'project-a','task-a')).toBe(false);
 expect(validProjectKnowledgeSource(original)).toBe(true);expect(deliveryKnowledgeSourceInput(original)).not.toHaveProperty('delivery');
});

it('rejects inconsistent or unbounded typed delivery evidence and retains actual report and candidate metadata',()=>{
 const original=source(),basis=original.delivery!;
 const report={...basis.tests[0],plan_id:'plan-a',plan_revision:2,plan_sha256:sha,report_id:'report-a',report_sha256:sha,source_commit:'c'.repeat(40)};
 const candidate={kind:'sealed_bundle',run_id:'run-a',node_id:'develop-a',attempt:1,fence:1,worker_id:'worker-a',head_commit:'c'.repeat(40),evidence_sha256:sha,transfer_id:'transfer-a',bundle_sha256:sha};
 expect(validProjectKnowledgeSource({...original,delivery:{...basis,tests:[report],candidates:[candidate]}})).toBe(true);
 const {transfer_id:_,bundle_sha256:__,...workspace}=candidate;
 expect(validProjectKnowledgeSource({...original,delivery:{...basis,candidates:[{...workspace,kind:'workspace_revision'}]}})).toBe(true);
 expect(validProjectKnowledgeSource({...original,delivery:{...basis,candidates:[workspace]}})).toBe(false);
 for(const patch of [{protocol:'other'},{task:{...basis.task,version:3}},{task:{...basis.task,extra:'proof'}},{run_version:0},{tests:null},{candidates:null},{sha256:'BAD'},{tests:[{...report,run_id:'foreign'}]},{tests:[{...report,result_sha256:'bad'}]},{tests:[report,report]},{tests:Array.from({length:257},(_,i)=>({...report,node_id:'test-'+i}))},{candidates:[{...candidate,run_id:'foreign'}]},{candidates:Array.from({length:129},(_,i)=>({...candidate,node_id:'dev-'+i}))},{candidates:[{...candidate,raw_patch:'private'}]}])expect(validProjectKnowledgeSource({...original,delivery:{...basis,...patch}})).toBe(false);
});

it('carries the same typed basis into exact published revision and fixed task context',()=>{
 const original=source(),{quote:_,...contextSource}=original,pin={id:'knowledge-a',revision:2,sha256:sha};
 const context={...pin,kind:'skill',title:'Reviewed lesson',summary:'',content:'Fixed content',sources:[contextSource]};
 const snapshot={revision:2,based_on_revision:1,state:'published',kind:'skill',title:context.title,summary:'',content:context.content,sha256:sha,sources:[original],method:'model_assisted',created_by:'user-a',created_at:'2026-10-31T00:00:00Z'};
 expect(validProjectKnowledgeContextSource(contextSource)).toBe(true);expect(validKnowledgeContext([context])).toBe(true);
 expect(validKnowledgeRevisionDetail({project_id:'project-a',knowledge_id:pin.id,snapshot},'project-a',pin)).toBe(true);
 expect(validKnowledgeContext([{...context,sources:[{...contextSource,delivery:{...original.delivery,task:{...original.delivery!.task,id:'another-task'}}}]}])).toBe(false);
 expect(validKnowledgeRevisionDetail({project_id:'project-a',knowledge_id:pin.id,snapshot:{...snapshot,sources:[{...original,delivery:null}]}},'project-a',pin)).toBe(false);
});

it('keeps ordinary source requests and contexts separate from accepted delivery proofs',()=>{
 const original=source(),ordinary={kind:'documents',id:'doc-a',version:2,title:'Document',quote:'Exact ordinary content',sha256:sha};
 expect(knowledgeSourceInputs([ordinary as Parameters<typeof knowledgeSourceInputs>[0][number]])).toEqual([{kind:'documents',id:'doc-a',version:2,quote:ordinary.quote}]);
 expect(intelligenceSource({kind:'documents',id:'doc-a',version:2})).toBe(true);expect(intelligenceSource({kind:'documents',id:'doc-a',version:2,basis_sha256:''})).toBe(true);
 expect(validProjectKnowledgeSource(ordinary)).toBe(true);expect(validProjectKnowledgeSource({...ordinary,delivery:original.delivery})).toBe(false);
 const context={id:'knowledge-a',revision:1,sha256:sha,kind:'memory',title:'Ordinary',summary:'',content:'Content',sources:[ordinary]};
 expect(validKnowledgeContext([context])).toBe(true);
 expect(validKnowledgeContext([{...context,sources:[{...ordinary,delivery:original.delivery}]}])).toBe(false);
 expect(validKnowledgeContext([{...context,sources:[{...ordinary,basis_sha256:basisSHA}]}])).toBe(false);
});
