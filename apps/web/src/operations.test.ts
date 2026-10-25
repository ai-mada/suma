import {afterEach,describe,expect,it,vi} from 'vitest';
import {mutate} from './operations';
import {bindApiView} from './api-context';

afterEach(()=>vi.unstubAllGlobals());

describe('recoverable immutable write envelope',()=>{
 it('preserves a persisted key across a fresh command scope and an edited nested form after a lost receipt',async()=>{
  const fetcher=vi.fn().mockRejectedValueOnce(new TypeError('connection lost')).mockResolvedValueOnce(new Response('{"ok":true}'));
  vi.stubGlobal('fetch',fetcher);
  const body={project_id:'p',idempotency_key:'persisted-operation-2026',expected_version:3,spec:{name:'nightly',workflow_version:2}};
  await expect(mutate('/automations/r/commands',body,'POST','immutable-recovery')).rejects.toMatchObject({status:0});
  body.spec.name='edited after uncertain save';body.spec.workflow_version=8;body.expected_version=4;
  await mutate('/automations/r/commands',body,'POST','immutable-recovery');
  const first=JSON.parse(fetcher.mock.calls[0][1].body),replayed=JSON.parse(fetcher.mock.calls[1][1].body);
  expect(replayed).toEqual(first);
  expect(replayed).toMatchObject({idempotency_key:'persisted-operation-2026',expected_version:3,spec:{name:'nightly',workflow_version:2}});
 });
 it('allows an explicit new operation after a definite conflict without retaining the rejected body',async()=>{
  const fetcher=vi.fn().mockResolvedValueOnce(new Response('{"error":{"code":"version_conflict","message":"reload"}}',{status:409})).mockResolvedValueOnce(new Response('{"ok":true}'));
  vi.stubGlobal('fetch',fetcher);
  await expect(mutate('/automations/r/commands',{idempotency_key:'known-conflict-key',expected_version:3},'POST','definite-conflict')).rejects.toMatchObject({status:409});
  await mutate('/automations/r/commands',{idempotency_key:'rebased-operation-key',expected_version:4},'POST','definite-conflict');
  expect(JSON.parse(fetcher.mock.calls[1][1].body)).toEqual({idempotency_key:'rebased-operation-key',expected_version:4});
 });
 it.each([403,409])('retains the original uncertain command after a later %s refusal and releases it only on success',async(status)=>{
  let attempt=0;
  const fetcher=vi.fn(async()=>{
   attempt++;
   if(attempt===1)throw new TypeError('receipt lost after sending');
   if(attempt===2)return Response.json({error:{code:'current_refusal',message:'Original operation remains unknown'}},{status});
   return Response.json({ok:true});
  });
  vi.stubGlobal('fetch',fetcher);
  const session=(name:string)=>({user:{id:'user-'+name,name,email:name+'@example.test'},organization:{id:'org-'+name,name},projects:[]});
  const original=bindApiView(session('A')),later=bindApiView(session('B')),identity='sticky-unknown-refusal-'+status;
  const body={project_id:'p',idempotency_key:'original-operation-'+status,expected_version:3,nested:{name:'reviewed A body'}};
  await expect(mutate('/tasks',body,'POST',identity,original)).rejects.toMatchObject({status:0});
  body.idempotency_key='edited-operation-'+status;body.expected_version=9;body.nested.name='edited in later view';
  await expect(mutate('/other-task',body,'PATCH',identity,later)).rejects.toMatchObject({status});
  await mutate('/other-task',body,'PATCH',identity,later);
  const calls=fetcher.mock.calls as unknown as [string,RequestInit][];
  expect(calls).toHaveLength(3);
  const first=JSON.parse(String(calls[0][1].body));
  for(const [path,init]of calls){
   expect(path).toBe('/api/v1/tasks');expect(init.method).toBe('POST');
   expect(JSON.parse(String(init.body))).toEqual(first);
   const headers=new Headers(init.headers);
   expect(headers.get('X-Suma-Expected-User')).toBe('user-A');
   expect(headers.get('X-Suma-Expected-Organization')).toBe('org-A');
  }
  expect(first).toMatchObject({idempotency_key:'original-operation-'+status,expected_version:3,nested:{name:'reviewed A body'}});
  // Success settles the old operation; a subsequent explicit command may use B.
  await mutate('/other-task',body,'PATCH',identity,later);
  const next=fetcher.mock.calls[3] as unknown as [string,RequestInit];
  expect(next[0]).toBe('/api/v1/other-task');expect(next[1].method).toBe('PATCH');
  expect(JSON.parse(String(next[1].body))).toEqual(body);
  expect(new Headers(next[1].headers).get('X-Suma-Expected-User')).toBe('user-B');
 });

});
