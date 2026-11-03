import {expect,it} from 'vitest';
import {validToolInstallationProbeDiagnostic,validToolInstallationProbeDiagnosticField} from './tool-installation-probe-diagnostic-model';
import {readInstalledToolDiscovery} from './tool-quick-connect-model';
import {readLocalToolsScanResult,scanAdapterIDs} from './local-tools-scan-model';
import {validRemoteToolScanItem} from './remote-tool-scan-model';

it('T226 reads classified probe facts through both strict readers without inventing missing diagnostics or accepting null, aliases and contradictory actions',()=>{
 const diagnostic={stage:'help',code:'help_contract_mismatch',next_action:'guide'};
 expect(validToolInstallationProbeDiagnostic(diagnostic)).toBe(true);
 for(const invalid of [null,undefined,{}, {...diagnostic,nextAction:'guide'}, {...diagnostic,next_action:'scan'}, {...diagnostic,stage:'version'}, {...diagnostic,code:'arbitrary_stderr'}, {...diagnostic,message:'private path'}, {stage:'version',code:'stop_unverified',next_action:'scan'}])expect(validToolInstallationProbeDiagnostic(invalid)).toBe(false);
 for(const stage of ['resolve','source','version','help','protocol'])expect(validToolInstallationProbeDiagnostic({stage,code:'permission_denied',next_action:'guide'})).toBe(true);
 for(const reason_code of ['not_installed','workspace_required','configuration_required','acp_configuration_required','cursor_editor_only'])expect(validToolInstallationProbeDiagnosticField({reason_code,launch_supported:false,diagnostic})).toBe(false);
 expect(validToolInstallationProbeDiagnosticField({reason_code:'probe_failed',launch_supported:true,diagnostic})).toBe(false);
 expect(validToolInstallationProbeDiagnosticField({reason_code:'probe_failed',launch_supported:false})).toBe(true);
 const scope={project:'1'.repeat(32),userID:'2'.repeat(32),organizationID:'3'.repeat(32)},products=['Codex','CodeBuddy Code','DeepSeek Harness / dsh','Claude Code','Cursor','Qoder CLI','Codex','OpenCode','Claude Code'];
 const row=(index:number)=>({adapter_id:scanAdapterIDs[index],product:products[index],installed:true,launch_supported:false,reason_code:'probe_failed',diagnostic});
 const result=(items:unknown[])=>({status:'observed',report:{protocol:'suma-installed-tools-v1',items},scan:{id:'4'.repeat(32),project_id:scope.project,user_id:scope.userID,tenant_id:scope.organizationID,observed_at:'2026-11-03T00:00:00Z',expires_at:'2026-11-03T00:01:00Z',cached:false}});
 const items=scanAdapterIDs.map((_,index)=>row(index)),local=readLocalToolsScanResult(result(items),scope);
 expect(local?.status).toBe('observed');if(local?.status==='observed')expect(local.report.items.map(item=>item.diagnostic)).toEqual(items.map(()=>diagnostic));
 const remote={adapter_id:'codebuddy-cli',protocol:'codebuddy-jsonl-v1',installed:true,launch_supported:false,reason_code:'probe_failed',diagnostic};
 expect(validRemoteToolScanItem(remote)).toBe(true);
 for(const invalid of [null,undefined,{...diagnostic,next_action:'scan'},{...diagnostic,extra:'private'}]){
  expect(readLocalToolsScanResult(result([{...items[0],diagnostic:invalid},...items.slice(1)]),scope)).toBeUndefined();
  expect(validRemoteToolScanItem({...remote,diagnostic:invalid})).toBe(false);
 }
 const noDiagnostic={adapter_id:'codebuddy-cli',product:'CodeBuddy Code',installed:true,launch_supported:false,reason_code:'probe_failed'};
 expect(readInstalledToolDiscovery({status:'observed',report:{protocol:'suma-installed-tools-v1',items:[noDiagnostic]}},'codebuddy-cli')).toEqual(noDiagnostic);
 const incomplete={...remote,installed:false,reason_code:'scan_incomplete',diagnostic:{stage:'resolve',code:'source_unavailable',next_action:'guide'}};
 expect(validRemoteToolScanItem(incomplete)).toBe(true);expect(validRemoteToolScanItem({...incomplete,installed:true})).toBe(false);expect(validRemoteToolScanItem({...incomplete,version:'unverified'})).toBe(false);expect(validRemoteToolScanItem({...incomplete,diagnostic})).toBe(false);
});
