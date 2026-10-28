import type {IntelligentOperationAnalysis,IntelligentOperationOutput,IntelligentOperationPreview,IntelligentOperationStep,ProjectKnowledgeSourceInput,RequirementDesignTask,ToolProfile} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';

export const operationActions=['requirement.create_draft','requirement.design.save_draft','knowledge.create_draft'] as const;
export const operationStates=['running','ready','failed','reconciling','applied','dismissed'];
export const operationSourceKinds=['requirements','documents','tasks','issues'] as const;
export const operationKnowledgeKinds=['memory','spec','skill'] as const;
export const operationID=(value:unknown):value is string=>typeof value==='string'&&/^[A-Za-z0-9_-]{1,64}$/.test(value);
export const operationText=(value:unknown,max:number,empty=false):value is string=>typeof value==='string'&&(empty||!!value.trim())&&new TextEncoder().encode(value).length<=max;
export const operationHash=(value:unknown):value is string=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const positive=(value:unknown):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>0;
const keys=(value:Record<string,unknown>,allowed:string[])=>Object.keys(value).every(key=>allowed.includes(key));
const strings=(value:unknown):value is string[]=>Array.isArray(value)&&value.every(item=>typeof item==='string');
const indices=(value:unknown):value is number[]=>Array.isArray(value)&&value.every(item=>typeof item==='number'&&Number.isSafeInteger(item));
const unique=(values:unknown[])=>new Set(values).size===values.length;
const criteria=(value:string[],max:number,empty=true)=>value.length<=max&&(empty||value.length>0)&&value.every(item=>operationText(item,1000));
const priority=(value:string)=>['','low','medium','high','urgent'].includes(value);
export const operationPath=(id='')=>'/intelligent-operations/analyses'+(id?'/'+encodeURIComponent(id):'');
export const operationStringify=(value:unknown)=>JSON.stringify(value,null,2)||'';

export function operationProfile(profile:ToolProfile){
 const capability=profile.profile?.intelligent_operations;
 return profile.state==='enabled'&&profile.available&&profile.profile.adapter==='suma'&&profile.profile.read_only&&capability?.protocol==='suma-intelligent-operations-v1'&&!!capability.model&&!!capability.endpoint_origin;
}

function taskShape(value:unknown):value is RequirementDesignTask{
 return object(value)&&keys(value,['key','title','description','priority','owner_id','depends_on','acceptance_criteria','covers'])&&['key','title','description','priority','owner_id'].every(key=>typeof value[key]==='string')&&strings(value.depends_on)&&strings(value.acceptance_criteria)&&indices(value.covers);
}
function stepShape(value:unknown):value is IntelligentOperationStep{
 if(!object(value)||!keys(value,['key','action','depends_on','requirement','design','knowledge'])||typeof value.key!=='string'||!strings(value.depends_on)||!operationActions.includes(value.action as typeof operationActions[number]))return false;
 const selected=value.action==='requirement.create_draft'?'requirement':value.action==='requirement.design.save_draft'?'design':'knowledge';
 if(['requirement','design','knowledge'].some(key=>key!==selected&&value[key]!==undefined)||!object(value[selected]))return false;
 const payload=value[selected] as Record<string,unknown>;
 if(selected==='requirement')return keys(payload,['title','description','priority','acceptance_criteria'])&&['title','description','priority'].every(key=>typeof payload[key]==='string')&&strings(payload.acceptance_criteria);
 if(selected==='knowledge')return keys(payload,['kind','title','summary','content','source_indices'])&&['kind','title','summary','content'].every(key=>typeof payload[key]==='string')&&indices(payload.source_indices);
 return keys(payload,['requirement_id','requirement_step_key','content','tasks'])&&typeof payload.requirement_id==='string'&&typeof payload.requirement_step_key==='string'&&object(payload.content)&&keys(payload.content,['product','experience','technical'])&&['product','experience','technical'].every(key=>typeof (payload.content as Record<string,unknown>)[key]==='string')&&Array.isArray(payload.tasks)&&payload.tasks.every(taskShape);
}

