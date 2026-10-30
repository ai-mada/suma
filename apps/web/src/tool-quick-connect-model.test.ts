import {expect,it} from 'vitest';
import {quickConnectAdapters,quickConnectWorkers,matchingQuickTools,validQuickConnectReceipt,readInstalledToolDiscovery} from './tool-quick-connect-model';
import type {AdapterDescriptor} from './AdapterConnections';

const adapter={id:'codex-jsonl',product:'Codex',direction:'suma_to_tool',implementation:'implemented',setup_kind:'runner_profile',adapter:'codex',protocol:'codex-jsonl-v1'} as AdapterDescriptor;
const profile={command_id:'codex-team',adapter:'codex',protocol:'codex-jsonl-v1',profile_sha256:'a'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'1.0',read_only:false};
it('offers only implemented execution entries and distinct active authorized workers',()=>{
 expect(quickConnectAdapters([adapter,{...adapter,id:'planned',implementation:'planned'},{...adapter,id:'client',direction:'tool_to_suma',setup_kind:'client_export'}])).toEqual([adapter]);
 expect(quickConnectWorkers([{worker_id:'one',status:'active'},{worker_id:'old',status:'revoked'}],[{worker_id:'one',status:'active'},{worker_id:'two',status:'active'}])).toEqual(['one','two']);
});
it('keeps adapter and protocol identity exact rather than matching a product name',()=>{
 const observation={profile,available:true,reason:''};
 expect(matchingQuickTools([observation,{...observation,profile:{...profile,protocol:'codex-app-server-v1'}}],adapter)).toEqual([observation]);
});
it('accepts only the exact paused registration receipt, including complete profile capabilities',()=>{
 const request={project_id:'p',worker_id:'one',name:'Team Codex',profile};
 const receipt={...request,id:'saved',version:1,state:'paused'};
 expect(validQuickConnectReceipt(receipt,request)).toBe(true);
 for(const change of [{state:'enabled'},{project_id:'other'},{worker_id:'two'},{name:'wrong'},{version:0},{profile:{...profile,read_only:true}},{profile:{...profile,terminal_session_protocol:'other'}}])expect(validQuickConnectReceipt({...receipt,...change},request)).toBe(false);
});
it('validates installation observations without converting them into dispatch profiles',()=>{
 const result={status:'observed',report:{protocol:'suma-installed-tools-v1',items:[{adapter_id:'codex-jsonl',product:'Codex',installed:true,launch_supported:true,executable:'C:/tools/codex.exe',version:'1.0',reason_code:'workspace_required'}]}};
 expect(readInstalledToolDiscovery(result,'codex-jsonl')?.installed).toBe(true);
 expect(readInstalledToolDiscovery({...result,token:'private'},'codex-jsonl')).toBeUndefined();
 expect(readInstalledToolDiscovery(result,'codebuddy-cli')).toBeUndefined();
 expect(readInstalledToolDiscovery({...result,report:{...result.report,items:[{...result.report.items[0],profile}]}},'codex-jsonl')).toBeUndefined();
 for(const change of [{product:'Other'},{reason_code:'healthy'},{installed:false},{launch_supported:false},{executable:'relative.exe'},{version:undefined}])expect(readInstalledToolDiscovery({...result,report:{...result.report,items:[{...result.report.items[0],...change}]}},'codex-jsonl')).toBeUndefined();
 const withVersion=(version:string)=>({...result,report:{...result.report,items:[{...result.report.items[0],version}]}});
 expect(readInstalledToolDiscovery(withVersion('v'.repeat(256)),'codex-jsonl')?.version).toHaveLength(256);
 for(const version of ['v'.repeat(257),'版'.repeat(86),'version\u2028next','version\u2029next','version\nnext'])expect(readInstalledToolDiscovery(withVersion(version),'codex-jsonl')).toBeUndefined();
});
