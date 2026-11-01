import {expect,it} from 'vitest';
import {parseToolManifest} from './tool-model';
import {sameToolProfile} from './tool-profile-reuse-model';

it('accepts only explicit Codex app-server input metadata and preserves complete profile identity independently of availability',()=>{
 const original={command_id:'codex-inputs',adapter:'codex',protocol:'codex-app-server-v1',profile_sha256:'1'.repeat(64),executable_sha256:'2'.repeat(64),tool_version:'1.0',read_only:false},enabled={...original,user_input_protocol:'codex-user-input-v1'};
 const read=(profile:unknown,available=false)=>parseToolManifest(JSON.stringify([{profile,available,reason:'Catalog not observed'}]));
 expect(read(enabled)).toEqual([{profile:enabled,available:false,reason:'Catalog not observed'}]);expect(read({...original,user_input_protocol:''})[0].profile.user_input_protocol).toBe('');
 expect(sameToolProfile(enabled,enabled)).toBe(true);expect(sameToolProfile(enabled,original)).toBe(false);expect(sameToolProfile(enabled,{...original,user_input_protocol:''})).toBe(false);
 for(const profile of [{...enabled,user_input_protocol:'unknown-v1'},{...enabled,user_input_protocol:null},{...enabled,user_input_protocol:true},{...enabled,adapter:'command',protocol:'text-v1'},{...enabled,protocol:'codex-jsonl-v1'},{...enabled,read_only:true},{...enabled,configured_ready:true}])expect(()=>read(profile)).toThrow();
 expect(()=>read(enabled,true)).toThrow();
 const cursorCapability={runtime:'cursor',protocol_version:1,sdk_version:'0.13.5',configuration_sha256:'3'.repeat(64),model:'reviewed-cursor-model',model_source:'administrator_session_config',budget_mode:'observe_only',identity_evidence:'configuration_ack_only',auth_strategy:'explicit_api_key_env',permission_mode:'human_once'},cursor={...original,adapter:'cursor',protocol:'acp-stdio-v1',user_input_protocol:'cursor-acp-input-v1',acp_execution:cursorCapability};expect(read(cursor,true)[0].profile).toEqual(cursor);expect(sameToolProfile(cursor,{...cursor,user_input_protocol:''})).toBe(false);
 expect(read({...cursor,acp_execution:undefined},false)[0].profile.user_input_protocol).toBe('cursor-acp-input-v1');expect(()=>read({...cursor,acp_execution:undefined},true)).toThrow();
 for(const profile of [{...cursor,user_input_protocol:''},{...cursor,user_input_protocol:undefined},{...cursor,user_input_protocol:'codex-user-input-v1'},{...cursor,adapter:'qoder',acp_execution:{...cursorCapability,runtime:'qoder',auth_strategy:'existing_cli_login'}},{...cursor,acp_execution:{...cursorCapability,runtime:'qoder',auth_strategy:'existing_cli_login'}}])expect(()=>read(profile)).toThrow();
});
