import { useEffect, useRef, useState } from 'react';
import { ChevronRight, Copy, RefreshCw } from 'lucide-react';
import type { PoolRunnerReadiness } from '../../../../packages/contracts/src';
import { ApiError } from '../api';
import { useApi, useApiView } from '../api-context';
import { formatDateTime, formatNumber, tr, useLocale } from '../i18n';
import {
  buildPoolToolHref, poolConfigConnectionCommands, poolRunnerReadinessPath,
  poolRunnerReadinessRemainingMs, readPoolRunnerReadiness, type PreparedPoolConnection,
  validPoolConnectionControl,
} from './pool-config-connection-model';
import './PoolConfigConnection.css';
import type {PoolToolRecovery} from './PoolToolRecoveryPanel';
import {buildPoolRecoveryProjectHref,validPoolToolRecoveryPin} from './pool-tool-recovery-model';
import {buildPoolSetupProjectHref,validPoolToolSetupPin} from './pool-tool-setup-model';

export type PoolConfigConnectionPanelProps = {
  ownerTenantID: string;
  poolID: string;
  runnerID: string;
  workerID: string;
  prepared: PreparedPoolConnection;
  disabled?: boolean;
  recovery?: PoolToolRecovery;
};
const t = (key: string, values?: Record<string, unknown>) => tr('pool-config-connection.' + key, values);
const observationDateOptions: Intl.DateTimeFormatOptions = {
  year: 'numeric', month: '2-digit', day: '2-digit',
  hour: '2-digit', minute: '2-digit', second: '2-digit',
};

/** This local guide never launches a process. Its immutable preparation scope
 * is remounted on caller, actor, target or configuration changes. */
export function PoolConfigConnectionPanel(props: PoolConfigConnectionPanelProps) {
  const api = useApi(), view = useApiView();
  const basis = JSON.stringify([view?.userID, view?.organizationID, props.ownerTenantID,
    props.poolID, props.runnerID, props.workerID, props.prepared,props.recovery]);
  const owner = useRef({ api, basis, epoch: 0 });
  if (owner.current.api !== api || owner.current.basis !== basis)
    owner.current = { api, basis, epoch: owner.current.epoch + 1 };
  return <ConnectionScope key={owner.current.epoch} {...props}
    authorized={!!view?.userID && view.organizationID === props.ownerTenantID} />;
}

