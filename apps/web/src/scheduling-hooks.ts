import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "./api";
import type { SchedulingPage } from "./scheduling-model";
export function useSchedulingPage<T>(
  path: string,
  key: (item: T) => string,
  refresh: number | string = 0,
  preserveOnRefresh = false,
) {
  const epoch = useRef(0),
    identity = useRef(key);
  identity.current = key;
  const [state, setState] = useState<{
    path: string;
    items: T[];
    total: number;
    next: string;
    loading: boolean;
    error: string;
  }>({ path, items: [], total: 0, next: "", loading: !!path, error: "" });
  const read = useCallback(
    async (cursor = "", preserve = false) => {
      const generation = ++epoch.current;
      setState((before) => ({
        ...before,
        path,
        items: before.path === path ? before.items : [],
        total: before.path === path ? before.total : 0,
        next: before.path === path ? before.next : "",
        loading: true,
        error: "",
      }));
      try {
        const page = await api<SchedulingPage<T>>(
          path +
            "&limit=50" +
            (cursor ? "&cursor=" + encodeURIComponent(cursor) : ""),
        );
        if (
          !Array.isArray(page.items) ||
          !Number.isSafeInteger(page.total) ||
          page.total < 0
        )
          throw Error("分页响应缺少有效范围。");
        if (epoch.current !== generation) return;
        setState((before) => {
          const map = new Map<string, T>();
          for (const item of [
            ...((cursor || preserve) && before.path === path
              ? before.items
              : []),
            ...page.items,
          ])
            map.set(identity.current(item), item);
          return {
            path,
            items: [...map.values()],
            total: page.total,
            next:
              preserve && before.items.length
                ? before.next
                : page.next_cursor || "",
            loading: false,
            error: "",
          };
        });
      } catch (error) {
        if (epoch.current === generation)
          setState((before) => ({
            ...before,
            path,
            loading: false,
            error: (error as Error).message,
          }));
      }
    },
    [path],
  );
  useEffect(() => {
    if (path) void read("", preserveOnRefresh);
    return () => {
      epoch.current++;
    };
  }, [read, path, refresh, preserveOnRefresh]);
  return {
    ...(state.path === path
      ? state
      : { path, items: [], total: 0, next: "", loading: !!path, error: "" }),
    read,
  };
}
