import {expect,it} from 'vitest';
import type {WriteRecoveryRequest} from './write-action';
import {readRunnerPairing,readRunnerPairingPage,runnerPairingInboxPath,runnerPairingInboxSearch,runnerPairingCommand,validRunnerPairingReceipt,validRunnerPairingRequest} from './runner-pairing-model';

const project='a'.repeat(32),id='b'.repeat(32),sha='c'.repeat(64),worker='remote-'+'d'.repeat(32);
const waiting=()=>({id,project_id:project,version:1,state:'waiting',control_url:'https://control.example.test/suma',created_at:'2026-11-01T00:00:00Z',expires_at:'2026-11-01T00:10:00Z',can_approve:false});
const proposed=()=>({...waiting(),version:2,state:'proposed',can_approve:true,device_name:'Self-reported developer device',credential_sha256:sha});
const approved=()=>({...proposed(),version:3,state:'approved',can_approve:false,worker_id:worker,credential_expires_at:'2026-11-02T00:01:00Z'});
function create():WriteRecoveryRequest{return {path:'/runner-pairings',method:'POST',idempotency_key:'create-original',body:{project_id:project,idempotency_key:'create-original'}};}
function command(action:'approve'|'reject'='approve',version=2):WriteRecoveryRequest{return {path:'/runner-pairings/'+id+'/commands',method:'POST',idempotency_key:'decision-original',body:{project_id:project,expected_version:version,idempotency_key:'decision-original',action}};}

it('reads only the closed non-secret current pairing shape with exact scope and independent state qualification',()=>{
 expect(readRunnerPairing(waiting(),project)).toEqual(waiting());expect(readRunnerPairing(proposed(),project)).toEqual(proposed());expect(readRunnerPairing({...proposed(),can_approve:false},project).can_approve).toBe(false);expect(readRunnerPairing(approved(),project)).toEqual(approved());
 for(const [reason_code,value] of [['awaiting_machine',waiting()],['approval_required',proposed()],['control_changed',{...proposed(),can_approve:false}],['identity_approved',approved()],['pairing_rejected',{...proposed(),state:'rejected',can_approve:false}],['pairing_expired',{...proposed(),state:'expired',can_approve:false}]] as const)expect(readRunnerPairing({...value,reason_code},project).reason_code).toBe(reason_code);
 expect(()=>readRunnerPairing({...proposed(),reason_code:'healthy'},project)).toThrow();
 const item={...proposed(),reason_code:'approval_required'};
 expect(readRunnerPairingPage({project_id:project,status:'active',q:'',items:[item],total:1,limit:20,as_of:'2026-11-01T00:00:00Z'},{project,status:'active',q:'',limit:20}).items).toEqual([item]);
 for(const project_id of ['p','project-a','项'.repeat(21)])expect(readRunnerPairing({...waiting(),project_id},project_id).project_id).toBe(project_id);
 for(const project_id of ['a'.repeat(65),'项'.repeat(22),'has space','slash/project','back\\project','delete\u007f','bad\ud800'])expect(()=>readRunnerPairing({...waiting(),project_id},project_id)).toThrow();
 for(const device_name of ['a'.repeat(100),'项'.repeat(33)])expect(readRunnerPairing({...proposed(),device_name},project).device_name).toBe(device_name);
 for(const device_name of ['a'.repeat(101),'项'.repeat(34)])expect(()=>readRunnerPairing({...proposed(),device_name},project)).toThrow();
 for(const state of ['rejected','expired'])for(const base of [waiting(),proposed()])expect(readRunnerPairing({...base,state,can_approve:false},project).state).toBe(state);
 const invalid=[{...waiting(),project_id:'e'.repeat(32)},{...waiting(),id:id.toUpperCase()},{...waiting(),version:0},{...waiting(),state:'connected'},{...waiting(),can_approve:true},{...waiting(),device_name:'Extra device',credential_sha256:sha},{...proposed(),credential_sha256:undefined},{...proposed(),device_name:''},{...proposed(),credential_sha256:'not-a-digest'},{...proposed(),worker_id:worker},{...approved(),can_approve:true},{...approved(),credential_expires_at:undefined},{...approved(),worker_id:'pool-'+id},{...waiting(),token:'never-public'},{...waiting(),secret:'never-public'},{...waiting(),checked_at:'2026-11-01T00:00:00Z'},{...waiting(),created_at:'2026-02-30T00:00:00Z'},{...waiting(),expires_at:'not-a-date'}];
 for(const value of invalid)expect(()=>readRunnerPairing(value,project)).toThrow();
});

