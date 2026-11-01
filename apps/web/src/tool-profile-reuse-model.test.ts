import {expect,it} from 'vitest';
import type {ToolProfileMatchRequest} from '../../../packages/contracts/src';
import {readToolProfileMatches,sameToolProfile,toolProfileMatchKey} from './tool-profile-reuse-model';
import {readToolMaintenanceStatus} from './tool-maintenance-model';
const profile={command_id:'codex-team',adapter:'codex',protocol:'codex-jsonl-v1',profile_sha256:'a'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'1.0',read_only:false};
const request:ToolProfileMatchRequest={project_id:'1'.repeat(32),worker_id:'desktop-'+'2'.repeat(32),profile};
const item=(id='profile-one',state='paused')=>({id,project_id:request.project_id,worker_id:request.worker_id,name:'Team tool',version:1,state,profile:{...profile},enabled_by:'',created_at:'2026-11-01T00:00:00Z',updated_at:'2026-11-01T00:00:00Z',available:false,reason:'Paused',last_seen_at:'2026-11-01T00:00:00Z',observation:{profile:{...profile},available:true,reason:''},reason_diagnostic:{schema_version:1,message_key:'diagnostic.profile_paused',message_params:{count:1}}});
const result=()=>({...request,items:[item()],total:1});
it('reads complete exact profile matches retaining state and observation as separate facts',()=>{
 const value={...request,items:[item('one','paused'),item('two','enabled'),item('three','revoked')],total:3};
 expect(readToolProfileMatches(value,request)).toEqual(value);
 expect(readToolProfileMatches({...request,items:[],total:0},request)).toEqual({...request,items:[],total:0});
 expect(sameToolProfile(profile,Object.fromEntries(Object.entries(profile).reverse()))).toBe(true);
 expect(toolProfileMatchKey(request)).toBe(toolProfileMatchKey({profile:request.profile,worker_id:request.worker_id,project_id:request.project_id}));
 expect(sameToolProfile(profile,{...profile,read_only:true})).toBe(false);
 const planned={...profile,adapter:'suma',protocol:'suma-jsonl-v1',read_only:true,planning:{protocol:'suma-schedule-plan-v1',model:'first',endpoint_origin:'https://provider.example'}};
 expect(sameToolProfile(planned,{...planned,planning:{...planned.planning}})).toBe(true);
 expect(sameToolProfile(planned,{...planned,planning:{...planned.planning,model:'second'}})).toBe(false);
});
it('rejects incomplete collections and cross-source matches even when profile digests are identical',()=>{
 const value=result();
 for(const patch of [{project_id:'other'},{worker_id:'other'},{profile:{...profile,read_only:true}},{total:0},{total:1.5},{next_cursor:'next'},{truncated:false},{token:'secret'},{items:[item(),item()],total:2},{items:Array.from({length:1001},(_,i)=>item('p'+i)),total:1001}])expect(()=>readToolProfileMatches({...value,...patch},request)).toThrow();
 for(const patch of [{project_id:'other'},{worker_id:'other'},{profile:{...profile,read_only:true}},{profile:{...profile,secret:'private'}}])expect(()=>readToolProfileMatches({...value,items:[{...item(),...patch}]},request)).toThrow();
});
it('rejects malformed public records and private fields without dropping valid read metadata',()=>{
 const value=result();
 for(const patch of [{id:''},{id:'x'.repeat(201)},{version:0},{version:Number.MAX_SAFE_INTEGER+1},{state:'healthy'},{name:5},{available:'true'},{reason:5},{token:'secret'},{path:'private'},{reason_diagnostic:{schema_version:1,message_key:'x',token:'secret'}},{observation:{...item().observation,secret:'value'}}])expect(()=>readToolProfileMatches({...value,items:[{...item(),...patch}]},request)).toThrow();
 expect(readToolProfileMatches({...value,items:[{...item(),reason_diagnostic:null,observation:null}]},request)).toBeDefined();
 for(const key of ['id','enabled_by','created_at','updated_at','profile','state']){const missing:Record<string,unknown>={...item()};delete missing[key];expect(()=>readToolProfileMatches({...value,items:[missing]},request)).toThrow();}
});

it('T195 retains exact matches with real maintenance states without treating capability as availability or accepting malformed declarations',()=>{
 const declared={checked_at:'2026-11-01T00:00:20Z',protocol:'suma-tool-maintenance-v1',instance_id:'continuous-instance',observed_at:'2026-11-01T00:00:00Z',expires_at:'2026-11-01T00:01:00Z'};
 const states=[{state:'unknown',checked_at:declared.checked_at},{state:'supported',...declared},{state:'expired',...declared,checked_at:declared.expires_at},{state:'unsupported',...declared,protocol:'suma-tool-maintenance-v2',checked_at:declared.expires_at}];
 for(const maintenance of states){const value={...request,items:[{...item(),maintenance}],total:1};const read=readToolProfileMatches(value,request);expect(read).toEqual(value);expect(read.items[0].available).toBe(false);expect(sameToolProfile(read.items[0].profile,profile)).toBe(true);}
 expect(readToolMaintenanceStatus(states[1])).toEqual({status:states[1],remainingMs:40000});for(const state of [states[0],states[2],states[3]])expect(readToolMaintenanceStatus(state)?.remainingMs).toBe(0);
 for(const maintenance of [null,{},'supported',{state:'healthy'},{state:'unknown'},{state:'supported',...declared,checked_at:undefined},{state:'supported',...declared,protocol:'suma-tool-maintenance-v2'},{state:'unsupported',...declared},{state:'unknown',...declared},{state:'supported',...declared,instance_id:''},{state:'supported',...declared,expires_at:undefined},{state:'supported',...declared,expires_at:declared.observed_at},{state:'supported',...declared,checked_at:declared.expires_at},{state:'expired',...declared},{state:'supported',...declared,checked_at:'2026-10-31T23:59:59Z'},{state:'supported',...declared,observed_at:'2026-02-30T00:00:00Z'},{state:'supported',...declared,token:'private'}])expect(()=>readToolProfileMatches({...result(),items:[{...item(),maintenance}]},request)).toThrow();
 expect(()=>readToolProfileMatches({...result(),items:[{...item(),maintenance:states[0],unexpected:true}]},request)).toThrow();
});
