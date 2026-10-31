import {expect,it} from 'vitest';
import {localToolsScanBridge,validLocalToolsScanScope,readLocalToolsScanResult,scanAdapterIDs,type LocalToolsScanScope} from './local-tools-scan-model';
const scope:LocalToolsScanScope={project:'1'.repeat(32),userID:'2'.repeat(32),organizationID:'3'.repeat(32)};
function result(){const names=['Codex','CodeBuddy Code','DeepSeek Harness / dsh','Claude Code','Cursor','Qoder CLI','Codex','OpenCode'];return {status:'observed',report:{protocol:'suma-installed-tools-v1',items:scanAdapterIDs.map((adapter_id,index)=>({adapter_id,product:names[index],installed:false,launch_supported:false,reason_code:'not_installed'}))},scan:{id:'4'.repeat(32),project_id:scope.project,user_id:scope.userID,tenant_id:scope.organizationID,observed_at:'2026-10-31T10:00:00.123456789Z',expires_at:'2026-10-31T10:01:00.123456789Z',cached:false}};}
it('accepts exactly eight scoped installation facts and rejects omitted duplicate or contaminated rows',()=>{
 const value=result();expect(readLocalToolsScanResult(value,scope)).toEqual(value);
 for(const patch of [{project_id:'5'.repeat(32)},{user_id:'5'.repeat(32)},{tenant_id:'5'.repeat(32)},{id:'BAD'},{cached:'true'},{token:'secret'}])expect(readLocalToolsScanResult({...value,scan:{...value.scan,...patch}},scope)).toBeUndefined();
 for(const items of [value.report.items.slice(1),[...value.report.items,value.report.items[0]],[value.report.items[1],value.report.items[0],...value.report.items.slice(2)],[...value.report.items.slice(0,6),value.report.items[0]]])expect(readLocalToolsScanResult({...value,report:{...value.report,items}},scope)).toBeUndefined();
 for(const patch of [{command:['codex']},{profile:{}},{executable_sha256:'a'.repeat(64)},{product:'Other'},{version:'bad\ud800'}])expect(readLocalToolsScanResult({...value,report:{...value.report,items:[{...value.report.items[0],...patch},...value.report.items.slice(1)]}},scope)).toBeUndefined();
 expect(readLocalToolsScanResult({...value,report:{...value.report,token:'secret'}},scope)).toBeUndefined();
 const qoder={...value.report.items[5],installed:true,reason_code:'acp_configuration_required',executable:'C:/tools/qoder.exe',version:'Qoder CLI 1.2',executable_sha256:'a'.repeat(64)};
 const native={...value,report:{...value.report,items:value.report.items.map((item,index)=>index===5?qoder:item)}};
 expect(readLocalToolsScanResult(native,scope)).toEqual(native);
 for(const patch of [{launch_supported:true},{reason_code:'workspace_required'},{executable_sha256:undefined},{command:[]},{executable:'relative.exe'}])expect(readLocalToolsScanResult({...native,report:{...native.report,items:native.report.items.map((item,index)=>index===5?{...qoder,...patch}:item)}},scope)).toBeUndefined();
});
it('preserves exact sixty-second scan timestamps and cached provenance without renewing an older observation',()=>{
 const value=result(),cached={...value,scan:{...value.scan,cached:true}};
 expect(readLocalToolsScanResult(cached,scope)).toEqual(cached);
 for(const patch of [{expires_at:'2026-10-31T10:01:00.123456788Z'},{expires_at:'2026-10-31T10:01:00.123456790Z'},{expires_at:'2026-10-31T10:00:00.123456789Z'},{observed_at:'2026-02-30T10:00:00Z',expires_at:'2026-02-30T10:01:00Z'},{observed_at:'2026-10-31T10:00:00+00:00'},{observed_at:'not-time'}])expect(readLocalToolsScanResult({...value,scan:{...value.scan,...patch}},scope)).toBeUndefined();
 expect(readLocalToolsScanResult({status:'failed',code:'installed_tools_access_changed'},scope)).toEqual({status:'failed',code:'installed_tools_access_changed'});
 for(const bad of [{status:'failed',code:'unsafe\ncode'},{status:'failed',code:'failed',token:'secret'},{...value,status:'healthy'},{...value,unknown:true}])expect(readLocalToolsScanResult(bad,scope)).toBeUndefined();
});
it('uses only the original native discovery bridge and a complete current identity scope',()=>{
 const discoverInstalledTools=async()=>result();
 const bridge={platform:'win32',version:'1.0.0',discoverInstalledTools};expect(localToolsScanBridge(bridge)).toBe(bridge);expect(validLocalToolsScanScope(scope)).toBe(true);
 for(const patch of [{platform:'web'},{platform:['win32']},{version:'2.0.0'},{discoverInstalledTools:true}])expect(localToolsScanBridge({...bridge,...patch})).toBeUndefined();
 for(const patch of [{project:''},{userID:'A'.repeat(32)},{organizationID:'other'},{adapterID:'codex-jsonl'}])expect(validLocalToolsScanScope({...scope,...patch})).toBe(false);
 expect(localToolsScanBridge({platform:'win32',version:'1.0.0',manageLocalTool:discoverInstalledTools})).toBeUndefined();
});
