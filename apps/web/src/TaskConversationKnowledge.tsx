import {useEffect,useRef,useState} from 'react';
import {BookOpen} from 'lucide-react';
import type {TaskConversationKnowledgeReference} from '../../../packages/contracts/src';
import {ApiError} from './api';
import {useApi} from './api-context';
import {formatNumber,tr,UiError,useLocale} from './i18n';
import {ErrorNotice,Loading,Modal} from './ui';
import {KnowledgeRevisionFacts} from './KnowledgeFacts';
import type {KnowledgeRevision} from './knowledge-model';
import {knowledgeRevisionPath,validKnowledgeRevisionDetail} from './task-conversation-knowledge-model';
import {ProjectKnowledgePicker,type ProjectKnowledgePickerProps} from './ProjectKnowledgePicker';
import './TaskConversationKnowledge.css';

const t=(key:string,values?:Record<string,unknown>)=>tr('task-conversation-knowledge.'+key,values);
const refused=(error:unknown)=>error instanceof ApiError&&[401,403].includes(error.status);

export function TaskConversationKnowledge(props:ProjectKnowledgePickerProps){return <ProjectKnowledgePicker {...props}/>;}

/** The recorded title is immutable metadata. Opening it never adopts a newer
 * publication or grants permission to consume the historical revision again. */
type RecordedProps = {project: string; items?: TaskConversationKnowledgeReference[]; refresh: number; onAccessDenied: () => void};
export function RecordedConversationKnowledge(props: RecordedProps) {
  const api = useApi(), caller = useRef({api, epoch: 0});
  if (caller.current.api !== api) caller.current = {api, epoch: caller.current.epoch + 1};
  return <RecordedKnowledgeScope key={JSON.stringify([props.project, caller.current.epoch])} {...props}/>;
}
function RecordedKnowledgeScope({project, items, refresh, onAccessDenied}: RecordedProps) {
  useLocale(); const api = useApi();
  const [selected, setSelected] = useState<TaskConversationKnowledgeReference>();
  const [state, setState] = useState<{key: string; revision?: KnowledgeRevision; error?: unknown}>();
  const [retry, setRetry] = useState(0), [denied, setDenied] = useState(false), alive = useRef(true);
  const key = JSON.stringify([project, selected, refresh]), current = useRef(key); current.current = key;
  useEffect(() => {alive.current = true; return () => {alive.current = false;};}, []);
  useEffect(() => {
    if (!selected) return;
    const abort = new AbortController();setState(undefined);
    void api<unknown>(knowledgeRevisionPath(project, selected), undefined, 'GET', undefined, abort.signal).then(value => {
      if (!alive.current || abort.signal.aborted || current.current !== key) return;
      if (!validKnowledgeRevisionDetail(value, project, selected)) throw new UiError('task-conversation-knowledge.invalid_read');
      setState({key, revision: value.snapshot});
    }).catch(error => {
      if (!alive.current || abort.signal.aborted || current.current !== key) return;
      setState({key, error});if (refused(error)) {setSelected(undefined);setDenied(true);onAccessDenied();}
    });
    return () => abort.abort();
  }, [api, project, key, retry, onAccessDenied]);
  if (denied || !items?.length) return null;
  const value = state?.key === key ? state : undefined;
  return <div className="conversation-knowledge-recorded"><small>{t('recorded')}</small><ul aria-label={t('recorded')}>
    {items.map(pin => <li key={pin.id}><button type="button" data-knowledge-history={pin.id} onClick={() => setSelected(pin)}><BookOpen size={13} aria-hidden="true"/><span>{pin.title_at_post || pin.id} · {tr('knowledge.kind.' + pin.kind_at_post)} · {t('revision', {revision: formatNumber(pin.revision)})}</span></button></li>)}
  </ul><Modal open={!!selected} onClose={() => {setSelected(undefined);setState(undefined);}} title={selected?.title_at_post || t('history')} description={t('history_hint')}>
    {selected && <><p>{!value?.revision && <>{t('revision', {revision: formatNumber(selected.revision)})} · </>}<code>{selected.id}</code></p>{!value?.revision && <code className="knowledge-hash">{selected.sha256}</code>}
      {!value && <Loading/>}{!!value?.error && <ErrorNotice error={value.error} onRetry={() => setRetry(number => number + 1)}/>}
      {value?.revision && <div onClickCapture={event => {if (event.target instanceof Element && event.target.closest('a[href]') && !window.dispatchEvent(new Event('suma:navigate', {cancelable: true}))) event.preventDefault();}}><KnowledgeRevisionFacts project={project} revision={value.revision}/></div>}</>}
  </Modal></div>;
}
