import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { ApiError, type ApiCaller } from "./api";
import { useApi, useApiView } from "./api-context";
import { UiError, renderDiagnostic, useLocale } from "./i18n";
import type { SchedulingPage } from "./scheduling-model";

type ReadScope = { path: string; caller: ApiCaller; headers?: Record<string, string> };
type PageState<T> = {
  scope: ReadScope;
  items: T[];
  total: number;
  next: string;
  loading: boolean;
  hasSnapshot: boolean;
  accessLost: boolean;
  failure?: unknown;
  failedRead?: { cursor: string; preserve: boolean };
};

function emptyPage<T>(scope: ReadScope): PageState<T> {
  return { scope, items: [], total: 0, next: "", loading: !!scope.path, hasSnapshot: false, accessLost: false };
}

export function useSchedulingPage<T>(
  path: string,
  key: (item: T) => string,
  refresh: number | string = 0,
  preserveOnRefresh = false,
) {
  useLocale();
  const caller = useApi();
  const { userID, organizationID } = useApiView() ?? {};
  const scope = useMemo<ReadScope>(() => ({
    path,
    caller,
    headers: userID && organizationID ? {
      "X-Suma-Expected-User": userID,
      "X-Suma-Expected-Organization": organizationID,
    } : undefined,
  }), [path, caller, userID, organizationID]);
  const [state, setState] = useState(() => emptyPage<T>(scope));
  const activeScope = useRef<ReadScope | undefined>(undefined);
  const flight = useRef<AbortController | undefined>(undefined);
  const identity = useRef(key);
  identity.current = key;

  useLayoutEffect(() => {
    activeScope.current = scope;
    return () => {
      activeScope.current = undefined;
      flight.current?.abort();
      flight.current = undefined;
    };
  }, [scope]);

  const read = useCallback(async (cursor = "", preserve = false) => {
    if (!scope.path || activeScope.current !== scope) return;
    flight.current?.abort();
    const controller = new AbortController();
    flight.current = controller;
    const isCurrent = () => flight.current === controller && activeScope.current === scope && !controller.signal.aborted;
    setState((before) => ({
      ...(before.scope === scope ? before : emptyPage<T>(scope)),
      loading: true,
      failure: undefined,
      failedRead: undefined,
    }));

    try {
      const page = await scope.caller<SchedulingPage<T>>(
        scope.path + "&limit=50" + (cursor ? "&cursor=" + encodeURIComponent(cursor) : ""),
        undefined, "GET", scope.headers, controller.signal,
      );
      if (!isCurrent()) return;
      if (!page || !Array.isArray(page.items) || !Number.isSafeInteger(page.total) || page.total < 0 ||
          (page.next_cursor !== undefined && typeof page.next_cursor !== "string")) {
        throw new UiError("development.errors.page_scope_invalid");
      }
      const identify = identity.current;
      const received = page.items.map((item) => [identify(item), item] as const);
      setState((before) => {
        const prior = before.scope === scope ? before.items : [];
        const existing = cursor || preserve ? prior.map((item) => [identify(item), item] as const) : [];
        const items = new Map([...existing, ...received]);
        return {
          scope,
          items: [...items.values()],
          total: page.total,
          next: preserve && prior.length ? before.next : page.next_cursor || "",
          loading: false,
          hasSnapshot: true,
          accessLost: false,
        };
      });
    } catch (error) {
      if (!isCurrent()) return;
      const denied = error instanceof ApiError && [401, 403, 404].includes(error.status);
      setState((before) => {
        const sameScope = before.scope === scope ? before : emptyPage<T>(scope);
        const accessLost = denied || sameScope.accessLost;
        return {
          ...(accessLost ? emptyPage<T>(scope) : sameScope),
          loading: false,
          accessLost,
          failure: error,
          failedRead: accessLost ? undefined : { cursor, preserve },
        };
      });
    } finally {
      if (flight.current === controller) flight.current = undefined;
    }
  }, [scope]);

  useEffect(() => {
    void read("", preserveOnRefresh);
  }, [read, refresh, preserveOnRefresh]);

  const current = state.scope === scope ? state : emptyPage<T>(scope);
  const retry = () => current.accessLost
    ? read()
    : read(current.failedRead?.cursor, current.failedRead?.preserve ?? preserveOnRefresh);
  return {
    path,
    items: current.items,
    total: current.total,
    next: current.next,
    loading: current.loading,
    hasSnapshot: current.hasSnapshot,
    accessLost: current.accessLost,
    error: current.failure ? renderDiagnostic(current.failure) : "",
    failure: current.failure,
    read,
    retry,
  };
}
