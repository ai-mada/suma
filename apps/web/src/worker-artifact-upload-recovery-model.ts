import type {ExecutionWorker} from './tool-model';
import type {WriteRecoveryRequest} from './write-action';

export type UploadRecoveryKind='artifact-uploads'|'candidate-transfers';
type TransferEvidence={
  run_id:string;node_id:string;attempt:number;fence:string;created_at:string;size_bytes:number;
  attempt_state:string;disposition:'eligible'|'awaiting_attempt'|'unverified';
};
type ArtifactUpload=TransferEvidence&{upload_id:string;manifest_sha256:string};
type CandidateTransfer=TransferEvidence&{transfer_id:string;bundle_sha256:string;base_commit:string;head_commit:string};
export type UploadRecoveryItem=TransferEvidence&{id:string;sha256:string;commits?:{base:string;head:string}};
export type UploadRecoveryView={
  project_id:string;worker_registry_id:string;worker_id:string;worker_version:number;
  items:ArtifactUpload[];next_cursor:string;
};
export type UploadAbandonReceipt={
  project_id:string;worker_registry_id:string;operation_id:string;state:'abandoned';
}&({upload_id:string}|{transfer_id:string});

const identifier=/^[a-f0-9]{32}$/;
const digest=/^[a-f0-9]{64}$/;
const commit=/^(?:[a-f0-9]{40}|[a-f0-9]{64})$/;
const operation=/^[A-Za-z0-9][A-Za-z0-9._-]{7,127}$/;
const abandonRoute=/^\/release-workers\/([a-f0-9]{32})\/(artifact-uploads|candidate-transfers)\/([a-f0-9]{32})\/abandon$/;
const commonFields=['run_id','node_id','attempt','fence','created_at','size_bytes','attempt_state','disposition'];
const states=['running','waiting','reconciling','succeeded','failed','cancelled','unverified'];
const dispositions=['eligible','awaiting_attempt','unverified'];
const isObject=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const matches=(value:unknown,pattern:RegExp):value is string=>typeof value==='string'&&pattern.test(value);
const hasFields=(value:Record<string,unknown>,fields:string[])=>Object.keys(value).length===fields.length&&fields.every(field=>Object.hasOwn(value,field));
const integer=(value:unknown,min:number,max:number):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>=min&&value<=max;
const version=(value:unknown)=>integer(value,1,2147483647);
const publicID=(value:unknown,max:number)=>typeof value==='string'&&value.length>=1&&value.length<=max&&!/[\u0000-\u001f\u007f]/.test(value);
const digestField=(kind:string)=>kind==='candidate-transfers'?'expected_bundle_sha256':'expected_manifest_sha256';

export function uploadRecoveryPath(worker:ExecutionWorker,kind:UploadRecoveryKind,cursor:string){
  const params=new URLSearchParams({project_id:worker.project_id});
  if(cursor)params.set('cursor',cursor);
  return '/release-workers/'+encodeURIComponent(worker.id)+'/'+kind+'?'+params.toString();
}
export function uploadAbandonPath(worker:ExecutionWorker,kind:UploadRecoveryKind,id:string){
  return '/release-workers/'+encodeURIComponent(worker.id)+'/'+kind+'/'+encodeURIComponent(id)+'/abandon';
}
export function uploadAbandonBasis(worker:ExecutionWorker,kind:UploadRecoveryKind,item:UploadRecoveryItem){
  return {project_id:worker.project_id,expected_worker_version:worker.version,[digestField(kind)]:item.sha256,acknowledge_incomplete:true};
}

