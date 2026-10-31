import {createHash} from 'node:crypto';
import {expect,it} from 'vitest';
import type {ExecutionSkillBundle,ExecutionSkillEvidence,ExpertTeamPlan,TaskDispatchPlan} from '../../../packages/contracts/src';
import {validExecutionSkillBundle,validExecutionSkillEvidence,executionSkillEvidenceMatches,validExecutionSkillPlan,frozenExecutionSkillsForNode} from './codex-native-skills-model';
import {validCodexDispatchReceipt} from './codex-execution-model';
import {ToolWriteRecovery} from './tool-write';
import type {ApiCaller} from './api';
import {validManagedTaskDispatchRun} from './task-dispatch-response';
import {validOrchestratedPlan} from './expert-team-plan-model';

const sha=(value:string)=>createHash('sha256').update(value).digest('hex');
function document(id='knowledge-a'){
 const name='suma-'+sha(id).slice(0,24),content='  Reviewed 中文 instruction\n{{literal}}  ',description='Precise approved scope';
 return {id,revision:2,sha256:sha('published '+id),title:'Reviewed skill',name,description,content,content_sha256:sha(content),document_sha256:sha('---\nname: '+JSON.stringify(name)+'\ndescription: '+JSON.stringify(description)+'\n---\n\n'+content+'\n')};
}
function bundle():ExecutionSkillBundle {const value={protocol:'suma-codex-skills-v1',documents:[document()],bundle_sha256:''};value.bundle_sha256=sha(JSON.stringify(value));return value;}
function evidence(original=bundle()):ExecutionSkillEvidence{return {bundle_sha256:original.bundle_sha256,documents:original.documents.map(({id,revision,sha256,name,document_sha256})=>({id,revision,sha256,name,document_sha256})),materialized:true,catalog_acknowledged:false,invocation_requested:false,turn_acknowledged:false,files_verified:true};}
const selection={catalog_sha256:'b'.repeat(64),model:'model-a',reasoning_effort:'high'};
const binding={protocol:'codex-app-server-v1',configuration_sha256:'a'.repeat(64),...selection,budget_mode:'observe_only',identity_evidence:'configuration_ack_only',binding_sha256:'c'.repeat(64)};
function plan(){const original=bundle();return {
 id:'plan-a',project_id:'project-a',version:1,state:'ready',task_id:'task-a',task_version:2,workflow_id:'flow-a',workflow_version:1,agent_node_id:'develop',tool_profile_id:'tool-a',tool_profile_version:3,tool_profile_sha256:'d'.repeat(64),run_id:'reserved-run',created_at:'2026-10-31T00:00:00Z',expires_at:'2026-10-31T01:00:00Z',valid:true,reason:'',codex_execution:binding,execution_skills:original,
 context:{task:{id:'task-a',version:2,title:'Task',description:'',acceptance_criteria:[]},knowledge:original.documents.map(doc=>({id:doc.id,revision:doc.revision,sha256:doc.sha256,kind:'skill',title:doc.title,summary:doc.description,content:doc.content,sources:[]}))},
 workflow:{id:'flow-a',name:'Fixed workflow',version:1,edges:[],nodes:[{id:'develop',name:'Develop',type:'agent',command:['codex'],depends_on:[],join:'all',quorum:0,timeout_seconds:60,max_retries:0,idempotent:false,config:{codex_execution:binding,tool_profile_id:'tool-a',tool_profile_sha256:'d'.repeat(64),execution_skills:structuredClone(original)}}]}
 } satisfies TaskDispatchPlan;}

it('accepts only bounded closed native skill documents and preserves reviewed text bytes',()=>{
 const original=bundle();expect(validExecutionSkillBundle(original)).toBe(true);expect(original.documents[0].content).toBe('  Reviewed 中文 instruction\n{{literal}}  ');
 for(const bad of [{...original,documents:[]},{...original,documents:null},{...original,protocol:'other'},{...original,path:'/home/private'},{...original,documents:[original.documents[0],original.documents[0]]},{...original,documents:Array.from({length:17},(_,i)=>document('k'+i))}])expect(validExecutionSkillBundle(bad)).toBe(false);
 for(const patch of [{revision:0},{revision:Number.MAX_SAFE_INTEGER+1},{name:'../../SKILL.md'},{content:'\ud800'},{content:'x\0y'},{content:' '},{content:'文'.repeat(5334)},{description:'x'.repeat(1001)},{title:'x'.repeat(201)},{sha256:'wrong'},{content_sha256:'wrong'},{document_sha256:'wrong'},{argv:['shell']}])expect(validExecutionSkillBundle({...original,documents:[{...original.documents[0],...patch}]})).toBe(false);
});

