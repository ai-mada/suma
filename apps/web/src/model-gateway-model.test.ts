import {expect,it} from 'vitest';
import {gatewayFacts,validGatewayEvidence} from './model-gateway-model';
import {frozenModelForNode,validModelBinding} from './model-execution-model';
import {modelBinding,gatewayEvidence} from './model-execution.test-support';
it('T118 retains exact known consumption and missing counters without inferring ledger settlement',()=>{
 const evidence=gatewayEvidence(),facts=gatewayFacts(evidence,modelBinding);expect(validModelBinding(modelBinding)).toBe(true);expect(facts).toMatchObject({coverage:'complete',usageState:'complete',completed:1,pending:0,modelChanged:true,totals:{input_tokens:321n,output_tokens:23n}});expect(facts.totals.cached_input_tokens).toBeUndefined();expect(facts).not.toHaveProperty('settlement_state');expect(facts).not.toHaveProperty('held_tokens');
 const unknown={...evidence,requests:[...evidence.requests,{state:'unknown',step:1,request_sha256:'1'.repeat(64),output_allowance:100,usage_state:'unreported'}]};expect(gatewayFacts(unknown,modelBinding)).toMatchObject({coverage:'pending',usageState:'partial',pending:1,totals:{input_tokens:321n,output_tokens:23n}});
 const partial={...evidence,requests:[{...evidence.requests[0],usage_state:'partial',usage:{input_tokens:321}}]};expect(gatewayFacts(partial,modelBinding)).toMatchObject({coverage:'complete',usageState:'partial',totals:{input_tokens:321n}});expect(gatewayFacts(partial,modelBinding).totals.output_tokens).toBeUndefined();
});
it('T118 requires real gateway closure, preserves unknown requests and accepts a genuine zero-call close',()=>{
 const evidence=gatewayEvidence();expect(gatewayFacts({...evidence,gateway_closed:false},modelBinding).coverage).toBe('pending');expect(gatewayFacts({...evidence,requests:[]},modelBinding)).toMatchObject({coverage:'complete',usageState:'complete',completed:0,pending:0,totals:{}});
 expect(gatewayFacts({...evidence,requests:[{state:'rejected',step:0,request_sha256:'e'.repeat(64),output_allowance:100,usage_state:'unreported'}]},modelBinding)).toMatchObject({coverage:'complete',usageState:'unreported'});
 expect(gatewayFacts({...evidence,binding_sha256:'0'.repeat(64)},modelBinding).coverage).toBe('mismatch');for(const patch of [{requests:[...evidence.requests,...evidence.requests]},{gateway_closed:'true'},{requests:[{...evidence.requests[0],usage:{input_tokens:-1}}]}])expect(validGatewayEvidence({...evidence,...patch})).toBe(false);
});
it('T118 reads frozen model basis from the matching published attempt rather than inventory or dirty data',()=>{
 const config={tool_profile_id:'tool-a',tool_profile_sha256:'a'.repeat(64),model_execution:modelBinding},declared={id:'develop',type:'agent',command:['octop-code'],config},actual={id:'develop',type:'agent',command:['octop-code'],attempt:2,fence:4,tool_profile_id:config.tool_profile_id,tool_profile_sha256:config.tool_profile_sha256},definition={id:'flow-a',version:2,nodes:[declared]},run={workflow_id:'flow-a',workflow_version:2,nodes:[actual]};expect(frozenModelForNode(definition,run,actual)).toEqual(modelBinding);expect(frozenModelForNode(definition,{...run,workflow_version:3},actual)).toBeUndefined();expect(frozenModelForNode(definition,run,{...actual,fence:3})).toBeUndefined();
});
