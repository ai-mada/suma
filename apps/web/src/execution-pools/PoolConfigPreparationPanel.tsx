import { useEffect, useLayoutEffect, useRef, useState } from 'react';
import { Check, ChevronRight, Copy, Plus, Trash2 } from 'lucide-react';
import type { ExecutionPoolRunner, ExecutionPoolGrant } from '../../../../packages/contracts/src';
import { useApi, useApiView } from '../api-context';
import { formatNumber, tr, useLocale } from '../i18n';
import { emptyPoolConfigScope, poolConfigPreparationCommands, poolConfigPreparationIssue, validPoolConfigID, type PoolConfigPreparationInput, type PoolConfigPreparationScope, type PoolConfigPreparationIssue } from './pool-config-preparation-model';
import './PoolConfigPreparation.css';
export type PoolConfigPreparationGrant=Pick<ExecutionPoolGrant, 'id'|'pool_id'|'owner_tenant_id'|'tenant_id'|'project_id'|'status'|'shared_project_name'>;
export type PoolConfigPreparationPanelProps={
  ownerTenantID: string;
  poolID: string;
  runner: Pick<ExecutionPoolRunner, 'id'|'pool_id'|'worker_id'|'max_slots'|'status'>;
  grants?: PoolConfigPreparationGrant[];
  disabled?: boolean;
};
type Row={
  key: string;
  value: PoolConfigPreparationScope;
  grantID: string;
};
const t=(key: string, values?: Record<string, unknown>) => tr('pool-config-preparation.'+key, values);
function eligibleGrants(grants: PoolConfigPreparationGrant[]|undefined, ownerTenantID: string, poolID: string) {
  const current=Array.isArray(grants)? grants:[], counts=new Map<string, number>();
  for(const grant of current)
    if(grant&&typeof grant.id==='string')
      counts.set(grant.id, (counts.get(grant.id)||0)+1);
  return current.filter(grant => grant&&grant.pool_id===poolID&&grant.owner_tenant_id===ownerTenantID&&grant.status==='approved'&&validPoolConfigID(grant.id)&&validPoolConfigID(grant.tenant_id)&&validPoolConfigID(grant.project_id)&&typeof grant.shared_project_name==='string'&&counts.get(grant.id)===1);
}
/** No server, browser storage or consuming-project read belongs in this form.
 * Its scope is the current authorized Pool and registered machine only. */
