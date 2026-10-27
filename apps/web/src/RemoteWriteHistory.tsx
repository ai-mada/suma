import {useEffect,useRef,useState} from 'react';
import type {WorkspaceWriteCommand,WorkspaceWriteInspectionRequest,WorkspaceWritePage} from '../../../packages/contracts/src';
import {operation,query,type ApiCaller} from './api';
import {useApi} from './api-context';
import {formatDateTime,formatNumber,tr,UiError,useLocale} from './i18n';
import {Empty,ErrorNotice,Loading} from './ui';
import {remoteWritePending,validWriteMetadata} from './remote-workspace-write-model';

const t=(key:string,values?:Record<string,unknown>)=>tr('remote-workspace-write.'+key,values);
type View<T>={api:ApiCaller;value:T};

/** Receipt history is the recovery entry after refresh or closing a draft.
 * It contains metadata only and loads on demand, rather than polling every
 * command or transferring source in the background. */
export function RemoteWriteHistory({project,revision,active}:{project:string;revision:number;active:boolean}){
 const api=useApi();useLocale();
 const [open,setOpen]=useState(false),[page,setPage]=useState<View<WorkspaceWritePage>>(),[selected,setSelected]=useState<View<WorkspaceWriteCommand>>(),[error,setError]=useState<unknown>(),[loading,setLoading]=useState(false),[busy,setBusy]=useState(false),[refresh,setRefresh]=useState(0);
 const listEpoch=useRef(0),detailEpoch=useRef(0),controller=useRef<AbortController|undefined>(undefined),timer=useRef<ReturnType<typeof setTimeout>|undefined>(undefined);
 const inspection=useRef<{api:ApiCaller;id:string;body:WorkspaceWriteInspectionRequest}|undefined>(undefined);
 const records=page?.api===api?page.value:undefined,command=selected?.api===api?selected.value:undefined;
 function stop(){detailEpoch.current++;controller.current?.abort();if(timer.current)clearTimeout(timer.current);}
 useEffect(()=>{stop();setBusy(false);setSelected(undefined);setPage(undefined);setError(undefined);inspection.current=undefined;},[api,project]);
 useEffect(()=>{
  if(!open||!active)return;
  const generation=++listEpoch.current,abort=new AbortController();setLoading(true);setError(undefined);
  void api<WorkspaceWritePage>('/workspace/writes'+query(project),undefined,'GET',undefined,abort.signal).then(value=>{
   if(abort.signal.aborted||generation!==listEpoch.current)return;
   if(!value||value.project_id!==project||!Array.isArray(value.items)||value.items.length>50||!Number.isSafeInteger(value.total)||value.total<value.items.length||value.limit!==50||typeof value.truncated!=='boolean'||!value.items.every(item=>validWriteMetadata(item,project)))throw new UiError('remote-workspace-write.invalid_receipt');
   setPage({api,value});
  }).catch(reason=>{if(!abort.signal.aborted&&generation===listEpoch.current){setPage(undefined);setSelected(undefined);setError(reason);}}).finally(()=>{if(!abort.signal.aborted&&generation===listEpoch.current)setLoading(false);});
  return()=>abort.abort();
 },[api,project,revision,refresh,open,active]);
 useEffect(()=>()=>stop(),[]);
 useEffect(()=>{if(!open||!active){stop();setBusy(false);}},[open,active]);

 async function detail(id:string,prior?:WorkspaceWriteCommand){
  stop();const generation=detailEpoch.current,abort=new AbortController();controller.current=abort;setBusy(true);setError(undefined);
  async function receive(){
   try{
    const value=await api<WorkspaceWriteCommand>('/workspace/writes/'+encodeURIComponent(id)+query(project),undefined,'GET',undefined,abort.signal);
    if(abort.signal.aborted||generation!==detailEpoch.current)return;
    if(!validWriteMetadata(value,project)||value.id!==id||prior&&(value.request_sha256!==prior.request_sha256||value.version<prior.version))throw new UiError('remote-workspace-write.invalid_receipt');
    setSelected({api,value});setBusy(false);
    if(remoteWritePending(value))timer.current=setTimeout(()=>void receive(),1500);
   }catch(reason){if(!abort.signal.aborted&&generation===detailEpoch.current){setSelected(undefined);setBusy(false);setError(reason);}}
  }
  await receive();
 }
 async function inspect(){
  if(!command||busy)return;
  stop();setBusy(true);setError(undefined);const generation=detailEpoch.current;
  const original=inspection.current??{api,id:command.id,body:{project_id:project,expected_version:command.version,idempotency_key:operation()}};inspection.current=original;
  try{
   const value=await original.api<WorkspaceWriteCommand>('/workspace/writes/'+encodeURIComponent(original.id)+'/inspect',original.body);
   if(generation!==detailEpoch.current)return;
   if(!validWriteMetadata(value,project)||value.id!==original.id||value.request_sha256!==command.request_sha256)throw new UiError('remote-workspace-write.invalid_receipt');
   inspection.current=undefined;setSelected({api,value});setRefresh(value=>value+1);await detail(value.id,value);
  }catch(reason){if(generation===detailEpoch.current){setError(reason);setBusy(false);}}
 }
 return <details className="remote-write-history" open={open} onToggle={event=>setOpen(event.currentTarget.open)}><summary>{t('history')}</summary>
  {open&&<><div className="remote-write-toolbar"><p className="muted">{t('history_help')}</p><button type="button" disabled={loading} onClick={()=>setRefresh(value=>value+1)}>{t('refresh_history')}</button></div>{loading&&<Loading/>}{!!error&&<ErrorNotice error={error}/>}
   {records&&<><p className="muted">{t('history_count',{loaded:formatNumber(records.items.length),total:formatNumber(records.total)})}</p>{!records.items.length?<Empty title={t('history_empty')}/>:<div className="table-scroll"><table><thead><tr><th>{t('file')}</th><th>{t('state')}</th><th>{t('created')}</th><th>{t('details')}</th></tr></thead><tbody>{records.items.map(item=><tr key={item.id}><td>{item.path}</td><td>{t('state.'+item.state)}</td><td>{formatDateTime(item.created_at,{dateStyle:'short',timeStyle:'short'})}</td><td><button type="button" disabled={busy} aria-label={t('open_command',{id:item.id.slice(0,8)})} onClick={()=>{inspection.current=undefined;void detail(item.id,item);}}>{t('view')}</button></td></tr>)}</tbody></table></div>}{records.truncated&&<p className="muted">{t('history_truncated')}</p>}</>}
   {command&&<section className="remote-write-receipt" aria-label={t('receipt')}><div className="remote-heading"><strong>{t('state.'+command.state)}</strong><button type="button" disabled={busy} onClick={()=>void detail(command.id,command)}>{t('refresh_command')}</button></div><dl><dt>{t('command')}</dt><dd><code>{command.id}</code></dd><dt>{t('file')}</dt><dd>{command.path}</dd><dt>{t('desired_sha')}</dt><dd><code>{command.content_sha256}</code></dd>{command.result&&<><dt>{t('generation')}</dt><dd>{formatNumber(command.result.generation)}</dd></>}</dl>{command.error_code&&<p>{t('error.'+command.error_code)}</p>}{command.state==='reconciling'&&<><p>{t('reconciliation_help')}</p><button type="button" disabled={busy||remoteWritePending(command)} onClick={()=>void inspect()}>{inspection.current?t('retry_inspection'):t('inspect')}</button></>}<p className="muted">{t('quality_boundary')}</p></section>}
  </>}
 </details>;
}
