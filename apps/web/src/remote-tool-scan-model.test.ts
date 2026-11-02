import {createHash,webcrypto} from 'node:crypto';
import {expect,it,vi} from 'vitest';
import {readRemoteToolScan,readRemoteToolScans,validRemoteToolScanReceipt,validRemoteToolScanRequest} from './remote-tool-scan-model';

const scope={project:'p',workerID:'worker-one',userID:'user-one',organizationID:'team-one'};
const at='2026-11-02T00:00:00Z';
export function scanFixture(state='completed',adapter='codebuddy-cli'){
 const source={ID:'scan-one',ProjectID:'p',WorkerID:'worker-one',AdapterIDs:[adapter],InstanceID:'instance-one',RequestedBy:'user-one',CreatedAt:at,ExpiresAt:'2026-11-02T00:05:00Z'};
 const source_sha256=createHash('sha256').update(JSON.stringify(source)).digest('hex');
 const base={id:source.ID,project_id:source.ProjectID,worker_id:source.WorkerID,adapter_ids:source.AdapterIDs,instance_id:source.InstanceID,version:1,state,requested_by:source.RequestedBy,created_at:source.CreatedAt,expires_at:source.ExpiresAt,source_sha256,fence:0};
 if(state==='pending')return base;
 const item={adapter_id:adapter,protocol:adapter==='claude-code-sdk'?'claude-sdk-v1':'codebuddy-jsonl-v1',installed:true,launch_supported:false,reason_code:'configuration_required',version:'reviewed-native-version',executable_sha256:'a'.repeat(64)};
 return {...base,version:3,fence:1,claim_id:'claim-one',claimed_at:at,lease_until:'2026-11-02T00:01:00Z',finished_at:'2026-11-02T00:00:03Z',result_code:'scan_completed',result:{items:[item],stop_proof:{protocol:'suma-installed-tool-stop-v1',scan:{scan_id:base.id,claim_id:'claim-one',fence:1},source_sha256,receipt_sha256:'b'.repeat(64),probe_returned:true,all_native_trees_stopped:true,native_process_count:0,stopped_process_count:0,observed_at:'2026-11-02T00:00:02Z'}}};
}
export const maintenanceFixture=()=>({state:'supported',checked_at:at,protocol:'suma-tool-maintenance-v1',instance_id:'instance-one',observed_at:at,expires_at:'2026-11-02T00:01:00Z',installed_discovery:{protocol:'suma-installed-tool-scan-v1',adapter_ids:['codebuddy-cli','claude-code-sdk']}});

it('T205 reads exact installation source and required zero tree proof, rejecting null, missing fields, scope drift and capability contradictions',async()=>{
 vi.stubGlobal('crypto',webcrypto);try{
 const value=scanFixture();expect((await readRemoteToolScan(value,scope)).result?.stop_proof?.native_process_count).toBe(0);
 const bad=structuredClone(value) as any;delete bad.result.stop_proof.native_process_count;await expect(readRemoteToolScan(bad,scope)).rejects.toThrow();
 for(const change of [{result:null},{source_sha256:'c'.repeat(64)},{worker_id:'wrong'},{source_changed:null},{extra:'not-public'},{fence:0}])await expect(readRemoteToolScan({...value,...change},scope)).rejects.toThrow();
 const page={project_id:'p',worker_id:'worker-one',as_of:at,maintenance:maintenanceFixture(),items:[value]};expect((await readRemoteToolScans(page,scope)).items).toHaveLength(1);
 for(const maintenance of [{...page.maintenance,installed_discovery:null},{state:'unknown',checked_at:at,installed_discovery:page.maintenance.installed_discovery},{...page.maintenance,state:'unsupported',protocol:'suma-tool-maintenance-v2'}])await expect(readRemoteToolScans({...page,maintenance},scope)).rejects.toThrow();
 const request={path:'/tool-installation-scans',method:'POST',body:{project_id:'p',worker_id:'worker-one',adapter_ids:['codebuddy-cli'],idempotency_key:'operation-one'},idempotency_key:'operation-one'};
 expect(validRemoteToolScanRequest(request,scope)).toBe(true);expect(await validRemoteToolScanReceipt(value,request,scope)).toBe(true);expect(await validRemoteToolScanReceipt(value,{...request,body:{...request.body,adapter_ids:['claude-code-sdk']}},scope)).toBe(false);
 }finally{vi.unstubAllGlobals();}
});
