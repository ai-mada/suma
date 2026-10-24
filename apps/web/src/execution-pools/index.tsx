import { tr, useLocale, UiError } from "../i18n";
import { PoolWorkspace } from "./PoolWorkspace";
import { ProjectExecutionWorkspace } from "./ProjectExecution";
import type { PoolProps } from "./model";
export type { PoolProps } from "./model";
// Scope keys also erase memory-only credential views when organization, actor or
// selected pool changes. Persisted recovery stores contain requests, never secrets.
export function ExecutionPools(props: PoolProps) {
  useLocale();
  return (
    <PoolWorkspace
      key={JSON.stringify([
        props.session.organization.id,
        props.session.user.id,
        props.selected,
      ])}
      {...props}
    />
  );
}
export function ProjectExecution(props: PoolProps) {
  useLocale();
  return (
    <ProjectExecutionWorkspace
      key={JSON.stringify([
        props.session.organization.id,
        props.session.user.id,
        props.project,
      ])}
      {...props}
    />
  );
}
