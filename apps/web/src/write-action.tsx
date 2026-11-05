import { useEffect, useRef, useState, type ReactNode } from 'react';
import { tr, useLocale, renderDiagnostic, UiError } from './i18n';
import { ApiError, operation, api } from './api';
import { mutate } from './operations';
import { useApi } from './api-context';
import { ErrorNotice } from './ui';
export const uncertainWrite = (error: unknown) => !(error instanceof ApiError) || error.status === 0 || error.status >= 500;
export function useLeaveGuard(blocked: boolean, setError: (message: string) => void) { useEffect(() => { const guard = (event: Event) => { if (blocked) {
    event.preventDefault();
    setError(tr('preferences.write.guard'));
} }; const unload = (event: BeforeUnloadEvent) => { if (blocked) {
    event.preventDefault();
    event.returnValue = '';
} }; window.addEventListener('suma:navigate', guard); window.addEventListener('beforeunload', unload); return () => { window.removeEventListener('suma:navigate', guard); window.removeEventListener('beforeunload', unload); }; }, [blocked, setError]); }
type StorageLike = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>;
export type WriteRecoveryRequest = {
    path: string;
    body: Record<string, unknown>;
    method: string;
    idempotency_key: string;
};
export type WriteActionOptions = {
    scope?: string;
    storage?: StorageLike;
    validateRequest?: (request: WriteRecoveryRequest) => boolean;
    validateResponse?: (value: unknown, request: WriteRecoveryRequest) => boolean | Promise<boolean>;
    /** Route-specific conflicts that prove the exact restored request did not commit. */
    definitiveRefusalOnRestore?: (error: ApiError, request: WriteRecoveryRequest) => boolean;
};
type Pending = {
    request: WriteRecoveryRequest;
    caller: typeof api;
    identity: string;
    uncertain: boolean;
    validateResponse?: WriteActionOptions['validateResponse'];
    definitiveRefusalOnRestore?: WriteActionOptions['definitiveRefusalOnRestore'];
    outcome?: {
        ok: true;
        value: unknown;
    } | {
        ok: false;
        error: unknown;
    };
};
type RecoveryState = {
    scope?: string;
    storage?: StorageLike;
    key?: string;
    pending?: Pending;
    error?: unknown;
    cleanupFailure?: boolean;
    busy: boolean;
};
const recoveryError = (key: string) => new UiError('preferences.write.' + key);
function validRecovery(value: unknown, validate?: WriteActionOptions['validateRequest']): value is WriteRecoveryRequest {
    if (!value || typeof value !== 'object' || Array.isArray(value))
        return false;
    const request = value as WriteRecoveryRequest;
    return Object.keys(request).sort().join(',') === 'body,idempotency_key,method,path' && typeof request.path === 'string' && /^\/(?!\/)[A-Za-z0-9_./%?=&:-]+$/.test(request.path) && !request.path.includes('..') && ['POST', 'PATCH', 'DELETE', 'PUT'].includes(request.method) && !!request.body && typeof request.body === 'object' && !Array.isArray(request.body) && typeof request.idempotency_key === 'string' && request.idempotency_key.length >= 8 && request.idempotency_key.length <= 128 && !/[\x00-\x1f\x7f]/.test(request.idempotency_key) && request.body.idempotency_key === request.idempotency_key && (!validate || validate(request));
}
function recoveryIdentity(scope: string | undefined, instance: string, request: WriteRecoveryRequest) { return JSON.stringify(['write-action', scope || instance, request.path, request.method, request.idempotency_key]); }
function loadRecovery(options: WriteActionOptions, caller: typeof api, instance: string): RecoveryState {
    const state: RecoveryState = { scope: options.scope, storage: options.storage, key: options.scope ? 'suma-write-recovery-v1:' + options.scope : undefined, busy: false };
    if (!state.key)
        return state;
    try {
        if (!state.storage)
            throw Error('unavailable');
        const raw = state.storage.getItem(state.key);
        if (raw) {
            let request: unknown;
            try {
                request = JSON.parse(raw);
            }
            catch {
                state.error = recoveryError('recovery_invalid');
                return state;
            }
            if (!validRecovery(request, options.validateRequest)) {
                state.error = recoveryError('recovery_invalid');
                return state;
            }
            state.pending = { request, caller, identity: recoveryIdentity(state.scope, instance, request), uncertain: true, validateResponse: options.validateResponse, definitiveRefusalOnRestore: options.definitiveRefusalOnRestore };
        }
    }
    catch {
        state.error = recoveryError('storage_unavailable');
    }
    return state;
}
export function useWriteAction(options: WriteActionOptions = {}) {
    useLocale();
    const api = useApi(), instance = useRef(crypto.randomUUID()), alive = useRef(true), viewScope = useRef(options.scope), viewCaller = useRef(api), holder = useRef<RecoveryState | undefined>(undefined);
    useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
    viewScope.current = options.scope;
    viewCaller.current = api;
    // A mounted unknown operation retains its original scope/caller even if the
    // Provider changes. A fresh mount only reads its own exact scoped storage key.
    if (!holder.current || (!holder.current.pending && holder.current.scope !== options.scope))
        holder.current = loadRecovery(options, api, instance.current);
    const [busy, setBusy] = useState(false), [failure, setError] = useState<unknown>(''), [conflict, setConflict] = useState(false), [, render] = useState(0);
    const state = holder.current, unknown = !!state.pending?.uncertain;
    useLeaveGuard(busy || !!state.pending, setError);
    function clearStored(current: RecoveryState) { if (current.key) {
        if (!current.storage)
            throw recoveryError('storage_unavailable');
        current.storage.removeItem(current.key);
    } current.error = undefined; current.cleanupFailure = false; }
    async function run<T = any>(path: string, body: Record<string, unknown>, method?: string): Promise<T | undefined> {
        let current = holder.current!;
        if (current.busy)
            return;
        if (current.error && !current.cleanupFailure) {
            const reloaded = loadRecovery(options, api, instance.current);
            if (reloaded.error) {
                setError(reloaded.error);
                return;
            }
            holder.current = current = reloaded;
        }
        if (!current.pending) {
            try {
                const key = typeof body.idempotency_key === 'string' ? body.idempotency_key : operation();
                const request: WriteRecoveryRequest = { path, body: JSON.parse(JSON.stringify({ ...body, idempotency_key: key })), method: method || 'POST', idempotency_key: key };
                if (!validRecovery(request, options.validateRequest))
                    throw recoveryError('recovery_invalid');
                // Persist before sending. Storage failure cannot create an untracked write.
                if (current.key) {
                    if (!current.storage)
                        throw recoveryError('storage_unavailable');
                    current.storage.setItem(current.key, JSON.stringify(request));
                }
                current.pending = { request, caller: api, identity: recoveryIdentity(current.scope, instance.current, request), uncertain: false, validateResponse: options.validateResponse, definitiveRefusalOnRestore: options.definitiveRefusalOnRestore };
            }
            catch (error) {
                current.error = error instanceof UiError ? error : recoveryError('storage_unavailable');
                setError(current.error);
                render(value => value + 1);
                return;
            }
        }
        const pending = current.pending, restoring = pending.uncertain;
        const originalScope=current.scope, originalIdentity=pending.identity, originalRequest=pending.request, originalKey=pending.request.idempotency_key, ownerCaller=api;
        // An asynchronous receipt validator (for example, Web Crypto) creates
        // another ownership boundary after HTTP. Caller identity detects an
        // account/organization change; a complete consumer scope includes its
        // project. Neither the request nor its durable key can move to a new
        // holder while a late result is being verified.
        const ownsRequest=()=>alive.current&&viewCaller.current===ownerCaller&&viewScope.current===originalScope&&current.scope===originalScope&&holder.current===current&&current.pending===pending&&pending.identity===originalIdentity&&pending.request===originalRequest&&pending.request.idempotency_key===originalKey&&pending.request.body.idempotency_key===originalKey;
        current.busy = true;
        setBusy(true);
        setError('');
        try {
            if (!pending.outcome) {
                try {
                    const request = pending.request;
                    const value = await mutate<T>(request.path, request.body, request.method, pending.identity, pending.caller);
                    if(!ownsRequest()){pending.uncertain=true;return;}
                    // A success status alone does not acknowledge a usable result.
                    // Keep the original validator, caller and durable request until
                    // that exact operation returns a response its consumer can read.
                    let valid = true;
                    try { valid = !pending.validateResponse || await pending.validateResponse(value, structuredClone(request)); }
                    catch { valid = false; }
                    if(!ownsRequest()){pending.uncertain=true;return;}
                    if (!valid) {
                        const diagnostic = new UiError('common.responseUnknown');
                        throw new ApiError(0, 'response_unknown', diagnostic.message, undefined, undefined, diagnostic);
                    }
                    pending.outcome = { ok: true, value };
                }
                catch (error) {
                    if(!ownsRequest()){pending.uncertain=true;return;}
                    let definitive = false;
                    if (restoring && error instanceof ApiError && error.status === 409) {
                        try { definitive = pending.definitiveRefusalOnRestore?.(error, structuredClone(originalRequest)) === true; }
                        catch { /* Unverified refusal still retains the original request. */ }
                    }
                    if ((restoring && !definitive) || uncertainWrite(error)) {
                        pending.uncertain = true;
                        setConflict(false);
                        setError(error);
                        return;
                    }
                    pending.outcome = { ok: false, error };
                }
            }
            // A late result belongs to the original scoped view. Keep its
            // recovery row until that view consumes it; clearing first would
            // lose a create operation's locator after unmount/account switch.
            if(!ownsRequest()){pending.uncertain=true;return undefined;}
            pending.uncertain = false;
            try {
                clearStored(current);
            }
            catch {
                current.error = recoveryError('cleanup_failed');
                current.cleanupFailure = true;
                setError(current.error);
                return;
            }
            const outcome = pending.outcome;
            current.pending = undefined;
            if (outcome.ok) {
                setConflict(false);
                return !alive.current || current.scope !== undefined && current.scope !== viewScope.current ? undefined : outcome.value as T;
            }
            setConflict(outcome.error instanceof ApiError && outcome.error.status === 409);
            setError(outcome.error);
            return;
        }
        finally {
            current.busy = false;
            setBusy(false);
            render(value => value + 1);
        }
    }
    // Keep authorization status separate from localized prose so a consumer can
    // immediately hide protected facts after403, including a recovered write.
    // This does not grant a retry, clear the original operation, or change scope.
    const terminalFailure = state.pending?.outcome?.ok === false ? state.pending.outcome.error : undefined;
    const httpFailure = [failure, state.error, terminalFailure].find((value): value is ApiError => value instanceof ApiError);
    return { run, busy, error: renderDiagnostic(failure || state.error), errorStatus: httpFailure?.status, setError, unknown, conflict, pending: state.pending ? structuredClone(state.pending.request) : undefined, persistenceBlocked: !!state.error, clearConflict: () => { if (holder.current?.pending)
            return; setConflict(false); setError(''); } };
}
export type WriteAction = ReturnType<typeof useWriteAction>;
export function WriteNotice({ action, onRebase }: {
    action: WriteAction;
    onRebase?: () => Promise<void>;
}) { useLocale(); return <>{action.error && <ErrorNotice error={action.error}/>} {action.unknown && <p className="warning" role="status">{tr('preferences.write.unknown')}</p>}{action.conflict && onRebase && <div className="conflict-banner"><div><strong>{tr('preferences.write.conflict')}</strong><p>{tr('preferences.write.preserved')}</p><button type="button" disabled={action.busy} onClick={async () => { try {
    await onRebase();
    action.clearConflict();
}
catch (e) {
    action.setError(e);
} }}>{tr('preferences.write.rebase')}</button></div></div>}</>; }
export function WriteFields({ action, children }: {
    action: WriteAction;
    children: ReactNode;
}) { return <fieldset className="form-fields" disabled={action.busy || !!action.pending}>{children}</fieldset>; }
export function WriteButton({ action, children, disabled = false }: {
    action: WriteAction;
    children: ReactNode;
    disabled?: boolean;
}) { useLocale(); return <button className="primary" disabled={action.busy || disabled || action.conflict}>{action.busy ? tr('preferences.write.submitting') : action.pending ? tr('preferences.write.restore') : children}</button>; }
