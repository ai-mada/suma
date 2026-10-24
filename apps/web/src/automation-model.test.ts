import {afterEach,describe,expect,it,vi} from 'vitest';
import {automationDetailPath,automationListPath,automationPermissions,automationReason,automationMoment,automationSpec,automationWorkflowOptions,AutomationRecovery,AutomationRevisionPager} from './automation-model';
import {parseWorkbenchRoute} from './navigation';
const draft={name:'每日检查',workflow_id:'wf',workflow_version:2,trigger:'cron' as const,cron:'0 9 * * 1-5',timezone:'Asia/Shanghai',misfire:'latest' as const,catch_up_limit:5,overlap:'queue' as const};
const storage=()=>{const values=new Map<string,string>();return {getItem:(key:string)=>values.get(key)||null,setItem:(key:string,value:string)=>{values.set(key,value);},removeItem:(key:string)=>{values.delete(key);}};};
afterEach(()=>vi.unstubAllGlobals());
describe('automation rule contracts',()=>{
 it('pins the actual workflow version and keeps manual triggers free of a stale cron expression',()=>{
  expect(automationSpec({...draft,trigger:'manual',cron:'old schedule'})).toEqual({...draft,trigger:'manual',cron:''});
  expect(()=>automationSpec({...draft,workflow_version:0})).toThrow('版本');
  expect(()=>automationSpec({...draft,catch_up_limit:101})).toThrow('100');
  expect(()=>automationSpec({...draft,timezone:'wrong/timezone'})).toThrow('时区');
 });
 it('enforces the server name limit for Unicode characters instead of counting UTF-16 characters',()=>{
  expect(automationSpec({...draft,name:'中'.repeat(66)+'aa'}).name).toHaveLength(68);
  expect(()=>automationSpec({...draft,name:'中'.repeat(67)})).toThrow('名称过长');
  expect(()=>automationSpec({...draft,name:'a'.repeat(201)})).toThrow('名称过长');
 });
 it('chooses the actual latest available version without guessing authority from workflow names',()=>{
  const rows=[{id:'check',name:'历史检查',version:2,nodes:[],edges:[]},{id:'check',name:'检查',version:3,nodes:[],edges:[]},{id:'managed-user-check',name:'普通检查',version:1,nodes:[],edges:[]}];
  const options=automationWorkflowOptions(rows as any);
  expect(options.map(row=>[row.id,row.version])).toEqual([['check',3],['check',2],['managed-user-check',1]]);
 });
 it('builds stable scoped pagination and reads a deep linked rule without depending on the list page',()=>{
  const route=parseWorkbenchRoute('?project=project%2Fa&view=development&sub=automations&record=rule%20a');
  expect(automationDetailPath(route.project,route.record)).toBe('/automations/rule%20a?project_id=project%2Fa');
  expect(automationListPath('p','next/+')).toBe('/automations?project_id=p&limit=40&cursor=next%2F%2B');
  expect(automationListPath('p','next','r/a')).toBe('/automations/r%2Fa/occurrences?project_id=p&limit=40&cursor=next');
 });
 it('uses real organization and project grants rather than assuming every member can approve',()=>{
  expect(automationPermissions({user:{role:'member'},projects:[{id:'p',role:'viewer'}]} as any,'p')).toEqual({write:false,approve:false});
  expect(automationPermissions({user:{role:'member'},projects:[{id:'p',role:'developer'}]} as any,'p')).toEqual({write:true,approve:false});
  expect(automationPermissions({user:{role:'owner'},projects:[{id:'p'}]} as any,'p')).toEqual({write:true,approve:true});
  expect(automationPermissions({user:{role:'owner'},projects:[]} as any,'p')).toEqual({write:false,approve:false});
 });
 it('explains bounded recovery windows in the rule timezone and preserves unknown server reasons for audit',()=>{
  const reason=automationReason('misfire_catch_up_limit;window_end=2026-10-24T04:00:00Z','2026-10-24T00:00:00Z','Asia/Shanghai');
  expect(reason.message).toContain('剩余触发跳过');expect(reason.range).toContain('08:00:00');expect(reason.range).toContain('12:00:00');expect(reason.range).toContain('Asia/Shanghai');expect(reason.raw).toBe('misfire_catch_up_limit;window_end=2026-10-24T04:00:00Z');
  expect(automationReason('overlap_wait').message).toContain('排队等待');expect(automationReason('new_server_reason').message).toBe('服务端原因');
  expect(automationMoment('2026-10-24T00:00:00Z','Asia/Shanghai')).toEqual({local:'2026/10/24 08:00:00',timezone:'Asia/Shanghai',utc:'2026-10-24T00:00:00Z'});
 });
 it('reads older immutable revision pages from the history API without collapsing distinct revisions',async()=>{
  vi.stubGlobal('fetch',vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({items:[{revision:102,spec:draft,created_at:'2026-10-24T00:00:00Z'},{revision:101,spec:{...draft,workflow_version:1},created_at:'2026-10-23T00:00:00Z'}],total:102,next_cursor:'older/+='}),{status:200})).mockResolvedValueOnce(new Response(JSON.stringify({items:[{revision:100,spec:{...draft,name:'历史规则'},created_at:'2026-10-22T00:00:00Z'}],total:102,next_cursor:'next'}),{status:200})));
  const pager=new AutomationRevisionPager('p/a','r a');await pager.first();await pager.more();
  expect(pager.snapshot().items?.map(item=>item.revision)).toEqual([102,101,100]);expect(pager.snapshot().total).toBe(102);expect(pager.snapshot().next_cursor).toBe('next');
  const fetcher=vi.mocked(fetch);expect(fetcher.mock.calls[0][0]).toBe('/api/v1/automations/r%20a/revisions?project_id=p%2Fa&limit=40');expect(fetcher.mock.calls[1][0]).toContain('cursor=older%2F%2B%3D');
 });
});
describe('automation write recovery',()=>{
 it('restores the original exact command and operation ID after a lost receipt and browser refresh',async()=>{
  const store=storage(),first=new AutomationRecovery('org:user:p',store),fetcher=vi.fn().mockRejectedValueOnce(new TypeError('lost')).mockResolvedValueOnce(new Response(JSON.stringify({rule:{id:'r',version:8}}),{status:200}));vi.stubGlobal('fetch',fetcher);
  await expect(first.run('/automations/r/commands',{project_id:'p',action:'trigger',expected_version:7})).rejects.toMatchObject({status:0});
  const refreshed=new AutomationRecovery('org:user:p',store);
  expect(refreshed.pending?.body.expected_version).toBe(7);
  await refreshed.run('/automations/other/commands',{project_id:'p',action:'trigger',expected_version:9});
  expect(fetcher.mock.calls[1][0]).toBe('/api/v1/automations/r/commands');
  expect(JSON.parse(fetcher.mock.calls[1][1].body)).toEqual(JSON.parse(fetcher.mock.calls[0][1].body));
  expect(refreshed.pending).toBeUndefined();
 });
 it('clears a definite CAS rejection but retains a previously unknown request on a later denial',async()=>{
  const store=storage(),request=new AutomationRecovery('conflict',store);vi.stubGlobal('fetch',vi.fn().mockResolvedValue(new Response(JSON.stringify({error:{code:'version_conflict',message:'已变更'}}),{status:409})));
  await expect(request.run('/automations/r/commands',{project_id:'p',action:'pause',expected_version:3})).rejects.toMatchObject({status:409});expect(request.pending).toBeUndefined();
  const unknown=new AutomationRecovery('unknown',store);vi.stubGlobal('fetch',vi.fn().mockRejectedValueOnce(new TypeError('lost')).mockResolvedValueOnce(new Response(JSON.stringify({error:{code:'forbidden',message:'已撤权'}}),{status:403})));
  await expect(unknown.run('/automations/r/commands',{project_id:'p',action:'trigger',expected_version:4})).rejects.toMatchObject({status:0});
  await expect(unknown.restore()).rejects.toMatchObject({status:403});expect(unknown.pending?.body.expected_version).toBe(4);
 });
 it('refuses secret data and other endpoints before saving or sending any request',async()=>{
  const recovery=new AutomationRecovery('secret',storage()),fetcher=vi.fn();vi.stubGlobal('fetch',fetcher);
  await expect(recovery.run('/tokens',{project_id:'p'})).rejects.toThrow('自动化');
  await expect(recovery.run('/automations',{project_id:'p',spec:{...draft,token:'secret'}})).rejects.toThrow('秘密');
  expect(fetcher).not.toHaveBeenCalled();expect(recovery.pending).toBeUndefined();
 });
 it('isolates saved commands by account/project and stops when recovery storage is unavailable',async()=>{
  const store=storage(),recovery=new AutomationRecovery('org:a:p',store);vi.stubGlobal('fetch',vi.fn().mockRejectedValue(new TypeError('lost')));
  await expect(recovery.run('/automations',{project_id:'p',spec:draft})).rejects.toMatchObject({status:0});
  expect(new AutomationRecovery('org:b:p',store).pending).toBeUndefined();expect(new AutomationRecovery('org:a:q',store).pending).toBeUndefined();
  const fetcher=vi.fn();vi.stubGlobal('fetch',fetcher);
  await expect(new AutomationRecovery('disabled',undefined).run('/automations',{project_id:'p',spec:draft})).rejects.toThrow('未发送');expect(fetcher).not.toHaveBeenCalled();
 });
});
