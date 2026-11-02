import {expect,it} from 'vitest';
import {desktopToolBridge,readDesktopToolResult,validDesktopToolScope} from './desktop-tool-connector-model';

const scope={project:'1'.repeat(32),userID:'2'.repeat(32),organizationID:'3'.repeat(32),adapterID:'codex-jsonl'};
const connection={project_id:scope.project,adapter_id:'codex-jsonl',phase:'connected',worker_id:'desktop-'+'4'.repeat(32),base_commit:'a'.repeat(40),expires_at:'2099-01-01T00:00:00Z',repository_name:'project'};
it('T201 keeps complete Claude SDK host proof distinct from CLI discovery and private setup',()=>{
 const sdkScope={...scope,adapterID:'claude-code-sdk'},capability={sdk_version:'0.3.314',cli_version:'2.1.314',model:'claude-sonnet-4-6',configuration_sha256:'b'.repeat(64),model_source:'administrator_sdk_option',budget_mode:'observe_only',identity_evidence:'sdk_init_model_ack_only',auth_strategy:'api_key_env_ref'},native={...connection,adapter_id:sdkScope.adapterID,model:capability.model,credential_env_ref:'ANTHROPIC_API_KEY',tool_version:'2.1.314',executable_sha256:'a'.repeat(64),runtime_executable_sha256:'c'.repeat(64),configuration_sha256:capability.configuration_sha256,claude_sdk_execution:capability};
 expect(validDesktopToolScope(sdkScope)).toBe(true);expect(readDesktopToolResult({status:'ok',connection:native},sdkScope)).toEqual({status:'ok',connection:native});
 for(const key of ['model','credential_env_ref','tool_version','executable_sha256','runtime_executable_sha256','configuration_sha256','claude_sdk_execution']){const missing:Record<string,unknown>={...native};delete missing[key];expect(readDesktopToolResult({status:'ok',connection:missing},sdkScope)).toBeUndefined();}
 for(const patch of [{tool_version:'suma-claude-runtime 1.0.0'},{model:'claude-opus-4-6'},{runtime_executable_sha256:'bad'},{configuration_sha256:'d'.repeat(64)},{credential_env_ref:'NODE_OPTIONS'},{credential_env_ref:'sk-secret-value'},{credential_env_ref:null},{claude_sdk_setup:{}},{node_executable:'C:/private/node.exe'},{catalog_sha256:'d'.repeat(64)},{claude_sdk_execution:{...capability,auth_strategy:'login_cache'}},{claude_sdk_execution:{...capability,secret:'not-public'}}])expect(readDesktopToolResult({status:'ok',connection:{...native,...patch}},sdkScope)).toBeUndefined();
 expect(readDesktopToolResult({status:'ok',connection:{...connection,credential_env_ref:native.credential_env_ref}},scope)).toBeUndefined();
 expect(readDesktopToolResult({status:'ok',connection:{...connection,runtime_executable_sha256:native.runtime_executable_sha256}},scope)).toBeUndefined();
 expect(readDesktopToolResult({status:'ok',connection:{project_id:scope.project,adapter_id:sdkScope.adapterID,phase:'unconfigured'}},sdkScope)).toBeDefined();
 const progress={project_id:scope.project,adapter_id:sdkScope.adapterID,phase:'starting',worker_id:connection.worker_id};
 for(const fields of [{model:native.model},{credential_env_ref:native.credential_env_ref},{model:native.model,credential_env_ref:native.credential_env_ref},{configuration_sha256:native.configuration_sha256},{claude_sdk_execution:capability},{model:native.model,configuration_sha256:native.configuration_sha256,claude_sdk_execution:capability}])expect(readDesktopToolResult({status:'ok',connection:{...progress,...fields}},sdkScope)).toBeDefined();
 for(const fields of [{model:'not-a-claude-model'},{credential_env_ref:'PATH'},{credential_env_ref:null},{tool_version:'2.1.315'},{model:'claude-opus-4-6',claude_sdk_execution:capability},{configuration_sha256:'e'.repeat(64),claude_sdk_execution:capability},{claude_sdk_execution:{...capability,auth_strategy:'login_cache'}}])expect(readDesktopToolResult({status:'ok',connection:{...progress,...fields}},sdkScope)).toBeUndefined();
});
it('reads result recovery metadata without replacing connection or credential facts',()=>{
 const recovery={operation_id:'5'.repeat(32),phase:'partial',total:3,recovered:1,remaining:2},original={...connection,phase:'stopped',credential_version:2,credential_expires_at:'2026-01-01T00:00:00Z'};
 expect(readDesktopToolResult({status:'ok',connection:{...original,recovery}},scope)).toEqual({status:'ok',connection:{...original,recovery}});
 for(const phase of ['prepared','checking','unknown','complete','partial','blocked'])expect(readDesktopToolResult({status:'ok',connection:{...original,recovery:{...recovery,phase}}},scope)).toBeDefined();
 for(const reason_code of ['no_saved_results','receipt_missing','receipt_mismatch','unsupported_pending','state_changed','source_changed','access_changed','invalid_state','active_worker','result_unknown','proof_expired'])expect(readDesktopToolResult({status:'ok',connection:{...original,recovery:{...recovery,reason_code}}},scope)).toBeDefined();
 expect(readDesktopToolResult({status:'ok',connection:{...original,recovery:{...recovery,total:Number.MAX_SAFE_INTEGER,recovered:0,remaining:0}}},scope)).toBeDefined();
 const absent=readDesktopToolResult({status:'ok',connection:original},scope);if(absent?.status==='ok')expect(absent.connection).not.toHaveProperty('recovery');
});
it('rejects malformed result recovery counters identities and private plan fields',()=>{
 const recovery={operation_id:'5'.repeat(32),phase:'unknown',total:3,recovered:1,remaining:2};
 const read=(value:unknown)=>readDesktopToolResult({status:'ok',connection:{...connection,recovery:value}},scope);
 expect(read(recovery)).toBeDefined();
 for(const patch of [{operation_id:'5'.repeat(31)},{operation_id:'A'.repeat(32)},{phase:'succeeded'},{reason_code:'future_reason'},{reason_code:'secret\nvalue'},{reason_code:null},{plan:{}},{proof:{}},{key:'secret'},{path:'private'},{secret:'value'}])expect(read({...recovery,...patch})).toBeUndefined();
 for(const key of ['total','recovered','remaining'])for(const value of [-1,0.5,Number.MAX_SAFE_INTEGER+1,NaN,Infinity,'1',null])expect(read({...recovery,[key]:value})).toBeUndefined();
 for(const key of Object.keys(recovery)){const missing:Record<string,unknown>={...recovery};delete missing[key];expect(read(missing)).toBeUndefined();}
 for(const value of [null,[],Object.assign(Object.create({plan:{}}),recovery),Object.defineProperty({...recovery},'proof',{value:{}})])expect(read(value)).toBeUndefined();
});
it('keeps credential registration expiry and version independent from inventory and renewal outcome',()=>{
 const current={...connection,phase:'renewal_unknown',credential_expires_at:'2026-01-01T00:00:00.123456789Z',credential_version:2};
 expect(readDesktopToolResult({status:'ok',connection:current},scope)).toEqual({status:'ok',connection:current});
 for(const renewal_reason of ['execution_pending','local_state_pending','credential_changed','access_changed','context_changed','result_unknown'])expect(readDesktopToolResult({status:'ok',connection:{...current,renewal_reason}},scope)).toEqual({status:'ok',connection:{...current,renewal_reason}});
 for(const renewal_reason of ['future_reason','secret=value','execution_pending\n',null,{}])expect(readDesktopToolResult({status:'ok',connection:{...current,renewal_reason}},scope)).toBeUndefined();
 expect(readDesktopToolResult({status:'ok',connection},scope)?.status).toBe('ok');
 const missing=readDesktopToolResult({status:'ok',connection},scope);if(missing?.status==='ok'){expect(missing.connection).not.toHaveProperty('credential_expires_at');expect(missing.connection).not.toHaveProperty('credential_version');}
 for(const value of [{...current,credential_version:Number.MAX_SAFE_INTEGER},{...current,phase:'stopped'},{...current,phase:'connected'},{...connection,credential_version:1},{...connection,credential_expires_at:current.credential_expires_at}])expect(readDesktopToolResult({status:'ok',connection:value},scope)).toBeDefined();
 for(const patch of [{credential_version:0},{credential_version:-1},{credential_version:1.5},{credential_version:Number.MAX_SAFE_INTEGER+1},{credential_version:'2'},{credential_expires_at:null},{credential_expires_at:'2026-02-30T00:00:00Z'},{credential_expires_at:'2026-01-01T00:00:00+00:00'},{credential_expires_at:'tomorrow'},{credential_sha256:'a'.repeat(64)},{project_id:'5'.repeat(32)}])expect(readDesktopToolResult({status:'ok',connection:{...current,...patch}},scope)).toBeUndefined();
});
it('requires all OpenCode original configuration facts on connected and rejects private setup metadata',()=>{
 const openScope={...scope,adapterID:'opencode-sdk'},native={...connection,adapter_id:openScope.adapterID,model:'vendor/model',tool_version:'1.19.4',executable_sha256:'a'.repeat(64),configuration_sha256:'b'.repeat(64),provider_id:'provider',endpoint_origin:'https://provider.example'};
 expect(validDesktopToolScope(openScope)).toBe(true);expect(readDesktopToolResult({status:'ok',connection:native},openScope)).toEqual({status:'ok',connection:native});
 for(const key of ['model','tool_version','executable_sha256','configuration_sha256','provider_id','endpoint_origin']){const missing:Record<string,unknown>={...native};delete missing[key];expect(readDesktopToolResult({status:'ok',connection:missing},openScope)).toBeUndefined();}
 for(const patch of [{tool_version:'1.19.5'},{model:'bad model'},{provider_id:'bad provider'},{endpoint_origin:'https://provider.example/v1'},{base_url:'https://provider.example/v1'},{api_key_env_ref:'PRIVATE_KEY'},{catalog_sha256:'c'.repeat(64)}])expect(readDesktopToolResult({status:'ok',connection:{...native,...patch}},openScope)).toBeUndefined();
 expect(readDesktopToolResult({status:'ok',connection:{project_id:scope.project,adapter_id:'opencode-sdk',phase:'unconfigured'}},openScope)).toBeDefined();
});
it('accepts only the supported native function and exact current identity scope',()=>{
 const manageLocalTool=async()=>({});
 expect(desktopToolBridge({platform:'win32',version:'1.0.0',manageLocalTool})).toBeDefined();
 for(const value of [{platform:'web',version:'1.0.0',manageLocalTool},{platform:'linux',version:'2.0.0',manageLocalTool},{platform:'linux',version:'1.0.0',manageLocalTool:true}])expect(desktopToolBridge(value)).toBeUndefined();
 expect(validDesktopToolScope(scope)).toBe(true);
 for(const change of [{project:'p'},{userID:'A'.repeat(32)},{organizationID:''},{adapterID:'planned-tool'}])expect(validDesktopToolScope({...scope,...change})).toBe(false);
});
it('rejects leaked fields and mismatched connection facts before presenting a node',()=>{
 expect(readDesktopToolResult({status:'ok',connection},scope)).toEqual({status:'ok',connection});
 for(const change of [{project_id:'5'.repeat(32)},{adapter_id:'codebuddy-cli'},{phase:'healthy'},{worker_id:undefined},{base_commit:'main'},{expires_at:'tomorrow'},{repository_name:'secret\nline'},{token:'secret'},{path:'D:/private'},{command:['run']}])expect(readDesktopToolResult({status:'ok',connection:{...connection,...change}},scope)).toBeUndefined();
 expect(readDesktopToolResult({status:'ok',connection,token:'secret'},scope)).toBeUndefined();
 expect(readDesktopToolResult({status:'failed',code:'authorization_required'},scope)).toEqual({status:'failed',code:'authorization_required'});
 expect(readDesktopToolResult({status:'failed',code:'failed',environment:{}},scope)).toBeUndefined();
});
it('keeps Qoder fixed configuration facts scoped to the ACP connection',()=>{
 const qoderScope={...scope,adapterID:'qoder-acp'},qoderConnection={...connection,adapter_id:'qoder-acp',model:'model:一',tool_version:'Qoder 1.2.3',executable_sha256:'a'.repeat(64),configuration_sha256:'b'.repeat(64)};
 expect(validDesktopToolScope(qoderScope)).toBe(true);
 expect(readDesktopToolResult({status:'ok',connection:qoderConnection},qoderScope)).toEqual({status:'ok',connection:qoderConnection});
 for(const patch of [{model:'bad\nmodel'},{model:'x'.repeat(257)},{tool_version:''},{executable_sha256:'bad'},{configuration_sha256:'A'.repeat(64)},{acp_selection:{installation_id:'private'}}])expect(readDesktopToolResult({status:'ok',connection:{...qoderConnection,...patch}},qoderScope)).toBeUndefined();
 expect(readDesktopToolResult({status:'ok',connection:{...qoderConnection,adapter_id:scope.adapterID}},scope)).toBeUndefined();
});
it('requires complete Cursor ACP connection proof and keeps the exact tool identity separate from Qoder and Codex',()=>{
 const cursorScope={...scope,adapterID:'cursor-cli'},cursor={...connection,adapter_id:'cursor-cli',model:'cursor-observed-model',tool_version:'Cursor Agent fixture',executable_sha256:'a'.repeat(64),configuration_sha256:'b'.repeat(64)};
 expect(validDesktopToolScope(cursorScope)).toBe(true);expect(readDesktopToolResult({status:'ok',connection:cursor},cursorScope)).toEqual({status:'ok',connection:cursor});
 for(const key of ['model','tool_version','executable_sha256','configuration_sha256']){const missing:Record<string,unknown>={...cursor};delete missing[key];expect(readDesktopToolResult({status:'ok',connection:missing},cursorScope)).toBeUndefined();}
 for(const patch of [{adapter_id:'qoder-acp'},{adapter_id:'codex-app-server'},{catalog_sha256:'c'.repeat(64)},{acp_configuration:{}},{credential_env_ref:'CURSOR_PRIVATE_KEY'},{model:'bad\nmodel'}])expect(readDesktopToolResult({status:'ok',connection:{...cursor,...patch}},cursorScope)).toBeUndefined();
 expect(readDesktopToolResult({status:'ok',connection:{project_id:scope.project,adapter_id:'cursor-cli',phase:'unconfigured'}},cursorScope)).toBeDefined();
 expect(readDesktopToolResult({status:'ok',connection:cursor},{...cursorScope,adapterID:'qoder-acp'})).toBeUndefined();
});
it('requires complete native Codex connection proof without inventing a fixed task model',()=>{
 const codexScope={...scope,adapterID:'codex-app-server'},native={...connection,adapter_id:codexScope.adapterID,tool_version:'codex-cli 0.134.0',executable_sha256:'a'.repeat(64),configuration_sha256:'b'.repeat(64),catalog_sha256:'c'.repeat(64)};
 expect(validDesktopToolScope(codexScope)).toBe(true);expect(readDesktopToolResult({status:'ok',connection:native},codexScope)).toEqual({status:'ok',connection:native});
 for(const key of ['tool_version','executable_sha256','configuration_sha256','catalog_sha256']){const missing:Record<string,unknown>={...native};delete missing[key];expect(readDesktopToolResult({status:'ok',connection:missing},codexScope)).toBeUndefined();}
 for(const patch of [{model:'fixed-by-connect'},{configuration_sha256:'bad'},{catalog_sha256:'bad'},{codex_configuration:{}},{codex_selection:{}}])expect(readDesktopToolResult({status:'ok',connection:{...native,...patch}},codexScope)).toBeUndefined();
 expect(readDesktopToolResult({status:'ok',connection:{project_id:scope.project,adapter_id:codexScope.adapterID,phase:'unconfigured'}},codexScope)).toBeDefined();
 expect(readDesktopToolResult({status:'ok',connection:{...connection,catalog_sha256:native.catalog_sha256}},scope)).toBeUndefined();
});
