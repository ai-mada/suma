import {QoderLocalConnector} from './QoderLocalConnector';
import {CodexLocalConnector} from './CodexLocalConnector';
import {OpenCodeLocalConnector} from './OpenCodeLocalConnector';
import {ClaudeSDKLocalConnector} from './ClaudeSDKLocalConnector';
import {toolPreparationTarget} from './tool-preparation-target';
import type {DesktopCodexConnectedProof} from './desktop-codex-connector-model';
import type {DesktopOpenCodeConnectedProof} from './desktop-opencode-connector-model';
import type {DesktopClaudeSDKConnectedProof} from './desktop-claude-sdk-connector-model';
import {ToolConnectionDiagnostic} from './ToolConnectionDiagnostic';
import {desktopACPBridge,desktopACPRuntimeForAdapter} from './desktop-acp-connector-model';
import {useCallback,useEffect,useId,useLayoutEffect,useRef,useState} from 'react';
import {ArrowRight,CheckCircle2,Monitor,RefreshCw,Server,ShieldCheck} from 'lucide-react';
import {useApi,useApiView} from './api-context';
import {readAdapterCatalog,type AdapterDescriptor,type AdapterCatalogPage} from './AdapterConnections';
import {parseToolDiscovery,type ToolDiscoveryView} from './tool-discovery-model';
import {isCursorConnectionGuide,localDiscoveryAdapters,matchingQuickTools,quickConnectAdapters,quickConnectWorkers,readInstalledToolDiscovery,validQuickConnectReceipt,type InstalledToolFact,type ConfirmedNativeToolProof,type ConfirmedOpenCodeToolProof,type ConfirmedClaudeSDKToolProof} from './tool-quick-connect-model';
import {tr,useLocale,UiError} from './i18n';
import {ErrorNotice,Field,Loading} from './ui';
import {toolError,type ToolAction} from './tool-hooks';
import type {IdentityRead,SharedIdentityRead} from './execution-pools/ToolIdentity';
import type {ToolProfile} from './tool-model';
import type {ToolProfileMatchList,ToolProfileMatchRequest,RunnerPairing} from '../../../packages/contracts/src';
import {readRunnerPairing} from './runner-pairing-model';
import {readToolProfileMatches,sameToolProfile,toolProfileMatchKey} from './tool-profile-reuse-model';
import {ToolProfileReplacementConfirm} from './ToolProfileReplacementView';
import type {ToolProfileReplacementDraft} from './tool-profile-replacement-model';
import {ExistingToolProfiles} from './ExistingToolProfiles';
import {DesktopToolConnector} from './DesktopToolConnector';
import {desktopToolBridge,validDesktopToolScope} from './desktop-tool-connector-model';
import {CursorConnectionGuide} from './CursorConnectionGuide';
import {LocalToolsScanPanel} from './LocalToolsScanPanel';
import {localToolsScanBridge,validLocalToolsScanScope} from './local-tools-scan-model';
import './ToolQuickConnect.css';

