import {afterEach,describe,expect,it,vi} from 'vitest';
import {IngressRecovery,IngressPager,ingressSpec,ingressFingerprint,ingressKey,ingressCanEnable,ingressPanel,selectedIngress,ingressReason,type AutomationIngress} from './ingress-model';
const spec={name:'仓库检查',provider:'github' as const,events:['push'],repository:'Suma/Repo',repository_id:'42',ref:'refs/heads/main',conclusions:[]};
const entry:AutomationIngress={...spec,id:'in-1',project_id:'p',rule_id:'r',rule_revision:3,version:5,revision:2,state:'paused',activated_by:'',key_configured:true,key_generation:1,key_fingerprint:'abc',endpoint_path:'/api/v1/hooks/in-1',created_at:'2026-10-24T00:00:00Z',updated_at:'2026-10-24T00:00:00Z',last_error:''};
function storage(){const values=new Map<string,string>();return {values,getItem:(key:string)=>values.get(key)||null,setItem:(key:string,value:string)=>{values.set(key,value);},removeItem:(key:string)=>{values.delete(key);}};}
afterEach(()=>vi.unstubAllGlobals());
describe('restricted event metadata',()=>{
 it('rejects unsupported events, duplicate filters, unbound github repositories and command input',()=>{
  expect(ingressSpec(spec)).toEqual(spec);
  for(const change of [{events:['ci.completed']},{events:['push','push']},{repository_id:''},{ref:'main'},{conclusions:['SUCCESS']},{name:'中'.repeat(67)},{command:['sh']}] as any[])expect(()=>ingressSpec({...spec,...change})).toThrow();
  expect(ingressSpec({...spec,provider:'suma',events:['monitor.firing'],repository:'',repository_id:'',ref:''})).toMatchObject({repository:'',ref:''});
 });
 it('only enables an unrevoked configured ingress bound to the current enabled rule revision',()=>{
  expect(ingressCanEnable(entry,{version:8,revision:3,state:'enabled'})).toBe(true);
  for(const change of [{revision:4},{state:'paused'}])expect(ingressCanEnable(entry,{version:8,revision:3,state:'enabled',...change})).toBe(false);
  expect(ingressCanEnable({...entry,key_configured:false},{version:8,revision:3,state:'enabled'})).toBe(false);
  expect(ingressCanEnable({...entry,state:'revoked'},{version:8,revision:3,state:'enabled'})).toBe(false);
 });
 it('accepts full branch and tag refs while rejecting empty names and unsupported prefixes',()=>{
  expect(ingressSpec({...spec,ref:'refs/tags/v1.0.0'}).ref).toBe('refs/tags/v1.0.0');
  expect(ingressSpec({...spec,ref:'refs/heads/release/stable'}).ref).toBe('refs/heads/release/stable');
  expect(ingressSpec({...spec,ref:''}).ref).toBe('');
  for(const ref of ['refs/tag/v1.0.0','refs/remotes/origin/main','refs/heads/','refs/tags/'])expect(()=>ingressSpec({...spec,ref})).toThrow();
 });
 it('generates 64 lower hex characters and fingerprints their literal UTF8 bytes',async()=>{
  expect(ingressKey()).toMatch(/^[a-f0-9]{64}$/);
  expect(await ingressFingerprint('0'.repeat(64))).toBe('60e05bd1b195af2f94112fa7197a5c88289058840ce7c6df9693756bc6250f55');
 });
 it('uses the existing URL panel without losing encoded ingress identifiers',()=>{
  expect(ingressPanel('in/a b')).toBe('ingress:in/a b');expect(selectedIngress('ingress:in/a b')).toBe('in/a b');expect(selectedIngress('unrelated')).toBe('');
 });
});
describe('persisted ingress decisions',()=>{
 it.each([
  ['connection_check','连接检查'],['branch_deleted','分支或标签'],['action_not_completed','completed'],
  ['ingress_inactive','核对入口状态'],['rule_inactive_or_revised','核对当前规则'],['activation_authority_revoked','入口或规则启用人'],
  ['workflow_unavailable','固定工作流'],['filter_mismatch','当时来源修订'],['ingress_delivery_unavailable','关联交付记录'],['ingress_revised_or_rotated','原排队触发'],
  ['ingress_pause','已暂停'],['ingress_revise','来源已修改'],['ingress_revoke','已撤销'],['ingress_rotate_key','密钥已变更']
 ])('explains %s using only its persisted meaning',(code,fragment)=>{
  const value=ingressReason(code);expect(value.known).toBe(true);expect(value.message).toContain(fragment);expect(value.raw).toBe(code);
 });
 it('does not guess which filter failed and preserves unknown raw reasons',()=>{
  const filter=ingressReason('filter_mismatch');expect(filter.message).toContain('受限条件');expect(filter.message).not.toMatch(/未订阅|仓库不匹配|分支不匹配/);
  expect(ingressReason('future_server_reason;field=value')).toMatchObject({known:false,raw:'future_server_reason;field=value'});
 });
});
describe('ingress recovery boundaries',()=>{
 it('keeps a lost secret in memory but persists only its fingerprint and exact nonsecret operation',async()=>{
  const store=storage(),recovery=new IngressRecovery('org:user:p:r','p','r',store),fetcher=vi.fn().mockRejectedValueOnce(new TypeError('lost')).mockResolvedValueOnce(new Response(JSON.stringify(entry),{status:200}));vi.stubGlobal('fetch',fetcher);
  await expect(recovery.run('/automation-ingresses/in-1/commands',{project_id:'p',action:'rotate_key',expected_version:5,secret:'0'.repeat(64)})).rejects.toMatchObject({status:0});
  expect([...store.values.values()].join()).not.toContain('0'.repeat(64));expect(recovery.pending?.fingerprint).toBe('60e05bd1b195af2f94112fa7197a5c88289058840ce7c6df9693756bc6250f55');
  await recovery.restore();expect(JSON.parse(fetcher.mock.calls[1][1].body)).toEqual(JSON.parse(fetcher.mock.calls[0][1].body));expect(recovery.pending).toBeUndefined();
 });
 it('refuses to replay a rotation after reload loses the secret and requires metadata reconciliation',async()=>{
  const store=storage(),first=new IngressRecovery('scope','p','r',store);vi.stubGlobal('fetch',vi.fn().mockRejectedValue(new TypeError('lost')));
  await expect(first.run('/automation-ingresses/in-1/commands',{project_id:'p',action:'rotate_key',expected_version:5,secret:'0'.repeat(64)})).rejects.toMatchObject({status:0});
  const refreshed=new IngressRecovery('scope','p','r',store),fetcher=vi.fn();vi.stubGlobal('fetch',fetcher);
  expect(refreshed.pending?.secret_lost).toBe(true);await expect(refreshed.restore()).rejects.toThrow('元数据');expect(fetcher).not.toHaveBeenCalled();
  expect(refreshed.reconcile({...entry,key_fingerprint:'different'})).toBe(false);expect(refreshed.pending).toBeDefined();
  expect(refreshed.reconcile({...entry,key_fingerprint:'60e05bd1b195af2f94112fa7197a5c88289058840ce7c6df9693756bc6250f55',version:6})).toBe(true);expect(refreshed.pending).toBeUndefined();
 });
 it('replays metadata writes using their original rule CAS and operation after refresh',async()=>{
  const store=storage(),first=new IngressRecovery('scope','p','r',store),fetcher=vi.fn().mockRejectedValueOnce(new TypeError('lost')).mockResolvedValueOnce(new Response(JSON.stringify(entry),{status:200}));vi.stubGlobal('fetch',fetcher);
  await expect(first.run('/automations/r/ingresses',{project_id:'p',expected_rule_version:8,expected_rule_revision:3,spec})).rejects.toMatchObject({status:0});
  await new IngressRecovery('scope','p','r',store).restore();expect(JSON.parse(fetcher.mock.calls[1][1].body)).toEqual(JSON.parse(fetcher.mock.calls[0][1].body));
 });
 it('blocks a second submission while busy and retains unknown writes when a later replay is denied',async()=>{
  const store=storage(),recovery=new IngressRecovery('scope','p','r',store);let reject!:(reason:unknown)=>void;vi.stubGlobal('fetch',vi.fn().mockImplementationOnce(()=>new Promise((_resolve,no)=>{reject=no;})).mockResolvedValueOnce(new Response(JSON.stringify({error:{code:'forbidden',message:'撤权'}}),{status:403})));
  const first=recovery.run('/automation-ingresses/in-1/commands',{project_id:'p',action:'pause',expected_version:5});await expect(recovery.run('/automation-ingresses/in-1/commands',{project_id:'p',action:'revoke',expected_version:5})).rejects.toThrow('回执');reject(new TypeError('lost'));await expect(first).rejects.toMatchObject({status:0});await expect(recovery.restore()).rejects.toMatchObject({status:403});expect(recovery.pending?.body.action).toBe('pause');
 });
 it('rejects cross-project, arbitrary endpoints and secret-bearing metadata before saving or sending',async()=>{
  const store=storage(),recovery=new IngressRecovery('scope','p','r',store),fetcher=vi.fn();vi.stubGlobal('fetch',fetcher);
  for(const[path,body]of [['/automations/other/ingresses',{project_id:'p',spec}],['/automation-ingresses/in-1/commands',{project_id:'other',action:'pause'}],['/automation-ingresses/in-1/commands',{project_id:'p',action:'revise',spec:{...spec,secret:'bad'}}]] as const)await expect(recovery.run(path,body)).rejects.toThrow();
  expect(fetcher).not.toHaveBeenCalled();expect(store.values.size).toBe(0);
 });
 it('does not send writes if tab recovery storage is unavailable',async()=>{
  const fetcher=vi.fn();vi.stubGlobal('fetch',fetcher);await expect(new IngressRecovery('scope','p','r',undefined).run('/automation-ingresses/in-1/commands',{project_id:'p',action:'pause',expected_version:5})).rejects.toThrow('未发送');expect(fetcher).not.toHaveBeenCalled();
 });
 it('treats an accidental plaintext secret in persisted recovery as unreadable rather than replaying it',()=>{
  const store=storage();store.setItem('suma-ingress-recovery-v1:scope',JSON.stringify({path:'/automation-ingresses/in-1/commands',body:{project_id:'p',action:'rotate_key',expected_version:5,secret:'0'.repeat(64)},idempotency_key:'old',fingerprint:'60e05bd1b195af2f94112fa7197a5c88289058840ce7c6df9693756bc6250f55'}));
  const recovery=new IngressRecovery('scope','p','r',store);expect(recovery.error).toContain('无法读取');expect(recovery.pending).toBeUndefined();
 });
});
describe('delivery and immutable history pagination',()=>{
 it('retains a loaded history page on a next-page error and retries the same cursor',async()=>{
  vi.stubGlobal('fetch',vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({items:[{id:'d1'}],total:2,next_cursor:'older/+='}),{status:200})).mockResolvedValueOnce(new Response(JSON.stringify({error:{message:'离线'}}),{status:500})).mockResolvedValueOnce(new Response(JSON.stringify({items:[{id:'d2'}],total:2}),{status:200})));
  const pager=new IngressPager<{id:string}>('/automation-ingresses/in-1/deliveries','p');await pager.first();await pager.more();expect(pager.snapshot()).toMatchObject({items:[{id:'d1'}],more_error:true,next_cursor:'older/+='});await pager.more();expect(pager.snapshot().items?.map(item=>item.id)).toEqual(['d1','d2']);
  expect(vi.mocked(fetch).mock.calls[2][0]).toContain('cursor=older%2F%2B%3D');
 });
 it('keeps distinct revisions and rejects an older in-flight first page',async()=>{
  let old!:(value:Response)=>void;vi.stubGlobal('fetch',vi.fn().mockImplementationOnce(()=>new Promise(resolve=>{old=resolve;})).mockResolvedValueOnce(new Response(JSON.stringify({items:[{revision:3},{revision:2}],total:3,next_cursor:'older'}),{status:200})).mockResolvedValueOnce(new Response(JSON.stringify({items:[{revision:2},{revision:1}],total:3}),{status:200})));
  const pager=new IngressPager<{revision:number}>('/automation-ingresses/in-1/revisions','p','revision');const stale=pager.first();await pager.first();await pager.more();old(new Response(JSON.stringify({items:[{revision:99}],total:99}),{status:200}));await stale;expect(pager.snapshot().items?.map(item=>item.revision)).toEqual([3,2,1]);
 });
});
