import {type MouseEvent} from 'react';
import type {CodeBaselineBinding} from '../../../packages/contracts/src';
import {formatNumber, tr, useLocale} from './i18n';

const t = (key: string) => tr('terminal.baseline.' + key);
type Props = {project: string; binding?: CodeBaselineBinding | null; adminCommit?: string; selectedCommit?: string; candidateCommit?: string; title?: string};

/** Immutable source facts only. Current qualification and acceptance are
 * displayed separately by the owning terminal/candidate surface. */
export function TerminalBaselineFacts({project, binding, adminCommit, selectedCommit, candidateCommit, title}: Props) {
  useLocale();
  const admin = adminCommit || (binding ? binding.source.bundle_base_commit ?? binding.source.base_commit : '');
  const selected = selectedCommit || binding?.source.head_commit || '';
  function follow(event: MouseEvent) {if (!window.dispatchEvent(new Event('suma:navigate', {cancelable: true}))) event.preventDefault();}
  return <section className="terminal-baseline-facts" data-terminal-baseline aria-label={title || t('title')}>
    <strong>{title || t('title')}</strong>
    <dl className="terminal-facts"><dt>{t('admin')}</dt><dd data-terminal-admin-base>{admin ? <code>{admin}</code> : t('not_reported')}</dd>
      <dt>{t('selected')}</dt><dd data-terminal-selected-base>{selected ? <code>{selected}</code> : t('not_reported')}</dd>
      {candidateCommit && <><dt>{t('candidate')}</dt><dd data-terminal-new-candidate><code>{candidateCommit}</code></dd></>}
    </dl>
    {!binding && <p className="terminal-note">{t('administrator')}</p>}
    {binding && <details><summary>{t('source')}</summary><p className="terminal-note">{t('binding_version')} {formatNumber(binding.task_version)}</p>
      <a href={'?' + new URLSearchParams({project, view: 'development', sub: 'runs', run: binding.source.source.run_id})} onClick={follow}>{t('open_source')}</a>
      <dl className="terminal-facts"><dt>{t('binding')}</dt><dd>{binding.id}</dd><dt>{t('digest')}</dt><dd>{binding.binding_sha256}</dd></dl></details>}
  </section>;
}