it('validates a live pairing page against its exact query and treats UTF-8 cursors as opaque data',()=>{
 const query={project,status:'active' as const,q:'项目',limit:2},cursor='next:opaque/中文%&=';
 const page={project_id:project,status:'active',q:query.q,items:[waiting(),{...proposed(),id:'e'.repeat(32)}],total:7,limit:2,next_cursor:cursor,as_of:'2026-11-01T00:00:00.125Z'};
 expect(readRunnerPairingPage(page,query)).toEqual(page);
 expect(runnerPairingInboxSearch('\u0085 \t项目\n')).toBe('项目');expect(runnerPairingInboxSearch('😀'.repeat(50))).toBe('😀'.repeat(50));expect(runnerPairingInboxSearch('😀'.repeat(51))).toBeUndefined();expect(runnerPairingInboxSearch('bad\ud800')).toBeUndefined();
 const path=runnerPairingInboxPath({...query,cursor}),parameters=new URL('https://local.test'+path).searchParams;
 expect(parameters.get('q')).toBe(query.q);expect(parameters.get('cursor')).toBe(cursor);expect(parameters.get('limit')).toBe('2');expect(parameters.get('status')).toBe('active');
 const empty:Record<string,unknown>={...page,items:[],total:0};delete empty.next_cursor;expect(readRunnerPairingPage(empty,query).items).toEqual([]);
 expect(readRunnerPairingPage({...page,next_cursor:'界'.repeat(1365)},query).next_cursor).toHaveLength(1365);
 for(const patch of [{project_id:'another-project'},{status:'all'},{q:'other'},{limit:3},{limit:0},{total:1},{total:-1},{total:Number.MAX_SAFE_INTEGER+1},{items:[waiting(),waiting()]},{items:[...page.items,approved()]},{items:[approved()]},{next_cursor:''},{next_cursor:'界'.repeat(1366)},{next_cursor:123},{as_of:'2026-02-30T00:00:00Z'},{as_of:'2026-11-01T00:00:00+00:00'},{token:'secret'}])expect(()=>readRunnerPairingPage({...page,...patch},query)).toThrow();
 expect(()=>readRunnerPairingPage({...page,next_cursor:cursor},{...query,cursor})).toThrow();
 expect(()=>runnerPairingInboxPath({...query,q:'项'.repeat(67)})).toThrow();expect(()=>runnerPairingInboxPath({...query,q:' 项目 '})).toThrow();expect(()=>runnerPairingInboxPath({...query,limit:101})).toThrow();
});

