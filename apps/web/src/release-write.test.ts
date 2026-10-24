import {afterEach,describe,expect,it,vi} from 'vitest';
import {ReleaseWriteRecovery} from './release-write';
const storage=()=>{const values=new Map<string,string>();return {getItem:(key:string)=>values.get(key)||null,setItem:(key:string,value:string)=>{values.set(key,value);},removeItem:(key:string)=>{values.delete(key);}};};
afterEach(()=>vi.unstubAllGlobals());
describe('release write durable request consumer',()=>{
 it('restores the original request and idempotency key after a lost receipt and page refresh',async()=>{
  const store=storage(),first=new ReleaseWriteRecovery('org:user:project',store),fetcher=vi.fn().mockRejectedValueOnce(new TypeError('disconnected')).mockResolvedValueOnce(new Response(JSON.stringify({operation_id:'op-real',version:1}),{status:201}));vi.stubGlobal('fetch',fetcher);
  await expect(first.run('/release-plans/plan-real/execute',{project_id:'project',expected_version:4,plan_revision:1,plan_sha256:'frozen'})).rejects.toMatchObject({status:0});
  const recovered=new ReleaseWriteRecovery('org:user:project',store);expect(recovered.pending?.path).toBe('/release-plans/plan-real/execute');
  await recovered.run('/release-plans/other/execute',{project_id:'project',expected_version:9});
  expect(fetcher.mock.calls[1][0]).toBe('/api/v1/release-plans/plan-real/execute');expect(JSON.parse(fetcher.mock.calls[1][1].body)).toEqual(JSON.parse(fetcher.mock.calls[0][1].body));expect(recovered.pending).toBeUndefined();
 });
 it('keeps a previously unknown operation pending when current permissions prevent restoring its receipt',async()=>{
  const store=storage(),request=new ReleaseWriteRecovery('permission-lost',store),fetcher=vi.fn().mockRejectedValueOnce(new TypeError('lost')).mockResolvedValueOnce(new Response(JSON.stringify({error:{code:'forbidden',message:'权限已变化'}}),{status:403}));vi.stubGlobal('fetch',fetcher);
  await expect(request.run('/release-plans/p/execute',{project_id:'project',expected_version:2})).rejects.toMatchObject({status:0});
  await expect(request.restore()).rejects.toMatchObject({status:403});expect(request.pending?.body.expected_version).toBe(2);
 });
 it('does not retain a first request definitely rejected before an unknown outcome',async()=>{
  const request=new ReleaseWriteRecovery('definite-rejection',storage());vi.stubGlobal('fetch',vi.fn().mockResolvedValue(new Response(JSON.stringify({error:{code:'forbidden',message:'权限不足'}}),{status:403})));
  await expect(request.run('/environments',{project_id:'p',name:'环境'})).rejects.toMatchObject({status:403});expect(request.pending).toBeUndefined();
 });
 it('scopes pending recovery to the exact organization, account and project',async()=>{
  const store=storage(),request=new ReleaseWriteRecovery('organization-a:user-a:project-a',store);vi.stubGlobal('fetch',vi.fn().mockRejectedValue(new TypeError('lost')));
  await expect(request.run('/environments',{project_id:'project-a'})).rejects.toMatchObject({status:0});
  expect(new ReleaseWriteRecovery('organization-b:user-a:project-a',store).pending).toBeUndefined();
  expect(new ReleaseWriteRecovery('organization-a:user-b:project-a',store).pending).toBeUndefined();
  expect(new ReleaseWriteRecovery('organization-a:user-a:project-b',store).pending).toBeUndefined();
 });
 it('refuses to persist or submit secret endpoints and credential fields',async()=>{
  const request=new ReleaseWriteRecovery('secret',storage()),fetcher=vi.fn();vi.stubGlobal('fetch',fetcher);
  await expect(request.run('/release-workers',{project_id:'p'})).rejects.toThrow('恢复');
  await expect(request.run('/environments',{project_id:'p',authorization_ticket:'secret'})).rejects.toThrow('秘密');
  expect(fetcher).not.toHaveBeenCalled();expect(request.pending).toBeUndefined();
 });
 it('saves the complete recovery envelope before dispatch and stops before network when storage is unavailable',async()=>{
  const store=storage(),request=new ReleaseWriteRecovery('before-network',store);vi.stubGlobal('fetch',vi.fn(async()=>{expect(new ReleaseWriteRecovery('before-network',store).pending?.body).toMatchObject({project_id:'p',name:'原草稿'});return new Response('{}',{status:201});}));
  await request.run('/environments',{project_id:'p',name:'原草稿'});
  const fetcher=vi.fn();vi.stubGlobal('fetch',fetcher);const broken=new ReleaseWriteRecovery('unavailable',{...store,setItem:()=>{throw new Error('storage disabled');}});
  await expect(broken.run('/environments',{project_id:'p'})).rejects.toThrow('未发送');expect(fetcher).not.toHaveBeenCalled();
 });
 it('reports an unreadable saved envelope before accepting any new write',async()=>{
  const store=storage();store.setItem('suma-release-recovery-v1:corrupt','{partial');const request=new ReleaseWriteRecovery('corrupt',store),fetcher=vi.fn();vi.stubGlobal('fetch',fetcher);
  expect(request.error).toContain('恢复记录无法读取');await expect(request.run('/environments',{project_id:'p'})).rejects.toThrow('未发送');expect(fetcher).not.toHaveBeenCalled();
 });
});
