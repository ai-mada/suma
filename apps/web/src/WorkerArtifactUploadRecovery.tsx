import {useEffect,useId,useRef,useState} from 'react';
import {ChevronDown,FileWarning} from 'lucide-react';
import {stateLabel} from './api';
import {formatDateTime,formatNumber,tr,useLocale,UiError} from './i18n';
import {toolSessionStorage,useToolRead} from './tool-hooks';
import type {ExecutionWorker} from './tool-model';
import {ErrorNotice,Loading} from './ui';
import {useWriteAction,WriteNotice,type WriteAction} from './write-action';
import {
  uploadAbandonPath,uploadRecoveryPath,validUploadAbandonReceipt,validUploadAbandonRequest,validUploadRecoveryView,
  type UploadAbandonReceipt,type UploadRecoveryItem,
} from './worker-artifact-upload-recovery-model';
import './WorkerArtifactUploadRecovery.css';

const t=(key:string,values?:Record<string,unknown>)=>tr('worker-artifact-upload-recovery.'+key,values);
const definiteRefusals=['version_conflict','artifact_upload_attempt_unresolved','idempotency_conflict'];

export function useWorkerUploadRecoveryAction(project:string,userID:string,organizationID:string){
  return useWriteAction({
    scope:JSON.stringify(['worker-artifact-upload-recovery',organizationID,userID,project]),
    storage:toolSessionStorage(),
    validateRequest:request=>validUploadAbandonRequest(request,project),
    validateResponse:(value,request)=>validUploadAbandonReceipt(value,request,project),
    definitiveRefusalOnRestore:(error,request)=>error.status===409&&definiteRefusals.includes(error.code)&&validUploadAbandonRequest(request,project),
  });
}

export async function restoreWorkerUpload(action:WriteAction){
  const request=action.pending;
  if(!request||action.busy)return;
  return action.run<UploadAbandonReceipt>(request.path,request.body,request.method);
}

export function WorkerUploadRecoveryNotice({action,disabled,onRestoreStart,onRestored}:{
  action:WriteAction;disabled:boolean;onRestoreStart?:()=>void;onRestored:(receipt:UploadAbandonReceipt)=>void;
}){
  useLocale();
  if(!action.pending&&!action.error&&!action.conflict)return null;
  return <div className="worker-upload-recovery-notice">
    <WriteNotice action={action}/>
    {action.pending&&<>
      <p role="status">{t('pending')}</p>
      <button type="button" data-worker-upload-restore disabled={disabled||action.busy} onClick={async()=>{
        onRestoreStart?.();
        const receipt=await restoreWorkerUpload(action);
        if(receipt)onRestored(receipt);
      }}>{t('restore')}</button>
    </>}
  </div>;
}

export function WorkerUploadRecoveryReceipt(){
  useLocale();
  return <p className="worker-upload-recovery-receipt" role="status" data-worker-upload-receipt>{t('receipt')}</p>;
}

