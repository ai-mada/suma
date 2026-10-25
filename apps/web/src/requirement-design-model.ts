import { UiError, tr } from './i18n';
import type { RecordItem } from './api';
import type {
    RequirementDesignContent, RequirementDesignTask, RequirementDesignDiagnostic,
    RequirementDesignRevision, RequirementDesignView, RequirementDesignAdoptedTask,
    RequirementDesignAdoption, RequirementDesignChange, RequirementDesignPreview,
    RequirementDesignRevisionList, TaskContextRecord,
} from '../../../packages/contracts/src';

// Public HTTP shapes have one source: generated contracts. Only Record's UI
// normalization (criteria arrays become display text) is projected locally.
export type DesignContent = RequirementDesignContent;
export type DesignTask = RequirementDesignTask;
export type DesignRequirement = TaskContextRecord;
export type DesignDiagnostic = RequirementDesignDiagnostic;
export type DesignRevision = RequirementDesignRevision;
export type AdoptedDesignTask = RequirementDesignAdoptedTask;
export type DesignAdoption = RequirementDesignAdoption;
export type RequirementDesign = RequirementDesignView;
export type DesignChange = Omit<RequirementDesignChange, 'current'> & {current?: RecordItem};
export type DesignPreview = Omit<RequirementDesignPreview, 'changes'> & {changes: DesignChange[]};
export type DesignPage<T> = Omit<RequirementDesignRevisionList, 'items'> & {items: T[]};
export type DesignDraft = {
    // A recovery row contains a source version, not a fabricated source snapshot.
    source_unknown?: boolean;
    expected_version: number;
    requirement: DesignRequirement;
    content: DesignContent;
    tasks: DesignTask[];
};
export const designSections = ['product', 'experience', 'technical'] as const;
export function designPath(id: string) { return '/requirements/' + encodeURIComponent(id) + '/design'; }
export function designDraft(value: RequirementDesign): DesignDraft {
    return structuredClone({ expected_version: value.version, requirement: value.latest?.requirement || value.requirement, content: value.latest?.content || { product: '', experience: '', technical: '' }, tasks: value.latest?.tasks || [] });
}
export function newDesignTask(): DesignTask { return { key: 'task-' + crypto.randomUUID().slice(0, 12), title: '', description: '', priority: 'medium', owner_id: '', depends_on: [], acceptance_criteria: [], covers: [] }; }
export function taskCriteria(value: unknown): string[] { return Array.isArray(value) ? value.map(String) : typeof value === 'string' ? value.split('\n').filter(Boolean) : []; }
export function designDiagnostic(value: DesignDiagnostic) { return tr('requirement-design.diagnostic.' + value.code, { key: value.task_key || '', index: value.criterion_index || 0 }); }
/** Structural limits apply even to incomplete drafts. Never trim or truncate
 * approved/user text to make a request fit its source or execution budget. */
export function designSaveBody(project: string, draft: DesignDraft) {
    const bytes = (value: string) => new TextEncoder().encode(value).length;
    const tasks = structuredClone(draft.tasks).map(task => ({ ...task, acceptance_criteria: task.acceptance_criteria.filter(value => value.trim()) }));
    if (designSections.reduce((sum, key) => sum + bytes(draft.content[key]), 0) > 16000)
        throw new UiError('requirement-design.errors.content_limit');
    if (tasks.length > 100 || bytes(JSON.stringify(tasks)) > 262144)
        throw new UiError('requirement-design.errors.tasks_limit');
    const keys = new Set<string>();
    for (const task of tasks) {
        if (!/^[A-Za-z0-9_-]{1,64}$/.test(task.key) || keys.has(task.key))
            throw new UiError('requirement-design.errors.task_key');
        keys.add(task.key);
        if (bytes(task.title) > 200 || bytes(task.description) > 4000 || task.acceptance_criteria.length > 20 || task.acceptance_criteria.some(value => !value.trim() || bytes(value) > 1000) || task.depends_on.length > 100 || task.covers.length > 100 || task.covers.some(value => !Number.isSafeInteger(value) || value < 1) || !['urgent', 'high', 'medium', 'low'].includes(task.priority))
            throw new UiError('requirement-design.errors.task_limit');
    }
    return { project_id: project, expected_version: draft.expected_version, expected_requirement_version: draft.requirement.version, content: structuredClone(draft.content), tasks };
}
/** Fast, local publication guidance. The service repeats all checks against its
 * current transaction; client guidance never grants publication authority. */
export function designDiagnostics(draft: DesignDraft): DesignDiagnostic[] {
    const out: DesignDiagnostic[] = [];
    for (const key of designSections)
        if (!draft.content[key].trim())
            out.push({ code: key + '_required', field: 'content.' + key });
    if (!draft.requirement.acceptance_criteria.length)
        out.push({ code: 'requirement_criteria_required', field: 'requirement' });
    if (!draft.tasks.length)
        out.push({ code: 'tasks_required', field: 'tasks' });
    const keys = new Map(draft.tasks.map(task => [task.key, task])), covered = new Set<number>();
    for (const task of draft.tasks) {
        if (!task.title.trim())
            out.push({ code: 'task_title_required', field: 'tasks', task_key: task.key });
        if (!task.acceptance_criteria.some(value => value.trim()))
            out.push({ code: 'task_criteria_required', field: 'tasks', task_key: task.key });
        for (const key of task.depends_on)
            if (!keys.has(key))
                out.push({ code: 'dependency_missing', field: 'tasks', task_key: task.key });
        for (const index of task.covers)
            if (!Number.isSafeInteger(index) || index < 1 || index > draft.requirement.acceptance_criteria.length)
                out.push({ code: 'criterion_invalid', field: 'tasks', task_key: task.key, criterion_index: index });
            else
                covered.add(index);
    }
    const active = new Set<string>(), done = new Set<string>();
    function visit(key: string): boolean { if (active.has(key))
        return true; if (done.has(key))
        return false; active.add(key); for (const next of keys.get(key)?.depends_on || [])
        if (keys.has(next) && visit(next))
            return true; active.delete(key); done.add(key); return false; }
    if (draft.tasks.some(task => visit(task.key)))
        out.push({ code: 'dependency_cycle', field: 'tasks' });
    draft.requirement.acceptance_criteria.forEach((_, i) => { if (!covered.has(i + 1))
        out.push({ code: 'criterion_uncovered', field: 'tasks', criterion_index: i + 1 }); });
    return out;
}