function ConnectionScope({ poolID, runnerID, workerID, prepared, recovery, disabled = false, authorized }: PoolConfigConnectionPanelProps & { authorized: boolean }) {
  const api = useApi();
  useLocale();
  const [identity, setIdentity] = useState('');
  const [state, setState] = useState(prepared.output + '.runner-state');
  const [mode, setMode] = useState<'maintenance' | 'tasks'>('maintenance');
  const [discovery, setDiscovery] = useState(false);
  const [preparation, setPreparation] = useState(false);
  const [control, setControl] = useState(window.location.origin);
  const [scopeIndex, setScopeIndex] = useState(0);
  const [result, setResult] = useState<{ value: PoolRunnerReadiness; receivedAt: number; requestMs: number }>();
  const [phase, setPhase] = useState<'idle' | 'reading' | 'read' | 'forbidden' | 'unknown'>('idle');
  const [copied, setCopied] = useState<'check' | 'start' | 'failed'>();
  const [, tick] = useState(0);
  const alive = useRef(true), intent = useRef(0), abort = useRef<AbortController | undefined>(undefined);
  const active = !disabled && authorized;
  const latest = useRef({ active, scopeIndex });
  latest.current = { active, scopeIndex };
  const scope = prepared.scopes[scopeIndex];
  const commands = poolConfigConnectionCommands({ prepared, identity, state, control, mode, discovery, preparation });
  const elapsed = result ? performance.now() - result.receivedAt + result.requestMs : 0;
  const remaining = result ? poolRunnerReadinessRemainingMs(result.value, elapsed) : 0;
  const current = active && phase === 'read' ? result?.value : undefined;
  // This link only opens the original project page, which rechecks current
  // access and tools. An inventory lease never grants or removes navigation.
  const exactRecovery=recovery&&(validPoolToolRecoveryPin(recovery.pin)||validPoolToolSetupPin(recovery.pin))&&recovery.pin.pool_id===poolID&&recovery.pin.runner_id===runnerID&&recovery.pin.worker_id===workerID&&current?.scope.tenant_id===recovery.pin.tenant_id&&current.scope.project_id===recovery.pin.project_id&&current.scope.grant_status==='approved';
  const exactHref=exactRecovery?(validPoolToolSetupPin(recovery.pin)?buildPoolSetupProjectHref(recovery.pin):buildPoolRecoveryProjectHref(recovery.pin)):undefined;
  const href = current && current.scope.can_open_project
    ? exactHref??buildPoolToolHref(current.scope.project_id, { poolID, runnerID }) : undefined;
  const registrationState = (value: string) => t('registration_state.' + (['active', 'paused', 'revoked'].includes(value) ? value : 'unknown'));
  useEffect(() => {
    alive.current = true;
    return () => { alive.current = false; intent.current++; abort.current?.abort(); };
  }, []);
  useEffect(() => {
    if (!active) { intent.current++; abort.current?.abort(); setResult(undefined); setPhase('idle'); setCopied(undefined); }
  }, [active]);
  useEffect(() => {
    if (!result || !active) return;
    const timer = setInterval(() => tick(value => value + 1), 1000);
    return () => clearInterval(timer);
  }, [result, active]);
  function changed() {
    intent.current++; abort.current?.abort(); setResult(undefined); setPhase('idle'); setCopied(undefined);
  }
  async function refresh() {
    if (!active || !scope) return;
    const path = poolRunnerReadinessPath(poolID, runnerID, scope.tenant_id, scope.project_id);
    if (!path) return;
    const generation = ++intent.current, index = scopeIndex, started = performance.now();
    const controller = new AbortController(); abort.current?.abort(); abort.current = controller;
    setResult(undefined); setPhase('reading');
    const owns = () => alive.current && !controller.signal.aborted && intent.current === generation &&
      latest.current.active && latest.current.scopeIndex === index;
    try {
      const raw = await api<unknown>(path, undefined, 'GET', undefined, controller.signal);
      if (!owns()) return;
      const value = readPoolRunnerReadiness(raw, { poolID, runnerID, workerID, tenantID: scope.tenant_id, projectID: scope.project_id });
      if (!value) throw new Error('Invalid readiness response');
      const receivedAt = performance.now();
      setResult({ value, receivedAt, requestMs: receivedAt - started }); setPhase('read');
    } catch (error) {
      if (!owns()) return;
      setResult(undefined); setPhase(error instanceof ApiError && [401, 403, 404].includes(error.status) ? 'forbidden' : 'unknown');
    }
  }
  async function copy(kind: 'check' | 'start') {
    const value = commands?.[kind];
    if (!active || !value) return;
    const generation = intent.current;
    try {
      await navigator.clipboard.writeText(value);
      if (alive.current && latest.current.active && generation === intent.current) setCopied(kind);
    } catch {
      if (alive.current && latest.current.active && generation === intent.current) setCopied('failed');
    }
  }
  return <details className="pool-config-connection" data-pool-config-connection>
    <summary><ChevronRight size={16} aria-hidden="true" />{t('title')}</summary>
    <div className="pool-connection-body">
      <p>{t('prepared_boundary')}</p>
      <p className="pool-config-muted">{t('target', { worker: workerID })}</p>
      <div className="pool-connection-fields">
        <label>{t('identity')}<input data-pool-connection-field="identity" value={identity} disabled={!active}
          onChange={event => { changed(); setIdentity(event.target.value); }} placeholder={t('identity_placeholder')} />
          <small>{t('identity_hint')}</small></label>
        <label>{t('state')}<input data-pool-connection-field="state" value={state} disabled={!active}
          onChange={event => { changed(); setState(event.target.value); }} /><small>{t('state_hint')}</small></label>
      </div>
      <label>{t('control_label')}<input data-pool-connection-field="control" value={control} disabled={!active}
        onChange={event => { changed(); setControl(event.target.value); }} /><small>{t('control_hint')}</small></label>
      {!validPoolConnectionControl(control) && <p role="status">{t('control_invalid')}</p>}
      <label>{t('mode')}<select data-pool-connection-field="mode" value={mode} disabled={!active}
        onChange={event => { changed(); setMode(event.target.value as 'maintenance' | 'tasks'); setDiscovery(false); setPreparation(false); }}>
        <option value="maintenance">{t('mode_maintenance')}</option><option value="tasks">{t('mode_tasks')}</option>
      </select></label>
      <label className="pool-connection-consent"><input type="checkbox" data-pool-connection-field="discovery" checked={discovery} disabled={!active}
        onChange={event => { changed(); setDiscovery(event.target.checked); if(!event.target.checked)setPreparation(false); }} /><span>{t('discovery')}</span></label>
      <label className="pool-connection-consent"><input type="checkbox" data-pool-connection-field="preparation" checked={preparation} disabled={!active||!discovery}
        onChange={event=>{if(!active||!discovery)return;changed();setPreparation(event.target.checked);}}/><span>{tr('managed-tool-preparation.allow_configuration')}</span></label>
      <p className="pool-config-muted">{tr('managed-tool-preparation.allow_configuration_help')}</p>
      <p className="pool-config-muted">{t(mode === 'maintenance' ? 'maintenance_hint' : 'tasks_hint')}</p>
      {!commands && identity && <p role="status">{t('paths_invalid')}</p>}
      <div className="pool-connection-actions">
        <button type="button" data-pool-connection-copy="check" disabled={!active || !commands?.check} onClick={() => void copy('check')}><Copy size={15} aria-hidden="true" />{t('copy_check')}</button>
        <button type="button" data-pool-connection-copy="start" disabled={!active || !commands?.start} onClick={() => void copy('start')}><Copy size={15} aria-hidden="true" />{t('copy_start')}</button>
      </div>
      {copied && <p role="status">{t(copied === 'failed' ? 'copy_failed' : 'copied')}</p>}
      {active && commands && <details className="pool-connection-command-details"><summary>{t('commands')}</summary>
        <pre tabIndex={0} data-pool-connection-command="check">{commands.check}</pre>
        {commands.start && <pre tabIndex={0} data-pool-connection-command="start">{commands.start}</pre>}
      </details>}
      <div className="pool-connection-read">
        <label>{t('project')}<select data-pool-connection-field="scope" value={scopeIndex} disabled={!active}
          onChange={event => { changed(); setScopeIndex(Number(event.target.value)); }}>
          {prepared.scopes.map((item, index) => <option key={item.tenant_id + ':' + item.project_id} value={index}>{item.tenant_id + ' / ' + item.project_id}</option>)}
        </select></label>
        <button type="button" data-pool-connection-refresh disabled={!active || !scope || phase === 'reading'} onClick={() => void refresh()}><RefreshCw size={15} aria-hidden="true" />{t('refresh')}</button>
      </div>
      {phase !== 'read' && <p role="status" data-pool-connection-status={phase}>{t('phase.' + phase)}</p>}
      {current && <div className="pool-connection-facts" data-pool-connection-readiness data-source-current={current.scope.source_current && remaining > 0 ? 'true' : 'false'}>
        <p className="pool-config-muted">{t('as_of', { at: formatDateTime(current.checked_at, observationDateOptions) })}</p>
        <dl><div><dt>{t('registration')}</dt><dd>{t('registration_value', { pool: registrationState(current.pool_status), runner: registrationState(current.runner.status) })}</dd></div>
          <div><dt>{t('credential')}</dt><dd>{t('credential_state.' + current.runner.credential_state)}</dd></div>
          <div><dt>{t('last_contact')}</dt><dd>{current.runner.historical_last_seen_at ? formatDateTime(current.runner.historical_last_seen_at, observationDateOptions) : t('unknown')}</dd></div>
          <div><dt>{t('inventory')}</dt><dd>{current.scope.source_current && remaining <= 0 ? t('expired') : t('source.' + current.scope.source_reason)}
            {current.scope.source_current && remaining > 0 && typeof current.scope.inventory_item_count === 'number' && <span>{t('count', { count: formatNumber(current.scope.inventory_item_count) })}</span>}</dd></div></dl>
        <p className="pool-config-muted">{t('qualification_boundary')}</p>
        {href ? <a className="button" data-pool-connection-tools href={href}>{t('open_tools')}</a> : <p>{t('project_restricted')}</p>}
      </div>}
    </div>
  </details>;
}
