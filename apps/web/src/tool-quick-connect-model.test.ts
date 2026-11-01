import {expect,it} from 'vitest';
import {quickConnectAdapters,quickConnectWorkers,matchingQuickTools,validQuickConnectReceipt,readInstalledToolDiscovery,isCursorConnectionGuide,localDiscoveryAdapters} from './tool-quick-connect-model';
import type {AdapterDescriptor} from './AdapterConnections';

const adapter={id:'codex-jsonl',product:'Codex',direction:'suma_to_tool',implementation:'implemented',setup_kind:'runner_profile',adapter:'codex',protocol:'codex-jsonl-v1'} as AdapterDescriptor;
const profile={command_id:'codex-team',adapter:'codex',protocol:'codex-jsonl-v1',profile_sha256:'a'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'1.0',read_only:false};
it('matches OpenCode fixed inventory without comparing native server identity to the Node host',()=>{
 const openAdapter={...adapter,id:'opencode-sdk',product:'OpenCode',adapter:'opencode',protocol:'opencode-sdk-v1'};
 const cap={sdk_version:'1.19.4',server_version:'1.19.4',provider_id:'provider',model:'vendor/model',endpoint_origin:'https://provider.example',configuration_sha256:'c'.repeat(64),budget_mode:'observe_only',identity_evidence:'configuration_ack_only'};
 const item={profile:{...profile,adapter:'opencode',protocol:'opencode-sdk-v1',tool_version:'suma-opencode-runtime 1.0.0',opencode_execution:cap},available:true,reason:''};
 const proof={model:cap.model,provider_id:cap.provider_id,endpoint_origin:cap.endpoint_origin,tool_version:'1.19.4',executable_sha256:'d'.repeat(64),configuration_sha256:cap.configuration_sha256};
 expect(matchingQuickTools([item],openAdapter,proof)).toEqual([item]);
 for(const patch of [{model:'other'},{provider_id:'other'},{endpoint_origin:'https://other.example'},{configuration_sha256:'e'.repeat(64)}])expect(matchingQuickTools([item],openAdapter,{...proof,...patch})).toEqual([]);
 expect(matchingQuickTools([{...item,profile:{...item.profile,tool_version:'1.19.4'}}],openAdapter,proof)).toEqual([]);
 expect(matchingQuickTools([{...item,available:false}],openAdapter,proof)).toEqual([]);
 const row={adapter_id:'opencode-sdk',product:'OpenCode',installed:true,launch_supported:false,executable:'C:/tools/opencode.exe',version:'1.19.4',executable_sha256:'a'.repeat(64),reason_code:'configuration_required'};
 const read=(value:unknown)=>readInstalledToolDiscovery({status:'observed',report:{protocol:'suma-installed-tools-v1',items:[value]}},'opencode-sdk');
 expect(read(row)).toEqual(row);expect(read({...row,version:'1.20.0',reason_code:'version_unsupported'})).toBeDefined();
 for(const patch of [{version:'1.20.0'},{reason_code:'version_unsupported'},{launch_supported:true},{command:['run']},{profile:{}},{executable:'relative.exe'},{executable_sha256:undefined}])expect(read({...row,...patch})).toBeUndefined();
});
it('offers only implemented execution entries and distinct active authorized workers',()=>{
 expect(quickConnectAdapters([adapter,{...adapter,id:'planned',implementation:'planned'},{...adapter,id:'client',direction:'tool_to_suma',setup_kind:'client_export'}])).toEqual([adapter]);
 expect(quickConnectWorkers([{worker_id:'one',status:'active'},{worker_id:'old',status:'revoked'}],[{worker_id:'one',status:'active'},{worker_id:'two',status:'active'}])).toEqual(['one','two']);
});

