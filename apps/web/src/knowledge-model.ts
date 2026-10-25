import { UiError, tr } from './i18n';
export const knowledgeKinds = ['memory', 'spec', 'skill'] as const;
export type KnowledgeKind = typeof knowledgeKinds[number];
export type KnowledgeState = 'draft' | 'published' | 'retired';
export type KnowledgeSourceKind = 'documents' | 'requirements' | 'tasks' | 'issues';
export type KnowledgeSource = {
    kind: KnowledgeSourceKind;
    id: string;
    version: number;
    title: string;
    quote: string;
    sha256: string;
};
export type KnowledgeSourceInput = Pick<KnowledgeSource, 'kind' | 'id' | 'version'> & {
    quote?: string;
};
export type KnowledgeReference = {
    id: string;
    revision: number;
    sha256: string;
};
export type KnowledgeRevision = {
    revision: number;
    based_on_revision: number;
    state: KnowledgeState;
    kind: KnowledgeKind;
    title: string;
    summary: string;
    content: string;
    sha256: string;
    sources: KnowledgeSource[];
    method: 'manual' | 'extractive';
    created_by: string;
    created_at: string;
};
export type KnowledgeSummary = {
    id: string;
    project_id: string;
    version: number;
    kind: KnowledgeKind;
    state: KnowledgeState;
    title: string;
    summary: string;
    latest_revision: number;
    published_revision: number;
    published_sha256: string;
    published_title: string;
    published_summary: string;
    has_draft: boolean;
    published_eligible: boolean;
    eligibility: 'none' | 'current' | 'source_changed' | 'retired' | 'publication_changed';
    created_at: string;
    updated_at: string;
};
export type KnowledgeDetail = KnowledgeSummary & {
    latest: KnowledgeRevision;
    published?: KnowledgeRevision;
};
export type KnowledgePage = {
    items: KnowledgeSummary[];
    total: number;
    next_cursor?: string;
};
export type KnowledgeSnapshot = KnowledgeReference & {
    kind: string;
    title: string;
    summary: string;
    content: string;
    sources: (Omit<KnowledgeSource, 'kind' | 'quote'> & {
        kind: string;
        quote?: string;
    })[];
};
export type KnowledgeFilters = {
    kind: 'all' | KnowledgeKind;
    state: 'all' | KnowledgeState;
    q: string;
};
export function knowledgeListPath(project: string, filters: KnowledgeFilters, cursor = '') {
    const query = new URLSearchParams({ project_id: project, limit: '50' });
    if (filters.kind !== 'all')
        query.set('kind', filters.kind);
    if (filters.state !== 'all')
        query.set('state', filters.state);
    if (filters.q.trim())
        query.set('q', filters.q.trim());
    if (cursor)
        query.set('cursor', cursor);
    return '/project-knowledge?' + query;
}
// Selection is explicit and binds the published revision/hash returned by the
// server. An edited draft never silently replaces the already selected version.
export function publishedKnowledgeReference(entry: KnowledgeSummary, project: string): KnowledgeReference {
    if (entry.project_id !== project || entry.state === 'retired' || entry.published_eligible !== true || entry.eligibility !== 'current' || !Number.isInteger(entry.published_revision) || entry.published_revision < 1 || !/^[a-f0-9]{64}$/.test(entry.published_sha256))
        throw new UiError('knowledge.errors.ineligible');
    return { id: entry.id, revision: entry.published_revision, sha256: entry.published_sha256 };
}
export function knowledgeSourceInputs(sources: readonly KnowledgeSource[]): KnowledgeSourceInput[] {
    return sources.map(({ kind, id, version, quote }) => ({ kind, id, version, ...(quote ? { quote } : {}) }));
}
export function pinnedKnowledgeReferences(entries: readonly KnowledgeSummary[], project: string): KnowledgeReference[] {
    if (entries.length > 16 || new Set(entries.map(entry => entry.id)).size !== entries.length)
        throw new UiError('knowledge.errors.selection_limit');
    return entries.map(entry => publishedKnowledgeReference(entry, project));
}

// Record families do not share one translation namespace: task naming belongs
// to the workbench tabs. Unknown provider kinds remain literal metadata.
export function knowledgeSourceLabel(kind:string):string {
 const keys:Record<string,string>={documents:'common.entities.documents',requirements:'common.entities.requirements',tasks:'shell.tabs.tasks',issues:'common.entities.issues'};
 return keys[kind]?tr(keys[kind]):kind;
}