const t=(key:string,params?:Record<string,unknown>)=>tr('tool-quick-connect.'+key,params);
export type ToolConnectDraft={adapterID:string;workerID:string;commandID:string;name:string;nameEdited:boolean};
export type ToolQuickConnectProps={project:string;adapter?:AdapterDescriptor;draft?:ToolConnectDraft;onDraftChange?:(draft:ToolConnectDraft)=>void;replacementSource?:ToolProfile;replacementDisabled?:boolean;onReplace?:(draft:ToolProfileReplacementDraft)=>Promise<void>|void;preferredWorkerID?:string;pairedRunner?:RunnerPairing;onPrepareRunner?:(pairing:RunnerPairing,adapter:AdapterDescriptor)=>void;workers:IdentityRead;sharedWorkers:SharedIdentityRead;action:ToolAction;onRegistered:(profile:ToolProfile)=>void;onEnable:(profile:ToolProfile)=>void;onOpen?:(profile:ToolProfile)=>void;onAdvanced:(adapter?:AdapterDescriptor,expectedWorkerID?:string)=>void;onViewExisting?:(adapter:AdapterDescriptor)=>void;onWorker:()=>void};
export function ToolQuickConnect(props:ToolQuickConnectProps){
 const view=useApiView(),scope=JSON.stringify([props.project,view?.userID,view?.organizationID,props.preferredWorkerID,props.replacementSource?.id,props.replacementSource?.version]);
 return <QuickConnectScope {...props} key={scope}/>;
}
function QuickConnectScope(props:ToolQuickConnectProps){
 useLocale();const api=useApi(),id=useId(),[revision,setRevision]=useState(0),[catalog,setCatalog]=useState<{caller:typeof api;revision:number;page?:AdapterCatalogPage;error?:unknown}>(),[selected,setSelected]=useState(props.adapter?.id||props.draft?.adapterID||''),[localBlocked,setLocalBlocked]=useState(false);
 const drafts=useRef(new Map<string,QuickToolDraft>(props.draft?[[props.draft.adapterID,{chosenWorker:props.draft.workerID,chosenCommand:props.draft.commandID,name:props.draft.name,nameEdited:props.draft.nameEdited}]]:[])),toolChoice=useRef<HTMLSelectElement>(null),draftOwner=useRef({api,callback:props.onDraftChange}),lastPublished=useRef('');draftOwner.current={api,callback:props.onDraftChange};
 const rememberDraft=useCallback((adapterID:string,draft:QuickToolDraft)=>{
  if(draftOwner.current.api!==api)return;drafts.current.set(adapterID,draft);
  const value:ToolConnectDraft={adapterID,workerID:draft.chosenWorker,commandID:draft.chosenCommand,name:draft.name,nameEdited:draft.nameEdited},key=JSON.stringify(value);
  if(lastPublished.current===key)return;lastPublished.current=key;draftOwner.current.callback?.(value);
 },[api]);
 const rememberConfiguration=useCallback((value:QuickToolDraft)=>rememberDraft(selected,value),[rememberDraft,selected]);
 useEffect(()=>{
  const controller=new AbortController();let active=true;setCatalog(undefined);
  void api<unknown>('/adapter-catalog?project_id='+encodeURIComponent(props.project),undefined,'GET',undefined,controller.signal).then(value=>{
   if(!active)return;const page=readAdapterCatalog(value,props.project);setCatalog({caller:api,revision,...(page?{page}:{error:new UiError('adapter-connections.invalid_response')})});
  }).catch(error=>{if(active&&!controller.signal.aborted)setCatalog({caller:api,revision,error});});
  return()=>{active=false;controller.abort();};
 },[api,props.project,revision]);
 const current=catalog?.caller===api&&catalog.revision===revision?catalog:undefined,ordinary=current?.page?quickConnectAdapters(current.page.items):[],guide=current?.page?.items.find(isCursorConnectionGuide),adapters=guide?[...ordinary,guide]:ordinary,adapter=adapters.find(item=>item.id===selected),cursorGuide=!!adapter&&isCursorConnectionGuide(adapter),blocked=props.action.busy||!!props.action.pending||localBlocked;
 function choose(adapterID:string){if(blocked||adapterID&&!adapters.some(item=>item.id===adapterID))return false;setSelected(adapterID);rememberDraft(adapterID,drafts.current.get(adapterID)??{chosenWorker:'',chosenCommand:'',name:'',nameEdited:false});return true;}
 function openAdvanced(){
  if(blocked||draftOwner.current.api!==api)return;
  const remembered=adapter?drafts.current.get(adapter.id):undefined;
  props.onAdvanced(adapter,adapter?(remembered?remembered.chosenWorker:props.preferredWorkerID)||undefined:undefined);
 }
 return <div className="tool-quick-connect">
  {!cursorGuide&&<p className="tool-quick-intro">{t('intro')}</p>}
  {!current?<Loading/>:current.error?<ErrorNotice error={current.error} onRetry={()=>setRevision(value=>value+1)}/>:<>
   <LocalToolsScanPanel project={props.project} adapters={adapters} selectedAdapterID={adapter?.id} disabled={blocked} onSelect={item=>{if(choose(item.id)){toolChoice.current?.focus({preventScroll:true});toolChoice.current?.scrollIntoView?.({block:'start',behavior:'auto'});}}} onWorker={props.onWorker} onViewExisting={props.onViewExisting}/>
   <Field label={t('tool')}><select ref={toolChoice} data-quick-tool value={adapter?.id||''} disabled={blocked} onChange={event=>choose(event.target.value)} aria-describedby={cursorGuide?undefined:id}><option value="">{t('choose_tool')}</option>{adapters.map(item=><option key={item.id} value={item.id}>{isCursorConnectionGuide(item)?tr('cursor-connection.title'):<>{item.product} · {item.entrypoint_kind==='app_server'?t('app_server'):item.entrypoint_kind.toUpperCase()}</>}</option>)}</select></Field>
   {!cursorGuide&&<p id={id} className="tool-quick-muted">{guide?tr('cursor-connection.picker_hint'):t('catalog_hint')}</p>}
   {adapter&&(cursorGuide?<CursorConnectionGuide key={adapter.id} project={props.project} disabled={blocked}/>:<QuickToolConfiguration {...props} key={adapter.id} adapter={adapter} initialDraft={drafts.current.get(adapter.id)} onConfigurationDraftChange={rememberConfiguration} onLocalBlockedChange={setLocalBlocked}/>)}
  </>}
  {adapter?.id==='qoder-acp'&&!desktopACPBridge()&&<p className="tool-quick-muted">{tr('acp-runtime.setup_no_scan')}</p>}
  <div className="tool-quick-secondary">{adapter&&props.onViewExisting&&<button data-quick-view-existing type="button" className="text-button" disabled={blocked} onClick={()=>{if(!blocked)props.onViewExisting?.(adapter);}}>{t('view_existing')}</button>}{!cursorGuide&&<button data-quick-advanced type="button" className="text-button" disabled={blocked} onClick={openAdvanced}>{adapter?.id==='claude-code-sdk'?tr('claude-sdk-setup.title'):desktopACPRuntimeForAdapter(adapter?.id)?tr('acp-runtime.setup_configure',{tool:adapter?.id==='cursor-cli'?'Cursor Agent':'Qoder'}):t('advanced')}</button>}</div>
 </div>;
}
type InventorySnapshot={caller:ReturnType<typeof useApi>;key:string;view?:ToolDiscoveryView;deadline:number;error?:unknown};
type MatchSnapshot={caller:ReturnType<typeof useApi>;key:string;data?:ToolProfileMatchList;error?:unknown};
type QuickToolDraft=Readonly<{chosenWorker:string;chosenCommand:string;name:string;nameEdited:boolean}>;
function QuickToolConfiguration({project,adapter,replacementSource,replacementDisabled,onReplace,preferredWorkerID,pairedRunner,onPrepareRunner,workers,sharedWorkers,action,onRegistered,onEnable,onOpen,onWorker,onAdvanced,onLocalBlockedChange,initialDraft,onConfigurationDraftChange}:ToolQuickConnectProps&{adapter:AdapterDescriptor;onLocalBlockedChange:(blocked:boolean)=>void;initialDraft?:QuickToolDraft;onConfigurationDraftChange:(draft:QuickToolDraft)=>void}){
 const acpAdapterID=adapter.id==='qoder-acp'||adapter.id==='cursor-cli'?adapter.id:undefined;
 useLocale();const api=useApi(),[chosenWorker,setChosenWorker]=useState(preferredWorkerID||initialDraft?.chosenWorker||''),[revision,setRevision]=useState(0),[snapshot,setSnapshot]=useState<InventorySnapshot>(),[expired,setExpired]=useState(false),[chosenCommand,setChosenCommand]=useState(preferredWorkerID&&preferredWorkerID!==initialDraft?.chosenWorker?'':initialDraft?.chosenCommand||''),[name,setName]=useState(initialDraft?.name||''),[nameEdited,setNameEdited]=useState(initialDraft?.nameEdited||false),[registered,setRegistered]=useState<ToolProfile>();
 const [localOperationBlocked,setLocalOperationBlocked]=useState(false),[nativeRegistrationReady,setNativeRegistrationReady]=useState(true),[nativeConnection,setNativeConnection]=useState<{caller:typeof api;connection:DesktopCodexConnectedProof|DesktopOpenCodeConnectedProof|DesktopClaudeSDKConnectedProof}>(),currentCaller=useRef(api);currentCaller.current=api;
 const [lockedWorker,setLockedWorker]=useState(preferredWorkerID||initialDraft?.chosenWorker||''),[nativeMismatch,setNativeMismatch]=useState<{caller:typeof api;target:string;worker:string}>(),workerChoice=useRef<HTMLSelectElement>(null),target=useRef({api,worker:lockedWorker}),lastNativeWorker=useRef(''),reportedNativeReady=useRef(true);target.current={api,worker:lockedWorker};
 const [acpConnection,setACPConnection]=useState<{caller:typeof api;workerID:string}>();
 const [matchRevision,setMatchRevision]=useState(0),[matchSnapshot,setMatchSnapshot]=useState<MatchSnapshot>(),[existingChoice,setExistingChoice]=useState<{key:string;id:string}>(),[createAttempt,setCreateAttempt]=useState<{key:string;ticket:number}>();
 const createSequence=useRef(0),recheckedConflict=useRef(0);
 const [replacementPlan,setReplacementPlan]=useState<{caller:typeof api;key:string;draft:ToolProfileReplacementDraft}>(),replacementBusy=useRef(false);
 useEffect(()=>()=>onLocalBlockedChange(false),[onLocalBlockedChange]);
 const confirmedNative=nativeConnection?.caller===api?nativeConnection.connection:undefined;
 const acpWorker=acpConnection?.caller===api?acpConnection.workerID:undefined,reportedWorker=confirmedNative?.worker_id??acpWorker,localWorker=!lockedWorker||reportedWorker===lockedWorker?reportedWorker:undefined;
 const actor=useApiView(),managesLocal=!!desktopToolBridge()&&(!acpAdapterID||!!desktopACPBridge())&&!!actor&&validDesktopToolScope({project,userID:actor.userID,organizationID:actor.organizationID,adapterID:adapter.id});
 const scansLocal=!!localToolsScanBridge()&&!!actor&&validLocalToolsScanScope({project,userID:actor.userID,organizationID:actor.organizationID});
 const identitiesKnown=!!workers.data&&!workers.loading&&!workers.error&&!!sharedWorkers.data&&!sharedWorkers.loading&&!sharedWorkers.error;
 const chosenTarget=lockedWorker||chosenWorker,nodes=identitiesKnown?quickConnectWorkers(workers.data!.items,sharedWorkers.data!.items):[],worker=localWorker?(nodes.includes(localWorker)?localWorker:''):nodes.includes(chosenTarget)?chosenTarget:!preferredWorkerID&&!chosenTarget&&nodes.length===1?nodes[0]:'',active=identitiesKnown&&!!worker;
 const mismatch=identitiesKnown&&nativeMismatch?.caller===api&&nativeMismatch.target===lockedWorker&&nativeMismatch.worker!==lockedWorker?nativeMismatch:undefined;
 const preferredMissing=identitiesKnown&&!!preferredWorkerID&&!worker&&!nodes.includes(preferredWorkerID);
 const targetWorker=localWorker||chosenWorker,targetMissing=identitiesKnown&&!!targetWorker&&!worker;
 let prepareTarget:RunnerPairing|undefined;
 if(onPrepareRunner&&toolPreparationTarget(adapter)&&pairedRunner?.project_id===project&&pairedRunner.worker_id===worker)try{const value=readRunnerPairing(pairedRunner,project);if(value.state==='approved')prepareTarget=value;}catch{/* Only an exact approved public source can seed the remote guide. */}
 const key=JSON.stringify([worker,active,revision]),reportedReceipt=useRef('');
 const alive=useRef(true);useEffect(()=>{alive.current=true;return()=>{alive.current=false;};},[]);
 function lockTarget(workerID:string){target.current={api,worker:workerID};setLockedWorker(workerID);}
 function reportNativeReady(ready:boolean){
  if(!alive.current||currentCaller.current!==api||target.current.api!==api)return;
  reportedNativeReady.current=ready;setNativeRegistrationReady(ready&&(!target.current.worker||lastNativeWorker.current===target.current.worker));
 }
 function acceptNative(workerID:string,commit?:()=>void){
  if(!alive.current||currentCaller.current!==api||target.current.api!==api)return;
  lastNativeWorker.current=workerID;const expected=target.current.worker;
  if(expected&&workerID!==expected){
   setNativeRegistrationReady(false);setNativeMismatch(previous=>previous?.caller===api&&previous.target===expected&&previous.worker===workerID?previous:{caller:api,target:expected,worker:workerID});return;
  }
  setNativeMismatch(undefined);setNativeRegistrationReady(reportedNativeReady.current);commit?.();setChosenCommand('');setChosenWorker(workerID);setRevision(value=>value+1);void workers.read();void sharedWorkers.read();
 }
 useEffect(()=>{
  const controller=new AbortController();let current=true;setSnapshot(undefined);setExpired(false);
  if(active){const started=performance.now();void api<unknown>('/tool-discovery?project_id='+encodeURIComponent(project)+'&worker_id='+encodeURIComponent(worker),undefined,'GET',undefined,controller.signal).then(value=>{
   if(!current)return;const view=parseToolDiscovery(value,project,worker),remaining=Math.max(0,view.remainingMs-(performance.now()-started));setSnapshot({caller:api,key,view,deadline:performance.now()+remaining});setExpired(view.state==='fresh'&&remaining<=0);
  }).catch(error=>{if(current&&!controller.signal.aborted)setSnapshot({caller:api,key,deadline:0,error});});}
  return()=>{current=false;controller.abort();};
 },[api,project,worker,active,key]);
 const current=active&&snapshot?.caller===api&&snapshot.key===key?snapshot:undefined;
 useEffect(()=>{if(!current?.view||current.view.state!=='fresh')return;const timer=setTimeout(()=>setExpired(true),Math.max(0,current.deadline-performance.now()));return()=>clearTimeout(timer);},[current]);
 const expectedNative:ConfirmedNativeToolProof|ConfirmedOpenCodeToolProof|ConfirmedClaudeSDKToolProof|undefined=confirmedNative&&confirmedNative.worker_id===worker?(()=>{const {worker_id:_,...proof}=confirmedNative;return proof;})():undefined;
 const view=current?.view,rows=view?.state==='fresh'&&!expired&&(!localWorker||nativeRegistrationReady)?matchingQuickTools(view.items,adapter,expectedNative):[],available=rows.filter(item=>item.available),selected=chosenCommand?available.find(item=>item.profile.command_id===chosenCommand):available.length===1?available[0]:undefined,suggestedName=nameEdited?name:selected?.profile.command_id||'',writeBlocked=action.busy||!!action.pending,usable=!!selected&&!!current&&!expired&&performance.now()<current.deadline;
 useLayoutEffect(()=>{
  // Capture automatic single-item choices once; a later read cannot silently
  // substitute a different target. Only public fields leave this component.
  if(!chosenWorker&&worker)setChosenWorker(worker);if(!chosenCommand&&selected)setChosenCommand(selected.profile.command_id);
  onConfigurationDraftChange({chosenWorker:worker||chosenWorker,chosenCommand:selected?.profile.command_id||chosenCommand,name,nameEdited});
 },[chosenWorker,worker,chosenCommand,selected?.profile.command_id,name,nameEdited,onConfigurationDraftChange]);
 const receipt=registered&&active&&registered.worker_id===worker?registered:undefined;
 const matchRequest:ToolProfileMatchRequest|undefined=usable&&selected?{project_id:project,worker_id:worker,profile:selected.profile}:undefined;
 const matchKey=matchRequest?JSON.stringify([adapter.id,key,current!.deadline,toolProfileMatchKey(matchRequest),matchRevision]):'',matchEligible=!!matchRequest&&!receipt&&!writeBlocked&&!localOperationBlocked;
 const matches=matchEligible&&matchSnapshot?.caller===api&&matchSnapshot.key===matchKey?matchSnapshot:undefined,lookupBusy=matchEligible&&!matches,blocked=writeBlocked||localOperationBlocked||lookupBusy;
 const advancedTarget=useRef({caller:api,worker:worker||chosenTarget,blocked});advancedTarget.current={caller:api,worker:worker||chosenTarget,blocked};
 function openAdvanced(){
  const source=advancedTarget.current;
  if(!alive.current||currentCaller.current!==api||source.caller!==api||source.blocked)return;
  onAdvanced(adapter,source.worker||undefined);
 }
 const reusable=matches?.data?.items.filter(item=>item.state!=='revoked')??[],selectedExisting=existingChoice?.key===matchKey&&reusable.some(item=>item.id===existingChoice.id)?existingChoice.id:!replacementSource&&reusable.length===1?reusable[0].id:'',canCreate=!!matches?.data&&!reusable.length;
 const replacementCandidates=reusable.filter(item=>item.id!==replacementSource?.id),reuseTarget=replacementCandidates.find(item=>item.id===selectedExisting),unchangedSource=!!replacementSource&&!!selected&&replacementSource.worker_id===worker&&sameToolProfile(replacementSource.profile,selected.profile);
 const replacementKey=JSON.stringify([matchKey,replacementSource?.id,replacementSource?.version,replacementSource?.worker_id,replacementSource?.profile.profile_sha256,suggestedName,reuseTarget?.id,reuseTarget?.version]),replacementEligible=!!replacementSource&&!!onReplace&&!replacementDisabled&&!blocked&&!action.conflict&&usable&&!unchangedSource&&(canCreate&&!!suggestedName.trim()||!!reuseTarget),prepared=replacementPlan?.caller===api&&replacementPlan.key===replacementKey?replacementPlan:undefined;
 const replacementGate=useRef({key:replacementKey,eligible:replacementEligible,prepared});replacementGate.current={key:replacementKey,eligible:replacementEligible,prepared};
 function prepareReplacement(){
  if(!replacementGate.current.eligible||!replacementSource||!selected||!alive.current||currentCaller.current!==api||replacementBusy.current)return;
  setReplacementPlan({caller:api,key:replacementKey,draft:{original:replacementSource,workerID:worker,profile:selected.profile,pauseOriginal:false,...(reuseTarget?{reuse:reuseTarget}:{name:suggestedName.trim()})}});
 }
 async function confirmReplacement(){
  const current=replacementGate.current;if(!alive.current||currentCaller.current!==api||!current.eligible||!current.prepared||replacementBusy.current||!onReplace)return;
  replacementBusy.current=true;try{await onReplace(current.prepared.draft);}finally{replacementBusy.current=false;}
 }
 useEffect(()=>{onLocalBlockedChange(localOperationBlocked||lookupBusy);},[localOperationBlocked,lookupBusy,onLocalBlockedChange]);
 useEffect(()=>{
  const controller=new AbortController();let latest=true;setMatchSnapshot(undefined);
  if(matchEligible&&matchRequest){const request=matchRequest,deadline=current!.deadline;void api<unknown>('/tool-profiles/matches',request,'POST',undefined,controller.signal).then(value=>{
   if(!latest||performance.now()>=deadline)return;
   setMatchSnapshot({caller:api,key:matchKey,data:readToolProfileMatches(value,request)});
  }).catch(error=>{if(latest&&!controller.signal.aborted&&performance.now()<deadline)setMatchSnapshot({caller:api,key:matchKey,error});});}
  return()=>{latest=false;controller.abort();};
 },[api,matchEligible,matchKey]);
 // A confirmed create conflict has no unknown write to replay. Read the exact
 // current set once; never substitute another operation's profile as our receipt.
 useEffect(()=>{
  if(!action.conflict||action.pending||!createAttempt||createAttempt.key!==matchKey||recheckedConflict.current===createAttempt.ticket)return;
  recheckedConflict.current=createAttempt.ticket;setMatchRevision(value=>value+1);
 },[action.conflict,action.pending,createAttempt,matchKey]);
 function readMatches(){if(blocked)return;action.clearError();setMatchRevision(value=>value+1);}
 useEffect(()=>{
  if(!receipt||reportedReceipt.current===receipt.id)return;
  reportedReceipt.current=receipt.id;onRegistered(receipt);
 },[receipt,onRegistered]);
 async function register(){
  if(blocked||action.conflict||!canCreate||!usable||!suggestedName.trim()||!selected||!current||performance.now()>=current.deadline){if(current&&performance.now()>=current.deadline)setExpired(true);return;}
  const body={project_id:project,worker_id:worker,name:suggestedName.trim(),profile:selected.profile};
  // A deliberate registration fixes the original operation's public target.
  // Later automatic local status cannot move its pending write or receipt.
  lockTarget(worker);
  setCreateAttempt({key:matchKey,ticket:++createSequence.current});
  const result=await action.run<ToolProfile>('/tool-profiles',body);
  // A transient node read must not erase an already matched original receipt.
  // The scoped component owns it; current authorization still guards display.
  if(result&&alive.current&&validQuickConnectReceipt(result,body))setRegistered(result);
 }
 return <section className="tool-quick-configuration">
  {adapter.id==='qoder-acp'&&<details className="tool-quick-muted"><summary>{tr('acp-quick-connect.native_policy')}</summary><p>{tr('acp-runtime.setup_policy')}</p></details>}
   {adapter.id==='claude-code-sdk'?<ClaudeSDKLocalConnector project={project} disabled={writeBlocked} secondary={!!receipt||available.length>0} onBlockedChange={setLocalOperationBlocked} onRegistrationReadyChange={reportNativeReady} onGuide={openAdvanced} onConnected={proof=>acceptNative(proof.worker_id,()=>setNativeConnection({caller:api,connection:{...proof}}))}/>:acpAdapterID?<QoderLocalConnector adapterID={acpAdapterID} project={project} disabled={writeBlocked} secondary={!!receipt||available.length>0} onBlockedChange={setLocalOperationBlocked} onRegistrationReadyChange={reportNativeReady} onConnected={workerID=>acceptNative(workerID,()=>setACPConnection({caller:api,workerID}))}/>:adapter.id==='codex-app-server'?<CodexLocalConnector project={project} disabled={writeBlocked} secondary={!!receipt||available.length>0} onBlockedChange={setLocalOperationBlocked} onRegistrationReadyChange={reportNativeReady} onConnected={proof=>acceptNative(proof.worker_id,()=>setNativeConnection({caller:api,connection:{...proof}}))}/>:adapter.id==='opencode-sdk'?<OpenCodeLocalConnector project={project} disabled={writeBlocked} secondary={!!receipt||available.length>0} onBlockedChange={setLocalOperationBlocked} onRegistrationReadyChange={reportNativeReady} onGuide={openAdvanced} onConnected={proof=>acceptNative(proof.worker_id,()=>setNativeConnection({caller:api,connection:{...proof}}))}/>:<DesktopToolConnector project={project} adapterID={adapter.id} disabled={writeBlocked} secondary={!!receipt||available.length>0} onBlockedChange={setLocalOperationBlocked} onGuide={openAdvanced} onConnected={workerID=>acceptNative(workerID)}/>}
  {mismatch&&<div className="tool-quick-target-mismatch" data-quick-native-target-mismatch role="status"><p>{t('native_target_mismatch',{node:mismatch.worker,target:mismatch.target})}</p><button type="button" className="text-button" data-quick-target-picker disabled={blocked||!!receipt||!nodes.length} onClick={()=>workerChoice.current?.focus()}>{t('choose_target')}</button></div>}
  {preferredMissing?<p className="tool-quick-warning" data-quick-preferred-worker-missing role="status">{tr('runner-pairing.worker_not_current')}</p>:targetMissing&&<p className="tool-quick-warning" data-quick-worker-missing role="status">{t('saved_node_missing',{node:targetWorker})}</p>}
  {!identitiesKnown?<div className="tool-quick-node-state" role="status"><p>{t('reading_nodes')}</p>{workers.error&&<ErrorNotice error={workers.error} onRetry={()=>void workers.read()}/>} {sharedWorkers.error&&<ErrorNotice error={sharedWorkers.error} onRetry={()=>void sharedWorkers.read()}/>}</div>:!nodes.length&&!preferredWorkerID?<div className="tool-quick-empty"><Server size={22} aria-hidden="true"/><div><strong>{t('no_node')}</strong><p>{t('no_node_hint')}</p><button data-quick-connect-worker type="button" className={managesLocal?'text-button':'primary'} disabled={blocked} onClick={onWorker}>{t('connect_node')}<ArrowRight size={15} aria-hidden="true"/></button></div>{!managesLocal&&!scansLocal&&<LocalInstallation project={project} adapter={adapter} onGuide={openAdvanced}/>}</div>:<>
   {nodes.length>1||!!mismatch||!!(preferredWorkerID||chosenWorker||localWorker)&&!worker?<Field label={t('node')}><select ref={workerChoice} data-quick-worker value={worker} disabled={blocked||!!receipt} onChange={event=>{lockTarget(event.target.value);setNativeMismatch(undefined);setNativeRegistrationReady(false);setNativeConnection(undefined);setACPConnection(undefined);setChosenCommand('');setChosenWorker(event.target.value);}}><option value="">{t('choose_node')}</option>{nodes.map(node=><option key={node} value={node}>{node}</option>)}</select></Field>:<p className="tool-quick-node"><Server size={15} aria-hidden="true"/>{t('using_node',{node:worker})}</p>}
    {prepareTarget&&<button type="button" className="text-button" data-quick-prepare-runner disabled={blocked} onClick={()=>{const target=prepareTarget;if(!blocked&&alive.current&&currentCaller.current===api&&target&&target===pairedRunner&&target.worker_id===worker)onPrepareRunner?.(target,adapter);}}>{tr('remote-runner-setup.prepare_node')}<ArrowRight size={15} aria-hidden="true"/></button>}
   {receipt?<div className="tool-quick-saved" role="status"><CheckCircle2 size={22} aria-hidden="true"/><div><strong>{t('registered',{name:receipt.name})}</strong><p>{t('paused_hint')}</p><button data-quick-enable type="button" className="primary" disabled={blocked} onClick={()=>onEnable(receipt)}>{t('enable')}<ArrowRight size={15} aria-hidden="true"/></button></div></div>:worker&&<>
    {!current?<Loading/>:current.error?<ErrorNotice error={toolError(current.error)} onRetry={()=>setRevision(value=>value+1)}/>:view&&<>
     <div className="tool-quick-inventory-heading"><strong>{t('detected')}</strong><button type="button" className="text-button" disabled={blocked} onClick={()=>setRevision(value=>value+1)}><RefreshCw size={14} aria-hidden="true"/>{t('refresh')}</button></div>
     {view.state==='fresh'&&!expired&&chosenCommand&&!selected&&<p className="tool-quick-warning" data-quick-command-missing role="status">{t('saved_configuration_missing',{command:chosenCommand})}</p>}
     {(expired||view.state!=='fresh')?<p className="tool-quick-warning" role="status">{t(view.state==='unreported'?'unreported':'expired')}</p>:!rows.length?<p className="tool-quick-warning" role="status">{t('no_matching')}</p>:<>
      {(available.length>1||!!chosenCommand&&!selected)&&<Field label={t('configuration')}><select data-quick-configuration value={selected?.profile.command_id||''} disabled={blocked} onChange={event=>setChosenCommand(event.target.value)}><option value="">{t('choose_configuration')}</option>{available.map(item=><option key={item.profile.command_id} value={item.profile.command_id}>{item.profile.command_id} · {item.profile.tool_version}</option>)}</select></Field>}
       {rows.filter(item=>!item.available).map(item=><div className="tool-quick-unavailable" data-quick-unavailable={item.profile.command_id} key={item.profile.command_id} role="status"><strong>{t('unavailable',{command:item.profile.command_id})}</strong><span className="tool-quick-muted" data-quick-reason-source>{t('node_reason')}</span><p data-quick-unavailable-reason={item.profile.command_id}>{item.reason}</p><button type="button" className="text-button" data-quick-configure={item.profile.command_id} disabled={blocked} onClick={openAdvanced}>{t('configure_tool')}<ArrowRight size={15} aria-hidden="true"/></button></div>)}
      {selected&&<><div className="tool-quick-facts"><span><ShieldCheck size={16} aria-hidden="true"/>{t('probe_confirmed')}</span><span>{adapter.product} · {selected.profile.tool_version}</span><span>{t(selected.profile.read_only?'read_only':'worktree_write')}</span></div>{!reusable.length&&<Field label={t('name')}><input data-quick-name required maxLength={200} value={suggestedName} disabled={blocked} onChange={event=>{setNameEdited(true);setName(event.target.value);}}/></Field>}<details className="tool-quick-technical"><summary>{t('technical')}</summary><p>{adapter.adapter} / {adapter.protocol}</p><code>{selected.profile.profile_sha256}</code></details></>}
     </>}
     {lookupBusy&&<p role="status" className="tool-quick-muted">{tr('tool-profile-reuse.loading')}</p>}
     {matches?.error&&<div><p className="tool-quick-warning">{tr('tool-profile-reuse.query_failed')}</p><ErrorNotice error={toolError(matches.error)}/></div>}
     {replacementSource?unchangedSource?<div className="tool-quick-muted" data-quick-replacement-unchanged><p>{tr('tool-profile-replacement.source_unchanged')}</p>{onOpen&&<button type="button" className="text-button" data-existing-tool-open={replacementSource.id} disabled={blocked} onClick={()=>{if(!blocked)onOpen(replacementSource);}}>{tr('tool-profile-replacement.view_source')}</button>}</div>:replacementCandidates.length>0&&<Field label={tr('tool-profile-replacement.choose_existing')}><select data-quick-replacement-existing value={selectedExisting} disabled={blocked} onChange={event=>{if(!blocked)setExistingChoice({key:matchKey,id:event.target.value});}}><option value="">{tr('tool-profile-replacement.choose_existing_placeholder')}</option>{replacementCandidates.map(item=><option key={item.id} value={item.id}>{item.name} · {tr('tool-profile-reuse.state.'+item.state)} · {item.version}</option>)}</select></Field>:matches?.data&&!!matches.data.items.length&&<ExistingToolProfiles items={matches.data.items} selectedID={selectedExisting} disabled={blocked} onSelect={id=>setExistingChoice({key:matchKey,id})} onEnable={onEnable} onOpen={onOpen}/>}
     {!!matches&&<button data-quick-matches-refresh type="button" className="text-button" disabled={blocked} onClick={readMatches}><RefreshCw size={14} aria-hidden="true"/>{tr('tool-profile-reuse.read_again')}</button>}
     {canCreate&&<p className="tool-quick-muted">{tr('tool-profile-reuse.empty')}</p>}
     {replacementSource?!unchangedSource&&(prepared?<ToolProfileReplacementConfirm draft={prepared.draft} disabled={!replacementEligible} onPauseOriginalChange={value=>{if(!replacementGate.current.eligible||!prepared)return;setReplacementPlan({...prepared,draft:{...prepared.draft,pauseOriginal:value}});}} onConfirm={()=>void confirmReplacement()} onBack={()=>{if(!blocked)setReplacementPlan(undefined);}}/>:<button type="button" className="primary tool-quick-submit" data-quick-replacement-prepare disabled={!replacementEligible} onClick={prepareReplacement}>{tr('tool-profile-replacement.prepare')}</button>):!reusable.length&&<><p className="tool-quick-muted">{t('register_hint')}</p><button data-quick-register type="button" className="primary tool-quick-submit" disabled={blocked||action.conflict||!canCreate||!usable||!suggestedName.trim()} onClick={()=>void register()}>{t(action.busy?'registering':'register')}</button></>}
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
