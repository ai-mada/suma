import {createHash,webcrypto} from 'node:crypto';
import {beforeEach,afterEach,expect,it,vi} from 'vitest';
import type {ToolInspection,ToolInspectionRecovery,ToolInspectionStopProof} from '../../../packages/contracts/src';
import {buildToolInspectionRecoveryIntent,readToolInspectionRecoveries,readToolInspectionRecovery,readToolInspectionRecoveryIntent,toolInspectionSourceDigest,validToolInspectionRecoveryReceipt} from './tool-inspection-recovery-model';
const scope={project:'p',toolID:'tool-a',userID:'manager-new',organizationID:'org-a'};
const inspection:ToolInspection={id:'inspection-a',project_id:'p',worker_id:'worker-a',tool_profile_id:'tool-a',tool_profile_version:3,profile_sha256:'a'.repeat(64),command_id:'codex-team',version:3,fence:1,state:'unknown',requested_by:'former-manager',created_at:'2026-11-01T00:00:00Z',expires_at:'2026-11-01T00:05:00Z',claim_id:'claim-original',claimed_at:'2026-11-01T00:00:10Z',lease_until:'2026-11-01T00:01:10Z',result_code:'probe_interrupted'};
const source=createHash('sha256').update(JSON.stringify({ID:inspection.id,ProjectID:'p',WorkerID:'worker-a',ToolProfileID:'tool-a',ToolProfileVersion:3,ProfileSHA256:'a'.repeat(64),CommandID:'codex-team',RequestedBy:'former-manager',CreatedAt:inspection.created_at,ExpiresAt:inspection.expires_at,ClaimID:'claim-original',ClaimedAt:inspection.claimed_at,LeaseUntil:inspection.lease_until,Fence:1})).digest('hex');
const reference={inspection_id:inspection.id,claim_id:'claim-original',fence:1};
const requested:ToolInspectionRecovery={id:'recovery-a',project_id:'p',worker_id:'worker-a',inspection:reference,inspection_version:3,source_sha256:source,version:1,state:'requested',requested_by:'manager-new',created_at:'2026-11-01T00:02:00Z',expires_at:'2026-11-01T00:07:00Z'};
const proof:ToolInspectionStopProof={protocol:'suma-tool-probe-stop-v1',inspection:reference,source_sha256:source,receipt_sha256:'b'.repeat(64),probe_returned:true,all_native_trees_stopped:true,no_pending_inventory:true,native_process_count:2,stopped_process_count:2,observed_at:'2026-11-01T00:02:10Z'};
const proofDigest=createHash('sha256').update(JSON.stringify(proof)).digest('hex');
const closed:ToolInspectionRecovery={...requested,version:3,state:'closed_unknown',proof,proof_sha256:proofDigest,proof_stored_at:'2026-11-01T00:02:11Z',closed_at:'2026-11-01T00:02:12Z'};
beforeEach(()=>vi.stubGlobal('crypto',webcrypto));afterEach(()=>vi.unstubAllGlobals());
it('T195 validates original recovery and native proof digests without converting closure into a completed inspection',async()=>{
 expect(await toolInspectionSourceDigest(inspection)).toBe(source);expect(await readToolInspectionRecovery(requested,scope,inspection)).toEqual(requested);expect(await readToolInspectionRecoveries({items:[closed]},scope,inspection)).toEqual([closed]);
 for(const value of [{...closed,source_sha256:'c'.repeat(64)},{...closed,proof_sha256:'d'.repeat(64)},{...closed,proof:{...proof,all_native_trees_stopped:false}},{...closed,proof:{...proof,stopped_process_count:1}},{...closed,inspection:{...reference,fence:2}},{...closed,proof:undefined},{...requested,state:'closed_unknown'},{...closed,token:'private'},{...closed,worker_id:'worker-b'}])await expect(readToolInspectionRecovery(value,scope,inspection)).rejects.toThrow();
 await expect(readToolInspectionRecoveries({items:[requested,requested]},scope,inspection)).rejects.toThrow();
});
it('T195 preserves original purpose authorization across source hints and a changed current manager without acknowledging from a current head',async()=>{
 const request={path:'/tool-inspections/inspection-a/recoveries',method:'POST',idempotency_key:'recovery-original',body:{project_id:'p',expected_version:3,inspection:reference,idempotency_key:'recovery-original'}};
 const intent=await buildToolInspectionRecoveryIntent({...inspection,source_changed:true},scope,request);expect(intent).toMatchObject({scope,inspection:{id:'inspection-a',source_changed:true},source_sha256:source,request});expect(await readToolInspectionRecoveryIntent(intent,scope)).toEqual(intent);
 expect(await validToolInspectionRecoveryReceipt(requested,request,scope,intent!)).toBe(true);expect(await validToolInspectionRecoveryReceipt({...requested,inspection_version:4},request,scope,intent!)).toBe(true);expect(await validToolInspectionRecoveryReceipt({...requested,inspection_version:5},request,scope,intent!)).toBe(false);expect(await validToolInspectionRecoveryReceipt(closed,request,scope,intent!)).toBe(false);
 expect(await validToolInspectionRecoveryReceipt({...requested,requested_by:'former-manager'},request,scope,intent!)).toBe(false);await expect(readToolInspectionRecoveryIntent(intent,{...scope,userID:'other'})).rejects.toThrow();
 expect(await buildToolInspectionRecoveryIntent(inspection,scope,{...request,body:{...request.body,kill:true}})).toBeUndefined();
});
