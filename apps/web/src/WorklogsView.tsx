import {useCallback,useEffect,useRef,useState} from 'react';
import {Clock3,Plus,RefreshCw} from 'lucide-react';
import type {Worklog,WorklogPage,WorklogRevisionPage} from '../../../packages/contracts/src';
import {ApiError,query} from './api';
import {useApi,useApiView} from './api-context';
import {formatDateTime,formatNumber,tr,UiError,useLocale} from './i18n';
import {isCapacityWeek} from './planning-model';
import {Empty,ErrorNotice,Field,Loading,Modal} from './ui';
import {WorklogsEditor} from './WorklogsEditor';
import {validWorklog,validWorklogPage,validWorklogRevisions,worklogPagePath,worklogPath,worklogWeekEnd,worklogScope,worklogRecovery,type WorklogMode} from './worklogs-model';
import {useWorklogRead,worklogAccessLost} from './worklogs-read';
import {toolSessionStorage} from './tool-hooks';
import {useLeaveGuard} from './write-action';
import './WorklogsView.css';

const t=(key:string,values?:Record<string,unknown>)=>tr('worklogs.'+key,values);
const day=(date:string)=>formatDateTime(date+'T00:00:00Z',{year:'numeric',month:'short',day:'numeric',timeZone:'UTC'});
const mayNavigate=()=>window.dispatchEvent(new Event('suma:navigate',{cancelable:true}));
type Props={project:string;week:string;onWeek:(value:string)=>boolean;refresh:number;onAccessDenied?:()=>void};
export function WorklogsView(props:Props){
 const api=useApi(),view=useApiView(),caller=useRef({api,epoch:0});if(caller.current.api!==api)caller.current={api,epoch:caller.current.epoch+1};
 const scope=worklogScope(view?.userID??'',view?.organizationID??'',props.project,props.week);
 return <WorklogsScope key={scope+':'+caller.current.epoch} {...props} scope={scope}/>;
}

