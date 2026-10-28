import {expect,it} from 'vitest';
import type {RequirementTraceability,TaskDeliveryNodeSummary} from '../../../packages/contracts/src';

async function model(){return import('./requirement-traceability-model');}
function trace():RequirementTraceability{return {project_id:'project-a',requirement_id:'requirement-a',requirement_version:2,requirement_status:'draft',design_version:0,published_revision:0,design_sha256:'',eligibility:'none',criteria:[{index:1,text:'Keep the original criterion',task_keys:[]}],tasks:[]};}
function summary():TaskDeliveryNodeSummary{return {total:1,waiting:0,running:0,succeeded:1,failed:0,skipped:0,reconciling:0,other:0,limit:20,complete:true,items:[{id:'verify',name:'Independent test',type:'test',state:'succeeded',attempt:1}]};}
function published():RequirementTraceability{return {...trace(),design_version:3,published_revision:1,design_sha256:'a'.repeat(64),eligibility:'current',criteria:[{index:1,text:'Keep the original criterion',task_keys:['task-a','task-b']}],tasks:[{key:'task-a',title:'Formal task',covers:[1],task_id:'formal-a',task_version:4,task_status:'accepted',binding_state:'current',run_basis_state:'current',formal_acceptance:true,run:{run_id:'run-a',version:5,state:'succeeded',nodes:summary(),tests:summary()}},{key:'task-b',title:'Unadopted proposal',covers:[1],task_id:'',task_version:0,task_status:'',binding_state:'not_adopted',run_basis_state:'not_dispatched',formal_acceptance:false,run:null}]};}

it('retains unmapped current criteria and rejects a different requirement identity',async()=>{
 const {validRequirementTraceability}=await model(),value=trace();
 expect(validRequirementTraceability(value,'project-a','requirement-a')).toBe(true);
 expect(validRequirementTraceability(value,'project-b','requirement-a')).toBe(false);
 expect(validRequirementTraceability(value,'project-a','requirement-b')).toBe(false);
});

it('rejects incomplete or asymmetric criterion mappings instead of inferring coverage',async()=>{
 const {validRequirementTraceability}=await model(),value=trace();
 value.criteria[0].task_keys.push('missing-task');
 expect(validRequirementTraceability(value,'project-a','requirement-a')).toBe(false);
 const duplicate=trace();duplicate.criteria.push({...duplicate.criteria[0]});
 expect(validRequirementTraceability(duplicate,'project-a','requirement-a')).toBe(false);
});

it('counts each current accepted mapped task and rejects stale acceptance or crossed mappings',async()=>{
 const {validRequirementTraceability,traceCriterionProgress}=await model(),value=published();
 expect(validRequirementTraceability(value,'project-a','requirement-a')).toBe(true);
 expect(traceCriterionProgress(value,value.criteria[0])).toEqual({mapped:2,accepted:1});
 value.tasks[0].binding_state='task_changed';
 expect(traceCriterionProgress(value,value.criteria[0])).toEqual({mapped:2,accepted:0});
 expect(validRequirementTraceability(value,'project-a','requirement-a')).toBe(false);
 const crossed=published();crossed.tasks[0].covers=[];
 expect(validRequirementTraceability(crossed,'project-a','requirement-a')).toBe(false);
 const missing=published();missing.criteria[0].task_keys=[];
 expect(validRequirementTraceability(missing,'project-a','requirement-a')).toBe(false);
});

it('preserves the bounded exact-run test summary and rejects false completeness',async()=>{
 const {validRequirementTraceability}=await model(),value=published(),run=value.tasks[0].run!;
 for(const part of [run.nodes,run.tests]){part.total=21;part.succeeded=21;part.complete=false;part.items=Array.from({length:20},(_,index)=>({id:'node-'+index,name:'Check '+index,type:'test',state:'succeeded',attempt:1}));}
 expect(validRequirementTraceability(value,'project-a','requirement-a')).toBe(true);
 run.tests.complete=true;
 expect(validRequirementTraceability(value,'project-a','requirement-a')).toBe(false);
 const stale=trace();stale.eligibility='requirement_changed';stale.tasks=published().tasks;
 expect(validRequirementTraceability(stale,'project-a','requirement-a')).toBe(false);
});
