import {useCallback,useEffect,useRef,useState} from 'react';
import {useApi,useApiView} from './api-context';
import {tr,useLocale} from './i18n';
import {toolSessionStorage} from './tool-hooks';
import {useLeaveGuard} from './write-action';
import {DesktopToolConnector} from './DesktopToolConnector';
import {desktopToolBridge,readDesktopToolResult,validDesktopToolScope,type DesktopToolConnection,type DesktopToolScope} from './desktop-tool-connector-model';
import {readInstalledToolDiscovery,type InstalledToolFact} from './tool-quick-connect-model';
import {localToolsScanBridge} from './local-tools-scan-model';
import {ToolConnectionDiagnostic} from './ToolConnectionDiagnostic';
import {makeDesktopOpenCodeSetup,openCodeConfigurationSHA,validDesktopOpenCodeConfiguration,type DesktopOpenCodeConnectedProof,type DesktopOpenCodeSetup} from './desktop-opencode-connector-model';
import {openCodeEndpointOrigin} from './opencode-execution-model';
import './OpenCodeLocalConnector.css';

export type OpenCodeLocalConnectorProps={project:string;disabled?:boolean;secondary?:boolean;onConnected:(proof:DesktopOpenCodeConnectedProof)=>void;onBlockedChange?:(blocked:boolean)=>void;onRegistrationReadyChange?:(ready:boolean)=>void;onGuide?:()=>void};
type Draft={provider_id:string;model:string;base_url:string;api_key_env_ref:string};
const emptyDraft=():Draft=>({provider_id:'',model:'',base_url:'',api_key_env_ref:''});
const t=(key:string)=>tr('opencode-quick-connect.'+key);
const draftKey=(scope:DesktopToolScope)=>'suma:opencode-local:'+JSON.stringify(scope);
const sourceKey=(value:Draft)=>JSON.stringify([value.provider_id,value.model,value.base_url,value.api_key_env_ref]);
const configuration=(draft:Draft)=>({provider_id:draft.provider_id,model:draft.model,base_url:draft.base_url,...(draft.api_key_env_ref?{api_key_env_ref:draft.api_key_env_ref}:{})});
function safeDraft(value:unknown):value is Draft{
 if(!value||typeof value!=='object'||Array.isArray(value))return false;const row=value as Record<string,unknown>,keys=Object.keys(emptyDraft());
 if(Object.keys(row).length!==4||!keys.every(key=>typeof row[key]==='string')||!/^[A-Za-z0-9_.-]{0,100}$/.test(String(row.provider_id))||!/^[A-Za-z0-9._:/@+\-]{0,256}$/.test(String(row.model))||!/^[A-Z0-9_]{0,128}$/.test(String(row.api_key_env_ref))||String(row.base_url).length>2048||/[\p{Cc}\u2028\u2029?#@]/u.test(String(row.base_url)))return false;
 return true;
}
function readLocal(scope:DesktopToolScope){
 const key=draftKey(scope);let draft=emptyDraft(),failed=false;
 try{const storage=toolSessionStorage();if(!storage)throw Error();const raw=storage.getItem(key+':draft');if(raw){if(raw.length>8192)throw Error();const value:unknown=JSON.parse(raw);if(!safeDraft(value))throw Error();draft=value;}}catch{failed=true;}
 return {draft,failed};
}
function setupMatches(setup:DesktopOpenCodeSetup,value:DesktopToolConnection){
 return value.phase==='connected'&&value.tool_version==='1.19.4'&&value.executable_sha256===setup.executable_sha256&&value.provider_id===setup.provider_id&&value.model===setup.model&&value.endpoint_origin===openCodeEndpointOrigin(setup.base_url);
}
export function OpenCodeLocalConnector(props:OpenCodeLocalConnectorProps){
 useLocale();const api=useApi(),view=useApiView(),bridge=desktopToolBridge(),detector=localToolsScanBridge(),owner=useRef({api,manage:bridge?.manageLocalTool,detect:detector?.discoverInstalledTools,epoch:0});
 if(owner.current.api!==api||owner.current.manage!==bridge?.manageLocalTool||owner.current.detect!==detector?.discoverInstalledTools)owner.current={api,manage:bridge?.manageLocalTool,detect:detector?.discoverInstalledTools,epoch:owner.current.epoch+1};
 const scope={project:props.project,userID:view?.userID??'',organizationID:view?.organizationID??'',adapterID:'opencode-sdk'};
 if(!bridge||!detector)return <p className="opencode-local-muted">{t('desktop_required')}</p>;
 if(!validDesktopToolScope(scope))return null;
 // The existing bridge accepts the original single-adapter discovery request.
 const discover=detector.discoverInstalledTools as unknown as (request:{project_id:string;user_id:string;tenant_id:string;adapter_id:string;locale:string})=>Promise<unknown>;
 return <LocalOpenCodeController key={draftKey(scope)+owner.current.epoch} {...props} scope={scope} discover={discover}/>;
}
function LocalOpenCodeController({project,scope,discover,onConnected,onBlockedChange,onRegistrationReadyChange,onGuide,disabled=false,secondary=false}:OpenCodeLocalConnectorProps&{scope:DesktopToolScope;discover:(request:{project_id:string;user_id:string;tenant_id:string;adapter_id:string;locale:string})=>Promise<unknown>}){
 const locale=useLocale(),[initial]=useState(()=>readLocal(scope)),[draft,setDraft]=useState(initial.draft),[storageFailed,setStorageFailed]=useState(initial.failed),[fact,setFact]=useState<InstalledToolFact>(),[readFailed,setReadFailed]=useState(false),[busy,setBusy]=useState(false),[managedBlocked,setManagedBlocked]=useState(false),[active,setActive]=useState<DesktopToolConnection>(),[digest,setDigest]=useState<{key:string;sha?:string}>();
 const alive=useRef(true),attempted=useRef(false),inflight=useRef(false),provider=useRef<HTMLInputElement>(null),reported=useRef(''),callbacks=useRef({onConnected,onBlockedChange,onRegistrationReadyChange});callbacks.current={onConnected,onBlockedChange,onRegistrationReadyChange};
 const denied=!!active&&['authorization_required','binding_changed','origin_changed','credential_unrecoverable'].includes(active.phase),blocking=busy||managedBlocked;
 useLeaveGuard(blocking,()=>setReadFailed(true));
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;callbacks.current.onBlockedChange?.(false);callbacks.current.onRegistrationReadyChange?.(false);};},[]);
 useEffect(()=>{callbacks.current.onBlockedChange?.(blocking);},[blocking]);
 async function detect(){
  if(disabled||busy||managedBlocked||denied||inflight.current)return;inflight.current=true;setBusy(true);setReadFailed(false);
  try{const raw=await discover({project_id:project,user_id:scope.userID,tenant_id:scope.organizationID,adapter_id:'opencode-sdk',locale});if(!alive.current)return;const value=readInstalledToolDiscovery(raw,'opencode-sdk');setFact(value);if(!value)setReadFailed(true);}catch{if(alive.current){setFact(undefined);setReadFailed(true);}}finally{if(alive.current){inflight.current=false;setBusy(false);}}
 }
 useEffect(()=>{if(!disabled&&!managedBlocked&&!denied&&active&&!attempted.current){attempted.current=true;void detect();}},[disabled,managedBlocked,denied,active?.phase]);
 const configured=validDesktopOpenCodeConfiguration(configuration(draft)),supported=fact?.installed&&fact.reason_code==='configuration_required'&&fact.version==='1.19.4';
 const candidate=supported?makeDesktopOpenCodeSetup(configuration(draft),fact.executable_sha256??''):undefined;
 const digestKey=candidate&&fact?.executable?JSON.stringify([sourceKey(draft),fact.executable,fact.executable_sha256,fact.version]):'';
 useEffect(()=>{if(!digestKey||!fact?.executable)return;let current=true;const source=configuration(draft),executable=fact.executable;void openCodeConfigurationSHA(source,executable).then(sha=>{if(current&&alive.current)setDigest({key:digestKey,sha});}).catch(()=>{if(current&&alive.current)setDigest({key:digestKey});});return()=>{current=false;};},[digestKey]);
 const verifiedDigest=digestKey&&digest?.key===digestKey?digest.sha:undefined;
 const setup=!blocking&&!denied&&!storageFailed&&!readFailed&&verifiedDigest?candidate:undefined;
 const expectedSHA=verifiedDigest;
 const exactConfiguration=!!candidate&&!!active&&!!verifiedDigest&&active.configuration_sha256===verifiedDigest&&setupMatches(candidate,active);
 const registrationReady=!!(!blocking&&!denied&&!storageFailed&&!readFailed&&active?.phase==='connected'&&active.project_id===project&&active.adapter_id==='opencode-sdk'&&active.worker_id&&active.model&&active.tool_version==='1.19.4'&&active.executable_sha256&&active.configuration_sha256&&active.provider_id&&active.endpoint_origin&&exactConfiguration);
 useEffect(()=>{callbacks.current.onRegistrationReadyChange?.(registrationReady);if(!registrationReady||!active)return;const proof={worker_id:active.worker_id!,model:active.model!,tool_version:active.tool_version!,executable_sha256:active.executable_sha256!,configuration_sha256:active.configuration_sha256!,provider_id:active.provider_id!,endpoint_origin:active.endpoint_origin!},identity=JSON.stringify([proof,active.credential_version]);if(reported.current!==identity){reported.current=identity;callbacks.current.onConnected(proof);}},[registrationReady,active,onRegistrationReadyChange]);
 const receive=useCallback((value:DesktopToolConnection)=>{if(!alive.current)return;const result=readDesktopToolResult({status:'ok',connection:value},scope);if(result?.status==='ok'){if(result.connection.phase==='stopped')reported.current='';setActive(result.connection);}},[project,scope.userID,scope.organizationID]);
 function edit(next:Draft){if(disabled||blocking||denied)return;setDraft(next);try{const storage=toolSessionStorage();if(!storage||!safeDraft(next))throw Error();storage.setItem(draftKey(scope)+':draft',JSON.stringify(next));setStorageFailed(false);}catch{setStorageFailed(true);}}
 const ignoreWorker=useCallback(()=>{},[]),reason=fact?.reason_code==='version_unsupported'?undefined:readFailed?'probe_failed':fact?.reason_code==='configuration_required'&&configured?undefined:fact?.reason_code;
 return <section className="opencode-local" aria-label={t('title')} aria-busy={busy}>
  <header><h3>{t('title')}</h3><p>{t('intro')}</p></header>
  <div className="opencode-local-actions"><button type="button" data-opencode-detect disabled={disabled||blocking||denied} onClick={()=>void detect()}>{t(busy?'detecting':'detect')}</button>{onGuide&&<button type="button" disabled={disabled||blocking} onClick={onGuide}>{t('guide')}</button>}</div>
  {storageFailed&&<p role="alert">{t('storage_failed')}</p>}
  {fact&&<p role="status">{t(fact.installed?'installed':'not_installed')}{fact.version?' · '+fact.version:''}</p>}
  {fact?.reason_code==='version_unsupported'&&<p role="status">{t('version_unsupported')}</p>}
  {!!digestKey&&digest?.key===digestKey&&!digest.sha&&<p role="status">{t('verification_unavailable')}</p>}
  <ToolConnectionDiagnostic reasonCode={reason} disabled={disabled||blocking} onDetect={()=>void detect()} onConfigure={()=>provider.current?.focus()} onGuide={onGuide}/>
  <fieldset disabled={disabled||blocking||denied} className="opencode-local-fields"><legend>{t('configuration')}</legend>
   <label>{t('provider')}<input name="provider_id" ref={provider} value={draft.provider_id} maxLength={100} autoComplete="off" spellCheck={false} onChange={event=>edit({...draft,provider_id:event.target.value})}/></label>
   <label>{t('model')}<input name="model" value={draft.model} maxLength={256} autoComplete="off" spellCheck={false} onChange={event=>edit({...draft,model:event.target.value})}/></label>
   <label className="opencode-local-wide">{t('base_url')}<input name="base_url" type="url" value={draft.base_url} maxLength={2048} autoComplete="off" spellCheck={false} onChange={event=>edit({...draft,base_url:event.target.value})}/></label>
   <label className="opencode-local-wide">{t('credential_ref')}<input name="api_key_env_ref" value={draft.api_key_env_ref} maxLength={128} autoComplete="off" spellCheck={false} onChange={event=>edit({...draft,api_key_env_ref:event.target.value})}/></label>
   <p className="opencode-local-wide opencode-local-muted">{t('credential_hint')}</p>
  </fieldset>
  <p className="opencode-local-muted">{t('model_hint')}</p>
  {active?.phase==='connected'&&!registrationReady&&!blocking&&<p role="status">{t('draft_not_adopted')}</p>}
  <DesktopToolConnector project={project} adapterID="opencode-sdk" secondary={secondary} disabled={disabled||busy} opencodeSetup={setup} expectedConfigurationSHA={expectedSHA} onConnection={receive} onConnected={ignoreWorker} onBlockedChange={setManagedBlocked} onConfigure={()=>provider.current?.focus()} onGuide={onGuide}/>
 </section>;
}
