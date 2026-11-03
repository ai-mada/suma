import {expect,it} from 'vitest';
import {toolConnectObservationMatches,validToolConnectObservationPin} from './tool-connect-observation-model';
import type {ToolObservation} from './tool-model';

it('T224 pins the complete configuration and nanosecond observation without granting unavailable tools or renewing a deadline',()=>{
 const pin={profileSHA256:'a'.repeat(64),observedAt:'2026-11-03T00:00:00.000000002Z'},view={state:'fresh' as const,observedAt:'2026-11-03T00:00:00.000000003Z'},observation={available:true,reason:'',profile:{command_id:'same-command',adapter:'codex',protocol:'codex-jsonl-v1',profile_sha256:pin.profileSHA256}} as ToolObservation;
 expect(toolConnectObservationMatches(pin,view,observation)).toBe(true);
 expect(toolConnectObservationMatches(pin,{...view,observedAt:'2026-11-03T00:00:00.000000001Z'},observation)).toBe(false);
 expect(toolConnectObservationMatches(pin,view,{...observation,profile:{...observation.profile,profile_sha256:'b'.repeat(64)}})).toBe(false);
 expect(toolConnectObservationMatches(pin,{...view,state:'stale'},observation)).toBe(false);
 expect(toolConnectObservationMatches(pin,view,{...observation,available:false})).toBe(false);
 // The operation locator permits only a paused-save candidate. Quick still
 // re-reads its completed receipt; this pure predicate never grants authority.
 expect(toolConnectObservationMatches({...pin,preparationID:'preparation-one'},view,{...observation,available:false})).toBe(true);
 for(const invalid of [null,{...pin,deadline:Infinity},{...pin,observedAt:'bad'},{...pin,preparationID:null},{...pin,profileSHA256:pin.profileSHA256.toUpperCase()}])expect(validToolConnectObservationPin(invalid)).toBe(false);
});
