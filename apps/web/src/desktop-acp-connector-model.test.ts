import {expect,it} from 'vitest';
import {desktopACPBridge,desktopACPRecoveryScope,makeDesktopACPRequest,readDesktopACPResult,validDesktopACPRequest,validDesktopACPScope} from './desktop-acp-connector-model';

const scope={project:'1'.repeat(32),userID:'2'.repeat(32),organizationID:'3'.repeat(32),adapterID:'qoder-acp'};
const installation='4'.repeat(32),operation='5'.repeat(32),probe='6'.repeat(32),sha='a'.repeat(64);
function result(){const now=Date.now();return {status:'ok',observation:{project_id:scope.project,user_id:scope.userID,tenant_id:scope.organizationID,adapter_id:'qoder-acp',phase:'ready',installations:[{installation_id:installation,label:'Qoder',kind:'native',tool_version:'Qoder 1.2.3',executable_sha256:sha}],probe:{id:probe,operation_id:operation,installation_id:installation,source_sha256:'b'.repeat(64),created_at:new Date(now-1000).toISOString(),expires_at:new Date(now+59000).toISOString(),discovery:{protocol:'suma-acp-model-discovery-v1',runtime:'qoder',state:'ready',executable_sha256:sha,tool_version:'Qoder 1.2.3',catalogue:{source:'acp_session_config',protocol_version:1,config_id:'model',current_value:'',models:[{id:'model:一',name:'Model',configuration_sha256:'c'.repeat(64)}]},evidence:{session_created:true,session_cancel_requested:false,session_cancel_written:false,prompt_requests:0,configuration_writes:0,permission_requests:0,unexpected_activity:0,host_process_stopped:true,host_process_tree_stopped:true,host_process_exit_code:0,host_stopped_after_rpc:false}}}}};}
it('makes only explicit scoped ACP observations and preserves the original recovery operation',()=>{
 expect(validDesktopACPScope(scope)).toBe(true);
 expect(desktopACPBridge({platform:'win32',version:'1.0.0',inspectLocalACP:async()=>({})})).toBeDefined();
 expect(desktopACPBridge({platform:'web',version:'1.0.0',inspectLocalACP:async()=>({})})).toBeUndefined();
 const request=makeDesktopACPRequest('models',scope,{installation_id:installation,operation_id:operation,locale:'en-US'});
 expect(request).toEqual({project_id:scope.project,user_id:scope.userID,tenant_id:scope.organizationID,adapter_id:'qoder-acp',locale:'en-US',action:'models',installation_id:installation,operation_id:operation});
 expect(validDesktopACPRequest(request,scope)).toBe(true);
 expect(desktopACPRecoveryScope(scope)).not.toBe(desktopACPRecoveryScope({...scope,userID:'7'.repeat(32)}));
 expect(makeDesktopACPRequest('status',scope,{operation_id:operation,locale:'en-US'})).not.toHaveProperty('installation_id');
 for(const patch of [{project_id:'7'.repeat(32)},{adapter_id:'claude-code-cli'},{executable:'qoder'},{credential_env_ref:'PATH'},{credential_env_ref:'SUMA_TOKEN'},{operation_id:'old'}])expect(validDesktopACPRequest({...request,...patch},scope)).toBe(false);
 const reservedPrefixes=['SUMA_','CODEX','OPENAI','ANTHROPIC','CLAUDE','AZURE_','GEMINI','GOOGLE_','QWEN','DASHSCOPE','DEEPSEEK','OPENCODE','DSH_','WORKBUDDY','CODEBUDDY','AWS_','HUGGINGFACE','HF_','GIT_','NODE_','PYTHON'];
 const reservedNames=['HOME','USERPROFILE','PATH','APPDATA','LOCALAPPDATA','TEMP','TMP','TMPDIR','COMSPEC','SYSTEMROOT','WINDIR','PATHEXT','HOMEDRIVE','HOMEPATH','SYSTEMDRIVE','PROGRAMDATA','PROGRAMFILES','PUBLIC','LANG','GOFLAGS','GOCACHE','GOTMPDIR'];
 for(const ref of [...reservedPrefixes.map(prefix=>prefix+'CREDENTIAL'),...reservedNames])expect(validDesktopACPRequest({...request,credential_env_ref:ref},scope),ref).toBe(false);
 expect(validDesktopACPRequest({...request,credential_env_ref:'MY_QODER_CREDENTIAL'},scope)).toBe(true);
 expect(validDesktopACPRequest({...request,action:'status'},scope)).toBe(false);
 const cursorScope={...scope,adapterID:'cursor-cli'};expect(validDesktopACPScope(cursorScope)).toBe(true);const cursorRequest={...request,adapter_id:'cursor-cli',credential_env_ref:'CURSOR_API_KEY'};expect(validDesktopACPRequest(cursorRequest,cursorScope)).toBe(true);expect(makeDesktopACPRequest('models',cursorScope,{installation_id:installation,operation_id:operation,locale:'en-US',credential_env_ref:'CURSOR_API_KEY'})).toEqual(cursorRequest);expect(desktopACPRecoveryScope(cursorScope)).not.toBe(desktopACPRecoveryScope(scope));
 for(const credential_env_ref of [undefined,'','PATH','SUMA_TOKEN','OPENAI_API_KEY'])expect(validDesktopACPRequest({...cursorRequest,credential_env_ref},cursorScope)).toBe(false);const {credential_env_ref:_,...withoutCredential}=cursorRequest;expect(validDesktopACPRequest(withoutCredential,cursorScope)).toBe(false);expect(validDesktopACPRequest(cursorRequest,scope)).toBe(false);expect(validDesktopACPRequest(request,cursorScope)).toBe(false);expect(validDesktopACPScope({...scope,adapterID:'cursor-editor'})).toBe(false);
});
it('accepts only a current exact installation and original probe with bounded lifetime',()=>{
 const raw=result();expect(readDesktopACPResult(raw,scope,operation)).toEqual(raw);
 raw.observation.probe.discovery.catalogue.current_value='unlisted-current';expect(readDesktopACPResult(raw,scope,operation)).toEqual(raw);
 expect(readDesktopACPResult(raw,{...scope,userID:'7'.repeat(32)},operation)).toBeUndefined();
 expect(readDesktopACPResult(raw,scope,'7'.repeat(32))).toBeUndefined();
 for(const patch of [{installation_id:'7'.repeat(32)},{expires_at:new Date(Date.now()+120000).toISOString()},{expires_at:'2020-01-01T00:00:00Z'},{created_at:'2026-02-30T00:00:00Z'},{source_sha256:'bad'}])expect(readDesktopACPResult({...raw,observation:{...raw.observation,probe:{...raw.observation.probe,...patch}}},scope,operation)).toBeUndefined();
 const changed=result();changed.observation.installations[0].tool_version='different';expect(readDesktopACPResult(changed,scope,operation)).toBeUndefined();
 const cursorScope={...scope,adapterID:'cursor-cli'},base=result(),cursor={...base,observation:{...base.observation,adapter_id:'cursor-cli',probe:{...base.observation.probe,discovery:{...base.observation.probe.discovery,runtime:'cursor'}}}};expect(readDesktopACPResult(cursor,cursorScope,operation)).toEqual(cursor);expect(readDesktopACPResult(cursor,scope,operation)).toBeUndefined();expect(readDesktopACPResult({...cursor,observation:{...cursor.observation,probe:{...cursor.observation.probe,discovery:{...cursor.observation.probe.discovery,runtime:'qoder'}}}},cursorScope,operation)).toBeUndefined();expect(readDesktopACPResult({...base,observation:{...base.observation,probe:{...base.observation.probe,discovery:{...base.observation.probe.discovery,runtime:'cursor'}}}},scope,operation)).toBeUndefined();
});
it('requires stopped zero-prompt discovery and refuses credential or raw-session fields',()=>{
 for(const patch of [{prompt_requests:1},{configuration_writes:1},{permission_requests:1},{unexpected_activity:1},{session_created:false},{host_process_stopped:false},{host_process_tree_stopped:false},{session_cancel_requested:true}]){const raw=result();Object.assign(raw.observation.probe.discovery.evidence,patch);expect(readDesktopACPResult(raw,scope,operation)).toBeUndefined();}
 for(const patch of [{session_id:'private'},{executable:'D:/private/qoder.exe'},{token:'private'},{args:['--acp']}]){const raw=result();Object.assign(raw.observation.probe.discovery,patch);expect(readDesktopACPResult(raw,scope,operation)).toBeUndefined();}
 const malformed=result();malformed.observation.probe.discovery.catalogue.models.push({...malformed.observation.probe.discovery.catalogue.models[0]});expect(readDesktopACPResult(malformed,scope,operation)).toBeUndefined();
});
it('keeps unknown and expired observations honest without turning their catalogue into readiness',()=>{
 const raw=result();const {catalogue:_,...discovery}=raw.observation.probe.discovery;
 const unknown={...raw,observation:{...raw.observation,phase:'unknown',probe:{...raw.observation.probe,discovery:{...discovery,state:'unknown',error_code:'acp_discovery_unknown'}}}};
 expect(readDesktopACPResult(unknown,scope,operation)).toEqual(unknown);
 expect(readDesktopACPResult({...unknown,observation:{...unknown.observation,phase:'ready'}},scope,operation)).toBeUndefined();
 const expired={...raw,observation:{...raw.observation,phase:'expired',probe:{...raw.observation.probe,created_at:'2020-01-01T00:00:00Z',expires_at:'2020-01-01T00:01:00Z'}}};
 expect(readDesktopACPResult(expired,scope,operation)).toEqual(expired);
 expect(readDesktopACPResult({status:'failed',code:'authorization_required'},scope)).toEqual({status:'failed',code:'authorization_required'});
});
it('T191 preserves actual Cursor post-RPC host exit and refuses incomplete or foreign lifecycle evidence',()=>{
 const cursorScope={...scope,adapterID:'cursor-cli'};
 const current=()=>{const raw=result();raw.observation.adapter_id='cursor-cli';raw.observation.probe.discovery.runtime='cursor';Object.assign(raw.observation.probe.discovery.evidence,{session_cancel_requested:true,session_cancel_written:true,host_process_exit_code:1,host_stopped_after_rpc:true});return raw;};
 const raw=current();expect(readDesktopACPResult(raw,cursorScope,operation)).toEqual(raw);expect(raw.observation.probe.discovery.evidence.host_process_exit_code).toBe(1);
 for(const host_process_exit_code of [-1,0,4294967295]){const value=current();value.observation.probe.discovery.evidence.host_process_exit_code=host_process_exit_code;expect(readDesktopACPResult(value,cursorScope,operation)).toEqual(value);}
 for(const patch of [{host_stopped_after_rpc:false},{host_process_exit_code:undefined},{host_process_exit_code:null},{host_process_exit_code:-2},{host_process_exit_code:1.5},{host_process_exit_code:4294967296},{session_cancel_requested:false},{session_cancel_written:false},{session_created:false},{host_process_stopped:false},{host_process_tree_stopped:false},{prompt_requests:1},{configuration_writes:1},{permission_requests:1},{unexpected_activity:1},{session_close_acknowledged:true}]){const value=current();Object.assign(value.observation.probe.discovery.evidence,patch);expect(readDesktopACPResult(value,cursorScope,operation),JSON.stringify(patch)).toBeUndefined();}
 const missing=current();delete (missing.observation.probe.discovery.evidence as Record<string,unknown>).host_stopped_after_rpc;expect(readDesktopACPResult(missing,cursorScope,operation)).toBeUndefined();
 const qoder=current();qoder.observation.adapter_id='qoder-acp';qoder.observation.probe.discovery.runtime='qoder';expect(readDesktopACPResult(qoder,scope,operation)).toBeUndefined();
 const changed=current();changed.observation.probe.discovery.executable_sha256='f'.repeat(64);expect(readDesktopACPResult(changed,cursorScope,operation)).toBeUndefined();
 const natural=result();expect(readDesktopACPResult(natural,scope,operation)).toEqual(natural);delete (natural.observation.probe.discovery.evidence as Record<string,unknown>).host_process_exit_code;expect(readDesktopACPResult(natural,scope,operation)).toEqual(natural);
 const unstopped=result();unstopped.observation.phase='unknown';const {catalogue:_,...failed}=unstopped.observation.probe.discovery;const unknown={...unstopped,observation:{...unstopped.observation,probe:{...unstopped.observation.probe,discovery:{...failed,state:'unknown',error_code:'acp_discovery_unknown',evidence:{...failed.evidence,host_process_stopped:false,host_process_tree_stopped:false}}}}};expect(readDesktopACPResult(unknown,scope,operation)).toBeUndefined();
 const uncertain=current(),{catalogue:__,...partial}=uncertain.observation.probe.discovery;const history={...uncertain,observation:{...uncertain.observation,phase:'unknown',probe:{...uncertain.observation.probe,discovery:{...partial,state:'unknown',error_code:'acp_discovery_unknown',evidence:{...partial.evidence,host_stopped_after_rpc:false,permission_requests:4096}}}}};expect(readDesktopACPResult(history,cursorScope,operation)).toEqual(history);expect(readDesktopACPResult({...history,observation:{...history.observation,phase:'ready'}},cursorScope,operation)).toBeUndefined();
});
