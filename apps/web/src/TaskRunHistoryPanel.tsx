import {useCallback,useId,useState} from 'react';
import {ChevronDown,History} from 'lucide-react';
import type {TaskRunProvenancePage} from '../../../packages/contracts/src';
import {ApiError,query,type Session} from './api';
import {useApi} from './api-context';
import {formatDateTime,formatNumber,tr,useLocale} from './i18n';
import {useCompositionPage} from './candidate-composition-hooks';
import {repairTaskURL} from './candidate-conflict-repair-model';
import {taskRunHistoryKey,validTaskRunHistory} from './task-run-history-model';
import {ErrorNotice,Loading} from './ui';
import {toolError} from './tool-hooks';
import './CandidateCompositionPanel.css';
const t=(key:string,values?:Record<string,unknown>)=>tr('task-run-history.'+key,values);
const stamp=(value:string)=>formatDateTime(value,{year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'});
type Props={session:Session;project:string;taskID:string;refresh?:number};
export function TaskRunHistoryPanel(props:Props){return <HistoryScope key={JSON.stringify([props.session.user.id,props.session.organization.id,props.project,props.taskID])} {...props}/>;}
function HistoryScope({project,taskID,refresh=0}:Props){
 useLocale();const api=useApi(),bodyID=useId(),[open,setOpen]=useState(false),[tick,setTick]=useState(0),[forbidden,setForbidden]=useState<unknown>();
 const valid=useCallback((value:unknown):value is TaskRunProvenancePage=>validTaskRunHistory(value,taskID),[taskID]);
 const failed=useCallback((error:unknown)=>{if(error instanceof ApiError&&[401,403].includes(error.status))setForbidden(error);},[]);
 const history=useCompositionPage(api,'/tasks/'+encodeURIComponent(taskID)+'/run-history'+query(project)+'&limit=20',open&&!forbidden,refresh+tick,valid,failed,'before',taskRunHistoryKey);
 return <section className="candidate-composition task-run-history"><button type="button" data-task-run-history-toggle className="composition-toggle" aria-expanded={open} aria-controls={bodyID} onClick={()=>setOpen(value=>!value)}><History size={16}/><span>{t('title')}</span><ChevronDown size={16}/></button>{open&&<div id={bodyID} className="composition-body"><p className="composition-muted">{t('intro')}</p>{Boolean(forbidden)?<ErrorNotice error={toolError(forbidden)} onRetry={()=>{setForbidden(undefined);setTick(value=>value+1);}}/>:<><button data-task-run-history-refresh type="button" disabled={history.loading} onClick={()=>setTick(value=>value+1)}>{t('refresh')}</button>{history.loading&&<Loading/>}{history.error&&<ErrorNotice error={toolError(history.error)} onRetry={()=>void history.read()}/>}{history.value&&!history.value.items.length&&<p>{t('empty')}</p>}<ol className="composition-run-history">{history.value?.items.map(item=><li key={item.run_id} data-task-run-history={item.run_id}><div className="composition-actions"><a href={'?'+new URLSearchParams({project,view:'development',sub:'runs',run:item.run_id})}>{item.run_id}</a><strong>{t(item.current?'current':'earlier')}</strong></div><p className="composition-muted">{t('kind_'+item.binding_kind)}</p><dl className="composition-facts">{item.task_version!==null&&<><dt>{t(item.binding_kind==='bound'?'task_version':'recorded_task_version')}</dt><dd>v{formatNumber(item.task_version)}</dd></>}{item.bound_at!==null&&<><dt>{t('bound_at')}</dt><dd>{stamp(item.bound_at)}</dd></>}<dt>{t('observed_at')}</dt><dd>{stamp(item.observed_at)}</dd></dl>{item.bound_at===null&&item.task_version===null&&<p className="composition-muted">{t('unrecorded')}</p>}{item.plan_id&&<a data-task-run-plan href={repairTaskURL(project,taskID,item.plan_id)}>{t('open_plan')}</a>}</li>)}</ol>{history.value?.next_cursor&&<button data-task-run-history-more type="button" disabled={history.loading} onClick={()=>void history.read(true)}>{t('more')}</button>}</>}</div>}</section>;
}
