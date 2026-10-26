import { useEffect, useRef, useState, type FormEvent } from 'react';
import { BookOpen, ChevronRight, FileText, Plus, RefreshCw, ShieldCheck, X } from 'lucide-react';
import { query, time, type RecordItem, type Session } from '../api';
import { useApi } from '../api-context';
import { formatNumber, tr as t, UiError, useLocale } from '../i18n';
import { Empty, ErrorNotice, Field, Loading, Modal } from '../ui';
import { useLeaveGuard, useWriteAction, WriteNotice, type WriteAction } from '../write-action';
import { useToolList, useToolRead, toolSessionStorage } from '../tool-hooks';
import { useKnowledgeEntries } from '../knowledge-hooks';
import { knowledgeKinds, knowledgeSourceInputs, knowledgeSourceLabel, type KnowledgeDetail, type KnowledgeFilters, type KnowledgeKind, type KnowledgeRevision, type KnowledgeSourceInput, type KnowledgeSourceKind, type KnowledgeState } from '../knowledge-model';
import { CatalogAdoptionFacts, ModelAdoptionFacts, KnowledgeRevisionFacts } from '../KnowledgeFacts';
import {KnowledgeIntelligence} from '../KnowledgeIntelligence';
import {catalogHref} from '../skill-catalog-model';
import '../knowledge.css';
type Props = {
    intelligenceId?: string;
    project: string;
    session: Session;
    selected: string;
    filters: KnowledgeFilters;
    onFilter: (filters: Partial<KnowledgeFilters>) => void;
    onSelect: (id: string) => void;
    refresh: number;
    onRefresh: () => void;
};
type Editor = {
    mode: 'create' | 'extract' | 'edit';
    entry?: KnowledgeDetail;
};
type Decision = {
    action: 'publish' | 'retire';
    entry: KnowledgeDetail;
};
const sourceKinds: KnowledgeSourceKind[] = ['documents', 'requirements', 'tasks', 'issues'];
const objectPath = (id: string) => '/project-knowledge/' + encodeURIComponent(id);
function canLeave() { return window.dispatchEvent(new Event('suma:navigate', { cancelable: true })); }
export function KnowledgeWorkspace({ intelligenceId, project, session, selected, filters, onFilter, onSelect, refresh, onRefresh }: Props) {
    useLocale();
    const api = useApi(), [tick, setTick] = useState(0), [search, setSearch] = useState(filters.q), [editor, setEditor] = useState<Editor>(), [decision, setDecision] = useState<Decision>(), [confirmation, setConfirmation] = useState(false), [needsRead, setNeedsRead] = useState(false), [view, setView] = useState<'latest' | 'published'>('latest');
    const action = useWriteAction({ scope: session.user.id + ':' + session.organization.id + ':' + project, storage: toolSessionStorage(), validateRequest: request => request.body.project_id === project && ((request.path === '/project-knowledge' || request.path === '/project-knowledge/extractions') && request.method === 'POST' || /^\/project-knowledge\/[A-Za-z0-9_-]+$/.test(request.path) && request.method === 'PATCH' || /^\/project-knowledge\/[A-Za-z0-9_-]+\/commands$/.test(request.path) && request.method === 'POST') }), entries = useKnowledgeEntries(project, filters, refresh + tick), detail = useToolRead<KnowledgeDetail>(selected ? objectPath(selected) + query(project) : '', refresh + tick);
    const pending = useRef<{
        path: string;
        body: Record<string, unknown>;
        method?: string;
    } | undefined>(undefined);
    const current = detail.data, stale = needsRead || detail.loading || !!detail.error, blocked = action.busy || !!action.pending;
    useEffect(() => { setSearch(filters.q); }, [filters.q]);
    useEffect(() => { setView('latest'); setConfirmation(false); }, [selected]);
    useEffect(() => { if (detail.data)
        setNeedsRead(false); }, [detail.data]);
    const shown = current && (view === 'published' ? current.published || current.latest : current.latest);
    async function write(path: string, body: Record<string, unknown>, method?: string) {
        const request = action.pending ? { path: action.pending.path, body: action.pending.body, method: action.pending.method } : pending.current && action.unknown ? pending.current : { path, body, method };
        pending.current = request;
        const result = await action.run<KnowledgeDetail>(request.path, request.body, request.method);
        if (result) {
            // A receipt can be historical. Never enable another CAS command from its
            // version: read the official detail/list after every success or recovery.
            pending.current = undefined;
            setEditor(undefined);
            setDecision(undefined);
            setConfirmation(false);
            setNeedsRead(true);
            onSelect(result.id);
            setTick(value => value + 1);
            onRefresh();
        }
    }
    function changeSelection(id: string) { if (!blocked && canLeave())
        onSelect(id); }
    return <><KnowledgeIntelligence key={session.user.id+':'+session.organization.id+':'+project} session={session} project={project} refresh={refresh} onChanged={onRefresh} analysisId={intelligenceId}/><div className="knowledge-workspace" data-detail={!!selected}>
  {!editor && !decision && (action.error || action.pending) && <div className="knowledge-recovery"><WriteNotice action={action}/>{action.pending && <><p>{t('knowledge.recovery_title')}</p><p>{String(action.pending.body.title || (action.pending.body.action === 'publish' ? t('knowledge.publish') : action.pending.body.action === 'retire' ? t('knowledge.retire') : ''))}</p><details><summary>{t('knowledge.recovery_details')}</summary><code>{action.pending.path} · {action.pending.idempotency_key}</code>{typeof action.pending.body.content === 'string' && <pre className="knowledge-content">{action.pending.body.content}</pre>}</details><button className="primary" disabled={action.busy} onClick={() => void write(action.pending!.path, action.pending!.body, action.pending!.method)}>{t('knowledge.restore')}</button></>}</div>}
  <section className="panel knowledge-list" aria-label={t('knowledge.title')}>
   <div className="panel-heading"><div><h2>{t('knowledge.title')}</h2><p>{t('knowledge.description')}</p></div><BookOpen size={22}/></div>
   <form className="knowledge-toolbar" onSubmit={event => { event.preventDefault(); if (!blocked && canLeave())
        onFilter({ q: search }); }}>
    <Field label={t('knowledge.kind_label')}><select value={filters.kind} disabled={blocked} onChange={event => { if (canLeave())
        onFilter({ kind: event.target.value as KnowledgeFilters['kind'] }); }}><option value="all">{t('knowledge.kind.all')}</option>{knowledgeKinds.map(kind => <option key={kind} value={kind}>{t('knowledge.kind.' + kind)}</option>)}</select></Field>
    <Field label={t('knowledge.state_label')}><select value={filters.state} disabled={blocked} onChange={event => { if (canLeave())
        onFilter({ state: event.target.value as KnowledgeFilters['state'] }); }}>{['all', 'draft', 'published', 'retired'].map(state => <option key={state} value={state}>{t('knowledge.state.' + state)}</option>)}</select></Field>
    <div className="knowledge-filter-search"><input aria-label={t('knowledge.search')} placeholder={t('knowledge.search_placeholder')} value={search} onChange={event => setSearch(event.target.value)} disabled={blocked}/><button disabled={blocked}>{t('knowledge.apply_filters')}</button></div>
   </form>
   <div className="knowledge-toolbar knowledge-actions"><button className="primary" disabled={blocked} onClick={() => { action.clearConflict(); setEditor({ mode: 'create' }); }}><Plus size={15}/>{t('knowledge.create')}</button><button disabled={blocked} onClick={() => { action.clearConflict(); setEditor({ mode: 'extract' }); }}><FileText size={15}/>{t('knowledge.extract')}</button><button className="icon-button" aria-label={t('knowledge.reload')} disabled={blocked || entries.loading} onClick={() => void entries.read()}><RefreshCw size={16}/></button></div>
   {Boolean(entries.error) && <ErrorNotice error={entries.error} onRetry={() => void entries.read(entries.more)}/>}
   {entries.loading && !entries.items && <Loading />}
   <ul className="knowledge-list-items">{entries.items?.map(entry => <li key={entry.id}><button className="knowledge-entry" aria-current={selected === entry.id} disabled={blocked} onClick={() => changeSelection(entry.id)}><span><strong>{entry.title}</strong><span className="knowledge-meta"><span className="type-chip">{t('knowledge.kind.' + entry.kind)}</span><span>{t('knowledge.state.' + entry.state)}</span><span>{t('knowledge.revision', { revision: formatNumber(entry.latest_revision) })}</span>{entry.has_draft && <span>{t('knowledge.has_draft')}</span>}</span>{entry.summary && <p>{entry.summary}</p>}<small className={entry.eligibility === 'source_changed' ? 'text-danger' : 'muted'}>{t('knowledge.eligible.' + entry.eligibility)}</small></span><ChevronRight size={16}/></button></li>)}</ul>
   {entries.items?.length === 0 && !entries.loading && !entries.error && <Empty title={t('knowledge.empty')} description={t('knowledge.empty_hint')}/>}
   {entries.items && <div className="knowledge-pager"><small>{t('knowledge.loaded', { loaded: formatNumber(entries.items.length), total: formatNumber(entries.total) })}</small>{entries.cursor && <button disabled={entries.loading || blocked} onClick={() => void entries.read(true)}>{t('knowledge.more')}</button>}</div>}
  </section>
  <section className="panel knowledge-detail" aria-label={t('knowledge.details')}>
   {selected ? <>
    <div className="knowledge-detail-header"><button className="knowledge-back" disabled={blocked} onClick={() => changeSelection('')}>{t('knowledge.back')}</button><span className="eyebrow">{current?.id || selected}</span><button className="icon-button" aria-label={t('common.close')} disabled={blocked} onClick={() => changeSelection('')}><X size={17}/></button></div>
    {detail.error && <ErrorNotice error={detail.error} onRetry={() => void detail.read()}/>}
    {detail.loading && !current && <Loading />}
    {current && shown && <><h2>{shown.title}</h2><div className="knowledge-meta"><span className="type-chip">{t('knowledge.kind.' + current.kind)}</span><span>{t('knowledge.state.' + current.state)}</span><span>{t('knowledge.version', { version: formatNumber(current.version) })}</span><span>{time(current.updated_at)}</span></div>
     {(needsRead || detail.loading) && <p className="muted" role="status">{t('knowledge.refreshing_current')}</p>}
     <p className={current.eligibility === 'source_changed' ? 'warning' : 'muted'}>{t('knowledge.eligible.' + current.eligibility)}</p>
     {current.eligibility === 'source_changed' && <p>{t('knowledge.changed_hint')}</p>}
     {current.published && <Field label={t('knowledge.revision_view')}><select value={view} onChange={event => setView(event.target.value as typeof view)}><option value="latest">{t('knowledge.view_latest')} · {current.latest.revision}</option><option value="published">{t('knowledge.view_published')} · {current.published.revision}</option></select></Field>}
     {current.kind==='skill'&&current.published_eligible&&current.eligibility==='current'&&<a className="sc-link" href={catalogHref(project,current.id)} onClick={event=>{if(blocked||!canLeave())event.preventDefault();}}>{t('skill-catalog.share_link')}</a>}
     <KnowledgeRevisionFacts project={project} revision={shown}/><CatalogAdoptionFacts project={project} value={current.catalog_adoption}/><ModelAdoptionFacts project={project} value={current.model_adoption}/><KnowledgeHistory key={current.id + ':' + current.version} project={project} id={current.id}/>
     <div className="knowledge-actions"><button disabled={blocked || stale || current.state === 'retired'} onClick={() => { action.clearConflict(); setEditor({ mode: 'edit', entry: current }); }}>{t('knowledge.edit')}</button><button className="primary" disabled={blocked || stale || current.state === 'retired' || !current.has_draft} onClick={() => { action.clearConflict(); setDecision({ action: 'publish', entry: current }); setConfirmation(false); }}><ShieldCheck size={15}/>{t('knowledge.publish')}</button><button disabled={blocked || stale || current.state === 'retired'} onClick={() => { action.clearConflict(); setDecision({ action: 'retire', entry: current }); setConfirmation(false); }}>{t('knowledge.retire')}</button></div><small className="muted">{t('knowledge.approval_hint')}</small>
    </>}
   </> : <Empty title={t('knowledge.no_selection')} description={t('knowledge.select_hint')}/>}
  </section>
  {editor && <KnowledgeEditor key={editor.mode + ':' + (editor.entry?.id || 'new')} project={project} editor={editor} action={action} onClose={() => { if (!blocked)
        setEditor(undefined); }} onSave={write}/>}
  {decision && <Modal open title={t(decision.action === 'publish' ? 'knowledge.publish_title' : 'knowledge.retire_title')} description={t(decision.action === 'publish' ? 'knowledge.publish_hint' : 'knowledge.retire_hint')} onClose={() => { if (!blocked)
            setDecision(undefined); }}>
   <h3>{decision.entry.title}</h3><KnowledgeRevisionFacts project={project} revision={decision.entry.latest}/><WriteNotice action={action} onRebase={async () => { const latest = await api<KnowledgeDetail>(objectPath(decision.entry.id) + query(project)); setDecision({ ...decision, entry: latest }); setConfirmation(false); }}/>
   <label className="check"><input type="checkbox" checked={confirmation} disabled={blocked} onChange={event => setConfirmation(event.target.checked)}/>{t('knowledge.review_confirm')}</label>
   <div className="dialog-actions"><button disabled={blocked} onClick={() => setDecision(undefined)}>{t('knowledge.cancel')}</button><button className="primary" disabled={action.busy || (!action.pending && !confirmation) || action.conflict} onClick={() => void write(objectPath(decision.entry.id) + '/commands', { project_id: project, action: decision.action, expected_version: decision.entry.version })}>{t(action.pending ? 'knowledge.restore' : decision.action === 'publish' ? 'knowledge.publish' : 'knowledge.retire')}</button></div>
  </Modal>}
 </div></>;
}
function KnowledgeEditor({ project, editor, action, onClose, onSave }: {
    project: string;
    editor: Editor;
    action: WriteAction;
    onClose: () => void;
    onSave: (path: string, body: Record<string, unknown>, method?: string) => Promise<void>;
}) {
    const api = useApi(), entry = editor.entry, [kind, setKind] = useState<KnowledgeKind>(entry?.kind || 'memory'), [title, setTitle] = useState(entry?.latest.title || ''), [summary, setSummary] = useState(entry?.latest.summary || ''), [content, setContent] = useState(entry?.latest.content || ''), [baseVersion, setBaseVersion] = useState(entry?.version || 0), [dirty, setDirty] = useState(false), [discard, setDiscard] = useState(false), [guardError, setGuardError] = useState<unknown>(''), [sourceKind, setSourceKind] = useState<KnowledgeSourceKind>('documents'), [sourceID, setSourceID] = useState(''), [quote, setQuote] = useState(''), [serverContent, setServerContent] = useState<string>(), [fixedSources, setFixedSources] = useState<KnowledgeSourceInput[]>(entry ? knowledgeSourceInputs(entry.latest.sources) : []), [currentSources, setCurrentSources] = useState<Record<string, RecordItem>>({}), [sourceBusy, setSourceBusy] = useState('');
    const extracting = editor.mode === 'extract', sources = useToolList<RecordItem>(project, sourceKind), blocked = action.busy || !!action.pending;
    const selected = sources.items?.find(source => source.id === sourceID);
    useLeaveGuard(dirty, setGuardError);
    function close() { if (blocked)
        return; if (dirty) {
        setDiscard(true);
        return;
    } onClose(); }
    async function submit(event: FormEvent) {
        event.preventDefault();
        setGuardError('');
        if (action.pending) {
            await onSave(action.pending.path, action.pending.body, action.pending.method);
            return;
        }
        let fixed: KnowledgeSourceInput[] = fixedSources;
        if (extracting) {
            if (!selected) {
                setGuardError(new UiError('knowledge.source_required'));
                return;
            }
            fixed = [{ kind: sourceKind, id: selected.id, version: selected.version, ...(quote ? { quote } : {}) }];
        }
        const body = { project_id: project, kind, title, summary, sources: fixed, ...(!extracting ? { content, expected_version: baseVersion } : {}) };
        await onSave(extracting ? '/project-knowledge/extractions' : entry ? objectPath(entry.id) : '/project-knowledge', body, entry ? 'PATCH' : undefined);
    }
    return <Modal open title={t(extracting ? 'knowledge.extraction_title' : entry ? 'knowledge.edit_title' : 'knowledge.create_title')} description={t(extracting ? 'knowledge.extraction_hint' : 'knowledge.manual_hint')} onClose={close}>
  <form className="knowledge-editor" onSubmit={event => void submit(event)}>
   <WriteNotice action={action} onRebase={entry ? async () => { const current = await api<KnowledgeDetail>(objectPath(entry.id) + query(project)); setBaseVersion(current.version); setServerContent(current.latest.content); } : undefined}/>
   {Boolean(guardError) && <ErrorNotice error={guardError}/>}
   <fieldset className="form-fields" disabled={blocked} onChange={() => setDirty(true)}>
    <Field label={t('knowledge.kind_label')}><select value={kind} disabled={!!entry} onChange={event => setKind(event.target.value as KnowledgeKind)}>{knowledgeKinds.map(value => <option key={value} value={value}>{t('knowledge.kind.' + value)}</option>)}</select></Field>
    <Field label={t('knowledge.title_label')} hint={extracting ? t('knowledge.optional_title') : undefined}><input name="title" required={!extracting} maxLength={300} value={title} onChange={event => setTitle(event.target.value)}/></Field>
    <Field label={t('knowledge.summary_label')}><textarea name="summary" rows={2} maxLength={2000} value={summary} onChange={event => setSummary(event.target.value)}/></Field>
    {extracting ? <><Field label={t('knowledge.source_kind')}><select value={sourceKind} onChange={event => { setSourceKind(event.target.value as KnowledgeSourceKind); setSourceID(''); setQuote(''); }}>{sourceKinds.map(value => <option key={value} value={value}>{knowledgeSourceLabel(value)}</option>)}</select></Field>
     {sources.error && <ErrorNotice error={sources.failure || sources.error} onRetry={() => void sources.read(sources.error_action === 'more')}/>}
     <Field label={t('knowledge.source_record')}><select name="source" required value={sourceID} onChange={event => setSourceID(event.target.value)}><option value="">{t('knowledge.source_choose')}</option>{sources.items?.map(source => <option value={source.id} key={source.id}>{source.title || source.id} · v{source.version}</option>)}</select></Field>
     {sources.next_cursor && <button type="button" disabled={sources.loading} onClick={() => void sources.read(true)}>{t('knowledge.more')}</button>}
     {selected && <small>{selected.id} · {t('knowledge.source_version', { version: formatNumber(selected.version) })}</small>}
     <Field label={t('knowledge.source_quote')} hint={t('knowledge.source_quote_hint')}><textarea name="quote" rows={5} value={quote} onChange={event => setQuote(event.target.value)}/></Field>
    </> : <><Field label={t('knowledge.content_label')}><textarea name="content" required rows={12} value={content} onChange={event => setContent(event.target.value)}/></Field>{entry && <details><summary>{t('knowledge.source_list')}</summary>{fixedSources.map((source, index) => { const key = source.kind + ':' + source.id, current = currentSources[key]; return <div className="knowledge-source" key={key}><strong>{entry.latest.sources.find(item => item.id === source.id && item.kind === source.kind)?.title || source.id}</strong><p>{t('knowledge.source_version', { version: formatNumber(source.version) })}</p><button type="button" disabled={blocked || !!sourceBusy} onClick={async () => { setSourceBusy(key); setGuardError(''); try {
            const value = await api<RecordItem>('/' + source.kind + '/' + encodeURIComponent(source.id) + query(project));
            setCurrentSources(before => ({ ...before, [key]: value }));
        }
        catch (error) {
            setGuardError(error);
        }
        finally {
            setSourceBusy('');
        } }}>{t('knowledge.source_read_current')}</button>{current && <><p>{t('knowledge.source_current_version', { version: formatNumber(current.version) })} · {current.title || current.id}</p><pre className="knowledge-content">{[current.description, current.content, current.acceptance_criteria].filter(value => typeof value === 'string').join('\n\n')}</pre><p className="muted">{t('knowledge.source_adopt_hint')}</p><button type="button" disabled={blocked} onClick={() => { setFixedSources(before => before.map((item, at) => at === index ? { kind: item.kind, id: item.id, version: current.version } : item)); setDirty(true); }}>{t('knowledge.source_adopt', { version: formatNumber(current.version) })}</button></>}</div>; })}</details>}</>}
   </fieldset>
   {serverContent !== undefined && <details open><summary>{t('knowledge.current_content')}</summary><pre className="knowledge-content">{serverContent}</pre></details>}
   {discard && <div className="warning" role="alert"><strong>{t('knowledge.discard_title')}</strong><p>{t('knowledge.discard_confirm')}</p><div className="knowledge-actions"><button type="button" onClick={() => setDiscard(false)}>{t('knowledge.keep_editing')}</button><button type="button" onClick={() => { setDirty(false); onClose(); }}>{t('knowledge.discard')}</button></div></div>}
   <div className="dialog-actions"><button type="button" disabled={blocked} onClick={close}>{t('knowledge.cancel')}</button><button className="primary" disabled={action.busy || action.conflict || (!action.pending && extracting && (!selected || sources.loading))}>{t(action.pending ? 'knowledge.restore' : extracting ? 'knowledge.extract_submit' : 'knowledge.save')}</button></div>
  </form>
 </Modal>;
}
function KnowledgeHistory({ project, id }: {
    project: string;
    id: string;
}) {
    const [open, setOpen] = useState(false), [cursor, setCursor] = useState(''), [rows, setRows] = useState<KnowledgeRevision[]>([]), [selected, setSelected] = useState<number>(), [refresh, setRefresh] = useState(0);
    const page = useToolRead<{
        items: KnowledgeRevision[];
        total: number;
        next_cursor?: string;
    }>(open ? objectPath(id) + '/revisions' + query(project) + '&limit=20' + (cursor ? '&cursor=' + encodeURIComponent(cursor) : '') : '', refresh);
    useEffect(() => { if (page.data) {
        setRows(before => { const revisions = new Map<number, KnowledgeRevision>(); for (const row of [...(cursor ? before : []), ...page.data!.items])
            revisions.set(row.revision, row); return [...revisions.values()]; });
    } }, [page.data, cursor]);
    return <details onToggle={event => setOpen(event.currentTarget.open)}><summary>{t('knowledge.history')}</summary>{open && <><div className="knowledge-actions"><button type="button" disabled={page.loading} onClick={() => { setCursor(''); setRefresh(value => value + 1); }}>{t('knowledge.reload')}</button><small>{t('knowledge.loaded', { loaded: formatNumber(rows.length), total: formatNumber(page.data?.total ?? rows.length) })}</small></div>{page.error && <ErrorNotice error={page.error} onRetry={() => void page.read()}/>}<ul className="knowledge-list-items">{rows.map(row => <li key={row.revision}><button type="button" className="knowledge-entry" aria-expanded={selected === row.revision} onClick={() => setSelected(selected === row.revision ? undefined : row.revision)}><span><strong>{t('knowledge.revision', { revision: formatNumber(row.revision) })} · {row.title}</strong><span className="knowledge-meta">{t('knowledge.state.' + row.state)} · {t('knowledge.method.' + row.method)} · {time(row.created_at)}</span></span><ChevronRight size={15}/></button>{selected === row.revision && <KnowledgeRevisionFacts project={project} revision={row}/>}</li>)}</ul>{page.loading && <Loading />}{page.data?.next_cursor && <button type="button" disabled={page.loading} onClick={() => setCursor(page.data!.next_cursor!)}>{t('knowledge.more')}</button>}</>}</details>;
}
