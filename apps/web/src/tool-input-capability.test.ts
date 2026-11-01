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
});
