import { useEffect, useRef } from "react";
import { query, stateLabel } from "./api";
import { ErrorNotice, Loading } from "./ui";
import { useToolRead } from "./tool-hooks";
import { useSchedulingPage } from "./scheduling-hooks";
import { AdmissionFacts, AttemptFacts } from "./SchedulingFacts";
import {
  admissionCurrent,
  executionHref,
  parseAttempt,
  type NodeAdmission,
  type ExecutionAttempt,
} from "./scheduling-model";
import "./scheduling.css";
export function NodeSchedulingFacts({
  project,
  run,
  node,
  runVersion,
  currentAttempt,
  nodeState,
  nodeFence,
  selectedAttempt,
  onAttempt,
}: {
  project: string;
  run: string;
  node: string;
  runVersion: number;
  currentAttempt: number;
  nodeState?: string;
  nodeFence?: number;
  selectedAttempt?: string;
  onAttempt: (value: string) => void;
}) {
  const detailRoot = useRef<HTMLDivElement>(null),
    listRoot = useRef<HTMLElement>(null);
  const base =
      "/runs/" + encodeURIComponent(run) + "/nodes/" + encodeURIComponent(node),
    snapshotKey = [runVersion, currentAttempt, nodeFence, nodeState].join(":"),
    diagnostic = useToolRead<NodeAdmission>(
      base + "/scheduling" + query(project),
      runVersion,
    ),
    page = useSchedulingPage<ExecutionAttempt>(
      base + "/attempts" + query(project),
      (item) => String(item.attempt),
      snapshotKey,
      true,
    ),
    attempt = parseAttempt(selectedAttempt || ""),
    exact = useToolRead<ExecutionAttempt>(
      attempt ? base + "/attempts/" + attempt + query(project) : "",
      snapshotKey,
    );
  useEffect(() => {
    const timer = setInterval(() => void diagnostic.read(), 10000);
    return () => clearInterval(timer);
  }, [diagnostic.read]);
  useEffect(() => {
    if (attempt && exact.data?.attempt === attempt) {
      detailRoot.current?.scrollIntoView?.({
        block: "start",
        behavior: "auto",
      });
      detailRoot.current?.focus({ preventScroll: true });
    }
  }, [attempt, exact.data?.attempt]);
  const current =
      diagnostic.data &&
      admissionCurrent(diagnostic.data, run, node, runVersion),
    record = exact.data;
  return (
    <div className="node-scheduling-facts">
      {diagnostic.error && (
        <>
          <ErrorNotice
            error={diagnostic.error}
            onRetry={() => void diagnostic.read()}
          />
          <p className="warning">
            当前派发条件未确认，不以旧数据宣称仍受阻或可执行。
          </p>
        </>
      )}
      {!diagnostic.error && current ? (
        <AdmissionFacts snapshot={diagnostic.data!} project={project} />
      ) : !diagnostic.error ? (
        <p role="status">
          {diagnostic.loading
            ? "正在读取当前派发条件…"
            : "诊断依据早于当前运行，请重新读取。"}
        </p>
      ) : null}
      <button
        disabled={diagnostic.loading}
        onClick={() => void diagnostic.read()}
      >
        重新核对当前条件
      </button>
      <section
        ref={listRoot}
        tabIndex={-1}
        className="scheduling-attempts"
        aria-label="持久尝试历史"
      >
        <h4>已发生的执行</h4>
        <p className="muted">尝试来自持久表，独立于当前阻塞与最近事件窗口。</p>
        <button
          disabled={page.loading}
          onClick={() => void page.read("", true)}
        >
          重新读取尝试历史
        </button>
        {page.loading && page.items.length > 0 && (
          <p role="status">正在更新尝试历史，保留上次读取内容。</p>
        )}
        {page.error && (
          <p className="warning">
            历史读取未更新；以下如有记录是上次已读内容。
          </p>
        )}
        {page.error && (
          <ErrorNotice error={page.error} onRetry={() => void page.read()} />
        )}
        <ul>
          {page.items.map((item) => (
            <li key={item.attempt}>
              <a
                className="scheduling-open-link"
                aria-current={attempt === item.attempt ? "page" : undefined}
                href={executionHref(project, run, node, item.attempt)}
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
                  onAttempt(String(item.attempt));
                }}
              >
                尝试 {item.attempt} · {stateLabel(item.state)} · fence{" "}
                {item.fence}
              </a>
            </li>
          ))}
        </ul>
        {page.loading && !page.items.length && <Loading />}
        {!page.loading && !page.error && !page.items.length && (
          <p>尚无已领取尝试；当前等待不生成假 attempt。</p>
        )}
        <small>
          已加载 {page.items.length} / {page.total} 次正式尝试
        </small>
        {page.next && (
          <button
            disabled={page.loading}
            onClick={() => void page.read(page.next)}
          >
            加载更早尝试
          </button>
        )}
        {attempt && (
          <>
            <button disabled={exact.loading} onClick={() => void exact.read()}>
              重新读取所选尝试
            </button>
            {exact.loading && record && (
              <p role="status">正在更新所选尝试，以下暂留上次已读内容。</p>
            )}
            {exact.error && record && (
              <p className="warning">
                所选尝试尚未重新确认，以下保留上次读取。
              </p>
            )}
            <button
              onClick={() => {
                onAttempt("");
                listRoot.current?.focus({ preventScroll: true });
              }}
            >
              关闭历史尝试详情
            </button>
            {exact.error && (
              <ErrorNotice
                error={exact.error}
                onRetry={() => void exact.read()}
              />
            )}{" "}
            {exact.loading && !record && <Loading />}
            {record &&
            record.run_id === run &&
            record.node_id === node &&
            record.attempt === attempt ? (
              <div ref={detailRoot} tabIndex={-1} aria-label="所选历史尝试详情">
                <AttemptFacts attempt={record} />
              </div>
            ) : !exact.loading && !exact.error ? (
              <p className="warning">
                历史尝试不属于当前选择，未显示其他执行证据。
              </p>
            ) : null}
          </>
        )}
      </section>
    </div>
  );
}
