import {useEffect,useId,useRef,useState} from 'react';
import {ChevronDown,FileWarning,GitBranch,Package,RefreshCw} from 'lucide-react';
import {stateLabel} from './api';
import {formatDateTime,formatNumber,tr,useLocale,UiError} from './i18n';
import {toolSessionStorage,useToolRead} from './tool-hooks';
import type {ExecutionWorker} from './tool-model';
import {ErrorNotice,Loading} from './ui';
import {useWriteAction,WriteNotice,type WriteAction} from './write-action';
import {
  definitiveUploadRefusal,readUploadRecoveryView,uploadAbandonBasis,uploadAbandonPath,uploadRecoveryPath,
  validUploadAbandonReceipt,validUploadAbandonRequest,
  type UploadAbandonReceipt,type UploadRecoveryItem,type UploadRecoveryKind,
} from './worker-artifact-upload-recovery-model';
import './WorkerArtifactUploadRecovery.css';

const t=(key:string,values?:Record<string,unknown>)=>tr('worker-artifact-upload-recovery.'+key,values);
const artifactKind='artifact-uploads';
const candidateKind='candidate-transfers';

export function useWorkerUploadRecoveryAction(project:string,userID:string,organizationID:string){
  return useWriteAction({
    scope:JSON.stringify(['worker-artifact-upload-recovery',organizationID,userID,project]),
    storage:toolSessionStorage(),
    validateRequest:request=>validUploadAbandonRequest(request,project),
    validateResponse:(value,request)=>validUploadAbandonReceipt(value,request,project),
    definitiveRefusalOnRestore:(error,request)=>error.status===409&&definitiveUploadRefusal(error.code,request,project),
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

export function WorkerUploadRecoveryReceipt({receipt}:{receipt:UploadAbandonReceipt}){
  useLocale();
  return <p className="worker-upload-recovery-receipt" role="status" data-worker-upload-receipt>{t('transfer_id' in receipt?'candidate.receipt':'receipt')}</p>;
}

export function WorkerArtifactUploadRecovery({worker,action,disabled,restoreDisabled,receipt,onSaved}:{
  worker:ExecutionWorker;action:WriteAction;disabled:boolean;restoreDisabled:boolean;receipt?:UploadAbandonReceipt;
  onSaved:(receipt:UploadAbandonReceipt)=>void;
}){
  useLocale();
  const region=useId();
  const [open,setOpen]=useState(false),[pages,setPages]=useState(['']),[epoch,setEpoch]=useState(0);
  const [kind,setKind]=useState<UploadRecoveryKind>(()=>action.pending?.path.includes('/candidate-transfers/')?candidateKind:artifactKind);
  const [selected,setSelected]=useState<UploadRecoveryItem>(),[ack,setAck]=useState(false);
  const locked=action.busy||!!action.pending;
  const visible=open||locked;
  const candidate=kind===candidateKind;
  const label=(key:string)=>t(candidate?'candidate.'+key:key);
  const cursor=pages[pages.length-1];
  const read=useToolRead<unknown>(visible&&!locked?uploadRecoveryPath(worker,kind,cursor):'',epoch);
  const view=readUploadRecoveryView(read.data,worker,kind,cursor);
  const readable=!!view&&!read.loading&&!read.error;
  const stale=read.data!==undefined&&!view&&!read.loading&&!read.error;
  const confirmation=(readable&&selected&&view.items.find(item=>item.id===selected.id&&item.sha256===selected.sha256&&
    item.run_id===selected.run_id&&item.node_id===selected.node_id&&item.attempt===selected.attempt&&item.fence===selected.fence&&
    item.commits?.base===selected.commits?.base&&item.commits?.head===selected.commits?.head&&item.disposition==='eligible'))||undefined;
  const confirmHeading=useRef<HTMLElement>(null);
  useEffect(()=>{if(confirmation)confirmHeading.current?.focus();},[confirmation?.id]);

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
  function changeKind(next:UploadRecoveryKind){
    if(next===kind||locked)return;
    setKind(next);
    refresh();
  }
  function saved(value:UploadAbandonReceipt){
    onSaved(value);
    setKind('transfer_id' in value?candidateKind:artifactKind);
    refresh();
  }
  async function abandon(){
    if(!confirmation||!ack||disabled||locked||action.conflict||action.persistenceBlocked)return;
    const value=await action.run<UploadAbandonReceipt>(
      uploadAbandonPath(worker,kind,confirmation.id),
      uploadAbandonBasis(worker,kind,confirmation),
    );
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
    {receipt&&<WorkerUploadRecoveryReceipt receipt={receipt}/>}
    <div id={region} hidden={!visible} className="worker-upload-recovery-body">
      {visible&&<>
        <div className="worker-upload-recovery-kinds" role="group" aria-label={t('kind')}>
          <button type="button" aria-pressed={!candidate} disabled={locked} onClick={()=>changeKind(artifactKind)}>
            <Package size={16} aria-hidden="true"/>{t('list')}
          </button>
          <button type="button" aria-pressed={candidate} disabled={locked} data-worker-candidate-recovery-tab onClick={()=>changeKind(candidateKind)}>
            <GitBranch size={16} aria-hidden="true"/>{t('candidate.list')}
          </button>
        </div>
        <p>{label('intro')}</p>
        <WorkerUploadRecoveryNotice action={action} disabled={restoreDisabled} onRestoreStart={()=>setOpen(true)} onRestored={saved}/>
        {!locked&&<>
          {read.loading&&<Loading/>}
          {read.error&&<ErrorNotice error={read.error}/>}
          {stale&&<ErrorNotice error={new UiError(candidate?'worker-artifact-upload-recovery.candidate.stale':'worker-artifact-upload-recovery.stale')}/>}
          {action.conflict&&<p role="status">{label('conflict')}</p>}
          {readable&&(view.items.length>0?
            <ul className="worker-upload-list" aria-label={label('list')}>
              {view.items.map(item=><li key={item.id} className="worker-upload-row" data-worker-upload-row={candidate?undefined:item.id}
                data-worker-candidate-transfer-row={candidate?item.id:undefined}>
                <div className="worker-upload-row-main">
                  <strong>{t('original',{attempt:formatNumber(item.attempt),fence:item.fence})}</strong>
                  <span className="worker-upload-disposition" data-disposition={item.disposition}>{item.disposition==='eligible'?label('eligible'):t(item.disposition)}</span>
                </div>
                <dl>
                  <dt>{t('run')}</dt><dd><code>{item.run_id}</code></dd>
                  <dt>{t('node')}</dt><dd><code>{item.node_id}</code></dd>
                  <dt>{t('attempt_state')}</dt><dd>{stateLabel(item.attempt_state)}</dd>
                </dl>
                {item.commits?.base&&<div className="worker-upload-commits">
                  <span>{t('candidate.base')} <code title={item.commits.base}>{item.commits.base.slice(0,12)}</code></span>
                  <span>{t('candidate.head')} <code title={item.commits.head}>{item.commits.head.slice(0,12)}</code></span>
                </div>}
                <div className="worker-upload-row-meta">{formatDateTime(item.created_at)} · {item.sha256?t('size',{bytes:formatNumber(item.size_bytes)}):t('candidate.size_unverified')}</div>
                {item.sha256&&<details>
                  <summary>{label('manifest')}</summary>
                  {item.commits?<dl>
                    <dt>{t('candidate.base')}</dt><dd><code>{item.commits.base}</code></dd>
                    <dt>{t('candidate.head')}</dt><dd><code>{item.commits.head}</code></dd>
                    <dt>{t('candidate.digest')}</dt><dd><code>{item.sha256}</code></dd>
                  </dl>:<code>{item.sha256}</code>}
                </details>}
                {item.disposition==='eligible'&&<button type="button" data-worker-upload-select={candidate?undefined:item.id}
                  data-worker-candidate-transfer-select={candidate?item.id:undefined} disabled={disabled||action.conflict||action.persistenceBlocked} onClick={()=>{
                    setSelected(item);setAck(false);
                  }}>{label('select')}</button>}
              </li>)}
            </ul>:<p role="status" className="worker-upload-empty">{candidate&&view.next_cursor?t('candidate.empty_page'):label('empty')}</p>)}
          {confirmation&&!action.conflict&&<div className="worker-upload-confirm">
            <strong ref={confirmHeading} tabIndex={-1}>{label('confirm')}</strong>
            <p>{label('upload')} <code>{confirmation.id}</code></p>
            {confirmation.commits&&<p>{t('original',{attempt:formatNumber(confirmation.attempt),fence:confirmation.fence})} · {t('candidate.head')} <code>{confirmation.commits.head.slice(0,12)}</code></p>}
            <label className="check">
              <input type="checkbox" data-worker-upload-ack checked={ack} disabled={disabled} onChange={event=>setAck(event.target.checked)}/>
              {label('acknowledge')}
            </label>
            <div className="worker-upload-actions">
              <button type="button" onClick={clearSelection}>{t('cancel')}</button>
              <button type="button" className="danger" data-worker-upload-abandon disabled={!ack||disabled||action.persistenceBlocked} onClick={()=>void abandon()}>{label('abandon')}</button>
            </div>
          </div>}
          <div className="worker-upload-actions worker-upload-pagination">
            <button type="button" data-worker-upload-refresh disabled={read.loading} onClick={refresh}>
              <RefreshCw size={14} aria-hidden="true"/>{t('refresh')}
            </button>
            {pages.length>1&&<button type="button" disabled={read.loading} onClick={()=>changePage(pages.slice(0,-1))}>{t('previous')}</button>}
            {readable&&view.next_cursor&&<button type="button" onClick={()=>changePage([...pages,view.next_cursor])}>{t('next')}</button>}
          </div>
        </>}
      </>}
    </div>
  </section>;
}
