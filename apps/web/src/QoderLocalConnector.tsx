import {useCallback,useEffect,useRef,useState} from 'react';
import {ChevronDown,Monitor,RefreshCw} from 'lucide-react';
import {useApi,useApiView} from './api-context';
import {operation} from './api';
import {tr,UiError,useLocale} from './i18n';
import {ErrorNotice} from './ui';
import {toolSessionStorage} from './tool-hooks';
import {useLeaveGuard} from './write-action';
import {DesktopToolConnector} from './DesktopToolConnector';
import type {DesktopToolConnection} from './desktop-tool-connector-model';
import {ACPQuickConnectSelection} from './ACPQuickConnectSelection';
import {selectedQuickACPModel,validQuickACPCatalogue,type ACPQuickSelectionState} from './acp-quick-connect-selection-model';
import {desktopACPBridge,desktopACPRecoveryScope,makeDesktopACPRequest,readDesktopACPResult,validDesktopACPRequest,validDesktopACPScope,type DesktopACPRequest,type DesktopACPObservation,type DesktopACPSelection,type DesktopACPScope,type DesktopACPBridge} from './desktop-acp-connector-model';

const t=(key:string)=>tr('acp-quick-connect.'+key);
type Props={project:string;onConnected:(workerID:string)=>void;secondary?:boolean;disabled?:boolean};
type SavedRequest=Extract<DesktopACPRequest,{action:'models'}>;
type LocalRecord={request?:SavedRequest;error?:unknown};
type Snapshot={observation?:DesktopACPObservation;error?:unknown};
const refusalPhases=new Set(['authorization_required','binding_changed','stop_unconfirmed']);
const sourceKey=(scope:DesktopACPScope,installation:string,credential:string)=>JSON.stringify([desktopACPRecoveryScope(scope),installation,credential]);
function readRecord(key:string,scope:DesktopACPScope):LocalRecord{
 try{
  const storage=toolSessionStorage();if(!storage)throw new UiError('acp-quick-connect.storage_failed');
  const raw=storage.getItem(key);if(!raw)return {};
  if(raw.length>4096)throw new UiError('acp-quick-connect.original_unavailable');
  const value:unknown=JSON.parse(raw);
  if(!validDesktopACPRequest(value,scope)||value.action!=='models')throw new UiError('acp-quick-connect.original_unavailable');
  return {request:value};
 }catch{return {error:new UiError('acp-quick-connect.original_unavailable')};}
}

/** The native owner holds the executable, credentials and process. This view
 * retains the exact observation locator and never turns a status read into a
 * second NewSession, a connection or a tool registration. */
export function QoderLocalConnector(props:Props){
 const api=useApi(),view=useApiView(),bridge=desktopACPBridge(),owner=useRef({api,method:bridge?.inspectLocalACP,epoch:0});
 if(owner.current.api!==api||owner.current.method!==bridge?.inspectLocalACP)owner.current={api,method:bridge?.inspectLocalACP,epoch:owner.current.epoch+1};
 const scope:DesktopACPScope={project:props.project,userID:view?.userID||'',organizationID:view?.organizationID||'',adapterID:'qoder-acp'};
 if(!bridge||!validDesktopACPScope(scope))return null;
 return <LocalACPController key={JSON.stringify([desktopACPRecoveryScope(scope),owner.current.epoch])} {...props} scope={scope} bridge={bridge}/>;
}

