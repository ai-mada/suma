import { RefreshCw } from "lucide-react";
import { query, time } from "./api";
import { ErrorNotice, Loading, Empty } from "./ui";
import {tr as t,useLocale,formatNumber} from './i18n';
import {Trans} from './i18n/Trans';
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
  useLocale();
  const page = useSchedulingPage<ReadyEntry>(
    project?"/scheduling/queue" + query(project):'',
    (item) => item.run_id + "\0" + item.node_id,
    refresh,
  );
  if(!project)return <Empty title={t('workspace.select_project')}/>;
  return (
    <section className="panel scheduling-workspace" aria-label={t('workspace.queue.label')}>
      <div className="panel-heading">
        <div>
          <h2>{t('workspace.queue.title')}</h2>
          <p>{t('workspace.queue.description')}</p>
        </div>
        <button type="button" aria-disabled={page.loading} onClick={() => {
          if (!page.loading) void (page.error ? page.retry() : page.read());
        }}>
          <RefreshCw size={15} aria-hidden="true" />
          {t(page.error ? 'workspace.queue.retry' : 'workspace.queue.reload')}
        </button>
      </div>
      {page.error && (
        <>
          <ErrorNotice error={page.failure||page.error} />
          {page.hasSnapshot && <p className="warning">{t('workspace.queue.unconfirmed')}</p>}
        </>
      )}
      {page.loading && page.hasSnapshot && <p role="status">{t('workspace.queue.refreshing')}</p>}
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
                  <span className="badge">{t('workspace.queue.waiting')}</span>
                </header>
                <p className="muted">
                  {t('workspace.queue.ready_at',{at:time(item.ready_at),attempts:formatNumber(item.attempt)})}
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
                  <Trans i18nKey="workspace.queue.open_run" values={{node:item.node_id}} components={{code:<code/>}}/>
                </a>
                {item.admission && !page.error ? (
                  <AdmissionFacts snapshot={item.admission} project={project} />
                ) : (
                  <p className="muted">
                    {t('workspace.queue.conditions_unknown')}
                  </p>
                )}
                <p className="scheduling-queue-note">
                  {t('workspace.queue.next_dispatch')}
                </p>
              </article>
            </li>
          ))}
        </ul>
      ) : page.hasSnapshot && !page.error && !page.loading ? (
        <p>{t('workspace.queue.empty')}</p>
      ) : null}
      {page.hasSnapshot && <p className="loaded-scope">
        {t('workspace.queue.loaded',{loaded:formatNumber(page.items.length),total:formatNumber(page.total)})}
      </p>}
      {page.next && !page.error && (
        <button
          disabled={page.loading}
          onClick={() => void page.read(page.next)}
        >
          {t('workspace.queue.more')}
        </button>
      )}
    </section>
  );
}
