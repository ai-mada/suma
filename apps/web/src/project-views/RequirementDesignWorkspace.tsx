import { useCallback, useEffect, useRef, useState } from 'react';
import { Plus, ChevronDown, GitBranch } from 'lucide-react';
import { query, time, stateLabel, type Session, type RecordItem, type ApiCaller } from '../api';
import { useApi } from '../api-context';
import { tr, useLocale, formatNumber, UiError } from '../i18n';
import { Field, Modal, Loading, ErrorNotice, Empty } from '../ui';
import { WriteNotice, useWriteAction, useLeaveGuard } from '../write-action';
import { toolSessionStorage } from '../tool-hooks';
import { sourceHref } from '../navigation';
import { withCurrentOption } from '../record-options';
import { designPath, designDraft, designSections, designSaveBody, designDiagnostics, designDiagnostic, newDesignTask, taskCriteria, type RequirementDesign, type DesignDraft, type DesignRevision, type DesignAdoption, type DesignPreview, type DesignPage, type DesignTask, type DesignRequirement } from '../requirement-design-model';
import '../requirement-design.css';
type Props = {
    project: string;
    requirementId: string;
    session: Session;
    onClose: () => void;
    onRefresh: () => void;
};
type LocalPage = 'product' | 'experience' | 'technical' | 'tasks';
const text = (key: string, values?: Record<string, unknown>) => tr('requirement-design.' + key, values);
/** One focused workspace in the requirement route. No action here starts a run.
 * Recovery keeps an immutable request in the original user/org/project/source. */
