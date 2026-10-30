import {CircleHelp,Clock3,Activity,TriangleAlert,FileText} from 'lucide-react';
import {tr,useLocale,formatDateTime,formatNumber,renderDiagnostic} from './i18n';
import {executionFocus,executionNext,executionNodeHref,executionStates,readTeamExecution,type TeamExecutionNode} from './ExpertTeamExecutionFactsModel';
import './ExpertTeamExecutionFacts.css';
const t=(key:string,values?:Record<string,unknown>)=>tr('team-execution-facts.'+key,values);
const stamp=(value:string)=>formatDateTime(value,{year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',second:'2-digit'});

export function ExpertTeamExecutionFacts({project,runID,execution}:{project:string;runID:string;execution:unknown}){
 useLocale();
 const fact=readTeamExecution(execution,runID);
 if(!fact)return <section className="team-execution-facts"><p className="team-execution-facts-muted">{t('unavailable')}</p><a href={executionNodeHref(project,runID)}>{t('open_run')}</a></section>;
 const focus=executionFocus(fact.nodes),shown=new Set(focus.map(node=>node.node_id)),remaining=fact.nodes.filter(node=>!shown.has(node.node_id));
 return <section className="team-execution-facts" aria-label={t('title')}>
  <header><strong>{t('title')}</strong><span className="team-execution-facts-muted">{t('observed',{at:stamp(fact.observed_at)})}</span></header>
  <p className="team-execution-facts-muted">{t('range',{shown:formatNumber(fact.nodes.length),total:formatNumber(fact.nodes_total)})}{fact.truncated?' '+t('truncated'):''}</p>
  {!fact.nodes.length?<p>{t('empty')}</p>:<ul className="team-execution-facts-focus">{focus.map(node=><Node key={node.node_id} project={project} run={runID} node={node}/>)}</ul>}
  {!!remaining.length&&<details data-execution-more><summary>{t('more',{count:formatNumber(remaining.length)})}</summary><ul>{remaining.map(node=><Node key={node.node_id} project={project} run={runID} node={node}/>)}</ul></details>}
  <a className="team-execution-facts-full" href={executionNodeHref(project,runID)}>{t('open_run')}</a>
 </section>;
}
function Node({project,run,node}:{project:string;run:string;node:TeamExecutionNode}){
 const reason=renderDiagnostic(node.reason);
 const known=(executionStates as readonly string[]).includes(node.state),tone=!known?'unknown':['failed','blocked','reconciling'].includes(node.state)?'attention':node.state==='running'?'active':'neutral';
 const Icon=!known?CircleHelp:tone==='attention'?TriangleAlert:tone==='active'?Activity:['pending','ready','waiting','cancelling'].includes(node.state)?Clock3:FileText;
 return <li className="team-execution-facts-node" data-tone={tone} data-execution-node={node.node_id}>
  <div className="team-execution-facts-node-heading"><Icon size={16} aria-hidden="true"/><strong>{node.name||node.node_id}</strong><span>{known?t('state.'+node.state):t('state_unknown')}</span></div>
  {!known&&<p className="team-execution-facts-muted">{t('raw_state',{value:node.state||t('not_reported')})}</p>}
  {node.reason&&<p className="team-execution-facts-reason"><span>{t(reason===node.reason?'reason_review':'reason')}</span> {reason}</p>}
  {!node.reason&&['waiting','ready','pending','blocked','reconciling'].includes(node.state)&&<p className="team-execution-facts-muted">{t('no_reason')}</p>}
  <div className="team-execution-facts-node-footer"><span className="team-execution-facts-muted">{node.attempt>0?t('attempt',{value:formatNumber(node.attempt)}):t('no_attempt')}{node.worker_id?' · '+t('worker',{value:node.worker_id}):''}</span><a href={executionNodeHref(project,run,node)}>{t('next.'+executionNext(node.state))}</a></div>
  {node.ready_at&&<p className="team-execution-facts-muted">{t('ready_at',{at:stamp(node.ready_at)})}</p>}
 </li>;
}
