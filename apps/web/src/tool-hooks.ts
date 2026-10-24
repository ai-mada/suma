import { useCallback, useEffect, useRef, useState } from "react";
import { ApiError, api } from "./api";
import { EntityPager } from "./pagination";
import { useLeaveGuard } from "./write-action";
import { ToolWriteRecovery } from "./tool-write";
export function toolError(error: unknown) {
  return error instanceof ApiError
    ? `${error.message}（${error.code}）`
    : error instanceof Error
      ? error.message
      : "读取未完成，请重新核对。";
}
export function toolSessionStorage() {
  try {
    return typeof sessionStorage === "undefined" ? undefined : sessionStorage;
  } catch {
    return undefined;
  }
}
export function useToolRead<T>(path: string, refresh = 0) {
  const [state, setState] = useState<{
      path: string;
      data?: T;
      loading: boolean;
      error: string;
    }>({ path, loading: !!path, error: "" }),
    epoch = useRef(0);
  const read = useCallback(async () => {
    const generation = ++epoch.current;
    if (!path) {
      setState({ path, loading: false, error: "" });
      return;
    }
    setState((before) => ({
      path,
      data: before.path === path ? before.data : undefined,
      loading: true,
      error: "",
    }));
    try {
      const data = await api<T>(path);
      if (generation === epoch.current)
        setState({ path, data, loading: false, error: "" });
    } catch (error) {
      if (generation === epoch.current)
        setState((before) => ({
          ...before,
          loading: false,
          error: toolError(error),
        }));
    }
  }, [path]);
  useEffect(() => {
    void read();
    return () => {
      epoch.current++;
    };
  }, [read, refresh]);
  return {
    ...(state.path === path
      ? state
      : { path, data: undefined, loading: !!path, error: "" }),
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
export function useToolAction(
  recovery: ToolWriteRecovery,
  onDefer?: () => void,
) {
  const epoch = useRef(0);
  useEffect(() => {
    const generation = ++epoch.current;
    return () => {
      if (epoch.current === generation) epoch.current++;
    };
  }, [recovery]);
  const [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [conflict, setConflict] = useState(false),
    [deferred, setDeferred] = useState(false),
    [, render] = useState(0);
  const pending = recovery.pending;
  useLeaveGuard(busy || (!!pending && !deferred), setError);
  async function perform<T>(send: () => Promise<T>) {
    if (busy) return;
    const generation = epoch.current;
    setBusy(true);
    setError("");
    setConflict(false);
    setDeferred(false);
    try {
      const value = await send();
      // An old actor/project result must not navigate the current workbench.
      if (generation === epoch.current) return value;
    } catch (error) {
      if (generation !== epoch.current) return undefined;
      setError(
        toolError(error) +
          (recovery.pending &&
          error instanceof ApiError &&
          error.status > 0 &&
          error.status < 500
            ? " 当前拒绝不能证明原未知操作没有生效，请保留原标识核对。"
            : ""),
      );
      setConflict(
        error instanceof ApiError && error.status === 409 && !recovery.pending,
      );
      return undefined;
    } finally {
      if (generation === epoch.current) {
        setBusy(false);
        render((value) => value + 1);
      }
    }
  }
  return {
    busy,
    error: recovery.error || error,
    conflict,
    pending,
    deferred,
    setError,
    clearError: () => {
      setError("");
      setConflict(false);
    },
    defer: () => {
      if (!busy && pending) {
        setDeferred(true);
        onDefer?.();
      }
    },
    run: <T>(
      path: string,
      body: Record<string, unknown>,
      method: "POST" | "PATCH" = "POST",
    ) => perform<T>(() => recovery.run<T>(path, body, method)),
    restore: <T>() => perform<T>(() => recovery.restore<T>()),
  };
}
export type ToolAction = ReturnType<typeof useToolAction>;
