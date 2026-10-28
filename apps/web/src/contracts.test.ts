import {afterEach,describe,expect,it,vi} from 'vitest';
import {api,ApiError,normalizeResponse,recordPayload,type Workflow} from './api';
import {declaredEdges,dependencyError,updateDependency,canCompensate,canApproveNode} from './workflow-model';
import {mutate} from './operations';
import {TokenCreation} from './token-requests';
import {createElement} from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
import {ErrorRecovery,recoveryDetails,ErrorBoundary} from './ErrorBoundary';
import {EntityPager} from './pagination';
import {FileSaveRequest,draftAfterFileSave} from './file-saves';
import {withCurrentOption} from './record-options';
import {parseWorkbenchRoute,initializeSessionRoute} from './navigation';
import {readLinkedRun,includeCurrentRun} from './run-selection';
afterEach(()=>{vi.unstubAllGlobals();vi.useRealTimers();});
describe('record wire contract',()=>{
 it('preserves criteria and custom member/document data through roundtrip',()=>{
  const body=recordPayload('documents',{title:'交付指南',content:'# 正文',acceptance_criteria:'测试通过\n人工验收'});
  expect(body).toEqual({title:'交付指南',acceptance_criteria:['测试通过','人工验收'],data:{content:'# 正文'}});
  const result=normalizeResponse({items:[{id:'doc-1',version:2,title:'交付指南',acceptance_criteria:['测试通过','人工验收'],data:{content:'# 正文'}}]});
  expect(result.items[0].content).toBe('# 正文');expect(result.items[0].acceptance_criteria).toBe('测试通过\n人工验收');
  expect(recordPayload('members',{name:'Agent A',kind:'agent',email:'a@local.dev',role:'member'})).toEqual({name:'Agent A',kind:'agent',data:{email:'a@local.dev',role:'member'}});
 });
 it('converts task dependencies and release task selections to actual arrays',()=>{
  expect(recordPayload('tasks',{depends_on:'a, b, ',acceptance_criteria:''})).toEqual({depends_on:['a','b'],acceptance_criteria:[]});
  expect(recordPayload('releases',{task_ids:'a, b',release_version:'1.0'}).data).toEqual({task_ids:['a','b'],release_version:'1.0'});
 });
 it('treats canonical record identifiers as authoritative over custom data',()=>{
  expect(normalizeResponse({id:'real',version:1,title:'Real',data:{id:'wrong',version:99}}).id).toBe('real');
 });
});
describe('DAG rendering contract',()=>{
 const base:Workflow={id:'flow',name:'测试',version:1,nodes:[{id:'a',name:'A',type:'test'},{id:'b',name:'B',type:'approval'}],edges:[]};
 it('does not fabricate sequential edges for independent nodes',()=>expect(declaredEdges(base)).toEqual([]));
 it('deduplicates depends_on without erasing explicit branch conditions',()=>{
  const edges=declaredEdges({...base,nodes:[base.nodes[0],{...base.nodes[1],depends_on:['a','a']}],edges:[{from:'a',to:'b',condition:'succeeded(a)'}]});
  expect(edges).toEqual([{from:'a',to:'b',condition:'succeeded(a)'}]);
 });
 it('allows parallel dependencies and removes only the selected incoming edge',()=>{
  const original=[{from:'a',to:'c',condition:'a.state == succeeded'},{from:'b',to:'c'},{from:'c',to:'d'}];
  expect(dependencyError(['a','b','c','d'],original)).toBeUndefined();
  const result=updateDependency(['a','b','c','d'],original,'b','c',false);
  expect(result.error).toBeUndefined();expect(result.edges).toEqual([original[0],original[2]]);expect(original).toHaveLength(3);
 });
 it('rejects direct and transitive cycles without changing the actual graph',()=>{
  const original=[{from:'a',to:'b'},{from:'b',to:'c'}];
  const cycle=updateDependency(['a','b','c'],original,'c','a',true);
  expect(cycle.error).toContain('环路');expect(cycle.edges).toBe(original);
  expect(updateDependency(['a','b','c'],original,'a','a',true).error).toContain('自身');
  expect(updateDependency(['a','b','c'],original,'missing','a',true).error).toContain('不存在');
 });
 it('uses the same dependency operation for addition, deduplication and later valid edits',()=>{
  const original=[{from:'a',to:'b',condition:'true'}];
  const duplicate=updateDependency(['a','b','c'],original,'a','b',true);expect(duplicate.edges).toEqual(original);
  const next=updateDependency(['a','b','c'],duplicate.edges,'b','c',true);expect(next.error).toBeUndefined();expect(next.edges).toEqual([...original,{from:'b',to:'c'}]);
  const removed=updateDependency(['a','b','c'],next.edges,'a','b',false);
  expect(updateDependency(['a','b','c'],removed.edges,'c','a',true).error).toBeUndefined();
 });
 it('offers compensation only for finished runs with actual successful configured work',()=>{
  const workflow={...base,nodes:[{id:'a',name:'A',type:'command',config:{compensation:{type:'command',command:['undo']}}}]};
  const run:any={state:'failed',nodes:[{id:'a',state:'succeeded',attempt:1,compensatable:true}]};
  expect(canCompensate(workflow,run)).toBe(true);expect(canCompensate(workflow,{...run,state:'running'})).toBe(false);
  expect(canCompensate(base,run)).toBe(true);expect(canCompensate(workflow,{...run,nodes:[{id:'a',state:'succeeded',attempt:1}]})).toBe(false);expect(canCompensate(workflow,{...run,compensation_run_id:'already-created'})).toBe(false);
  expect(canCompensate(workflow,{...run,nodes:[{id:'a',state:'reconciling',attempt:1}]})).toBe(false);
 });
 it('only exposes approval actions for actual waiting approval nodes',()=>{
  expect(canApproveNode({type:'approval',state:'waiting'})).toBe(true);
  for(const type of ['map','timer','join','subworkflow'])expect(canApproveNode({type,state:'waiting'})).toBe(false);
  expect(canApproveNode({type:'approval',state:'succeeded'})).toBe(false);
 });
});
describe('render failure recovery',()=>{
 it('preserves project and exact current URL and only offers explicit recovery',()=>{
  const url='http://localhost:8080/?project=real-project&view=development&sub=workflows&run=run-1&node=node-2';
  const reload=vi.fn();vi.stubGlobal('location',{href:url,reload});
  const error=new Error('Failed to fetch dynamically imported module: /assets/Workflow-old.js');
  const info=recoveryDetails(error,url);expect(info.currentURL).toBe(url);expect(info.title).toBe('页面资源已更新');
  expect(new URL(info.homeURL).searchParams.get('project')).toBe('real-project');expect(new URL(info.homeURL).searchParams.has('run')).toBe(false);
  const html=renderToStaticMarkup(createElement(ErrorRecovery,{error}));
  expect(html).toContain('重新加载当前页面');expect(html).toContain('node=node-2');expect(html).toContain('页面不会自动刷新');expect(reload).not.toHaveBeenCalled();
  expect(ErrorBoundary.getDerivedStateFromError(error)).toEqual({error});
 });
});
describe('HTTP mutation recovery',()=>{
 it('sends same-origin cookie requests and surfaces structured errors',async()=>{
  const fetcher=vi.fn().mockResolvedValue(new Response(JSON.stringify({error:{code:'forbidden',message:'权限不足'}}),{status:403}));vi.stubGlobal('fetch',fetcher);
  await expect(api('/tasks')).rejects.toMatchObject({status:403,code:'forbidden',message:'权限不足'});
  expect(fetcher.mock.calls[0][1].credentials).toBe('same-origin');
 });
 it('replays the original command key and version after an unknown response',async()=>{
  const fetcher=vi.fn().mockRejectedValueOnce(new TypeError('disconnected')).mockResolvedValueOnce(new Response(JSON.stringify({id:'r',version:8}),{status:200}));vi.stubGlobal('fetch',fetcher);
  const path='/runs/recovery-test/commands',identity='recovery-test-identity';
  await expect(mutate(path,{expected_version:5,action:'approve'},undefined,identity)).rejects.toBeInstanceOf(ApiError);
  await mutate(path,{expected_version:8,action:'approve'},undefined,identity);
  const first=JSON.parse(fetcher.mock.calls[0][1].body),second=JSON.parse(fetcher.mock.calls[1][1].body);
  expect(second).toEqual(first);expect(second.expected_version).toBe(5);expect(first.idempotency_key).toBeTruthy();
 });
 it('treats a successful but truncated JSON receipt as unknown and reuses the original mutation key',async()=>{
  const fetcher=vi.fn().mockResolvedValueOnce(new Response('{"id":',{status:201})).mockResolvedValueOnce(new Response(JSON.stringify({id:'created',version:1}),{status:201}));vi.stubGlobal('fetch',fetcher);
  const body={project_id:'p',title:'Receipt recovery'};
  await expect(mutate('/requirements/parse-recovery',body)).rejects.toMatchObject({status:0,code:'response_unknown'});
  await expect(mutate('/requirements/parse-recovery',body)).resolves.toMatchObject({id:'created'});
  expect(JSON.parse(fetcher.mock.calls[1][1].body)).toEqual(JSON.parse(fetcher.mock.calls[0][1].body));
 });
 it('ends a stalled request as an unknown timeout rather than keeping the save control busy forever',async()=>{
  vi.useFakeTimers();vi.stubGlobal('fetch',vi.fn((_:unknown,options:RequestInit)=>new Promise((_,reject)=>options.signal!.addEventListener('abort',()=>reject(new Error('aborted'))))));
  const observed=expect(api('/tasks',{project_id:'p',title:'Stalled'})).rejects.toMatchObject({status:0,code:'request_timeout'});
  await vi.advanceTimersByTimeAsync(30000);await observed;
 });
});
describe('project scoped token creation',()=>{
 it('sends the selected project and scope and displays the actual response contract',async()=>{
  const response={token:'one-time-value',project_id:'project-a',scope:'agent',label:'Agent接入',expires_at:'2026-10-24T00:00:00Z'};
  const fetcher=vi.fn().mockResolvedValue(new Response(JSON.stringify(response),{status:201}));vi.stubGlobal('fetch',fetcher);
  const creation=new TokenCreation();expect(await creation.create('project-a','Agent接入','agent')).toEqual(response);
  expect(JSON.parse(fetcher.mock.calls[0][1].body)).toMatchObject({project_id:'project-a',label:'Agent接入',scope:'agent',idempotency_key:expect.any(String)});
 });
 it('retains operation identity and refuses to send again after a lost token receipt',async()=>{
  const fetcher=vi.fn().mockRejectedValue(new TypeError('network disconnected'));vi.stubGlobal('fetch',fetcher);
  const creation=new TokenCreation();await expect(creation.create('project-a','Runner','runner')).rejects.toMatchObject({status:0});
  const operationID=creation.operationID;expect(creation.unknown).toBe(true);
  await expect(creation.create('project-a','Runner','runner')).rejects.toMatchObject({code:'token_outcome_unknown'});
  expect(fetcher).toHaveBeenCalledTimes(1);expect(creation.operationID).toBe(operationID);
 });
});
describe('bounded entity pagination',()=>{
 const record=(id:string,version=1)=>({id,version,title:id});
 const response=(items:any[],total:number,next_cursor='')=>new Response(JSON.stringify({items,total,next_cursor}),{status:200});
 it('loads one bounded page per action, follows cursor and deduplicates while retaining full total',async()=>{
  const fetcher=vi.fn().mockResolvedValueOnce(response([record('a'),record('b')],4,'cursor with/& symbols')).mockResolvedValueOnce(response([record('b',2),record('c')],4,'next-cursor'));
  vi.stubGlobal('fetch',fetcher);const pager=new EntityPager('project-a','tasks');
  await pager.first();expect(fetcher).toHaveBeenCalledTimes(1);expect(pager.snapshot()).toMatchObject({total:4,next_cursor:'cursor with/& symbols'});expect(pager.snapshot().items).toHaveLength(2);
  await pager.more();expect(fetcher).toHaveBeenCalledTimes(2);expect(pager.snapshot().items?.map(r=>[r.id,r.version])).toEqual([['a',1],['b',2],['c',1]]);expect(pager.snapshot().total).toBe(4);
  const url=new URL(fetcher.mock.calls[1][0],'http://localhost');expect(url.searchParams.get('limit')).toBe('100');expect(url.searchParams.get('cursor')).toBe('cursor with/& symbols');expect(url.searchParams.get('project_id')).toBe('project-a');
 });
 it('keeps rows, cursor and total after a failed next page, then retries that exact cursor',async()=>{
  const fetcher=vi.fn().mockResolvedValueOnce(response([record('a')],2,'page-2')).mockRejectedValueOnce(new TypeError('disconnected')).mockResolvedValueOnce(response([record('b')],2));vi.stubGlobal('fetch',fetcher);
  const pager=new EntityPager('project-a','requirements');await pager.first();await pager.more();
  expect(pager.snapshot()).toMatchObject({items:[record('a')],next_cursor:'page-2',total:2,error_action:'more',loading:false});expect(pager.snapshot().error).not.toBe('');
  await pager.more();expect(fetcher.mock.calls[2][0]).toBe(fetcher.mock.calls[1][0]);expect(pager.snapshot().items).toHaveLength(2);expect(pager.snapshot().next_cursor).toBe('');expect(pager.snapshot().error).toBe('');
 });
 it('resets scope and cursor and ignores an old in-flight project response',async()=>{
  let finishOld!:(value:Response)=>void;
  const fetcher=vi.fn().mockReturnValueOnce(new Promise<Response>(resolve=>{finishOld=resolve;})).mockResolvedValueOnce(response([record('new-project-item')],1));vi.stubGlobal('fetch',fetcher);
  const pager=new EntityPager('old-project','tasks');const old=pager.first();pager.setScope('new-project','issues');expect(pager.snapshot()).toMatchObject({items:undefined,total:0,next_cursor:''});
  await pager.first();finishOld(response([record('old-project-item')],500,'old-cursor'));await old;
  expect(pager.snapshot().items?.map(r=>r.id)).toEqual(['new-project-item']);expect(pager.snapshot().total).toBe(1);expect(pager.snapshot().next_cursor).toBe('');
  const url=new URL(fetcher.mock.calls[1][0],'http://localhost');expect(url.pathname).toBe('/api/v1/issues');expect(url.searchParams.get('project_id')).toBe('new-project');expect(url.searchParams.has('cursor')).toBe(false);
 });
 it('does not discard an already loaded window when refreshing the first page fails',async()=>{
  const fetcher=vi.fn().mockResolvedValueOnce(response([record('kept')],8,'next')).mockResolvedValueOnce(new Response(JSON.stringify({error:{code:'unavailable',message:'服务暂时不可用'}}),{status:503}));vi.stubGlobal('fetch',fetcher);
  const pager=new EntityPager('project-a','documents');await pager.first();await pager.first();expect(pager.snapshot()).toMatchObject({items:[record('kept')],total:8,next_cursor:'next',error_action:'first',error:'服务暂时不可用'});
 });
});
describe('file save concurrency and lost receipts',()=>{
 it('retains typing after save submission and uses the saved base hash for the next write',()=>{
  const pending=new FileSaveRequest(),before={project_id:'p',path:'main.ts',content:'old',hash:'hash-old',language:'typescript',workspace_name:'workspace-a',context_sha256:'a'.repeat(64)};
  const submitted=pending.begin('p',before,'submitted');const response={...before,content:'submitted',hash:'hash-saved'};
  const draft=draftAfterFileSave('submitted plus later typing',submitted.content,response.content);
  expect(draft).toBe('submitted plus later typing');pending.completed();
  expect(pending.begin('p',response,draft)).toMatchObject({content:draft,expected_hash:'hash-saved'});
  expect(draftAfterFileSave('submitted','submitted','submitted')).toBe('submitted');
 });
 it('replays the entire original file envelope even when the editor has changed after a lost response',()=>{
  const pending=new FileSaveRequest(),before={project_id:'p',path:'main.ts',content:'old',hash:'hash-old',language:'typescript',workspace_name:'workspace-a',context_sha256:'a'.repeat(64)};
  const first=pending.begin('p',before,'submitted');pending.failed(new ApiError(0,'network_error','Lost'));
  const retry=pending.begin('p',{...before,hash:'different'},'new typing');
  expect(retry).toBe(first);expect(retry).toMatchObject({content:'submitted',expected_hash:'hash-old'});expect(pending.unknown).toBe(true);
 });
 it('keeps reconciliation identity instead of treating an unknown prepared operation as a new hash conflict',()=>{
  const pending=new FileSaveRequest(),first=pending.begin('p',{project_id:'p',path:'a.ts',content:'before',hash:'before-hash',language:'typescript',workspace_name:'workspace-a',context_sha256:'a'.repeat(64)},'submitted');
  pending.failed(new ApiError(409,'workspace_reconciliation_required','Recover',{operation_id:first.idempotency_key,path:'a.ts'}));
  expect(pending.pending).toBe(first);expect(pending.reconciliation).toBe(true);expect(pending.reconciliationDetails?.operation_id).toBe(first.idempotency_key);
  pending.completed();expect(pending.pending).toBeUndefined();expect(pending.unknown).toBe(false);
 });
});
describe('bounded relation options',()=>{
 it('retains current relations outside the first 100 rows so an unrelated edit cannot clear them',()=>{
  const page=Array.from({length:100},(_,index)=>({id:'m-'+index,version:1,title:'Member '+index}));
  const options=withCurrentOption(page,'m-101','负责人');
  expect(options).toHaveLength(101);expect(options[0].id).toBe('m-101');expect(options[0].title).toContain('未在已加载范围');
  expect(withCurrentOption(options,'m-101','负责人')).toBe(options);expect(page).toHaveLength(100);
  const html=renderToStaticMarkup(createElement('select',{name:'owner_id',defaultValue:'m-101'},options.map(item=>createElement('option',{key:item.id,value:item.id},item.title))));
  expect(html).toContain('value="m-101" selected=""');
 });
});
describe('session initialization deep links',()=>{
 it('retains workflow version context, run, node and view across session initialization',()=>{
  const route=parseWorkbenchRoute('?project=project-b&view=development&sub=workflows&run=run-2&node=node-3&status=failed');
  expect(initializeSessionRoute(route,[{id:'project-a'},{id:'project-b'}])).toEqual(route);
  expect(route).toMatchObject({project:'project-b',view:'development',sub:'workflows',run:'run-2',node:'node-3',status:'failed'});
 });
 it('only fills a missing project after login and preserves quality record deep links on refresh',()=>{
  const route=parseWorkbenchRoute('?view=quality&sub=issues&record=issue-9');
  expect(initializeSessionRoute(route,[{id:'first-project'}])).toEqual({...route,project:'first-project'});
  expect(parseWorkbenchRoute('?project=p&view=quality&sub=issues&record=issue-9')).toMatchObject({project:'p',view:'quality',sub:'issues',record:'issue-9'});
 });
});
describe('bounded execution summaries and pinned review evidence',()=>{
 it('reads a linked run outside the summary page directly without loading all history',async()=>{
  const summary={id:'recent',workflow_name:'Recent',workflow_version:2,version:1,state:'running'};
  const actual={id:'linked-old',workflow_name:'Older evidence',workflow_version:1,version:8,state:'succeeded',nodes:[{id:'test',type:'test',state:'succeeded',attempt:1}]};
  const fetcher=vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({items:[summary],total:501,next_cursor:'next-page'}),{status:200})).mockResolvedValueOnce(new Response(JSON.stringify(actual),{status:200}));vi.stubGlobal('fetch',fetcher);
  const pager=new EntityPager<any>('p','runs');await pager.first();const linked=await readLinkedRun('p','linked-old');
  const options=includeCurrentRun(pager.snapshot().items||[],linked);
  expect(fetcher).toHaveBeenCalledTimes(2);expect(fetcher.mock.calls[1][0]).toBe('/api/v1/runs/linked-old?project_id=p');
  expect(pager.snapshot().total).toBe(501);expect(pager.snapshot().items).toHaveLength(1);expect(pager.snapshot().items?.[0].nodes).toBeUndefined();
  expect(options.map(run=>run.id)).toEqual(['recent','linked-old']);expect(options[1].nodes[0].type).toBe('test');
 });
 it('replaces a summary with the actual linked run without duplicating its choice',()=>{
  const summary:any={id:'same',version:1,state:'running'},actual:any={id:'same',version:4,state:'succeeded',nodes:[]};
  expect(includeCurrentRun([summary],actual)).toEqual([actual]);
 });
});