it('keeps five evidence facts separate and allows file verification before a turn',()=>{
 const original=evidence();expect(validExecutionSkillEvidence(original)).toBe(true);
 expect(validExecutionSkillEvidence({...original,materialized:false,files_verified:false})).toBe(true);
 expect(validExecutionSkillEvidence({...original,catalog_acknowledged:true,invocation_requested:true,turn_acknowledged:true,failure_code:'post_run_changed'})).toBe(true);
 for(const patch of [{materialized:false},{materialized:false,catalog_acknowledged:true,files_verified:false},{invocation_requested:true},{turn_acknowledged:true},{files_verified:1},{used:true},{failure_code:'private secret\nurl'},{documents:[]},{documents:[original.documents[0],original.documents[0]]},{documents:[{...original.documents[0],content:'private'}]}])expect(validExecutionSkillEvidence({...original,...patch})).toBe(false);
});

it('matches original receipt order and exact published and document digests without upgrading evidence',()=>{
 const original=bundle();original.documents.push(document('knowledge-b'));const received=evidence(original);
 expect(executionSkillEvidenceMatches(original,received)).toBe(true);expect(received.turn_acknowledged).toBe(false);
 for(const bad of [{...received,bundle_sha256:'f'.repeat(64)},{...received,documents:[...received.documents].reverse()},{...received,documents:[received.documents[0]]},...['id','revision','sha256','name','document_sha256'].map(field=>({...received,documents:received.documents.map((doc,i)=>i?doc:{...doc,[field]:field==='revision'?3:field==='name'?'suma-'+'e'.repeat(24):field==='id'?'other':'e'.repeat(64)})}))])expect(executionSkillEvidenceMatches(original,bad)).toBe(false);
});

it('binds the native plan to only its protected Codex node and ordered fixed skill context',()=>{
 const original=plan();expect(validExecutionSkillPlan(original)).toBe(true);
 const memory={...original.context.knowledge[0],kind:'memory',id:'memory-a'};expect(validExecutionSkillPlan({...original,context:{knowledge:[memory,...original.context.knowledge]}})).toBe(true);
 const fallback=plan();fallback.context.knowledge[0].summary='';fallback.execution_skills.documents[0].description=fallback.context.knowledge[0].title;fallback.workflow.nodes[0].config.execution_skills=structuredClone(fallback.execution_skills);expect(validExecutionSkillPlan(fallback)).toBe(true);
 for(const mutate of [(p:ReturnType<typeof plan>)=>{p.context.knowledge[0].revision++;},(p:ReturnType<typeof plan>)=>{p.context.knowledge[0].content='Changed';},(p:ReturnType<typeof plan>)=>{p.context.knowledge=[];},(p:ReturnType<typeof plan>)=>{p.workflow.nodes[0].config.execution_skills.bundle_sha256='f'.repeat(64);},(p:ReturnType<typeof plan>)=>{p.workflow.nodes[0].config.execution_skills.documents[0].content='Different';},(p:ReturnType<typeof plan>)=>{p.workflow.nodes[0].config.tool_profile_id='foreign';},(p:ReturnType<typeof plan>)=>{p.workflow.nodes.push({...structuredClone(p.workflow.nodes[0]),id:'other'});},(p:ReturnType<typeof plan>)=>{p.workflow.nodes.push(structuredClone(p.workflow.nodes[0]));}]){const bad=plan();mutate(bad);expect(validExecutionSkillPlan(bad)).toBe(false);}
 expect(validExecutionSkillPlan({...original,execution_skills:null})).toBe(false);
 expect(validExecutionSkillPlan({id:'ordinary-plan',context:{knowledge:[memory]},workflow:{nodes:[{id:'develop',type:'agent',config:{}}]}})).toBe(true);
});

