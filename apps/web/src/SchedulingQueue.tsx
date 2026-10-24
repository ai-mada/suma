import { RefreshCw } from "lucide-react";
import { query, time } from "./api";
import { ErrorNotice, Loading } from "./ui";
import { AdmissionFacts } from "./SchedulingFacts";
import { executionHref, type ReadyEntry } from "./scheduling-model";
import { useSchedulingPage } from "./scheduling-hooks";
import "./scheduling.css";
export function SchedulingQueue({
  project,
  refresh,
  onRun,
}: {
  project: string;
  refresh: number;
  onRun: (run: string, node: string) => void;
}) {
  const page = useSchedulingPage<ReadyEntry>(
    "/scheduling/queue" + query(project),
    (item) => item.run_id + "\0" + item.node_id,
    refresh,
  );
  return (
    <section className="panel scheduling-workspace" aria-label="真实就绪队列">
      <div className="panel-heading">
        <div>
          <h2>就绪队列</h2>
          <p>查看等待派发的节点、阻塞原因和运行详情。</p>
        </div>
        <button disabled={page.loading} onClick={() => void page.read()}>
          <RefreshCw size={15} />
          重新读取队列
        </button>
      </div>
      {page.error && (
        <>
          <ErrorNotice error={page.error} onRetry={() => void page.read()} />
          <p className="warning">
            当前队列未确认；以下如有记录，仅为上次成功读取。
          </p>
        </>
      )}
      {page.loading && !page.items.length ? (
        <Loading />
      ) : page.items.length ? (
        <ul className="scheduling-queue">
          {page.items.map((item) => (
            <li key={item.run_id + "\0" + item.node_id}>
              <article>
                <header>
                  <div>
                    <h3>{item.node_name || item.node_id}</h3>
                    <p>
                      {item.workflow_name || item.run_id} · {item.type}
                    </p>
                  </div>
                  <span className="badge">等待派发</span>
                </header>
                <p className="muted">
                  就绪于 {time(item.ready_at)} · 已发生尝试 {item.attempt}
                </p>
                <a
                  className="scheduling-open-link"
                  href={executionHref(project, item.run_id, item.node_id)}
                  onClick={(event) => {
                    if (
                      event.defaultPrevented ||
                      event.button !== 0 ||
                      event.metaKey ||
                      event.ctrlKey ||
                      event.shiftKey ||
                      event.altKey
                    )
                      return;
                    event.preventDefault();
                    onRun(item.run_id, item.node_id);
                  }}
                >
                  查看真实运行与节点 <code>{item.node_id}</code>
                </a>
                {item.admission && !page.error ? (
                  <AdmissionFacts snapshot={item.admission} project={project} />
                ) : (
                  <p className="muted">
                    当前调度条件尚未确认，请进入节点读取。记录在读取后可能已被领取。
                  </p>
                )}
                <p className="scheduling-queue-note">
                  当前判断用于下一次派发，历史尝试见运行详情。
                </p>
              </article>
            </li>
          ))}
        </ul>
      ) : !page.error && !page.loading ? (
        <p>本次读取没有就绪节点。等待依赖、审批或计时的节点不在此队列中。</p>
      ) : null}
      <p className="loaded-scope">
        已显示 {page.items.length} / {page.total} 个待派发节点。
      </p>
      {page.next && (
        <button
          disabled={page.loading}
          onClick={() => void page.read(page.next)}
        >
          加载更多就绪节点
        </button>
      )}
    </section>
  );
}
