import { useEffect, useId, useRef, useState } from 'react';
import { ArrowDownToLine, CircleAlert, CircleCheck, CircleHelp, RefreshCw } from 'lucide-react';
import { ApiError } from './api';
import { useApi, useApiView } from './api-context';
import { formatNumber, tr, useLocale } from './i18n';
import { dispatchOptionsScopeKey, exactDispatchOptionPath, readExactDispatchOption, type DispatchOptionsScope } from './task-dispatch-options-model';
import { toolReason } from './ToolFacts';
import type { ToolProfile } from './tool-model';
import { readToolTaskHandoffProfile, validToolTaskHandoffPin, type ToolTaskHandoffPin } from './tool-task-handoff-model';
import './TaskDispatchToolHandoff.css';

type Props = {
  scope: DispatchOptionsScope;
  pin: ToolTaskHandoffPin;
  scopeKey: string;
  disabled?: boolean;
  adopted: boolean;
  onInvalidate: () => void;
  onAdopt: (profile: ToolProfile) => void;
  onRefreshContext: () => void;
  onReturn: () => void;
};
type Phase = 'idle' | 'reading' | 'adopted' | 'changed' | 'stale' | 'forbidden' | 'missing' | 'unknown' |
  'tool_unavailable' | 'workflow_incompatible' | 'route_incompatible';
type Check = { phase: Phase; reason?: string; diagnostic?: unknown };
const t = (key: string, values?: Record<string, unknown>) => tr('tool-task-handoff-dispatch.' + key, values);
const returnsToTool = new Set<Phase>(['changed', 'forbidden', 'missing', 'tool_unavailable']);

function readFailure(error: unknown): Phase {
  if (!(error instanceof ApiError)) return 'unknown';
  switch (error.status) {
    case 401:
    case 403:
      return 'forbidden';
    case 404:
      return 'missing';
    case 409:
      return 'stale';
    default:
      return 'unknown';
  }
}

/** The pin is navigation intent. The current exact read checks task, workflow
 * and route compatibility before the original selection can adopt it. */
export function TaskDispatchToolHandoff(props: Props) {
  const api = useApi(), view = useApiView();
  const key = JSON.stringify([view?.userID, view?.organizationID, dispatchOptionsScopeKey(props.scope), props.scopeKey, props.pin]);
  const owner = useRef({ api, key, epoch: 0 });
  if (owner.current.api !== api || owner.current.key !== key)
    owner.current = { api, key, epoch: owner.current.epoch + 1 };
  const current = owner.current;
  const organization = view?.organizationID || '';
  return <HandoffScope key={current.epoch} {...props} organization={organization}
    disabled={props.disabled || !view?.userID || !organization || !validToolTaskHandoffPin(props.pin) || props.pin.project_id !== props.scope.project_id}
    owns={() => owner.current === current} />;
}

function HandoffScope({
  scope, organization, pin, disabled = false, adopted, onInvalidate, onAdopt, onRefreshContext, onReturn, owns,
}: Props & { organization: string; owns: () => boolean }) {
  const api = useApi();
  useLocale();
  const titleID = useId(), statusID = useId();
  const [check, setCheck] = useState<Check>({ phase: 'idle' });
  const alive = useRef(true), blocked = useRef(disabled);
  const controller = useRef<AbortController | undefined>(undefined);
  blocked.current = disabled;
  useEffect(() => {
    alive.current = true;
    return () => { alive.current = false; controller.current?.abort(); controller.current = undefined; };
  }, []);
  useEffect(() => {
    if (disabled) {
      controller.current?.abort();
      controller.current = undefined;
      setCheck({ phase: 'idle' });
    }
  }, [disabled]);

  async function adopt() {
    if (blocked.current || !owns() || controller.current) return;
    if (check.phase === 'stale') {
      setCheck({ phase: 'idle' });
      onRefreshContext();
      return;
    }
    const abort = new AbortController();
    controller.current = abort;
    // Rechecking invalidates only a selection owned by this handoff. Other
    // tools chosen manually or from the directory remain in the form.
    onInvalidate();
    setCheck({ phase: 'reading' });
    const current = () => alive.current && owns() && !blocked.current &&
      controller.current === abort && !abort.signal.aborted;
    try {
      const raw = await api<unknown>(exactDispatchOptionPath(scope, pin.id), undefined, 'GET', undefined, abort.signal);
      if (!current()) return;
      const option = readExactDispatchOption(raw, scope, organization, pin.id);
      if (!option) { setCheck({ phase: 'unknown' }); return; }

      const sameSource = option.tool.worker_id === pin.worker_id && option.tool.profile.profile_sha256 === pin.profile_sha256;
      // A new version can explain why the same underlying configuration was
      // disabled. It still never replaces or updates the original pin.
      const unavailable = sameSource && option.reason_code === 'tool_unavailable';
      if (!sameSource || option.tool.version !== pin.version) {
        setCheck({ phase: 'changed', reason: unavailable ? option.reason : undefined,
          diagnostic: unavailable ? option.reason_diagnostic : undefined });
        return;
      }
      if (option.reason_code !== 'eligible') {
        setCheck({ phase: option.reason_code, reason: option.reason, diagnostic: option.reason_diagnostic });
        return;
      }
      const profile = readToolTaskHandoffProfile(option.tool, pin);
      if (!profile) { setCheck({ phase: 'unknown' }); return; }
      onAdopt(profile);
      setCheck({ phase: 'adopted' });
    } catch (error) {
      if (current()) setCheck({ phase: readFailure(error) });
    } finally {
      if (controller.current === abort) controller.current = undefined;
    }
  }

  const phase = check.phase === 'adopted' && !adopted ? 'idle' : check.phase;
  const StatusIcon = phase === 'adopted' ? CircleCheck : phase === 'unknown' ? CircleHelp :
    phase === 'reading' ? RefreshCw : phase === 'idle' ? undefined : CircleAlert;
  const ActionIcon = phase === 'idle' ? ArrowDownToLine : RefreshCw;
  const action = phase === 'reading' ? 'reading' : phase === 'stale' ? 'refresh_context' :
    phase === 'idle' ? 'adopt' : 'recheck';
  return (
    <section className="task-dispatch-tool-handoff" data-dispatch-handoff aria-labelledby={titleID}>
      <div className="task-dispatch-tool-handoff-heading">
        <div>
          <h3 id={titleID}>{t('title')}</h3>
          <p>{t('target', { name: pin.name, version: formatNumber(pin.version), worker: pin.worker_id })}</p>
        </div>
        <button type="button" data-dispatch-handoff-adopt disabled={disabled} aria-disabled={phase === 'reading' || undefined}
          aria-describedby={statusID} onClick={() => void adopt()}>
          <ActionIcon size={16} aria-hidden="true" />{t(action)}
        </button>
      </div>
      <p id={statusID} role="status" data-dispatch-handoff-status={phase}>
        {StatusIcon && <StatusIcon size={16} aria-hidden="true" />}{t('phase.' + phase)}
      </p>
      {check.reason && <details className="task-dispatch-tool-handoff-reason">
        <summary>{t('details')}</summary>
        <p>{toolReason(check.reason, check.diagnostic)}</p>
      </details>}
      {returnsToTool.has(phase) && <button type="button" className="text-button" disabled={disabled} data-dispatch-handoff-return
        onClick={onReturn}>{t('back')}</button>}
      <small>{t('boundary')}</small>
    </section>
  );
}
