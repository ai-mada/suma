import { useEffect, useState } from 'react';
import type { TaskDesignContextView } from '../../../packages/contracts/src';
import { query, type ApiCaller, type Session } from './api';
import { useApi } from './api-context';
import { formatDateTime, formatNumber, tr, UiError, useLocale } from './i18n';
import { sourceHref } from './navigation';
import { ErrorNotice, Loading } from './ui';
import './task-design-provenance.css';

type Props = { project: string; taskId: string; taskVersion: number; refresh: number; session: Session };
type ReadState = { caller: ApiCaller; scope: string; value?: TaskDesignContextView; error?: unknown; loading: boolean };
const text = (key: string, values?: Record<string, unknown>) => tr('task-design-provenance.' + key, values);
const eligibilityCodes = new Set(['none', 'current', 'task_changed', 'requirement_changed', 'publication_changed', 'retired', 'excluded', 'source_unavailable']);

/** The task's adopted source is a separate, authorized read. Editable task data
 * and the latest requirement publication cannot supply historical evidence. */
export function TaskDesignProvenance({ project, taskId, taskVersion, refresh, session }: Props) {
  useLocale();
  const api = useApi(), [retry, setRetry] = useState(0), [read, setRead] = useState<ReadState>();
  const scope = JSON.stringify([session.user.id, session.organization.id, project, taskId, taskVersion, refresh, retry]);
  const current = read?.caller === api && read.scope === scope ? read : undefined;
  useEffect(() => {
    let alive = true;
    const controller = new AbortController();
    setRead({ caller: api, scope, loading: true });
    void api<TaskDesignContextView>('/tasks/' + encodeURIComponent(taskId) + '/design-context' + query(project), undefined, 'GET', undefined, controller.signal)
      .then(value => {
        if (value.project_id !== project || value.task_id !== taskId) throw new UiError('task-design-provenance.invalid_read');
        if (alive) setRead({ caller: api, scope, loading: false, value });
      })
      .catch(error => { if (alive) setRead({ caller: api, scope, loading: false, error }); });
    return () => { alive = false; controller.abort(); };
  }, [api, scope, project, taskId]);

  const value = current?.value;
  const eligibility = value && eligibilityCodes.has(value.eligibility) && (value.bound ? value.eligibility !== 'none' : value.eligibility === 'none') ? value.eligibility : 'unknown';
  const snapshot = value?.bound && value.context && value.requirement && value.adopted_task
    && value.context.requirement_id === value.requirement.id && value.context.requirement_version === value.requirement.version && value.context.task_key === value.adopted_task.key
    ? { context: value.context, requirement: value.requirement, task: value.adopted_task } : undefined;
  const source = snapshot ? sourceHref(project, { project_id: value?.project_id, kind: 'requirements', id: snapshot.context.requirement_id }) : undefined;
  const href = source ? source + '&panel=requirement-design' : undefined;
  return <section className="detail-section task-design-provenance" aria-label={text('title')} aria-busy={!current || current.loading}>
    <h3>{text('title')}</h3>
    {(!current || current.loading) && <Loading/>}
    {Boolean(current?.error) && <ErrorNotice error={current?.error} onRetry={() => setRetry(count => count + 1)}/>}
    {value && <>
      <p className="tdp-status" data-state={eligibility}>{text('eligibility.' + eligibility)}</p>
      <p className="tdp-hint">{text('reason.' + eligibility)}</p>
      {snapshot && <>
        <strong className="tdp-source-title">{snapshot.requirement.title}</strong>
        <div className="tdp-versions"><span>{text('requirement_version', { version: formatNumber(snapshot.context.requirement_version) })}</span><span>{text('design_revision', { revision: formatNumber(snapshot.context.revision) })}</span></div>
        <p className="tdp-key">{text('task_key')}: <code>{snapshot.context.task_key}</code></p>
        {href && <a className="tdp-workspace" href={href}>{text('open_workspace')}</a>}
      </>}
      {value.bound && !snapshot && <p className="tdp-unavailable">{text('snapshot_unavailable')}</p>}
      <div className="tdp-current">
        <span>{text('checked_task', { version: formatNumber(value.task_version) })}</span>
        {value.bound && value.current_requirement_version > 0 && <span>{text('current_requirement', { version: formatNumber(value.current_requirement_version) })}</span>}
        {value.bound && value.current_published_revision > 0 && <span>{text('current_publication', { revision: formatNumber(value.current_published_revision) })}</span>}
      </div>
      {snapshot && <details className="tdp-snapshot">
        <summary>{text('review_snapshot')}</summary>
        <dl className="tdp-metadata">
          <dt>{text('adoption_id')}</dt><dd><code>{snapshot.context.adoption_id}</code></dd>
          <dt>{text('design_hash')}</dt><dd><code>{snapshot.context.sha256}</code></dd>
          {value.adopted_by && <><dt>{text('adopted_by')}</dt><dd>{value.adopted_by}</dd></>}
          {value.adopted_at && <><dt>{text('adopted_at')}</dt><dd><time dateTime={value.adopted_at}>{formatDateTime(value.adopted_at)}</time></dd></>}
          {value.current_design_version > 0 && <><dt>{text('current_design')}</dt><dd>{formatNumber(value.current_design_version)}</dd></>}
        </dl>
        {(['product', 'experience', 'technical'] as const).map(section => <article key={section}><h4>{text('section.' + section)}</h4><pre className="tdp-body">{snapshot.context.content[section]}</pre></article>)}
        <article><h4>{text('original_requirement')}</h4><strong>{snapshot.requirement.title}</strong><p className="tdp-body">{snapshot.requirement.description}</p><h5>{text('acceptance')}</h5><ul>{snapshot.requirement.acceptance_criteria.map((criterion, index) => <li key={index}>{criterion}</li>)}</ul></article>
        <article><h4>{text('original_task')}</h4><strong>{snapshot.task.title}</strong><p className="tdp-body">{snapshot.task.description}</p><h5>{text('acceptance')}</h5><ul>{snapshot.task.acceptance_criteria.map((criterion, index) => <li key={index}>{criterion}</li>)}</ul>
          <h5>{text('dependencies')}</h5>{snapshot.task.depends_on.length ? <ul>{snapshot.task.depends_on.map(key => <li key={key}><code>{key}</code></li>)}</ul> : <p className="tdp-hint">{text('no_dependencies')}</p>}
          <h5>{text('coverage')}</h5><p>{snapshot.task.covers.map(index => formatNumber(index)).join(', ')}</p>
        </article>
      </details>}
    </>}
  </section>;
}