function validEvidence(value:Record<string,unknown>){
  return publicID(value.run_id,100)&&publicID(value.node_id,150)&&version(value.attempt)&&matches(value.fence,/^[1-9][0-9]{0,18}$/)&&
    typeof value.created_at==='string'&&Number.isFinite(Date.parse(value.created_at))&&
    typeof value.attempt_state==='string'&&states.includes(value.attempt_state)&&
    typeof value.disposition==='string'&&dispositions.includes(value.disposition);
}
function validArtifactUpload(value:unknown):value is ArtifactUpload{
  return isObject(value)&&hasFields(value,[...commonFields,'upload_id','manifest_sha256'])&&validEvidence(value)&&
    matches(value.upload_id,identifier)&&matches(value.manifest_sha256,digest)&&integer(value.size_bytes,0,1073741824)&&
    (value.disposition!=='eligible'||['succeeded','failed','cancelled'].includes(String(value.attempt_state)));
}
function validCandidateTransfer(value:unknown):value is CandidateTransfer{
  return isObject(value)&&hasFields(value,[...commonFields,'transfer_id','bundle_sha256','base_commit','head_commit'])&&validEvidence(value)&&
    matches(value.transfer_id,identifier)&&
    (value.disposition!=='eligible'||value.attempt_state==='failed'||value.attempt_state==='cancelled')&&
    (matches(value.bundle_sha256,digest)&&matches(value.base_commit,commit)&&matches(value.head_commit,commit)&&
      value.base_commit.length===value.head_commit.length&&integer(value.size_bytes,1,64*1024*1024)||
    value.disposition==='unverified'&&value.bundle_sha256===''&&value.base_commit===''&&value.head_commit===''&&value.size_bytes===0);
}

export function readUploadRecoveryView(value:unknown,worker:ExecutionWorker,kind:UploadRecoveryKind,cursor:string){
  if(!isObject(value)||!hasFields(value,['project_id','worker_registry_id','worker_id','worker_version','items','next_cursor'])||
    value.project_id!==worker.project_id||value.worker_registry_id!==worker.id||value.worker_id!==worker.worker_id||
    !version(value.worker_version)||value.worker_version!==worker.version||
    !Array.isArray(value.items)||value.items.length>20||typeof value.next_cursor!=='string')return;
  const {next_cursor}=value;
  let items:UploadRecoveryItem[];
  if(kind==='candidate-transfers'){
    if(!value.items.every(validCandidateTransfer))return;
    items=value.items.map(({transfer_id,bundle_sha256,base_commit,head_commit,...evidence})=>({
      ...evidence,id:transfer_id,sha256:bundle_sha256,commits:{base:base_commit,head:head_commit},
    }));
    // The cursor tracks scanned rows, including rows intentionally withheld by the server.
    if(next_cursor&&(!matches(next_cursor,identifier)||next_cursor<=cursor||next_cursor<(items.at(-1)?.id??'')))return;
  }else{
    if(!value.items.every(validArtifactUpload))return;
    items=value.items.map(({upload_id,manifest_sha256,...evidence})=>({...evidence,id:upload_id,sha256:manifest_sha256}));
    if(next_cursor&&(items.length!==20||next_cursor!==items[19].id))return;
  }
  if(items.some((item,index)=>item.id<=(index===0?cursor:items[index-1].id)))return;
  return {items,next_cursor};
}

export function validUploadAbandonRequest(request:WriteRecoveryRequest,project:string){
  const {path,body,method,idempotency_key}=request;
  const route=abandonRoute.exec(path);
  if(method!=='POST'||!route)return false;
  const expectedDigest=digestField(route[2]);
  return hasFields(body,['project_id','expected_worker_version',expectedDigest,'acknowledge_incomplete','idempotency_key'])&&
    body.project_id===project&&version(body.expected_worker_version)&&matches(body[expectedDigest],digest)&&
    body.acknowledge_incomplete===true&&body.idempotency_key===idempotency_key&&operation.test(idempotency_key);
}

export function validUploadAbandonReceipt(value:unknown,request:WriteRecoveryRequest,project:string):value is UploadAbandonReceipt{
  const route=abandonRoute.exec(request.path);
  if(!route||!validUploadAbandonRequest(request,project)||!isObject(value))return false;
  const transferField=route[2]==='candidate-transfers'?'transfer_id':'upload_id';
  return hasFields(value,['project_id','worker_registry_id',transferField,'operation_id','state'])&&value.project_id===project&&
    value.worker_registry_id===route[1]&&value[transferField]===route[3]&&value.operation_id===request.idempotency_key&&value.state==='abandoned';
}

export function definitiveUploadRefusal(code:string,request:WriteRecoveryRequest,project:string){
  if(!validUploadAbandonRequest(request,project))return false;
  const kind=abandonRoute.exec(request.path)?.[2];
  return code==='version_conflict'||code==='idempotency_conflict'||
    code===(kind==='candidate-transfers'?'candidate_transfer_attempt_unresolved':'artifact_upload_attempt_unresolved');
}
