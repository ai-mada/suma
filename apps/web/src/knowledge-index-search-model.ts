import type {ProjectKnowledgeIndexSearch} from '../../../packages/contracts/src';
import {knowledgeDiscoveryPath,type KnowledgeDiscoveryReadScope} from './knowledge-discovery-model';
import {UiError} from './i18n';
import {wellFormed} from './i18n/catalog';
const encoder=new TextEncoder();
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value)&&[Object.prototype,null].includes(Object.getPrototypeOf(value));
const closed=(value:Record<string,unknown>,keys:string[])=>keys.every(key=>Object.hasOwn(value,key))&&Reflect.ownKeys(value).every(key=>typeof key==='string'&&keys.includes(key));
const integer=(value:unknown,min:number,max:number):value is number=>Number.isSafeInteger(value)&&(value as number)>=min&&(value as number)<=max;
const text=(value:unknown,max:number):value is string=>typeof value==='string'&&wellFormed(value)&&!value.includes('\0')&&encoder.encode(value).length<=max;
const token=(value:unknown,max:number):value is string=>text(value,max)&&!!value&&value.trim()===value&&!/\p{Cc}/u.test(value);
const id=(value:unknown):value is string=>typeof value==='string'&&/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(value);
const digest=(value:unknown):value is string=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
function stamp(value:unknown):value is string{
 if(typeof value!=='string'||!/^\d{4}-\d\d-\d\dT(?:[01]\d|2[0-3]):[0-5]\d:[0-5]\d(?:\.\d{1,9})?(?:Z|[+-](?:[01]\d|2[0-3]):[0-5]\d)$/.test(value))return false;
 const base=value.slice(0,19),milliseconds=Date.parse(base+'Z');return Number.isFinite(milliseconds)&&new Date(milliseconds).toISOString().slice(0,19)===base&&Number.isFinite(Date.parse(value));
}
export function knowledgeIndexSearchPath(scope:KnowledgeDiscoveryReadScope):string{
 try{knowledgeDiscoveryPath(scope);if(!scope.query.trim())throw new Error();}catch{throw new UiError('knowledge-discovery.invalid_index_query');}
 const params=new URLSearchParams({project_id:scope.project,q:scope.query.trim(),limit:String(scope.limit)});if(scope.kind)params.set('kind',scope.kind);return '/project-knowledge/index-search?'+params;
}
export function readProjectKnowledgeIndexSearch(value:unknown,scope:KnowledgeDiscoveryReadScope):ProjectKnowledgeIndexSearch{
 const fail=()=>new UiError('knowledge-discovery.invalid_index_response');
 try{knowledgeIndexSearchPath(scope);}catch{throw fail();}
 if(!object(value)||!closed(value,['items','mode','generation','index_watermark','candidates_examined','discarded','scope'])||!Array.isArray(value.items)||value.items.length>scope.limit||!['lancedb_fts','lancedb_hybrid','oceanbase_fts','oceanbase_hybrid'].includes(value.mode as string)||!token(value.generation,128)||!integer(value.index_watermark,0,Number.MAX_SAFE_INTEGER)||!integer(value.candidates_examined,value.items.length,100)||!integer(value.discarded,0,value.candidates_examined)||value.scope!=='provider_candidates')throw fail();
 const seen=new Set<string>();
 for(const hit of value.items){
  if(!object(hit)||!closed(hit,['reference','project_id','kind','title','summary','snippet','source_count','published_at','source','locator'])||hit.project_id!==scope.project||!['memory','spec','skill'].includes(hit.kind as string)||scope.kind!==''&&hit.kind!==scope.kind||!text(hit.title,200)||!hit.title||!text(hit.summary,1000)||!text(hit.snippet,600)||!integer(hit.source_count,0,8)||!stamp(hit.published_at))throw fail();
  const pin=hit.reference,source=hit.source,locator=hit.locator;
  if(!object(pin)||!closed(pin,['id','revision','sha256'])||!id(pin.id)||!integer(pin.revision,1,2147483647)||!digest(pin.sha256)||seen.has(pin.id))throw fail();seen.add(pin.id);
  if(!object(source)||!closed(source,['kind','id','sequence','revision','sha256'])||source.kind!=='project_knowledge'||source.id!==pin.id||source.revision!==pin.revision||source.sha256!==pin.sha256||!integer(source.sequence,1,2147483647))throw fail();
  if(!object(locator)||!closed(locator,['unit','start','end'])||locator.unit!=='unicode_codepoints'||!integer(locator.start,0,1048575)||!integer(locator.end,locator.start+1,1048576)||locator.end-locator.start!==Array.from(hit.snippet).length)throw fail();
 }
 return value as unknown as ProjectKnowledgeIndexSearch;
}
