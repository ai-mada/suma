import type {ExecutionSkillBundle,ExecutionSkillEvidence,ExecutionSkillDocument,ExecutionSkillReceipt} from '../../../packages/contracts/src';

const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value)&&(Object.getPrototypeOf(value)===Object.prototype||Object.getPrototypeOf(value)===null);
const closed=(value:Record<string,unknown>,required:readonly string[],optional:readonly string[]=[])=>required.every(key=>Object.hasOwn(value,key))&&Object.keys(value).every(key=>required.includes(key)||optional.includes(key));
const id=(value:unknown):value is string=>typeof value==='string'&&/^[A-Za-z0-9][A-Za-z0-9_.:/@+\-]{0,127}$/.test(value);
const hash=(value:unknown):value is string=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
const positive=(value:unknown):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>0;
const name=(value:unknown):value is string=>typeof value==='string'&&/^suma-[a-f0-9]{24}$/.test(value);
const bytes=(value:string)=>new TextEncoder().encode(value).length;
// Go strings.TrimSpace defines the fallback for a published empty summary.
const trim=(value:string)=>value.replace(/^[\u0009-\u000d\u0020\u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+|[\u0009-\u000d\u0020\u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+$/g,'');
const text=(value:unknown,limit:number,required=true):value is string=>typeof value==='string'&&!/[\uD800-\uDBFF](?![\uDC00-\uDFFF])|(?<![\uD800-\uDBFF])[\uDC00-\uDFFF]/u.test(value)&&!value.includes('\0')&&bytes(value)<=limit&&(!required||!!trim(value));
const receiptKeys=['id','revision','sha256','name','document_sha256'] as const;
const documentKeys=[...receiptKeys,'title','description','content','content_sha256'] as const;
const factKeys=['materialized','catalog_acknowledged','invocation_requested','turn_acknowledged','files_verified'] as const;
function validReceipt(value:unknown):value is ExecutionSkillReceipt{
 return object(value)&&closed(value,receiptKeys)&&id(value.id)&&positive(value.revision)&&hash(value.sha256)&&name(value.name)&&hash(value.document_sha256);
}
function validDocument(value:unknown):value is ExecutionSkillDocument{
 return object(value)&&closed(value,documentKeys)&&id(value.id)&&positive(value.revision)&&hash(value.sha256)&&name(value.name)&&hash(value.document_sha256)&&hash(value.content_sha256)&&text(value.title,200)&&text(value.description,1000)&&text(value.content,16000);
}
function uniqueDocuments(values:readonly ExecutionSkillReceipt[]):boolean{
 const seen=new Set<string>();
 for(const value of values){if(seen.has(value.id)||seen.has(value.name))return false;seen.add(value.id);seen.add(value.name);}
 return true;
}
// Bound the same rendered UTF-8 bytes, without recomputing trusted server
// digests or treating a shape check as current publication authorization.
const yamlString=(value:string)=>JSON.stringify(value).replace(/[<>&\u2028\u2029]/g,c=>'\\u'+c.charCodeAt(0).toString(16).padStart(4,'0'));
const documentBytes=(value:ExecutionSkillDocument)=>bytes('---\nname: '+yamlString(value.name)+'\ndescription: '+yamlString(value.description)+'\n---\n\n'+value.content+'\n');
export function validExecutionSkillBundle(value:unknown):value is ExecutionSkillBundle{
 return object(value)&&closed(value,['protocol','documents','bundle_sha256'])&&value.protocol==='suma-codex-skills-v1'&&hash(value.bundle_sha256)&&Array.isArray(value.documents)&&value.documents.length>0&&value.documents.length<=16&&value.documents.every(validDocument)&&uniqueDocuments(value.documents)&&value.documents.reduce((sum,doc)=>sum+documentBytes(doc),0)<=32000;
}
export function validExecutionSkillEvidence(value:unknown):value is ExecutionSkillEvidence{
 if(!object(value)||!closed(value,['bundle_sha256','documents',...factKeys],['failure_code'])||!hash(value.bundle_sha256)||!Array.isArray(value.documents)||value.documents.length<1||value.documents.length>16||!value.documents.every(validReceipt)||!uniqueDocuments(value.documents)||!factKeys.every(key=>typeof value[key]==='boolean'))return false;
 return (!Object.hasOwn(value,'failure_code')||value.failure_code===''||id(value.failure_code))&&
  (!value.catalog_acknowledged||value.materialized===true)&&(!value.invocation_requested||value.catalog_acknowledged===true)&&(!value.turn_acknowledged||value.invocation_requested===true)&&(!value.files_verified||value.materialized===true);
}
export function executionSkillEvidenceMatches(bundle:unknown,evidence:unknown):boolean{
 return validExecutionSkillBundle(bundle)&&validExecutionSkillEvidence(evidence)&&bundle.bundle_sha256===evidence.bundle_sha256&&bundle.documents.length===evidence.documents.length&&bundle.documents.every((doc,index)=>receiptKeys.every(key=>doc[key]===evidence.documents[index][key]));
}
function sameBundle(left:ExecutionSkillBundle,right:ExecutionSkillBundle):boolean{
 return left.bundle_sha256===right.bundle_sha256&&left.documents.length===right.documents.length&&left.documents.every((doc,index)=>documentKeys.every(key=>doc[key]===right.documents[index][key]));
}
/** This is an additive binding guard, not a replacement for the surrounding
 * task/actor/plan validator. Existing plans without native files stay text-only. */
export function validExecutionSkillPlan(plan:unknown):boolean{
 if(!object(plan))return false;
 const nodes=object(plan.workflow)&&Array.isArray(plan.workflow.nodes)?plan.workflow.nodes:[];
 const protectedNodes=nodes.filter(node=>object(node)&&object(node.config)&&Object.hasOwn(node.config,'execution_skills'));
 if(!Object.hasOwn(plan,'execution_skills')&&!protectedNodes.length)return true;
 if(!validExecutionSkillBundle(plan.execution_skills)||!object(plan.codex_execution)||plan.codex_execution.protocol!=='codex-app-server-v1'||!id(plan.tool_profile_id)||!hash(plan.tool_profile_sha256))return false;
 const matching=nodes.filter(node=>object(node)&&node.id===plan.agent_node_id),node=matching[0];
 if(matching.length!==1||protectedNodes.length!==1||!object(node)||protectedNodes[0]!==node||node.type!=='agent'||!Array.isArray(node.command)||node.command.length!==1||!id(node.command[0])||!object(node.config)||node.config.tool_profile_id!==plan.tool_profile_id||node.config.tool_profile_sha256!==plan.tool_profile_sha256||!validExecutionSkillBundle(node.config.execution_skills)||!sameBundle(plan.execution_skills,node.config.execution_skills))return false;
 if(!object(plan.context)||!Array.isArray(plan.context.knowledge)||plan.context.knowledge.length>16||!plan.context.knowledge.every(object))return false;
 const skills=plan.context.knowledge.filter(item=>item.kind==='skill'),bundle=plan.execution_skills;
 return skills.length===bundle.documents.length&&skills.every((skill,index)=>{
  const doc=bundle.documents[index];
  return skill.id===doc.id&&skill.revision===doc.revision&&skill.sha256===doc.sha256&&skill.title===doc.title&&skill.content===doc.content&&text(skill.summary,1000,false)&&doc.description===(trim(skill.summary)?skill.summary:skill.title);
 });
}
export function frozenExecutionSkillsForNode(definition:unknown,run:unknown,node:unknown):ExecutionSkillBundle|undefined{
 if(!object(definition)||!object(run)||!object(node)||definition.id!==run.workflow_id||definition.version!==run.workflow_version||!Array.isArray(definition.nodes)||!Array.isArray(run.nodes))return;
 const actuals=run.nodes.filter(item=>object(item)&&item.id===node.id),declaredNodes=definition.nodes.filter(item=>object(item)&&item.id===node.id),actual=actuals[0],declared=declaredNodes[0];
 if(actuals.length!==1||declaredNodes.length!==1||!object(actual)||!object(declared)||!object(declared.config)||node.type!=='agent'||actual.type!=='agent'||declared.type!=='agent'||!validExecutionSkillBundle(declared.config.execution_skills)||!id(declared.config.tool_profile_id)||!hash(declared.config.tool_profile_sha256)||!Array.isArray(declared.command)||declared.command.length!==1||!id(declared.command[0]))return;
 const command=declared.command;
 for(const current of [actual,node])if(current.attempt!==node.attempt||current.fence!==node.fence||current.tool_profile_id!==declared.config.tool_profile_id||current.tool_profile_sha256!==declared.config.tool_profile_sha256||!Array.isArray(current.command)||current.command.length!==command.length||!current.command.every((value,index)=>value===command[index]))return;
 return declared.config.execution_skills;
}
