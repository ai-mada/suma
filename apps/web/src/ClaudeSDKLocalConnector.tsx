import {useCallback,useEffect,useId,useRef,useState} from 'react';
import {useApi,useApiView} from './api-context';
import {tr,useLocale} from './i18n';
import {toolSessionStorage} from './tool-hooks';
import {useLeaveGuard} from './write-action';
import {DesktopToolConnector} from './DesktopToolConnector';
import {desktopToolBridge,readDesktopToolResult,validDesktopToolScope,type DesktopToolConnection,type DesktopToolScope} from './desktop-tool-connector-model';
import {readInstalledToolDiscovery,type InstalledToolFact} from './tool-quick-connect-model';
import {localToolsScanBridge} from './local-tools-scan-model';
import {ToolConnectionDiagnostic} from './ToolConnectionDiagnostic';
import {makeDesktopClaudeSDKSetup,matchesDesktopClaudeSDKSetup,validDesktopClaudeSDKConfiguration,type DesktopClaudeSDKConfiguration,type DesktopClaudeSDKConnectedProof} from './desktop-claude-sdk-connector-model';
import './ClaudeSDKLocalConnector.css';

export type ClaudeSDKLocalConnectorProps={project:string;disabled?:boolean;secondary?:boolean;onConnected:(proof:DesktopClaudeSDKConnectedProof)=>void;onBlockedChange?:(blocked:boolean)=>void;onRegistrationReadyChange?:(ready:boolean)=>void;onGuide?:()=>void};
type Draft=DesktopClaudeSDKConfiguration;
const t=(key:string)=>tr('claude-sdk-quick-connect.'+key);
const emptyDraft=():Draft=>({model:'',credential_env_ref:''});
const draftKey=(scope:DesktopToolScope)=>'suma:claude-sdk-local:'+JSON.stringify(scope)+':draft';
function safeDraft(value:unknown):value is Draft{
 if(!value||typeof value!=='object'||Array.isArray(value)||![Object.prototype,null].includes(Object.getPrototypeOf(value)))return false;
 const row=value as Record<string,unknown>;
 return Reflect.ownKeys(row).length===2&&Object.hasOwn(row,'model')&&Object.hasOwn(row,'credential_env_ref')&&typeof row.model==='string'&&/^[a-z0-9.-]{0,256}$/.test(row.model)&&typeof row.credential_env_ref==='string'&&/^[A-Z0-9_]{0,128}$/.test(row.credential_env_ref);
}
function readLocal(scope:DesktopToolScope){
 let draft=emptyDraft(),failed=false;
 try{const storage=toolSessionStorage();if(!storage)throw Error();const raw=storage.getItem(draftKey(scope));if(raw){if(raw.length>4096)throw Error();const value:unknown=JSON.parse(raw);if(!safeDraft(value))throw Error();draft=value;}}catch{failed=true;}
 return {draft,failed};
}
const installationKey=(value:InstalledToolFact|undefined)=>value?JSON.stringify([value.executable,value.executable_sha256,value.version,value.reason_code]):'';

/** A current trusted host read proves the original public variable name and
 * complete SDK capability. The renderer never reads its value or private paths. */
