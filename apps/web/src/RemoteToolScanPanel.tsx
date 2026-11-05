import {useEffect,useLayoutEffect,useRef,useState} from 'react';
import {ChevronRight,RefreshCw,Search} from 'lucide-react';
import {ApiError} from './api';
import {useApi,useApiView} from './api-context';
import {readAdapterCatalog,type AdapterDescriptor} from './AdapterConnections';
import {useWriteAction} from './write-action';
import {toolError,toolSessionStorage} from './tool-hooks';
import {toolPreparationTarget} from './tool-preparation-target';
import {toolConnectionDiagnostic} from './tool-connection-diagnostics-model';
import {readRemoteToolScan,readRemoteToolScans,remoteToolScanListPath,remoteToolScanPath,remoteToolScanScopeKey,validRemoteToolScanScope,validRemoteToolScanRequest,validRemoteToolScanReceipt,type RemoteToolScan,type RemoteToolScanItem,type RemoteToolScanScope} from './remote-tool-scan-model';
import {validInstalledDiscoveryCapability} from './tool-installation-capability-model';
import {ManagedToolPreparationPanel} from './ManagedToolPreparationPanel';
import {validManagedToolPreparationCapability} from './managed-tool-preparation-capability-model';
import {canPrepareManagedToolItem} from './managed-tool-preparation-model';
import type {ToolReportFeedbackSelection} from './tool-report-feedback-model';
import {hasUnverifiedInstallationStop} from './tool-installation-probe-diagnostic-model';
import {tr,useLocale,formatNumber,UiError} from './i18n';
import './RemoteToolScanPanel.css';

export type RemoteToolScanPanelProps={project:string;workerID:string;selectedAdapterID?:string;disabled?:boolean;onPrepare?:(adapter:AdapterDescriptor,workerID:string)=>void;onGuide?:()=>void;onBlockedChange?:(blocked:boolean)=>void;onPreparedConfiguration?:(selection:ToolReportFeedbackSelection,preparationID:string)=>void};
type Page=Awaited<ReturnType<typeof readRemoteToolScans>>;
type Snapshot={page?:Page;catalog?:AdapterDescriptor[];receivedAt?:number;capDeadline:number;loading:boolean;denied?:boolean;error?:unknown};
const t=(key:string,values?:Record<string,unknown>)=>tr('remote-tool-scan.'+key,values);
const probeText=(key:string)=>tr('tool-installation-probe-diagnostics.'+key);
const unresolved=(scan:RemoteToolScan)=>['pending','claimed','unknown'].includes(scan.state);
const id=(value:unknown):value is string=>typeof value==='string'&&/^[A-Za-z0-9][A-Za-z0-9_.-]{0,149}$/.test(value);
const denied=(error:unknown)=>error instanceof ApiError&&[401,403,404].includes(error.status);

/** Remote facts always come from this authenticated node. Desktop discovery
 * is deliberately absent: it cannot attest a server or another machine. */