function WorklogsScope({project,week,onWeek,refresh,scope,onAccessDenied}:Props&{scope:string}){
 useLocale();const api=useApi(),alive=useRef(true),[cursor,setCursor]=useState(''),[editing,setEditing]=useState<{mode:WorklogMode;row?:Worklog;authority:WorklogPage}>(),[history,setHistory]=useState<Worklog>(),[denied,setDenied]=useState(false),[weekIssue,setWeekIssue]=useState(''),[recoveryError,setRecoveryError]=useState<unknown>(''),[restoring,setRestoring]=useState(false);
 const clearPrivate=useCallback(()=>{setDenied(true);setEditing(undefined);setHistory(undefined);onAccessDenied?.();},[onAccessDenied]);
 const validate=useCallback((value:unknown):value is WorklogPage=>validWorklogPage(value,project),[project]);
 const read=useWorklogRead(isCapacityWeek(week)?worklogPagePath(project,week,cursor):undefined,validate,refresh,clearPrivate);
 const page=denied?undefined:read.value,stale=read.error instanceof ApiError&&read.error.status===409;
 const recovery=worklogRecovery(toolSessionStorage(),scope,project);
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;};},[]);
 useLeaveGuard(!!recovery,setRecoveryError);
 async function restore(){
  if(!page||!recovery||'invalid' in recovery||restoring)return;setRestoring(true);setRecoveryError('');
  try{let row:Worklog|undefined;if(recovery.log){const value=await api<unknown>(worklogPath(recovery.log)+query(project));if(!validWorklog(value,project,recovery.log))throw new UiError('common.responseUnknown');row=value;}
   if(alive.current)setEditing({mode:recovery.mode,row,authority:page});
  }catch(error){if(alive.current){setRecoveryError(error);if(worklogAccessLost(error))clearPrivate();}}
  finally{if(alive.current)setRestoring(false);}
 }
 const reload=()=>{if(!mayNavigate())return;setDenied(false);if(cursor)setCursor('');else void read.read();};
 const changeWeek=(event:{currentTarget:HTMLInputElement})=>{const value=event.currentTarget.value;if(value===week)return;if(!isCapacityWeek(value)){event.currentTarget.value=week;setWeekIssue('invalid_week');return;}if(!onWeek(value)){event.currentTarget.value=week;setWeekIssue('week_blocked');return;}setWeekIssue('');};
 return <section className="panel worklogs-view" aria-label={t('title')}>
  <div className="panel-heading"><div><h2><Clock3 size={18} aria-hidden="true"/>{t('title')}</h2><p>{t('explainer')}</p></div><div className="worklogs-heading-actions"><Field label={t('week')}><input aria-label={t('week')} name="worklog-week" type="date" value={week} onInput={changeWeek} onChange={changeWeek}/></Field><button type="button" aria-label={t('refresh')} disabled={read.loading} onClick={reload}><RefreshCw size={15} aria-hidden="true"/></button>{page?.can_create&&!recovery&&<button type="button" className="primary" data-worklog-new onClick={()=>setEditing({mode:'create',authority:page})}><Plus size={15} aria-hidden="true"/>{t('new')}</button>}</div></div>
  {page&&recovery&&('invalid' in recovery?<ErrorNotice error={tr('preferences.write.recovery_invalid')}/>:<p className="warning" role="status">{t('original_pending')}<button type="button" data-worklog-restore disabled={restoring} onClick={()=>void restore()}>{t('restore')}</button></p>)}
  {!!recoveryError&&<ErrorNotice error={recoveryError}/>}
  {(weekIssue||!isCapacityWeek(week))&&<ErrorNotice error={t(weekIssue||'invalid_week')}/>}
  {!!read.error&&<ErrorNotice error={read.error} onRetry={reload}/>}
  {stale&&<p className="warning" role="status">{t('stale_scope')}<button type="button" data-worklog-reset onClick={reload}>{t('refresh_first')}</button></p>}
  {denied&&<p className="warning" role="status">{t('access_changed')}</p>}
  {read.loading&&<Loading/>}
  {page&&<>
   <div className="worklogs-totals" aria-label={t('totals')}><span><strong>{formatNumber(page.summary.total_minutes)}</strong>{t('total_minutes')}</span><span><strong>{formatNumber(page.summary.active_count)}</strong>{t('active_count')}</span><span><strong>{formatNumber(page.summary.voided_count)}</strong>{t('voided_count')}</span><small>{t('week_range',{from:day(week),to:day(worklogWeekEnd(week))})}</small></div>
   {!page.can_create&&<p className="worklogs-help">{t('read_only')}</p>}
   {page.items.length?<div className="table-scroll"><table className="worklogs-table"><thead><tr><th>{t('date')}</th><th>{t('member')}</th><th>{t('task')}</th><th>{t('minutes')}</th><th>{t('note')}</th><th>{t('status')}</th><th>{t('actions')}</th></tr></thead><tbody>{page.items.map(row=><tr key={row.id} className={row.status==='voided'?'worklogs-voided':''}>
    <td>{day(row.work_date)}</td><td>{row.member_name||row.member_id}<small className="cell-sub">{row.user_name||row.user_id}</small></td><td>{row.task_title||row.task_id}<small className="cell-sub">{t('task_version',{version:formatNumber(row.task_version)})}</small></td><td>{t('minutes_value',{minutes:formatNumber(row.minutes)})}</td><td className="worklogs-note">{row.note||'—'}</td><td><span className={'worklogs-status '+row.status}>{t(row.status)}</span><small className="cell-sub">{t('manual_source')}</small></td><td><div className="worklogs-row-actions"><button type="button" data-worklog-history={row.id} onClick={()=>setHistory(row)}>{t('history')}</button>{!recovery&&row.status==='active'&&row.can_correct&&<button type="button" data-worklog-correct={row.id} onClick={()=>setEditing({mode:'correct',row,authority:page})}>{t('correct')}</button>}{!recovery&&row.status==='active'&&row.can_void&&<button type="button" data-worklog-void={row.id} onClick={()=>setEditing({mode:'void',row,authority:page})}>{t('void')}</button>}</div></td>
   </tr>)}</tbody></table></div>:<Empty title={t('empty')} description={t('empty_help')}/>}
   <div className="planning-coverage"><span>{t('page_scope',{loaded:formatNumber(page.items.length),total:formatNumber(page.total)})}</span><div className="inline">{cursor&&<button type="button" onClick={()=>setCursor('')}>{t('first_page')}</button>}{page.next_cursor&&<button type="button" data-worklog-next onClick={()=>setCursor(page.next_cursor)}>{t('next_page')}</button>}</div></div>
  </>}
  <Modal open={!!editing&&!denied} title={t(editing?.mode==='correct'?'correct_title':editing?.mode==='void'?'void_title':'new_title')} description={t('manual_help')} onClose={()=>{if(mayNavigate())setEditing(undefined);}}>{editing&&<WorklogsEditor project={project} week={week} mode={editing.mode} row={editing.row} authority={editing.authority} onDenied={clearPrivate} onCancel={()=>setEditing(undefined)} onSaved={()=>{setEditing(undefined);if(cursor)setCursor('');else void read.read();}}/>}</Modal>
  <Modal open={!!history&&!denied} title={t('history_title')} description={t('history_help')} onClose={()=>setHistory(undefined)}>{history&&<WorklogHistory key={history.id} project={project} row={history} onDenied={clearPrivate}/>}</Modal>
 </section>;
}

