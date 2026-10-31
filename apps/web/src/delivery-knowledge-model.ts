import type {DeliveryKnowledgeSourceView,ProjectKnowledgeSource,ProjectKnowledgeSourceInput,ProjectKnowledgeContextSource,DeliveryKnowledgeBasis} from '../../../packages/contracts/src';

const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value)&&(Object.getPrototypeOf(value)===Object.prototype||Object.getPrototypeOf(value)===null);
const closed=(value:Record<string,unknown>,required:readonly string[],optional:readonly string[]=[])=>required.every(key=>Object.hasOwn(value,key))&&Object.keys(value).every(key=>required.includes(key)||optional.includes(key));
const text=(value:unknown):value is string=>typeof value==='string'&&!/[\uD800-\uDBFF](?![\uDC00-\uDFFF])|(?<![\uD800-\uDBFF])[\uDC00-\uDFFF]/u.test(value);
const id=(value:unknown,max=64):value is string=>typeof value==='string'&&value.length<=max&&/^(?!\.{1,2}$)[A-Za-z0-9_.-]+$/.test(value);
const positive=(value:unknown):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>0;
const version=(value:unknown):value is number=>positive(value)&&value<=2147483647;
const hash=(value:unknown):value is string=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
const commit=(value:unknown)=>typeof value==='string'&&/^(?:[a-f0-9]{40}|[a-f0-9]{64})$/.test(value);
const recordKinds=['documents','requirements','tasks','issues'];
const sourceKeys=['kind','id','version','title','sha256'];
const evidenceKeys=['run_id','node_id','attempt','fence','worker_id'];
const evidenceIdentity=(value:Record<string,unknown>)=>JSON.stringify(evidenceKeys.map(key=>value[key]));
const identity=(value:Record<string,unknown>,runID:string)=>value.run_id===runID&&id(value.node_id,100)&&positive(value.attempt)&&positive(value.fence)&&id(value.worker_id,150);
function testEvidence(value:unknown,runID:string):value is Record<string,unknown>{
 return object(value)&&closed(value,[...evidenceKeys,'result_sha256'],['plan_id','plan_revision','plan_sha256','report_id','report_sha256','source_commit'])&&identity(value,runID)&&hash(value.result_sha256)&&
  (!Object.hasOwn(value,'plan_id')||id(value.plan_id))&&(!Object.hasOwn(value,'plan_revision')||version(value.plan_revision))&&(!Object.hasOwn(value,'plan_sha256')||hash(value.plan_sha256))&&
  (!Object.hasOwn(value,'report_id')||id(value.report_id))&&(!Object.hasOwn(value,'report_sha256')||hash(value.report_sha256))&&(!Object.hasOwn(value,'source_commit')||commit(value.source_commit));
}
function candidateEvidence(value:unknown,runID:string):value is Record<string,unknown>{
 return object(value)&&closed(value,[...evidenceKeys,'kind','head_commit','evidence_sha256'],['transfer_id','bundle_sha256'])&&identity(value,runID)&&
  ['workspace_revision','sealed_bundle'].includes(String(value.kind))&&commit(value.head_commit)&&hash(value.evidence_sha256)&&
  (!Object.hasOwn(value,'transfer_id')||id(value.transfer_id))&&(!Object.hasOwn(value,'bundle_sha256')||hash(value.bundle_sha256))&&
  (value.kind!=='sealed_bundle'||id(value.transfer_id)&&hash(value.bundle_sha256));
}
/** Check the published shape and exact task/run relationships, not a substitute
 * for server authorization, canonical hashing or current acceptance checks. */
export function validDeliveryKnowledgeBasis(value:unknown,taskID:string,taskVersion:number):value is DeliveryKnowledgeBasis{
 if(!object(value)||!closed(value,['protocol','task','run_version','tests','candidates','sha256'])||value.protocol!=='suma-accepted-delivery-v1'||!version(value.run_version)||!hash(value.sha256))return false;
 const task=value.task;
 if(!object(task)||!closed(task,['id','version','run_id','review_id','sha256'])||!id(task.id)||task.id!==taskID||!version(task.version)||task.version!==taskVersion||!id(task.run_id,100)||!id(task.review_id)||!hash(task.sha256))return false;
 return Array.isArray(value.tests)&&value.tests.length<=256&&value.tests.every(item=>testEvidence(item,task.run_id as string))&&new Set(value.tests.map(evidenceIdentity)).size===value.tests.length&&
  Array.isArray(value.candidates)&&value.candidates.length<=128&&value.candidates.every(item=>candidateEvidence(item,task.run_id as string))&&new Set(value.candidates.map(evidenceIdentity)).size===value.candidates.length;
}
function source(value:unknown,withQuote:boolean):boolean{
 if(!object(value)||!closed(value,withQuote?[...sourceKeys,'quote']:sourceKeys,['delivery'])||!id(value.id)||!version(value.version)||!text(value.title)||!hash(value.sha256))return false;
 if(withQuote&&(!text(value.quote)||!value.quote.length||new TextEncoder().encode(value.quote).length>16000))return false;
 return value.kind==='accepted_delivery'?validDeliveryKnowledgeBasis(value.delivery,value.id,value.version):recordKinds.includes(String(value.kind))&&!Object.hasOwn(value,'delivery');
}
export function validProjectKnowledgeSource(value:unknown):value is ProjectKnowledgeSource{return source(value,true);}
export function validProjectKnowledgeContextSource(value:unknown):value is ProjectKnowledgeContextSource{return source(value,false);}
export function validDeliveryKnowledgeSourceView(value:unknown,project:string,taskID:string):value is DeliveryKnowledgeSourceView{
 return object(value)&&closed(value,['project_id','source'])&&value.project_id===project&&validProjectKnowledgeSource(value.source)&&value.source.kind==='accepted_delivery'&&value.source.id===taskID;
}
export function deliveryKnowledgeSourceInput(source:ProjectKnowledgeSource):ProjectKnowledgeSourceInput{
 if(!validProjectKnowledgeSource(source)||source.kind!=='accepted_delivery'||!source.delivery)throw new Error('Invalid accepted delivery source');
 return {kind:source.kind,id:source.id,version:source.version,quote:source.quote,basis_sha256:source.delivery.sha256};
}
