import {i18n,getLocale} from './index';
import {wellFormed} from './catalog';

// Only these owned static labels accept raw data through this path. This is
// not a diagnostic renderer or an open server-catalog lookup API.
const parameters = {
 'automation.fixed_rule_basis':['name','revision'],
 'development.candidate.source_option':['name','id','type'],
 'development.candidate.build_option':['name','id','command'],
 'collaboration.capacity_resource_week':['name','week'],
 'release.prepare.environment_option':['name','archived','pending'],
 'development.dispatch.tool_option':['name','worker','version','status'],
 'development.workflow.graph_node_aria':['name','id','state'],
 'development.workflow.upstream_node':['name','id'],
 'flow.handle.input':['name','id'],
 'flow.handle.output':['name','id'],
} as const;
export type LiteralKey = keyof typeof parameters;
export type LiteralParameters<K extends LiteralKey> = Record<(typeof parameters)[K][number],string>;

function ownValues(value: unknown, order?: readonly string[]): string {
 if(value===null || typeof value!=='object')return typeof value==='string'?value:'';
 const fields=Object.getOwnPropertyDescriptors(value),keys=Object.keys(fields);
 const ordered=order?[...order.filter(name=>Object.hasOwn(fields,name)),...keys.filter(name=>!order.includes(name))]:keys;
 return ordered.flatMap(name=>{
  const field=fields[name];if(!Object.hasOwn(field,'value'))return [];
  const item=field.value;
  return typeof item==='string'?[item]:typeof item==='number'||typeof item==='boolean'?[String(item)]:[];
 }).filter(value=>value!=='').join(' · ');
}

/** Pure display formatting: validate the closed key/shape and static template,
 * then scan that template once. Inserted strings never enter the translator. */
export function formatLiteralValues<K extends LiteralKey>(key: K, values: LiteralParameters<K>): {text:string;formatted:boolean} {
 const names=typeof key==='string' && Object.hasOwn(parameters,key)?parameters[key]:undefined;
 const fallback={text:ownValues(values,names),formatted:false};
 if(!names || !values || typeof values!=='object' || Array.isArray(values)
  || ![Object.prototype,null].includes(Object.getPrototypeOf(values)))return fallback;
 const fields=Object.getOwnPropertyDescriptors(values);
 if(Reflect.ownKeys(fields).length!==names.length || names.some(name=>!Object.hasOwn(fields,name)
  || !Object.hasOwn(fields[name],'value') || typeof fields[name].value!=='string'))return fallback;
 const template=i18n.getResource(getLocale(),'translation',key);
 if(typeof template!=='string' || !template.trim() || !wellFormed(template))return fallback;
 const token=/\{\{([a-z][a-z0-9_]*)\}\}/g;
 const found=new Set([...template.matchAll(token)].map(match=>match[1]));
 const remaining=template.replace(token,'');
 if(remaining.includes('{{') || remaining.includes('}}') || found.size!==names.length
  || names.some(name=>!found.has(name)))return fallback;
 return {text:template.replace(token,(_match,name:string)=>fields[name].value as string),formatted:true};
}

export function literalText<K extends LiteralKey>(key: K, values: LiteralParameters<K>): string {
 return formatLiteralValues(key,values).text;
}