function WorklogHistory({project,row,onDenied}:{project:string;row:Worklog;onDenied:()=>void}){
 useLocale();const [cursor,setCursor]=useState(''),validate=useCallback((value:unknown):value is WorklogRevisionPage=>validWorklogRevisions(value,project,row.id),[project,row.id]);
 const read=useWorklogRead(worklogPath(row.id,'revisions')+query(project)+'&limit=20'+(cursor?'&cursor='+encodeURIComponent(cursor):''),validate,0,onDenied),stale=read.error instanceof ApiError&&read.error.status===409;
 const reload=()=>{if(cursor)setCursor('');else void read.read();};
 return <div className="worklogs-history">{!!read.error&&<ErrorNotice error={read.error} onRetry={reload}/>} {stale&&<p className="warning">{t('stale_scope')}</p>}{read.loading&&<Loading/>}{read.value&&<><p className="worklogs-help">{t('history_scope',{loaded:formatNumber(read.value.items.length),total:formatNumber(read.value.total)})}</p>{read.value.items.map(revision=><article className="worklogs-revision" key={revision.version}><div><strong>{t('head_version',{version:formatNumber(revision.version)})}</strong><span className={'worklogs-status '+revision.status}>{t(revision.status)}</span></div><dl><dt>{t('member')}</dt><dd>{revision.member_name||revision.member_id}</dd><dt>{t('task')}</dt><dd>{revision.task_title||revision.task_id} · {t('task_version',{version:formatNumber(revision.task_version)})}</dd><dt>{t('date')}</dt><dd>{day(revision.work_date)}</dd><dt>{t('minutes')}</dt><dd>{formatNumber(revision.minutes)}</dd><dt>{t('note')}</dt><dd>{revision.note||'—'}</dd><dt>{t('reason')}</dt><dd>{revision.reason||'—'}</dd><dt>{t('recorded_by')}</dt><dd>{revision.actor_name||revision.actor_id} · {formatDateTime(revision.updated_at)}{revision.on_behalf_ack&&<small className="cell-sub">{t('on_behalf_recorded')}</small>}</dd></dl></article>)}<div className="inline">{cursor&&<button type="button" onClick={()=>setCursor('')}>{t('first_page')}</button>}{read.value.next_cursor&&<button type="button" onClick={()=>setCursor(read.value!.next_cursor)}>{t('next_page')}</button>}</div></>}</div>;
}
