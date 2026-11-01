import type {ProjectKnowledgeDiscovery} from '../../../packages/contracts/src';
import {UiError} from './i18n';
import {wellFormed} from './i18n/catalog';
export type KnowledgeDiscoveryKind=''|'memory'|'spec'|'skill';
export type KnowledgeDiscoveryReadScope={project:string;query:string;kind:KnowledgeDiscoveryKind;limit:number};
const encoder=new TextEncoder();
const record=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value)&&[Object.prototype,null].includes(Object.getPrototypeOf(value));
const closed=(value:Record<string,unknown>,keys:readonly string[])=>keys.every(key=>Object.hasOwn(value,key))&&Reflect.ownKeys(value).every(key=>typeof key==='string'&&keys.includes(key));
const id=(value:unknown):value is string=>typeof value==='string'&&/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(value);
const text=(value:unknown,max:number):value is string=>typeof value==='string'&&wellFormed(value)&&!value.includes('\0')&&encoder.encode(value).length<=max;
const integer=(value:unknown,min:number,max:number):value is number=>Number.isSafeInteger(value)&&(value as number)>=min&&(value as number)<=max;
const kind=(value:unknown):value is Exclude<KnowledgeDiscoveryKind,''>=>typeof value==='string'&&['memory','spec','skill'].includes(value);
const query=(value:unknown):value is string=>text(value,200)&&!/\p{Cc}/u.test(value);
const cursor=(value:unknown):value is string=>typeof value==='string'&&wellFormed(value)&&encoder.encode(value).length<=2048;
function validScope(scope:KnowledgeDiscoveryReadScope){return id(scope.project)&&query(scope.query)&&(scope.kind===''||kind(scope.kind))&&integer(scope.limit,1,20);}
function stamp(value:unknown):value is string{
 if(typeof value!=='string'||!/^\d{4}-\d\d-\d\dT(?:[01]\d|2[0-3]):[0-5]\d:[0-5]\d(?:\.\d{1,9})?(?:Z|[+-](?:[01]\d|2[0-3]):[0-5]\d)$/.test(value))return false;
 const base=value.slice(0,19),milliseconds=Date.parse(base+'Z');return Number.isFinite(milliseconds)&&new Date(milliseconds).toISOString().slice(0,19)===base&&Number.isFinite(Date.parse(value));
}
export function knowledgeDiscoveryPath(scope:KnowledgeDiscoveryReadScope&{cursor?:string}):string{
 if(!validScope(scope)||scope.cursor!==undefined&&!cursor(scope.cursor))throw new UiError('knowledge-discovery.invalid_query');
 const params=new URLSearchParams({project_id:scope.project,limit:String(scope.limit)});if(scope.query.trim())params.set('q',scope.query.trim());if(scope.kind)params.set('kind',scope.kind);if(scope.cursor)params.set('cursor',scope.cursor);
 return '/project-knowledge/discovery?'+params;
}
export function readProjectKnowledgeDiscovery(value:unknown,scope:KnowledgeDiscoveryReadScope):ProjectKnowledgeDiscovery{
 const fail=()=>new UiError('knowledge-discovery.invalid_response');
 if(!validScope(scope)||!record(value)||!closed(value,['items','next_cursor','mode','scope','scanned','scan_limit','exhausted'])||!Array.isArray(value.items)||value.items.length>scope.limit||value.mode!=='keyword'||value.scope!=='current_project_publications'||!integer(value.scanned,value.items.length,200)||value.scan_limit!==200||!cursor(value.next_cursor)||typeof value.exhausted!=='boolean'||value.exhausted!==!value.next_cursor)throw fail();
 const seen=new Set<string>();
 for(const hit of value.items){
  if(!record(hit)||!closed(hit,['reference','project_id','kind','title','summary','snippet','matched_field','source_count','published_at'])||hit.project_id!==scope.project||!kind(hit.kind)||scope.kind!==''&&hit.kind!==scope.kind||!text(hit.title,200)||!hit.title||!text(hit.summary,1000)||!text(hit.snippet,600)||!integer(hit.source_count,0,8)||!stamp(hit.published_at))throw fail();
  const pin=hit.reference;if(!record(pin)||!closed(pin,['id','revision','sha256'])||!id(pin.id)||!integer(pin.revision,1,2147483647)||typeof pin.sha256!=='string'||!/^[a-f0-9]{64}$/.test(pin.sha256)||seen.has(pin.id))throw fail();seen.add(pin.id);
  if(scope.query.trim()?typeof hit.matched_field!=='string'||!['title','summary','content'].includes(hit.matched_field):hit.matched_field!=='none')throw fail();
 }
 // Go's response byte budget includes JSON HTML/line-separator escaping.
 const json=JSON.stringify(value).replace(/[<>&\u2028\u2029]/g,char=>'\\u'+char.charCodeAt(0).toString(16).padStart(4,'0'));
 if(encoder.encode(json).length>48*1024)throw fail();return value as unknown as ProjectKnowledgeDiscovery;
}
