import {useEffect,useLayoutEffect,useRef,useState} from 'react';
import {ApiError} from './api';
import {useApi,useApiView} from './api-context';
import {useWriteAction} from './write-action';
import {toolSessionStorage} from './tool-hooks';
import {readManagedToolPreparation,readManagedToolPreparationPage,managedToolPreparationListPath,managedToolPreparationPath,managedToolPreparationUnresolved,validManagedToolPreparationRequest,validManagedToolPreparationReceipt,validManagedToolPreparationConfiguration,validManagedToolPreparationCapability,canPrepareManagedToolItem,type ToolPreparationOperation} from './managed-tool-preparation-model';
import {remoteToolScanScopeKey,type RemoteToolScan,type RemoteToolScanItem,type RemoteToolScanScope} from './remote-tool-scan-model';
import {readToolReportFeedback,toolReportFeedbackRemainingMs,type ToolReportFeedbackSelection} from './tool-report-feedback-model';
import {toolDiscoveryTimestampNs} from './tool-discovery-model';
import {sameToolProfile} from './tool-profile-reuse-model';
import {Field,ErrorNotice} from './ui';
import {managedToolPreparationMode,managedToolPreparationSupports} from './managed-tool-preparation-capability-model';
import {preparationForLocalTool} from './tool-preparation-target';
import {tr,useLocale,UiError} from './i18n';
import './ManagedToolPreparationPanel.css';

export type ManagedToolPreparationPanelProps={project:string;workerID:string;scan?:RemoteToolScan;item?:RemoteToolScanItem;selectedScanID?:string;selectionEpoch?:number;disabled?:boolean;newOperationsBlocked?:boolean;onBlockedChange?:(blocked:boolean)=>void;onPreparedConfiguration?:(selection:ToolReportFeedbackSelection,preparationID:string)=>void};
const t=(key:string)=>tr('managed-tool-preparation.'+key);
const denied=(error:unknown)=>error instanceof ApiError&&[401,403,404].includes(error.status);
type Page=Awaited<ReturnType<typeof readManagedToolPreparationPage>>;
/** This remains mounted while a write is uncertain, even if its old scan or
 * capability disappears. Only the original body/key can recover that write. */
