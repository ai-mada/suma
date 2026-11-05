import type {ExecutionWorker} from './tool-model';
import type {WriteRecoveryRequest} from './write-action';

export type UploadRecoveryItem={
  upload_id:string;run_id:string;node_id:string;attempt:number;fence:string;
  manifest_sha256:string;created_at:string;size_bytes:number;
  attempt_state:string;disposition:'eligible'|'awaiting_attempt'|'unverified';
};
export type UploadRecoveryView={
  project_id:string;worker_registry_id:string;worker_id:string;worker_version:number;
  items:UploadRecoveryItem[];next_cursor:string;
};
export type UploadAbandonReceipt={
  project_id:string;worker_registry_id:string;upload_id:string;operation_id:string;state:'abandoned';
};

const identifier=/^[a-f0-9]{32}$/;
const digest=/^[a-f0-9]{64}$/;
const operation=/^[A-Za-z0-9][A-Za-z0-9._-]{7,127}$/;
const abandonRoute=/^\/release-workers\/([a-f0-9]{32})\/artifact-uploads\/([a-f0-9]{32})\/abandon$/;
const states=['running','waiting','reconciling','succeeded','failed','cancelled','unverified'];
const dispositions=['eligible','awaiting_attempt','unverified'];
const isObject=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const matches=(value:unknown,pattern:RegExp):value is string=>typeof value==='string'&&pattern.test(value);
const hasFields=(value:Record<string,unknown>,fields:string[])=>Object.keys(value).length===fields.length&&fields.every(field=>Object.hasOwn(value,field));
const integer=(value:unknown,min:number,max:number):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>=min&&value<=max;
const version=(value:unknown)=>integer(value,1,2147483647);
const publicID=(value:unknown,max:number)=>typeof value==='string'&&value.length>=1&&value.length<=max&&!/[\u0000-\u001f\u007f]/.test(value);

export function uploadRecoveryPath(worker:ExecutionWorker,cursor:string){
  const params=new URLSearchParams({project_id:worker.project_id});
  if(cursor)params.set('cursor',cursor);
  return '/release-workers/'+encodeURIComponent(worker.id)+'/artifact-uploads?'+params.toString();
}
export function uploadAbandonPath(worker:ExecutionWorker,item:UploadRecoveryItem){
  return '/release-workers/'+encodeURIComponent(worker.id)+'/artifact-uploads/'+encodeURIComponent(item.upload_id)+'/abandon';
}

function validItem(value:unknown):value is UploadRecoveryItem{
  return isObject(value)&&hasFields(value,['upload_id','run_id','node_id','attempt','fence','manifest_sha256','created_at','size_bytes','attempt_state','disposition'])&&
    matches(value.upload_id,identifier)&&publicID(value.run_id,100)&&publicID(value.node_id,150)&&version(value.attempt)&&
    matches(value.fence,/^[1-9][0-9]{0,18}$/)&&matches(value.manifest_sha256,digest)&&
    typeof value.created_at==='string'&&Number.isFinite(Date.parse(value.created_at))&&integer(value.size_bytes,0,1073741824)&&
    typeof value.attempt_state==='string'&&states.includes(value.attempt_state)&&
    typeof value.disposition==='string'&&dispositions.includes(value.disposition)&&
    (value.disposition!=='eligible'||['succeeded','failed','cancelled'].includes(value.attempt_state));
}

export function validUploadRecoveryView(value:unknown,worker:ExecutionWorker,cursor:string):value is UploadRecoveryView{
  if(!isObject(value)||!hasFields(value,['project_id','worker_registry_id','worker_id','worker_version','items','next_cursor'])||
    value.project_id!==worker.project_id||value.worker_registry_id!==worker.id||value.worker_id!==worker.worker_id||
    !version(value.worker_version)||value.worker_version!==worker.version||
    !Array.isArray(value.items)||value.items.length>20||!value.items.every(validItem))return false;
  const items=value.items;
  if(items.some((item,index)=>item.upload_id<=(index===0?cursor:items[index-1].upload_id)))return false;
  return value.next_cursor===''||items.length===20&&value.next_cursor===items[19].upload_id;
}

export function validUploadAbandonRequest(request:WriteRecoveryRequest,project:string){
  const {path,body,method,idempotency_key}=request;
  return method==='POST'&&abandonRoute.test(path)&&hasFields(body,['project_id','expected_worker_version','expected_manifest_sha256','acknowledge_incomplete','idempotency_key'])&&
    body.project_id===project&&version(body.expected_worker_version)&&matches(body.expected_manifest_sha256,digest)&&
    body.acknowledge_incomplete===true&&body.idempotency_key===idempotency_key&&operation.test(idempotency_key);
}

export function validUploadAbandonReceipt(value:unknown,request:WriteRecoveryRequest,project:string):value is UploadAbandonReceipt{
  const route=abandonRoute.exec(request.path);
  return !!route&&validUploadAbandonRequest(request,project)&&isObject(value)&&
    hasFields(value,['project_id','worker_registry_id','upload_id','operation_id','state'])&&value.project_id===project&&
    value.worker_registry_id===route[1]&&value.upload_id===route[2]&&value.operation_id===request.idempotency_key&&value.state==='abandoned';
}