export function WorkerArtifactUploadRecovery({worker,action,disabled,restoreDisabled,receipt,onSaved}:{
  worker:ExecutionWorker;action:WriteAction;disabled:boolean;restoreDisabled:boolean;receipt?:UploadAbandonReceipt;
  onSaved:(receipt:UploadAbandonReceipt)=>void;
}){
  useLocale();
  const region=useId();
  const [open,setOpen]=useState(false),[pages,setPages]=useState(['']),[epoch,setEpoch]=useState(0);
  const [selected,setSelected]=useState<UploadRecoveryItem>(),[ack,setAck]=useState(false);
  const locked=action.busy||!!action.pending;
  const visible=open||locked;
  const cursor=pages[pages.length-1];
  const read=useToolRead<unknown>(visible&&!locked?uploadRecoveryPath(worker,cursor):'',epoch);
  const view=validUploadRecoveryView(read.data,worker,cursor)?read.data:undefined;
  const readable=!!view&&!read.loading&&!read.error;
  const stale=read.data!==undefined&&!view&&!read.loading&&!read.error;
  const match=readable&&selected&&view.items.find(item=>item.upload_id===selected.upload_id&&item.manifest_sha256===selected.manifest_sha256&&item.disposition==='eligible');
  const confirmation=match||undefined;
  const confirmHeading=useRef<HTMLElement>(null);
  useEffect(()=>{if(confirmation)confirmHeading.current?.focus();},[confirmation?.upload_id]);

  function clearSelection(){
    setSelected(undefined);
    setAck(false);
  }
  function changePage(next:string[]){
    clearSelection();
    setPages(next);
    action.clearConflict();
  }
  function refresh(){
    changePage(['']);
    setEpoch(value=>value+1);
  }
  function saved(value:UploadAbandonReceipt){
    onSaved(value);
    refresh();
  }
  async function abandon(){
    if(!confirmation||!ack||disabled||locked||action.conflict||action.persistenceBlocked)return;
    const value=await action.run<UploadAbandonReceipt>(uploadAbandonPath(worker,confirmation),{
      project_id:worker.project_id,
      expected_worker_version:worker.version,
      expected_manifest_sha256:confirmation.manifest_sha256,
      acknowledge_incomplete:true,
    });
    if(value)saved(value);
  }

  return <section className="worker-upload-recovery">
    <button type="button" className="worker-upload-recovery-toggle" aria-expanded={visible} aria-controls={region}
      data-worker-upload-recovery-open disabled={locked} onClick={()=>{
        if(visible)clearSelection();
        else refresh();
        setOpen(!visible);
      }}>
      <FileWarning size={17} aria-hidden="true"/>
      <span>{t('open')}</span>
      <ChevronDown size={16} aria-hidden="true"/>
    </button>
    {receipt&&<WorkerUploadRecoveryReceipt/>}
    <div id={region} hidden={!visible} className="worker-upload-recovery-body">
      {visible&&<>
        <p>{t('intro')}</p>
        <WorkerUploadRecoveryNotice action={action} disabled={restoreDisabled} onRestoreStart={()=>setOpen(true)} onRestored={saved}/>
        {!locked&&<>
          {read.loading&&<Loading/>}
          {read.error&&<ErrorNotice error={read.error}/>}
          {stale&&<ErrorNotice error={new UiError('worker-artifact-upload-recovery.stale')}/>}
          {action.conflict&&<p role="status">{t('conflict')}</p>}
          {readable&&(view.items.length>0?
            <ul className="worker-upload-list" aria-label={t('list')}>
              {view.items.map(item=><li key={item.upload_id} className="worker-upload-row" data-worker-upload-row={item.upload_id}>
                <div className="worker-upload-row-main">
                  <strong>{t('original',{attempt:formatNumber(item.attempt),fence:item.fence})}</strong>
                  <span className="worker-upload-disposition" data-disposition={item.disposition}>{t(item.disposition)}</span>
                </div>
                <dl>
                  <dt>{t('run')}</dt><dd><code>{item.run_id}</code></dd>
                  <dt>{t('node')}</dt><dd><code>{item.node_id}</code></dd>
                  <dt>{t('attempt_state')}</dt><dd>{stateLabel(item.attempt_state)}</dd>
                </dl>
                <div className="worker-upload-row-meta">{formatDateTime(item.created_at)} · {t('size',{bytes:formatNumber(item.size_bytes)})}</div>
                <details><summary>{t('manifest')}</summary><code>{item.manifest_sha256}</code></details>
                {item.disposition==='eligible'&&<button type="button" data-worker-upload-select={item.upload_id} disabled={disabled||action.conflict||action.persistenceBlocked} onClick={()=>{
                  setSelected(item);setAck(false);
                }}>{t('select')}</button>}
              </li>)}
            </ul>:<p role="status">{t('empty')}</p>)}
          {confirmation&&!action.conflict&&<div className="worker-upload-confirm">
            <strong ref={confirmHeading} tabIndex={-1}>{t('confirm')}</strong>
            <p>{t('upload')} <code>{confirmation.upload_id}</code></p>
            <label className="check">
              <input type="checkbox" data-worker-upload-ack checked={ack} disabled={disabled} onChange={event=>setAck(event.target.checked)}/>
              {t('acknowledge')}
            </label>
            <div className="worker-upload-actions">
              <button type="button" onClick={clearSelection}>{t('cancel')}</button>
              <button type="button" className="danger" data-worker-upload-abandon disabled={!ack||disabled||action.persistenceBlocked} onClick={()=>void abandon()}>{t('abandon')}</button>
            </div>
          </div>}
          <div className="worker-upload-actions">
            <button type="button" data-worker-upload-refresh disabled={read.loading} onClick={refresh}>{t('refresh')}</button>
            {pages.length>1&&<button type="button" disabled={read.loading} onClick={()=>changePage(pages.slice(0,-1))}>{t('previous')}</button>}
            {readable&&view.next_cursor&&<button type="button" onClick={()=>changePage([...pages,view.next_cursor])}>{t('next')}</button>}
          </div>
        </>}
      </>}
    </div>
  </section>;
}