function LocalACPController({project,scope,bridge,onConnected,secondary=false,disabled=false}:Props&{scope:DesktopACPScope;bridge:DesktopACPBridge}){
 const locale=useLocale(),key='suma-desktop-acp-observation:'+desktopACPRecoveryScope(scope);
 const [record,setRecord]=useState<LocalRecord>(()=>readRecord(key,scope)),[snapshot,setSnapshot]=useState<Snapshot>(),[busy,setBusy]=useState<DesktopACPRequest['action']|undefined>('status'),[uncertain,setUncertain]=useState(false),[clock,setClock]=useState(()=>Date.now());
 const [installationID,setInstallationID]=useState(''),[modelID,setModelID]=useState(''),[credentialRef,setCredentialRef]=useState(''),[draftEdited,setDraftEdited]=useState(false),[active,setActive]=useState<DesktopToolConnection>();
 const [selected,setSelected]=useState<{selection:DesktopACPSelection;sha:string;source:string}>();
 const alive=useRef(true),inflight=useRef(false),reads=useRef(0),original=useRef(record.request),currentObservation=useRef<DesktopACPObservation|undefined>(undefined),inputRestored=useRef(false);
 original.current=record.request;
 const observation=snapshot?.observation,denied=!!observation&&refusalPhases.has(observation.phase);
 const pending=uncertain||observation?.phase==='reading';
 useLeaveGuard(!!busy||pending,message=>setSnapshot(value=>({...value,error:message})));
 const store=(value:Snapshot)=>{currentObservation.current=value.observation;setSnapshot(value);};
 const clearSelection=()=>{setInstallationID('');setModelID('');setSelected(undefined);setActive(undefined);setCredentialRef('');inputRestored.current=false;};
 const perform=useCallback(async(request:DesktopACPRequest)=>{
  if(inflight.current||!alive.current)return;
  inflight.current=true;setBusy(request.action);
  try{
   const raw=await bridge.inspectLocalACP(request);if(!alive.current)return;
   const operationID=request.action==='models'||request.action==='status'?request.operation_id:undefined;
   const result=readDesktopACPResult(raw,scope,operationID);
   if(!result)throw new UiError('acp-quick-connect.invalid_observation');
   if(result.status==='failed'){
    if(['managed_runner_access_changed','authorization_required','desktop_acp_access_changed','acp_discovery_authorization_changed'].includes(result.code)){clearSelection();store({observation:{project_id:scope.project,user_id:scope.userID,tenant_id:scope.organizationID,adapter_id:'qoder-acp',phase:'authorization_required',installations:[]}});}
    else store({observation:currentObservation.current,error:new UiError('acp-quick-connect.observation_failed')});
    if(original.current)setUncertain(true);return;
   }
   const next=result.observation;
   if(refusalPhases.has(next.phase)){clearSelection();store({observation:{...next,installations:[],probe:undefined}});setUncertain(!!original.current);return;}
   store({observation:next});
   const saved=original.current,probe=next.probe;
   if(saved&&probe?.operation_id===saved.operation_id&&probe.installation_id!==saved.installation_id)throw new UiError('acp-quick-connect.invalid_observation');
   if(saved&&probe?.operation_id===saved.operation_id){
    if(!inputRestored.current){setInstallationID(saved.installation_id);setCredentialRef(saved.credential_env_ref||'');inputRestored.current=true;}
    setUncertain(next.phase==='unknown'||next.phase==='stop_unconfirmed');
   }else if(!saved)setUncertain(next.phase==='unknown');
   if(!saved&&!installationID){const native=next.installations.filter(item=>item.kind==='native'&&item.tool_version&&item.executable_sha256);if(native.length===1)setInstallationID(native[0].installation_id);}
  }catch(error){if(alive.current){setSnapshot(value=>({...value,error}));if(original.current)setUncertain(true);}}
  finally{if(alive.current){inflight.current=false;setBusy(undefined);}}
 },[bridge,scope.project,scope.userID,scope.organizationID,scope.adapterID]);
 const performNow=useRef(perform);performNow.current=perform;
 useEffect(()=>{
  alive.current=true;
  const request=makeDesktopACPRequest('status',scope,{locale,...(original.current?{operation_id:original.current.operation_id}:{})});void performNow.current(request);
  return()=>{alive.current=false;};
 },[]);
 useEffect(()=>{
  if(observation?.phase!=='reading'||uncertain||disabled)return;
  const timer=window.setInterval(()=>{
   if(document.visibilityState==='hidden'||inflight.current)return;
   if(++reads.current>30){setUncertain(true);return;}
   const operationID=original.current?.operation_id??currentObservation.current?.probe?.operation_id;
   if(operationID)void performNow.current(makeDesktopACPRequest('status',scope,{locale,operation_id:operationID}));
  },1000);
  return()=>window.clearInterval(timer);
 },[observation?.phase,uncertain,disabled,locale]);
 const currentSource=sourceKey(scope,installationID,credentialRef),probe=observation?.probe,saved=record.request;
 const catalogueMatches=!!saved&&probe?.operation_id===saved.operation_id&&probe.installation_id===saved.installation_id;
 const phase:ACPQuickSelectionState['phase']=uncertain?'unknown':busy==='models'?'reading':observation?.phase==='ready'?(catalogueMatches?'ready':'idle'):observation?.phase==='reading'?'reading':observation?.phase==='expired'?'expired':observation?.phase==='unknown'||observation?.phase==='stop_unconfirmed'?'unknown':observation?.phase==='failed'||observation?.phase==='unavailable'?'failed':'idle';
 const catalogue=probe?.discovery?.catalogue;
 const selectionState:ACPQuickSelectionState={installations:(observation?.installations||[]).map(item=>({id:item.installation_id,label:item.label,version:item.tool_version,kind:item.kind})),installationID,modelID,phase,sourceKey:currentSource,...(catalogueMatches&&saved?{catalogueSourceKey:sourceKey(scope,saved.installation_id,saved.credential_env_ref||'')}:{ }),catalogue:catalogueMatches&&validQuickACPCatalogue(catalogue)?catalogue:undefined,expiresAt:probe?.expires_at};
 useEffect(()=>{if(!probe?.expires_at)return;const timer=window.setTimeout(()=>setClock(Date.now()),Math.max(0,Date.parse(probe.expires_at)-Date.now()));return()=>window.clearTimeout(timer);},[probe?.id,probe?.expires_at]);
 const choice=selectedQuickACPModel(selectionState,Math.max(clock,Date.now())),freshSelection=choice?.configuration_sha256&&probe?{installation_id:installationID,probe_id:probe.id,model_id:choice.id}:undefined;
 const expectedSHA=selected?.source===currentSource?selected.sha:!draftEdited?active?.configuration_sha256:undefined;
 const desiredMatches=!!expectedSHA&&active?.configuration_sha256===expectedSHA&&(!selected||active.model===selected.selection.model_id);
 const editBlocked=disabled||!!busy||pending||!!record.error;
 function updateDraft(){setDraftEdited(true);setModelID('');setSelected(undefined);}
 async function detect(){if(editBlocked)return;updateDraft();reads.current=0;await performNow.current(makeDesktopACPRequest('detect',scope,{locale}));}
 async function discover(){
  if(editBlocked||denied)return;
  try{
   const request=makeDesktopACPRequest('models',scope,{locale,installation_id:installationID,operation_id:operation().replace(/-/g,''),...(credentialRef?{credential_env_ref:credentialRef}:{})}) as SavedRequest;
   const storage=toolSessionStorage();if(!storage)throw new UiError('acp-quick-connect.storage_failed');
   storage.setItem(key,JSON.stringify(request));original.current=request;setRecord({request});inputRestored.current=true;setDraftEdited(true);setModelID('');setSelected(undefined);reads.current=0;setUncertain(false);
   await performNow.current(request);
  }catch(error){setSnapshot(value=>({...value,error}));}
 }
 async function recover(){
  if(disabled||inflight.current)return;
  const loaded=readRecord(key,scope);setRecord(loaded);original.current=loaded.request;
  if(loaded.error)return;
  await performNow.current(makeDesktopACPRequest('status',scope,{locale,...(loaded.request?{operation_id:loaded.request.operation_id}:{})}));
 }
 const connection=useCallback((value:DesktopToolConnection)=>setActive(value),[]);
 return <section className="desktop-tool-connector" aria-label={t('title')} aria-busy={!!busy}>
  <details open={!secondary} className="qoder-local-configuration"><summary><Monitor size={16}/>{t('title')}<ChevronDown size={15}/></summary><div className="desktop-tool-content">
   {!!record.error&&<ErrorNotice error={record.error}/>}{!!snapshot?.error&&<ErrorNotice error={snapshot.error}/>}
   {denied?<p role="status">{t('access_changed')}</p>:<>
    <ACPQuickConnectSelection state={selectionState} disabled={disabled||!!record.error||!!busy} detecting={busy==='detect'} canDiscover={!editBlocked&&!denied&&!!installationID} onInspect={()=>void detect()} onDiscover={()=>void discover()} onRecover={()=>void recover()} onInstallationChange={value=>{if(!editBlocked){updateDraft();setInstallationID(value);}}} onModelChange={value=>{
     const selectedChoice=selectedQuickACPModel({...selectionState,modelID:value});if(!selectedChoice?.configuration_sha256||!probe)return;
     setDraftEdited(true);setModelID(value);setSelected({source:currentSource,sha:selectedChoice.configuration_sha256,selection:{installation_id:installationID,probe_id:probe.id,model_id:value}});
    }}/>
    <details className="qoder-local-credentials"><summary>{t('credential_settings')}</summary><label>{t('credential_reference')}<input data-acp-credential-reference autoComplete="off" spellCheck={false} value={credentialRef} disabled={editBlocked} onChange={event=>{if(!editBlocked){updateDraft();setCredentialRef(event.target.value);}}}/></label><p className="muted">{t('credential_hint')}</p></details>
   </>}
   <button type="button" data-acp-observation-status disabled={disabled||!!busy} onClick={()=>void recover()}><RefreshCw size={14}/>{t('check_observation')}</button>
   {draftEdited&&active?.configuration_sha256&&!desiredMatches&&<p className="muted" role="status">{t('draft_not_connected')}</p>}
   {!denied&&<DesktopToolConnector project={project} adapterID="qoder-acp" secondary={secondary} disabled={disabled} onConnected={onConnected} onConnection={connection} acpSelection={freshSelection} expectedConfigurationSHA={expectedSHA}/>}
  </div></details>
 </section>;
}
