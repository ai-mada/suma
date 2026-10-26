import { formatDateTime, formatNumber, tr as t, useLocale } from './i18n';
import type {KnowledgeIntelligenceAdoption,SkillCatalogAdoption} from '../../../packages/contracts/src';
import { sourceHref } from './navigation';
import {knowledgeSourceLabel,type KnowledgeRevision,type KnowledgeSnapshot} from './knowledge-model';
import './knowledge.css';
type TextSource = Pick<KnowledgeSnapshot, 'content' | 'sha256' | 'sources'>;
function KnowledgeTextFacts({ project, value }: {
    project: string;
    value: TextSource;
}) {
    return <><pre className="knowledge-content">{value.content}</pre><span className="knowledge-hash">{t('knowledge.hash')}: <code>{value.sha256}</code></span><h4>{t('knowledge.source_list')}</h4>{value.sources.length ? value.sources.map((source, index) => { const href = sourceHref(project, source); return <div className="knowledge-source" key={source.kind + ':' + source.id + ':' + index}><strong>{source.title}</strong><div className="knowledge-meta"><span>{knowledgeSourceLabel(source.kind)}</span><span>{t('knowledge.source_version', { version: formatNumber(source.version) })}</span>{href && <a href={href}>{t('knowledge.source_open')}</a>}</div><code className="knowledge-hash">{source.sha256}</code>{source.quote !== undefined && <details><summary>{t('knowledge.source_quote_view')}</summary><blockquote>{source.quote}</blockquote></details>}</div>; }) : <p className="muted">{t('knowledge.source_none')}</p>}</>;
}
export function KnowledgeRevisionFacts({ project, revision }: {
    project: string;
    revision: KnowledgeRevision;
}) {
    useLocale();
    return <><p className="knowledge-meta">{t('knowledge.revision', { revision: formatNumber(revision.revision) })} · {t('knowledge.state.' + revision.state)} · {t('knowledge.method.' + revision.method)}</p>{revision.summary && <p>{revision.summary}</p>}<KnowledgeTextFacts project={project} value={revision}/></>;
}
export function CatalogAdoptionFacts({project,value}:{project:string;value?:SkillCatalogAdoption|null}){
 useLocale();if(!value)return null;
 return <details className="knowledge-catalog-lineage"><summary>{t('skill-catalog.adoption_lineage')}</summary><p>{t('skill-catalog.adoption_lineage_hint')}</p><p>{t('skill-catalog.version',{value:formatNumber(value.catalog_version)})} · {t('skill-catalog.origin_revision',{value:formatNumber(value.origin.revision)})}</p><code className="knowledge-hash">{value.catalog_id} · {value.origin.project_id} · {value.origin.knowledge_id}</code><code className="knowledge-hash">{value.origin.sha256}</code><p>{t('skill-catalog.adopted_by',{value:value.adopted_by})} · {formatDateTime(value.adopted_at)}</p><p>{value.reason}</p><a href={'?'+new URLSearchParams({project,view:'knowledge',sub:'skill-catalog',record:value.catalog_id})}>{t('skill-catalog.open_catalog')}</a></details>;
}
export function ModelAdoptionFacts({project,value}:{project:string;value?:KnowledgeIntelligenceAdoption|null}){
 useLocale();if(!value||value.project_id!==project)return null;
 return <details className="knowledge-model-lineage"><summary>{t('knowledge-intelligence.lineage')}</summary><p>{t('knowledge-intelligence.lineage_hint')}</p><p>{t('knowledge-intelligence.lineage_analysis',{id:value.analysis_id,version:formatNumber(value.analysis_version)})}</p><code className="knowledge-hash">{value.run_id}</code><p>{t('knowledge-intelligence.lineage_sources',{value:value.source_indices.map(index=>formatNumber(index)).join(', ')})}</p><p>{t('knowledge-intelligence.lineage_source_hash')}</p><code className="knowledge-hash">{value.source_sha256}</code><p>{t('knowledge-intelligence.lineage_output_hash')}</p><code className="knowledge-hash">{value.original_output_sha256}</code><p>{t('knowledge-intelligence.lineage_profile_hash')}</p><code className="knowledge-hash">{value.tool_profile_sha256}</code><p>{t('knowledge-intelligence.lineage_draft',{value:formatNumber(value.revision)})}</p><code className="knowledge-hash">{value.knowledge_id} · {value.sha256}</code><p>{t('knowledge-intelligence.lineage_by',{value:value.adopted_by})} · {formatDateTime(value.adopted_at)}</p><p>{value.reason}</p><a href={'?'+new URLSearchParams({project,view:'knowledge',sub:'project-knowledge',panel:'knowledge-intelligence:'+value.analysis_id})}>{t('knowledge-intelligence.lineage_open')}</a>{' · '}<a href={'?'+new URLSearchParams({project,view:'development',sub:'runs',run:value.run_id})}>{t('knowledge-intelligence.open_run')}</a></details>;
}
/** These are immutable server snapshots from the reviewed plan, not freshly
 * resolved publications. Never replace their text with a later list result. */
export function KnowledgeContextFacts({ project, items }: {
    project: string;
    items?: readonly KnowledgeSnapshot[];
}) {
    useLocale();
    return <section className="knowledge-fixed" aria-label={t('knowledge.dispatch.fixed')}><h3>{t('knowledge.dispatch.fixed')}</h3>{items?.length ? items.map(item => <article key={item.id + ':' + item.revision}><details><summary><strong>{item.title}</strong> · {t('knowledge.kind.' + item.kind)} · {t('knowledge.revision', { revision: formatNumber(item.revision) })}</summary>{item.summary && <p>{item.summary}</p>}<KnowledgeTextFacts project={project} value={item}/><CatalogAdoptionFacts project={project} value={item.catalog_adoption}/><ModelAdoptionFacts project={project} value={item.model_adoption}/></details></article>) : <p className="muted">{t('knowledge.dispatch.none')}</p>}</section>;
}