it('retains the original preview key after a skill mismatch until the exact protected plan is recovered',async()=>{
 const original=plan(),body={project_id:'project-a',task_id:'task-a',expected_task_version:2,workflow_id:'flow-a',workflow_version:1,agent_node_id:'develop',tool_profile_id:'tool-a',expected_tool_version:3,codex_execution_selection:selection,knowledge:[{id:original.context.knowledge[0].id,revision:2,sha256:original.context.knowledge[0].sha256}]};
 const wrong=structuredClone(original);wrong.workflow.nodes[0].config.execution_skills.documents[0].document_sha256='f'.repeat(64);
 expect(validCodexDispatchReceipt(original,{...body,knowledge:[{...body.knowledge[0],revision:3}]})).toBe(false);
 expect(validCodexDispatchReceipt(original,{...body,knowledge:undefined})).toBe(false);
 let response=wrong;const calls:unknown[][]=[],map=new Map<string,string>(),storage={getItem:(k:string)=>map.get(k)??null,setItem:(k:string,v:string)=>{map.set(k,v);},removeItem:(k:string)=>{map.delete(k);}};
 const api:ApiCaller=async(...args)=>{calls.push(args);return response as never;};
 const validate=(value:unknown,request:{body:Record<string,unknown>})=>validCodexDispatchReceipt(value,request.body);
 const first=new ToolWriteRecovery('native-skills',storage,api,validate);await expect(first.run('/task-dispatch-plans',body)).rejects.toThrow();expect(first.pending).toBeDefined();
 response=original;const second=new ToolWriteRecovery('native-skills',storage,api,validate);await second.restore();expect(calls[1]).toEqual(calls[0]);expect(second.pending).toBeUndefined();
});

it('selects only the native bundle from the exact immutable workflow and matching attempt authority',()=>{
 const original=plan(),definition={id:'managed-run-a',version:1,nodes:original.workflow.nodes},node={id:'develop',type:'agent',attempt:2,fence:7,tool_profile_id:original.tool_profile_id,tool_profile_sha256:original.tool_profile_sha256,command:['codex']},run={id:'run-a',workflow_id:definition.id,workflow_version:1,nodes:[node]};
 expect(frozenExecutionSkillsForNode(definition,run,node)).toBe(original.workflow.nodes[0].config.execution_skills);
 expect(frozenExecutionSkillsForNode({...definition,version:2},run,node)).toBeUndefined();expect(frozenExecutionSkillsForNode({...definition,id:'another-flow'},run,node)).toBeUndefined();
 for(const patch of [{attempt:3},{fence:8},{tool_profile_id:'another-tool'},{tool_profile_sha256:'f'.repeat(64)},{command:['other']},{type:'test'}])expect(frozenExecutionSkillsForNode(definition,run,{...node,...patch})).toBeUndefined();
 expect(frozenExecutionSkillsForNode(definition,{...run,nodes:[{...node,fence:9}]},node)).toBeUndefined();
 expect(frozenExecutionSkillsForNode({...definition,nodes:[{...definition.nodes[0],config:{...definition.nodes[0].config,execution_skills:null}}]},run,node)).toBeUndefined();
});

it('checks native skill bindings on the original Start plan and each expert team item',()=>{
 const original={...plan(),state:'started',run_id:'reserved-run'},run={id:'reserved-run',project_id:'project-a',workflow_id:'managed-reserved-run',workflow_version:1};
 expect(validManagedTaskDispatchRun(run,original,'project-a')).toBe(true);
 const wrong=structuredClone(original);wrong.workflow.nodes[0].config.execution_skills.documents[0].document_sha256='f'.repeat(64);
 expect(validManagedTaskDispatchRun(run,wrong,'project-a')).toBe(false);
 const team={id:'team-plan-a',project_id:'project-a',team_id:'team-a',team_name:'Team',version:1,revision:1,graph_sha256:'a'.repeat(64),activation_generation:0,state:'ready',configuration_version:1,created_at:'2026-10-31T00:00:00Z',expires_at:'2026-10-31T01:00:00Z',valid:true,blockers:[],can_start:true,allowed_actions:['start'],
  configuration:{project_id:'project-a',team_id:'team-a',team_name:'Team',team_version:1,version:1,state:'published',roles:[],created_at:'2026-10-31T00:00:00Z',updated_at:'2026-10-31T00:00:00Z',valid:true,blockers:[],can_edit:true,can_publish:false,can_retire:true},
  items:[{task_id:'task-a',task_version:2,task_basis_sha256:'a'.repeat(64),source_revision:1,depends_on:[],dependencies:[],state:'ready',blockers:[],role_key:'developer',role_title:'Developer',resource_id:'resource-a',resource_name:'Worker',run_state:'',plan:{...plan(),id:'',run_id:'',context:{...plan().context,task:{id:'task-a',version:2,title:'Task',description:'',acceptance_criteria:[]}}}}],progress:{total:1,waiting:0,ready:1,executing:0,awaiting_review:0,accepted:0,blocked:0,stopped:0}} satisfies ExpertTeamPlan;
 expect(validOrchestratedPlan(team,'project-a')).toBe(true);
 team.items[0].plan.context.knowledge[0].sha256='f'.repeat(64);
 expect(validOrchestratedPlan(team,'project-a')).toBe(false);
});
