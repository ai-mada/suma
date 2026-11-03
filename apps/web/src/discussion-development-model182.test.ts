import {expect,it} from 'vitest';
import type {WriteRecoveryRequest} from './write-action';
import {modelBinding} from './model-execution.test-support';
import {validDiscussionDevelopment,validDiscussionDevelopmentReceipt,validDiscussionDevelopmentRequest} from './discussion-development-model';
import {continuationBinding,continuationSource} from './development-continuation.test-support';

const project='project-a',task='task-a',user='user-a',sha='a'.repeat(64),stamp='2026-11-01T00:00:00Z';
const nativeSelection={deployment_id:modelBinding.deployment.deployment_id,deployment_revision:modelBinding.deployment.revision,deployment_sha256:modelBinding.deployment.deployment_sha256,parameters:modelBinding.parameters,budget:modelBinding.budget};
const codexSelection={catalog_sha256:sha,model:'fixed-model',reasoning_effort:'high',token_notice_limit:1200};
const codexBinding={protocol:'codex-app-server-v1',configuration_sha256:sha,catalog_sha256:sha,model:'fixed-model',reasoning_effort:'high',token_notice_limit:1200,budget_mode:'observe_only',identity_evidence:'configuration_ack_only',binding_sha256:sha};
const pin={id:'skill-one',revision:2,sha256:'b'.repeat(64)};
const knowledge={...pin,kind:'skill',title:'Reviewed skill',summary:'Use the exact reviewed source',content:'Fixed skill content',sources:[]};
const bundle={protocol:'suma-codex-skills-v1',bundle_sha256:'c'.repeat(64),documents:[{...pin,name:'suma-'+'d'.repeat(24),document_sha256:'e'.repeat(64),content_sha256:'f'.repeat(64),title:knowledge.title,description:knowledge.summary,content:knowledge.content}]};
const capabilities={
 opencode_execution:{sdk_version:'1.19.4',server_version:'1.19.4',provider_id:'provider-a',model:'fixed-model',endpoint_origin:'https://provider.example.test',configuration_sha256:sha,budget_mode:'observe_only',identity_evidence:'configuration_ack_only'},
 claude_execution:{model:'claude-sonnet-5',model_source:'administrator_cli_argument',configuration_sha256:sha,budget_mode:'observe_only',identity_evidence:'configuration_ack_only',auth_strategy:'existing_cli_login'},
 acp_execution:{runtime:'qoder',protocol_version:1,sdk_version:'0.13.5',configuration_sha256:sha,model:'fixed-model',model_source:'administrator_session_config',budget_mode:'observe_only',identity_evidence:'configuration_ack_only',auth_strategy:'existing_cli_login',permission_mode:'human_once'},
};
function request(selection:'native'|'codex'|'fixed'='codex'):WriteRecoveryRequest{
 return {path:'/task-discussion-development-plans',method:'POST',idempotency_key:'original-operation',body:{project_id:project,task_id:task,expected_task_version:3,reply_id:'reply-one',expected_reply_version:2,workflow_id:'flow-a',workflow_version:4,agent_node_id:'develop',tool_profile_id:'profile-a',expected_tool_version:5,reviewed_goal:'Implement the reviewed behavior',acceptance_criteria:['Verify the exact candidate'],acknowledge_execution:true,idempotency_key:'original-operation',...(selection==='native'?{model_execution_selection:nativeSelection}:selection==='codex'?{codex_execution_selection:codexSelection}:{})}};
}
function packet(selection:'native'|'codex'|'fixed'='codex',skills=false):Record<string,any>{
 const binding=selection==='native'?{model_execution:structuredClone(modelBinding)}:selection==='codex'?{codex_execution:structuredClone(codexBinding)}:{};
 const skillFacts=skills?{execution_skills:structuredClone(bundle)}:{};
 return {id:'plan-a',project_id:project,task_id:task,version:1,reply_id:'reply-one',reply_version:2,source_sha256:sha,reviewed_goal:'Implement the reviewed behavior',acceptance_criteria:['Verify the exact candidate'],created_by:user,created_at:stamp,
  message:{id:'message-one',sequence:2,task_version:3,body:'Reviewed engineering proposal',author_id:'reply-profile',author_name:'Reply expert',actor_kind:'assistant',created_at:stamp,requirement_id:'',requirement_version:0,run_id:'',reply_id:'reply-one',reply_to_sequence:1,execution_run_id:'reply-run',...(skills?{knowledge:[{...pin,title_at_post:knowledge.title,kind_at_post:knowledge.kind}]}:{})},
  dispatch_plan:{id:'plan-a',project_id:project,task_id:task,version:1,task_version:3,state:'ready',run_id:'reserved-run',workflow_id:'flow-a',workflow_version:4,agent_node_id:'develop',tool_profile_id:'profile-a',tool_profile_version:5,tool_profile_sha256:sha,created_at:stamp,expires_at:'2026-11-01T00:15:00Z',valid:true,reason:'',context:{task:{id:task,version:3,title:'Task',description:'Current formal scope',acceptance_criteria:[]},...(skills?{knowledge:[structuredClone(knowledge)]}:{})},...binding,...skillFacts,
   workflow:{id:'flow-a',version:4,name:'Develop and verify',nodes:[{id:'develop',type:'agent',command:['fixed-command'],max_retries:0,config:{tool_profile_id:'profile-a',tool_profile_sha256:sha,...structuredClone(binding),...structuredClone(skillFacts)}},{id:'verify',type:'test',depends_on:['develop'],config:{candidate_from:'develop'}}],edges:[{from:'develop',to:'verify'}]}}};
}
const accepts=(value:unknown,req=request())=>validDiscussionDevelopmentReceipt(value,req,project,task,user);

