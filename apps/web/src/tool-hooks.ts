import { tr, useLocale, renderDiagnostic } from "./i18n";
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { ApiError } from "./api";
import { EntityPager } from "./pagination";
import { useLeaveGuard } from "./write-action";
import { ToolWriteRecovery } from "./tool-write";
import {useApi} from './api-context';
import {ToolProfilePager,type ToolListFilters} from './tool-management-list-model';
export function toolError(error: unknown) {
  if (typeof error === "string") return renderDiagnostic(error);
  return error instanceof ApiError
    ? tr("development.errors.with_code",{message:renderDiagnostic(error),code:error.code})
    : error instanceof Error
      ? renderDiagnostic(error)
      : tr("common.readIncomplete");
}
export function toolSessionStorage() {
  try {
    return typeof sessionStorage === "undefined" ? undefined : sessionStorage;
  } catch {
    return undefined;
  }
}
type ToolReadView<T>={path:string;data?:T;loading:boolean;error:string;failure?:unknown;read:()=>Promise<void>};
export function useToolRead<T>(path: string, refresh: number | string = 0):ToolReadView<T> {
  useLocale();
  const caller=useApi();
  const [state, setState] = useState<{
      path: string;
      caller:typeof caller;
      data?: T;
      loading: boolean;
      error: string;
      failure?: unknown;
    }>({ path, caller, loading: !!path, error: "" }),
    epoch = useRef(0);
  const owner=useRef(caller);
  if(owner.current!==caller){owner.current=caller;epoch.current++;}
  const read = useCallback(async () => {
    const generation = ++epoch.current;
    if (!path) {
      setState({ path, caller, loading: false, error: "" });
      return;
    }
    setState((before) => ({
      path,
      caller,
      data: before.path === path&&before.caller===caller ? before.data : undefined,
      loading: true,
      error: "",
      failure: undefined,
    }));
    try {
      const data = await caller<T>(path);
      if (generation === epoch.current)
        setState({ path, caller, data, loading: false, error: "" });
    } catch (error) {
      if (generation === epoch.current)
        setState((before) => ({
          ...before,
          data:error instanceof ApiError&&[401,403,404].includes(error.status)?undefined:before.data,
          loading: false,
          error: toolError(error),
          failure: error,
        }));
    }
  }, [path,caller]);
  useEffect(() => {
    void read();
    return () => {
      epoch.current++;
    };
  }, [read, refresh]);
  const current=state.path===path&&state.caller===caller;
  return {
    path,
    data:current?state.data:undefined,
    loading:current?state.loading:!!path,
    failure:current?state.failure:undefined,
    error:
      current
        ? state.failure
          ? toolError(state.failure)
          : state.error
        : "",
    read,
  };
}
export function useToolList<T extends { id: string }>(
  project: string,
  kind: string,
  refresh = 0,
) {
  const pager = useRef(new EntityPager<T>(project, kind)),
    [page, setPage] = useState(() => pager.current.snapshot());
  const read = useCallback(
    async (more = false) => {
      pager.current.setScope(project, kind);
      const request = more ? pager.current.more() : pager.current.first();
      setPage(pager.current.snapshot());
      await request;
      setPage(pager.current.snapshot());
    },
    [project, kind],
  );
  useEffect(() => {
    void read();
  }, [read, refresh]);
  return { ...page, read };
}
export function useManagedToolProfiles<T extends {id:string}>(project:string,filters:ToolListFilters,refresh:number){
 const caller=useApi(),holder=useRef<{caller:typeof caller;pager:ToolProfilePager<T>}|undefined>(undefined);
 if(!holder.current||holder.current.caller!==caller){holder.current?.pager.invalidate();holder.current={caller,pager:new ToolProfilePager<T>(project,filters,caller)};}
 const pager=holder.current.pager;pager.setScope(project,filters);
 const [,render]=useState(0),mounted=useRef(false),current=useRef(pager);current.current=pager;
 useLayoutEffect(()=>{mounted.current=true;return()=>{mounted.current=false;pager.invalidate();};},[pager]);
 const read=useCallback(async(more=false)=>{
  if(!mounted.current||current.current!==pager)return;
  const request=more?pager.more():pager.first();render(value=>value+1);
  await request;if(mounted.current&&current.current===pager)render(value=>value+1);
 },[pager,project,filters.state,filters.query]);
 useEffect(()=>{void read();},[read,refresh]);
 return {...pager.snapshot(),read};
}

