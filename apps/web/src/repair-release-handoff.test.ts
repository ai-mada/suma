import {beforeEach,expect,it,vi} from 'vitest';
import type {ApiCaller} from './api';
import {parseWorkbenchRoute} from './navigation';
import {repairReleaseCandidateRequest,repairReleaseHref,repairReleaseReview,repairReleaseTarget} from './repair-release-handoff';
import {ReleaseWriteRecovery} from './release-write';

const handoff={issue_id:'issue-a',issue_version:5,repair_id:'repair-a',repair_version:3,cycle:1,task_id:'task-a',task_version:4,run_id:'run-a',review_id:'review-a',artifact_id:'artifact-a',build_attestation_id:'proof-build',package_test_attestation_id:'proof-package'};
const target={issue:'issue-a',cycle:1};
const project='project-a';
const current={
 project_id:project,issue_id:target.issue,id:handoff.repair_id,version:3,task_id:handoff.task_id,cycle:1,
 closure:{task_version:4,run_id:'run-a',review_id:'review-a',package_regression:{artifact_id:'artifact-a',proof_id:'proof-package',manifest_sha256:'a'.repeat(64),source_commit:'b'.repeat(40)}}
};
const view={issue:{id:target.issue,project_id:project,status:'closed',version:5},current,task:{project_id:project,id:handoff.task_id,version:4,status:'accepted',title:'Verified task',run_id:'run-a'},cycle:1,release_handoff:handoff};
const body=repairReleaseCandidateRequest(project,'Repair candidate',handoff);
const response={candidate:{...body,id:'candidate-a',repair_handoff:handoff,tasks:[{id:handoff.task_id,version:handoff.task_version,run_id:handoff.run_id,review_id:handoff.review_id}]}};

beforeEach(()=>{vi.restoreAllMocks();});

it('locates the repair cycle without transferring authority in the URL',()=>{
 const href=repairReleaseHref(project,target);
 const route=parseWorkbenchRoute(href);
 expect(route).toMatchObject({project,view:'operations',sub:'release-candidates',record:'',panel:'repair-release:issue-a:1'});
 expect(repairReleaseTarget(route.panel)).toEqual(target);
 expect(href).not.toContain('artifact-a');
 for(const panel of ['repair-release:issue-a:0','repair-release:issue-a:01','repair-release:issue-a:2147483647','repair-release:../a:1'])expect(repairReleaseTarget(panel)).toBeUndefined();
});

it('requires the same current accepted task and sealed proof under the selected project and cycle',()=>{
 expect(repairReleaseReview(view,project,target)).toMatchObject({handoff,taskTitle:'Verified task'});
 expect(repairReleaseReview(view,'other-project',target)).toBeUndefined();
 expect(repairReleaseReview(view,project,{...target,cycle:2})).toBeUndefined();
 expect(repairReleaseReview({...view,issue:{...view.issue,status:'open'}},project,target)).toBeUndefined();
 expect(repairReleaseReview({...view,task:{...view.task,version:6}},project,target)).toBeUndefined();
 expect(repairReleaseReview({...view,release_handoff:{...handoff,package_test_attestation_id:'other'}},project,target)).toBeUndefined();
 expect(body).toMatchObject({artifact_id:'artifact-a',tasks:[{id:'task-a',expected_version:4}],repair_handoff:{issue_id:'issue-a',expected_issue_version:5,expected_repair_version:3}});
});

it('preserves the exact original candidate request if a returned receipt does not match',async()=>{
 const saved=new Map<string,string>();
 const storage={getItem:(key:string)=>saved.get(key)??null,setItem:(key:string,value:string)=>{saved.set(key,value);},removeItem:(key:string)=>{saved.delete(key);}};
 const send=vi.fn()
  .mockResolvedValueOnce({...response,candidate:{...response.candidate,repair_handoff:{...handoff,issue_id:'different-issue'}}})
  .mockResolvedValueOnce(response);
 const original=new ReleaseWriteRecovery('repair',storage,send as unknown as ApiCaller);
 await expect(original.run('/release-candidates',body)).rejects.toThrow();
 const request=original.pending;
 expect(request?.body).toEqual(body);
 const restored=new ReleaseWriteRecovery('repair',storage,send as unknown as ApiCaller);
 await expect(restored.restore()).resolves.toEqual(response);
 expect(send.mock.calls[1]).toEqual(send.mock.calls[0]);
 expect(restored.pending).toBeUndefined();
});
