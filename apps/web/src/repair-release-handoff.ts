import type {IssueRepairReleaseHandoff} from '../../../packages/contracts/src';

export type RepairReleaseTarget={issue:string;cycle:number};
export type RepairReleaseReview={
 handoff:IssueRepairReleaseHandoff;
 taskTitle:string;
 manifest:string;
 commit:string;
};

const id=(value:unknown):value is string=>typeof value==='string'&&/^[A-Za-z0-9_-]{1,64}$/.test(value);
const positive=(value:unknown):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>0&&value<=2147483646;
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const digest=(value:unknown):value is string=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
const revision=(value:unknown):value is string=>typeof value==='string'&&/^(?:[a-f0-9]{40}|[a-f0-9]{64})$/.test(value);

export function repairReleaseTarget(panel:string):RepairReleaseTarget|undefined{
 const match=/^repair-release:([A-Za-z0-9_-]{1,64}):([1-9][0-9]{0,9})$/.exec(panel);
 if(!match)return;
 const cycle=Number(match[2]);
 if(positive(cycle))return {issue:match[1],cycle};
}

export function repairReleaseHref(project:string,target:RepairReleaseTarget){
 return '?'+new URLSearchParams({project,view:'operations',sub:'release-candidates',panel:'repair-release:'+target.issue+':'+target.cycle});
}

export function repairReleaseIssueHref(project:string,issue:string,cycle?:number){
 return '?'+new URLSearchParams({project,view:'quality',sub:'issues',record:issue,...(positive(cycle)?{panel:'repair-cycle:'+cycle}:{})});
}

export function issueRepairCycleTarget(panel:string){
 const match=/^repair-cycle:([1-9][0-9]{0,9})$/.exec(panel);
 if(!match)return;
 const cycle=Number(match[1]);
 if(positive(cycle))return cycle;
}

export function validRepairReleaseHandoff(value:unknown):value is IssueRepairReleaseHandoff{
 if(!object(value))return false;
 return ['issue_id','repair_id','task_id','run_id','review_id','artifact_id','build_attestation_id','package_test_attestation_id'].every(key=>id(value[key]))&&
  ['issue_version','repair_version','task_version','cycle'].every(key=>positive(value[key]))&&
  value.build_attestation_id!==value.package_test_attestation_id;
}

/** The URL locates one cycle. Only the current protected read supplies evidence. */
export function repairReleaseReview(value:unknown,project:string,target:RepairReleaseTarget):RepairReleaseReview|undefined{
 if(!object(value)||!object(value.issue)||!object(value.current)||!object(value.task))return;
 const {issue,current,task}=value,closure=current.closure,handoff=value.release_handoff;
 if(!validRepairReleaseHandoff(handoff)||!object(closure)||!object(closure.package_regression))return;
 const proof=closure.package_regression;
 if(value.cycle!==target.cycle||handoff.cycle!==target.cycle||issue.id!==target.issue||handoff.issue_id!==target.issue||
    issue.project_id!==project||issue.status!=='closed'||issue.version!==handoff.issue_version||
    current.project_id!==project||current.issue_id!==target.issue||current.cycle!==target.cycle||current.id!==handoff.repair_id||current.version!==handoff.repair_version||
    current.task_id!==handoff.task_id||task.project_id!==project||task.id!==handoff.task_id||task.version!==handoff.task_version||
    task.status!=='accepted'||task.run_id!==handoff.run_id||typeof task.title!=='string'||
    closure.task_version!==handoff.task_version||closure.run_id!==handoff.run_id||closure.review_id!==handoff.review_id||
    proof.artifact_id!==handoff.artifact_id||proof.proof_id!==handoff.package_test_attestation_id||!digest(proof.manifest_sha256)||!revision(proof.source_commit))return;
 return {handoff,taskTitle:task.title,manifest:proof.manifest_sha256,commit:proof.source_commit};
}

export function repairReleaseCandidateRequest(project:string,name:string,handoff:IssueRepairReleaseHandoff){
 return {project_id:project,name:name.trim(),artifact_id:handoff.artifact_id,build_attestation_id:handoff.build_attestation_id,
  package_test_attestation_id:handoff.package_test_attestation_id,tasks:[{id:handoff.task_id,expected_version:handoff.task_version}],
  repair_handoff:{issue_id:handoff.issue_id,expected_issue_version:handoff.issue_version,expected_repair_version:handoff.repair_version}};
}

export function matchingRepairReleaseReceipt(value:unknown,body:Record<string,unknown>){
 if(!object(body.repair_handoff)||!Array.isArray(body.tasks)||body.tasks.length!==1||!object(body.tasks[0])||
    !object(value)||!object(value.candidate))return false;
 const request=body.repair_handoff,task=body.tasks[0],candidate=value.candidate,handoff=candidate.repair_handoff;
 if(!validRepairReleaseHandoff(handoff)||!id(candidate.id)||candidate.project_id!==body.project_id||
    candidate.name!==body.name||candidate.artifact_id!==body.artifact_id||
    candidate.build_attestation_id!==body.build_attestation_id||candidate.package_test_attestation_id!==body.package_test_attestation_id||
    handoff.issue_id!==request.issue_id||handoff.issue_version!==request.expected_issue_version||handoff.repair_version!==request.expected_repair_version||
    handoff.artifact_id!==body.artifact_id||handoff.build_attestation_id!==body.build_attestation_id||
    handoff.package_test_attestation_id!==body.package_test_attestation_id||handoff.task_id!==task.id||handoff.task_version!==task.expected_version)return false;
 return Array.isArray(candidate.tasks)&&candidate.tasks.length===1&&object(candidate.tasks[0])&&candidate.tasks[0].id===handoff.task_id&&
  candidate.tasks[0].version===handoff.task_version&&candidate.tasks[0].run_id===handoff.run_id&&candidate.tasks[0].review_id===handoff.review_id;
}
