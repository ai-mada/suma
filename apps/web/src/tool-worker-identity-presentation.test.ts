import {expect,it} from 'vitest';
import {classifyToolWorkerIdentity,projectToolWorkerIdentities,type ToolWorkerIdentitySource} from './tool-worker-identity-presentation';

const now=Date.parse('2026-11-03T08:00:00Z');
const worker=(worker_id:string,status='active',expires_at='2026-11-03T09:00:00Z'):ToolWorkerIdentitySource=>({worker_id,status,expires_at});

it('expires at the exact boundary and keeps a revoked credential revoked even with invalid time evidence',()=>{
 expect(classifyToolWorkerIdentity(worker('future'),now)).toBe('current');
 expect(classifyToolWorkerIdentity(worker('past','active','2026-11-03T07:59:59.999Z'),now)).toBe('expired');
 expect(classifyToolWorkerIdentity(worker('boundary','active','2026-11-03T08:00:00Z'),now)).toBe('expired');
 expect(classifyToolWorkerIdentity(worker('offset','active','2026-11-03T16:00:00+08:00'),now)).toBe('expired');
 expect(classifyToolWorkerIdentity(worker('revoked','revoked','not-a-time'),Number.NaN)).toBe('revoked');
});

it('never presents an unknown status, invalid calendar date or nonfinite clock as a current credential',()=>{
 expect(classifyToolWorkerIdentity(worker('unknown','pending'),now)).toBe('unknown');
 for(const expiry of ['','not-a-time','2026-02-30T08:00:00Z','2026-11-03'])expect(classifyToolWorkerIdentity(worker('invalid','active',expiry),now)).toBe('unknown');
 for(const clock of [Number.NaN,Number.POSITIVE_INFINITY,Number.NEGATIVE_INFINITY])expect(classifyToolWorkerIdentity(worker('clock'),clock)).toBe('unknown');
});

it('filters exact read records using only public worker IDs and preserves source objects and query-scoped counts',()=>{
 const current=Object.freeze({...worker('Team-Codex'),token:'private-needle',path:'C:/private-needle',id:'private-needle'});
 const expired=Object.freeze(worker('Team-Codex','active','2026-11-03T08:00:00Z'));
 const revoked=Object.freeze(worker('Team-Codex','revoked'));
 const unknown=Object.freeze(worker('team-codex-review','unknown'));
 const unrelated=Object.freeze(worker('other-worker'));
 const records=Object.freeze([current,expired,revoked,unknown,unrelated]),before=JSON.stringify(records);
 const view=projectToolWorkerIdentities(records,now,'all',' TEAM-CODEX ');
 expect(view.items).toEqual([current,expired,revoked,unknown]);
 expect(view.items[0]).toBe(current);expect(view.items[2]).toBe(revoked);
 expect(view.readCount).toBe(5);
 expect(view.counts).toEqual({current:1,expired:1,revoked:1,unknown:1,attention:2,all:4});
 expect(projectToolWorkerIdentities(records,now,'current','team-codex').items).toEqual([current]);
 expect(projectToolWorkerIdentities(records,now,'attention','team-codex').items).toEqual([expired,unknown]);
 expect(projectToolWorkerIdentities(records,now,'revoked','team-codex').items).toEqual([revoked]);
 expect(projectToolWorkerIdentities(records,now,'all','private-needle').items).toEqual([]);
 expect(projectToolWorkerIdentities(records,now).items).toEqual([current,unrelated]);
 expect(JSON.stringify(records)).toBe(before);
});
