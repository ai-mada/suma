import { tr } from "./index";
const kinds = new Set([
  "requirements",
  "tasks",
  "members",
  "issues",
  "documents",
  "releases",
  "tests",
  "evidence",
]);
/** Full entity-specific sentences avoid plural nouns in singular actions and
 * leave user titles and protocol kinds untouched. */
export function recordText(
  kind: string,
  action: string,
  params?: Record<string, unknown>,
) {
  return tr(
    `records.entity.${kinds.has(kind) ? kind : "records"}.${action}`,
    params,
  );
}
