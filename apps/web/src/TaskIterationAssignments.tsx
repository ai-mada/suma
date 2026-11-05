import {useEffect,useRef,useState} from 'react';
import {ArrowRight,CalendarRange,CheckCircle2} from 'lucide-react';
import type {TaskIterationAssignmentRequest,TaskIterationAssignmentResult,TaskWorkspacePlanRef} from '../../../packages/contracts/src';
import {ApiError,stateLabel,type RecordItem} from './api';
import {useApi} from './api-context';
import {formatNumber,tr,UiError,useLocale} from './i18n';
import type {Iteration} from './planning-model';
import {validOwnerTargets} from './task-owner-changes-model';
import {
  assignableIteration,assignmentRows,iterationAssignmentPath,iterationAssignmentReceipt,iterationAssignmentRecovery,
  maxIterationAssignmentVersion,type AssignmentRow,
} from './task-iteration-assignments-model';
import {toolSessionStorage} from './tool-hooks';
import {ErrorNotice,Field,Modal} from './ui';
import {useWriteAction,WriteNotice} from './write-action';
import './TaskIterationAssignments.css';

type Props={
  project:string;
  userID:string;
  organizationID:string;
  tasks:RecordItem[];
  plans:TaskWorkspacePlanRef[];
  iterations:Iteration[];
  disabled?:boolean;
  authorityError?:unknown;
  onChanged:()=>void|Promise<void>;
  onForbidden:(error:unknown)=>void;
  onRefresh?:()=>void;
  onAuthorityRestored?:()=>void;
};
type Target={id:string;version:number;title:string};
const remove='remove',iterationPrefix='iteration:';
const t=(key:string,values?:Record<string,unknown>)=>tr('task-iteration-assignments.'+key,values);
const deniedStatus=(status:number|undefined)=>status!==undefined&&[401,403,404].includes(status);

// Keep the original-operation control mounted even when the collection selection clears.
export function TaskIterationAssignments(props:Props){
  const scope=JSON.stringify(['task-iteration-assignments',props.userID,props.organizationID,props.project]);
  return <AssignmentScope key={scope} {...props} scope={scope}/>;
}

