import {expect,it} from 'vitest';
import {parseToolDiscovery} from './tool-discovery-model';

const project='project-a',worker='desktop-'+'4'.repeat(32);
const item={profile:{command_id:'desktop-codebuddy-cli',adapter:'codebuddy',protocol:'codebuddy-jsonl-v1',profile_sha256:'a'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'1.0',read_only:false},available:true,reason:''};
const report={project_id:project,worker_id:worker,state:'fresh',as_of:'2026-10-30T07:34:52.895Z',observed_at:'2026-10-30T07:34:50.622Z',expires_at:'2026-10-30T07:35:50.622Z',items:[item]};

it('accepts the actual fractional inventory response while retaining strict UTC calendar validation and original timestamps',()=>{
 const parsed=parseToolDiscovery(report,project,worker);
 expect(parsed.remainingMs).toBe(57727);expect(parsed.items).toEqual([item]);expect(parsed.asOf).toBe('2026-10-30T07:34:52.895Z');expect(parsed.observedAt).toBe('2026-10-30T07:34:50.622Z');expect(parsed.expiresAt).toBe('2026-10-30T07:35:50.622Z');
 for(const suffix of ['Z','.1Z','.12Z','.123Z','.1234Z','.12345Z','.123456Z','.1234567Z','.12345678Z','.123456789Z']){
  expect(parseToolDiscovery({...report,as_of:'2026-10-30T08:00:20'+suffix,observed_at:'2026-10-30T08:00:00'+suffix,expires_at:'2026-10-30T08:01:00'+suffix},project,worker).remainingMs).toBe(40000);
 }
 for(const as_of of ['2026-02-30T07:34:52.895Z','2025-02-29T07:34:52.895Z','2026-10-30T24:00:00.000Z','2026-10-30T07:60:00.000Z','2026-10-30T07:34:60.000Z','2026-10-30T07:34:52.Z','2026-10-30T07:34:52.1234567890Z','2026-10-30T07:34:52.895+00:00'])expect(()=>parseToolDiscovery({...report,as_of},project,worker)).toThrow();
});

it('keeps nanosecond ordering and exactly sixty seconds even when Date milliseconds would collapse the boundary',()=>{
 const precise={...report,observed_at:'2026-10-30T08:00:00.000000001Z',as_of:'2026-10-30T08:00:59.999999999Z',expires_at:'2026-10-30T08:01:00.000000001Z'};
 const parsed=parseToolDiscovery(precise,project,worker);expect(parsed.remainingMs).toBe(0);expect(parsed.observedAt).toBe('2026-10-30T08:00:00.000000001Z');
 expect(()=>parseToolDiscovery({...precise,as_of:'2026-10-30T08:00:00.000000000Z'},project,worker)).toThrow();
 for(const expires_at of ['2026-10-30T08:01:00.000000000Z','2026-10-30T08:01:00.000000002Z'])expect(()=>parseToolDiscovery({...precise,expires_at},project,worker)).toThrow();
 expect(()=>parseToolDiscovery({...precise,as_of:'2026-10-30T08:01:00.000000001Z'},project,worker)).toThrow();
 expect(parseToolDiscovery({...precise,state:'stale',items:[],as_of:'2026-10-30T08:01:00.000000001Z'},project,worker).state).toBe('stale');
});