it('T223 binds discussion test choices to the exact derived preview and retains malformed receipts for reconciliation',()=>{
 const selected={node_id:'verify',tool_profile_id:'independent-tester',expected_tool_version:3};
 const original=request('fixed'),req={...original,body:{...original.body,test_bindings:[selected]}};
 const value=packet('fixed'),frozen={node_id:'verify',tool_profile_id:selected.tool_profile_id,tool_profile_version:3,tool_profile_sha256:'7'.repeat(64),worker_id:'test-worker',command_id:'verify-command'};
 value.dispatch_plan.test_bindings=[frozen];
 Object.assign(value.dispatch_plan.workflow.nodes[1],{command:[frozen.command_id],config:{candidate_from:'develop',tool_profile_id:frozen.tool_profile_id,tool_profile_sha256:frozen.tool_profile_sha256}});
 expect(validDiscussionDevelopmentRequest(req,project,task)).toBe(true);expect(accepts(value,req)).toBe(true);
 for(const bad of [null,[selected,selected],[{...selected,expected_tool_version:0}],[{...selected,expected_tool_version:3.1}],[{...selected,worker_id:'injected'}]])
  expect(validDiscussionDevelopmentRequest({...req,body:{...req.body,test_bindings:bad}},project,task)).toBe(false);
 for(const mutate of [(v:Record<string,any>)=>delete v.dispatch_plan.test_bindings,(v:Record<string,any>)=>v.dispatch_plan.test_bindings[0].tool_profile_version++,
  (v:Record<string,any>)=>v.dispatch_plan.workflow.nodes[1].command=['other'],(v:Record<string,any>)=>v.dispatch_plan.test_bindings[0].worker_id='']){
  const changed=structuredClone(value);mutate(changed);expect(accepts(changed,req)).toBe(false);
 }
 expect(accepts(value,original)).toBe(false);
});

it('keeps an explicit code continuation identical across the reviewed wrapper, typed plan and original producer marker',()=>{
 const value=packet('codex',true),binding={...structuredClone(continuationBinding),id:value.id,task_version:3,source_task_version:2},original=request();
 const selected={...original,body:{...original.body,continuation_source:structuredClone(continuationSource)}};
 value.development_continuation=binding;value.dispatch_plan.development_continuation=structuredClone(binding);value.dispatch_plan.workflow.nodes[0].config.development_continuation_id=binding.id;
 value.dispatch_plan.context.code_baseline={protocol:'suma.code-baseline.v1',id:'ancestor',project_id:project,task_id:task,task_version:1,composition_id:'accepted-composition',composition_version:1,accepted_basis_sha256:sha,binding_sha256:sha,source:binding.source};
 expect(validDiscussionDevelopmentRequest(selected,project,task)).toBe(true);expect(validDiscussionDevelopment(value,project,task)).toBe(true);expect(accepts(value,selected)).toBe(true);
 expect(validDiscussionDevelopmentRequest({...selected,body:{...selected.body,continuation_source:binding}},project,task)).toBe(false);
 expect(accepts(value,original)).toBe(false);expect(accepts(packet('codex',true),selected)).toBe(false);
 for(const mutate of [(v:Record<string,any>)=>{delete v.development_continuation;},(v:Record<string,any>)=>{v.dispatch_plan.development_continuation.binding_sha256='9'.repeat(64);},(v:Record<string,any>)=>{v.dispatch_plan.workflow.nodes[0].config.development_continuation_id='other-plan';},(v:Record<string,any>)=>{v.dispatch_plan.workflow.nodes[0].config.code_baseline=v.dispatch_plan.context.code_baseline;},(v:Record<string,any>)=>{v.dispatch_plan.workflow.nodes[0].config.development_continuation=binding;},(v:Record<string,any>)=>{v.dispatch_plan.workflow.nodes[0].config.codex_execution.model='other-model';}]){const changed=structuredClone(value);mutate(changed);expect(accepts(changed,selected)).toBe(false);}
 for(const purpose of ['',null]){const changed=structuredClone(value);changed.dispatch_plan.workflow.nodes[0].config.purpose=purpose;expect(accepts(changed,selected)).toBe(false);}
 expect(accepts(value,{...selected,body:{...selected.body,continuation_source:{...continuationSource,attempt:3}}})).toBe(false);
});