const cursorGuide={id:'cursor-cli',product:'Cursor',direction:'suma_to_tool',implementation:'planned',setup_kind:'manual_guide'} as AdapterDescriptor;
const cursorRow=(reason_code='cursor_agent_unverified',executable:string|undefined='C:/tools/agent.exe')=>({adapter_id:'cursor-cli',product:'Cursor',installed:!!executable,launch_supported:false,reason_code,...(executable?{executable}:{})});
const cursorReport=(row:unknown)=>({status:'observed',report:{protocol:'suma-installed-tools-v1',items:[row]}});
it('routes only the implemented Cursor ACP descriptor and its exact current runtime inventory into configuration',()=>{
 const active={...cursorGuide,implementation:'implemented',setup_kind:'runner_profile',adapter:'cursor',protocol:'acp-stdio-v1'} as AdapterDescriptor;
 const capability={runtime:'cursor',protocol_version:1,sdk_version:'0.13.5',configuration_sha256:'c'.repeat(64),model:'observed-model',model_source:'administrator_session_config',budget_mode:'observe_only',identity_evidence:'configuration_ack_only',auth_strategy:'explicit_api_key_env',permission_mode:'human_once'};
 const item={profile:{...profile,adapter:'cursor',protocol:'acp-stdio-v1',user_input_protocol:'cursor-acp-input-v1',acp_execution:capability},available:true,reason:''};
 expect(quickConnectAdapters([adapter,cursorGuide,active])).toEqual([adapter,active]);expect(isCursorConnectionGuide(active)).toBe(false);expect(matchingQuickTools([item],active)).toEqual([item]);
 for(const patch of [{adapter:'qoder'},{protocol:'codex-jsonl-v1'},{product:'Other'},{direction:'tool_to_suma'},{implementation:'planned'},{setup_kind:'manual_guide'}])expect(quickConnectAdapters([{...active,...patch} as AdapterDescriptor])).toEqual([]);
 for(const patch of [{runtime:'qoder'},{auth_strategy:'existing_cli_login'}])expect(matchingQuickTools([{...item,profile:{...item.profile,acp_execution:{...capability,...patch}}}],active)).toEqual([]);
 expect(matchingQuickTools([{...item,profile:{...item.profile,adapter:'qoder'}}],active)).toEqual([]);
});
it('offers the exact Cursor guide without admitting forged execution or other planned entries',()=>{
 expect(quickConnectAdapters([adapter,cursorGuide,{...cursorGuide,id:'other-planned'},{...adapter,id:'cursor-cli'}])).toEqual([adapter]);
 expect(isCursorConnectionGuide(cursorGuide)).toBe(true);expect(isCursorConnectionGuide({...cursorGuide,adapter:'',protocol:''})).toBe(true);
 for(const patch of [{product:'cursor'},{direction:'tool_to_suma'},{implementation:'implemented'},{setup_kind:'runner_profile'},{adapter:'cursor'},{protocol:'cursor-stream-v1'},{adapter:null},{protocol:null}])expect(isCursorConnectionGuide({...cursorGuide,...patch} as AdapterDescriptor)).toBe(false);
 expect(matchingQuickTools([{profile,available:true,reason:''}],{...adapter,id:'cursor-cli'})).toEqual([]);
 expect(localDiscoveryAdapters.has('cursor-cli')).toBe(true);
});
it('reads all five passive Cursor observations without claiming execution or version',()=>{
 for(const reason of ['not_installed','cursor_editor_only','cursor_scan_incomplete']){
  const row=cursorRow(reason,'');expect(readInstalledToolDiscovery(cursorReport(row),'cursor-cli')).toEqual(row);
 }
 for(const [reason,path] of [['cursor_agent_unverified','C:\\tools\\agent.exe'],['cursor_launcher_unsupported','/opt/tools/cursor-agent']]){
  const row=cursorRow(reason,path);expect(readInstalledToolDiscovery(cursorReport(row),'cursor-cli')).toEqual(row);
 }
});
it('reads the reviewed Cursor package Node fact while keeping passive candidates and failed version probes unusable as execution proof',()=>{
 const row={adapter_id:'cursor-cli',product:'Cursor',installed:true,launch_supported:false,reason_code:'acp_configuration_required',executable:'C:/tools/cursor-agent/node.exe',version:'Cursor fixture exact version',executable_sha256:'b'.repeat(64)};
 expect(readInstalledToolDiscovery(cursorReport(row),'cursor-cli')).toEqual(row);
 expect(readInstalledToolDiscovery(cursorReport({adapter_id:'cursor-cli',product:'Cursor',installed:true,launch_supported:false,reason_code:'probe_failed'}),'cursor-cli')).toBeDefined();
 for(const patch of [{executable:'C:/tools/cursor.cmd'},{executable:'C:/tools/agent.exe'},{executable_sha256:undefined},{version:undefined},{installed:false},{launch_supported:true},{command:['agent','acp']},{profile},{reason_code:'cursor_agent_unverified'},{reason_code:'probe_failed'}])expect(readInstalledToolDiscovery(cursorReport({...row,...patch}),'cursor-cli')).toBeUndefined();
});
it('rejects Cursor facts that invent execution, inconsistent reasons, paths or open wire fields',()=>{
 const row=cursorRow();
 for(const patch of [{launch_supported:true},{launch_supported:'false'},{installed:false},{installed:'true'},{product:'Other'},{adapter_id:'codex-jsonl'},{reason_code:'workspace_required'},{reason_code:'not_installed'},{reason_code:'cursor_editor_only'},{reason_code:'cursor_scan_incomplete'},{version:undefined},{version:'1.0'},{command:undefined},{command:'agent'},{profile},{token:'secret'},{executable:undefined},{executable:'relative.exe'},{executable:'C:agent.exe'},{executable:'https://host/agent'},{executable:'C:/tools/\ud800.exe'},{executable:'/tools/agent\n'},{executable:'/tools/agent\u2028'},{executable:'/'+ 'x'.repeat(4096)}])expect(readInstalledToolDiscovery(cursorReport({...row,...patch}),'cursor-cli'),JSON.stringify(Object.keys(patch))).toBeUndefined();
 for(const key of ['adapter_id','product','installed','launch_supported','reason_code']){const missing:Record<string,unknown>={...row};delete missing[key];expect(readInstalledToolDiscovery(cursorReport(missing),'cursor-cli')).toBeUndefined();}
 for(const value of [{...cursorReport(row),extra:true},{status:'observed',report:{protocol:'suma-installed-tools-v1',items:[row],extra:true}},{status:'observed',report:{protocol:'other',items:[row]}},{status:'observed',report:{protocol:'suma-installed-tools-v1',items:[]}},{status:'observed',report:{protocol:'suma-installed-tools-v1',items:[row,row]}}])expect(readInstalledToolDiscovery(value,'cursor-cli')).toBeUndefined();
});
it('rejects abnormal Cursor container and row prototypes and hidden own fields',()=>{
 const row=cursorRow(),report=cursorReport(row);
 for(const value of [Object.assign(Object.create({inherited:true}),report),{...report,report:Object.assign(Object.create({inherited:true}),report.report)},cursorReport(Object.assign(Object.create({inherited:true}),row)),cursorReport(Object.defineProperty({...row},'command',{value:undefined})),cursorReport({...row,[Symbol('profile')]:profile})])expect(readInstalledToolDiscovery(value,'cursor-cli')).toBeUndefined();
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
it('matches native Codex inventory to the exact confirmed connection configuration and catalogue',()=>{
 const nativeAdapter={...adapter,id:'codex-app-server',protocol:'codex-app-server-v1'},nativeProfile={...profile,protocol:nativeAdapter.protocol};
 const catalogue={protocol:nativeAdapter.protocol,configuration_sha256:'c'.repeat(64),catalog_sha256:'d'.repeat(64),models:[{model:'model-a',reasoning_efforts:[],default_reasoning_effort:''}]};
 const observation={profile:nativeProfile,available:true,reason:'',codex_catalog:catalogue};
 const proof={configuration_sha256:catalogue.configuration_sha256,catalog_sha256:catalogue.catalog_sha256,executable_sha256:nativeProfile.executable_sha256,tool_version:nativeProfile.tool_version};
 expect(matchingQuickTools([observation],nativeAdapter,proof)).toEqual([observation]);
 for(const patch of [{configuration_sha256:'e'.repeat(64)},{catalog_sha256:'e'.repeat(64)},{executable_sha256:'e'.repeat(64)},{tool_version:'different'}])expect(matchingQuickTools([observation],nativeAdapter,{...proof,...patch})).toEqual([]);
 expect(matchingQuickTools([{...observation,available:false}],nativeAdapter,proof)).toEqual([]);
 expect(matchingQuickTools([{profile:nativeProfile,available:true,reason:''}],nativeAdapter,proof)).toEqual([]);
 expect(matchingQuickTools([observation],nativeAdapter)).toEqual([observation]);
 expect(localDiscoveryAdapters.has('codex-app-server')).toBe(true);
 const row={adapter_id:'codex-app-server',product:'Codex',installed:true,launch_supported:false,executable:'C:/tools/codex.exe',version:'codex-cli 0.134.0',executable_sha256:'a'.repeat(64),reason_code:'configuration_required'};
 const report={status:'observed',report:{protocol:'suma-installed-tools-v1',items:[row]}};
 expect(readInstalledToolDiscovery(report,'codex-app-server')).toEqual(row);
 for(const patch of [{launch_supported:true},{executable_sha256:undefined},{reason_code:'workspace_required'},{command:['app-server']}])expect(readInstalledToolDiscovery({...report,report:{...report.report,items:[{...row,...patch}]}},'codex-app-server')).toBeUndefined();
});
