import { tr, useLocale, UiError } from "../i18n";
import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "../api";
/** Common collection metadata, not a second domain DTO. Actions are supplied by
 * the server even for empty lists, so an owner role alone never unlocks writes. */
type Collection<T> = {
  items: T[];
  total: number;
  next_cursor?: string;
  allowed_actions: string[];
};
export function usePoolPage<T extends { id: string }>(
  path: string,
  refresh: number,
) {
  const epoch = useRef(0);
  const [state, setState] = useState({
    path,
    items: [] as T[],
    total: 0,
    next: "",
    allowed: [] as string[],
    loading: !!path,
    error: "",
  });
  const read = useCallback(
    async (cursor = "") => {
      const generation = ++epoch.current;
      if (!path) {
        setState({
          path,
          items: [],
          total: 0,
          next: "",
          allowed: [],
          loading: false,
          error: "",
        });
        return;
      }
      setState((before) => ({
        ...before,
        path,
        items: before.path === path ? before.items : [],
        allowed: [],
        loading: true,
        error: "",
      }));
      try {
        const page = await api<Collection<T>>(
          path +
            (path.includes("?") ? "&" : "?") +
            "limit=50" +
            (cursor ? "&cursor=" + encodeURIComponent(cursor) : ""),
        );
        if (
          !Array.isArray(page.items) ||
          !Number.isSafeInteger(page.total) ||
          page.total < 0
        )
          throw new UiError("pools.list.invalid_scope");
        if (epoch.current !== generation) return;
        setState((before) => {
          const items = new Map<string, T>();
          for (const item of [
            ...(cursor && before.path === path ? before.items : []),
            ...page.items,
          ])
            items.set(item.id, item);
          return {
            path,
            items: [...items.values()],
            total: page.total,
            next: page.next_cursor || "",
            allowed: page.allowed_actions || [],
            loading: false,
            error: "",
          };
        });
      } catch (error) {
        if (epoch.current === generation)
          setState((before) => ({
            ...before,
            loading: false,
            error: (error as Error).message,
            allowed: [],
          }));
      }
    },
    [path],
  );
  useEffect(() => {
    void read();
    return () => {
      epoch.current++;
    };
  }, [read, refresh]);
  return {
    ...(state.path === path
      ? state
      : {
          path,
          items: [],
          total: 0,
          next: "",
          allowed: [],
          loading: !!path,
          error: "",
        }),
    read,
  };
}
