import {expect,it,vi} from 'vitest';
import {ApiError,type ApiCaller} from './api';
import {ToolWriteRecovery} from './tool-write';

function memory(){const values=new Map<string,string>();return {getItem:(key:string)=>values.get(key)??null,setItem:(key:string,value:string)=>{values.set(key,value);},removeItem:(key:string)=>{values.delete(key);}};}
const body={project_id:'project-a',task_id:'task-a',model_execution_selection:{deployment_id:'model-a',deployment_revision:1,deployment_sha256:'a'.repeat(64),parameters:{max_steps:2,max_output_tokens:100},budget:{token_limit:20000,cost_limit_micros:20000,currency:'CNY'}}};
it('restores the exact non-secret model budget with the original operation after an uncertain acknowledgement',async()=>{
 const calls:unknown[][]=[];const store=memory();let fail=true;
 const send:ApiCaller=async(...args)=>{calls.push(args);if(fail)throw new ApiError(0,'network','unknown');return {id:'original-plan'} as never;};
 const first=new ToolWriteRecovery('model-test',store,send);
 await expect(first.run('/task-dispatch-plans',body)).rejects.toThrow('unknown');
 expect(first.pending?.body).toEqual(body);fail=false;
 const recovered=new ToolWriteRecovery('model-test',store,send);await recovered.restore();
 expect(calls[1]).toEqual(calls[0]);expect(recovered.pending).toBeUndefined();
});
it('still rejects credentials, disguised numeric fields and allowances outside the exact dispatch selection path',async()=>{
 const send=vi.fn(async()=>({id:'never'})) as ApiCaller;
 for(const candidate of [
  {...body,model_execution_selection:{...body.model_execution_selection,api_token:'credential'}},
  {...body,model_execution_selection:{...body.model_execution_selection,parameters:{max_steps:2,max_output_tokens:'credential'}}},
  {...body,model_execution_selection:{...body.model_execution_selection,budget:{token_limit:{value:'credential'}}}},
  {project_id:'project-a',other:{max_output_tokens:100}},
 ])await expect(new ToolWriteRecovery('bad',memory(),send).run('/task-dispatch-plans',candidate)).rejects.toThrow();
 await expect(new ToolWriteRecovery('wrong-path',memory(),send).run('/tool-profiles',body)).rejects.toThrow();
 expect(send).not.toHaveBeenCalled();
});