export function RequirementDesignWorkspace({ project, requirementId, session, onClose, onRefresh }: Props) {
    useLocale();
    const api = useApi(), path = designPath(requirementId), scope = session.user.id + ':' + session.organization.id + ':' + project + ':' + requirementId;
    const action = useWriteAction({ scope, storage: toolSessionStorage(), validateRequest: request => request.body.project_id === project && ((request.path === path && request.method === 'PUT') || (request.path === path + '/commands' || request.path === path + '/adopt') && request.method === 'POST') });
    const [loaded, setLoaded] = useState<{
        caller: ApiCaller;
        value: RequirementDesign;
    }>(), [loading, setLoading] = useState(true), [needsRead, setNeedsRead] = useState(true), [readError, setReadError] = useState<unknown>(), [draft, setDraft] = useState<DesignDraft>(), [dirty, setDirty] = useState(false), [page, setPage] = useState<LocalPage>('product'), [view, setView] = useState<'working' | 'published'>('working'), [preview, setPreview] = useState<DesignPreview>(), [previewing, setPreviewing] = useState(false), [review, setReview] = useState<'publish' | 'retire'>(), [confirmed, setConfirmed] = useState(false), [closing, setClosing] = useState(false), [rebased, setRebased] = useState(false);
    const generation = useRef(0), alive = useRef(true), draftRef = useRef(draft);
    draftRef.current = draft;
    useEffect(() => { alive.current = true; return () => { alive.current = false; generation.current++; }; }, []);
    const current = loaded?.caller === api ? loaded.value : undefined, blocked = action.busy || !!action.pending, editorDisabled = blocked || view === 'published' || !!review;
    useLeaveGuard(dirty, action.setError);
    const load = useCallback(async (preserve = false, ignorePending = false) => {
        const ticket = ++generation.current;
        setLoading(true);
        setNeedsRead(true);
        setReadError(undefined);
        try {
            const value = await api<RequirementDesign>(path + query(project));
            if (!alive.current || ticket !== generation.current)
                return;
            setLoaded({ caller: api, value });
            if (!value.published) setView('working');
            setNeedsRead(false);
            setPreview(undefined);
            setReview(undefined);
            setConfirmed(false);
            if (preserve && draftRef.current) {
                setDraft({ ...draftRef.current, expected_version: value.version });
                setRebased(false);
            }
            else {
                // A reloaded unknown save shows its submitted original, not a later server
                // draft. The hook alone controls replay; editing cannot replace its body.
                const pending = ignorePending ? undefined : action.pending;
                if (pending?.path === path && pending.method === 'PUT') {
                    const version = Number(pending.body.expected_requirement_version);
                    const fixed = [value.latest?.requirement, value.published?.requirement, value.requirement].find(source => source?.version === version);
                    // Never attach a newer source's text to the original source
                    // version. Without an exact snapshot, expose only that fact
                    // until the immutable original operation is recovered.
                    setDraft({ expected_version: Number(pending.body.expected_version), source_unknown: !fixed, requirement: structuredClone(fixed || {id: requirementId, version, title: '', description: '', acceptance_criteria: []}), content: structuredClone(pending.body.content) as DesignDraft['content'], tasks: structuredClone(pending.body.tasks) as DesignTask[] });
                }
                else
                    setDraft(designDraft(value));
                setDirty(false);
                setRebased(false);
            }
            return value;
        }
        catch (error) {
            if (alive.current && ticket === generation.current)
                setReadError(error);
        }
        finally {
            if (alive.current && ticket === generation.current)
                setLoading(false);
        }
    }, [api, path, project]);
    useEffect(() => { void load(); }, [load]);
    const update = (change: Partial<DesignDraft>) => { setDraft(value => value ? { ...value, ...change } : value); setDirty(true); setPreview(undefined); setConfirmed(false); };
    const updateTask = (key: string, change: Partial<DesignTask>) => update({ tasks: (draft?.tasks || []).map(task => task.key === key ? { ...task, ...change } : task) });
    async function write(target: string, body: Record<string, unknown>, method = 'POST') {
        try {
            const result = await action.run(target, body, method);
            if (result !== undefined) {
                await load(false, true);
                onRefresh();
            }
        }
        catch (error) {
            action.setError(error);
        }
    }
    async function save() { if (!draft)
        return; try {
        await write(path, designSaveBody(project, draft), 'PUT');
    }
    catch (error) {
        action.setError(error);
    } }
    async function recover() { const original = action.pending; if (original)
        await write(original.path, original.body, original.method); }
    function close() { if (blocked)
        return; if (dirty) {
        setClosing(true);
        return;
    } onClose(); }
    async function readPreview() {
        if (!current?.published || dirty || blocked)
            return;
        setPreviewing(true);
        setConfirmed(false);
        setReview(undefined);
        action.setError('');
        const ticket = generation.current, publication = current.published;
        try {
            const value = await api<DesignPreview>(path + '/preview', { project_id: project, expected_version: current.version, revision: publication.revision, sha256: publication.sha256 });
            if (alive.current && generation.current === ticket)
                setPreview(value);
        }
        catch (error) {
            action.setError(error);
            setPreview(undefined);
        }
        finally {
            if (alive.current)
                setPreviewing(false);
        }
    }
    const basisChanged = !!draft && !!current && draft.requirement.version !== current.requirement.version;
    const shown = view === 'published' ? current?.published : undefined, shownContent = shown?.content || draft?.content, shownTasks = shown?.tasks || draft?.tasks || [], basis = shown?.requirement || draft?.requirement, diagnostics = draft ? designDiagnostics(draft) : [], unavailable = loading || needsRead || !current || blocked || previewing;
    // Eligibility describes the existing publication. A newer saved draft can
    // already have the reviewed current source while that publication is stale.
    const canPublish = !!current?.latest && current.has_draft && current.latest.state === 'draft' && !dirty && !draft?.source_unknown && !current.validation.length && sameRequirement(current.latest.requirement, current.requirement);
    return <Modal open title={text('title')} description={text('intro')} onClose={close}><div className="requirement-design">
  {Boolean(readError) && <ErrorNotice error={readError} onRetry={() => void load(dirty)}/>}
  {loading && !current && <Loading />}
  <WriteNotice action={action} onRebase={async () => { const fresh = await load(true); if (!fresh)
        throw new UiError('requirement-design.errors.read'); }}/>
  {action.pending && <div className="rd-recovery"><p>{text('restore')}</p><button type="button" className="primary" disabled={action.busy} onClick={() => void recover()}>{text('restore')}</button></div>}
  {closing && <div className="warning rd-discard" role="alert"><strong>{text('discard_title')}</strong><p>{text('discard_hint')}</p><button onClick={() => setClosing(false)}>{text('continue_editing')}</button><button onClick={() => { setDirty(false); onClose(); }}>{text('discard')}</button></div>}
  {current && draft && <>
   <div className="rd-heading"><div><h3>{current.requirement.title}</h3><div className="rd-meta"><span>{text('record_version', { version: formatNumber(current.version) })}</span><span>{text('requirement_version', { version: formatNumber(current.requirement.version) })}</span>{dirty && <strong>{text('unsaved')}</strong>}</div></div><button type="button" disabled={unavailable || dirty} onClick={() => void load()}>{text('reload')}</button></div>
   <p className={current.eligibility === 'requirement_changed' ? 'warning' : 'muted'}>{text('eligible.' + current.eligibility)}</p>
   {current.has_draft && current.published && <small className="muted">{text('has_draft')}</small>}
   {!current.version && <Empty title={text('empty')} description={text('empty_hint')}/>}
   {rebased && <p className="warning" role="status">{text('new_basis')}</p>}
   {basisChanged && <section className="rd-review" aria-label={text('basis_changed')}><h3>{text('basis_changed')}</h3><div className="rd-change-comparison"><div><h4>{text('old_basis')} · v{draft.requirement.version}</h4><RequirementFacts value={draft.requirement} known={!draft.source_unknown}/></div><div><h4>{text('current_basis')} · v{current.requirement.version}</h4><RequirementFacts value={current.requirement}/></div></div><button disabled={blocked} onClick={() => { setDraft({ ...draft, expected_version: current.version, source_unknown: false, requirement: structuredClone(current.requirement) }); setDirty(true); setRebased(true); setPreview(undefined); setReview(undefined); }}>{text('use_current_basis')}</button></section>}
   <details className="rd-basis"><summary>{text('basis')} · {text('requirement_version', { version: formatNumber(basis?.version || 0) })}</summary>{basis && <RequirementFacts value={basis} known={!!shown || !draft.source_unknown}/>}</details>
   {current.published && <Field label={text('title')}><select aria-label={text('working')} value={view} disabled={blocked} onChange={event => { setView(event.target.value as typeof view); setReview(undefined); setConfirmed(false); }}><option value="working">{text('working')}</option><option value="published">{text('published', { revision: formatNumber(current.published.revision) })}</option></select></Field>}
   {shown && <p className="muted">{text('read_only')} <code>{shown.sha256}</code></p>}
   <div className="segmented rd-tabs" role="group" aria-label={text('title')}>{[...designSections, 'tasks'].map(section => <button key={section} type="button" aria-pressed={page === section} className={page === section ? 'active' : ''} onClick={() => setPage(section as LocalPage)}>{text(section)}</button>)}</div>
   <fieldset className="form-fields rd-editor" disabled={blocked || !!review}>
    {page !== 'tasks' && shownContent && <Field label={text(page)} hint={text(page + '_hint')}><textarea name={page} value={shownContent[page]} readOnly={editorDisabled} rows={12} onChange={event => update({ content: { ...draft.content, [page]: event.target.value } })}/></Field>}
    {page === 'tasks' && <><div className="rd-task-heading"><p className="muted">{text('version_basis', { version: formatNumber(basis?.version || 0) })}</p><button type="button" disabled={editorDisabled || shownTasks.length >= 100} onClick={() => update({ tasks: [...draft.tasks, newDesignTask()] })}><Plus size={15}/>{text('add_task')}</button></div>
     {!shownTasks.length ? <Empty title={text('tasks')} description={text('diagnostic.tasks_required')}/> : <div className="rd-table-scroll"><table className="rd-task-table"><thead><tr><th>{text('task_title')}</th><th>{text('task_acceptance')}</th><th>{text('dependencies')}</th></tr></thead><tbody>{shownTasks.map(task => <TaskProposalRow key={task.key} readOnly={editorDisabled} task={task} tasks={shownTasks} basis={basis!} project={project} onChange={patch => updateTask(task.key, patch)} onRemove={() => update({ tasks: draft.tasks.filter(value => value.key !== task.key) })}/>)}</tbody></table></div>}
    </>}
   </fieldset>
   {!shown && !action.pending && diagnostics.length > 0 && <details className="rd-validation"><summary>{text('validation')} · {formatNumber(diagnostics.length)}</summary><ul>{diagnostics.map((diagnostic, index) => <li key={index}>{designDiagnostic(diagnostic)}</li>)}</ul></details>}
   <div className="rd-actions"><button className="primary" disabled={unavailable || action.conflict || basisChanged || (!dirty && current.version > 0) || view === 'published'} onClick={() => void save()}>{text('save')}</button><button disabled={unavailable || !canPublish} onClick={() => { setReview('publish'); setConfirmed(false); setPreview(undefined); }}>{text('publish')}</button><button disabled={unavailable || dirty || !current.published || current.eligibility !== 'current'} onClick={() => void readPreview()}><GitBranch size={15}/>{text('preview')}</button>{current.published && current.eligibility !== 'retired' && <button disabled={unavailable || dirty} onClick={() => { setReview('retire'); setConfirmed(false); setPreview(undefined); }}>{text('retire')}</button>}</div>
   {review && current.latest && <section className="rd-review" aria-label={text(review)}><h3>{text(review)}</h3><p>{text(review + '_hint')}</p><RevisionFacts value={current.latest}/><label className="check"><input type="checkbox" checked={confirmed} disabled={blocked} onChange={event => setConfirmed(event.target.checked)}/>{text('review_confirm')}</label><div className="rd-actions"><button disabled={blocked} onClick={() => setReview(undefined)}>{text('cancel')}</button><button className="primary" disabled={unavailable || !confirmed || action.conflict} onClick={() => void write(path + '/commands', { project_id: project, action: review, expected_version: current.version, expected_requirement_version: current.requirement.version })}>{text('confirm_' + review)}</button></div></section>}
   {preview && <section className="rd-review" aria-label={text('preview')}><h3>{text('preview')}</h3><p>{text('preview_hint')}</p><p className="rd-meta">{text('published', { revision: formatNumber(preview.revision) })} · {text('requirement_version', { version: formatNumber(preview.requirement_version) })}</p><div className="rd-change-counts">{(['create', 'update', 'keep', 'exclude'] as const).map(kind => <span key={kind}>{text('change.' + kind)} <strong>{formatNumber(preview[(kind + '_count') as 'create_count'])}</strong></span>)}</div>
    <ul className="rd-changes">{preview.changes.map(change => <li key={change.key}><details open={change.action === 'update' || change.action === 'exclude'}><summary><strong>{text('change.' + change.action)}</strong> · {change.proposed?.title || change.current?.title || change.key} <code>{change.key}</code></summary><div className="rd-change-comparison"><div><h4>{text('current_task')}{change.current && ' · v' + change.expected_task_version}</h4>{change.current ? <TaskText value={change.current}/> : <p className="muted">{text('no_current')}</p>}</div><div><h4>{text('proposed_task')}</h4>{change.proposed ? <TaskText value={change.proposed}/> : <p>{text('excluded')}</p>}</div></div></details></li>)}</ul>
    <small>{text('plan_hash')} <code>{preview.plan_sha256}</code></small>{preview.has_changes ? <><label className="check"><input type="checkbox" checked={confirmed} disabled={blocked} onChange={event => setConfirmed(event.target.checked)}/>{text('adopt_confirm')}</label><button className="primary" disabled={unavailable || !confirmed || action.conflict} onClick={() => void write(path + '/adopt', { project_id: project, expected_version: preview.version, revision: preview.revision, sha256: preview.sha256, plan_sha256: preview.plan_sha256 })}>{text('adopt')}</button></> : <p>{text('no_changes')}</p>}
   </section>}
   {current.last_adoption && <section className="rd-adoption"><h3>{text('adopted')}</h3><AdoptionFacts value={current.last_adoption} project={project}/></section>}
   <DesignHistory project={project} id={requirementId} refresh={current.version}/>
   <small className="muted">{text('manual')}</small>
  </>}
 </div></Modal>;
}
function sameRequirement(left: DesignRequirement, right: DesignRequirement) {
    return left.id === right.id && left.version === right.version && left.title === right.title && left.description === right.description && JSON.stringify(left.acceptance_criteria) === JSON.stringify(right.acceptance_criteria);
}
function RequirementFacts({ value, known = true }: {
    value: DesignRequirement;
    known?: boolean;
}) { if (!known) return <p className="muted">{text('pending_basis_unknown')}</p>; return <><h4>{value.title}</h4><p className="preserve">{value.description}</p>{value.acceptance_criteria.length ? <ol>{value.acceptance_criteria.map((criterion, index) => <li key={index}>{criterion}</li>)}</ol> : <p className="muted">{text('criteria_empty')}</p>}</>; }
function TaskText({ value }: {
    value: DesignTask | RecordItem;
}) { return <div className="rd-task-text"><strong>{value.title}</strong>{'status' in value && typeof value.status === 'string' && <small>{tr('records.status')}: {stateLabel(value.status)}</small>}<p className="preserve">{value.description}</p><ul>{taskCriteria(value.acceptance_criteria).map((criterion, index) => <li key={index}>{criterion}</li>)}</ul><small>{text('dependencies')}: {(value.depends_on || []).join(', ') || '—'}</small><small>{text('owner')}: {value.owner_id || text('unassigned')} · {text('priority')}: {tr('common.priority.'+(value.priority||'medium'))}</small>{'covers' in value && <small>{text('coverage')}: {(value.covers as number[]).join(', ') || '—'}</small>}</div>; }
function RevisionFacts({ value }: {
    value: DesignRevision;
}) { return <div className="rd-revision-facts"><p>{text('revision', { revision: formatNumber(value.revision) })} · {text('state.' + value.state)} · {time(value.created_at)}</p><code>{value.sha256}</code><details><summary>{text('basis')}</summary><RequirementFacts value={value.requirement}/></details>{designSections.map(section => <details key={section}><summary>{text(section)}</summary><pre>{value.content[section]}</pre></details>)}<details><summary>{text('tasks')} · {formatNumber(value.tasks.length)}</summary><ul>{value.tasks.map(task => <li key={task.key}><code>{task.key}</code><TaskText value={task}/></li>)}</ul></details></div>; }
function AdoptionFacts({ value, project }: {
    value: DesignAdoption;
    project: string;
}) { return <><small>{text('revision', { revision: formatNumber(value.revision) })} · {time(value.created_at)} · <code>{value.id}</code></small><ul>{value.tasks.map(task => <li key={task.key}><span>{text('change.' + task.action)} · {task.key} · v{task.version} </span><a href={sourceHref(project, { kind: 'tasks', id: task.task_id })}>{text('open_task')}</a></li>)}</ul></>; }
function TaskProposalRow({ task, tasks, basis, project, onChange, onRemove, readOnly }: {
    readOnly: boolean;
    task: DesignTask;
    tasks: DesignTask[];
    basis: DesignRequirement;
    project: string;
    onChange: (patch: Partial<DesignTask>) => void;
    onRemove: () => void;
}) {
    const [expanded, setExpanded] = useState(false);
    return <><tr><td><input aria-label={text('task_title') + ' ' + task.key} value={task.title} readOnly={readOnly} onChange={event => onChange({ title: event.target.value })}/><code>{task.key}</code></td><td><textarea aria-label={text('task_acceptance') + ' ' + task.key} rows={2} value={task.acceptance_criteria.join('\n')} readOnly={readOnly} onChange={event => onChange({ acceptance_criteria: event.target.value.split('\n') })}/></td><td><button type="button" aria-expanded={expanded} onClick={() => setExpanded(value => !value)}><ChevronDown size={14}/>{formatNumber(task.depends_on.length)} · {text('advanced')}</button></td></tr>{expanded && <tr className="rd-task-details"><td colSpan={3}>{readOnly ? <TaskText value={task}/> : <><div className="rd-task-details-grid"><div><Field label={text('task_description')}><textarea rows={3} value={task.description} onChange={event => onChange({ description: event.target.value })}/></Field><ResourceFields project={project} task={task} onChange={onChange}/></div><div><h4>{text('dependencies')}</h4>{tasks.filter(value => value.key !== task.key).map(value => <label className="check" key={value.key}><input type="checkbox" checked={task.depends_on.includes(value.key)} onChange={event => onChange({ depends_on: event.target.checked ? [...task.depends_on, value.key] : task.depends_on.filter(key => key !== value.key) })}/>{value.title || value.key}</label>)}{!tasks.some(value => value.key !== task.key) && <small className="muted">{text('no_dependencies')}</small>}{task.depends_on.filter(key => key === task.key || !tasks.some(value => value.key === key)).map(key => <label className="check warning" key={key}><input type="checkbox" checked onChange={() => onChange({ depends_on: task.depends_on.filter(value => value !== key) })}/>{key === task.key ? text('diagnostic.dependency_cycle') + ' · ' + key : text('orphan_dependency', { key })}</label>)}<h4>{text('coverage')}</h4>{basis.acceptance_criteria.map((criterion, index) => <label className="check" key={index}><input type="checkbox" checked={task.covers.includes(index + 1)} onChange={event => onChange({ covers: event.target.checked ? [...task.covers, index + 1].sort((a, b) => a - b) : task.covers.filter(value => value !== index + 1) })}/><span>{index + 1}. {criterion}</span></label>)}{task.covers.filter(index => index > basis.acceptance_criteria.length || index < 1).map(index => <label className="check warning" key={index}><input type="checkbox" checked onChange={() => onChange({ covers: task.covers.filter(value => value !== index) })}/>{text('diagnostic.criterion_invalid', { key: task.key, index })}</label>)}</div></div><button type="button" className="danger-quiet" onClick={onRemove}>{text('remove_task')}</button></>}</td></tr>}</>;
}
/** Resource options are lazy and paged, never assumed to be the entire project. */
function ResourceFields({ project, task, onChange }: {
    project: string;
    task: DesignTask;
    onChange: (patch: Partial<DesignTask>) => void;
}) {
    const api = useApi(), [rows, setRows] = useState<RecordItem[]>([]), [cursor, setCursor] = useState(''), [busy, setBusy] = useState(false), [error, setError] = useState<unknown>();
    const load = useCallback(async (more = false) => { setBusy(true); setError(undefined); try {
        const value = await api<DesignPage<RecordItem>>('/members' + query(project) + (more && cursor ? '&cursor=' + encodeURIComponent(cursor) : ''));
        setRows(old => [...(more ? old : []), ...value.items]);
        setCursor(value.next_cursor);
    }
    catch (reason) {
        setError(reason);
    }
    finally {
        setBusy(false);
    } }, [api, project, cursor]);
    useEffect(() => { void load(); }, [api, project]);
    return <div className="form-grid"><Field label={text('priority')}><select value={task.priority} onChange={event => onChange({ priority: event.target.value })}>{['urgent', 'high', 'medium', 'low'].map(priority => <option key={priority} value={priority}>{tr('common.priority.' + priority)}</option>)}</select></Field><Field label={text('owner')} hint={text('owner_hint')}><select value={task.owner_id} onChange={event => onChange({ owner_id: event.target.value })}><option value="">{text('unassigned')}</option>{withCurrentOption(rows, task.owner_id, text('owner')).map(member => <option key={member.id} value={member.id}>{member.name || member.title || member.id}</option>)}</select>{cursor && <button type="button" disabled={busy} onClick={() => void load(true)}>{text('more_members')}</button>}{Boolean(error) && <ErrorNotice error={error} onRetry={() => void load()}/>}</Field></div>;
}
function DesignHistory({ project, id, refresh }: {
    project: string;
    id: string;
    refresh: number;
}) { return <div className="rd-histories"><HistoryPage<DesignRevision> project={project} id={id} kind="revisions" refresh={refresh} render={value => <RevisionFacts value={value}/>} label={value => text('revision', { revision: formatNumber(value.revision) }) + ' · ' + text('state.' + value.state) + ' · ' + time(value.created_at)}/><HistoryPage<DesignAdoption> project={project} id={id} kind="adoptions" refresh={refresh} render={value => <AdoptionFacts value={value} project={project}/>} label={value => text('revision', { revision: formatNumber(value.revision) }) + ' · ' + time(value.created_at)}/></div>; }
function HistoryPage<T extends {
    revision: number;
}>({ project, id, kind, refresh, render, label }: {
    project: string;
    id: string;
    kind: 'revisions' | 'adoptions';
    refresh: number;
    render: (value: T) => React.ReactNode;
    label: (value: T) => string;
}) {
    const api = useApi(), [open, setOpen] = useState(false), [items, setItems] = useState<T[]>([]), [total, setTotal] = useState(0), [cursor, setCursor] = useState(''), [busy, setBusy] = useState(false), [error, setError] = useState<unknown>(), epoch = useRef(0);
    const load = useCallback(async (more = false) => { const ticket = ++epoch.current; setBusy(true); setError(undefined); try {
        const value = await api<DesignPage<T>>(designPath(id) + '/' + kind + query(project) + '&limit=20' + (more && cursor ? '&cursor=' + encodeURIComponent(cursor) : ''));
        if (epoch.current !== ticket)
            return;
        setItems(old => [...(more ? old : []), ...value.items]);
        setTotal(value.total);
        setCursor(value.next_cursor);
    }
    catch (reason) {
        if (epoch.current === ticket)
            setError(reason);
    }
    finally {
        if (epoch.current === ticket)
            setBusy(false);
    } }, [api, project, id, kind, cursor]);
    useEffect(() => { if (open)
        void load(); return () => { epoch.current++; }; }, [open, refresh, api, project, id]);
    return <details onToggle={event => setOpen(event.currentTarget.open)}><summary>{text(kind === 'revisions' ? 'history' : 'adoptions')}</summary>{open && <><small>{text('loaded', { loaded: formatNumber(items.length), total: formatNumber(total) })}</small>{Boolean(error) && <ErrorNotice error={error} onRetry={() => void load()}/>}<ul>{items.map((value, index) => <li key={index}><details><summary>{label(value)}</summary>{render(value)}</details></li>)}</ul>{busy && <Loading />}{cursor && <button disabled={busy} onClick={() => void load(true)}>{text('more')}</button>}</>}</details>;
}
