import {expect,it} from 'vitest';
import {octopHarnessConfiguration,octopRunFacts,validOctopCapability,type OctopHarnessInputs} from './octop-execution-model';
import {runtimeCapability,octopEvidence} from './model-execution.test-support';
it('T118 accepts runtime-only capability and a runtime-only administrator configuration',()=>{
 expect(validOctopCapability(runtimeCapability)).toBe(true);for(const patch of [{model:'fixed'},{provider_id:'openai'},{budget_mode:'observe_only'},{acp_sdk_version:'0.10.1'},{api_key:'private'}])expect(validOctopCapability({...runtimeCapability,...patch})).toBe(false);
 const input:OctopHarnessInputs={python:'C:/Python313/python.exe',bundle:'D:/tools/suma/octop-runtime.pyz',manifest:'D:/tools/suma/octop-runtime.manifest.json',checks:'formal-check'};
 const command=octopHarnessConfiguration(input)?.['octop-harness-develop'];expect(command).toMatchObject({adapter:'octop',protocol:'octop-harness-acp-v1',executable:input.python,args:[input.bundle],input:'octop-acp-jsonl',model_execution_protocol:'suma-model-execution-v1',read_only:false,native_workspace:{checks:['formal-check'],runtime_manifest:input.manifest}});for(const key of ['octop','env_from','env'])expect(command).not.toHaveProperty(key);
 for(const patch of [{python:'python'},{bundle:'relative.pyz'},{manifest:'D:/different/runtime.json'},{checks:''},{checks:'check,check'}])expect(octopHarnessConfiguration({...input,...patch})).toBeUndefined();
});
it('T118 correlates ACP with the shared binding and keeps process stop distinct from cancellation',()=>{
 const evidence=octopEvidence(),proof=evidence.octop_execution!;expect(octopRunFacts('succeeded',evidence).phase).toBe('completed');expect(octopRunFacts('reconciling',{...evidence,octop_execution:{...proof,process_stopped:false,cancel_requested:true}}).phase).toBe('unknown');expect(octopRunFacts('cancelled',{...evidence,octop_execution:{...proof,cancel_requested:true}}).phase).toBe('stopped');expect(octopRunFacts('failed',evidence).phase).toBe('failed');expect(octopRunFacts('succeeded',{...evidence,octop_execution:{...proof,binding_sha256:'0'.repeat(64)}}).phase).toBe('mismatch');expect(octopRunFacts('succeeded',{...evidence,model_execution:{...evidence.model_execution!,gateway_closed:false}}).phase).toBe('unknown');expect(octopRunFacts('succeeded',{...evidence,octop_execution:undefined}).phase).toBe('unrecorded');
});
