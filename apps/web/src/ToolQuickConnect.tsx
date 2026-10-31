import {QoderLocalConnector} from './QoderLocalConnector';
import {CodexLocalConnector} from './CodexLocalConnector';
import type {DesktopCodexConnectedProof} from './desktop-codex-connector-model';
import {ToolConnectionDiagnostic} from './ToolConnectionDiagnostic';
import {desktopACPBridge} from './desktop-acp-connector-model';
import {useEffect,useId,useRef,useState} from 'react';
import {ArrowRight,CheckCircle2,Monitor,RefreshCw,Server,ShieldCheck} from 'lucide-react';
import {useApi,useApiView} from './api-context';
import {readAdapterCatalog,type AdapterDescriptor,type AdapterCatalogPage} from './AdapterConnections';
import {parseToolDiscovery,type ToolDiscoveryView} from './tool-discovery-model';
import {isCursorConnectionGuide,localDiscoveryAdapters,matchingQuickTools,quickConnectAdapters,quickConnectWorkers,readInstalledToolDiscovery,validQuickConnectReceipt,type InstalledToolFact,type ConfirmedNativeToolProof} from './tool-quick-connect-model';
import {tr,useLocale,UiError} from './i18n';
import {ErrorNotice,Field,Loading} from './ui';
import {toolError,type ToolAction} from './tool-hooks';
import type {IdentityRead,SharedIdentityRead} from './execution-pools/ToolIdentity';
import type {ToolProfile} from './tool-model';
import {DesktopToolConnector} from './DesktopToolConnector';
import {desktopToolBridge,validDesktopToolScope} from './desktop-tool-connector-model';
import {CursorConnectionGuide} from './CursorConnectionGuide';
import './ToolQuickConnect.css';

