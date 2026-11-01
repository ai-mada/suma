import {expect,it} from 'vitest';
import type {ToolProfile} from '../../../packages/contracts/src';
import {buildToolReinspectionIntent,inspectionMatchesTool,readToolInspection,readToolInspections,toolReinspectionRecoveryScope,validToolInspectionReceipt,validToolReinspectionIntent} from './tool-reinspection-model';

const scope={project:'p',toolID:'tool-a',userID:'user-a',organizationID:'org-a'};
const tool:ToolProfile={id:'tool-a',project_id:'p',worker_id:'worker-a',name:'Team Codex',version:3,state:'paused',profile:{command_id:'codex-team',adapter:'codex',protocol:'codex-jsonl-v1',profile_sha256:'a'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'1.0',read_only:false},enabled_by:'',created_at:'2026-11-01T00:00:00Z',updated_at:'2026-11-01T00:00:00Z',available:false,reason:'Profile paused'};
const pending={id:'inspection-a',project_id:'p',worker_id:'worker-a',tool_profile_id:'tool-a',tool_profile_version:3,profile_sha256:'a'.repeat(64),command_id:'codex-team',version:1,fence:0,state:'pending',requested_by:'user-a',created_at:'2026-11-01T00:00:00Z',expires_at:'2026-11-01T00:05:00Z'};
const claimed={...pending,version:2,fence:1,state:'claimed',claim_id:'claim-original',claimed_at:'2026-11-01T00:00:10Z',lease_until:'2026-11-01T00:01:10Z'};

it('T194 reads bounded original inspection sources and inventory completion without inferring time or health',()=>{
 expect(readToolInspection(pending,scope)).toEqual(pending);expect(inspectionMatchesTool(readToolInspection(pending,scope),tool)).toBe(true);
 const completed={...claimed,version:3,state:'completed',observed_at:'2026-11-01T00:00:12Z',inventory_operation_id:'inventory-original',result_code:'reported_unavailable'};
 expect(readToolInspections({items:[{...claimed,id:'inspection-b',state:'unknown',result_code:'probe_timeout'},completed]},scope)).toEqual([{...claimed,id:'inspection-b',state:'unknown',result_code:'probe_timeout'},completed]);
 for(const invalid of [{...pending,project_id:'other'},{...pending,tool_profile_id:'other'},{...pending,token:'private'},{...pending,state:'healthy'},{...claimed,claim_id:undefined},{...completed,inventory_operation_id:''},{...pending,created_at:'2026-02-30T00:00:00Z'},{...claimed,result_code:'probe_timeout'},{...claimed,state:'unknown'},{...pending,state:'claimed'},{...pending,state:'expired'},{...claimed,observed_at:'2026-11-01T00:00:12Z'},{...claimed,lease_until:claimed.claimed_at}])expect(()=>readToolInspection(invalid,scope)).toThrow();
 expect(()=>readToolInspections({items:[pending,pending]},scope)).toThrow();expect(()=>readToolInspections({items:Array.from({length:11},(_,i)=>({...pending,id:'inspection-'+i}))},scope)).toThrow();
 expect(inspectionMatchesTool(readToolInspection({...pending,worker_id:'worker-b'},scope),tool)).toBe(false);
 expect(readToolInspection({...pending,state:'superseded',result_code:'source_changed'},scope).state).toBe('superseded');expect(()=>readToolInspection({...claimed,state:'superseded',result_code:'source_changed'},scope)).toThrow();expect(readToolInspection({...claimed,state:'unknown',result_code:'probe_timeout',source_changed:true},scope).state).toBe('unknown');
 const closedUnknown={...claimed,version:4,state:'closed_unknown',result_code:'probe_timeout',recovery_id:'recovery-original',closed_at:'2026-11-01T00:02:00Z',source_changed:true};expect(readToolInspection(closedUnknown,scope)).toEqual(closedUnknown);expect(()=>readToolInspection({...closedUnknown,recovery_id:undefined},scope)).toThrow();expect(()=>readToolInspection({...claimed,state:'unknown',result_code:'probe_timeout',source_changed:'true'},scope)).toThrow();
});

it('T194 keeps original profile and body-key recovery separate from current source or a fresh read',()=>{
 const request={path:'/tool-profiles/tool-a/inspections',method:'POST',idempotency_key:'request-original',body:{project_id:'p',expected_version:3,idempotency_key:'request-original'}};
 const intent=buildToolReinspectionIntent(tool,scope,request);expect(intent).toEqual({scope,tool,request});expect(validToolReinspectionIntent(intent,scope)).toBe(true);
 expect(toolReinspectionRecoveryScope(scope)).not.toContain('worker-a');expect(validToolReinspectionIntent(intent,{...scope,userID:'user-b'})).toBe(false);
 expect(validToolInspectionReceipt(pending,request,scope,intent!)).toBe(true);expect(validToolInspectionReceipt({...pending,version:3,state:'unknown',fence:0},request,scope,intent!)).toBe(false);
 for(const value of [{...pending,requested_by:'user-b'},{...pending,tool_profile_version:4},{...pending,profile_sha256:'c'.repeat(64)},{...pending,worker_id:'worker-b'}])expect(validToolInspectionReceipt(value,request,scope,intent!)).toBe(false);
 expect(buildToolReinspectionIntent(tool,scope,{...request,body:{...request.body,worker_id:'other'}})).toBeUndefined();
});