export function ClaudeSDKLocalConnector(props:ClaudeSDKLocalConnectorProps){
 useLocale();const api=useApi(),view=useApiView(),bridge=desktopToolBridge(),detector=localToolsScanBridge(),owner=useRef({api,manage:bridge?.manageLocalTool,detect:detector?.discoverInstalledTools,epoch:0});
 if(owner.current.api!==api||owner.current.manage!==bridge?.manageLocalTool||owner.current.detect!==detector?.discoverInstalledTools)owner.current={api,manage:bridge?.manageLocalTool,detect:detector?.discoverInstalledTools,epoch:owner.current.epoch+1};
 const scope={project:props.project,userID:view?.userID??'',organizationID:view?.organizationID??'',adapterID:'claude-code-sdk'};
 if(!bridge||!detector)return <p className="claude-sdk-local-muted">{t('desktop_required')}</p>;
 if(!validDesktopToolScope(scope))return null;
 const discover=detector.discoverInstalledTools as unknown as (request:{project_id:string;user_id:string;tenant_id:string;adapter_id:string;locale:string})=>Promise<unknown>;
 return <LocalClaudeSDKController key={draftKey(scope)+owner.current.epoch} {...props} scope={scope} discover={discover}/>;
}
function LocalClaudeSDKController({project,scope,discover,onConnected,onBlockedChange,onRegistrationReadyChange,onGuide,disabled=false,secondary=false}:ClaudeSDKLocalConnectorProps&{scope:DesktopToolScope;discover:(request:{project_id:string;user_id:string;tenant_id:string;adapter_id:string;locale:string})=>Promise<unknown>}){
 const locale=useLocale(),hintID=useId(),[initial]=useState(()=>readLocal(scope)),[draft,setDraft]=useState(initial.draft),[storageFailed,setStorageFailed]=useState(initial.failed),[fact,setFact]=useState<InstalledToolFact>(),[readFailed,setReadFailed]=useState(false),[busy,setBusy]=useState(false),[managedBlocked,setManagedBlocked]=useState(false),[active,setActive]=useState<DesktopToolConnection>(),[needsApplication,setNeedsApplication]=useState(false);
 const alive=useRef(true),attempted=useRef(false),inflight=useRef(false),modelField=useRef<HTMLInputElement>(null),credentialField=useRef<HTMLInputElement>(null),reported=useRef(''),seenConnected=useRef(false),callbacks=useRef({onConnected,onBlockedChange,onRegistrationReadyChange});callbacks.current={onConnected,onBlockedChange,onRegistrationReadyChange};
 const denied=!!active&&['authorization_required','binding_changed','origin_changed','credential_unrecoverable'].includes(active.phase),blocking=busy||managedBlocked;
 const configured=validDesktopClaudeSDKConfiguration(draft),supported=fact?.installed&&fact.reason_code==='configuration_required'&&fact.version==='2.1.314';
 const candidate=supported?makeDesktopClaudeSDKSetup(draft,fact.executable_sha256??''):undefined;
 const source=candidate?JSON.stringify([candidate.model,candidate.credential_env_ref,candidate.executable_sha256,fact?.executable,fact?.version]):'';
 const sourceRevision=useRef(0),previousSource=useRef(source);
 if(previousSource.current!==source)sourceRevision.current++;
 previousSource.current=source;const revision=sourceRevision.current;
 const exactConnection=!!candidate&&!!active&&matchesDesktopClaudeSDKSetup(candidate,active);
 const registrationReady=!!(!disabled&&!blocking&&!denied&&!storageFailed&&!readFailed&&exactConnection);
 // Draft validation only restricts a new setup. Original lifecycle recovery
 // stays available even when its request/result is still uncertain.
 const setup=!blocking&&!denied&&!storageFailed&&!readFailed?candidate:undefined;
 useLeaveGuard(blocking,()=>setReadFailed(true));
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;callbacks.current.onBlockedChange?.(false);callbacks.current.onRegistrationReadyChange?.(false);};},[]);
 useEffect(()=>{callbacks.current.onBlockedChange?.(blocking);},[blocking]);
 async function detect(){
  if(disabled||busy||managedBlocked||denied||inflight.current)return;inflight.current=true;setBusy(true);setReadFailed(false);
  const previous=installationKey(fact);
  try{
   const raw=await discover({project_id:project,user_id:scope.userID,tenant_id:scope.organizationID,adapter_id:'claude-code-sdk',locale});if(!alive.current)return;
   const value=readInstalledToolDiscovery(raw,'claude-code-sdk');
   if(installationKey(value)!==previous){sourceRevision.current++;if(previous)setActive(undefined);}
   setFact(value);if(!value){sourceRevision.current++;setActive(undefined);setReadFailed(true);}
  }catch{if(alive.current){sourceRevision.current++;setFact(undefined);setActive(undefined);setReadFailed(true);}}finally{if(alive.current){inflight.current=false;setBusy(false);}}
 }
 useEffect(()=>{if(!disabled&&!managedBlocked&&!denied&&active&&!attempted.current){attempted.current=true;void detect();}},[disabled,managedBlocked,denied,active?.phase]);
 const receive=useCallback((value:DesktopToolConnection)=>{
  // A monotonically changing revision rejects an old A callback after A→B→A;
  // equality of the visible model/reference alone cannot resurrect old proof.
  if(!alive.current||revision!==sourceRevision.current)return;
  const result=readDesktopToolResult({status:'ok',connection:value},scope);if(result?.status!=='ok')return;
  const connection=result.connection;
  // This flag only explains an edited draft after a connected configuration was
  // observed. It never contributes to readiness, permission or command inputs.
  if(connection.phase==='connected'){seenConnected.current=true;if(candidate&&matchesDesktopClaudeSDKSetup(candidate,connection))setNeedsApplication(false);}
  if(connection.phase==='stopped'){seenConnected.current=false;setNeedsApplication(false);reported.current='';}
  setActive(connection);
 },[project,scope.userID,scope.organizationID,revision]);
 useEffect(()=>{
  callbacks.current.onRegistrationReadyChange?.(registrationReady);
  if(!registrationReady||!active){reported.current='';return;}
  const proof:DesktopClaudeSDKConnectedProof={worker_id:active.worker_id!,model:active.model!,credential_env_ref:active.credential_env_ref!,tool_version:active.tool_version!,executable_sha256:active.executable_sha256!,runtime_executable_sha256:active.runtime_executable_sha256!,configuration_sha256:active.configuration_sha256!,claude_sdk_execution:{...active.claude_sdk_execution!}};
  const identity=JSON.stringify([proof,active.credential_version]);if(reported.current!==identity){reported.current=identity;callbacks.current.onConnected(proof);}
 },[registrationReady,active]);
 function edit(next:Draft){
  if(disabled||blocking||denied)return;sourceRevision.current++;if(seenConnected.current)setNeedsApplication(true);setDraft(next);setActive(undefined);
  try{const storage=toolSessionStorage();if(!storage||!safeDraft(next))throw Error();storage.setItem(draftKey(scope),JSON.stringify(next));setStorageFailed(false);}catch{setStorageFailed(true);}
 }
 const ignoreWorker=useCallback(()=>{},[]),reason=readFailed?'probe_failed':fact?.reason_code==='version_unsupported'?undefined:fact?.reason_code==='configuration_required'&&configured?undefined:fact?.reason_code;
 return <section className="claude-sdk-local" aria-label={t('title')} aria-busy={busy}>
  <header><h3>{t('title')}</h3><p>{t('intro')}</p></header>
  <div className="claude-sdk-local-actions"><button type="button" data-claude-sdk-detect disabled={disabled||blocking||denied} onClick={()=>void detect()}>{t(busy?'detecting':'detect')}</button>{onGuide&&<button type="button" disabled={disabled||blocking} onClick={onGuide}>{t('guide')}</button>}</div>
  {storageFailed&&<p role="alert">{t('storage_failed')}</p>}
  {fact&&<p className="claude-sdk-local-installation" role="status">{t(fact.installed?'installed':'not_installed')}{fact.version&&<span>{fact.version}</span>}</p>}
  {fact?.reason_code==='version_unsupported'&&<p role="status">{t('version_unsupported')}</p>}
  <ToolConnectionDiagnostic reasonCode={reason} disabled={disabled||blocking} onDetect={()=>void detect()} onConfigure={()=>modelField.current?.focus()} onGuide={onGuide}/>
  <fieldset disabled={disabled||blocking||denied} className="claude-sdk-local-fields"><legend>{t('configuration')}</legend>
   <label>{t('model')}<input ref={modelField} name="model" value={draft.model} maxLength={256} autoComplete="off" spellCheck={false} aria-describedby={hintID+'-model'} onChange={event=>edit({...draft,model:event.target.value})}/><span id={hintID+'-model'} className="claude-sdk-local-muted">{t('model_hint')}</span></label>
   <label>{t('credential_ref')}<input ref={credentialField} name="credential_env_ref" value={draft.credential_env_ref} maxLength={128} autoComplete="off" spellCheck={false} aria-describedby={hintID+'-credential'} onChange={event=>edit({...draft,credential_env_ref:event.target.value})}/><span id={hintID+'-credential'} className="claude-sdk-local-muted">{t('credential_hint')}</span></label>
  </fieldset>
  {(needsApplication||active?.phase==='connected'&&!registrationReady)&&!blocking&&<p role="status" className="claude-sdk-local-warning">{t('draft_not_adopted')}</p>}
  <DesktopToolConnector project={project} adapterID="claude-code-sdk" secondary={secondary} disabled={disabled||busy} claudeSDKSetup={setup} onConnection={receive} onConnected={ignoreWorker} onBlockedChange={setManagedBlocked} onConfigure={reason=>(reason==='credential_reference_unavailable'?credentialField:modelField).current?.focus()} onDetect={()=>void detect()} onGuide={onGuide}/>
  <p className="claude-sdk-local-muted">{t('boundary')}</p>
 </section>;
}
