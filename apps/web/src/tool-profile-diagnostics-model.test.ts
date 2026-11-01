import {expect,it} from 'vitest';
import {isToolDoctorAction,readToolProfileDiagnostics} from './tool-profile-diagnostics-model';

const expected={project:'project-one',toolID:'tool-one'};
const profile={command_id:'codex-team',adapter:'codex',protocol:'codex-jsonl-v1',profile_sha256:'a'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'1.0',read_only:false};
const tool={id:expected.toolID,project_id:expected.project,worker_id:'worker-one',name:'Team Codex',version:3,state:'paused',profile,enabled_by:'',created_at:'2026-11-01T00:00:00Z',updated_at:'2026-11-01T00:00:00Z',available:false,reason:'Profile paused'};
const report=()=>({tool:{...tool},checked_at:'2026-11-01T00:00:00Z',expires_at:'2026-11-01T00:01:00Z',checks:['authorization','worker','inventory','profile'].map((code,index)=>({code,status:index===0?'blocked':'pass',reason:index===0?'Profile paused':'Checked',next_action:index===0?'review_profile':'none'})),next_action:'review_profile'});

it('T195 preserves optional exact maintenance states in the diagnostic tool and rejects null or forged capability facts',()=>{
 const value=report(),checked_at=value.checked_at;
 const supported={state:'supported',checked_at,protocol:'suma-tool-maintenance-v1',instance_id:'maintenance-instance',observed_at:'2026-10-31T23:59:30Z',expires_at:'2026-11-01T00:00:30Z'};
 const expired={...supported,state:'expired',observed_at:'2026-10-31T23:59:00Z',expires_at:checked_at};
 const unsupported={...expired,state:'unsupported',protocol:'suma-tool-maintenance-v2'};
 const unknown={state:'unknown',checked_at};
 expect(readToolProfileDiagnostics(value,expected).report.tool).not.toHaveProperty('maintenance');
 for(const maintenance of [supported,expired,unsupported,unknown]){
  const result=readToolProfileDiagnostics({...value,tool:{...tool,maintenance}},expected);
  expect(result.report.tool).toHaveProperty('maintenance',maintenance);expect(result.remainingMs).toBe(60000);
 }
 for(const maintenance of [null,{...unknown,protocol:supported.protocol},{...supported,checked_at:undefined},{...supported,expires_at:checked_at},{...expired,expires_at:supported.expires_at},{...unsupported,protocol:supported.protocol},{...supported,can_repair:true}])expect(()=>readToolProfileDiagnostics({...value,tool:{...tool,maintenance}},expected)).toThrow();
});

it('reads the exact four current facts without deriving eligibility or using the local clock',()=>{
 const value=report();expect(readToolProfileDiagnostics(value,expected)).toEqual({report:value,remainingMs:60000});
 expect(readToolProfileDiagnostics({...value,tool:{...tool,version:4}},expected).report.tool.version).toBe(4);
 expect(readToolProfileDiagnostics({...value,expires_at:value.checked_at},expected).remainingMs).toBe(0);
 const partial={...value,expires_at:'2026-11-01T00:00:12.125Z',checks:value.checks.map(check=>({...check,status:'not_checked',next_action:'refresh'})),next_action:'refresh'};
 expect(readToolProfileDiagnostics(partial,expected).remainingMs).toBe(12125);
 for(const action of ['none','review_profile','open_worker','register_profile','refresh'])expect(isToolDoctorAction(action)).toBe(true);
 expect(isToolDoctorAction('enable')).toBe(false);
 const impact={scope:'active_project_runs',observed_at:value.checked_at,coverage:'complete',runs_examined:1,nodes_examined:1,observed_reference_count:1,items:[{run_id:'run-a',node_id:'node-a',run_state:'running',node_state:'pending',worker_id:'',attempt:0,fence:0}],items_truncated:false,reason:''};
 expect(readToolProfileDiagnostics({...value,impact},expected).report).toHaveProperty('impact',impact);
 for(const invalid of [null,{...impact,coverage:'unknown',reason:''},{...impact,observed_reference_count:0},{...impact,items:[{...impact.items[0],node_state:'failed'}]},{...impact,token:'private'}])expect(()=>readToolProfileDiagnostics({...value,impact:invalid},expected)).toThrow();
});
it('rejects cross-scope, private payloads and incomplete or reordered diagnostic collections',()=>{
 const value=report();
 for(const patch of [{tool:{...tool,id:'other'}},{tool:{...tool,project_id:'other'}},{tool:{...tool,token:'secret'}},{tool:{...tool,profile:{...profile,api_key:'secret'}}},{tool:{...tool,version:0}},{tool:{...tool,available:'true'}},{checks:value.checks.slice(1)},{checks:[...value.checks].reverse()},{checks:[...value.checks,value.checks[0]]},{checks:value.checks.map(check=>({...check,action:'enable'}))},{checks:value.checks.map(check=>({...check,status:'healthy'}))},{next_action:'execute'},{actions:['enable']},{checked_at:'2026-11-01T00:00:00+00:00'},{checked_at:'2026-02-30T00:00:00Z'},{expires_at:'2026-11-01T00:01:00.001Z'},{expires_at:'2026-10-31T23:59:59Z'}])expect(()=>readToolProfileDiagnostics({...value,...patch},expected)).toThrow();
});
it('preserves closed optional display diagnostics and historical tool observations without converting them to actions',()=>{
 const value=report(),diagnostic={schema_version:1,message_key:'display.tool.profile_paused'};
 value.checks[0]={...value.checks[0],...{reason_diagnostic:diagnostic}};
 const enriched={...value,tool:{...tool,reason_diagnostic:diagnostic,last_seen_at:'2026-10-31T23:59:00Z',observation:{profile,available:true,reason:''}}};
 expect(readToolProfileDiagnostics(enriched,expected).report).toEqual(enriched);
 for(const invalid of [{...diagnostic,token:'secret'},{...diagnostic,schema_version:2},{...diagnostic,message_params:{nested:{secret:'value'}}}])expect(()=>readToolProfileDiagnostics({...value,checks:value.checks.map(check=>({...check,reason_diagnostic:invalid}))},expected)).toThrow();
 expect(()=>readToolProfileDiagnostics({...enriched,tool:{...enriched.tool,observation:{...enriched.tool.observation,secret:'value'}}},expected)).toThrow();
});