const t=(key:string,params?:Record<string,unknown>)=>tr('tool-quick-connect.'+key,params);
export type ToolQuickConnectProps={project:string;adapter?:AdapterDescriptor;workers:IdentityRead;sharedWorkers:SharedIdentityRead;action:ToolAction;onRegistered:(profile:ToolProfile)=>void;onEnable:(profile:ToolProfile)=>void;onAdvanced:(adapter?:AdapterDescriptor)=>void;onWorker:()=>void};
export function ToolQuickConnect(props:ToolQuickConnectProps){
 const view=useApiView(),scope=JSON.stringify([props.project,view?.userID,view?.organizationID]);
 return <QuickConnectScope {...props} key={scope}/>;
}
function QuickConnectScope(props:ToolQuickConnectProps){
 useLocale();const api=useApi(),id=useId(),[revision,setRevision]=useState(0),[catalog,setCatalog]=useState<{caller:typeof api;revision:number;page?:AdapterCatalogPage;error?:unknown}>(),[selected,setSelected]=useState(props.adapter?.id||''),[localBlocked,setLocalBlocked]=useState(false);
 useEffect(()=>{
  const controller=new AbortController();let active=true;setCatalog(undefined);
  void api<unknown>('/adapter-catalog?project_id='+encodeURIComponent(props.project),undefined,'GET',undefined,controller.signal).then(value=>{
   if(!active)return;const page=readAdapterCatalog(value,props.project);setCatalog({caller:api,revision,...(page?{page}:{error:new UiError('adapter-connections.invalid_response')})});
  }).catch(error=>{if(active&&!controller.signal.aborted)setCatalog({caller:api,revision,error});});
  return()=>{active=false;controller.abort();};
 },[api,props.project,revision]);
 const current=catalog?.caller===api&&catalog.revision===revision?catalog:undefined,ordinary=current?.page?quickConnectAdapters(current.page.items):[],guide=current?.page?.items.find(isCursorConnectionGuide),adapters=guide?[...ordinary,guide]:ordinary,adapter=adapters.find(item=>item.id===selected),cursorGuide=!!adapter&&isCursorConnectionGuide(adapter),blocked=props.action.busy||!!props.action.pending||localBlocked;
 return <div className="tool-quick-connect">
  {!cursorGuide&&<p className="tool-quick-intro">{t('intro')}</p>}
  {!current?<Loading/>:current.error?<ErrorNotice error={current.error} onRetry={()=>setRevision(value=>value+1)}/>:<>
   <Field label={t('tool')}><select data-quick-tool value={adapter?.id||''} disabled={blocked} onChange={event=>setSelected(event.target.value)} aria-describedby={cursorGuide?undefined:id}><option value="">{t('choose_tool')}</option>{adapters.map(item=><option key={item.id} value={item.id}>{isCursorConnectionGuide(item)?tr('cursor-connection.title'):<>{item.product} · {item.entrypoint_kind==='app_server'?t('app_server'):item.entrypoint_kind.toUpperCase()}</>}</option>)}</select></Field>
   {!cursorGuide&&<p id={id} className="tool-quick-muted">{guide?tr('cursor-connection.picker_hint'):t('catalog_hint')}</p>}
   {adapter&&(cursorGuide?<CursorConnectionGuide key={adapter.id} project={props.project} disabled={blocked}/>:<QuickToolConfiguration {...props} key={adapter.id} adapter={adapter} onLocalBlockedChange={setLocalBlocked}/>)}
  </>}
  {adapter?.id==='qoder-acp'&&!desktopACPBridge()&&<p className="tool-quick-muted">{tr('acp-runtime.setup_no_scan')}</p>}
  {!cursorGuide&&<div className="tool-quick-secondary"><button data-quick-advanced type="button" className="text-button" disabled={blocked} onClick={()=>props.onAdvanced(adapter)}>{adapter?.id==='qoder-acp'?tr('acp-runtime.setup_configure'):t('advanced')}</button></div>}
 </div>;
}
type InventorySnapshot={caller:ReturnType<typeof useApi>;key:string;view?:ToolDiscoveryView;deadline:number;error?:unknown};
function QuickToolConfiguration({project,adapter,workers,sharedWorkers,action,onRegistered,onEnable,onWorker,onAdvanced,onLocalBlockedChange}:ToolQuickConnectProps&{adapter:AdapterDescriptor;onLocalBlockedChange:(blocked:boolean)=>void}){
 useLocale();const api=useApi(),[chosenWorker,setChosenWorker]=useState(''),[revision,setRevision]=useState(0),[snapshot,setSnapshot]=useState<InventorySnapshot>(),[expired,setExpired]=useState(false),[chosenCommand,setChosenCommand]=useState(''),[name,setName]=useState(''),[nameEdited,setNameEdited]=useState(false),[registered,setRegistered]=useState<ToolProfile>();
 const [localOperationBlocked,setLocalOperationBlocked]=useState(false),[nativeRegistrationReady,setNativeRegistrationReady]=useState(true),[nativeConnection,setNativeConnection]=useState<{caller:typeof api;connection:DesktopCodexConnectedProof}>(),currentCaller=useRef(api);currentCaller.current=api;
 useEffect(()=>{onLocalBlockedChange(localOperationBlocked);},[localOperationBlocked,onLocalBlockedChange]);
 useEffect(()=>()=>onLocalBlockedChange(false),[onLocalBlockedChange]);
 const confirmedNative=nativeConnection?.caller===api?nativeConnection.connection:undefined;
 const actor=useApiView(),managesLocal=!!desktopToolBridge()&&(adapter.id!=='qoder-acp'||!!desktopACPBridge())&&!!actor&&validDesktopToolScope({project,userID:actor.userID,organizationID:actor.organizationID,adapterID:adapter.id});
 const identitiesKnown=!!workers.data&&!workers.loading&&!workers.error&&!!sharedWorkers.data&&!sharedWorkers.loading&&!sharedWorkers.error;
 const nodes=identitiesKnown?quickConnectWorkers(workers.data!.items,sharedWorkers.data!.items):[],worker=confirmedNative?(nodes.includes(confirmedNative.worker_id)?confirmedNative.worker_id:''):nodes.includes(chosenWorker)?chosenWorker:nodes.length===1?nodes[0]:'',active=identitiesKnown&&!!worker;
 const key=JSON.stringify([worker,active,revision]),reportedReceipt=useRef('');
 const alive=useRef(true);useEffect(()=>{alive.current=true;return()=>{alive.current=false;};},[]);
 useEffect(()=>{
  const controller=new AbortController();let current=true;setSnapshot(undefined);setExpired(false);setChosenCommand('');
  if(active){const started=performance.now();void api<unknown>('/tool-discovery?project_id='+encodeURIComponent(project)+'&worker_id='+encodeURIComponent(worker),undefined,'GET',undefined,controller.signal).then(value=>{
   if(!current)return;const view=parseToolDiscovery(value,project,worker),remaining=Math.max(0,view.remainingMs-(performance.now()-started));setSnapshot({caller:api,key,view,deadline:performance.now()+remaining});setExpired(view.state==='fresh'&&remaining<=0);
  }).catch(error=>{if(current&&!controller.signal.aborted)setSnapshot({caller:api,key,deadline:0,error});});}
  return()=>{current=false;controller.abort();};
 },[api,project,worker,active,key]);
 const current=active&&snapshot?.caller===api&&snapshot.key===key?snapshot:undefined;
 useEffect(()=>{if(!current?.view||current.view.state!=='fresh')return;const timer=setTimeout(()=>setExpired(true),Math.max(0,current.deadline-performance.now()));return()=>clearTimeout(timer);},[current]);
 const expectedNative:ConfirmedNativeToolProof|undefined=confirmedNative&&confirmedNative.worker_id===worker?{configuration_sha256:confirmedNative.configuration_sha256,executable_sha256:confirmedNative.executable_sha256,tool_version:confirmedNative.tool_version,catalog_sha256:confirmedNative.catalog_sha256}:undefined;
 const view=current?.view,rows=view?.state==='fresh'&&!expired&&(!confirmedNative||nativeRegistrationReady)?matchingQuickTools(view.items,adapter,expectedNative):[],available=rows.filter(item=>item.available),selected=available.find(item=>item.profile.command_id===chosenCommand)||(available.length===1?available[0]:undefined),suggestedName=nameEdited?name:selected?.profile.command_id||'',writeBlocked=action.busy||!!action.pending,blocked=writeBlocked||localOperationBlocked,usable=!!selected&&!!current&&!expired&&performance.now()<current.deadline;
 const receipt=registered&&active&&registered.worker_id===worker?registered:undefined;
 useEffect(()=>{
  if(!receipt||reportedReceipt.current===receipt.id)return;
  reportedReceipt.current=receipt.id;onRegistered(receipt);
 },[receipt,onRegistered]);
 async function register(){
  if(blocked||action.conflict||!usable||!suggestedName.trim()||!selected||!current||performance.now()>=current.deadline){if(current&&performance.now()>=current.deadline)setExpired(true);return;}
  const body={project_id:project,worker_id:worker,name:suggestedName.trim(),profile:selected.profile};
  const result=await action.run<ToolProfile>('/tool-profiles',body);
  // A transient node read must not erase an already matched original receipt.
  // The scoped component owns it; current authorization still guards display.
  if(result&&alive.current&&validQuickConnectReceipt(result,body))setRegistered(result);
 }
 return <section className="tool-quick-configuration">
  {adapter.id==='qoder-acp'&&<details className="tool-quick-muted"><summary>{tr('acp-quick-connect.native_policy')}</summary><p>{tr('acp-runtime.setup_policy')}</p></details>}
   {adapter.id==='qoder-acp'?<QoderLocalConnector project={project} disabled={writeBlocked} secondary={!!receipt||available.length>0} onConnected={workerID=>{setChosenWorker(workerID);setRevision(value=>value+1);void workers.read();void sharedWorkers.read();}}/>:adapter.id==='codex-app-server'?<CodexLocalConnector project={project} disabled={writeBlocked} secondary={!!receipt||available.length>0} onBlockedChange={setLocalOperationBlocked} onRegistrationReadyChange={setNativeRegistrationReady} onConnected={proof=>{if(!alive.current||currentCaller.current!==api)return;setNativeConnection({caller:api,connection:{...proof}});setChosenWorker(proof.worker_id);setRevision(value=>value+1);void workers.read();void sharedWorkers.read();}}/>:<DesktopToolConnector project={project} adapterID={adapter.id} disabled={writeBlocked} secondary={!!receipt||available.length>0} onBlockedChange={setLocalOperationBlocked} onGuide={()=>onAdvanced(adapter)} onConnected={workerID=>{setChosenWorker(workerID);setRevision(value=>value+1);void workers.read();void sharedWorkers.read();}}/>}
  {!identitiesKnown?<div className="tool-quick-node-state" role="status"><p>{t('reading_nodes')}</p>{workers.error&&<ErrorNotice error={workers.error} onRetry={()=>void workers.read()}/>} {sharedWorkers.error&&<ErrorNotice error={sharedWorkers.error} onRetry={()=>void sharedWorkers.read()}/>}</div>:!nodes.length?<div className="tool-quick-empty"><Server size={22} aria-hidden="true"/><div><strong>{t('no_node')}</strong><p>{t('no_node_hint')}</p><button data-quick-connect-worker type="button" className={managesLocal?'text-button':'primary'} disabled={blocked} onClick={onWorker}>{t('connect_node')}<ArrowRight size={15} aria-hidden="true"/></button></div>{!managesLocal&&<LocalInstallation project={project} adapter={adapter} onGuide={()=>onAdvanced(adapter)}/>}</div>:<>
   {nodes.length>1?<Field label={t('node')}><select data-quick-worker value={worker} disabled={blocked||!!receipt} onChange={event=>{setNativeConnection(undefined);setChosenWorker(event.target.value);}}><option value="">{t('choose_node')}</option>{nodes.map(node=><option key={node} value={node}>{node}</option>)}</select></Field>:<p className="tool-quick-node"><Server size={15} aria-hidden="true"/>{t('using_node',{node:worker})}</p>}
   {receipt?<div className="tool-quick-saved" role="status"><CheckCircle2 size={22} aria-hidden="true"/><div><strong>{t('registered',{name:receipt.name})}</strong><p>{t('paused_hint')}</p><button data-quick-enable type="button" className="primary" disabled={blocked} onClick={()=>onEnable(receipt)}>{t('enable')}<ArrowRight size={15} aria-hidden="true"/></button></div></div>:worker&&<>
    {!current?<Loading/>:current.error?<ErrorNotice error={toolError(current.error)} onRetry={()=>setRevision(value=>value+1)}/>:view&&<>
     <div className="tool-quick-inventory-heading"><strong>{t('detected')}</strong><button type="button" className="text-button" disabled={blocked} onClick={()=>setRevision(value=>value+1)}><RefreshCw size={14} aria-hidden="true"/>{t('refresh')}</button></div>
     {(expired||view.state!=='fresh')?<p className="tool-quick-warning" role="status">{t(view.state==='unreported'?'unreported':'expired')}</p>:!rows.length?<p className="tool-quick-warning" role="status">{t('no_matching')}</p>:<>
      {available.length>1&&<Field label={t('configuration')}><select data-quick-configuration value={selected?.profile.command_id||''} disabled={blocked} onChange={event=>setChosenCommand(event.target.value)}><option value="">{t('choose_configuration')}</option>{available.map(item=><option key={item.profile.command_id} value={item.profile.command_id}>{item.profile.command_id} · {item.profile.tool_version}</option>)}</select></Field>}
      {rows.filter(item=>!item.available).map(item=><p className="tool-quick-warning" key={item.profile.command_id}>{t('unavailable',{command:item.profile.command_id})}</p>)}
      {selected&&<><div className="tool-quick-facts"><span><ShieldCheck size={16} aria-hidden="true"/>{t('probe_confirmed')}</span><span>{adapter.product} · {selected.profile.tool_version}</span><span>{t(selected.profile.read_only?'read_only':'worktree_write')}</span></div><Field label={t('name')}><input data-quick-name required maxLength={200} value={suggestedName} disabled={blocked} onChange={event=>{setNameEdited(true);setName(event.target.value);}}/></Field><details className="tool-quick-technical"><summary>{t('technical')}</summary><p>{adapter.adapter} / {adapter.protocol}</p><code>{selected.profile.profile_sha256}</code></details></>}
     </>}
     <p className="tool-quick-muted">{t('register_hint')}</p>
     <button data-quick-register type="button" className="primary tool-quick-submit" disabled={blocked||action.conflict||!usable||!suggestedName.trim()} onClick={()=>void register()}>{t(action.busy?'registering':'register')}</button>
    </>}
   </>}
  </>}
 </section>;
}
function LocalInstallation({project,adapter,onGuide}:{project:string;adapter:AdapterDescriptor;onGuide:()=>void}){
 const locale=useLocale(),view=useApiView(),[busy,setBusy]=useState(false),[fact,setFact]=useState<InstalledToolFact>(),[failed,setFailed]=useState(false),epoch=useRef(0);
 const raw=(window as unknown as {sumaDesktop?:{platform?:unknown;version?:unknown;discoverInstalledTools?:(request:Record<string,string>)=>Promise<unknown>}}).sumaDesktop;
 const bridge=raw&&['win32','darwin','linux'].includes(String(raw.platform))&&raw.version==='1.0.0'&&typeof raw.discoverInstalledTools==='function'?raw:undefined;
 useEffect(()=>()=>{epoch.current++;},[]);
 // The request is scoped to the same original API view as the node picker.
 // This optional probe cannot supply a Runner profile or enable registration.
 if(!bridge||!view||!localDiscoveryAdapters.has(adapter.id))return null;
 async function scan(){
  if(busy||!bridge?.discoverInstalledTools||!view)return;const generation=++epoch.current;setBusy(true);setFact(undefined);setFailed(false);
  try{const result=await bridge.discoverInstalledTools({project_id:project,user_id:view.userID,tenant_id:view.organizationID,adapter_id:adapter.id,locale});if(generation!==epoch.current)return;const observed=readInstalledToolDiscovery(result,adapter.id);if(!observed)setFailed(true);else setFact(observed);}catch{if(generation===epoch.current)setFailed(true);}finally{if(generation===epoch.current)setBusy(false);}
 }
 return <div className="tool-quick-local"><p>{t('local_hint')}</p><button type="button" disabled={busy} onClick={()=>void scan()}><Monitor size={15} aria-hidden="true"/>{t(busy?'local_scanning':'local_scan')}</button>{failed&&<p role="alert">{t('local_failed')}</p>}{fact&&<div role="status"><strong>{t(fact.installed?'local_installed':'local_missing')}</strong>{fact.version&&<span> · {fact.version}</span>}<p>{t(fact.installed&&!fact.launch_supported?'local_launch_pending':'local_boundary')}</p><ToolConnectionDiagnostic reasonCode={fact.reason_code} disabled={busy} onDetect={()=>void scan()} onGuide={onGuide}/>{fact.executable&&<details><summary>{t('technical')}</summary><code>{fact.executable}</code></details>}</div>}</div>;
}