export function RemoteToolScanPanel(props:RemoteToolScanPanelProps){
 useLocale();const api=useApi(),view=useApiView(),key=JSON.stringify([props.project,props.workerID,view?.userID,view?.organizationID]),owner=useRef({api,key,epoch:0});
 if(owner.current.api!==api||owner.current.key!==key)owner.current={api,key,epoch:owner.current.epoch+1};
 if(!view?.userID||!view.organizationID)return null;
 const scope={project:props.project,workerID:props.workerID,userID:view.userID,organizationID:view.organizationID};if(!validRemoteToolScanScope(scope))return null;
 const epoch=owner.current.epoch,owns=()=>owner.current.api===api&&owner.current.key===key&&owner.current.epoch===epoch;
 return <ScanController key={key+':'+epoch} {...props} scope={scope} owns={owns}/>;
}
function ScanController({scope,selectedAdapterID,disabled=false,onPrepare,onGuide,onBlockedChange,onPreparedConfiguration,owns}:RemoteToolScanPanelProps&{scope:RemoteToolScanScope;owns:()=>boolean}){
 useLocale();const api=useApi(),storage=toolSessionStorage(),base=remoteToolScanScopeKey(scope),selectionKey='suma-remote-tool-scan-selected:'+base;
 const [selected,setSelected]=useState(()=>{try{const value=storage?.getItem(selectionKey);return id(value)?value:'';}catch{return '';}}),selectedRef=useRef(selected);selectedRef.current=selected;
 const [snapshot,setSnapshot]=useState<Snapshot>({loading:true,capDeadline:0}),snapshotRef=useRef(snapshot);snapshotRef.current=snapshot;
 const observedStops=useRef(new Map<string,Pick<RemoteToolScan,'version'|'source_sha256'|'claim_id'|'fence'>>());
 const [managedTarget,setManagedTarget]=useState<{scanID:string;adapterID:string}>(),[managedBlocked,setManagedBlocked]=useState(false);
 const managedSelection=useRef({key:'',epoch:0});
 const managedMounted=useRef(false);if(managedTarget||validManagedToolPreparationCapability(snapshot.page?.maintenance.status.managed_preparation))managedMounted.current=true;
 if(!managedMounted.current)try{managedMounted.current=!!storage?.getItem('suma-write-recovery-v1:managed-tool-preparation:'+base)||!!storage?.getItem('suma-managed-preparation-selected:'+base);}catch{/* Existing write recovery remains fail-closed in its own mounted reader. */}
 const [timedOut,setTimedOut]=useState(false),[,tick]=useState(0),alive=useRef(true),requestEpoch=useRef(0),flight=useRef<AbortController|undefined>(undefined),submission=useRef(false),poll=useRef<ReturnType<typeof setTimeout>|undefined>(undefined),deadline=useRef<ReturnType<typeof setTimeout>|undefined>(undefined),wait=useRef<{id:string;until:number}|undefined>(undefined);
 const latest=useRef({disabled,onPrepare,onGuide,onBlockedChange});latest.current={disabled,onPrepare,onGuide,onBlockedChange};
 const action=useWriteAction({scope:'remote-tool-scan:'+base,storage,validateRequest:request=>validRemoteToolScanRequest(request,scope),validateResponse:async(value,request)=>owns()&&alive.current&&await validRemoteToolScanReceipt(value,request,scope)&&owns()&&alive.current});
 const pending=!!action.pending,writeDenied=!!action.errorStatus&&[401,403,404].includes(action.errorStatus),hidden=!!snapshot.denied||writeDenied;
 const managedKey=JSON.stringify([selected,managedTarget?.scanID,managedTarget?.adapterID,hidden]);if(managedSelection.current.key!==managedKey)managedSelection.current={key:managedKey,epoch:managedSelection.current.epoch+1};const managedOwner=managedSelection.current;
 const scan=snapshot.page?.items.find(value=>value.id===selected),activeScan=!!scan&&unresolved(scan),blocked=pending||action.busy||submission.current||action.persistenceBlocked||activeScan;
 const storedMaintenance=snapshot.page?.maintenance,cap=storedMaintenance?.status.installed_discovery,capValid=storedMaintenance?.status.state==='supported'&&validInstalledDiscoveryCapability(cap)&&performance.now()<snapshot.capDeadline;
 // Selection and bounded page omission are not proof that a process stopped.
 // This reader remounts for a different caller, actor, project or worker scope.
 const stopBlocked=observedStops.current.size>0;
 const newBlocked=disabled||hidden||snapshot.loading||!!snapshot.error||!capValid||blocked||managedBlocked||action.conflict||!!snapshot.page?.items.some(unresolved)||stopBlocked;
 function publish(value:Snapshot){
  for(const result of value.page?.items??[]){
   const known=observedStops.current.get(result.id),sameClaim=!!known&&known.source_sha256===result.source_sha256&&known.claim_id===result.claim_id&&known.fence===result.fence;
   if(hasUnverifiedInstallationStop(result.result?.items??[])){
    if(!known||sameClaim&&result.version>=known.version)observedStops.current.set(result.id,{version:result.version,source_sha256:result.source_sha256,claim_id:result.claim_id,fence:result.fence});
   }else if(known&&sameClaim&&result.version>known.version&&result.state==='closed_unknown'&&result.result?.stop_proof?.all_native_trees_stopped===true){
    // The strict result reader already binds the proof to this exact source/ref.
    observedStops.current.delete(result.id);
   }
  }
  snapshotRef.current=value;setSnapshot(value);
 }
 function stopWait(){clearTimeout(poll.current);clearTimeout(deadline.current);poll.current=undefined;deadline.current=undefined;wait.current=undefined;}
 function selectID(value:string){if(selectedRef.current!==value)managedSelection.current={...managedSelection.current,epoch:managedSelection.current.epoch+1};selectedRef.current=value;setSelected(value);try{storage?.setItem(selectionKey,value);}catch{/* Selection is navigation only; the durable write request remains separate. */}}
 function selectManagedTarget(value:{scanID:string;adapterID:string}|undefined){managedSelection.current={...managedSelection.current,epoch:managedSelection.current.epoch+1};setManagedTarget(value);}
 function abortRead(){++requestEpoch.current;flight.current?.abort();flight.current=undefined;}
 function deny(){abortRead();stopWait();publish({loading:false,capDeadline:0,denied:true});}
 function ownsRead(epoch:number,controller:AbortController){return alive.current&&owns()&&requestEpoch.current===epoch&&!controller.signal.aborted;}
 function schedule(value:RemoteToolScan|undefined,reset=false){
  clearTimeout(poll.current);if(!value||!unresolved(value)||hidden||latest.current.disabled){stopWait();return;}
  if(reset||!wait.current||wait.current.id!==value.id){
   stopWait();setTimedOut(false);wait.current={id:value.id,until:performance.now()+60000};
   deadline.current=setTimeout(()=>{
    if(!alive.current||!owns()||wait.current?.id!==value.id)return;
    clearTimeout(poll.current);
    // The scan wait bounds polling, while a full refresh has its own request deadline.
    if(!snapshotRef.current.loading)abortRead();
    setTimedOut(true);
   },60000);
  }
  if(performance.now()>=wait.current.until){setTimedOut(true);return;}if(document.visibilityState!=='hidden')poll.current=setTimeout(()=>void readCurrent(value.id),3000);
 }
 async function readCurrent(scanID:string){
  if(flight.current||!alive.current||!owns()||latest.current.disabled||document.visibilityState==='hidden')return;
  const epoch=++requestEpoch.current,controller=new AbortController();flight.current=controller;
  try{const raw=await api<unknown>(remoteToolScanPath(scope,scanID),undefined,'GET',undefined,controller.signal),value=await readRemoteToolScan(raw,scope,scanID);if(!ownsRead(epoch,controller))return;
   const before=snapshotRef.current;if(before.page)publish({...before,page:{...before.page,items:[value,...before.page.items.filter(item=>item.id!==value.id)]},loading:false,error:undefined});schedule(value);
  }catch(error){if(!ownsRead(epoch,controller))return;if(denied(error))deny();else{publish({...snapshotRef.current,loading:false,error});const original=snapshotRef.current.page?.items.find(item=>item.id===scanID);schedule(original);}}
  finally{if(flight.current===controller)flight.current=undefined;}
 }
 async function readPage(reset=false){
  if(flight.current||!alive.current||!owns())return;const epoch=++requestEpoch.current,controller=new AbortController();flight.current=controller;const started=performance.now();publish({...snapshotRef.current,loading:true,error:undefined});
  try{const [catalogRaw,pageRaw]=await Promise.all([api<unknown>('/adapter-catalog?project_id='+encodeURIComponent(scope.project),undefined,'GET',undefined,controller.signal),api<unknown>(remoteToolScanListPath(scope),undefined,'GET',undefined,controller.signal)]),page=await readRemoteToolScans(pageRaw,scope),catalog=readAdapterCatalog(catalogRaw,scope.project);if(!catalog)throw new UiError('remote-tool-scan.invalid_response');if(!ownsRead(epoch,controller))return;
   let selectedID=selectedRef.current;let value=page.items.find(item=>item.id===selectedID);
   if(selectedID&&!value){const raw=await api<unknown>(remoteToolScanPath(scope,selectedID),undefined,'GET',undefined,controller.signal);value=await readRemoteToolScan(raw,scope,selectedID);if(!ownsRead(epoch,controller))return;page.items=[value,...page.items];}
   if(!selectedID&&page.items.length){value=page.items[0];selectedID=value.id;selectID(selectedID);}
   const remaining=Math.max(0,page.maintenance.remainingMs-(performance.now()-started));publish({page,catalog:catalog.items,receivedAt:performance.now(),capDeadline:performance.now()+remaining,loading:false});schedule(value,reset);
   // A successful current read can restore visibility after an earlier 403.
   // It never acknowledges the retained write or grants it a new identity.
   if(writeDenied)action.setError('');
   if(reset&&!action.pending&&!page.items.some(unresolved))action.clearConflict();
  }catch(error){if(!ownsRead(epoch,controller))return;if(denied(error))deny();else publish({loading:false,capDeadline:0,error});}
  finally{if(flight.current===controller)flight.current=undefined;}
 }
 async function create(adapters:string[],restore=false){
  if(submission.current||action.busy||disabled||hidden||!owns()||!restore&&(newBlocked||observedStops.current.size>0))return;submission.current=true;
  try{const original=action.pending,body=original?.body??{project_id:scope.project,worker_id:scope.workerID,adapter_ids:adapters},value=await action.run<RemoteToolScan>(original?.path??'/tool-installation-scans',body,original?.method??'POST');
   if(!alive.current||!owns())return;if(value)selectID(value.id);void readPage(true);
  }finally{submission.current=false;tick(value=>value+1);}
 }
 function prepare(item:RemoteToolScanItem){
  const current=snapshotRef.current,original=current.page?.items.find(value=>value.id===selectedRef.current),candidates=current.catalog?.filter(adapter=>adapter.id===item.adapter_id&&toolPreparationTarget(adapter))??[];
  const maintenance=current.page?.maintenance.status,capability=maintenance?.installed_discovery;
  if(item.diagnostic&&(item.diagnostic.next_action!=='guide'||newBlocked||!original?.result?.items.includes(item)))return;
  if(!owns()||!alive.current||latest.current.disabled||current.denied||current.loading||current.error||pending||action.busy||maintenance?.state!=='supported'||performance.now()>=current.capDeadline||!validInstalledDiscoveryCapability(capability)||!capability.adapter_ids.includes(item.adapter_id)||original?.state!=='completed'||original.source_changed||!original.result?.items.some(value=>value.adapter_id===item.adapter_id&&value.protocol===item.protocol)||candidates.length!==1)return;latest.current.onPrepare?.(candidates[0],scope.workerID);
 }
 function recheck(item:RemoteToolScanItem){
  const current=snapshotRef.current,original=current.page?.items.find(value=>value.id===selectedRef.current),maintenance=current.page?.maintenance.status,capability=maintenance?.installed_discovery;
  if(!owns()||!alive.current||latest.current.disabled||current.denied||current.loading||current.error||newBlocked||item.diagnostic?.next_action!=='scan'||original?.state!=='completed'||original.source_changed||!original.result?.items.includes(item)||hasUnverifiedInstallationStop(original.result.items)||maintenance?.state!=='supported'||performance.now()>=current.capDeadline||!validInstalledDiscoveryCapability(capability)||!capability.adapter_ids.includes(item.adapter_id))return;
  void create([item.adapter_id]);
 }
 useLayoutEffect(()=>{alive.current=true;return()=>{alive.current=false;abortRead();stopWait();latest.current.onBlockedChange?.(false);};},[]);
 useEffect(()=>{void readPage();},[]);
 useEffect(()=>{latest.current.onBlockedChange?.(blocked||managedBlocked||stopBlocked);},[blocked,managedBlocked,stopBlocked]);
 useEffect(()=>{if(writeDenied)deny();},[writeDenied]);
 useEffect(()=>{if(!snapshot.capDeadline)return;const remaining=snapshot.capDeadline-performance.now();if(remaining<=0)return;const timer=setTimeout(()=>tick(value=>value+1),Math.ceil(remaining));return()=>clearTimeout(timer);},[snapshot.capDeadline]);
 useEffect(()=>{
  function visibility(){
   if(document.visibilityState==='hidden'){
    // An in-flight page or detail read can settle. Pause only scheduled polling.
    clearTimeout(poll.current);
   }else if(wait.current){
    if(performance.now()>=wait.current.until)setTimedOut(true);
    else void readCurrent(wait.current.id);
   }
  }
  document.addEventListener('visibilitychange',visibility);
  return()=>document.removeEventListener('visibilitychange',visibility);
 },[]);
 const status=hidden?'forbidden':action.busy?'loading':pending?'write_unknown':snapshot.loading?'loading':snapshot.error?'invalid':timedOut&&activeScan?'timed_out':scan?.state??(capValid?'ready':'capability_unavailable');
 const rows=!hidden&&!pending&&scan?.state==='completed'?scan.result?.items??[]:[],primary=rows.filter(item=>item.installed||item.reason_code!=='not_installed'||item.adapter_id===selectedAdapterID),other=rows.filter(item=>!primary.includes(item));
 function row(item:RemoteToolScanItem){const candidates=snapshot.catalog?.filter(adapter=>adapter.id===item.adapter_id&&toolPreparationTarget(adapter))??[],adapter=candidates.length===1?candidates[0]:undefined,reason=toolConnectionDiagnostic(item.reason_code==='acp_configuration_required'?'configuration_required':item.reason_code),diagnostic=item.diagnostic;
  const versionReason=item.reason_code==='version_unsupported'&&(item.adapter_id==='opencode-sdk'||item.adapter_id==='claude-code-sdk')?tr((item.adapter_id==='opencode-sdk'?'opencode-quick-connect':'claude-sdk-quick-connect')+'.version_unsupported'):undefined;
  const capability=storedMaintenance?.status.managed_preparation,managed=capValid&&validManagedToolPreparationCapability(capability)&&capability.adapter_ids.some(adapterID=>adapterID===item.adapter_id)&&canPrepareManagedToolItem(item);
  return <li key={item.adapter_id} data-remote-tool-scan-row={item.adapter_id}><div><strong>{adapter?.product||item.adapter_id}</strong><span>{item.reason_code==='scan_incomplete'?probeText('scan_incomplete'):t(item.installed?'installed':'not_installed')}{item.version?' · '+item.version:''}</span><code>{item.protocol}</code><p {...(diagnostic?{'data-tool-probe-diagnostic':diagnostic.code}:{})}>{diagnostic?probeText('code.'+diagnostic.code):versionReason??tr('tool-connection-diagnostics.reason.'+(reason?.reason??'unknown'))}</p><details><summary>{t('source_reason')}</summary>{diagnostic&&<><p>{probeText('stage_label')+': '+probeText('stage.'+diagnostic.stage)}</p><p>{probeText('hint.'+diagnostic.next_action)}</p></>}<code>{item.reason_code}</code>{item.executable_sha256&&<code>{item.executable_sha256}</code>}</details></div><div>{diagnostic?diagnostic.next_action==='guide'&&adapter&&onPrepare?<button type="button" data-remote-tool-scan-diagnostic-action="guide" disabled={newBlocked||!!scan?.source_changed} onClick={()=>prepare(item)}>{probeText('action.guide')}</button>:diagnostic.next_action==='scan'?<button type="button" data-remote-tool-scan-diagnostic-action="scan" disabled={newBlocked||!!scan?.source_changed||!requested.includes(item.adapter_id)} onClick={()=>recheck(item)}>{probeText('action.remote_scan')}</button>:null:<>{managed&&<button type="button" data-remote-tool-scan-configure={item.adapter_id} disabled={disabled||snapshot.loading||!!snapshot.error||blocked||managedBlocked||stopBlocked||!!scan?.source_changed} onClick={()=>{if(owns()&&alive.current&&!latest.current.disabled&&scan?.state==='completed'&&!scan.source_changed&&performance.now()<snapshot.capDeadline&&!blocked&&!managedBlocked&&observedStops.current.size===0)selectManagedTarget({scanID:scan.id,adapterID:item.adapter_id});}}>{tr('managed-tool-preparation.open')}</button>}{adapter&&onPrepare&&<button type="button" data-remote-tool-scan-prepare={item.adapter_id} disabled={disabled||snapshot.loading||!!snapshot.error||action.busy||managedBlocked||!capValid||!requested.includes(item.adapter_id)||!!scan?.source_changed} onClick={()=>prepare(item)}>{t('prepare')}</button>}</>}</div></li>;
 }
 const requested=capValid&&validInstalledDiscoveryCapability(cap)?cap.adapter_ids:[],single=selectedAdapterID?(requested.includes(selectedAdapterID)?[selectedAdapterID]:[]):requested;
 return <section className="remote-tool-scan" data-remote-tool-scan data-current-scan={scan?.id||''} data-remote-tool-scan-source-changed={scan?.source_changed?'true':'false'} data-remote-tool-scan-capability={hidden?'forbidden':capValid?'supported':storedMaintenance?.status.state==='supported'?'expired':storedMaintenance?.status.state||'unknown'} aria-label={t('title')} aria-busy={snapshot.loading||action.busy}>
  <header><div><h4><Search size={17} aria-hidden="true"/>{t('title')}</h4><p>{t('target',{worker:scope.workerID})}</p></div><button type="button" data-remote-tool-scan-refresh disabled={snapshot.loading||action.busy} onClick={()=>void readPage(true)}><RefreshCw size={15} aria-hidden="true"/>{t('refresh')}</button></header>
  <p role="status" data-remote-tool-scan-status={status}>{t('status.'+status)}</p>
  {!hidden&&stopBlocked&&<p role="status" className="remote-tool-scan-warning" data-remote-tool-scan-stop-blocked><strong>{probeText('code.stop_unverified')}</strong>{' '}{probeText('hint.none')}</p>}
  {!!(action.error||snapshot.error)&&<p role="alert" data-remote-tool-scan-error>{action.error||toolError(snapshot.error)}</p>}
  {!hidden&&<><div className="remote-tool-scan-actions">{pending?<button type="button" data-remote-tool-scan-recover disabled={disabled||action.busy} onClick={()=>void create([],true)}>{t('recover')}</button>:<><button type="button" data-remote-tool-scan-create disabled={newBlocked||!single.length} onClick={()=>void create(single)}>{t(selectedAdapterID?'scan_selected':'scan_all')}</button>{selectedAdapterID&&requested.length>1&&<button type="button" className="text-button" data-remote-tool-scan-create-all disabled={newBlocked} onClick={()=>void create(requested)}>{t('scan_all')}</button>}</>}
   {!capValid&&onGuide&&<button type="button" data-remote-tool-scan-guide disabled={disabled||action.busy||pending} onClick={()=>{if(owns()&&!latest.current.disabled&&!action.pending)latest.current.onGuide?.();}}>{t('enable_guide')}</button>}</div>
   {snapshot.page?.items.length? <label className="remote-tool-scan-history">{t('history')}<select data-remote-tool-scan-request value={selected} disabled={action.busy||pending||managedBlocked} onChange={event=>{stopWait();selectID(event.target.value);selectManagedTarget(undefined);setTimedOut(false);void readCurrent(event.target.value);}}>{snapshot.page.items.map(value=><option key={value.id} value={value.id}>{value.created_at} · {t('status.'+value.state)}</option>)}</select></label>:null}
   {scan?.source_changed&&<p className="remote-tool-scan-warning">{t('source_changed')}</p>}{scan?.finished_at&&<p className="remote-tool-scan-meta">{t('finished')} <time dateTime={scan.finished_at}>{scan.finished_at}</time></p>}
   {!!rows.length&&<><p>{t('counts',{detected:formatNumber(rows.filter(item=>item.installed).length),total:formatNumber(rows.length)})}</p><ul className="remote-tool-scan-list">{primary.map(row)}</ul>{other.length>0&&<details className="remote-tool-scan-other"><summary><ChevronRight size={16} aria-hidden="true"/>{t('other',{count:formatNumber(other.length)})}</summary><ul className="remote-tool-scan-list">{other.map(row)}</ul></details>}</>}
  </>}
  {managedMounted.current&&<ManagedToolPreparationPanel project={scope.project} workerID={scope.workerID} selectedScanID={selected} selectionEpoch={managedOwner.epoch} scan={!hidden?snapshot.page?.items.find(value=>value.id===managedTarget?.scanID):undefined} item={!hidden?snapshot.page?.items.find(value=>value.id===managedTarget?.scanID)?.result?.items.find(value=>value.adapter_id===managedTarget?.adapterID):undefined} disabled={disabled||hidden||pending||action.busy||activeScan} newOperationsBlocked={stopBlocked} onBlockedChange={setManagedBlocked} onPreparedConfiguration={(value,preparationID)=>{if(owns()&&alive.current&&managedSelection.current===managedOwner&&!latest.current.disabled&&!hidden&&observedStops.current.size===0&&value.project===scope.project&&value.workerID===scope.workerID)onPreparedConfiguration?.(value,preparationID);}}/>}
  <p className="remote-tool-scan-meta">{t('boundary')}</p>
 </section>;
}