it('recovers only the original create or exact CAS decision with its closed request and expected result',()=>{
 expect(validRunnerPairingRequest(create(),project)).toBe(true);expect(validRunnerPairingRequest(command(),project)).toBe(true);expect(validRunnerPairingReceipt(waiting(),create(),project)).toBe(true);expect(validRunnerPairingReceipt(approved(),command(),project)).toBe(true);
 for(const project_id of ['p','project-a']){const original=create(),request={...original,body:{...original.body,project_id}};expect(validRunnerPairingRequest(request,project_id)).toBe(true);expect(validRunnerPairingReceipt({...waiting(),project_id},request,project_id)).toBe(true);}
 for(const project_id of ['a'.repeat(65),'项'.repeat(22),'with space']){const original=create();expect(validRunnerPairingRequest({...original,body:{...original.body,project_id}},project_id)).toBe(false);}
 expect(validRunnerPairingReceipt({...proposed(),version:3,state:'rejected',can_approve:false},command('reject'),project)).toBe(true);
 expect(validRunnerPairingReceipt({...waiting(),version:2,state:'rejected'},command('reject',1),project)).toBe(true);
 for(const value of [proposed(),approved()])expect(validRunnerPairingReceipt(value,create(),project)).toBe(false);
 for(const value of [{...approved(),id:'e'.repeat(32)},{...approved(),version:2},{...approved(),version:4},{...proposed(),version:3},{...approved(),state:'rejected',worker_id:undefined,credential_expires_at:undefined}])expect(validRunnerPairingReceipt(value,command(),project)).toBe(false);
 const base=create();for(const change of [{path:'/runner-pairings/'+id+'/proposal'},{method:'GET'},{body:{...base.body,worker_id:worker}},{body:{...base.body,token:'secret'}},{body:{...base.body,idempotency_key:'different'}},{body:{...base.body,project_id:'e'.repeat(32)}}])expect(validRunnerPairingRequest({...base,...change},project)).toBe(false);
 const decision=command();for(const change of [{path:'/runner-pairings/not-hex/commands'},{body:{...decision.body,expected_version:0}},{body:{...decision.body,action:'enable'}},{body:{...decision.body,device_name:'Injected'}},{body:{...decision.body,credential_sha256:sha}}])expect(validRunnerPairingRequest({...decision,...change},project)).toBe(false);
});

it('builds shell literals from public pairing data while allowing the runtime default identity location',()=>{
 const pairing=readRunnerPairing(waiting(),project);
 expect(runnerPairingCommand(pairing,'powershell','')).toBe("suma-runner --pair-runner '"+id+"' --control 'https://control.example.test/suma' --project '"+project+"'");
 const windows="C:\\Users\\Dev's $(Write-Output unexpected)\\identity.json";
 expect(runnerPairingCommand(pairing,'powershell',windows)).toContain("--identity 'C:\\Users\\Dev''s $(Write-Output unexpected)\\identity.json'");
 const unix="/home/dev's/$(printf unexpected)/identity.json";
 expect(runnerPairingCommand(pairing,'bash',unix)).toContain("--identity '/home/dev'\"'\"'s/$(printf unexpected)/identity.json'");
 for(const shell of ['powershell','bash'] as const){const text=runnerPairingCommand(pairing,shell,'')!;expect(text).not.toMatch(/--(?:token|worker|device-name|report-tools|allow-exec)/);expect(text).not.toContain(sha);}
 expect(runnerPairingCommand(pairing,'','')).toBeUndefined();
});

it('rejects unsafe control addresses and incomplete or device paths without inventing a usable command',()=>{
 for(const control_url of ['https://user:pass@example.test/suma','https://user@example.test','https://example.test?x=1','https://example.test/#fragment','http://example.test/suma','http://127.1/suma','http://127.0.0.2','http://2130706433','http://localhost./suma','http://[0:0:0:0:0:0:0:1]','https://example.test\\@other.test','https://example.test\n/suma'])expect(()=>readRunnerPairing({...waiting(),control_url},project)).toThrow();
 for(const control_url of ['https://example.test/prefix','http://localhost:8080/suma','http://127.0.0.1:8080','http://[::1]:8080/prefix'])expect(readRunnerPairing({...waiting(),control_url},project).control_url).toBe(control_url);
 const pairing=readRunnerPairing(waiting(),project);for(const path of ['relative/identity.json','C:relative.json','/','C:\\','\\\\.\\pipe\\identity','\\\\?\\C:\\identity.json','/tmp/line\nbreak.json','/tmp/\ud800.json',' /tmp/identity.json'])expect(runnerPairingCommand(pairing,'bash',path)).toBeUndefined();
 expect(runnerPairingCommand({...pairing,control_url:'http://untrusted.test'},'bash','/tmp/identity.json')).toBeUndefined();
});
