import {lazy,Suspense,type ReactNode} from "react";
import { SchedulingQueue } from "./SchedulingQueue";
import type { QueueRoute } from "./scheduling-model";
import type {Session} from './api';
import {Loading} from './ui';
import {ErrorBoundary} from './ErrorBoundary';
import {tr as t,useLocale} from './i18n';
import './WorkBuddyProgressWorkspace.css';
const WorkBuddyProgressWorkspace=lazy(()=>import('./WorkBuddyProgressWorkspace').then(module=>({default:module.WorkBuddyProgressWorkspace})));
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
  queueRoute,
  queueRun,
  invalidQueue,
  onQueueScope,
  workBuddyFilter,
  invalidWorkBuddyFilter,
  onWorkBuddyFilter,
  workBuddyTask,
  invalidWorkBuddyTask,
  onWorkBuddyTask,
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
  onRun: (run: string, node: string, attempt?: number) => void;
  onPlan: (id: string) => void;
  onTasks: () => void;
  queueRoute?: QueueRoute;
  queueRun?: string;
  invalidQueue?: boolean;
  onQueueScope?: (route?: QueueRoute, run?: string) => void;
  workBuddyFilter?: 'attention';
  invalidWorkBuddyFilter?: boolean;
  onWorkBuddyFilter?: (filter?: 'attention') => void;
  workBuddyTask?: string;
  invalidWorkBuddyTask?: boolean;
  onWorkBuddyTask?: (task?: string) => void;
  team?: string;
  invalidTeam?: boolean;
  onTeam?: (id: string) => void;
  children: ReactNode;
}) {
  useLocale();
  return (
    <>
      <div className="segmented execution-workspace-tabs" aria-label={t('workspace.execution.view_label')}>
        <button
          aria-pressed={!["queue","team","workbuddy"].includes(layout)}
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
        <button data-workbuddy-progress-tab aria-pressed={layout === 'workbuddy'} onClick={()=>onLayout('workbuddy')}>
          {t('workbuddy-progress.tab')}
        </button>
      </div>
      {layout === 'team' ? <ErrorBoundary key={JSON.stringify([session.user.id,session.organization.id,project])} inline><Suspense fallback={<Loading/>}><TeamExecutionWorkspace session={session} project={project} refresh={refresh} panel={panel} team={team} invalidTeam={invalidTeam} onTeam={onTeam} onPlan={onPlan} onTasks={onTasks}/></Suspense></ErrorBoundary> : layout === "queue" ? (
        <SchedulingQueue project={project} refresh={refresh} onRun={onRun} queueRoute={queueRoute} queueRun={queueRun} invalidQueue={invalidQueue} onQueueScope={onQueueScope} />
      ) : layout === 'workbuddy' ? (
        <ErrorBoundary key={JSON.stringify([session.user.id,session.organization.id,project])} inline><Suspense fallback={<Loading/>}><WorkBuddyProgressWorkspace session={session} project={project} refresh={refresh} filter={workBuddyFilter} invalidFilter={invalidWorkBuddyFilter} onFilter={onWorkBuddyFilter} task={workBuddyTask} invalidTask={invalidWorkBuddyTask} onTask={onWorkBuddyTask} onRun={onRun}/></Suspense></ErrorBoundary>
      ) : (
        children
      )}
    </>
  );
}
