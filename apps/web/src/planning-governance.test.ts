import {createElement} from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
import {canRetryNode,latestProgress} from './workflow-model';
import {WorkspaceEvidence} from './WorkspaceEvidence';
import {afterEach,describe,expect,it,vi} from 'vitest';
import {ApiError,recordPayload,type Session} from './api';
import {mutate} from './operations';
import {parseWorkbenchRoute} from './navigation';
import {compareTask,emptyPlan,mondayOf,nullableHours,rebaseSchedule,scheduleDraft,scheduleDateField,scheduleChangesFromForm,timelinePosition,timelineRange,type PlannedTask} from './planning-model';
import {ScheduleForm} from './Planning';
import {invitationFragment,invitationURL,switchedRoute,teamIdentity} from './governance-model';
import {eligiblePeople} from './ResourceIdentityFields';
import {InvitationAcceptanceRequest} from './invitation-request';
afterEach(()=>vi.unstubAllGlobals());
const task=(patch:Partial<PlannedTask>={}):PlannedTask=>({id:'task-a',version:4,title:'实际交付任务',status:'in_review',depends_on:['dependency'],plan:{...emptyPlan,version:2,start_date:'2026-10-19',due_date:'2026-10-23',estimated_hours:null},...patch});
describe('planning task and independent plan concurrency',()=>{
 it('sends the full edited plan with both versions while keeping unknown estimate null',()=>{
  expect(scheduleDraft(task())).toEqual({task_id:'task-a',expected_task_version:4,expected_version:2,iteration_id:'',milestone_id:'',start_date:'2026-10-19',due_date:'2026-10-23',estimated_hours:null});
 });
 it('explicit rebase changes only versions and preserves the complete user draft',()=>{
  const draft={...scheduleDraft(task()),due_date:'2026-11-02',estimated_hours:0,iteration_id:'iteration-a'};
  const next=rebaseSchedule([draft],[task({version:8,plan:{...emptyPlan,version:9,due_date:'2026-10-25'}})])[0];
  expect(next).toEqual({...draft,expected_task_version:8,expected_version:9});expect(draft.expected_version).toBe(2);
  expect(()=>rebaseSchedule([draft],[])).toThrow('不在当前项目');
 });
 it('replays the original batch and both versions after a lost response',async()=>{
  const fetcher=vi.fn().mockRejectedValueOnce(new TypeError('lost response')).mockResolvedValueOnce(new Response(JSON.stringify({updated:1}),{status:200}));vi.stubGlobal('fetch',fetcher);
  const original={project_id:'project-a',changes:[scheduleDraft(task())]};
  await expect(mutate('/planning/schedule',original,'POST','schedule-contract')).rejects.toBeInstanceOf(ApiError);
  await mutate('/planning/schedule',{project_id:'other-project',changes:[scheduleDraft(task({version:99}))]},'POST','schedule-contract');
  const first=JSON.parse(fetcher.mock.calls[0][1].body),second=JSON.parse(fetcher.mock.calls[1][1].body);
  expect(second).toEqual(first);expect(second.project_id).toBe('project-a');expect(second.changes[0]).toMatchObject({expected_task_version:4,expected_version:2,estimated_hours:null});
 });
 it('submits visible native date values when a date widget did not fire a synthetic change',async()=>{
  const initial=task({plan:{...emptyPlan,version:2,iteration_id:'iteration-real',estimated_hours:8}}),draft=scheduleDraft(initial);
  const form=new FormData();form.set(scheduleDateField(initial.id,'start_date'),'2026-10-19');form.set(scheduleDateField(initial.id,'due_date'),'2026-10-23');
  const html=renderToStaticMarkup(createElement(ScheduleForm,{project:'project-real',tasks:[initial],iterations:[],milestones:[],onSaved:()=>{}}));
  expect(html).toContain('name="'+scheduleDateField(initial.id,'start_date')+'"');expect(html).toContain('name="'+scheduleDateField(initial.id,'due_date')+'"');
  const fetcher=vi.fn().mockResolvedValue(new Response(JSON.stringify({items:[],total:1}),{status:200}));vi.stubGlobal('fetch',fetcher);
  const changes=scheduleChangesFromForm([draft],form);
  await mutate('/planning/schedule',{project_id:'project-real',changes},'POST','native-dates-contract');
  const sent=JSON.parse(fetcher.mock.calls[0][1].body);expect(sent.changes[0]).toEqual({...draft,start_date:'2026-10-19',due_date:'2026-10-23'});
  expect(sent.changes[0]).toMatchObject({expected_task_version:4,expected_version:2,iteration_id:'iteration-real',estimated_hours:8});expect(draft.start_date).toBe('');
 });
 it('retains captured dates when disabled recovery fields are omitted and keeps batch dates separate',()=>{
  const first=scheduleDraft(task()),second=scheduleDraft(task({id:'task-b',plan:{...emptyPlan}}));
  const form=new FormData();form.set(scheduleDateField(second.task_id,'start_date'),'2026-11-02');form.set(scheduleDateField(second.task_id,'due_date'),'');
  expect(scheduleChangesFromForm([first,second],form)).toEqual([first,{...second,start_date:'2026-11-02',due_date:''}]);
 });
 it('keeps zero capacity distinct from unknown and rejects invalid hours',()=>{
  expect(nullableHours('')).toBeNull();expect(nullableHours('  ')).toBeNull();expect(nullableHours('0')).toBe(0);expect(nullableHours('7.5')).toBe(7.5);
  for(const value of ['-1','no','Infinity'])expect(()=>nullableHours(value)).toThrow();
 });
 it('uses Monday in the selected local calendar week including Sunday',()=>{
  expect(mondayOf(new Date(2026,9,25,18))).toBe('2026-10-19');expect(mondayOf(new Date(2026,9,26,1))).toBe('2026-10-26');
 });
});
describe('honest planning visualization and immutable scope comparison',()=>{
 it('derives actual date geometry without a fabricated unscheduled bar',()=>{
  const unscheduled=task({plan:{...emptyPlan}});expect(timelineRange([unscheduled],[])).toBeNull();
  const range=timelineRange([task()],[])!;expect(range.days).toBe(4);expect(timelinePosition('2026-10-19','2026-10-23',range)).toEqual({left:0,width:100});
  expect(timelinePosition('','',range)).toBeNull();
 });
 it('does not call an unloaded current task removed from the baseline',()=>{
  expect(compareTask(task(),undefined,false)).toBe('当前页未加载');expect(compareTask(task(),undefined,true)).toBe('已移出范围');
  const baseline=task();const current=task({status:'accepted',plan:{...baseline.plan,due_date:'2026-11-01'}});
  expect(compareTask(baseline,current,true)).toBe('排期、交付状态已变化');expect(baseline.status).toBe('in_review');expect(baseline.plan.due_date).toBe('2026-10-23');
 });
});
describe('organization navigation and real identities',()=>{
 const session:Session={user:{id:'user-a',name:'A',email:'a@example.test'},organization:{id:'org-b',name:'B'},projects:[{id:'project-b',version:1}]};
 it('restores an accessible saved project route when returning to an organization',()=>{
  const current=parseWorkbenchRoute('?project=project-a&view=development&sub=runs&run=old-run');
  const saved=parseWorkbenchRoute('?project=project-b&view=development&sub=planning&layout=baselines&record=baseline-b');
  expect(switchedRoute(current,session,saved)).toEqual(saved);
  expect(switchedRoute(current,session)).toMatchObject({project:'project-b',run:'',record:'',sub:'runs'});
  expect(switchedRoute(saved,session)).toEqual(saved);
 });
 it('preserves deep links to planning and governance across a refresh',()=>{
  expect(parseWorkbenchRoute('?project=p&view=development&sub=planning&layout=capacity')).toMatchObject({project:'p',sub:'planning',layout:'capacity'});
  expect(parseWorkbenchRoute('?view=settings&sub=teams&record=team-a')).toMatchObject({view:'settings',sub:'teams',record:'team-a'});
 });
 it('uses org user IDs for human team membership and project resource IDs for agents/tools',()=>{
  expect(teamIdentity('human','org-user','irrelevant-project')).toEqual({resource_kind:'human',resource_id:'org-user',project_id:''});
  expect(teamIdentity('agent','resource-a','project-a')).toEqual({resource_kind:'agent',resource_id:'resource-a',project_id:'project-a'});
  expect(recordPayload('members',{kind:'human',name:'Real account',user_id:'org-user',email:'a@example.test'})).toEqual({kind:'human',name:'Real account',data:{user_id:'org-user',email:'a@example.test'}});
 });
 it('excludes revoked project access while preserving owner/admin inherited authority',()=>{
  const people=['member','owner','admin'].map((role,index)=>({user_id:'u'+index,name:role,email:role+'@example.test',role,version:1}));
  expect(eligiblePeople(people,[{project_id:'p',user_id:'u0',name:'member',email:'',role:'',version:2,status:'removed'}]).map(p=>p.user_id)).toEqual(['u1','u2']);
  expect(eligiblePeople(people,[{project_id:'p',user_id:'u0',name:'member',email:'',role:'developer',version:3,status:'active'}])).toHaveLength(3);
 });
});
describe('invitation secrets and one-time acceptance',()=>{
 it('only consumes the URL fragment and constructs a fragment-only handoff link',()=>{
  const url=invitationURL('https://suma.example','/','secret/&=');const parsed=new URL(url);
  expect(parsed.search).toBe('');expect(invitationFragment(parsed.hash)).toBe('secret/&=');expect(invitationFragment('#other=value')).toBe('');
 });
 it('submits password proof to the public acceptance API using a request body',async()=>{
  const response={user:{id:'u',name:'User',email:'u@example.test'},organization:{id:'org',name:'Org'},projects:[]};
  const fetcher=vi.fn().mockResolvedValue(new Response(JSON.stringify(response),{status:200}));vi.stubGlobal('fetch',fetcher);
  const request=new InvitationAcceptanceRequest();expect(await request.accept({token:'one-time',email:'u@example.test',name:'User',password:'proof-password'})).toEqual(response);
  expect(fetcher.mock.calls[0][0]).toBe('/api/v1/auth/invitations/accept');expect(JSON.parse(fetcher.mock.calls[0][1].body)).toMatchObject({token:'one-time',email:'u@example.test',password:'proof-password',idempotency_key:expect.any(String)});
 });
 it('refuses automatic resubmission after a lost acceptance response',async()=>{
  const fetcher=vi.fn().mockRejectedValue(new TypeError('lost'));vi.stubGlobal('fetch',fetcher);const request=new InvitationAcceptanceRequest();
  const values={token:'single-use',email:'u@example.test',name:'U',password:'proof'};
  await expect(request.accept(values)).rejects.toMatchObject({status:0});expect(request.unknown).toBe(true);
  await expect(request.accept(values)).rejects.toMatchObject({code:'invitation_outcome_unknown'});expect(fetcher).toHaveBeenCalledTimes(1);
 });
 it('allows a corrected identity after a definite rejection with a fresh operation',async()=>{
  const fetcher=vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({error:{code:'invalid_credentials',message:'密码错误'}}),{status:401})).mockResolvedValueOnce(new Response(JSON.stringify({projects:[]}),{status:200}));vi.stubGlobal('fetch',fetcher);
  const request=new InvitationAcceptanceRequest();await expect(request.accept({token:'t',email:'u@example.test',name:'U',password:'wrong'})).rejects.toMatchObject({status:401});expect(request.unknown).toBe(false);
  await request.accept({token:'t',email:'u@example.test',name:'U',password:'correct'});
  const bodies=fetcher.mock.calls.map(call=>JSON.parse(call[1].body));expect(bodies[1].idempotency_key).not.toBe(bodies[0].idempotency_key);expect(bodies[1].password).toBe('correct');
 });
});