export function PoolConfigPreparationPanel(props: PoolConfigPreparationPanelProps) {
  useLocale();
  const api=useApi(), view=useApiView(), key=JSON.stringify([view?.userID, view?.organizationID, props.ownerTenantID, props.poolID, props.runner.id, props.runner.pool_id, props.runner.worker_id, props.runner.status, props.runner.max_slots]), owner=useRef({ api, key, epoch: 0 });
  if(owner.current.api!==api||owner.current.key!==key)
    owner.current={ api, key, epoch: owner.current.epoch+1 };
  if(!view||view.organizationID!==props.ownerTenantID||!validPoolConfigID(props.poolID)||!validPoolConfigID(props.runner.id)||props.runner.pool_id!==props.poolID||props.runner.status!=='active'||props.runner.worker_id!=='pool-'+props.runner.id||!Number.isInteger(props.runner.max_slots)||props.runner.max_slots<1||props.runner.max_slots>128)
    return <p role="status" data-pool-config-unavailable>{t('unavailable')}</p>;
  const epoch=owner.current.epoch, owns=() => owner.current.api===api&&owner.current.key===key&&owner.current.epoch===epoch;
  return <PreparationScope key={key+':'+epoch} {...props} owns={owns} />;
}
function PreparationScope({ ownerTenantID, poolID, runner, grants, disabled=false, owns }: PoolConfigPreparationPanelProps&{
  owns: () => boolean;
}) {
  const panel=useRef<HTMLDivElement>(null), sequence=useRef(1), [shell, setShell]=useState<PoolConfigPreparationInput['shell']>(''), [output, setOutput]=useState(''), [slots, setSlots]=useState('1'), [rows, setRows]=useState<Row[]>([{ key: 'scope-1', value: emptyPoolConfigScope(), grantID: '' }]), [preview, setPreview]=useState<ReturnType<typeof poolConfigPreparationCommands>>(), [previewBasis, setPreviewBasis]=useState(''), [message, setMessage]=useState(''), [copied, setCopied]=useState(false), [copying, setCopying]=useState(false), focusIntent=useRef<string|undefined>(undefined);
  const available=eligibleGrants(grants, ownerTenantID, poolID);
  const grantBasis=JSON.stringify([disabled, rows.map(row => [row.grantID, row.grantID? available.some(grant => grant.id===row.grantID&&grant.tenant_id===row.value.tenant_id&&grant.project_id===row.value.project_id):true])]), sourceOwner=useRef({ basis: grantBasis, epoch: 0 });
  // Observed revocation/loading and A→B→A cannot revive an old clipboard ACK
  // or preview merely because its public identifiers happen to match again.
  if(sourceOwner.current.basis!==grantBasis)
    sourceOwner.current={ basis: grantBasis, epoch: sourceOwner.current.epoch+1 };
  const sourceBasis=grantBasis+':'+sourceOwner.current.epoch;
  const owner=useRef({ alive: true, revision: 0, inflight: false, disabled, sourceBasis });
  owner.current.disabled=disabled;
  owner.current.sourceBasis=sourceBasis;
  function edited() { ++owner.current.revision; setPreview(undefined); setMessage(''); setCopied(false); }
  useEffect(() => { owner.current.alive=true; return () => { owner.current.alive=false; ++owner.current.revision; }; }, []);
  useEffect(() => {
    if(disabled)
      edited();
  }, [disabled]);
  useLayoutEffect(() => {
    const target=focusIntent.current; if(!target||disabled||!owns())
      return; focusIntent.current=undefined; const element=target==='add'? panel.current?.querySelector<HTMLButtonElement>('[data-pool-config-add]'):panel.current?.querySelector<HTMLInputElement>('[data-pool-config-row="'+target+'"] [data-pool-config-scope-field]'); element?.focus({ preventScroll: true });
  }, [rows, disabled]);
  function updateRow(key: string, field: keyof PoolConfigPreparationScope, value: string) {
    if(disabled||!owns())
      return; edited(); setRows(before => before.map(row => row.key===key? { ...row, value: { ...row.value, [field]: value } }:row));
  }
  function add() {
    if(disabled||rows.length>=256||!owns())
      return; edited(); const key='scope-'+(++sequence.current); focusIntent.current=key; setRows(before => [...before, { key, value: emptyPoolConfigScope(), grantID: '' }]);
  }
  function remove(key: string) {
    if(disabled||rows.length<=1||!owns())
      return; const index=rows.findIndex(row => row.key===key); if(index<0)
      return; edited(); const next=rows.filter(row => row.key!==key); focusIntent.current=next[Math.min(index, next.length-1)]?.key||'add'; setRows(next);
  }
  function field(row: Row, name: keyof PoolConfigPreparationScope) {
    return <label>
      <span>{t('field.'+name)}</span>
      <input data-pool-config-scope-field={name} value={row.value[name]} disabled={disabled} readOnly={!!row.grantID&&['tenant_id', 'project_id'].includes(name)} maxLength={['tenant_id', 'project_id'].includes(name)? 100:name==='base_commit'? 64:4096} autoComplete="off" spellCheck={false} onChange={event => updateRow(row.key, name, event.target.value)} />
    </label>;
  }
  function selectProject(key: string, grantID: string) {
    if(disabled||!owns())
      return; const selected=available.find(grant => grant.id===grantID); if(grantID&&!selected)
      return; edited(); setRows(before => before.map(row => row.key===key? { ...row, grantID, value: selected? { ...row.value, tenant_id: selected.tenant_id, project_id: selected.project_id }:row.value }:row));
  }
  function focusIssue(issue: PoolConfigPreparationIssue) {
    if(disabled||!owns()||!issue.field)
      return;
    const row=issue.row? rows[issue.row-1]:undefined, selector=row? '[data-pool-config-row="'+row.key+'"] [data-pool-config-scope-field="'+issue.field+'"]':'[data-pool-config-field="'+issue.field+'"]', element=panel.current?.querySelector<HTMLElement>(selector);
    if(!element)
      return;
    for(let ancestor=element.parentElement; ancestor&&ancestor!==panel.current; ancestor=ancestor.parentElement)
      if(ancestor instanceof HTMLDetailsElement)
        ancestor.open=true;
    element.scrollIntoView?.({ block: 'nearest' });
    element.focus({ preventScroll: true });
  }
  async function copy() {
    if(disabled||!owns()||owner.current.inflight)
      return;
    if(rows.some(row => row.grantID&&!available.some(grant => grant.id===row.grantID&&grant.tenant_id===row.value.tenant_id&&grant.project_id===row.value.project_id))) {
      setPreview(undefined);
      setMessage(t('grant_unavailable'));
      return;
    }
    const input: PoolConfigPreparationInput={ shell, output, pool_id: poolID, runner_id: runner.id, slots: /^[0-9]+$/.test(slots)? Number(slots):0, scopes: rows.map(row => ({ ...row.value })) }, issue=poolConfigPreparationIssue(input, runner.max_slots);
    if(issue) {
      setMessage(t('issue.'+issue.code, { row: issue.row? formatNumber(issue.row):'' }));
      setPreview(undefined);
      focusIssue(issue);
      return;
    }
    const result=poolConfigPreparationCommands(input, runner.max_slots);
    if(!result)
      return;
    const revision=owner.current.revision, basis=sourceBasis;
    setPreview(result);
    setPreviewBasis(basis);
    setMessage('');
    setCopied(false);
    setCopying(true);
    owner.current.inflight=true;
    try {
      await navigator.clipboard.writeText(result.prepare);
      if(owner.current.alive&&owns()&&owner.current.revision===revision&&owner.current.sourceBasis===basis&&!owner.current.disabled)
        setCopied(true);
    }
    catch {
      if(owner.current.alive&&owns()&&owner.current.revision===revision&&owner.current.sourceBasis===basis)
        setMessage(t('copy_failed'));
    }
    finally {
      owner.current.inflight=false;
      if(owner.current.alive)
        setCopying(false);
    }
  }
  return <div className="pool-config-preparation" data-pool-config-preparation ref={panel}>
    <p>{t('intro')}</p>
    <p className="pool-config-muted">{t('target', { pool: poolID, worker: runner.worker_id })}</p>
    <div className="pool-config-fields">
      <label>
        <span>{t('field.shell')}</span>
        <select data-pool-config-field="shell" value={shell} disabled={disabled} onChange={event => { edited(); setShell(event.target.value as PoolConfigPreparationInput['shell']); }}>
          <option value="">{t('choose_shell')}</option>
          <option value="powershell">PowerShell</option>
          <option value="bash">Bash</option>
        </select>
      </label>
      <label>
        <span>{t('field.slots')}</span>
        <input data-pool-config-field="slots" type="number" min={1} max={runner.max_slots} value={slots} disabled={disabled} onChange={event => { edited(); setSlots(event.target.value); }} />
      </label>
      <label className="pool-config-full">
        <span>{t('field.output')}</span>
        <input data-pool-config-field="output" value={output} disabled={disabled} maxLength={4096} autoComplete="off" spellCheck={false} onChange={event => { edited(); setOutput(event.target.value); }} />
      </label>
    </div>
    <p className="pool-config-muted">{t('output_help')}</p>
    <p className="pool-config-muted">{t('scope_help')}</p>
    <div className="pool-config-scopes">{rows.map((row, index) => <section key={row.key} data-pool-config-row={row.key} className="pool-config-scope">
      <header>
        <h4>{t('scope_title', { index: formatNumber(index+1) })}</h4>
        <button type="button" className="text-button" data-pool-config-remove={row.key} aria-label={t('remove_scope', { index: formatNumber(index+1) })} disabled={disabled||rows.length<=1} onClick={() => remove(row.key)}>
          <Trash2 size={15} aria-hidden="true" />{t('remove')}</button>
      </header>
      <label>
        <span>{t('choose_project')}</span>
        <select data-pool-config-project={row.key} value={row.grantID} disabled={disabled} onChange={event => selectProject(row.key, event.target.value)}>
          <option value="">{t('manual_ids')}</option>{row.grantID&&!available.some(grant => grant.id===row.grantID)&&<option value={row.grantID} disabled>{t('grant_unavailable')}</option>}
          {available.map(grant => <option key={grant.id} value={grant.id}>{grant.shared_project_name||grant.project_id} · {grant.project_id}</option>)}</select>
      </label>
      <div className="pool-config-fields">{field(row, 'tenant_id')}
        {field(row, 'project_id')}
        {field(row, 'workspace')}
        {field(row, 'commands_file')}</div>
      <p className="pool-config-muted">{t('commands_help')}</p>
      <details>
        <summary>
          <ChevronRight size={16} aria-hidden="true" />{t('advanced')}</summary>
        <div className="pool-config-fields">{field(row, 'repository')}
          {field(row, 'base_commit')}
          {field(row, 'worktrees')}
          {field(row, 'model_deployments_file')}
          {field(row, 'external_config_file')}</div>
        <p className="pool-config-muted">{t('advanced_help')}</p>
      </details>
    </section>)}</div>
    <div className="pool-config-actions">
      <button type="button" data-pool-config-add disabled={disabled||rows.length>=256} onClick={add}>
        <Plus size={15} aria-hidden="true" />{t('add_scope')}</button>
      <span className="pool-config-muted">{t('scope_count', { count: formatNumber(rows.length) })}</span>
    </div>
    {!!message&&<p role="alert" data-pool-config-message>{message}</p>}
    <button type="button" className="primary" data-pool-config-copy disabled={disabled||copying} onClick={() => void copy()}>{copied&&previewBasis===sourceBasis? <Check size={16} aria-hidden="true" />:<Copy size={16} aria-hidden="true" />}
      {t(copying? 'copying':copied&&previewBasis===sourceBasis? 'copied':'copy')}</button>
    {preview&&!disabled&&previewBasis===sourceBasis&&<div className="pool-config-preview">
      <pre tabIndex={0} data-pool-config-prepare aria-label={t('command')}>{preview.prepare}</pre>
      <details>
        <summary>{t('review')}</summary>
        <pre tabIndex={0} data-pool-config-input aria-label={t('review')}>{JSON.stringify(preview.request, null, 2)}</pre>
      </details>
    </div>}
    <p className="pool-config-muted">{t('boundary')}</p>
  </div>;
}
