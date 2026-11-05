import { useEffect, useId, useRef, useState } from 'react';
import { Copy } from 'lucide-react';
import { formatNumber, tr, useLocale } from '../i18n';
import {
  POOL_STATE_DIAGNOSTIC_MAX_CHARS,
  poolStateDiagnosticCommand,
  readPoolStateDiagnostic,
  type PoolStateDiagnostic,
} from './pool-state-diagnostic-model';
import './PoolStateDiagnosticPanel.css';

type Props = { runnerLabel: string; disabled?: boolean };
const t = (key: string, values?: Record<string, unknown>) => tr('pool-state-diagnostic.' + key, values);

/** Keep imported evidence scoped to an open, currently available runner dialog. */
export function PoolStateDiagnosticPanel({ runnerLabel, disabled }: Props) {
  useLocale();
  if (disabled) return <p role="status">{t('unavailable')}</p>;
  return <DiagnosticBody key={runnerLabel} runnerLabel={runnerLabel} />;
}

function DiagnosticBody({ runnerLabel }: Pick<Props, 'runnerLabel'>) {
  const locale = useLocale();
  const pathHint = useId();
  const pasteHint = useId();
  const slotCommandID = useId();
  const [shell, setShell] = useState<'powershell' | 'bash'>('powershell');
  const [state, setState] = useState('.cache/runner');
  const [raw, setRaw] = useState('');
  const [report, setReport] = useState<PoolStateDiagnostic>();
  const [invalid, setInvalid] = useState(false);
  const [slotID, setSlotID] = useState<string>();
  const [clipboard, setClipboard] = useState<{ target: 'overview' | 'slot'; result: 'copied' | 'copy_failed' }>();
  const copyGeneration = useRef(0);
  const pasteField = useRef<HTMLTextAreaElement>(null);
  const resultRegion = useRef<HTMLDivElement>(null);
  useEffect(() => () => { copyGeneration.current++; }, []);
  useEffect(() => {
    if (report) resultRegion.current?.focus();
    else if (invalid) pasteField.current?.focus();
  }, [report, invalid]);

  const commandOptions = { state, shell, locale };
  const command = poolStateDiagnosticCommand(commandOptions);
  const slotCommand = slotID === undefined ? undefined : poolStateDiagnosticCommand({ ...commandOptions, slotID });

  function clearClipboard() {
    copyGeneration.current++;
    setClipboard(undefined);
  }

  function clearReport() {
    clearClipboard();
    setRaw('');
    setReport(undefined);
    setInvalid(false);
    setSlotID(undefined);
  }

  function preview() {
    const parsed = readPoolStateDiagnostic(raw);
    clearReport();
    setReport(parsed);
    setInvalid(!parsed);
  }

  async function copy(value: string, target: 'overview' | 'slot') {
    const generation = ++copyGeneration.current;
    setClipboard(undefined);
    try {
      await navigator.clipboard.writeText(value);
      if (generation === copyGeneration.current) setClipboard({ target, result: 'copied' });
    } catch {
      if (generation === copyGeneration.current) setClipboard({ target, result: 'copy_failed' });
    }
  }

  return <div className="pool-state-diagnostic" data-pool-state-diagnostic>
    <p className="pool-diagnostic-target">{t('target')} <strong>{runnerLabel}</strong></p>
    <div className="pool-diagnostic-fields">
      <label>{t('shell')}
        <select data-pool-diagnostic-shell value={shell} onChange={event => {
          clearClipboard();
          setShell(event.currentTarget.value === 'bash' ? 'bash' : 'powershell');
        }}>
          <option value="powershell">{t('powershell')}</option>
          <option value="bash">{t('bash')}</option>
        </select>
      </label>
      <label>{t('state')}
        <input data-pool-diagnostic-state value={state} maxLength={4096}
          spellCheck={false} autoComplete="off" aria-invalid={!command} aria-describedby={pathHint}
          onChange={event => { clearReport(); setState(event.currentTarget.value); }} />
      </label>
    </div>
    <p className="pool-diagnostic-muted" id={pathHint}>{t('state_hint')}</p>
    {command ? <div className="pool-diagnostic-command">
      <pre data-pool-diagnostic-command tabIndex={0} aria-label={t('command_label')}><code>{command}</code></pre>
      <div className="pool-diagnostic-actions">
        <button type="button" data-pool-diagnostic-copy onClick={() => void copy(command, 'overview')}>
          <Copy size={15} aria-hidden="true" />{t('copy_command')}
        </button>
        <p className="pool-diagnostic-clipboard" role="status">{clipboard?.target === 'overview' && t(clipboard.result)}</p>
      </div>
    </div> : <p className="pool-diagnostic-message" role="status">{t('invalid_path')}</p>}
    <details className="pool-diagnostic-import" data-pool-diagnostic-import>
      <summary>{t('import')}</summary>
      <div className="pool-diagnostic-import-body">
        <p className="pool-diagnostic-muted" id={pasteHint}>{t('paste_hint')}</p>
        <label>{t('paste_label')}
          <textarea ref={pasteField} data-pool-diagnostic-paste value={raw} rows={4} maxLength={POOL_STATE_DIAGNOSTIC_MAX_CHARS}
            spellCheck={false} autoComplete="off" aria-describedby={pasteHint} onChange={event => {
              clearClipboard();
              setReport(undefined);
              setInvalid(false);
              setSlotID(undefined);
              setRaw(event.currentTarget.value);
            }} />
        </label>
        <div className="pool-diagnostic-actions">
          <button type="button" data-pool-diagnostic-preview disabled={!command || !raw.trim()} onClick={preview}>{t('preview')}</button>
          {(raw || report || invalid) && <button type="button" onClick={() => { clearReport(); pasteField.current?.focus(); }}>{t('clear')}</button>}
        </div>
        {invalid && <p className="pool-diagnostic-message" role="status">{t('invalid_report')}</p>}
        {report && <div ref={resultRegion} className="pool-diagnostic-report" role="region" aria-label={t('slots_label')} tabIndex={-1}>
          <p className="pool-diagnostic-muted">{t('report_boundary')}</p>
          <p role="status" className={report.complete ? 'pool-diagnostic-summary' : 'pool-diagnostic-message'}>
            {t(report.status === 'available' ? (report.complete ? 'complete' : 'partial') : 'report_status.' + report.status)}
          </p>
          {report.status === 'available' && report.slots.length === 0 && <p>{t('empty')}</p>}
          {report.slots.length > 0 && <ul className="pool-diagnostic-slots" aria-label={t('slots_label')} tabIndex={0}>
            {report.slots.map(slot => {
              const selected = slotID === slot.slot_id;
              const counts = slot.journal_counts;
              return <li key={slot.slot_id} data-pool-diagnostic-slot={slot.slot_id}>
                <div className="pool-diagnostic-slot-main">
                  <div>
                    <strong>{t('slot', { slot: slot.slot_id })}</strong>
                    <p className="pool-diagnostic-muted">{t('slot_states', {
                      journal: t('file_status.' + slot.journal_status),
                      claim: t('claim_status.' + slot.claim_state),
                    })}</p>
                    {counts && <div className="pool-diagnostic-counts">
                      <span className={counts.unknown_results > 0 ? 'pool-diagnostic-attention' : undefined}>
                        {t('unknown_results', { count: formatNumber(counts.unknown_results) })}
                      </span>
                      <span>{t('pending_results', { count: formatNumber(counts.pending_results) })}</span>
                      <span>{t('unresolved_inbox', { count: formatNumber(counts.unresolved_inbox) })}</span>
                    </div>}
                  </div>
                  <button type="button" data-pool-diagnostic-slot-action aria-expanded={selected}
                    aria-controls={selected ? slotCommandID : undefined} onClick={() => {
                      clearClipboard();
                      setSlotID(selected ? undefined : slot.slot_id);
                    }}>{t('original_log', { slot: slot.slot_id })}</button>
                </div>
                {selected && slotCommand && <div id={slotCommandID} className="pool-diagnostic-command">
                  <pre data-pool-diagnostic-slot-command tabIndex={0} aria-label={t('slot_command_label', { slot: slot.slot_id })}><code>{slotCommand}</code></pre>
                  <div className="pool-diagnostic-actions">
                    <button type="button" data-pool-diagnostic-slot-copy onClick={() => void copy(slotCommand, 'slot')}>
                      <Copy size={15} aria-hidden="true" />{t('copy_slot')}
                    </button>
                    <p className="pool-diagnostic-clipboard" role="status">{clipboard?.target === 'slot' && t(clipboard.result)}</p>
                  </div>
                </div>}
              </li>;
            })}
          </ul>}
        </div>}
      </div>
    </details>
  </div>;
}
