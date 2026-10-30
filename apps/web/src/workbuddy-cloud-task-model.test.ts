import {webcrypto} from 'node:crypto';
import {beforeEach,afterEach,expect,it,vi} from 'vitest';
import type {CloudNodeView,CloudDelegateRequest} from '../../../packages/contracts/src';
import {cloudRequestDigest,readCloudNode,readCloudSubmission,validCloudRecovery,validCloudReceipt,type CloudTaskScope} from './workbuddy-cloud-task-model';
import type {WriteRecoveryRequest} from './write-action';

const scope:CloudTaskScope={project:'project-a',runID:'run-a',nodeID:'node-a',userID:'user-a',organizationID:'org-a'},stamp='2026-10-30T00:00:00Z';
const base=async():Promise<CloudNodeView>=>{const binding={connection_id:'connection-a',grant_generation:1,request:{prompt:'Original <task> & request\n\u2028',name:'Cloud task'}};return {project_id:scope.project,run_id:scope.runID,node_id:scope.nodeID,run_version:3,node_spec_sha256:'a'.repeat(64),request_sha256:await cloudRequestDigest(binding.request),binding,allowed_actions:['delegate']};};
const delegation=(node:CloudNodeView)=>({id:'delegation-a',project_id:scope.project,owner_id:scope.userID,connection_id:'connection-a',version:2,run_id:scope.runID,node_id:scope.nodeID,node_spec_sha256:node.node_spec_sha256,connection_version_at_issue:3,grant_generation:1,client_binding_sha256:'b'.repeat(64),request_sha256:node.request_sha256,expires_at:'2030-01-01T00:00:00Z',max_creates:1,creates_consumed:1,state:'active',created_at:stamp,updated_at:stamp});
const submission=(node:CloudNodeView)=>({id:'submission-a',project_id:scope.project,delegation_id:'delegation-a',version:3,run_id:scope.runID,node_id:scope.nodeID,assignment:{run_id:scope.runID,node_id:scope.nodeID,attempt:1,fence:1,worker_id:'worker-a'},request_sha256:node.request_sha256,state:'task_known',task:{task_id:'vendor-task-a',status:'completed'},observed_at:stamp,last_error:'',usage_state:'unreported',created_at:stamp,updated_at:stamp});
beforeEach(()=>{vi.stubGlobal('crypto',webcrypto);});afterEach(()=>vi.unstubAllGlobals());

it('validates the exact source and Go-encoded request digest, rejecting extra secret or completion structures',async()=>{
 const node=await base();expect(await readCloudNode(node,scope)).toEqual(node);
 for(const value of [{...node,run_id:'other-run'},{...node,request_sha256:'b'.repeat(64)},{...node,token:'secret'},{...node,binding:{...node.binding,url:'https://unexpected.example'}},{...node,candidate:{accepted:true}}])expect(await readCloudNode(value,scope)).toBeUndefined();
 expect(await cloudRequestDigest({prompt:'<>&\u2028\u2029'})).toBe('4feb27e6926facc27a59e46004352d4173998dd46d580431d7e07dcc9892755e');
});

it('keeps shared submission facts readable without exposing another persons private delegation',async()=>{
 const node=await base();const full={...node,delegation:delegation(node),submission:submission(node),allowed_actions:['revoke','observe']};
 expect(await readCloudNode(full,scope)).toBeDefined();expect(await readCloudNode({...full,delegation:{...full.delegation,owner_id:'another-user'}},scope)).toBeUndefined();
 expect(await readCloudNode({...node,submission:submission(node),allowed_actions:[]},scope)).toBeDefined();
 expect(await readCloudNode({...node,submission:submission(node),allowed_actions:['delegate']},scope)).toBeUndefined();
});

it('refuses changed assignment, supplier identity material and invented usage facts',async()=>{
 const node=await base(),item=submission(node);
 expect(readCloudSubmission(item,scope)).toBeDefined();
 for(const value of [{...item,assignment:{...item.assignment,node_id:'other-node'}},{...item,task:{...item.task,token:'secret'}},{...item,usage_state:'reported'},{...item,cost:0},{...item,state:'unknown'},{...item,task:{task_id:'a/b',status:'working'}}])expect(readCloudSubmission(value,scope)).toBeUndefined();
});