type ToolRecovery = Pick<ToolWriteRecovery, "pending" | "error" | "run" | "restore">;
type ActionState = { recovery: ToolRecovery; busy: boolean; error: unknown; conflict: boolean; deferred: boolean; sequence: number; ticket?: number };
export function useToolAction(recovery: ToolRecovery, onDefer?: () => void) {
  useLocale();
  const currentRecovery = useRef(recovery), holder = useRef<ActionState | undefined>(undefined), lifetime = useRef<{ active: boolean } | undefined>(undefined);
  currentRecovery.current = recovery;
  if (!holder.current || holder.current.recovery !== recovery) holder.current = { recovery, busy: false, error: "", conflict: false, deferred: false, sequence: 0 };
  const current = holder.current;
  const [, render] = useState(0);
  useLayoutEffect(() => {
    const owner = { active: true };
    lifetime.current = owner;
    current.busy = false;
    current.ticket = undefined;
    return () => {
      owner.active = false;
      if (lifetime.current === owner) lifetime.current = undefined;
      current.busy = false;
      current.ticket = undefined;
    };
  }, [recovery]);
  const isCurrent = () => holder.current === current && currentRecovery.current === recovery && !!lifetime.current?.active;
  const notify = () => { if (isCurrent()) render(value => value + 1); };
  const setError = (value: unknown) => { if (isCurrent()) { current.error = typeof value === 'function' ? value(current.error) : value; notify(); } };
  const pending = recovery.pending;
  useLeaveGuard(current.busy || (!!pending && !current.deferred), setError);
  async function perform<T>(send: (ownsRequest: () => boolean) => Promise<T>) {
    if (!isCurrent() || current.busy) return;
    const owner = lifetime.current, ticket = ++current.sequence;
    current.ticket = ticket;
    current.busy = true;
    current.error = "";
    current.conflict = false;
    current.deferred = false;
    const ownsRequest = () => isCurrent() && lifetime.current === owner && !!owner?.active && current.ticket === ticket && current.busy;
    notify();
    try {
      const value = await send(ownsRequest);
      if (ownsRequest()) return value;
    } catch (error) {
      if (!ownsRequest()) return undefined;
      current.error = { source: error, originalRefusal: !!recovery.pending && error instanceof ApiError && error.status >= 400 && error.status < 500 };
      current.conflict = error instanceof ApiError && error.status === 409 && !recovery.pending;
      return undefined;
    } finally {
      if (ownsRequest()) {
        current.busy = false;
        current.ticket = undefined;
        notify();
      }
    }
  }
  return {
    busy: current.busy,
    error: renderDiagnostic(recovery.error) || (current.error && typeof current.error === 'object' && 'source' in current.error ? toolError(current.error.source) + ('originalRefusal' in current.error && current.error.originalRefusal ? tr('common.originalRefusal') : '') : current.error ? toolError(current.error) : ''),
    conflict: current.conflict,
    pending,
    deferred: current.deferred,
    setError,
    clearError: () => { if (isCurrent()) { current.error = ''; current.conflict = false; notify(); } },
    defer: () => { if (isCurrent() && !current.busy && recovery.pending) { current.deferred = true; notify(); onDefer?.(); } },
    run: <T>(path: string, body: Record<string, unknown>, method: 'POST' | 'PATCH' = 'POST') => perform<T>(ownsRequest => recovery.run<T>(path, body, method, ownsRequest)),
    restore: <T>() => perform<T>(ownsRequest => recovery.restore<T>(ownsRequest)),
  };
}
export type ToolAction = ReturnType<typeof useToolAction>;
