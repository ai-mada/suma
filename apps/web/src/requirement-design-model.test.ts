import { it, expect } from 'vitest';
import { designDraft, designSaveBody, designDiagnostics, type RequirementDesign, type DesignTask } from './requirement-design-model';
const task = (key: string): DesignTask => ({ key, title: key, description: '', priority: 'medium', owner_id: '', depends_on: [], acceptance_criteria: ['test'], covers: [1] });
const view: RequirementDesign = { project_id: 'p', requirement_id: 'r', version: 7, requirement: { id: 'r', version: 5, title: 'Current', description: 'Current source', acceptance_criteria: ['new'] }, latest_revision: 3, published_revision: 0, has_draft: true, eligibility: 'requirement_changed', validation: [], latest: { revision: 3, state: 'draft', sha256: 'a'.repeat(64), created_at: '', created_by: 'u', requirement: { id: 'r', version: 2, title: 'Fixed', description: 'Old source', acceptance_criteria: ['old'] }, content: { product: 'Product', experience: 'Experience', technical: 'Technical' }, tasks: [task('stable-a'), { ...task('stable-b'), depends_on: ['stable-a'] }] } };
it('keeps the saved source version and stable keys instead of silently rebasing to current requirement', () => {
    const draft = designDraft(view);
    draft.tasks.reverse();
    const body = designSaveBody('p', draft);
    expect(body.expected_requirement_version).toBe(2);
    expect(body.expected_version).toBe(7);
    expect(body.tasks.map(item => item.key)).toEqual(['stable-b', 'stable-a']);
    expect(body.tasks[0].depends_on).toEqual(['stable-a']);
    expect(designDiagnostics(draft)).toEqual([]);
    expect(view.latest!.tasks[0].key).toBe('stable-a');
});
it('reports missing dependencies, cycles and one-based incomplete coverage without guessing repaired edges', () => {
    const draft = designDraft(view);
    draft.tasks[0].depends_on = ['stable-b'];
    draft.tasks[1].covers = [0, 2];
    draft.requirement.acceptance_criteria.push('second');
    expect(designDiagnostics(draft).map(value => value.code)).toEqual(expect.arrayContaining(['dependency_cycle', 'criterion_invalid']));
    draft.tasks = draft.tasks.slice(0, 1);
    expect(designDiagnostics(draft).map(value => value.code)).toEqual(expect.arrayContaining(['dependency_missing', 'criterion_uncovered']));
});
it('preserves full text and rejects over-budget UTF-8 drafts; empty input separators are not criteria', () => {
    const draft = designDraft(view);
    draft.content.product = '中'.repeat(5334);
    expect(() => designSaveBody('p', draft)).toThrow();
    expect(draft.content.product).toHaveLength(5334);
    draft.content.product = 'Original {{raw}} <script>';
    draft.tasks[0].acceptance_criteria = [' Exact acceptance ', ''];
    expect(designSaveBody('p', draft).tasks[0].acceptance_criteria).toEqual([' Exact acceptance ']);
    expect(draft.tasks[0].acceptance_criteria).toEqual([' Exact acceptance ', '']);
});
