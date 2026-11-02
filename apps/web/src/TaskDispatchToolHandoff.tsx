import { useEffect, useRef, useState } from 'react';
import { ArrowDownToLine } from 'lucide-react';
import { ApiError, query } from './api';
import { useApi, useApiView } from './api-context';
import { formatNumber, tr, useLocale } from './i18n';
import type { ToolProfile } from './tool-model';
import { readToolTaskHandoffProfile, validToolTaskHandoffPin, type ToolTaskHandoffPin } from './tool-task-handoff-model';
import './TaskDispatchToolHandoff.css';

type Props = {
  project: string;
  pin: ToolTaskHandoffPin;
  scopeKey: string;
  disabled?: boolean;
  adopted: boolean;
  onInvalidate: () => void;
  onAdopt: (profile: ToolProfile) => void;
};
type Phase = 'idle' | 'reading' | 'adopted' | 'changed' | 'forbidden' | 'unknown';
const t = (key: string, values?: Record<string, unknown>) => tr('tool-task-handoff-dispatch.' + key, values);

/** The pin is a human's navigation intent, never current availability or task
 * permission. Every explicit adoption reads the original exact profile again. */
export function TaskDispatchToolHandoff(props: Props) {
  const api = useApi(), view = useApiView();
  const key = JSON.stringify([view?.userID, view?.organizationID, props.project, props.scopeKey, props.pin]);
  const owner = useRef({ api, key, epoch: 0 });
  if (owner.current.api !== api || owner.current.key !== key)
    owner.current = { api, key, epoch: owner.current.epoch + 1 };
  const current = owner.current;
  return <HandoffScope key={current.epoch} {...props}
    disabled={props.disabled || !view?.userID || !view.organizationID || !validToolTaskHandoffPin(props.pin) || props.pin.project_id !== props.project}
    owns={() => owner.current === current} />;
}

function HandoffScope({ project, pin, disabled = false, adopted, onInvalidate, onAdopt, owns }: Props & { owns: () => boolean }) {
  const api = useApi();
  useLocale();
  const [phase, setPhase] = useState<Phase>('idle');
  const alive = useRef(true), intent = useRef(0), blocked = useRef(disabled);
  const controller = useRef<AbortController | undefined>(undefined);
  blocked.current = disabled;
  useEffect(() => {
    alive.current = true;
    return () => { alive.current = false; intent.current++; controller.current?.abort(); };
  }, []);
  useEffect(() => {
    if (disabled) { intent.current++; controller.current?.abort(); setPhase('idle'); }
  }, [disabled]);
  async function adopt() {
    if (blocked.current || !owns() || phase === 'reading') return;
    const generation = ++intent.current, callback = onAdopt, abort = new AbortController();
    controller.current?.abort(); controller.current = abort;
    // Only the parent knows whether this exact tool still owns its selection.
    // It conditionally invalidates that selection before a new current read.
    onInvalidate();
    setPhase('reading');
    const current = () => alive.current && owns() && !blocked.current &&
      generation === intent.current && !abort.signal.aborted;
    try {
      const raw = await api<unknown>('/tool-profiles/' + encodeURIComponent(pin.id) + query(project), undefined, 'GET', undefined, abort.signal);
      if (!current()) return;
      const profile = readToolTaskHandoffProfile(raw, pin);
      if (!profile) { setPhase('changed'); return; }
      callback(profile); setPhase('adopted');
    } catch (error) {
      if (!current()) return;
      setPhase(error instanceof ApiError && [401, 403, 404].includes(error.status) ? 'forbidden' : 'unknown');
    }
  }
  const visiblePhase = phase === 'adopted' && !adopted ? 'idle' : phase;
  return <section className="task-dispatch-tool-handoff" data-dispatch-handoff aria-label={t('title')}>
    <div><h3>{t('title')}</h3><p>{t('target', { name: pin.name, version: formatNumber(pin.version), worker: pin.worker_id })}</p></div>
    <button type="button" data-dispatch-handoff-adopt disabled={disabled || phase === 'reading'} onClick={() => void adopt()}>
      <ArrowDownToLine size={16} aria-hidden="true" />{t(phase === 'reading' ? 'reading' : 'adopt')}
    </button>
    <p role="status" data-dispatch-handoff-status={visiblePhase}>{t('phase.' + visiblePhase)}</p>
    <small>{t('boundary')}</small>
  </section>;
}
