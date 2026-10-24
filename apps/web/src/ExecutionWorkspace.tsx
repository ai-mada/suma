import type { ReactNode } from "react";
import { SchedulingQueue } from "./SchedulingQueue";
import {tr as t,useLocale} from './i18n';
export function ExecutionWorkspace({
  project,
  refresh,
  layout,
  onLayout,
  onRun,
  children,
}: {
  project: string;
  refresh: number;
  layout: string;
  onLayout: (value: string) => void;
  onRun: (run: string, node: string) => void;
  children: ReactNode;
}) {
  useLocale();
  return (
    <>
      <div className="segmented" aria-label={t('workspace.execution.view_label')}>
        <button
          aria-pressed={layout !== "queue"}
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
      </div>
      {layout === "queue" ? (
        <SchedulingQueue project={project} refresh={refresh} onRun={onRun} />
      ) : (
        children
      )}
    </>
  );
}
