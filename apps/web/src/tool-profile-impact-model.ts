import type {ToolProfileImpact} from '../../../packages/contracts/src';
import {wellFormed} from './i18n/catalog';
const object=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==='object'&&!Array.isArray(v)&&[Object.prototype,null].includes(Object.getPrototypeOf(v));
const fields=(v:Record<string,unknown>,keys:readonly string[])=>keys.every(k=>Object.hasOwn(v,k))&&Reflect.ownKeys(v).every(k=>typeof k==='string'&&keys.includes(k));
const integer=(v:unknown,max=Number.MAX_SAFE_INTEGER):v is number=>typeof v==='number'&&Number.isSafeInteger(v)&&v>=0&&v<=max;
const text=(v:unknown,max:number,empty=false):v is string=>typeof v==='string'&&(empty||!!v)&&wellFormed(v)&&new TextEncoder().encode(v).length<=max&&v.trim()===v&&!/\p{Cc}/u.test(v);
const runState=(v:unknown)=>typeof v==='string'&&['running','paused','cancelling'].includes(v);
const nodeState=(v:unknown)=>typeof v==='string'&&['pending','ready','running','waiting','reconciling'].includes(v);
function stamp(v:unknown):v is string{return typeof v==='string'&&/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?Z$/.test(v)&&Number.isFinite(Date.parse(v))&&new Date(Date.parse(v)).toISOString().slice(0,19)===v.slice(0,19);}
/** This bounded project observation never proves all dependencies or that any
 * real process stopped. Malformed sources stay unknown in the presentation. */
export function readToolProfileImpact(v:unknown):ToolProfileImpact|undefined{
 if(!object(v)||!fields(v,['scope','observed_at','coverage','runs_examined','nodes_examined','observed_reference_count','items','items_truncated','reason'])||v.scope!=='active_project_runs'||!stamp(v.observed_at)||!integer(v.runs_examined,200)||!integer(v.nodes_examined,2000)||!integer(v.observed_reference_count,v.nodes_examined)||!Array.isArray(v.items)||v.items.length>5||v.items.length>v.observed_reference_count||typeof v.items_truncated!=='boolean'||v.items_truncated!==(v.observed_reference_count>v.items.length))return;
 if(!(v.coverage==='complete'&&v.reason===''||v.coverage==='partial'&&(v.reason==='run_limit'||v.reason==='node_limit')||v.coverage==='unknown'&&v.reason==='source_unavailable'))return;
 const ids=new Set<string>(),runs=new Set<string>();
 for(const row of v.items){if(!object(row)||!fields(row,['run_id','node_id','run_state','node_state','worker_id','attempt','fence'])||!text(row.run_id,150)||!text(row.node_id,150)||!runState(row.run_state)||!nodeState(row.node_state)||!text(row.worker_id,150,true)||!integer(row.attempt)||!integer(row.fence))return;const key=JSON.stringify([row.run_id,row.node_id]);if(ids.has(key))return;ids.add(key);runs.add(row.run_id);}
 return runs.size<=v.runs_examined?v as unknown as ToolProfileImpact:undefined;
}
