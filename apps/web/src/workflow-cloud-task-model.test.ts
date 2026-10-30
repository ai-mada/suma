import {expect,it} from 'vitest';
import {frozenCloudTaskNode} from './workflow-cloud-task-model';

const node=()=>({id:'cloud',name:'Fixed cloud task',type:'external_workflow',config:{plugin:'workbuddy-cloud',workflow:'workbuddy-cloud/v1',workbuddy_cloud:{connection_id:'connection-a',grant_generation:1,request:{prompt:'Original approved request'}}}});
const definition=()=>({id:'flow-a',name:'Original flow',version:1,nodes:[node()],edges:[]});
const run=()=>({id:'run-a',workflow_id:'flow-a',workflow_version:1,nodes:[{id:'cloud',name:'WorkBuddy display name',type:'external_workflow',state:'pending',attempt:0}]});

it('identifies only the selected cloud node in the exact immutable workflow version',()=>{
 const value=run();expect(Object.hasOwn(value.nodes[0],'config')).toBe(false);
 expect(frozenCloudTaskNode(definition(),value,value.nodes[0])).toEqual({nodeID:'cloud',name:'Fixed cloud task'});
 expect(frozenCloudTaskNode({...definition(),version:2},value,value.nodes[0])).toBeUndefined();
 expect(frozenCloudTaskNode({...definition(),id:'another-flow'},value,value.nodes[0])).toBeUndefined();
});

it('does not infer cloud support from the node name, Run config or a partial protocol match',()=>{
 const value=run(),declared=node();
 for(const spec of [{...declared,type:'command'},{...declared,config:{...declared.config,plugin:'other-service'}},{...declared,config:{...declared.config,workflow:'other/v1'}},{...declared,config:{}},{...declared,config:{...declared.config,workbuddy_cloud:{...declared.config.workbuddy_cloud,request:{prompt:''}}}}]){
  expect(frozenCloudTaskNode({...definition(),nodes:[spec]},value,{...value.nodes[0],config:declared.config})).toBeUndefined();
 }
});

it('requires an unambiguous selected identity in both the frozen definition and real run',()=>{
 const value=run(),declared=definition();
 expect(frozenCloudTaskNode(declared,value,{id:'other',type:'external_workflow'})).toBeUndefined();
 expect(frozenCloudTaskNode({...declared,nodes:[node(),node()]},value,value.nodes[0])).toBeUndefined();
 expect(frozenCloudTaskNode(declared,{...value,nodes:[]},value.nodes[0])).toBeUndefined();
 expect(frozenCloudTaskNode(declared,{...value,nodes:[value.nodes[0],value.nodes[0]]},value.nodes[0])).toBeUndefined();
});
