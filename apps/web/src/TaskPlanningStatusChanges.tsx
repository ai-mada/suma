import {useEffect,useRef,useState} from 'react';
import {ArrowRight,CheckCircle2,CircleAlert,ListChecks,LoaderCircle} from 'lucide-react';
import type {TaskPlanningStatusChangeResult,TaskPlanningStatusPreview,TaskPlanningStatusPreviewRequest} from '../../../packages/contracts/src';
import {ApiError,stateLabel,type RecordItem} from './api';
import {useApi} from './api-context';
import {formatNumber,tr,UiError,useLocale} from './i18n';
import {validOwnerTargets} from './task-owner-changes-model';
import {
  isTaskPlanningStatus,planningStatusRows,planningStatusSelections,taskPlanningStatuses,taskPlanningStatusPath,
  validPlanningStatusPreview,validPlanningStatusReceipt,validPlanningStatusRecovery,type PlanningStatusRow,type TaskPlanningStatus,
} from './task-planning-status-changes-model';
import {toolSessionStorage} from './tool-hooks';
import {Badge,ErrorNotice,Field,Modal} from './ui';
import {useWriteAction,WriteNotice} from './write-action';
import './TaskPlanningStatusChanges.css';

type Props={
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

const t=(key:string,values?:Record<string,unknown>)=>tr('task-planning-status-changes.'+key,values);
const isDenied=(status:number|undefined)=>status===401||status===403||status===404;
const refusalCodes=['version_conflict','idempotency_conflict','task_planning_status_locked'];

export function TaskPlanningStatusChanges(props:Props){
  const scope=JSON.stringify(['task-planning-status-changes',props.userID,props.organizationID,props.project]);
  return <PlanningStatusScope key={scope} scope={scope} {...props}/>;
}

function PlanningStatusScope({
  scope,project,userID,organizationID,tasks,disabled,authorityError,
  onChanged,onForbidden,onRefresh,onAuthorityRestored,
}:Props&{scope:string}){
  useLocale();
  const api=useApi();
  const currentApi=useRef(api);
  currentApi.current=api;
  const [open,setOpen]=useState(false);
  const [denied,setDenied]=useState(false);
  const [basis,setBasis]=useState<PlanningStatusRow[]>();
  const [preview,setPreview]=useState<TaskPlanningStatusPreview>();
  const [target,setTarget]=useState<TaskPlanningStatus>();
  const [receipt,setReceipt]=useState<TaskPlanningStatusChangeResult>();
  const [refreshFailed,setRefreshFailed]=useState(false);
  const [reading,setReading]=useState(false);
  const [readError,setReadError]=useState<unknown>();
  const requestController=useRef<AbortController|undefined>(undefined);
  const receiptFocus=useRef<HTMLElement>(null);
  const refreshFocus=useRef<HTMLButtonElement>(null);
  const blocked=useRef(false);
  const retired=useRef(false);
  const alive=useRef(true);
  const authorityBlocked=Boolean(authorityError)||denied;
  blocked.current=authorityBlocked;
  if(authorityBlocked)retired.current=true;
  const headers={'X-Suma-Expected-User':userID,'X-Suma-Expected-Organization':organizationID};

  const action=useWriteAction({
    scope,storage:toolSessionStorage(),
    validateRequest:request=>validPlanningStatusRecovery(request,project),
    validateResponse:(value,request)=>!blocked.current&&!retired.current&&validPlanningStatusReceipt(value,request,project),
    definitiveRefusalOnRestore:error=>!blocked.current&&!retired.current&&error.status===409&&refusalCodes.includes(error.code),
  });
  const pending=action.pending;
  const rows=planningStatusRows(project,tasks);
  const selectionChanged=!!basis&&!disabled&&JSON.stringify(rows)!==JSON.stringify(basis);
  const stale=action.conflict||selectionChanged||readError instanceof ApiError&&readError.status===409;
  const lockedCount=preview?.items.filter(item=>item.eligibility!=='editable').length??0;
  const changes=preview?.items.filter(item=>target&&item.eligibility==='editable'&&item.status!==target)??[];
  const exhausted=changes.some(item=>item.version===2147483647);
  const canApply=!!target&&!!basis&&!!preview&&!lockedCount&&!reading&&!disabled&&!stale&&!authorityBlocked&&
    !pending&&!action.busy&&!action.persistenceBlocked&&!exhausted&&changes.length>0;

  useEffect(()=>{
    alive.current=true;
    return ()=>{alive.current=false;requestController.current?.abort();};
  },[]);
  useEffect(()=>{
    if(!authorityBlocked)return;
    requestController.current?.abort();
    requestController.current=undefined;
    setReading(false);
    setReadError(undefined);
    setBasis(undefined);
    setPreview(undefined);
    setTarget(undefined);
    setReceipt(undefined);
    setRefreshFailed(false);
  },[authorityBlocked]);
  useEffect(()=>{
    if(blocked.current||!isDenied(action.errorStatus))return;
    rejectAccess(new ApiError(action.errorStatus!,'access_changed',t('access_changed')));
  },[action.errorStatus,onForbidden]);
  useEffect(()=>{if(receipt&&!authorityBlocked)receiptFocus.current?.focus();},[receipt,authorityBlocked]);
  useEffect(()=>{
    if(stale&&!authorityBlocked&&!pending&&!receipt)refreshFocus.current?.focus();
  },[stale,authorityBlocked,pending?.idempotency_key,receipt]);

  function rejectAccess(error:unknown){
    blocked.current=true;
    retired.current=true;
    setDenied(true);
    onForbidden(error);
  }

  async function read<T>(operation:(signal:AbortSignal)=>Promise<T>,receive:(value:T)=>void,checkingAccess=false){
    requestController.current?.abort();
    const abort=new AbortController();
    requestController.current=abort;
    setReading(true);
    setReadError(undefined);
    const current=()=>alive.current&&!abort.signal.aborted&&requestController.current===abort&&currentApi.current===api;
    try{
      const result=await operation(abort.signal);
      if(current()&&(checkingAccess||!blocked.current))receive(result);
    }catch(error){
      if(!current())return;
      if(!checkingAccess&&error instanceof ApiError&&isDenied(error.status))rejectAccess(error);
      else setReadError(error);
    }finally{
      if(current()){
        requestController.current=undefined;
        setReading(false);
      }
    }
  }

  function loadPreview(selection:PlanningStatusRow[]){
    setPreview(undefined);
    const request:TaskPlanningStatusPreviewRequest={project_id:project,tasks:planningStatusSelections(selection)};
    void read(async signal=>{
      const result=await api<unknown>(taskPlanningStatusPath+'/preview',request,'POST',headers,signal);
      if(!validPlanningStatusPreview(result,request))throw new UiError('task-planning-status-changes.preview_invalid');
      return result;
    },setPreview);
  }

  function launch(){
    if(!pending&&!action.persistenceBlocked&&!authorityBlocked){
      setBasis(rows);
      setTarget(undefined);
      setPreview(undefined);
      setReceipt(undefined);
      setRefreshFailed(false);
      setReadError(undefined);
      action.clearConflict();
      if(rows)loadPreview(rows);
    }
    setOpen(true);
  }

  function close(){
    if(!window.dispatchEvent(new Event('suma:navigate',{cancelable:true})))return;
    requestController.current?.abort();
    requestController.current=undefined;
    setReading(false);
    setOpen(false);
  }

  function checkAccess(){
    if(!authorityBlocked||reading||action.busy)return;
    const path='/task-owner-targets?'+new URLSearchParams({project_id:project,limit:'1'});
    void read(async signal=>{
      const result=await api<unknown>(path,undefined,'GET',headers,signal);
      if(!validOwnerTargets(result,project,1))throw new UiError('task-planning-status-changes.access_invalid');
    },()=>{
      setDenied(false);
      action.setError('');
      onAuthorityRestored?.();
      if(!pending)close();
    },true);
  }

  async function showReceipt(result:TaskPlanningStatusChangeResult|undefined){
    if(!result||blocked.current||retired.current||!alive.current||currentApi.current!==api)return;
    setReceipt(result);
    try{await onChanged();}
    catch{if(alive.current&&!blocked.current&&currentApi.current===api)setRefreshFailed(true);}
  }

  async function apply(){
    if(!canApply||!target||!basis)return;
    retired.current=false;
    const request={project_id:project,status:target,tasks:planningStatusSelections(basis)};
    await showReceipt(await action.run<TaskPlanningStatusChangeResult>(taskPlanningStatusPath,request,'POST'));
  }

  async function restore(){
    if(!pending||authorityBlocked||action.busy)return;
    retired.current=false;
    await showReceipt(await action.run<TaskPlanningStatusChangeResult>(pending.path,pending.body,pending.method));
  }

  const launchVisible=tasks.length>0||!!pending||action.persistenceBlocked;
  const receiptCount=receipt?.changes.filter(row=>row.changed).length??0;
  return <>
    {launchVisible&&<button type="button" data-task-planning-status-launch
      disabled={action.busy||!pending&&!action.persistenceBlocked&&(disabled||authorityBlocked)} onClick={launch}>
      <ListChecks size={16} aria-hidden="true"/>{pending||action.persistenceBlocked?t('recover'):t('launch')}
    </button>}
    <Modal open={open} title={t('title')} description={t('intro')} onClose={close}>
      <div className="task-planning-status">
        {!authorityBlocked&&<WriteNotice action={action}/>}
        {authorityBlocked?<section className="task-planning-status-notice">
          <p role="status">{t('access_changed')}</p>
          {Boolean(readError)&&<ErrorNotice error={readError}/>}
          <button type="button" data-task-planning-status-check-access disabled={reading||action.busy} onClick={checkAccess}>
            {t(reading?'checking_access':'check_access')}
          </button>
        </section>:pending?<section className="task-planning-status-notice">
          <strong>{t('original_operation')}</strong>
          <p>{t('recovery_help')}</p>
          <p><code>{pending.idempotency_key}</code></p>
          <button type="button" data-task-planning-status-restore disabled={action.busy} onClick={()=>void restore()}>
            {t(action.busy?'restoring':'restore')}
          </button>
        </section>:action.persistenceBlocked?<p role="status">{t('storage_help')}</p>:receipt?<section
          className="task-planning-status-notice task-planning-status-success" data-task-planning-status-receipt tabIndex={-1}
          ref={receiptFocus} aria-live="polite">
          <strong><CheckCircle2 size={18} aria-hidden="true"/>{t('saved')}</strong>
          <p>{t('summary',{changed:formatNumber(receiptCount),unchanged:formatNumber(receipt.changes.length-receiptCount)})}</p>
          {refreshFailed&&<p role="status">{t('refresh_failed')}</p>}
          <details>
            <summary>{t('receipt_details')}</summary>
            <p><code>{receipt.operation_id}</code></p>
            <ul>{receipt.changes.map(row=><li key={row.task_id}>
              <strong>{basis?.find(item=>item.task_id===row.task_id)?.title||row.task_id}</strong>
              <StatusTransition before={row.previous_status} after={row.changed?row.status:undefined}/>
            </li>)}</ul>
          </details>
        </section>:<>
          {stale?<section className="task-planning-status-notice">
            <p role="status">{t('stale')}</p>
            <button type="button" data-task-planning-status-refresh ref={refreshFocus} onClick={()=>{
              action.clearConflict();close();onRefresh?.();
            }}>{t('refresh')}</button>
          </section>:reading?<p className="task-planning-status-loading" role="status">
            <LoaderCircle size={17} aria-hidden="true"/>{t('loading_preview')}
          </p>:Boolean(readError)?<section className="task-planning-status-notice">
            <ErrorNotice error={readError}/>
            {basis&&<button type="button" data-task-planning-status-preview-retry onClick={()=>loadPreview(basis)}>{t('retry_preview')}</button>}
          </section>:!basis||!preview?<p role="status">{t('incomplete')}</p>:<>
            {lockedCount>0?<section className="task-planning-status-notice task-planning-status-blocked" role="status">
              <CircleAlert size={18} aria-hidden="true"/>
              <div><strong>{t('locked_count',{count:formatNumber(lockedCount)})}</strong><p>{t('locked_hint')}</p></div>
            </section>:<Field label={t('target')}>
              <select name="task_planning_status_target" data-task-planning-status-target value={target??''}
                disabled={disabled} onChange={event=>{
                  const value=event.target.value;
                  setTarget(isTaskPlanningStatus(value)?value:undefined);
                }}>
                <option value="">{t('choose')}</option>
                {taskPlanningStatuses.map(status=><option key={status} value={status}>{stateLabel(status)}</option>)}
              </select>
            </Field>}
            <div className="task-planning-status-review">
              <h3>{t('selection',{count:formatNumber(basis.length)})}</h3>
              {target&&<span role="status">{t('summary',{changed:formatNumber(changes.length),unchanged:formatNumber(basis.length-changes.length)})}</span>}
            </div>
            <ul className="task-planning-status-rows" aria-label={t('tasks')} tabIndex={0}>
              {preview.items.map((item,index)=>{
                const eligible=item.eligibility==='editable';
                const changesStatus=eligible&&!!target&&item.status!==target;
                return <li key={item.task_id} data-task-planning-status-row={item.task_id}>
                  <div className="task-planning-status-review">
                    <strong>{basis[index].title}</strong>
                    {!eligible?<span className="task-planning-status-lock-label">{t('locked')}</span>:
                      target&&<span className={changesStatus?'task-planning-status-delta':undefined}>{t(changesStatus?'will_change':'unchanged')}</span>}
                  </div>
                  <StatusTransition before={item.status} after={changesStatus?target:undefined}/>
                  {!eligible&&<p className="task-planning-status-reason">{t('reason.'+item.eligibility)}</p>}
                </li>;
              })}
            </ul>
            {target&&changes.length===0&&<p role="status">{t('no_changes')}</p>}
            {exhausted&&<p className="task-planning-status-notice" role="status">{t('version_exhausted')}</p>}
          </>}
          <div className="task-planning-status-actions">
            <button type="button" className="primary" data-task-planning-status-apply disabled={!canApply} onClick={()=>void apply()}>
              {changes.length?t('apply',{count:formatNumber(changes.length)}):t('apply_empty')}
            </button>
          </div>
        </>}
      </div>
    </Modal>
  </>;
}

function StatusTransition({before,after}:{before:string;after?:string}){
  return <div className="task-planning-status-transfer">
    {isTaskPlanningStatus(before)||before==='in_review'||before==='accepted'?<Badge state={before}/>:<span className="badge">{stateLabel(before)}</span>}
    {after&&<>
      <ArrowRight size={14} aria-hidden="true"/><span className="sr-only">{t('to')}</span>
      <Badge state={after}/>
    </>}
  </div>;
}
