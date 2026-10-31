import {useCallback,useEffect,useRef,useState} from 'react';
import {useApi,useApiView} from './api-context';
import {operation} from './api';
import {tr,UiError,useLocale} from './i18n';
import {ErrorNotice} from './ui';
import {toolSessionStorage} from './tool-hooks';
import {useLeaveGuard} from './write-action';
import {DesktopToolConnector} from './DesktopToolConnector';
import type {DesktopToolConnection} from './desktop-tool-connector-model';
import {ToolConnectionDiagnostic} from './ToolConnectionDiagnostic';
import {desktopCodexBridge,desktopCodexRecoveryScope,makeDesktopCodexRequest,readDesktopCodexResult,validDesktopCodexConfiguration,validDesktopCodexRequest,validDesktopCodexScope,type DesktopCodexRequest,type DesktopCodexObservation,type DesktopCodexConfiguration,type DesktopCodexScope,type DesktopCodexBridge,type DesktopCodexConnectedProof} from './desktop-codex-connector-model';
import './CodexLocalConnector.css';

export type CodexLocalConnectorProps={project:string;disabled?:boolean;secondary?:boolean;onConnected:(proof:DesktopCodexConnectedProof)=>void;onBlockedChange?:(blocked:boolean)=>void;onRegistrationReadyChange?:(ready:boolean)=>void};
type SavedRequest=Extract<DesktopCodexRequest,{action:'models'}>;
type Draft={installation_id:string;provider_id:string;base_url:string;api_key_env_ref:string;windows_sandbox:string};
const emptyDraft=():Draft=>({installation_id:'',provider_id:'',base_url:'',api_key_env_ref:'',windows_sandbox:''});
const t=(key:string,values?:Record<string,unknown>)=>tr('codex-quick-connect.'+key,values);
const sourceKey=(draft:Draft)=>JSON.stringify([draft.installation_id,draft.provider_id,draft.base_url,draft.api_key_env_ref,draft.windows_sandbox]);
const fromRequest=(request:SavedRequest):Draft=>({...emptyDraft(),...request.configuration,installation_id:request.installation_id});
function safeDraft(value:unknown):value is Draft{
 if(!value||typeof value!=='object'||Array.isArray(value))return false;
 const row=value as Record<string,unknown>,keys=Object.keys(emptyDraft());
 return Object.keys(row).length===keys.length&&keys.every(key=>typeof row[key]==='string')&&/^(?:[a-f0-9]{32})?$/.test(String(row.installation_id))&&String(row.provider_id).length<=64&&/^[A-Za-z0-9_-]*$/.test(String(row.provider_id))&&String(row.base_url).length<=2048&&!/[\p{Cc}@?#]/u.test(String(row.base_url))&&/^[A-Za-z0-9_]{0,128}$/.test(String(row.api_key_env_ref))&&['','unelevated','elevated','mxc'].includes(String(row.windows_sandbox));
}
function readLocal(scope:DesktopCodexScope,platform:DesktopCodexBridge['platform']){
 const key=desktopCodexRecoveryScope(scope),storage=toolSessionStorage();let draft=emptyDraft(),request:SavedRequest|undefined,error:unknown;
 try{
  if(!storage)throw Error();const saved=storage.getItem(key+':operation');
  if(saved){if(saved.length>16384)throw Error();const value:unknown=JSON.parse(saved);if(!validDesktopCodexRequest(value,scope,platform)||value.action!=='models')throw Error();request=value;}
  const savedDraft=storage.getItem(key+':draft');if(savedDraft){if(savedDraft.length>8192)throw Error();const value:unknown=JSON.parse(savedDraft);if(!safeDraft(value))throw Error();draft=value;}else if(request)draft=fromRequest(request);
 }catch{error=new UiError('codex-quick-connect.local_record_unavailable');}
 return {draft,request,error};
}

export function CodexLocalConnector(props:CodexLocalConnectorProps){
 const api=useApi(),view=useApiView(),bridge=desktopCodexBridge(),owner=useRef({api,method:bridge?.manageLocalTool,epoch:0});
 if(owner.current.api!==api||owner.current.method!==bridge?.manageLocalTool)owner.current={api,method:bridge?.manageLocalTool,epoch:owner.current.epoch+1};
 const scope:DesktopCodexScope={project:props.project,userID:view?.userID||'',organizationID:view?.organizationID||'',adapterID:'codex-app-server'};
 if(!bridge)return <p className="codex-local-muted">{t('desktop_required')}</p>;
 if(!validDesktopCodexScope(scope))return null;
 return <LocalCodexController key={desktopCodexRecoveryScope(scope)+':'+owner.current.epoch} {...props} scope={scope} bridge={bridge}/>;
}

function LocalCodexController({project,scope,bridge,onConnected,onBlockedChange,onRegistrationReadyChange,disabled=false,secondary=false}:CodexLocalConnectorProps&{scope:DesktopCodexScope;bridge:DesktopCodexBridge}){
 const locale=useLocale(),key=desktopCodexRecoveryScope(scope),[initial]=useState(()=>readLocal(scope,bridge.platform));
 const [draft,setDraft]=useState(initial.draft),[saved,setSaved]=useState(initial.request),[recordError,setRecordError]=useState(initial.error);
 const [snapshot,setSnapshot]=useState<{observation?:DesktopCodexObservation;error?:unknown;reason?:string}>(),[busy,setBusy]=useState<DesktopCodexRequest['action']|undefined>('model-status'),[uncertain,setUncertain]=useState(!!initial.request),[managedBlocked,setManagedBlocked]=useState(false),[clock,setClock]=useState(Date.now),[active,setActive]=useState<DesktopToolConnection>();
 const alive=useRef(true),inflight=useRef(false),original=useRef(saved),currentDraft=useRef(draft),currentObservation=useRef<DesktopCodexObservation|undefined>(undefined),activeConnection=useRef<DesktopToolConnection|undefined>(undefined),providerField=useRef<HTMLInputElement>(null),notifications=useRef({onConnected,onBlockedChange,onRegistrationReadyChange}),reported=useRef(''),polls=useRef(0),edited=useRef(false);
 original.current=saved;currentDraft.current=draft;notifications.current={onConnected,onBlockedChange,onRegistrationReadyChange};
 const observation=snapshot?.observation,denied=observation?.phase==='authorization_required'||observation?.phase==='binding_changed';
 const pending=uncertain||observation?.phase==='reading'||observation?.phase==='stop_unconfirmed';
 const blocking=!!busy||pending||managedBlocked;
 useLeaveGuard(blocking,error=>setSnapshot(value=>({...value,error})));
 useEffect(()=>{notifications.current.onBlockedChange?.(blocking);},[blocking]);
 useEffect(()=>()=>notifications.current.onBlockedChange?.(false),[]);
 const store=useCallback((value:typeof snapshot)=>{currentObservation.current=value?.observation;setSnapshot(value);},[]);
 function edit(next:Draft){
  if(disabled||busy||pending||managedBlocked||denied)return;edited.current=true;setDraft(next);
  try{const storage=toolSessionStorage();if(!storage||!safeDraft(next))throw Error();storage.setItem(key+':draft',JSON.stringify(next));setRecordError(undefined);}catch{setRecordError(new UiError('codex-quick-connect.draft_not_saved'));}
 }
 const perform=useCallback(async(request:DesktopCodexRequest)=>{
  if(inflight.current||!alive.current)return;inflight.current=true;setBusy(request.action);
  try{
   const raw=await bridge.manageLocalTool(request);if(!alive.current)return;
   const operationID=request.action==='models'||request.action==='model-status'?request.operation_id:undefined,result=readDesktopCodexResult(raw,scope,operationID);
   if(!result)throw new UiError('codex-quick-connect.invalid_observation');
   if(result.status==='failed'){
    const revoked=['managed_runner_access_changed','authorization_required','desktop_codex_access_changed'].includes(result.code);
    if(revoked){activeConnection.current=undefined;setActive(undefined);reported.current='';store({observation:{project_id:scope.project,user_id:scope.userID,tenant_id:scope.organizationID,adapter_id:'codex-app-server',phase:'authorization_required',installations:[]},reason:result.code});}
    else store({observation:currentObservation.current,error:new UiError('codex-quick-connect.read_failed'),reason:result.code});
    if(original.current)setUncertain(true);return;
   }
   const next=result.observation,requestSource=original.current,probe=next.probe;
   if(requestSource&&probe?.operation_id===requestSource.operation_id&&probe.installation_id!==requestSource.installation_id)throw new UiError('codex-quick-connect.invalid_observation');
   if(next.phase==='authorization_required'||next.phase==='binding_changed'){activeConnection.current=undefined;setActive(undefined);reported.current='';store({observation:{...next,installations:[],probe:undefined}});setUncertain(!!requestSource);return;}
   store({observation:next});
   setUncertain(next.phase==='unknown'||next.phase==='stop_unconfirmed'||probe?.discovery?.error_code==='codex_discovery_not_stopped'||!!requestSource&&request.action!=='detect'&&probe?.operation_id!==requestSource.operation_id);
   if(!currentDraft.current.installation_id){const installations=next.installations.filter(row=>row.kind==='native'&&row.tool_version&&row.executable_sha256);if(installations.length===1){const value={...currentDraft.current,installation_id:installations[0].installation_id};setDraft(value);currentDraft.current=value;try{toolSessionStorage()?.setItem(key+':draft',JSON.stringify(value));}catch{setRecordError(new UiError('codex-quick-connect.draft_not_saved'));}}}
  }catch(error){if(alive.current){setSnapshot(value=>({...value,error}));if(original.current)setUncertain(true);}}
  finally{if(alive.current){inflight.current=false;setBusy(undefined);}}
 },[bridge.manageLocalTool,key,scope.project,scope.userID,scope.organizationID]);
 const performNow=useRef(perform);performNow.current=perform;
 useEffect(()=>{alive.current=true;void performNow.current(makeDesktopCodexRequest('model-status',scope,{locale,...(original.current?{operation_id:original.current.operation_id}:{})},bridge.platform));return()=>{alive.current=false;};},[]);
 useEffect(()=>{if(observation?.phase!=='reading'||uncertain||disabled)return;const timer=setInterval(()=>{if(inflight.current||document.visibilityState==='hidden')return;if(++polls.current>30){setUncertain(true);return;}const id=original.current?.operation_id??currentObservation.current?.probe?.operation_id;if(id)void performNow.current(makeDesktopCodexRequest('model-status',scope,{locale,operation_id:id},bridge.platform));},1000);return()=>clearInterval(timer);},[observation?.phase,uncertain,disabled,locale]);
 const probe=observation?.probe,matching=!!saved&&probe?.operation_id===saved.operation_id&&probe.installation_id===saved.installation_id&&sourceKey(draft)===sourceKey(fromRequest(saved));
 const discovery=matching&&probe?.discovery?.state==='ready'?probe.discovery:undefined,catalogue=discovery?.catalogue;
 const fresh=!!discovery&&!!catalogue&&observation?.phase==='ready'&&!pending&&!denied&&!recordError&&Date.parse(probe!.expires_at)>Math.max(clock,Date.now());
 useEffect(()=>{if(!probe?.expires_at)return;const timer=setTimeout(()=>setClock(Date.now()),Math.max(0,Date.parse(probe.expires_at)-Date.now()));return()=>clearTimeout(timer);},[probe?.id,probe?.expires_at]);
 const selection=fresh?{installation_id:draft.installation_id,probe_id:probe!.id}:undefined;
 const pristine=!edited.current&&!saved&&sourceKey(draft)===sourceKey(emptyDraft());
 const expectedSHA=discovery&&!pending&&!denied?discovery.configuration_sha256:pristine?active?.configuration_sha256:undefined;
 const expectedCatalog=catalogue?.catalog_sha256??(pristine?active?.catalog_sha256:undefined);
 const registrationReady=!!(!blocking&&!denied&&!recordError&&!snapshot?.error&&active?.phase==='connected'&&active.project_id===project&&active.adapter_id==='codex-app-server'&&active.worker_id&&active.tool_version&&active.executable_sha256&&expectedSHA&&expectedCatalog&&active.configuration_sha256===expectedSHA&&active.catalog_sha256===expectedCatalog&&(!discovery||active.executable_sha256===discovery.executable_sha256&&active.tool_version===discovery.tool_version));
 useEffect(()=>{notifications.current.onRegistrationReadyChange?.(registrationReady);},[registrationReady,onRegistrationReadyChange]);
 useEffect(()=>()=>notifications.current.onRegistrationReadyChange?.(false),[]);
 const configuration={provider_id:draft.provider_id,base_url:draft.base_url,...(draft.api_key_env_ref?{api_key_env_ref:draft.api_key_env_ref}:{}),...(draft.windows_sandbox?{windows_sandbox:draft.windows_sandbox}:{})};
 const validConfiguration=validDesktopCodexConfiguration(configuration,bridge.platform);
 const installation=observation?.installations.find(row=>row.installation_id===draft.installation_id&&row.kind==='native');
 const editBlocked=disabled||!!busy||pending||managedBlocked||denied;
 async function detect(){if(editBlocked||recordError)return;polls.current=0;await performNow.current(makeDesktopCodexRequest('detect',scope,{locale},bridge.platform));}
 async function models(){
  if(editBlocked||recordError||!installation||!validConfiguration)return;
  try{const request=makeDesktopCodexRequest('models',scope,{locale,installation_id:draft.installation_id,operation_id:operation().replace(/-/g,''),configuration:configuration as DesktopCodexConfiguration},bridge.platform) as SavedRequest;const storage=toolSessionStorage();if(!storage)throw new UiError('codex-quick-connect.local_record_unavailable');storage.setItem(key+':operation',JSON.stringify(request));original.current=request;setSaved(request);setUncertain(false);polls.current=0;await performNow.current(request);}catch{setSnapshot(value=>({...value,error:new UiError('codex-quick-connect.local_record_unavailable')}));}
 }
 async function recover(){if(disabled||inflight.current)return;const local=readLocal(scope,bridge.platform);setRecordError(local.error);if(local.error)return;original.current=local.request;setSaved(local.request);await performNow.current(makeDesktopCodexRequest('model-status',scope,{locale,...(local.request?{operation_id:local.request.operation_id}:{})},bridge.platform));}
 const connection=useCallback((value:DesktopToolConnection)=>{if(!alive.current)return;activeConnection.current=value;setActive(value);},[]);
 function connected(workerID:string){const value=activeConnection.current;if(!alive.current||denied||pending||!expectedSHA||!expectedCatalog||value?.phase!=='connected'||value.project_id!==project||value.adapter_id!=='codex-app-server'||value.worker_id!==workerID||value.configuration_sha256!==expectedSHA||value.catalog_sha256!==expectedCatalog||!value.tool_version||!value.executable_sha256||discovery&&(value.executable_sha256!==discovery.executable_sha256||value.tool_version!==discovery.tool_version))return;const proof={worker_id:workerID,tool_version:value.tool_version,executable_sha256:value.executable_sha256,configuration_sha256:expectedSHA,catalog_sha256:expectedCatalog},identity=JSON.stringify(proof);if(reported.current===identity)return;reported.current=identity;notifications.current.onConnected(proof);}
 const phase=observation?.phase;
 const reason=snapshot?.reason??probe?.discovery?.error_code??(phase&&!['unconfigured','detected','ready','expired'].includes(phase)?phase:installation&&!validConfiguration?'configuration_required':undefined);
 return <section className="codex-local" aria-label={t('title')} aria-busy={!!busy}>
  <header><h3>{t('title')}</h3><p className="codex-local-muted">{t('intro')}</p></header>
  {!!recordError&&<ErrorNotice error={recordError}/>} {!!snapshot?.error&&<ErrorNotice error={snapshot.error}/>}
  <div className="codex-local-actions"><button type="button" data-codex-detect disabled={!!editBlocked||!!recordError} onClick={()=>void detect()}>{t(busy==='detect'?'detecting':'detect')}</button><button type="button" data-codex-status disabled={disabled||!!busy} onClick={()=>void recover()}>{t('check_original')}</button></div>
  <ToolConnectionDiagnostic reasonCode={reason} originalOperationPending={pending} disabled={disabled||!!busy} onDetect={()=>void detect()} onConfigure={()=>providerField.current?.focus()} onReconcile={()=>void recover()}/>
  {!denied&&<>
   {(observation?.installations.length||draft.installation_id||draft.provider_id)?<div className="codex-local-configuration">
    <label>{t('installation')}<select name="installation_id" value={draft.installation_id} disabled={!!editBlocked} onChange={event=>edit({...draft,installation_id:event.target.value})}><option value="">{t('choose_installation')}</option>{observation?.installations.map(row=><option key={row.installation_id} value={row.installation_id} disabled={row.kind!=='native'}>{row.label}{row.tool_version?' · '+row.tool_version:''}</option>)}</select></label>
    <div className="codex-local-fields"><label>{t('provider')}<input ref={providerField} name="provider_id" value={draft.provider_id} disabled={!!editBlocked} autoComplete="off" spellCheck={false} maxLength={64} onChange={event=>edit({...draft,provider_id:event.target.value})}/></label><label>{t('base_url')}<input name="base_url" value={draft.base_url} disabled={!!editBlocked} autoComplete="off" spellCheck={false} maxLength={2048} type="url" onChange={event=>edit({...draft,base_url:event.target.value})}/></label></div>
    <label>{t('credential_ref')}<input name="api_key_env_ref" value={draft.api_key_env_ref} disabled={!!editBlocked} autoComplete="off" spellCheck={false} maxLength={128} onChange={event=>edit({...draft,api_key_env_ref:event.target.value})}/></label><p className="codex-local-muted">{t('credential_hint')}</p>
    {bridge.platform==='win32'&&<label>{t('windows_sandbox')}<select name="windows_sandbox" value={draft.windows_sandbox} disabled={!!editBlocked} onChange={event=>edit({...draft,windows_sandbox:event.target.value})}><option value="">{t('choose_sandbox')}</option>{['unelevated','elevated','mxc'].map(value=><option key={value} value={value}>{t('sandbox_'+value)}</option>)}</select></label>}
    <button type="button" data-codex-models disabled={!!editBlocked||!!recordError||!installation||!validConfiguration} onClick={()=>void models()}>{t(busy==='models'?'reading_catalogue':'read_catalogue')}</button>
   </div>:null}
   {catalogue&&<div className="codex-local-catalogue"><p role="status">{t(fresh?'catalogue_ready':'catalogue_expired',{value:catalogue.models.length})}</p><p className="codex-local-muted">{t('selection_later')}</p><details><summary>{t('catalogue_details')}</summary><ul>{catalogue.models.map(model=><li key={model.model}><code>{model.model}</code></li>)}</ul><dl><dt>{t('configuration_sha')}</dt><dd><code>{discovery!.configuration_sha256}</code></dd><dt>{t('catalogue_sha')}</dt><dd><code>{catalogue.catalog_sha256}</code></dd></dl></details></div>}
   {active?.configuration_sha256&&active.configuration_sha256!==expectedSHA&&<p className="codex-local-muted" role="status">{t('draft_not_connected')}</p>}
   <DesktopToolConnector project={project} adapterID="codex-app-server" secondary={secondary} disabled={disabled} codexSelection={selection} expectedConfigurationSHA={expectedSHA} onConnection={connection} onConnected={connected} onBlockedChange={setManagedBlocked} onConfigure={()=>providerField.current?.focus()}/>
  </>}
 </section>;
}
