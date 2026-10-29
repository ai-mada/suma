/* Derived from Multica, commit e6f840ca115740a6fb80786537405bcf049d6497,
 * apps/web/features/issues/components/board-view.tsx, buildColumns (77–93).
 * Apache-2.0; see the repository's accompanying third-party source notice.
 * Changes: generic typed keys, status/owner/priority grouping, and preservation
 * of the server's ordering. No drag mutation or client-side task sorting.
 */
export function groupTaskRows<T extends {id: string}>(
  rows: readonly T[], keys: readonly string[], keyOf: (row: T) => string,
): {key: string; rows: T[]}[] {
  return keys.map(key => ({key, rows: rows.filter(row => keyOf(row) === key)}));
}