describe('server-frozen execution recovery and workspace evidence',()=>{
 it('uses the actual run node retry declaration even when a dynamic node has no editable definition',()=>{
  expect(canRetryNode({state:'failed',idempotent:true})).toBe(true);
  expect(canRetryNode({state:'failed',idempotent:false})).toBe(false);
  expect(canRetryNode({state:'failed'})).toBe(false);expect(canRetryNode({state:'running',idempotent:true})).toBe(false);
 });
 it('shows only the latest cumulative output snapshot for the current attempt and fence',()=>{
  const events=[{sequence:1,type:'node_progress',node_id:'dynamic:1',data:{attempt:1,fence:1,message:'older attempt'}},{sequence:2,type:'node_progress',node_id:'dynamic:1',data:{attempt:2,fence:3,message:'first snapshot'}},{sequence:3,type:'node_progress',node_id:'dynamic:1',data:{attempt:2,fence:3,message:'latest cumulative snapshot'}},{sequence:4,type:'node_progress',node_id:'dynamic:1',data:{attempt:2,fence:2,message:'stale fence'}}];
  expect(latestProgress(events,{id:'dynamic:1',attempt:2,fence:3,state:'running'})).toBe('latest cumulative snapshot');
  expect(latestProgress(events,{id:'dynamic:1',attempt:3,fence:4,state:'running'})).toBeUndefined();
 });
 it('renders actual commit and patch evidence with bounded path coverage and explicit merge status',()=>{
  const html=renderToStaticMarkup(createElement(WorkspaceEvidence,{evidence:{base_commit:'base123',head_commit:'head456',diff_sha256:'patch789',evidence_sha256:'receipt',changed_paths:['src/real.ts'],changed_paths_total:201,changed_paths_truncated:true}}));
  for(const value of ['base123','head456','patch789','src/real.ts','201','清单已截断','未在此回执中记录'])expect(html).toContain(value);
  const absent=renderToStaticMarkup(createElement(WorkspaceEvidence,{evidence:{base_commit:'base'}}));expect(absent).toContain('服务未提供路径清单');
 });
});