it('only restores the three exact safe commands and never stores a prompt or arbitrary path',async()=>{
 const node=await base(),d=delegation(node);const body:CloudDelegateRequest={project_id:scope.project,connection_id:'connection-a',expected_connection_version:3,grant_generation:1,run_id:scope.runID,node_id:scope.nodeID,expected_run_version:3,node_spec_sha256:node.node_spec_sha256,request_sha256:node.request_sha256,expires_at:'2030-01-01T00:00:00Z',max_creates:1,idempotency_key:'original-operation'};
 const request:WriteRecoveryRequest={path:'/workbuddy-cloud-delegations',method:'POST',idempotency_key:body.idempotency_key,body:{...body}};
 expect(validCloudRecovery(request,scope,node)).toBe(true);expect(validCloudRecovery({...request,body:{...body,prompt:'not persisted'}},scope,node)).toBe(false);
 expect(validCloudRecovery({...request,body:{...body,node_id:'other-node'}},scope,node)).toBe(false);
 const revoke={path:'/workbuddy-cloud-delegations/delegation-a/revoke',method:'POST',idempotency_key:'revoke-operation',body:{project_id:scope.project,expected_version:1,idempotency_key:'revoke-operation'}};
 expect(validCloudRecovery(revoke,scope)).toBe(true);expect(validCloudRecovery(revoke,scope,{...node,delegation:d})).toBe(true);
 expect(validCloudRecovery({...revoke,path:'/workbuddy-cloud-delegations/other/revoke'},scope,{...node,delegation:d})).toBe(false);
 expect(validCloudRecovery({...revoke,path:'/runner/workbuddy-cloud-submissions'},scope,node)).toBe(false);
});

it('accepts advanced original receipts while preserving exact node, delegation and known supplier task binding',async()=>{
 const node=await base(),d=delegation(node),s=submission(node),source={...node,delegation:d,submission:s,allowed_actions:['revoke','observe']};
 const revoke:WriteRecoveryRequest={path:'/workbuddy-cloud-delegations/delegation-a/revoke',method:'POST',idempotency_key:'revoke-operation',body:{project_id:scope.project,expected_version:1,idempotency_key:'revoke-operation'}};
 expect(validCloudReceipt({...d,state:'revoked'},revoke,scope,source)).toBe(true);expect(validCloudReceipt({...d,id:'other',state:'revoked'},revoke,scope,source)).toBe(false);
 const observe={...revoke,path:'/workbuddy-cloud-submissions/submission-a/observe'};
 expect(validCloudReceipt(s,observe,scope,source)).toBe(true);expect(validCloudReceipt({...s,task:{...s.task,task_id:'replacement'}},observe,scope,source)).toBe(false);
 expect(validCloudReceipt({...s,assignment:{...s.assignment,attempt:2}},observe,scope,source)).toBe(false);
 expect(validCloudReceipt({...d,state:'revoked',connection_id:'other-connection'},revoke,scope,source)).toBe(false);
});

it('accepts supplier RFC3339 offsets in exact node reads and original receipts while rejecting malformed zones',async()=>{
 const node=await base(),d=delegation(node),item=submission(node);
 const task={...item.task,created_at:'2026-10-30T16:00:00+08:00',updated_at:'2026-10-30T01:30:00.123456789-07:00'};
 const source={...node,delegation:d,submission:{...item,task},allowed_actions:['revoke','observe']};
 const request:WriteRecoveryRequest={path:'/workbuddy-cloud-submissions/submission-a/observe',method:'POST',idempotency_key:'observe-offset-operation',body:{project_id:scope.project,expected_version:1,idempotency_key:'observe-offset-operation'}};
 expect(await readCloudNode(source,scope)).toBeDefined();expect(validCloudReceipt(source.submission,request,scope,source)).toBe(true);
 for(const stamp of ['2026-10-30T16:00:00','2026-10-30T16:00:00+08','2026-10-30T16:00:00+08:60','2026-10-30T16:00:00+24:00','2026-02-30T16:00:00+08:00']){
  const changed={...source,submission:{...source.submission,task:{...task,updated_at:stamp}}};
  expect(await readCloudNode(changed,scope)).toBeUndefined();expect(validCloudReceipt(changed.submission,request,scope,source)).toBe(false);
 }
 expect(await readCloudNode({...source,submission:{...source.submission,observed_at:'2026-10-30T16:00:00+08:00'}},scope)).toBeUndefined();
});
