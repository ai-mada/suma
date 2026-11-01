import type {ToolProfileImpact as Impact} from '../../../packages/contracts/src';
import {useId} from 'react';
import {ArrowUpRight,CircleHelp,GitBranch} from 'lucide-react';
import {formatDateTime,formatNumber,tr,useLocale} from './i18n';
import {readToolProfileImpact} from './tool-profile-impact-model';
import './ToolProfileImpact.css';
export type ToolProfileImpactProps={impact?:Impact;onRun?:(runID:string,nodeID:string)=>void};
const t=(key:string,values?:Record<string,unknown>)=>tr('tool-management.impact.'+key,values);
/** A current project observation only; navigation is supplied by the original
 * view owner and never performs a stop, revoke or dispatch operation. */
export function ToolProfileImpact({impact,onRun}:ToolProfileImpactProps){
 useLocale();const id=useId(),value=readToolProfileImpact(impact),coverage=value?.coverage??'unknown',count=value?.observed_reference_count??0;
 const summary=!value?'unknown':coverage==='complete'?(count?'complete':'empty'):count?(coverage==='partial'?'partial_count':'unknown_count'):'uncertain';
 const countText=count===1?t('one'):t('many',{count:formatNumber(count)}),Icon=coverage==='complete'?GitBranch:CircleHelp;
 return <section className="tool-profile-impact" data-tool-profile-impact data-coverage={coverage} aria-labelledby={id}>
  <h4 id={id}>{t('title')}</h4><p className="tool-profile-impact-summary" data-impact-summary role="status"><Icon size={17} aria-hidden="true"/><span>{t(summary,{count:countText})}</span></p>
  <details className="tool-profile-impact-details"><summary>{t('details')}</summary>
   <p>{t('scope')}</p>{value?<>
    <dl className="tool-profile-impact-meta"><div><dt>{t('observed_at')}</dt><dd><time dateTime={value.observed_at} title={value.observed_at}>{formatDateTime(value.observed_at,{dateStyle:'short',timeStyle:'medium'})}</time></dd></div><div><dt>{t('examined')}</dt><dd>{t('examined_count',{runs:formatNumber(value.runs_examined),nodes:formatNumber(value.nodes_examined)})}</dd></div></dl>
    {value.reason&&<p className="tool-profile-impact-remaining">{t('reason.'+value.reason)}</p>}
    {!!value.items.length&&<ul className="tool-profile-impact-items">{value.items.map(item=><li key={JSON.stringify([item.run_id,item.node_id])}>
     {onRun?<button type="button" data-impact-run={item.run_id} data-impact-node={item.node_id} aria-label={t('open',{run:item.run_id,node:item.node_id})} onClick={()=>onRun(item.run_id,item.node_id)}><span><code>{item.run_id}</code><span aria-hidden="true"> / </span><code>{item.node_id}</code></span><ArrowUpRight size={16} aria-hidden="true"/></button>:<p><code>{item.run_id}</code><span aria-hidden="true"> / </span><code>{item.node_id}</code></p>}
     <p className="tool-profile-impact-item-meta">{t('states',{run:t('state.'+item.run_state),node:t('state.'+item.node_state)})}</p>
    </li>)}</ul>}
    {value.items_truncated&&<p>{t('truncated',{shown:formatNumber(value.items.length)})}</p>}
   </>:<p>{t('unknown_detail')}</p>}
   <p className="tool-profile-impact-boundary">{t('boundary')}</p>
  </details>
 </section>;
}