function AssignmentScope({
  scope,project,userID,organizationID,tasks,plans,iterations,disabled,authorityError,
  onChanged,onForbidden,onRefresh,onAuthorityRestored,
}:Props&{scope:string}){
  useLocale();
  const api=useApi(),currentApi=useRef(api);
  currentApi.current=api;
  const [open,setOpen]=useState(false),[denied,setDenied]=useState(false);
  const [basis,setBasis]=useState<AssignmentRow[]>(),[target,setTarget]=useState<Target>();
  const [receipt,setReceipt]=useState<TaskIterationAssignmentResult>(),[refreshError,setRefreshError]=useState<unknown>();
  const [checking,setChecking]=useState(false),[accessError,setAccessError]=useState<unknown>();
  const checkController=useRef<AbortController|undefined>(undefined);
  const receiptFocus=useRef<HTMLElement>(null),refreshFocus=useRef<HTMLButtonElement>(null);
  const blocked=useRef(false),writeRetired=useRef(false),alive=useRef(true);
  const authorityBlocked=Boolean(authorityError)||denied;
  blocked.current=authorityBlocked;
  if(authorityBlocked)writeRetired.current=true;

  const action=useWriteAction({
    scope,storage:toolSessionStorage(),
    validateRequest:request=>iterationAssignmentRecovery(request,project),
    validateResponse:(value,request)=>!blocked.current&&!writeRetired.current&&iterationAssignmentReceipt(value,request,project),
  });
  const pending=action.pending,locked=action.busy||!!pending||action.persistenceBlocked;
  const rows=assignmentRows(project,tasks,plans,iterations);
  const targets=iterations.filter(assignableIteration);
  const targetValue=target?(target.id?iterationPrefix+target.id:remove):'';
  const currentTarget=target?.id?targets.find(iteration=>iteration.id===target.id):undefined;
  const targetStale=!!target?.id&&(!currentTarget||currentTarget.version!==target.version||(currentTarget.title||currentTarget.id)!==target.title);
  const stale=open&&(!disabled&&(JSON.stringify(rows)!==JSON.stringify(basis)||targetStale)||action.conflict);
  const changed=basis?.filter(row=>target&&row.sourceID!==target.id)??[];
  const sourceClosed=changed.some(row=>row.source?.status==='completed');
  const sourceUnknown=changed.some(row=>row.sourceID&&(!row.source||!assignableIteration(row.source))&&row.source?.status!=='completed');
  const exhausted=changed.some(row=>row.task.expected_plan_version===maxIterationAssignmentVersion);
  const canApply=!!target&&!disabled&&!stale&&!locked&&!authorityBlocked&&changed.length>0&&!sourceClosed&&!sourceUnknown&&!exhausted;

  useEffect(()=>{
    alive.current=true;
    return ()=>{alive.current=false;checkController.current?.abort();};
  },[]);
  useEffect(()=>{
    if(!authorityBlocked)return;
    setBasis(undefined);
    setTarget(undefined);
    setReceipt(undefined);
    setRefreshError(undefined);
  },[authorityBlocked]);
  useEffect(()=>{
    if(blocked.current||!deniedStatus(action.errorStatus))return;
    blocked.current=true;
    writeRetired.current=true;
    setDenied(true);
    onForbidden(new ApiError(action.errorStatus!,'access_changed',t('access_changed')));
  },[action.errorStatus,onForbidden]);
  const parentDenied=useRef(false);
  useEffect(()=>{
    if(!authorityError){
      if(parentDenied.current){
        parentDenied.current=false;
        setDenied(false);
        action.setError('');
      }
      return;
    }
    parentDenied.current=true;
    checkController.current?.abort();
    checkController.current=undefined;
    setChecking(false);
    setAccessError(undefined);
  },[authorityError]);

  useEffect(()=>{if(receipt&&!authorityBlocked)receiptFocus.current?.focus();},[receipt,authorityBlocked]);
  useEffect(()=>{if(action.conflict&&!authorityBlocked)refreshFocus.current?.focus();},[action.conflict,authorityBlocked]);

  function launch(){
    if(!locked&&!authorityBlocked){
      setBasis(rows);
      setTarget(undefined);
      setReceipt(undefined);
      setRefreshError(undefined);
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
      // This existing endpoint verifies current project write authority. No
      // person-directory facts are retained or rendered in the iteration dialog.
      const headers={'X-Suma-Expected-User':userID,'X-Suma-Expected-Organization':organizationID};
      const path='/task-owner-targets?'+new URLSearchParams({project_id:project,limit:'1'});
      const result=await api<unknown>(path,undefined,'GET',headers,abort.signal);
      if(!current())return;
      if(!validOwnerTargets(result,project,1))throw new UiError('task-iteration-assignments.access_invalid');
      setDenied(false);
      action.setError('');
      onAuthorityRestored?.();
    }catch(error){
      if(current())setAccessError(error);
    }finally{
      if(current())setChecking(false);
    }
  }

  async function showReceipt(result:TaskIterationAssignmentResult|undefined){
    if(!result||blocked.current||writeRetired.current||!alive.current||currentApi.current!==api)return;
    setReceipt(result);
    try{await onChanged();}
    catch(error){
      if(alive.current&&!blocked.current&&currentApi.current===api)setRefreshError(error);
    }
  }

  async function apply(){
    if(!canApply||!target||!basis)return;
    const draft:Omit<TaskIterationAssignmentRequest,'idempotency_key'>={
      project_id:project,iteration_id:target.id,expected_iteration_version:target.version,tasks:basis.map(row=>row.task),
    };
    writeRetired.current=false;
    await showReceipt(await action.run<TaskIterationAssignmentResult>(iterationAssignmentPath,draft,'POST'));
  }

  async function restore(){
    if(!pending||authorityBlocked||action.busy)return;
    writeRetired.current=false;
    await showReceipt(await action.run<TaskIterationAssignmentResult>(pending.path,pending.body,pending.method));
  }

  const launchVisible=tasks.length>0||!!pending||action.persistenceBlocked;
  const reviewedCount=basis?.length??0;
  const receiptCount=receipt?.changes.filter(row=>row.changed).length??0;
  return <>
    {launchVisible&&<button type="button" data-iteration-assignment-launch data-iteration-assignment-open
      disabled={(!pending&&!action.persistenceBlocked&&(disabled||authorityBlocked))||action.busy} onClick={launch}>
      <CalendarRange size={16} aria-hidden="true"/>{pending||action.persistenceBlocked?t('recover'):t('launch',{count:formatNumber(tasks.length)})}
    </button>}
    <Modal open={open} title={t('title')} description={t('intro')} onClose={()=>{
      if(!checking&&window.dispatchEvent(new Event('suma:navigate',{cancelable:true})))setOpen(false);
    }}>
      <div className="task-iteration-assignment">
        {!authorityBlocked&&<WriteNotice action={action}/>}
        {authorityBlocked?<section className="task-iteration-assignment-notice">
          <p role="status">{t('access_changed')}</p>
          {Boolean(accessError)&&<ErrorNotice error={accessError}/>}
          <button type="button" data-iteration-assignment-check-access disabled={checking||action.busy} onClick={()=>void checkAccess()}>
            {t(checking?'checking_access':'check_access')}
          </button>
        </section>:pending?<section className="task-iteration-assignment-notice">
          <strong>{t('original_operation')}</strong>
          <p>{t('recovery_help')}</p>
          <p><code>{pending.idempotency_key}</code></p>
          <button type="button" data-iteration-assignment-restore disabled={action.busy} onClick={()=>void restore()}>
            {t(action.busy?'restoring':'restore')}
          </button>
        </section>:action.persistenceBlocked?<p role="status">{t('storage_help')}</p>:receipt?<section
          className="task-iteration-assignment-notice task-iteration-assignment-success" data-iteration-assignment-receipt tabIndex={-1} ref={receiptFocus} aria-live="polite">
          <strong><CheckCircle2 size={18} aria-hidden="true"/>{t('saved')}</strong>
          <p>{t('summary',{changed:formatNumber(receiptCount),unchanged:formatNumber(receipt.changes.length-receiptCount)})}</p>
          {Boolean(refreshError)&&<p role="status">{t('refresh_failed')}</p>}
          <details><summary>{t('receipt_details')}</summary><p><code>{receipt.operation_id}</code></p>
            <ul>{receipt.changes.map(row=><li key={row.task_id}>
              <span>{basis?.find(item=>item.task.task_id===row.task_id)?.title||row.task_id}</span>
              <span>{t(row.changed?'updated':'unchanged')} · {t('plan_version',{version:formatNumber(row.plan_version)})}</span>
            </li>)}</ul>
          </details>
        </section>:<>
          <Field label={t('target')} hint={t('scope')}>
            <select name="task_iteration_target" data-iteration-assignment-target value={targetValue}
              disabled={disabled||stale||!basis} onChange={event=>{
                const value=event.target.value;
                const next=targets.find(iteration=>iterationPrefix+iteration.id===value);
                setTarget(value===remove?{id:'',version:0,title:''}:next&&{id:next.id,version:next.version,title:next.title||next.id});
              }}>
              <option value="">{t('choose')}</option>
              <option value={remove}>{t('remove')}</option>
              {targets.map(iteration=><option key={iteration.id} value={iterationPrefix+iteration.id}>
                {iteration.title||iteration.id} · {stateLabel(iteration.status)}
                {iteration.start_date&&iteration.end_date?' · '+iteration.start_date+' – '+iteration.end_date:''}
              </option>)}
            </select>
          </Field>
          {stale?<section className="task-iteration-assignment-notice">
            <p role="status">{t('stale')}</p>
            <button type="button" data-iteration-assignment-refresh ref={refreshFocus} onClick={()=>{setOpen(false);onRefresh?.();}}>{t('refresh')}</button>
          </section>:!basis?<p role="status">{t('incomplete')}</p>:<>
            <div className="task-iteration-assignment-review">
              <h3>{t('selection',{count:formatNumber(reviewedCount)})}</h3>
              {target&&<span role="status">{t('review_summary',{changed:formatNumber(changed.length),unchanged:formatNumber(reviewedCount-changed.length)})}</span>}
            </div>
            <ul className="task-iteration-assignment-rows" aria-label={t('tasks')} tabIndex={0}>
              {basis.map(row=>{
                const changes=!!target&&row.sourceID!==target.id;
                const source=row.source?.title||row.sourceID||t('unassigned');
                return <li key={row.task.task_id} data-iteration-assignment-row={row.task.task_id}>
                  <div className="task-iteration-assignment-review">
                    <strong>{row.title}</strong>
                    {target&&<span className={changes?'task-iteration-assignment-delta':''}>{t(changes?'will_change':'unchanged')}</span>}
                  </div>
                  <div className="task-iteration-assignment-transfer">
                    <span>{source}</span>
                    {changes&&<><ArrowRight size={14} aria-hidden="true"/><b>{target.title||t('unassigned')}</b></>}
                  </div>
                </li>;
              })}
            </ul>
            {target&&changed.length===0&&<p role="status">{t('no_changes')}</p>}
            {(sourceClosed||sourceUnknown||exhausted)&&<div className="task-iteration-assignment-notice" role="status">
              {sourceClosed&&<p>{t('completed_source')}</p>}
              {sourceUnknown&&<p>{t('source_unavailable')}</p>}
              {exhausted&&<p>{t('version_exhausted')}</p>}
            </div>}
          </>}
          <div className="task-iteration-assignment-actions">
            <button type="button" className="primary" data-iteration-assignment-apply disabled={!canApply} onClick={()=>void apply()}>
              {t('apply',{count:formatNumber(changed.length)})}
            </button>
          </div>
        </>}
      </div>
    </Modal>
  </>;
}
