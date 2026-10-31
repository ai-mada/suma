import {Fragment,type MouseEvent} from 'react';
import type {ProjectKnowledgeSource} from '../../../packages/contracts/src';
import {tr,useLocale} from './i18n';
import './DeliveryKnowledgeSourceCard.css';

export type DeliveryKnowledgeSourceCardProps={project:string;source:ProjectKnowledgeSource;onRefresh?:()=>void;disabled?:boolean};
const t=(key:string,values?:Record<string,unknown>)=>tr('delivery-knowledge.'+key,values);
function Facts({values}:{values:[string,string|number|undefined][]}){
 return <dl className="dk-source-facts">{values.filter(([,value])=>value!==undefined&&value!=='').map(([label,value])=><Fragment key={label}><dt>{t(label)}</dt><dd><code>{value}</code></dd></Fragment>)}</dl>;
}

/** Presentation only. The owning reader validates the authorized source before rendering. */
export function DeliveryKnowledgeSourceCard({project,source,onRefresh,disabled=false}:DeliveryKnowledgeSourceCardProps){
 useLocale();const basis=source?.delivery;
 if(source?.kind!=='accepted_delivery'||basis?.protocol!=='suma-accepted-delivery-v1'||!basis.task||basis.task.id!==source.id||basis.task.version!==source.version||!Array.isArray(basis.tests)||!Array.isArray(basis.candidates))return <p className="dk-source-unavailable" role="status">{t('source_unavailable')}</p>;
 const navigate=(event:MouseEvent<HTMLAnchorElement>)=>{if(disabled||!window.dispatchEvent(new Event('suma:navigate',{cancelable:true})))event.preventDefault();};
 const link=(fields:Record<string,string>)=>disabled?undefined:'?'+new URLSearchParams({project,view:'development',...fields});
 return <section className="dk-source" aria-label={t('source_card')}>
  <header className="dk-source-heading"><div><p className="dk-source-label">{t('source_card')}</p><h3>{source.title}</h3></div><span>{t('task_version',{value:source.version})}</span></header>
  <div className="dk-source-counts"><span>{t('test_count',{value:basis.tests.length})}</span><span>{t('candidate_count',{value:basis.candidates.length})}</span></div>
  {basis.candidates.length===0&&<p className="dk-source-muted">{t('no_candidates')}</p>}
  <nav className="dk-source-actions" aria-label={t('source_card')}>
   <a data-delivery-task href={link({sub:'tasks',record:source.id})} aria-disabled={disabled||undefined} tabIndex={disabled?-1:undefined} onClick={navigate}>{t('open_task')}</a>
   <a data-delivery-run href={link({sub:'runs',run:basis.task.run_id})} aria-disabled={disabled||undefined} tabIndex={disabled?-1:undefined} onClick={navigate}>{t('open_run')}</a>
   {onRefresh&&<button type="button" data-delivery-refresh disabled={disabled} onClick={onRefresh}>{t('refresh_source')}</button>}
  </nav>
  <details className="dk-source-details"><summary>{t('source_details')}</summary>
   <Facts values={[["task_id",basis.task.id],["review_id",basis.task.review_id],["run_id",basis.task.run_id],["run_version",basis.run_version],["task_sha256",basis.task.sha256],["basis_sha256",basis.sha256],["source_sha256",source.sha256]]}/>
   <section><h4>{t('test_evidence')}</h4><ol className="dk-source-evidence">{basis.tests.map((test,index)=><li key={index}><Facts values={[["run_id",test.run_id],["node_id",test.node_id],["attempt",test.attempt],["fence",test.fence],["worker_id",test.worker_id],["result_sha256",test.result_sha256],["plan_id",test.plan_id],["plan_revision",test.plan_revision],["plan_sha256",test.plan_sha256],["report_id",test.report_id],["report_sha256",test.report_sha256],["source_commit",test.source_commit]]}/></li>)}</ol></section>
   {basis.candidates.length>0&&<section data-delivery-candidates><h4>{t('candidate_evidence')}</h4><ol className="dk-source-evidence">{basis.candidates.map((candidate,index)=><li key={index}><Facts values={[["candidate_kind",t("candidate_kind."+candidate.kind)],["run_id",candidate.run_id],["node_id",candidate.node_id],["attempt",candidate.attempt],["fence",candidate.fence],["worker_id",candidate.worker_id],["head_commit",candidate.head_commit],["evidence_sha256",candidate.evidence_sha256],["transfer_id",candidate.transfer_id],["bundle_sha256",candidate.bundle_sha256]]}/></li>)}</ol></section>}
  </details>
  <details className="dk-source-details"><summary>{t('source_quote')}</summary><div className="dk-source-quote" data-delivery-quote>{source.quote}</div></details>
 </section>;
}
