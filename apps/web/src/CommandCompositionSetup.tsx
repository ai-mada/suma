import { useLayoutEffect, useRef, useState } from 'react';
import { Check, ChevronRight, Copy, Plus, Trash2 } from 'lucide-react';
import { formatNumber, tr, useLocale } from './i18n';
import { commandCompositionCommands, commandCompositionIssue, validCommandCompositionFile, validCommandCompositionOutput, validCommandCompositionDraft, type CommandCompositionDraft, type CommandCompositionInput, type CommandCompositionIssue } from './command-composition-model';
import './CommandCompositionSetup.css';

type SourceRow = { key: string; path: string };
const t = (key: string, values?: Record<string, unknown>) => tr('command-composition.' + key, values);

/** The containing guide supplies a source key for account/project/node/API
 * changes. This local draft never uploads or persists any file reference. */
export function CommandCompositionSetup({ disabled = false, onUseCommands }: { disabled?: boolean; onUseCommands?: (draft: CommandCompositionDraft) => void }) {
  useLocale();
  const [shell, setShell] = useState<CommandCompositionInput['shell']>('');
  const [output, setOutput] = useState('');
  const [rows, setRows] = useState<SourceRow[]>([{ key: 'source-1', path: '' }, { key: 'source-2', path: '' }]);
  const [attempted, setAttempted] = useState(false), [copying, setCopying] = useState(false);
  const [copied, setCopied] = useState(false), [copyFailed, setCopyFailed] = useState(false);
  const [preview, setPreview] = useState<{ command: string; revision: number }>();
  const sequence = useRef(2), panel = useRef<HTMLDetailsElement>(null), focus = useRef<string | undefined>(undefined);
  const owner = useRef({ alive: true, disabled, revision: 0, inflight: false });
  if (owner.current.disabled !== disabled) { owner.current.disabled = disabled; owner.current.revision++; }
  useLayoutEffect(() => {
    owner.current.alive = true;
    return () => { owner.current.alive = false; owner.current.revision++; };
  }, []);
  useLayoutEffect(() => {
    if (!focus.current || disabled) return;
    const selector = focus.current === 'add' ? '[data-command-composition-add]' : '[data-command-composition-source="' + focus.current + '"]';
    focus.current = undefined; panel.current?.querySelector<HTMLElement>(selector)?.focus();
  }, [rows, disabled]);
  const input: CommandCompositionInput = { shell, output, files: rows.map(row => row.path) };
  const issue = commandCompositionIssue(input);
  const currentPreview = !disabled && preview?.revision === owner.current.revision ? preview.command : undefined;
  function edited() { owner.current.revision++; setPreview(undefined); setCopied(false); setCopyFailed(false); setAttempted(false); }
  function focusIssue(value: CommandCompositionIssue) {
    const selector = value.field === 'files'
      ? '[data-command-composition-source="' + (rows[(value.row || 1) - 1]?.key || '') + '"]'
      : '[data-command-composition-' + value.field + ']';
    panel.current?.querySelector<HTMLElement>(selector)?.focus();
  }
  async function copy() {
    if (disabled || owner.current.inflight) return;
    setAttempted(true); setCopyFailed(false);
    if (issue) { focusIssue(issue); return; }
    const result = commandCompositionCommands(input);
    if (!result) return;
    const revision = owner.current.revision;
    setPreview({ command: result.prepare, revision }); setCopied(false); setCopying(true); owner.current.inflight = true;
    try {
      await navigator.clipboard.writeText(result.prepare);
      if (owner.current.alive && !owner.current.disabled && revision === owner.current.revision) setCopied(true);
    } catch {
      if (owner.current.alive && !owner.current.disabled && revision === owner.current.revision) setCopyFailed(true);
    } finally {
      owner.current.inflight = false;
      if (owner.current.alive) setCopying(false);
    }
  }
  function add() {
    if (disabled || rows.length >= 16) return;
    edited(); const key = 'source-' + ++sequence.current; focus.current = key; setRows(before => [...before, { key, path: '' }]);
  }
  function remove(key: string) {
    if (disabled || rows.length <= 2) return;
    edited(); const index = rows.findIndex(row => row.key === key), next = rows.filter(row => row.key !== key);
    focus.current = next[Math.min(index, next.length - 1)]?.key || 'add'; setRows(next);
  }
  function useCommands() {
    if (!owner.current.alive || owner.current.disabled || !currentPreview || preview?.revision !== owner.current.revision || !onUseCommands) return;
    const draft = { kind: 'commands-only', shell, commands: output };
    if (validCommandCompositionDraft(draft)) onUseCommands(draft);
  }
  return <details className="command-composition" data-command-composition ref={panel}>
    <summary aria-disabled={disabled || undefined} tabIndex={disabled ? -1 : undefined} onClick={event => { if (disabled) event.preventDefault(); }}><ChevronRight size={16} aria-hidden="true" />{t('title')}</summary>
    <div className="command-composition-body">
      <p>{t('intro')}</p>
      <div className="command-composition-fields">
        <label>{t('shell')}<select data-command-composition-shell value={shell} disabled={disabled} aria-invalid={attempted && !shell || undefined}
          onChange={event => { edited(); setShell(event.target.value as CommandCompositionInput['shell']); }}>
          <option value="">{t('choose_shell')}</option><option value="powershell">PowerShell</option><option value="bash">Bash</option>
        </select></label>
        <label>{t('output')}<input data-command-composition-output value={output} disabled={disabled} maxLength={4096} spellCheck={false} autoComplete="off"
          aria-invalid={(!!output && !validCommandCompositionOutput(output)) || attempted && issue?.field === 'output' || undefined}
          onChange={event => { edited(); setOutput(event.target.value); }} />
          {!!output && !validCommandCompositionOutput(output) && <small>{t('issue.output_path')}</small>}
          {issue?.code === 'output_source' && <small>{t('issue.output_source')}</small>}</label>
      </div>
      <div className="command-composition-sources">{rows.map((row, index) => <div className="command-composition-row" key={row.key}>
        <label>{t('source', { number: formatNumber(index + 1) })}<input data-command-composition-source={row.key} value={row.path} disabled={disabled} maxLength={4096} spellCheck={false} autoComplete="off"
          aria-invalid={(!!row.path && !validCommandCompositionFile(row.path)) || attempted && issue?.row === index + 1 || undefined}
          onChange={event => { const path = event.target.value; edited(); setRows(before => before.map(value => value.key === row.key ? { ...value, path } : value)); }} />
          {!!row.path && !validCommandCompositionFile(row.path) && <small>{t('issue.source_path')}</small>}
          {issue?.code === 'duplicate_source' && issue.row === index + 1 && <small>{t('issue.duplicate_source', { number: formatNumber(index + 1) })}</small>}</label>
        <button type="button" data-command-composition-remove={row.key} disabled={disabled || rows.length <= 2} aria-label={t('remove', { number: formatNumber(index + 1) })} onClick={() => remove(row.key)}><Trash2 size={16} aria-hidden="true" /></button>
      </div>)}</div>
      <button type="button" className="text-button" data-command-composition-add disabled={disabled || rows.length >= 16} onClick={add}><Plus size={16} aria-hidden="true" />{t('add')}</button>
      <p className="command-composition-muted">{t('limits')}</p>
      {attempted && issue && <p role="alert" data-command-composition-error>{t('issue.' + issue.code, { number: formatNumber(issue.row || 0) })}</p>}
      <button type="button" data-command-composition-copy disabled={disabled || copying} onClick={() => void copy()}>
        {copied && currentPreview ? <Check size={16} aria-hidden="true" /> : <Copy size={16} aria-hidden="true" />}{t(copying ? 'copying' : copied && currentPreview ? 'copied' : 'copy')}
      </button>
      {copyFailed && <p role="status">{t('copy_failed')}</p>}
      {currentPreview && <><pre tabIndex={0} data-command-composition-command>{currentPreview}</pre><p>{t('next')}</p>
        {onUseCommands && <><button type="button" data-command-composition-use onClick={useCommands}>{t('use')}</button><small>{t('use_hint')}</small></>}
      </>}
      <p className="command-composition-muted">{t('boundary')}</p>
    </div>
  </details>;
}
