import {expect,it} from 'vitest';
import {operationEditableProposal,operationProposal,operationRecovery,operationProfile,operationPreview} from './intent-operations-model';

const proposal={schema_version:1,summary:'Requirement and design',rationale:'Reviewed goal',questions:[],steps:[{key:'req',action:'requirement.create_draft',depends_on:[],requirement:{title:'New requirement',description:'Exact source',priority:'medium',acceptance_criteria:['Works']}},{key:'design',action:'requirement.design.save_draft',depends_on:['req'],design:{requirement_id:'',requirement_step_key:'req',content:{product:'Product',experience:'Experience',technical:'Technical'},tasks:[]}}]};

it('permits a new requirement and dependent design while rejecting undeclared effects, cycles and invented sources',()=>{
 expect(operationProposal(JSON.stringify(proposal),0)).toEqual(proposal);
 expect(operationProposal(JSON.stringify({...proposal,steps:[{...proposal.steps[0],action:'run.dispatch'}]}),0)).toBeUndefined();
 expect(operationProposal(JSON.stringify({...proposal,steps:[{...proposal.steps[0],depends_on:['design']},proposal.steps[1]]}),0)).toBeUndefined();
 expect(operationProposal(JSON.stringify({...proposal,steps:[{key:'knowledge',action:'knowledge.create_draft',depends_on:[],knowledge:{kind:'skill',title:'Knowledge',summary:'',content:'Content',source_indices:[0]}}]}),0)).toBeUndefined();
});

it('preserves incomplete human edits structurally without authorizing their submission',()=>{
 const incomplete={...proposal,steps:[{...proposal.steps[0],requirement:{...proposal.steps[0].requirement,title:''}}]};
 expect(operationEditableProposal(JSON.stringify(incomplete))).toEqual(incomplete);
 expect(operationProposal(JSON.stringify(incomplete),0)).toBeUndefined();
 expect(operationEditableProposal('{"schema_version":1')).toBeUndefined();
});

it('accepts only exact scoped mutation envelopes and binds apply recovery to its original preview',()=>{
 const request={path:'/intelligent-operations/analyses/a1/commands',method:'POST',idempotency_key:'original-key',body:{project_id:'p1',expected_version:3,action:'apply',preview_sha256:'a'.repeat(64),acknowledge_drafts:true,reason:'Reviewed',idempotency_key:'original-key'}};
 expect(operationRecovery(request,'p1')).toBe(true);
 expect(operationRecovery(request,'p2')).toBe(false);
 expect(operationRecovery({...request,path:'/intelligent-operations/analyses/a1/preview'},'p1')).toBe(false);
 expect(operationRecovery({...request,body:{...request.body,preview_sha256:'',proposal}},'p1')).toBe(false);
});

it('admits only an enabled available readonly profile with the declared operation protocol',()=>{
 const profile={state:'enabled',available:true,profile:{adapter:'suma',read_only:true,intelligent_operations:{protocol:'suma-intelligent-operations-v1',model:'fixed-model',endpoint_origin:'https://model.example.test'}}};
 expect(operationProfile(profile as never)).toBe(true);
 expect(operationProfile({...profile,available:false} as never)).toBe(false);
 expect(operationProfile({...profile,profile:{...profile.profile,intelligent_operations:undefined}} as never)).toBe(false);
});

it('accepts a valid dependency graph and its server ordered preview independently of proposal display order',()=>{
 const unsorted={...proposal,steps:[proposal.steps[1],proposal.steps[0]]};
 expect(operationProposal(JSON.stringify(unsorted),0)).toEqual(unsorted);
 const current={id:'a1',project_id:'p1',version:2,source:{sha256:'a'.repeat(64)},proposal:unsorted};
 const preview={id:'a1',project_id:'p1',version:2,source_sha256:'a'.repeat(64),proposal_sha256:'b'.repeat(64),preview_sha256:'c'.repeat(64),policy:'suma-local-drafts-v1',can_apply:true,basis_current:true,questions:[],defaults:[],steps:proposal.steps.map(step=>({key:step.key,action:step.action,title:'Draft',depends_on:step.depends_on,required_permission:'project.write',risk:'draft_only'}))};
 expect(operationPreview(preview,current as never)).toBe(true);
 const duplicate={...proposal,steps:[...proposal.steps,{...proposal.steps[1],key:'design2'}]};
 expect(operationProposal(JSON.stringify(duplicate),0)).toBeUndefined();
});

it('keeps structurally valid incomplete design tasks as drafts without imposing publication acceptance',()=>{
 const incomplete={...proposal,steps:[proposal.steps[0],{...proposal.steps[1],design:{...proposal.steps[1].design,tasks:[{key:'task1',title:'',description:'',priority:'medium',owner_id:'',depends_on:['later_task'],acceptance_criteria:[''],covers:[0]}]}}]};
 expect(operationProposal(JSON.stringify(incomplete),0)).toEqual(incomplete);
});
