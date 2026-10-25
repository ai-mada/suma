import { useCallback, useEffect, useRef, useState } from 'react';
import { useApi } from './api-context';
import type { ApiCaller } from './api';
import { UiError } from './i18n';
import { knowledgeListPath, type KnowledgeFilters, type KnowledgePage, type KnowledgeSummary } from './knowledge-model';
type PageState = {
    scope: string;
    caller: ApiCaller;
    items?: KnowledgeSummary[];
    total: number;
    cursor: string;
    loading: boolean;
    error?: unknown;
    more: boolean;
};
/** One bounded server-filtered page per explicit action; an old project/filter
 * response cannot replace the current view. Failed refresh/more retains rows. */
export function useKnowledgeEntries(project: string, filters: KnowledgeFilters, refresh = 0, enabled = true) {
    const api = useApi(), scope = knowledgeListPath(project, filters), epoch = useRef(0), currentScope = useRef(scope);
    currentScope.current = scope;
    const [state, setState] = useState<PageState>({ scope, caller: api, total: 0, cursor: '', loading: false, more: false });
    const snapshot = useRef(state);
    snapshot.current = state;
    const read = useCallback(async (more = false) => {
        if (!enabled)
            return;
        const before = snapshot.current;
        if (more && (before.scope !== scope || before.caller !== api || before.loading || !before.cursor))
            return;
        const request = ++epoch.current, cursor = more ? before.cursor : '';
        setState(old => ({ scope, caller: api, items: old.scope === scope && old.caller === api ? old.items : undefined, total: old.scope === scope && old.caller === api ? old.total : 0, cursor: old.scope === scope && old.caller === api ? old.cursor : '', loading: true, more, error: undefined }));
        try {
            const page = await api<KnowledgePage>(scope + (cursor ? '&cursor=' + encodeURIComponent(cursor) : ''));
            if (!Array.isArray(page.items) || !Number.isSafeInteger(page.total) || page.total < 0)
                throw new UiError('knowledge.errors.invalid_page');
            if (epoch.current !== request || currentScope.current !== scope)
                return;
            const rows = new Map<string, KnowledgeSummary>();
            for (const entry of [...(more ? before.items || [] : []), ...page.items])
                rows.set(entry.id, entry);
            setState({ scope, caller: api, items: [...rows.values()], total: page.total, cursor: page.next_cursor || '', loading: false, more });
        }
        catch (error) {
            if (epoch.current === request && currentScope.current === scope)
                setState(old => ({ ...old, loading: false, error, more }));
        }
    }, [api, scope, enabled]);
    useEffect(() => { if (enabled)
        void read(); return () => { epoch.current++; }; }, [read, refresh]);
    return { ...(state.scope === scope && state.caller === api ? state : { scope, caller: api, items: undefined, total: 0, cursor: '', loading: true, more: false }), read };
}
