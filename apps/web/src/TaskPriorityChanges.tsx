import {useEffect,useRef,useState} from 'react';
import {ArrowRight,CheckCircle2,Flag} from 'lucide-react';
import type {TaskPriorityChangeResult} from '../../../packages/contracts/src';
import {ApiError,type RecordItem} from './api';
import {useApi} from './api-context';
import {formatNumber,tr,UiError,useLocale} from './i18n';
import {validOwnerTargets} from './task-owner-changes-model';
import {
  isTaskPriority,maxTaskPriorityVersion,priorityReviewRows,taskPriorityChangePath,validPriorityRecovery,validPriorityResult,
  type PriorityReviewRow,type TaskPriority,
} from './task-priority-changes-model';
import {taskPriorities} from './task-views-model';
import {toolSessionStorage} from './tool-hooks';
import {ErrorNotice,Field,Modal} from './ui';
import {useWriteAction,WriteNotice} from './write-action';
import './TaskPriorityChanges.css';

export type TaskPriorityChangesProps={
  project:string;
  userID:string;
  organizationID:string;
  tasks:RecordItem[];
  disabled?:boolean;
  authorityError?:unknown;
  onChanged:()=>void|Promise<void>;
  onForbidden:(error:unknown)=>void;
  onRefresh?:()=>void;
  onAuthorityRestored?:()=>void;
};

const t=(key:string,values?:Record<string,unknown>)=>tr('task-priority-changes.'+key,values);
const priorityLabel=(value:string)=>isTaskPriority(value)?tr('common.priority.'+value):value||tr('task-views.unspecified');
const isDenied=(status:number|undefined)=>status!==undefined&&[401,403,404].includes(status);

export function TaskPriorityChanges(props:TaskPriorityChangesProps){
  const scope=JSON.stringify(['task-priority-changes',props.userID,props.organizationID,props.project]);
  return <PriorityChangeScope key={scope} {...props} scope={scope}/>;
}

