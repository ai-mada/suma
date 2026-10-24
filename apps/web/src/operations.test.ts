import {afterEach,describe,expect,it,vi} from 'vitest';
import {mutate} from './operations';

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
});
