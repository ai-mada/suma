import { formatNumber, tr as t, useLocale } from './i18n';
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
/** These are immutable server snapshots from the reviewed plan, not freshly
 * resolved publications. Never replace their text with a later list result. */
export function KnowledgeContextFacts({ project, items }: {
    project: string;
    items?: readonly KnowledgeSnapshot[];
}) {
    useLocale();
    return <section className="knowledge-fixed" aria-label={t('knowledge.dispatch.fixed')}><h3>{t('knowledge.dispatch.fixed')}</h3>{items?.length ? items.map(item => <article key={item.id + ':' + item.revision}><details><summary><strong>{item.title}</strong> · {t('knowledge.kind.' + item.kind)} · {t('knowledge.revision', { revision: formatNumber(item.revision) })}</summary>{item.summary && <p>{item.summary}</p>}<KnowledgeTextFacts project={project} value={item}/></details></article>) : <p className="muted">{t('knowledge.dispatch.none')}</p>}</section>;
}
