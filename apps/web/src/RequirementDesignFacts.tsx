import type { RequirementDesignContext } from '../../../packages/contracts/src';
import { formatNumber, tr as t, useLocale } from './i18n';
import { sourceHref } from './navigation';
import './knowledge.css';

/** This is the immutable snapshot already recorded in the dispatch plan. A
 * fresh design read must never replace evidence shown for an older run. */
export function RequirementDesignFacts({ project, value }: { project: string; value?: RequirementDesignContext }) {
  useLocale();
  if (!value) return null;
  const href = sourceHref(project, { kind: 'requirements', id: value.requirement_id });
  const requirement = t('requirement-design.context.requirement_version', { version: formatNumber(value.requirement_version) });
  return <section className="knowledge-fixed" aria-label={t('requirement-design.context.fixed')}>
    <h3>{t('requirement-design.context.fixed')}</h3>
    <p className="knowledge-meta"><span>{t('requirement-design.context.revision', { revision: formatNumber(value.revision) })}</span>{href ? <a href={href}>{requirement}</a> : <span>{requirement}</span>}</p>
    <code className="knowledge-hash">{t('knowledge.hash')}: {value.sha256}</code>
    <small className="knowledge-hash">{t('requirement-design.context.adoption')}: {value.adoption_id} · {value.task_key}</small>
    {(['product', 'experience', 'technical'] as const).map(section => <article key={section}><details><summary>{t('requirement-design.' + section)}</summary><pre className="knowledge-content">{value.content[section]}</pre></details></article>)}
  </section>;
}