it('admits mutually exclusive explicit model choices or fixed tools without granting arbitrary source pins',()=>{
 for(const kind of ['native','codex','fixed'] as const)expect(validDiscussionDevelopmentRequest(request(kind),project,task)).toBe(true);
 const base=request('fixed');
 for(const patch of [{model_execution_selection:nativeSelection,codex_execution_selection:codexSelection},{model_execution_selection:null},{codex_execution_selection:null},{model_execution_selection:undefined},{codex_execution_selection:undefined},{codex_execution_selection:{...codexSelection,provider:'invented'}},{knowledge:[pin]},{codex_execution:{...codexBinding}}])expect(validDiscussionDevelopmentRequest({...base,body:{...base.body,...patch}},project,task)).toBe(false);
});

it('matches Codex selection and materialized skills only after the assistant source pins match the frozen context',()=>{
 const value=packet('codex',true);expect(validDiscussionDevelopment(value,project,task)).toBe(true);expect(accepts(value)).toBe(true);
 const mutations:[string,(value:Record<string,any>)=>void][]=[
  ['source pin',v=>{v.message.knowledge[0].revision=3;}],
  ['context pin',v=>{v.dispatch_plan.context.knowledge[0].sha256='0'.repeat(64);}],
  ['native document',v=>{v.dispatch_plan.execution_skills.documents[0].content='Changed content';}],
  ['producer document',v=>{v.dispatch_plan.workflow.nodes[0].config.execution_skills.documents[0].revision=3;}],
  ['missing native files',v=>{delete v.dispatch_plan.execution_skills;delete v.dispatch_plan.workflow.nodes[0].config.execution_skills;}],
  ['producer binding',v=>{v.dispatch_plan.workflow.nodes[0].config.codex_execution.model='different-model';}],
 ];
 for(const [label,mutate] of mutations){const changed=structuredClone(value);mutate(changed);expect(accepts(changed),label).toBe(false);}
 expect(accepts(value,{...request(),body:{...request().body,codex_execution_selection:{...codexSelection,reasoning_effort:'low'}}})).toBe(false);
});

it('preserves native and fixed-tool handoffs through the original exact producer capabilities',()=>{
 expect(accepts(packet('native'),request('native'))).toBe(true);
 const fixed=packet('fixed');expect(accepts(fixed,request('fixed'))).toBe(true);
 for(const [key,capability] of Object.entries(capabilities)){
  const value=structuredClone(fixed);value.dispatch_plan[key]=structuredClone(capability);value.dispatch_plan.workflow.nodes[0].config[key]=structuredClone(capability);
  expect(accepts(value,request('fixed')),key).toBe(true);
  value.dispatch_plan.workflow.nodes[0].config[key].configuration_sha256='0'.repeat(64);expect(accepts(value,request('fixed')),key+' drift').toBe(false);
 }
});

it('rejects substituted plan scope, forged or mixed bindings and duplicate producers before acknowledging the original operation',()=>{
 const valid=packet();expect(accepts(valid)).toBe(true);
 const mutations:[string,(value:Record<string,any>)=>void][]=[
  ['tool',v=>{v.dispatch_plan.tool_profile_id='other-tool';v.dispatch_plan.workflow.nodes[0].config.tool_profile_id='other-tool';}],
  ['workflow',v=>{v.dispatch_plan.workflow_id='other-flow';}],
  ['tool version',v=>{v.dispatch_plan.tool_profile_version=6;}],
  ['task version',v=>{v.dispatch_plan.task_version=4;}],
  ['producer tool',v=>{v.dispatch_plan.workflow.nodes[0].config.tool_profile_sha256='0'.repeat(64);}],
  ['duplicate producer',v=>{v.dispatch_plan.workflow.nodes.push(structuredClone(v.dispatch_plan.workflow.nodes[0]));}],
  ['missing binding',v=>{delete v.dispatch_plan.codex_execution;delete v.dispatch_plan.workflow.nodes[0].config.codex_execution;}],
  ['unpriced hard budget',v=>{v.dispatch_plan.codex_execution.budget_mode='enforce';v.dispatch_plan.workflow.nodes[0].config.codex_execution.budget_mode='enforce';}],
  ['mixed platform',v=>{v.dispatch_plan.model_execution=structuredClone(modelBinding);v.dispatch_plan.workflow.nodes[0].config.model_execution=structuredClone(modelBinding);}],
  ['mixed fixed',v=>{v.dispatch_plan.claude_execution=structuredClone(capabilities.claude_execution);v.dispatch_plan.workflow.nodes[0].config.claude_execution=structuredClone(capabilities.claude_execution);}],
 ];
 for(const [label,mutate] of mutations){const value=structuredClone(valid);mutate(value);expect(accepts(value),label).toBe(false);}
 expect(accepts(packet('fixed'),request())).toBe(false);
 expect(accepts(packet(),request('fixed'))).toBe(false);
 expect(accepts(packet('native'),request('fixed'))).toBe(false);
});
