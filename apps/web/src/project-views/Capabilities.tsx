import {Check,Minus,ShieldCheck,Terminal,X} from 'lucide-react';
import {api,query} from '../api';
import {Empty,ErrorNotice,Loading} from '../ui';
import {tr,useLocale} from '../i18n';
import sourceCatalog from '../i18n/locales/domains/workbench.zh-CN.json';
import {useProjectRead} from './useProjectRead';

type Capability={id?:string;name?:string;title?:string;provider?:string;type?:string;description?:string;reason?:string;detail?:string;configured?:unknown;available?:unknown;verified?:unknown};
const builtinIDs=new Set(['transactional_store','dag','files','semantic_search','payments','collaboration']);
// Server.capabilities owns these six declarations (internal/platform/http.go).
// Both identity and canonical source must match; custom or future prose stays raw.
function builtinText(item:Capability,field:'name'|'reason'){
 const value=item[field],key='workbench.capabilities.builtin.'+item.id+'.'+field;
 return builtinIDs.has(item.id||'')&&value===(sourceCatalog as Record<string,string>)[key]?tr(key):value;
}
async function readCapabilities(project:string){return api<{items?:Capability[];capabilities?:Capability[];[key:string]:unknown}>('/capabilities'+query(project));}
export function Capabilities({project,refresh}:{project:string;refresh:number}){
 useLocale();const snapshot=useProjectRead(project,refresh,readCapabilities);
 if(snapshot.error)return <ErrorNotice error={snapshot.error}/>;
 if(!snapshot.data)return <Loading/>;
 const data=snapshot.data;
 const items:Capability[]=Array.isArray(data.items)?data.items:Array.isArray(data.capabilities)?data.capabilities:Object.entries(data).map(([name,value])=>value&&typeof value==='object'&&!Array.isArray(value)?{name,...value}:{name,available:value});
 return <><div className="info-strip"><ShieldCheck size={18}/><p>{tr('workbench.capabilities.scope')}</p></div><div className="capability-grid">{items.map((item,index)=><section className="panel capability-card" key={item.id||item.name||index}><div className="capability-heading"><span className="row-icon"><Terminal size={19}/></span><div><h3>{item.title||builtinText(item,'name')||item.id}</h3><small>{item.provider||item.type||tr('workbench.capabilities.kind')}</small></div></div><p>{item.description||(item.reason?builtinText(item,'reason'):tr('workbench.capabilities.description'))}</p><div className="capability-states">{(['configured','available','verified'] as const).map(field=>{
  // Configuration prose and missing observations are not boolean evidence.
  const state=item[field]===true?'yes':item[field]===false?'no':'unknown';
  return <span key={field} data-capability-state={field}><i className={state}>{state==='yes'?<Check size={12}/>:state==='no'?<X size={12}/>:<Minus size={12}/>}</i>{tr('workbench.capabilities.fact',{label:tr('workbench.capabilities.'+field),value:tr('workbench.capabilities.value.'+state)})}</span>;
 })}</div>{item.detail&&<small className="muted">{item.detail}</small>}</section>)}</div>{!items.length&&<Empty title={tr('workbench.capabilities.empty')} description={tr('workbench.capabilities.empty_hint')}/>}</>;
}