export function ManagedToolPreparationPanel(props:ManagedToolPreparationPanelProps){
 useLocale();const api=useApi(),view=useApiView(),key=JSON.stringify([view?.organizationID,view?.userID,props.project,props.workerID]),owner=useRef({api,key,epoch:0});
 if(owner.current.api!==api||owner.current.key!==key)owner.current={api,key,epoch:owner.current.epoch+1};
 if(!view?.userID||!view.organizationID)return null;
 const captured=owner.current,scope={project:props.project,workerID:props.workerID,userID:view.userID,organizationID:view.organizationID};
 return <PreparationController key={key+':'+captured.epoch} {...props} scope={scope} owns={()=>owner.current===captured}/>;
}
function PreparationController({scope,scan,item,selectedScanID,selectionEpoch,disabled=false,newOperationsBlocked=false,onBlockedChange,onPreparedConfiguration,owns}:ManagedToolPreparationPanelProps&{scope:RemoteToolScanScope;owns:()=>boolean}){
 useLocale();const api=useApi(),storage=toolSessionStorage(),scopeKey=remoteToolScanScopeKey(scope),selectionKey='suma-managed-preparation-selected:'+scopeKey;
 const [selected,setSelected]=useState(()=>{try{return storage?.getItem(selectionKey)||'';}catch{return '';}}),selectedRef=useRef(selected);selectedRef.current=selected;
 const [template,setTemplate]=useState<{scope:string;id:string}>();
 const [page,setPage]=useState<Page>(),[operation,setOperation]=useState<ToolPreparationOperation>(),[loading,setLoading]=useState(true),[error,setError]=useState<unknown>(),[hidden,setHidden]=useState(false),[model,setModel]=useState(''),[credential,setCredential]=useState(''),[acknowledged,setAcknowledged]=useState(false),[timedOut,setTimedOut]=useState(false),[,tick]=useState(0);
 const alive=useRef(true),epoch=useRef(0),flight=useRef<AbortController|undefined>(undefined),timer=useRef<ReturnType<typeof setTimeout>|undefined>(undefined),waiting=useRef<{id:string;until:number}|undefined>(undefined),deadline=useRef(0),submitting=useRef(false),latest=useRef({disabled,newOperationsBlocked,onBlockedChange,onPreparedConfiguration,scan,item});latest.current={disabled,newOperationsBlocked,onBlockedChange,onPreparedConfiguration,scan,item};
 const newActionOwner=useRef({blocked:newOperationsBlocked});if(newActionOwner.current.blocked!==newOperationsBlocked)newActionOwner.current={blocked:newOperationsBlocked};
 const action=useWriteAction({scope:'managed-tool-preparation:'+scopeKey,storage,validateRequest:request=>validManagedToolPreparationRequest(request,scope),validateResponse:async(value,request)=>alive.current&&owns()&&await validManagedToolPreparationReceipt(value,request,scope)&&alive.current&&owns()});
 // Navigation ownership is separate from the durable operation. Switching a
 // scan retires only the old read-to-Quick intent; it never clears a write.
 const sourceKey=JSON.stringify([selectedScanID,selectionEpoch,scan?.id,scan?.version,item?.adapter_id,item?.executable_sha256]),sourceOwner=useRef({key:sourceKey,epoch:0});if(sourceOwner.current.key!==sourceKey)sourceOwner.current={key:sourceKey,epoch:sourceOwner.current.epoch+1};
 const acknowledgedSource=useRef<string|undefined>(undefined);
 const pending=!!action.pending,busy=action.busy,unresolved=!!operation&&managedToolPreparationUnresolved(operation),blocked=pending||busy||unresolved||action.persistenceBlocked;
 const pendingConfiguration=action.pending?.body.configuration;
 const pendingTemplateID=action.pending?.body.adapter_id==='codex-app-server'&&validManagedToolPreparationConfiguration('codex-app-server',pendingConfiguration)?pendingConfiguration.template_id:undefined;
 const maintenance=page?.maintenance.status,cap=maintenance?.managed_preparation,capCurrent=maintenance?.state==='supported'&&validManagedToolPreparationCapability(cap)&&performance.now()<deadline.current;
 const source=scan?.state==='completed'&&!scan.source_changed&&item&&canPrepareManagedToolItem(item)&&scan.result?.items.some(value=>value.adapter_id===item.adapter_id&&JSON.stringify(value)===JSON.stringify(item))?{scan,item}:undefined;
 const mode=managedToolPreparationMode(source?.item.adapter_id),codex=source?.item.adapter_id==='codex-app-server';
 const capabilityMatchesSource=capCurrent&&!!source&&managedToolPreparationSupports(cap,source.item.adapter_id)&&source.scan.instance_id===maintenance.instance_id;
 const templates=codex&&capabilityMatchesSource?cap.codex_template_ids:undefined;
 const templateScope=templates&&cap?JSON.stringify([sourceOwner.current.epoch,cap.generation_sha256,templates]):undefined;
 const templateID=templates?.length===1?templates[0]:template?.scope===templateScope?template?.id:undefined;
 const configuration={model:mode==='existing_local_configuration'?'':model,credential_env_ref:mode==='model_and_credential'?credential:'',...(templateID?{template_id:templateID}:{})};
 const consentKey=codex?JSON.stringify([sourceKey,templateScope,templateID]):sourceKey;
 const hasConsent=acknowledged&&acknowledgedSource.current===consentKey;
 const canCreate=!disabled&&!newOperationsBlocked&&!hidden&&!loading&&!error&&!blocked&&!action.conflict&&capabilityMatchesSource&&(!codex||!!templateID)&&validManagedToolPreparationConfiguration(source.item.adapter_id,configuration)&&hasConsent;
 function chooseTemplate(id:string){
  if(!templateScope||!templates?.includes(id)||id===templateID)return;
  acknowledgedSource.current=undefined;setAcknowledged(false);setTemplate({scope:templateScope,id});
 }
 function choose(id:string){selectedRef.current=id;setSelected(id);try{storage?.setItem(selectionKey,id);}catch{/* The write request, not this navigation locator, is durable authority. */}}
 function stop(){clearTimeout(timer.current);timer.current=undefined;}
 function current(sequence:number,controller:AbortController){return alive.current&&owns()&&epoch.current===sequence&&!controller.signal.aborted;}
 function deny(){stop();++epoch.current;flight.current?.abort();setPage(undefined);setOperation(undefined);setHidden(true);setLoading(false);deadline.current=0;}
 function schedule(value:ToolPreparationOperation|undefined,reset=false){
  stop();if(!value||!managedToolPreparationUnresolved(value)){waiting.current=undefined;return;}
  if(reset||waiting.current?.id!==value.id){waiting.current={id:value.id,until:performance.now()+60000};setTimedOut(false);}
  if(performance.now()>=waiting.current.until){setTimedOut(true);return;}
  if(document.visibilityState!=='hidden')timer.current=setTimeout(()=>void read(value.id),Math.min(3000,waiting.current!.until-performance.now()));
 }
 async function read(id=selectedRef.current,reset=false){
  if(flight.current||!alive.current||!owns())return;const controller=new AbortController(),sequence=++epoch.current,started=performance.now();flight.current=controller;setLoading(true);setError(undefined);
  try{
   const next=await readManagedToolPreparationPage(await api<unknown>(managedToolPreparationListPath(scope.project,scope.workerID),undefined,'GET',undefined,controller.signal),scope);if(!current(sequence,controller))return;
   let value=next.items.find(row=>row.id===id);if(id&&!value)value=await readManagedToolPreparation(await api<unknown>(managedToolPreparationPath(scope.project,id),undefined,'GET',undefined,controller.signal),scope,id);if(!current(sequence,controller))return;
   if(!id&&next.items.length){value=next.items[0];choose(value.id);}
   deadline.current=performance.now()+Math.max(0,next.maintenance.remainingMs-(performance.now()-started));setPage(next);setOperation(value);setHidden(false);setLoading(false);if(action.errorStatus&&[401,403,404].includes(action.errorStatus))action.setError('');if(reset&&!action.pending&&!next.items.some(managedToolPreparationUnresolved))action.clearConflict();schedule(value,reset);
  }catch(failure){if(!current(sequence,controller))return;if(denied(failure))deny();else{setError(failure);setLoading(false);stop();}}
  finally{if(flight.current===controller)flight.current=undefined;}
 }
 async function create(restore=false){
  if(submitting.current||busy||!owns()||!alive.current||hidden||disabled||!restore&&(latest.current.newOperationsBlocked||!canCreate))return;submitting.current=true;
  try{const original=action.pending,body=original?.body??{project_id:scope.project,worker_id:scope.workerID,installation_scan_id:source!.scan.id,expected_scan_version:source!.scan.version,adapter_id:source!.item.adapter_id,configuration,expected_generation_sha256:cap!.generation_sha256,acknowledge_configuration_write:true};const value=await action.run<ToolPreparationOperation>(original?.path??'/tool-preparations',body,original?.method??'POST');if(!owns()||!alive.current)return;if(value){choose(value.id);setOperation(value);}void read(value?.id||selectedRef.current,true);}
  finally{submitting.current=false;tick(value=>value+1);}
 }
 async function useInventory(){
  if(!owns()||!alive.current||disabled||newOperationsBlocked||hidden||loading||error||pending||busy||operation?.state!=='completed'||operation.source_changed||!operation.receipt||selectedScanID&&operation.installation_scan_id!==selectedScanID||!latest.current.onPreparedConfiguration||flight.current)return;
  const original=operation,controller=new AbortController(),sequence=++epoch.current,started=performance.now(),selectionOwner=sourceOwner.current,actionOwner=newActionOwner.current,callback=latest.current.onPreparedConfiguration;
  const ownsSelection=()=>current(sequence,controller)&&sourceOwner.current===selectionOwner&&newActionOwner.current===actionOwner&&!latest.current.newOperationsBlocked;flight.current=controller;setLoading(true);setError(undefined);
  try{
   const head=await readManagedToolPreparation(await api<unknown>(managedToolPreparationPath(scope.project,original.id),undefined,'GET',undefined,controller.signal),scope,original.id);if(!ownsSelection())return;
   if(head.state!=='completed'||head.source_changed||head.version!==original.version||head.configuration.template_id!==original.configuration.template_id||!head.receipt||!sameToolProfile(head.receipt.profile,original.receipt!.profile))throw new UiError('managed-tool-preparation.inventory_changed');
   const [catalog,inventory]=await Promise.all([api<unknown>('/adapter-catalog?project_id='+encodeURIComponent(scope.project),undefined,'GET',undefined,controller.signal),api<unknown>('/tool-discovery?project_id='+encodeURIComponent(scope.project)+'&worker_id='+encodeURIComponent(scope.workerID),undefined,'GET',undefined,controller.signal)]);if(!ownsSelection())return;
   const local=preparationForLocalTool(head.adapter_id);if(!local||!managedToolPreparationMode(local.localTool))throw new UiError('managed-tool-preparation.inventory_changed');
   const target={project:scope.project,workerID:scope.workerID,localTool:local.localTool},view=readToolReportFeedback(catalog,inventory,target,scope.project,performance.now()-started),choice=view?.choices.find(value=>sameToolProfile(value.profile,head.receipt!.profile));
   const selection:ToolReportFeedbackSelection|undefined=view&&choice&&view.discovery.observedAt&&toolReportFeedbackRemainingMs(view,0)>0?{project:scope.project,workerID:scope.workerID,adapterID:head.adapter_id,commandID:choice.profile.command_id,profileSHA256:choice.profile.profile_sha256!,observedAt:view.discovery.observedAt}:undefined;
   if(selection&&toolDiscoveryTimestampNs(selection.observedAt)<toolDiscoveryTimestampNs(head.inventory_observed_at))throw new UiError('managed-tool-preparation.inventory_changed');
   if(!selection)throw new UiError('managed-tool-preparation.inventory_changed');if(ownsSelection()&&!latest.current.disabled)callback(selection,head.id);
  }catch(failure){if(!ownsSelection())return;if(denied(failure))deny();else setError(failure);}
  finally{if(flight.current===controller){flight.current=undefined;setLoading(false);}}
 }
 useLayoutEffect(()=>{alive.current=true;return()=>{alive.current=false;++epoch.current;flight.current?.abort();flight.current=undefined;stop();latest.current.onBlockedChange?.(false);};},[]);
 useEffect(()=>{void read();},[]);
 useEffect(()=>{latest.current.onBlockedChange?.(blocked);},[blocked]);
 useEffect(()=>{if(action.errorStatus&&[401,403,404].includes(action.errorStatus))deny();},[action.errorStatus]);
 useEffect(()=>{const remaining=deadline.current-performance.now();if(remaining<=0)return;const handle=setTimeout(()=>tick(value=>value+1),Math.ceil(remaining));return()=>clearTimeout(handle);},[page]);
 useEffect(()=>{const visibility=()=>{if(document.visibilityState==='hidden'){stop();++epoch.current;flight.current?.abort();flight.current=undefined;setLoading(false);}else if(waiting.current){if(performance.now()>=waiting.current.until)setTimedOut(true);else void read(waiting.current.id);}};document.addEventListener('visibilitychange',visibility);return()=>document.removeEventListener('visibilitychange',visibility);},[]);
 useEffect(()=>{setTemplate(undefined);},[templateScope]);
 useEffect(()=>{setModel('');setCredential('');setAcknowledged(false);acknowledgedSource.current=undefined;},[consentKey]);
 if(!source&&!operation&&!pending&&!error)return null;
 const state=hidden?'forbidden':pending?'write_unknown':operation?.state??'configure';
 return <section className="managed-tool-preparation" data-managed-tool-preparation data-managed-preparation-state={state} aria-label={t('title')} aria-busy={loading||busy}>
  <header><div><h4>{t('title')}</h4><p>{scope.workerID}</p></div><button type="button" data-managed-preparation-refresh disabled={loading||busy} onClick={()=>void read(selectedRef.current,true)}>{t('refresh')}</button></header>
  <p role="status">{t('state.'+state)}</p>
  {!!(error||action.error)&&<ErrorNotice error={action.error||error}/>}
  {pending&&!hidden&&<>
   {pendingTemplateID&&<p data-managed-preparation-pending-template>{t('codex_original_template')} <strong>{pendingTemplateID}</strong></p>}
   <button type="button" data-managed-preparation-recover disabled={busy||disabled} onClick={()=>void create(true)}>{t('recover')}</button>
  </>}
  {!hidden&&!pending&&source&&!unresolved&&<div className="managed-tool-preparation-form" data-managed-preparation-mode={mode}>
   <p>{t('selected')} <strong>{codex?t('codex_name'):source.item.adapter_id}</strong>{codex&&source.item.version?' · '+source.item.version:null}</p>
   {mode!=='existing_local_configuration'&&<Field label={t('model')} hint={mode==='fixed_claude_model'?t('claude_model_hint'):undefined}><input data-managed-preparation-model value={model} disabled={disabled||busy} maxLength={256} onChange={event=>setModel(event.target.value)}/></Field>}
   {mode==='model_and_credential'&&<Field label={t('credential')} hint={t(source.item.adapter_id==='qoder-acp'?'credential_optional':'credential_required')}><input data-managed-preparation-credential value={credential} disabled={disabled||busy} maxLength={128} autoComplete="off" spellCheck={false} onChange={event=>setCredential(event.target.value)}/></Field>}
   {mode==='existing_local_configuration'&&(!codex||capabilityMatchesSource)&&<p data-managed-preparation-existing>{t(codex?'codex_existing_configuration':'existing_configuration')}</p>}
   {templates?.length===1&&<p data-managed-preparation-fixed-template>{t('codex_template')} <strong>{templates[0]}</strong></p>}
   {templates&&templates.length>1&&<Field label={t('codex_template')}>
    <select data-managed-preparation-template required value={templateID??''} disabled={disabled||loading||busy} onChange={event=>chooseTemplate(event.target.value)}>
     <option value="" disabled>{t('codex_template_placeholder')}</option>
     {templates.map(id=><option key={id} value={id}>{id}</option>)}
    </select>
   </Field>}
   {!capabilityMatchesSource&&!loading&&<p role="status">{t(codex&&capCurrent&&source.scan.instance_id!==maintenance.instance_id?'codex_rescan_required':codex&&capCurrent?'codex_capability_unavailable':'capability_unavailable')}</p>}
   {(!codex||capabilityMatchesSource)&&<>
    <label className="managed-tool-preparation-consent"><input type="checkbox" data-managed-preparation-confirm checked={hasConsent} disabled={disabled||busy||codex&&!templateID} onChange={event=>{acknowledgedSource.current=event.target.checked?consentKey:undefined;setAcknowledged(event.target.checked);}}/><span>{t('acknowledge')}</span></label>
    <button type="button" data-managed-preparation-create disabled={!canCreate} onClick={()=>void create()}>{t('create')}</button>
   </>}
  </div>}
  {!hidden&&operation&&<div className="managed-tool-preparation-progress"><code>{operation.id}</code>{!pending&&operation.configuration.template_id&&<p data-managed-preparation-operation-template>{t('codex_template')} <strong>{operation.configuration.template_id}</strong></p>}{operation.result_code&&<p>{t('result.'+operation.result_code)}</p>}{operation.receipt&&<><p>{t('configuration_accepted')}</p><code>{operation.receipt.profile.profile_sha256}</code></>}{operation.source_changed&&<p>{t('source_changed')}</p>}{timedOut&&unresolved&&<p>{t('wait_finished')}</p>}{operation.state==='completed'&&onPreparedConfiguration&&<button type="button" data-managed-preparation-use disabled={disabled||newOperationsBlocked||loading||busy||pending||!!error||!!operation.source_changed||!!selectedScanID&&operation.installation_scan_id!==selectedScanID} onClick={()=>void useInventory()}>{t('use')}</button>}</div>}
  <p className="managed-tool-preparation-boundary">{t('boundary')}</p>
 </section>;
}
