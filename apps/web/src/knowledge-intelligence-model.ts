import type {KnowledgeIntelligenceOutput,ProjectKnowledgeSourceInput,ToolProfile} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';
export const intelligenceKinds=['memory','spec','skill'] as const;
export const intelligenceSources=['documents','tasks','requirements','issues','accepted_delivery'] as const;
export const intelligenceStates=['running','ready','failed','reconciling','adopted','dismissed'];
export const intelligenceID=(value:unknown):value is string=>typeof value==='string'&&/^[A-Za-z0-9_-]{1,64}$/.test(value);
export const intelligenceText=(value:unknown,max:number,empty=false):value is string=>typeof value==='string'&&(empty||!!value.trim())&&new TextEncoder().encode(value).length<=max;
const positive=(value:unknown):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>0;
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
export const extractionProfile=(item:ToolProfile)=>item.state==='enabled'&&item.available&&item.profile.adapter==='suma'&&item.profile.read_only&&item.profile.knowledge_extraction?.protocol==='suma-knowledge-extraction-v1'&&!!item.profile.knowledge_extraction.model&&!!item.profile.knowledge_extraction.endpoint_origin;
// Structural parsing permits a temporarily blank field or source selection.
// Invalid syntax/shape stays in raw text and is never replaced by defaults.
export function intelligenceEditableProposal(raw:string):KnowledgeIntelligenceOutput|undefined{
 try{const value:unknown=JSON.parse(raw);if(!object(value)||Object.keys(value).sort().join(',')!=='content,rationale,schema_version,source_indices,summary,title'||value.schema_version!==1||['title','summary','content','rationale'].some(key=>typeof value[key]!=='string')||!Array.isArray(value.source_indices)||value.source_indices.some(index=>typeof index!=='number'||!Number.isSafeInteger(index)))return;return value as unknown as KnowledgeIntelligenceOutput;}catch{return;}
}
export function intelligenceProposal(raw:string,sourceCount=8):KnowledgeIntelligenceOutput|undefined{
 const value=intelligenceEditableProposal(raw);if(!value||!intelligenceText(value.title,200)||!intelligenceText(value.summary,1000,true)||!intelligenceText(value.content,16000)||!intelligenceText(value.rationale,8000)||!value.source_indices.length||value.source_indices.length>8||new Set(value.source_indices).size!==value.source_indices.length||value.source_indices.some(index=>index<0||index>=sourceCount))return;return value;
}
export function intelligenceSource(value:unknown):value is ProjectKnowledgeSourceInput{
 return object(value)&&Object.keys(value).every(key=>['kind','id','version','quote','basis_sha256'].includes(key))&&intelligenceSources.includes(value.kind as typeof intelligenceSources[number])&&intelligenceID(value.id)&&positive(value.version)&&(value.quote===undefined||intelligenceText(value.quote,16000,true))&&
  (value.kind==='accepted_delivery'?typeof value.basis_sha256==='string'&&/^[a-f0-9]{64}$/.test(value.basis_sha256):value.basis_sha256===undefined||value.basis_sha256==='');
}
export function intelligenceRecovery(request:WriteRecoveryRequest,project:string){
 const b=request.body;if(request.method!=='POST'||b.project_id!==project)return false;
 if(request.path==='/knowledge-intelligence/analyses')return Object.keys(b).every(key=>['project_id','goal','kind','locale','sources','tool_profile_id','expected_tool_version','max_steps','max_output_tokens','acknowledge_model_use','idempotency_key'].includes(key))&&intelligenceText(b.goal,4000)&&intelligenceKinds.includes(b.kind as typeof intelligenceKinds[number])&&['en-US','zh-CN'].includes(String(b.locale))&&Array.isArray(b.sources)&&b.sources.length>=1&&b.sources.length<=8&&b.sources.every(intelligenceSource)&&new Set(b.sources.map(value=>value.kind+':'+value.id)).size===b.sources.length&&intelligenceID(b.tool_profile_id)&&positive(b.expected_tool_version)&&b.max_steps===1&&positive(b.max_output_tokens)&&b.max_output_tokens<=8192&&b.acknowledge_model_use===true;
 const match=request.path.match(/^\/knowledge-intelligence\/analyses\/([A-Za-z0-9_-]+)\/commands$/);
 return !!match&&intelligenceID(match[1])&&Object.keys(b).every(key=>['project_id','expected_version','action','proposal','reason','acknowledge_draft','idempotency_key'].includes(key))&&positive(b.expected_version)&&['revise','adopt','dismiss'].includes(String(b.action))&&intelligenceText(b.reason,2000)&&typeof b.acknowledge_draft==='boolean'&&(b.action!=='adopt'||b.acknowledge_draft===true)&&(b.action!=='revise'||!!intelligenceProposal(JSON.stringify(b.proposal)));
}
