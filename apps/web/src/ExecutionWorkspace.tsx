import type { ReactNode } from "react";
import { SchedulingQueue } from "./SchedulingQueue";
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
  return (
    <>
      <div className="segmented" aria-label="执行记录视图">
        <button
          aria-pressed={layout !== "queue"}
          onClick={() => onLayout("list")}
        >
          运行记录
        </button>
        <button
          aria-pressed={layout === "queue"}
          onClick={() => onLayout("queue")}
        >
          就绪队列
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