/** Incomplete text remains editable; it never becomes a valid command by filling
 * missing fields or replacing the user's advanced JSON with defaults. */
export function operationEditableProposal(raw:string):IntelligentOperationOutput|undefined{
 try{const value:unknown=JSON.parse(raw);if(!object(value)||!keys(value,['schema_version','summary','rationale','questions','steps'])||value.schema_version!==1||typeof value.summary!=='string'||typeof value.rationale!=='string'||!strings(value.questions)||!Array.isArray(value.steps)||!value.steps.every(stepShape))return;return value as unknown as IntelligentOperationOutput;}catch{return;}
}
export function operationProposal(raw:string,sourceCount=8,requirementIDs?:Set<string>):IntelligentOperationOutput|undefined{
 const value=operationEditableProposal(raw);
 if(!value||new TextEncoder().encode(JSON.stringify(value)).length>32000||!operationText(value.summary,2000)||!operationText(value.rationale,8000)||value.questions.length>8||value.questions.some(question=>!operationText(question,1000))||value.steps.length>8||!value.steps.length&&!value.questions.length)return;
 const seen=new Map(value.steps.map(step=>[step.key,step])),designTargets=new Set<string>();
 if(seen.size!==value.steps.length)return;
 for(const step of value.steps){
  if(!operationID(step.key)||step.depends_on.length>8||!unique(step.depends_on)||step.depends_on.some(key=>key===step.key||!seen.has(key)))return;
  if(step.requirement){const item=step.requirement;if(!operationText(item.title,200)||!operationText(item.description,16000,true)||!priority(item.priority)||!criteria(item.acceptance_criteria,16,false))return;}
  if(step.knowledge){const item=step.knowledge;if(!operationKnowledgeKinds.includes(item.kind as typeof operationKnowledgeKinds[number])||!operationText(item.title,200)||!operationText(item.summary,1000,true)||!operationText(item.content,16000)||item.source_indices.length>8||!unique(item.source_indices)||item.source_indices.some(index=>index<0||index>=sourceCount))return;}
  if(step.design){const item=step.design,fromStep=!!item.requirement_step_key;
   if(fromStep===!!item.requirement_id||fromStep&&(seen.get(item.requirement_step_key)?.action!=='requirement.create_draft'||!step.depends_on.includes(item.requirement_step_key))||!fromStep&&(!operationID(item.requirement_id)||requirementIDs&&!requirementIDs.has(item.requirement_id)))return;
   const target=fromStep?'step:'+item.requirement_step_key:'source:'+item.requirement_id;if(designTargets.has(target))return;designTargets.add(target);
   if(new TextEncoder().encode(item.content.product+item.content.experience+item.content.technical).length>16000||item.tasks.length>100||new TextEncoder().encode(JSON.stringify(item.tasks)).length>262144)return;
   const taskKeys=new Set(item.tasks.map(task=>task.key));
   if(taskKeys.size!==item.tasks.length||item.tasks.some(task=>!operationID(task.key)||!operationText(task.title,200,true)||!operationText(task.description,4000,true)||!['low','medium','high','urgent'].includes(task.priority)||!operationText(task.owner_id,64,true)||task.acceptance_criteria.length>20||task.acceptance_criteria.some(value=>!operationText(value,1000,true))||task.depends_on.length>100||!unique(task.depends_on)||task.depends_on.some(key=>!operationID(key))||task.covers.length>100))return;
  }
 }
 const active=new Set<string>(),done=new Set<string>();
 function cycle(key:string):boolean{if(active.has(key))return true;if(done.has(key))return false;active.add(key);for(const dependency of seen.get(key)?.depends_on||[])if(cycle(dependency))return true;active.delete(key);done.add(key);return false;}
 if(value.steps.some(step=>cycle(step.key)))return;
 return value;
}
export function operationSource(value:unknown):value is ProjectKnowledgeSourceInput{
 return object(value)&&keys(value,['kind','id','version','quote'])&&operationSourceKinds.includes(value.kind as typeof operationSourceKinds[number])&&operationID(value.id)&&positive(value.version)&&(value.quote===undefined||operationText(value.quote,16000,true));
}
export function operationRecovery(request:WriteRecoveryRequest,project:string){
 const body=request.body;if(request.method!=='POST'||body.project_id!==project)return false;
 if(request.path===operationPath())return keys(body,['project_id','goal','locale','sources','tool_profile_id','expected_tool_version','max_output_tokens','acknowledge_model_use','idempotency_key'])&&operationText(body.goal,4000)&&['zh-CN','en-US'].includes(String(body.locale))&&Array.isArray(body.sources)&&body.sources.length<=8&&body.sources.every(operationSource)&&unique(body.sources.map(source=>source.kind+':'+source.id))&&operationID(body.tool_profile_id)&&positive(body.expected_tool_version)&&positive(body.max_output_tokens)&&body.max_output_tokens<=8192&&body.acknowledge_model_use===true;
 const match=request.path.match(/^\/intelligent-operations\/analyses\/([A-Za-z0-9_-]{1,64})\/commands$/);
 if(!match||!keys(body,['project_id','expected_version','action','proposal','preview_sha256','acknowledge_drafts','reason','idempotency_key'])||!positive(body.expected_version)||!operationText(body.reason,2000))return false;
 if(body.action==='revise')return !!operationProposal(JSON.stringify(body.proposal))&&!body.preview_sha256&&!body.acknowledge_drafts;
 if(body.action==='apply')return body.proposal===undefined&&operationHash(body.preview_sha256)&&body.acknowledge_drafts===true;
 return body.action==='dismiss'&&body.proposal===undefined&&!body.preview_sha256&&!body.acknowledge_drafts;
}
export function operationAnalysis(value:unknown,project:string,id?:string):value is IntelligentOperationAnalysis{
 return object(value)&&value.project_id===project&&operationID(value.id)&&(!id||value.id===id)&&positive(value.version)&&operationStates.includes(String(value.state))&&typeof value.goal==='string'&&Array.isArray(value.allowed_actions)&&value.allowed_actions.every(action=>['revise','apply','dismiss'].includes(String(action)))&&typeof value.basis_current==='boolean'&&object(value.source)&&operationHash(value.source.sha256)&&Array.isArray(value.source.sources)&&value.source.sources.every(source=>object(source)&&operationSource({kind:source.kind,id:source.id,version:source.version,...(source.quote!==undefined?{quote:source.quote}:{})})&&typeof source.title==='string'&&operationHash(source.sha256))&&Array.isArray(value.results)&&value.results.every(result=>object(result)&&operationID(result.key)&&operationActions.includes(result.action as typeof operationActions[number])&&operationID(result.resource_id)&&positive(result.version))&&(value.proposal===undefined||!!operationEditableProposal(JSON.stringify(value.proposal)));
}
export function operationPreview(value:unknown,analysis:IntelligentOperationAnalysis):value is IntelligentOperationPreview{
 return object(value)&&value.id===analysis.id&&value.project_id===analysis.project_id&&value.version===analysis.version&&value.source_sha256===analysis.source.sha256&&operationHash(value.preview_sha256)&&operationHash(value.proposal_sha256)&&value.policy==='suma-local-drafts-v1'&&typeof value.can_apply==='boolean'&&typeof value.basis_current==='boolean'&&strings(value.questions)&&Array.isArray(value.defaults)&&value.defaults.every(item=>object(item)&&['step_key','field','value','source'].every(key=>typeof item[key]==='string'))&&Array.isArray(value.steps)&&value.steps.length===(analysis.proposal?.steps.length||0)&&unique(value.steps.map(item=>object(item)?item.key:undefined))&&value.steps.every(item=>object(item)&&analysis.proposal?.steps.some(step=>step.key===item.key&&step.action===item.action)&&typeof item.title==='string'&&strings(item.depends_on)&&item.required_permission==='project.write'&&item.risk==='draft_only');
}