function PriorityChangeScope({
  scope,project,userID,organizationID,tasks,disabled,authorityError,
  onChanged,onForbidden,onRefresh,onAuthorityRestored,
}:TaskPriorityChangesProps&{scope:string}){
  useLocale();
  const api=useApi(),currentApi=useRef(api);
  currentApi.current=api;
  const [open,setOpen]=useState(false),[denied,setDenied]=useState(false);
  const [basis,setBasis]=useState<PriorityReviewRow[]>(),[target,setTarget]=useState<TaskPriority>();
  const [receipt,setReceipt]=useState<TaskPriorityChangeResult>(),[refreshFailed,setRefreshFailed]=useState(false);
  const [checking,setChecking]=useState(false),[accessError,setAccessError]=useState<unknown>();
  const checkController=useRef<AbortController|undefined>(undefined);
  const receiptFocus=useRef<HTMLElement>(null),refreshFocus=useRef<HTMLButtonElement>(null);
  const blocked=useRef(false),writeRetired=useRef(false),alive=useRef(true);
  const authorityBlocked=Boolean(authorityError)||denied;
  blocked.current=authorityBlocked;
  if(authorityBlocked)writeRetired.current=true;

  const action=useWriteAction({
    scope,storage:toolSessionStorage(),
    validateRequest:request=>validPriorityRecovery(request,project),
    validateResponse:(value,request)=>!blocked.current&&!writeRetired.current&&validPriorityResult(value,request,project),
    definitiveRefusalOnRestore:error=>error.status===409&&['version_conflict','idempotency_conflict'].includes(error.code),
  });
  const pending=action.pending,locked=action.busy||!!pending||action.persistenceBlocked;
  const rows=priorityReviewRows(project,tasks);
  const stale=action.conflict||open&&!disabled&&JSON.stringify(rows)!==JSON.stringify(basis);
  const changed=basis?.filter(row=>target&&row.priority!==target)??[];
  const exhausted=changed.some(row=>row.expected_version===maxTaskPriorityVersion);
  const canApply=!!target&&!disabled&&!stale&&!locked&&!authorityBlocked&&!exhausted&&changed.length>0;

  useEffect(()=>{
    alive.current=true;
    return ()=>{alive.current=false;checkController.current?.abort();};
  },[]);
  useEffect(()=>{
    if(!authorityBlocked)return;
    setBasis(undefined);
    setTarget(undefined);
    setReceipt(undefined);
    setRefreshFailed(false);
  },[authorityBlocked]);
  useEffect(()=>{
    if(blocked.current||!isDenied(action.errorStatus))return;
    blocked.current=true;
    writeRetired.current=true;
    setDenied(true);
    onForbidden(new ApiError(action.errorStatus!,'access_changed',t('access_changed')));
  },[action.errorStatus,onForbidden]);
  useEffect(()=>{
    if(!authorityError)return;
    checkController.current?.abort();
    checkController.current=undefined;
    setChecking(false);
    setAccessError(undefined);
  },[authorityError]);
  useEffect(()=>{if(receipt&&!authorityBlocked)receiptFocus.current?.focus();},[receipt,authorityBlocked]);
  useEffect(()=>{if(stale&&!authorityBlocked&&!pending&&!receipt)refreshFocus.current?.focus();},[stale,authorityBlocked,pending?.idempotency_key,receipt]);

  function launch(){
    if(!locked&&!authorityBlocked){
      setBasis(rows);
      setTarget(undefined);
      setReceipt(undefined);
      setRefreshFailed(false);
      action.clearConflict();
    }
    setOpen(true);
  }

  async function checkAccess(){
    if(!authorityBlocked||checking||action.busy)return;
    const abort=new AbortController();
    checkController.current=abort;
    setChecking(true);
    setAccessError(undefined);
    const current=()=>alive.current&&!abort.signal.aborted&&currentApi.current===api&&checkController.current===abort;
    try{
      // The existing owner directory checks project write authority. Its data
      // is discarded. Only the original batch can resume after a user check.
      const headers={'X-Suma-Expected-User':userID,'X-Suma-Expected-Organization':organizationID};
      const path='/task-owner-targets?'+new URLSearchParams({project_id:project,limit:'1'});
      const result=await api<unknown>(path,undefined,'GET',headers,abort.signal);
      if(!current())return;
      if(!validOwnerTargets(result,project,1))throw new UiError('task-priority-changes.access_invalid');
      setDenied(false);
      action.setError('');
      onAuthorityRestored?.();
    }catch(error){
      if(current())setAccessError(error);
    }finally{
      if(current())setChecking(false);
    }
  }

  async function showReceipt(result:TaskPriorityChangeResult|undefined){
    if(!result||blocked.current||writeRetired.current||!alive.current||currentApi.current!==api)return;
    setReceipt(result);
    try{await onChanged();}
    catch{
      if(alive.current&&!blocked.current&&currentApi.current===api)setRefreshFailed(true);
    }
  }

  async function apply(){
    if(!canApply||!target||!basis)return;
    const draft={project_id:project,priority:target,tasks:basis.map(({task_id,expected_version})=>({task_id,expected_version}))};
    writeRetired.current=false;
    await showReceipt(await action.run<TaskPriorityChangeResult>(taskPriorityChangePath,draft,'POST'));
  }

  async function restore(){
    if(!pending||authorityBlocked||action.busy)return;
    writeRetired.current=false;
    await showReceipt(await action.run<TaskPriorityChangeResult>(pending.path,pending.body,pending.method));
  }

  const launchVisible=tasks.length>0||!!pending||action.persistenceBlocked;
  const receiptCount=receipt?.changes.filter(row=>row.changed).length??0;
  return <>
    {launchVisible&&<button type="button" data-task-priority-launch
      disabled={action.busy||!pending&&!action.persistenceBlocked&&(disabled||authorityBlocked)} onClick={launch}>
      <Flag size={16} aria-hidden="true"/>{pending||action.persistenceBlocked?t('recover'):t('launch')}
    </button>}
    <Modal open={open} title={t('title')} description={t('intro')} onClose={()=>{
      if(!checking&&window.dispatchEvent(new Event('suma:navigate',{cancelable:true})))setOpen(false);
    }}>
      <div className="task-priority-change">
        {!authorityBlocked&&<WriteNotice action={action}/>}
        {authorityBlocked?<section className="task-priority-notice">
          <p role="status">{t('access_changed')}</p>
          {Boolean(accessError)&&<ErrorNotice error={accessError}/>}
          <button type="button" data-task-priority-check-access disabled={checking||action.busy} onClick={()=>void checkAccess()}>
            {t(checking?'checking_access':'check_access')}
          </button>
        </section>:pending?<section className="task-priority-notice">
          <strong>{t('original_operation')}</strong>
          <p>{t('recovery_help')}</p>
          <p><code>{pending.idempotency_key}</code></p>
          <button type="button" data-task-priority-restore disabled={action.busy} onClick={()=>void restore()}>
            {t(action.busy?'restoring':'restore')}
          </button>
        </section>:action.persistenceBlocked?<p role="status">{t('storage_help')}</p>:receipt?<section
          className="task-priority-notice task-priority-success" data-task-priority-receipt tabIndex={-1} ref={receiptFocus} aria-live="polite">
          <strong><CheckCircle2 size={18} aria-hidden="true"/>{t('saved')}</strong>
          <p>{t('summary',{changed:formatNumber(receiptCount),unchanged:formatNumber(receipt.changes.length-receiptCount)})}</p>
          {refreshFailed&&<p role="status">{t('refresh_failed')}</p>}
          <details>
            <summary>{t('receipt_details')}</summary>
            <p><code>{receipt.operation_id}</code></p>
            <ul>{receipt.changes.map(row=><li key={row.task_id}>
              <strong>{basis?.find(item=>item.task_id===row.task_id)?.title||row.task_id}</strong>
              <PriorityTransition before={row.previous_priority} after={row.changed?row.priority:undefined}/>
            </li>)}</ul>
          </details>
        </section>:<>
          <Field label={t('target')}>
            <select name="task_priority_target" data-task-priority-target value={target??''}
              disabled={disabled||stale||!basis} onChange={event=>{
                const value=event.target.value;
                setTarget(isTaskPriority(value)?value:undefined);
              }}>
              <option value="">{t('choose')}</option>
              {taskPriorities.map(priority=><option key={priority} value={priority}>{priorityLabel(priority)}</option>)}
            </select>
          </Field>
          {stale?<section className="task-priority-notice">
            <p role="status">{t('stale')}</p>
            <button type="button" data-task-priority-refresh ref={refreshFocus} onClick={()=>{
              action.clearConflict();setOpen(false);onRefresh?.();
            }}>{t('refresh')}</button>
          </section>:!basis?<p role="status">{t('incomplete')}</p>:<>
            <div className="task-priority-review">
              <h3>{t('selection',{count:formatNumber(basis.length)})}</h3>
              {target&&<span role="status">{t('summary',{changed:formatNumber(changed.length),unchanged:formatNumber(basis.length-changed.length)})}</span>}
            </div>
            <ul className="task-priority-rows" aria-label={t('tasks')} tabIndex={0}>
              {basis.map(row=>{
                const changes=!!target&&row.priority!==target;
                return <li key={row.task_id} data-task-priority-row={row.task_id}>
                  <div className="task-priority-review">
                    <strong>{row.title}</strong>
                    {target&&<span className={changes?'task-priority-delta':undefined}>{t(changes?'will_change':'unchanged')}</span>}
                  </div>
                  <PriorityTransition before={row.priority} after={changes?target:undefined}/>
                </li>;
              })}
            </ul>
            {target&&changed.length===0&&<p role="status">{t('no_changes')}</p>}
            {exhausted&&<p className="task-priority-notice" role="status">{t('version_exhausted')}</p>}
          </>}
          <div className="task-priority-actions">
            <button type="button" className="primary" data-task-priority-apply disabled={!canApply} onClick={()=>void apply()}>
              {changed.length?t('apply',{count:formatNumber(changed.length)}):t('apply_empty')}
            </button>
          </div>
        </>}
      </div>
    </Modal>
  </>;
}

function PriorityTransition({before,after}:{before:string;after?:string}){
  return <div className="task-priority-transfer">
    <span className={isTaskPriority(before)?'priority '+before:'priority'}>{priorityLabel(before)}</span>
    {after&&<>
      <ArrowRight size={14} aria-hidden="true"/><span className="sr-only">{t('to')}</span>
      <span className={'priority '+after}>{priorityLabel(after)}</span>
    </>}
  </div>;
}
