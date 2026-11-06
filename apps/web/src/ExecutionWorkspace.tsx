import {lazy,Suspense,type ReactNode} from "react";
import { SchedulingQueue } from "./SchedulingQueue";
import type {Session} from './api';
import {Loading} from './ui';
import {ErrorBoundary} from './ErrorBoundary';
import {tr as t,useLocale} from './i18n';
const TeamExecutionWorkspace=lazy(()=>import('./TeamExecutionWorkspace').then(module=>({default:module.TeamExecutionWorkspace})));
export function ExecutionWorkspace({
  project,
  session,
  panel,
  refresh,
  layout,
  onLayout,
  onRun,
  onPlan,
  onTasks,
  team,
  invalidTeam,
  onTeam,
  children,
}: {
  project: string;
  session: Session;
  panel: string;
  refresh: number;
  layout: string;
  onLayout: (value: string) => void;
  onRun: (run: string, node: string) => void;
  onPlan: (id: string) => void;
  onTasks: () => void;
  team?: string;
  invalidTeam?: boolean;
  onTeam?: (id: string) => void;
  children: ReactNode;
}) {
  useLocale();
  return (
    <>
      <div className="segmented" aria-label={t('workspace.execution.view_label')}>
        <button
          aria-pressed={layout !== "queue"&&layout !== "team"}
          onClick={() => onLayout("list")}
        >
          {t('workspace.execution.runs')}
        </button>
        <button
          aria-pressed={layout === "queue"}
          onClick={() => onLayout("queue")}
        >
          {t('workspace.queue.title')}
        </button>
        <button aria-pressed={layout === 'team'} onClick={()=>onLayout('team')}>
          {t('team-execution.tab')}
        </button>
      </div>
      {layout === 'team' ? <ErrorBoundary key={JSON.stringify([session.user.id,session.organization.id,project])} inline><Suspense fallback={<Loading/>}><TeamExecutionWorkspace session={session} project={project} refresh={refresh} panel={panel} team={team} invalidTeam={invalidTeam} onTeam={onTeam} onPlan={onPlan} onTasks={onTasks}/></Suspense></ErrorBoundary> : layout === "queue" ? (
        <SchedulingQueue project={project} refresh={refresh} onRun={onRun} />
      ) : (
        children
      )}
    </>
  );
}
