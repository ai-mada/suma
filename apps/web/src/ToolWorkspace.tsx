import {useApi} from './api-context';
import {ToolTaskHandoffDialog} from './ToolTaskHandoffDialog';
import {createToolTaskHandoffPin,type ToolTaskHandoffPin} from './tool-task-handoff-model';
import { Trans } from "./i18n/Trans";
import { tr as t, UiError, useLocale, formatDateTime, formatNumber, renderDiagnostic } from "./i18n";
import { lazy, Suspense, useCallback, useEffect, useId, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from "react";
import {
  ChevronDown,
  ChevronRight,
  Plus,
  RefreshCw,
  ShieldCheck,
  Terminal,
  X,
  Pause,
  Play,
  Search,
  CircleHelp,
  CheckCircle2,
  Copy,
} from "lucide-react";
import { ApiError, api, query, type Session, type RecordItem } from "./api";
import { Empty, ErrorNotice, Field, Loading, Modal } from "./ui";
import { ToolWriteRecovery } from "./tool-write";
import {manualWorkerRotationID,validManualWorkerRotationCommand,validManualWorkerRotationReceipt,type ManualWorkerRotationReceipt} from "./manual-worker-rotation-model";
import {
  toolSessionStorage,
  useToolAction,
  useManagedToolProfiles,
  useToolRead,
  type ToolAction,
} from "./tool-hooks";
import {
  parseToolManifest,
  toolStateNames,
  toolReturnRun,
  type ExecutionWorker,
  type ToolObservation,
  type ToolProfile,
} from "./tool-model";
import { ToolProfileFacts, toolReason } from "./ToolFacts";
import { ToolReadiness, matchesToolFilter, type ToolFilter } from './ToolReadiness';
import {readToolListFilters,toolListSearchParams,toolListStates,validToolListQuery,validToolListWorker,type ToolListFilters} from './tool-management-list-model';
import {ToolInventoryField} from './ToolInventoryField';
import {ToolDiscoveryPanel,type ToolDiscoveryAdoption} from './ToolDiscoveryPanel';
import {ToolRegistrationPreflight} from './ToolRegistrationPreflight';
import "./tool.css";
import "./ToolWorkspace.css";
import {classifyToolWorkerIdentity,projectToolWorkerIdentities,type ToolWorkerIdentityFilter} from './tool-worker-identity-presentation';
import './ToolWorkerIdentityList.css';
import { currentPoolToolWorker, parsePoolToolLocator } from './execution-pools/pool-config-connection-model';
import {buildPoolRecoveryOwnerHref,createPoolToolRecoveryPin,parsePoolToolRecoveryPin,readPoolRecoveryTool,type PoolToolRecoveryPin} from './execution-pools/pool-tool-recovery-model';
import {buildPoolSetupOwnerHref,createPoolToolSetupPin,parsePoolToolSetupPin,type PoolToolSetupPin} from './execution-pools/pool-tool-setup-model';
import { ToolRecoveryNotice } from "./ToolActions";
import {WorkerArtifactUploadRecovery,WorkerUploadRecoveryNotice,WorkerUploadRecoveryReceipt,useWorkerUploadRecoveryAction} from './WorkerArtifactUploadRecovery';
import type {UploadAbandonReceipt} from './worker-artifact-upload-recovery-model';
import {ToolSetupGuide} from './ToolSetupGuide';
import {toolPreparationTarget,preparationForLocalTool,type ToolPreparationPreset} from './tool-preparation-target';
import {validToolReportFeedbackSelection,type ToolReportFeedbackSelection} from './tool-report-feedback-model';
import {ToolQuickConnect,type ToolConnectDraft} from './ToolQuickConnect';
import {ToolInventoryConnectLink} from './ToolInventoryConnectLink';
import {RemoteToolScanPanel} from './RemoteToolScanPanel';
import {ToolProfileDoctor} from './ToolProfileDoctor';
import {ProfileDecisionForm} from './ToolProfileDecision';
import {ToolReinspectionPanel} from './ToolReinspectionPanel';
import {hasToolInspectionLocalOperation,readToolInspectionTarget,toolInspectionPath,validToolInspectionLocator,type ToolInspectionLocator} from './tool-reinspection-model';
import {readToolProfileDiagnostics} from './tool-profile-diagnostics-model';
import {RunnerPairingPanel} from './RunnerPairingPanel';
import {readRunnerPairing} from './runner-pairing-model';
import {validQuickConnectReceipt} from './tool-quick-connect-model';
import {buildToolProfileReplacementBody,isToolProfileReplacementPath,readToolProfileReplacement,toolProfileReplacementOriginalID,toolProfileReplacementPath,validToolProfileReplacementReceipt,type ToolProfileReplacementDraft} from './tool-profile-replacement-model';
import {ToolProfileReplacementReceipt} from './ToolProfileReplacementView';
import {ToolProfileReplacementHistory} from './ToolProfileReplacementHistory';
import {sameToolProfile} from './tool-profile-reuse-model';
import {AdapterConnections,readAdapterCatalog,type AdapterDescriptor} from './AdapterConnections';
import {DeveloperClientConnections} from './DeveloperClientConnections';
import {WorkBuddyConnections} from './WorkBuddyConnections';
import type {DeveloperClientID} from './developer-client-connections-model';
import type { ProjectPoolRunnerPage, ReleaseWorkerList, RunnerPairing, ToolProfileReplacement } from "../../../packages/contracts/src";
import {
  ToolIdentity,
  currentToolIdentity,
  type SharedIdentityRead,
  type IdentityRead,
} from "./execution-pools/ToolIdentity";
type Props = {
  project: string;
  session: Session;
  selected: string;
  onSelect: (id: string) => void;
  refresh: number;
  panel: string;
  onPoolRecovery?: (pin:PoolToolRecoveryPin) => void;
  onPoolSetup?: (pin:PoolToolSetupPin) => void;
};
const TaskDispatchDialog=lazy(()=>import('./TaskDispatch').then(module=>({default:module.TaskDispatchDialog})));
type Dialog =
  | { kind: "create"; adapter?:AdapterDescriptor; resumeQuick?:boolean }
  | { kind: "quick"; adapter?:AdapterDescriptor; preferredWorkerID?:string }
  | { kind: "remote-scan"; workerID:string; context:string; caller:unknown; generation:number }
  | { kind: "worker"; mode?:'manual'|'pairing' }
  | { kind: "inspection"; profile:ToolProfile;locator?:ToolInspectionLocator }
  | {
      kind: "decision";
      action: "enable" | "pause" | "revoke";
      profile: ToolProfile;
      justSaved?:true;
    }
  | { kind: "revoke-worker" | "rotate-worker"; worker: ExecutionWorker };
function canLeave() {
  return window.dispatchEvent(new Event("suma:navigate", { cancelable: true }));
}
export function ToolWorkspace(props:Props) {
  // Node observations, one-time credentials and dialogs belong to this exact
  // actor/project. Original uncertain writes remain in their scoped recovery key.
  return <ScopedToolWorkspace key={JSON.stringify([props.session.organization.id,props.session.user.id,props.project])} {...props}/>;
}
function ScopedToolWorkspace({
  project,
  session,
  selected,
  onSelect,
  refresh,
  panel,
  onPoolRecovery,
  onPoolSetup,
}: Props) { const api=useApi(); useLocale();
  const [revision, setRevision] = useState(0),
    [filters,setFilters]=useState(()=>readToolListFilters(new URLSearchParams(window.location.search))),
    [search, setSearch] = useState(filters.query),
    [searchError,setSearchError]=useState(false),
    [dialogState,setDialogState]=useState<{caller:typeof api;generation:number;value:Dialog}>(),
    [credentialState, setCredential] = useState<{
      caller:typeof api;
      generation:number;
      worker_id: string;
      token?: string;
      rotation?: boolean;
    }>(),
    [copyState,setCopyState]=useState<"copied"|"failed">(),
    [revealed, setRevealed] = useState(false),
    [live, setLive] = useState(true),
    [filter, setFilter] = useState<ToolFilter>('all'),
    [guideOpen, setGuideOpen] = useState(false),
    [pairingLocked, setPairingLocked] = useState(false),
    [inspectionLocked,setInspectionLocked]=useState(false),
    [receipt, setReceipt] = useState<{key:string;name:string;scope:string}>();
  const pairingCaller = useRef({api,generation:0});
  if(pairingCaller.current.api!==api)pairingCaller.current={api,generation:pairingCaller.current.generation+1};
  const pairingGeneration=pairingCaller.current.generation;
  const dialog=dialogState?.caller===api&&dialogState.generation===pairingGeneration?dialogState.value:undefined;
  const currentDialog=useRef(dialog);currentDialog.current=dialog;
  const handoffIntent=useRef(0);
  const [taskHandoff,setTaskHandoff]=useState<{epoch:number;intent:number;tool:ToolProfile;task?:RecordItem;pin?:ToolTaskHandoffPin;planID?:string}>();
  function setDialog(value:Dialog|undefined){if(pairingCaller.current.api===api&&pairingCaller.current.generation===pairingGeneration)setDialogState(value?{caller:api,generation:pairingGeneration,value}:undefined);}
  const contextKey=JSON.stringify([session.organization.id,session.user.id,project]);
  const [workerIdentityClock,setWorkerIdentityClock]=useState(()=>Date.now());
  const [workerIdentitySelection,setWorkerIdentitySelection]=useState<{caller:typeof api;generation:number;filter:ToolWorkerIdentityFilter;query:string}>(()=>({caller:api,generation:pairingGeneration,filter:'current',query:''}));
  const [workerMaintenanceOpen,setWorkerMaintenanceOpen]=useState<{caller:typeof api;id:string}>();
  const workerIdentityPreferences=workerIdentitySelection.caller===api&&workerIdentitySelection.generation===pairingGeneration?workerIdentitySelection:{filter:'current' as ToolWorkerIdentityFilter,query:''};
  const connectIntent=useRef(0),connectDraft=useRef<{caller:typeof api;generation:number;context:string;nonce:number;value:ToolConnectDraft;sourceTool?:ToolProfile;sourceEpoch?:number}|undefined>(undefined);
  const inspectionLookup=useRef<AbortController|undefined>(undefined);
  const replacementLookup=useRef<AbortController|undefined>(undefined),selectedProfile=useRef(selected);selectedProfile.current=selected;
  useEffect(()=>()=>replacementLookup.current?.abort(),[api,selected]);
  const [,renderDraft]=useState(0),[connecting,setConnecting]=useState(false),[connectError,setConnectError]=useState<unknown>();
  const activeWorkspace=useRef(true);
  useEffect(()=>{activeWorkspace.current=true;return()=>{activeWorkspace.current=false;connectIntent.current++;};},[]);
  useEffect(()=>{setConnecting(false);setConnectError(undefined);},[api]);
  const draftSource=connectDraft.current?.caller===api&&connectDraft.current.generation===pairingGeneration&&connectDraft.current.context===contextKey?connectDraft.current:undefined;
  const publicDraft=draftSource?.value;
  const replacementSource=draftSource?.sourceTool;
  const [replacementResult,setReplacementResult]=useState<{caller:typeof api;generation:number;context:string;value:ToolProfileReplacement}>();
  const replacementReceipt=replacementResult?.caller===api&&replacementResult.generation===pairingGeneration&&replacementResult.context===contextKey?replacementResult.value:undefined;
  const [pairedSelection,setPairedSelection]=useState<{caller:typeof api;generation:number;context:string;value:RunnerPairing}>();
  const pairedRunner=pairedSelection?.caller===api&&pairedSelection.generation===pairingGeneration&&pairedSelection.context===contextKey?pairedSelection.value:undefined;
  const guideID = useId();
  const workspaceElement=useRef<HTMLDivElement>(null),listHeading=useRef<HTMLHeadingElement>(null),navigationIntent=useRef(0);
  const [setupDisclosure,setSetupDisclosure]=useState(()=>({open:panel==='workbuddy'||/^workbuddy:[A-Za-z0-9_.-]+$/.test(panel),visited:false}));
  // The optional directory is mounted only after a deliberate visit, then kept
  // mounted so collapsing it never discards an authored setup draft.
  function openSetup(){setSetupDisclosure({open:true,visited:true});}
  function focusNavigation(resolve:()=>HTMLElement|null|undefined,optionalSetup=true){
    if(optionalSetup)openSetup();
    const intent=++navigationIntent.current,caller=api,generation=pairingGeneration;
    // Radix restores its opener on close. Queue the explicit destination after
    // that lifecycle, rather than changing the shared dialog's return policy.
    setTimeout(()=>requestAnimationFrame(()=>{
      if(!activeWorkspace.current||navigationIntent.current!==intent||pairingCaller.current.api!==caller||pairingCaller.current.generation!==generation)return;
      const target=resolve();if(!target||!target.isConnected)return;
      for(let ancestor=target.parentElement;ancestor&&ancestor!==workspaceElement.current;ancestor=ancestor.parentElement)if(ancestor instanceof HTMLDetailsElement)ancestor.open=true;
      target.scrollIntoView?.({block:'start',behavior:'auto'});target.focus({preventScroll:true});
    }),0);
  }
  const workerRows = useRef(new Map<string, HTMLElement>());
  const sharedWorkerRows=useRef(new Map<string,HTMLElement>());
  // Guide consumers retain their nonce high-water mark across navigation.
  // Clearing a selection must not restart this producer's request identity.
  const connectionRequestNonce=useRef(0);
  const [connectionSelection,setConnectionSelection]=useState<{context:string;caller:typeof api;generation:number;nonce:number;expectedWorkerID?:string;sourceEpoch?:number;client?:DeveloperClientID;preset?:ToolPreparationPreset|'octop';cloud?:boolean}>();
  const connection=connectionSelection?.context===contextKey&&connectionSelection.caller===api&&connectionSelection.generation===pairingGeneration?connectionSelection:undefined;
  const recovery = useMemo(
    () =>
      new ToolWriteRecovery(session.organization.id +
          ":" +
          session.user.id +
          ":" +
          project +
          ":profiles", toolSessionStorage(), api,
        (value, request) => {
          if(manualWorkerRotationID(request.path))return validManualWorkerRotationReceipt(value,request,project);
          if(isToolProfileReplacementPath(request.path))return validToolProfileReplacementReceipt(value,request,session.user.id);
          return request.path !== '/tool-profiles' || validQuickConnectReceipt(value, request.body);
        }, project),
    [session.organization.id, session.user.id, project, api],
  );
  const action = useToolAction(recovery, () => setDialog(undefined)),
    profiles = useManagedToolProfiles<ToolProfile>(project,filters,refresh+revision),
    workers = useToolRead<ReleaseWorkerList>(
      "/release-workers" + query(project),
      refresh + revision,
    ),
    sharedWorkers = useToolRead<ProjectPoolRunnerPage>(
      "/project-execution/runners" + query(project),
      refresh + revision,
    ),
    detail = useToolRead<ToolProfile>(
      selected
        ? "/tool-profiles/" + encodeURIComponent(selected) + query(project)
        : "",
      refresh + revision,
    );
  const workerAccessLost=workers.failure instanceof ApiError&&[401,403,404].includes(workers.failure.status);
  const workerManagement=workers.data?.credential_management_allowed;
  const workerManagementDenied=workerAccessLost||workerManagement===false;
  const workerManagementReady=workerManagement===true&&!workers.loading&&!workers.error;
  const credentialDialog=dialog?.kind==='rotate-worker'||dialog?.kind==='revoke-worker'||(dialog?.kind==='worker'&&dialog.mode!=='pairing');
  const credentialWriteRefused=credentialDialog&&(action.errorStatus===401||action.errorStatus===403);
  const credentialDialogDenied=credentialDialog&&(workerManagementDenied||credentialWriteRefused);
  const uploadAction=useWorkerUploadRecoveryAction(project,session.user.id,session.organization.id);
  const uploadRecoveryLocked=uploadAction.busy||!!uploadAction.pending;
  const [uploadReceipt,setUploadReceipt]=useState<UploadAbandonReceipt>();
  useEffect(()=>{
    if(!workerManagementDenied)return;
    setUploadReceipt(undefined);
    setWorkerMaintenanceOpen(undefined);
  },[workerManagementDenied]);
  const credential=credentialState?.caller===api&&credentialState.generation===pairingGeneration&&!workerManagementDenied?credentialState:undefined;
  const visibleCredential=useRef(credential);visibleCredential.current=credential;
  function clearCredential(){
    setCredential(undefined);
    setRevealed(false);
    setCopyState(undefined);
  }
  useEffect(()=>{if(credentialState&&!credential)clearCredential();},[credentialState,credential]);
  useEffect(()=>{
    if(!credentialDialogDenied)return;
    setDialog(undefined);
    if(credentialWriteRefused)void workers.read();
    focusNavigation(()=>document.getElementById(guideID+'-worker-identities'),false);
  },[credentialDialogDenied,credentialWriteRefused,workers.read]);
  const detailBasis=JSON.stringify([selected,detail.data?.id,detail.data?.version,detail.data?.worker_id,detail.data?.profile.profile_sha256]),latestDetailBasis=useRef(detailBasis);latestDetailBasis.current=detailBasis;
  // A value-equal A -> B -> A detail selection is a new read owner. This ref
  // carries provenance only; existing drafts and operation receipts stay put.
  const detailOwner=useRef({key:detailBasis,epoch:0});
  if(detailOwner.current.key!==detailBasis)detailOwner.current={key:detailBasis,epoch:detailOwner.current.epoch+1};
  const detailEpoch=detailOwner.current.epoch;
  useEffect(()=>()=>inspectionLookup.current?.abort(),[api,detailBasis]);
  const refreshAll = useCallback(() => {
    if (!profiles.loading) void profiles.read();
    if (!workers.loading) void workers.read();
    if (!sharedWorkers.loading) void sharedWorkers.read();
    if (selected && !detail.loading) void detail.read();
  }, [profiles.loading, profiles.read, workers.loading, workers.read, sharedWorkers.loading, sharedWorkers.read, selected, detail.loading, detail.read]);
  useEffect(() => {
    // Keep a deliberate task handoff stable while its own exact reads run.
    // The original background refresh resumes when the dialog is closed.
    if (!live || taskHandoff) return;
    const timer = setInterval(() => {
      if (document.visibilityState !== 'hidden') refreshAll();
    }, 10000);
    return () => clearInterval(timer);
  }, [live, refreshAll, taskHandoff]);
  const pending = !!action.pending,
    otherWriteBlocked = pending || action.busy || pairingLocked || inspectionLocked || connecting,
    writeBlocked = otherWriteBlocked || uploadRecoveryLocked,
    current = detail.data,
    stale = detail.loading || !!detail.error,
    uncertain = profiles.loading || !!profiles.error || !profiles.items,
    filtered = profiles.items?.filter((profile) => matchesToolFilter(profile, filter));
  const workerIdentityRecords=workers.data?.items??[];
  const rotationWorker=dialog?.kind==='rotate-worker'?dialog.worker:undefined;
  const maintenanceWorker=dialog?.kind==='revoke-worker'?dialog.worker:rotationWorker;
  const currentMaintenanceWorker=maintenanceWorker&&workerIdentityRecords.find(row=>
    row.id===maintenanceWorker.id&&row.worker_id===maintenanceWorker.worker_id&&row.version===maintenanceWorker.version&&row.project_id===project&&row.status==='active');
  const rotationWorkerCurrent=workerManagementReady&&currentMaintenanceWorker?.manual_rotation_allowed===true;
  const rotationUploadReceipt=!workerManagementDenied&&rotationWorker&&uploadReceipt?.worker_registry_id===rotationWorker.id?uploadReceipt:undefined;
  const workerIdentityView=projectToolWorkerIdentities(workerIdentityRecords,workerIdentityClock,workerIdentityPreferences.filter,workerIdentityPreferences.query);
  const matchingWorkerIdentityIDs=new Set(workerIdentityView.items.map(worker=>worker.id));
  // Keep the exact context row mounted and visible when a filter would hide an
  // original operation, setup draft or explicitly selected Doctor target.
  const workerIdentityTargets=new Set([
    publicDraft?.workerID,connection?.expectedWorkerID,
    current?.id===selected?current.worker_id:undefined,
    dialog?.kind==='remote-scan'?dialog.workerID:(dialog?.kind==='revoke-worker'||dialog?.kind==='rotate-worker')?dialog.worker.worker_id:dialog?.kind==='inspection'?dialog.profile.worker_id:undefined,
    typeof action.pending?.body.worker_id==='string'?action.pending.body.worker_id:undefined,
    workerIdentityRecords.find(worker=>action.pending?.path==='/release-workers/'+encodeURIComponent(worker.id)||worker.id===manualWorkerRotationID(action.pending?.path||''))?.worker_id,
  ].filter((value):value is string=>typeof value==='string'&&!!value));
  const retainedWorkerIdentities=workerIdentityRecords.filter(worker=>!matchingWorkerIdentityIDs.has(worker.id)&&workerIdentityTargets.has(worker.worker_id));
  const retainedWorkerIdentityIDs=new Set(retainedWorkerIdentities.map(worker=>worker.id));
  function updateWorkerIdentityView(filter:ToolWorkerIdentityFilter,query=workerIdentityPreferences.query){
    if(writeBlocked)return;
    setWorkerIdentitySelection({caller:api,generation:pairingGeneration,filter,query});
  }
  // Expiry is a local display boundary. These timers never read or mutate API
  // state, renew a credential, or infer online/dispatch eligibility.
  useEffect(()=>{setWorkerIdentityClock(Date.now());},[api,workers.data]);
  useEffect(()=>{
    const now=Date.now();let delay=60000;
    if(Number.isFinite(now))for(const worker of workerIdentityRecords){const expiry=Date.parse(worker.expires_at);if(worker.status==='active'&&Number.isFinite(expiry)&&expiry>now)delay=Math.min(delay,Math.max(1,expiry-now));}
    const timer=setTimeout(()=>setWorkerIdentityClock(Date.now()),delay);
    const visible=()=>{if(document.visibilityState!=='hidden')setWorkerIdentityClock(Date.now());};
    document.addEventListener('visibilitychange',visible);
    return()=>{clearTimeout(timer);document.removeEventListener('visibilitychange',visible);};
  },[api,workers.data,workerIdentityClock]);
  // A detail refresh is not a new dispatch owner. Once the task dialog exists,
  // its original pending operation must survive list/detail loading changes.
  // Exact task/profile reads and original preview remain authoritative.
  const handoffKey=JSON.stringify([contextKey,pairingGeneration]);
  const handoffOwner=useRef({key:handoffKey,epoch:0});
  if(handoffOwner.current.key!==handoffKey)handoffOwner.current={key:handoffKey,epoch:handoffOwner.current.epoch+1};
  const handoff=taskHandoff?.epoch===handoffOwner.current.epoch?taskHandoff:undefined;
  useEffect(()=>{if(taskHandoff&&!handoff)setTaskHandoff(undefined);},[taskHandoff,handoff]);
  const handoffPin=current&&!stale?createToolTaskHandoffPin(current,project):undefined;
  const ownsHandoff=(value:{epoch:number;intent:number})=>activeWorkspace.current&&pairingCaller.current.api===api&&pairingCaller.current.generation===pairingGeneration&&handoffOwner.current.epoch===value.epoch&&handoffIntent.current===value.intent;
  function closeTaskHandoff(){
    if(!handoff||!ownsHandoff(handoff)||writeBlocked)return;
    handoffIntent.current++;
    setTaskHandoff(undefined);
    focusNavigation(()=>workspaceElement.current?.querySelector<HTMLElement>('[data-tool-use-for-task]'),false);
  }
  const replacementSourceCurrent=!!replacementSource&&draftSource?.sourceEpoch===detailEpoch&&!stale&&current?.id===replacementSource.id&&current.version===replacementSource.version&&current.worker_id===replacementSource.worker_id&&current.state===replacementSource.state&&sameToolProfile(current.profile,replacementSource.profile);
  const reportReturnOwner=useRef({api,contextKey,generation:pairingGeneration,nonce:connection?.nonce,writeBlocked,workers,sharedWorkers});reportReturnOwner.current={api,contextKey,generation:pairingGeneration,nonce:connection?.nonce,writeBlocked,workers,sharedWorkers};
  const poolNavigation=useRef<{caller:typeof api;generation:number;panel:string}|undefined>(undefined);
  // The locator opens only the original UI. Current project identity and
  // catalog reads remain authoritative; no scan, registration or enable runs.
  useEffect(()=>{
    const locator=parsePoolToolLocator(panel);
    if(!locator||pending||action.busy||pairingLocked||inspectionLocked||sharedWorkers.loading||sharedWorkers.error||!sharedWorkers.data)return;
    if(poolNavigation.current?.caller===api&&poolNavigation.current.generation===pairingGeneration&&poolNavigation.current.panel===panel)return;
    const ticket={caller:api,generation:pairingGeneration,panel};poolNavigation.current=ticket;
    const workerID=currentPoolToolWorker(locator,sharedWorkers.data);
    if(!workerID){setConnectError(new UiError('tool-workspace.target_unavailable'));return;}
    const controller=new AbortController(),nonce=++connectIntent.current;let completed=false;
    setConnecting(true);setConnectError(undefined);
    const owns=()=>activeWorkspace.current&&!controller.signal.aborted&&nonce===connectIntent.current&&pairingCaller.current.api===api&&pairingCaller.current.generation===pairingGeneration;
    void(async()=>{
      try{
        const catalog=readAdapterCatalog(await api<unknown>('/adapter-catalog'+query(project),undefined,'GET',undefined,controller.signal),project);
        if(!owns())return;
        if(!catalog?.items.some(item=>item.direction==='suma_to_tool'&&item.implementation==='implemented'&&item.setup_kind==='runner_profile'))throw new UiError('tool-workspace.target_unavailable');
        beginPublicDraft({adapterID:'',workerID,commandID:'',name:'',nameEdited:false});
        action.clearError();setDialog({kind:'quick',preferredWorkerID:workerID});completed=true;
      }catch(error){if(owns())setConnectError(error);}
      finally{if(activeWorkspace.current&&pairingCaller.current.api===api&&pairingCaller.current.generation===pairingGeneration&&(connectIntent.current===nonce||completed))setConnecting(false);}
    })();
    return()=>{controller.abort();if(!completed&&poolNavigation.current===ticket)poolNavigation.current=undefined;};
  },[api,project,panel,pairingGeneration,pending,action.busy,pairingLocked,inspectionLocked,sharedWorkers.loading,sharedWorkers.error,sharedWorkers.data]);
  // A return locator keeps the original source for an explicit replacement.
  // Current exact profile, shared row and diagnostics are read again; the URL
  // neither enables a tool nor substitutes another version after a drift.
  const poolRecoveryNavigation=useRef<{caller:typeof api;generation:number;panel:string;epoch:number}|undefined>(undefined);
  useEffect(()=>{
    const pin=parsePoolToolRecoveryPin(panel);
    if(!pin){poolRecoveryNavigation.current=undefined;return;}
    if(pin.tenant_id!==session.organization.id||pin.project_id!==project||pin.tool_profile_id!==selected)return;
    if(pending||action.busy||pairingLocked||inspectionLocked||stale||sharedWorkers.loading||sharedWorkers.error||!sharedWorkers.data)return;
    const old=poolRecoveryNavigation.current;
    if(old?.caller===api&&old.generation===pairingGeneration&&old.panel===panel&&old.epoch===detailEpoch)return;
    const source=readPoolRecoveryTool(current,pin);
    const rowPin=source&&createPoolToolRecoveryPin(source,session.organization.id,sharedWorkers.data);
    if(!source||!rowPin||rowPin.pool_id!==pin.pool_id||rowPin.runner_id!==pin.runner_id){setConnectError(new UiError('pool-tool-recovery-navigation.changed'));return;}
    const ticket={caller:api,generation:pairingGeneration,panel,epoch:detailEpoch};poolRecoveryNavigation.current=ticket;
    const controller=new AbortController(),nonce=++connectIntent.current,epoch=detailEpoch;
    let completed=false;setConnecting(true);setConnectError(undefined);
    const owns=()=>activeWorkspace.current&&!controller.signal.aborted&&connectIntent.current===nonce&&detailOwner.current.epoch===epoch&&pairingCaller.current.api===api&&pairingCaller.current.generation===pairingGeneration;
    void(async()=>{try{
      const localScope={project,toolID:source.id,userID:session.user.id,organizationID:session.organization.id};
      if(hasToolInspectionLocalOperation(localScope,toolSessionStorage())){await inspectProfile(source);completed=true;return;}
      const started=performance.now();
      const {report,remainingMs}=readToolProfileDiagnostics(await api<unknown>('/tool-profiles/'+encodeURIComponent(source.id)+'/diagnostics'+query(project),undefined,'GET',undefined,controller.signal),{project,toolID:source.id});
      if(!owns())return;
      const fresh=readPoolRecoveryTool(report.tool,pin);
      if(!fresh||remainingMs-(performance.now()-started)<=0)throw new UiError('pool-tool-recovery-navigation.changed');
      const resolution=report.maintenance_resolution;
      if((resolution.action==='view_inspection'||resolution.action==='recover_inspection'||resolution.action==='view_recovery')&&resolution.inspection_id){await inspectProfile(fresh,{inspectionID:resolution.inspection_id,...(resolution.recovery_id?{recoveryID:resolution.recovery_id}:{})});completed=true;}
      // configureProfile owns a new intent nonce; this outer source ticket and
      // abort signal still retire it when a shared-source refresh invalidates us.
      else completed=await configureProfile(fresh,'quick',()=>!controller.signal.aborted&&poolRecoveryNavigation.current===ticket&&remainingMs-(performance.now()-started)>0)===true;
    }catch(error){if(owns())setConnectError(error);}
    finally{if(!completed&&poolRecoveryNavigation.current===ticket)poolRecoveryNavigation.current=undefined;if(activeWorkspace.current&&pairingCaller.current.api===api&&pairingCaller.current.generation===pairingGeneration&&connectIntent.current===nonce)setConnecting(false);}})();
    return()=>{controller.abort();if(!completed&&poolRecoveryNavigation.current===ticket)poolRecoveryNavigation.current=undefined;};
  },[api,project,panel,selected,session.organization.id,session.user.id,pairingGeneration,pending,action.busy,pairingLocked,inspectionLocked,stale,current,detailEpoch,sharedWorkers.loading,sharedWorkers.error,sharedWorkers.data]);
  // A fresh shared read retires an in-flight owner handoff even when an A -> B
  // -> A refresh later restores value-equal data. This tracks provenance only.
  const poolSetupReadOwner=useRef({api,contextKey,generation:pairingGeneration,data:sharedWorkers.data,loading:sharedWorkers.loading,error:sharedWorkers.error});
  if(poolSetupReadOwner.current.api!==api||poolSetupReadOwner.current.contextKey!==contextKey||poolSetupReadOwner.current.generation!==pairingGeneration||poolSetupReadOwner.current.data!==sharedWorkers.data||poolSetupReadOwner.current.loading!==sharedWorkers.loading||poolSetupReadOwner.current.error!==sharedWorkers.error)
    poolSetupReadOwner.current={api,contextKey,generation:pairingGeneration,data:sharedWorkers.data,loading:sharedWorkers.loading,error:sharedWorkers.error};
  const poolSetupNavigation=useRef<{api:typeof api;generation:number;panel:string}|undefined>(undefined);
  useEffect(()=>{
    const pin=parsePoolToolSetupPin(panel);
    if(!pin){poolSetupNavigation.current=undefined;return;}
    if(pin.tenant_id!==session.organization.id||pin.project_id!==project||selected||pending||action.busy||pairingLocked||inspectionLocked||sharedWorkers.loading||sharedWorkers.error||!sharedWorkers.data)return;
    const previous=poolSetupNavigation.current;
    if(previous?.api===api&&previous.generation===pairingGeneration&&previous.panel===panel)return;
    const ticket={api,generation:pairingGeneration,panel};poolSetupNavigation.current=ticket;
    const readOwner=poolSetupReadOwner.current,controller=new AbortController(),nonce=++connectIntent.current;
    let completed=false,settledNonce=nonce;setConnecting(true);setConnectError(undefined);
    const owns=()=>activeWorkspace.current&&!controller.signal.aborted&&poolSetupNavigation.current===ticket&&poolSetupReadOwner.current===readOwner&&connectIntent.current===nonce&&pairingCaller.current.api===api&&pairingCaller.current.generation===pairingGeneration;
    void(async()=>{try{
      const catalog=readAdapterCatalog(await api<unknown>('/adapter-catalog'+query(project),undefined,'GET',undefined,controller.signal),project);
      if(!owns())return;
      const choices=catalog?.items.filter(a=>a.id===pin.adapter_id&&a.adapter===pin.adapter&&a.protocol===pin.protocol&&toolPreparationTarget(a));
      const adapter=choices?.length===1?choices[0]:undefined;
      const currentPin=adapter&&createPoolToolSetupPin(adapter,pin.worker_id,pin.tenant_id,pin.project_id,sharedWorkers.data);
      if(!currentPin||currentPin.pool_id!==pin.pool_id||currentPin.runner_id!==pin.runner_id)throw new UiError('shared-tool-onboarding-navigation.changed');
      // This is a new setup. Existing profile IDs or replacement evidence must
      // never be invented from installation metadata or a navigation URL.
      settledNonce=beginPublicDraft({adapterID:pin.adapter_id,workerID:pin.worker_id,commandID:'',name:'',nameEdited:false});
      action.clearError();setDialog({kind:'quick',adapter,preferredWorkerID:pin.worker_id});completed=true;
    }catch(error){if(owns())setConnectError(error);}
    finally{if(!completed&&poolSetupNavigation.current===ticket)poolSetupNavigation.current=undefined;if(activeWorkspace.current&&pairingCaller.current.api===api&&pairingCaller.current.generation===pairingGeneration&&connectIntent.current===settledNonce)setConnecting(false);}})();
    return()=>{controller.abort();if(!completed&&poolSetupNavigation.current===ticket)poolSetupNavigation.current=undefined;};
  },[api,project,panel,selected,session.organization.id,pairingGeneration,pending,action.busy,pairingLocked,inspectionLocked,sharedWorkers.loading,sharedWorkers.error,sharedWorkers.data]);
  async function prepareSharedTool(localTool:string,sourceCurrent:()=>boolean){
    const workerID=connection?.expectedWorkerID,connectionNonce=connection?.nonce;
    if(writeBlocked||!workerID?.startsWith('pool-')||!sourceCurrent()||sharedWorkers.loading||sharedWorkers.error||!sharedWorkers.data)return;
    if(replacementSource&&!replacementSourceCurrent){setConnectError(new UiError('shared-tool-onboarding-navigation.changed'));return;}
    const readOwner=poolSetupReadOwner.current,sourceEpoch=detailOwner.current.epoch,nonce=++connectIntent.current;
    const owns=()=>activeWorkspace.current&&detailOwner.current.epoch===sourceEpoch&&poolSetupReadOwner.current===readOwner&&connectIntent.current===nonce&&connectionRequestNonce.current===connectionNonce&&pairingCaller.current.api===api&&pairingCaller.current.generation===pairingGeneration&&sourceCurrent();
    // Do not set the whole Guide to disabled for this read: its provenance
    // correctly treats a true pending-operation transition as cancellation.
    setConnectError(undefined);
    try{
      const catalog=readAdapterCatalog(await api<unknown>('/adapter-catalog'+query(project)),project);if(!owns())return;
      const choices=catalog?.items.filter(a=>a.id===localTool&&toolPreparationTarget(a)),adapter=choices?.length===1?choices[0]:undefined;
      if(!adapter)throw new UiError('shared-tool-onboarding-navigation.changed');
      const page=await api<ProjectPoolRunnerPage>('/project-execution/runners'+query(project));if(!owns())return;
      const pin=createPoolToolSetupPin(adapter,workerID,session.organization.id,project,page);
      if(!pin)throw new UiError('shared-tool-onboarding-navigation.changed');
      const original=replacementSource&&replacementSourceCurrent&&replacementSource.worker_id===workerID&&replacementSource.profile.adapter===adapter.adapter&&replacementSource.profile.protocol===adapter.protocol?createPoolToolRecoveryPin(replacementSource,session.organization.id,page):undefined;
      if(!owns()||!canLeave())return;
      if(original){if(onPoolRecovery)onPoolRecovery(original);else{const href=buildPoolRecoveryOwnerHref(original);if(href)window.location.assign(href);}}
      else if(onPoolSetup)onPoolSetup(pin);else{const href=buildPoolSetupOwnerHref(pin);if(href)window.location.assign(href);}
    }catch(error){if(owns())setConnectError(error);}
  }
  function applyFilters(value:ToolListFilters){
    if(!validToolListQuery(value.query)){setSearchError(true);return;}
    const next={...value,query:value.query.trim()};setSearchError(false);setFilters(next);setFilter('all');
    const params=toolListSearchParams(new URLSearchParams(window.location.search),next);
    window.history.replaceState(window.history.state,'',window.location.pathname+(params.size?'?'+params:'')+window.location.hash);
  }
  function showWorkerProfiles(workerID:string){
    if(writeBlocked||!validToolListWorker(workerID)||!canLeave())return;
    setSearch('');
    applyFilters({state:'all',query:'',worker:workerID});
    if(selected)onSelect('');
    focusNavigation(()=>listHeading.current,false);
  }
  function clearWorkerFilter(){
    if(writeBlocked||!canLeave())return;
    applyFilters({state:filters.state,query:filters.query});
    focusNavigation(()=>listHeading.current,false);
  }
  function clearToolFilters(){
    if(filters.worker&&(writeBlocked||!canLeave()))return;
    setSearch('');
    applyFilters({state:'active',query:''});
    if(filters.worker)focusNavigation(()=>listHeading.current,false);
  }
  useEffect(()=>{const update=()=>{const next=readToolListFilters(new URLSearchParams(window.location.search));setFilters(next);setSearch(next.query);setSearchError(false);setFilter('all');};window.addEventListener('popstate',update);return()=>window.removeEventListener('popstate',update);},[]);
  const identitiesKnown = !!workers.data && !workers.loading && !workers.error && !!sharedWorkers.data && !sharedWorkers.loading && !sharedWorkers.error;
  const identityCount = new Set([
    ...(workers.data?.items || []), ...(sharedWorkers.data?.items || []),
  ].filter(item => item.status === 'active').map(item => item.worker_id)).size;
  const hasToolFilters=filters.state!=='active'||!!filters.query||!!filters.worker;
  function openRegister() {
    if(writeBlocked)return;
    action.clearError();
    void workers.read();
    void sharedWorkers.read();
    setDialog({kind:'create'});
  }
  function openQuickConnect() {
    if(writeBlocked)return;
    action.clearError();
    void workers.read();
    void sharedWorkers.read();
    beginPublicDraft(publicDraft??{adapterID:'',workerID:'',commandID:'',name:'',nameEdited:false},replacementSource);
    setDialog({kind:'quick'});
  }
  function returnFromReport(value:ToolReportFeedbackSelection){
    const owner=reportReturnOwner.current,target=preparationForLocalTool(value.adapterID);
    if(!activeWorkspace.current||owner.writeBlocked||owner.api!==api||owner.contextKey!==contextKey||owner.generation!==pairingGeneration||owner.nonce!==connection?.nonce||value.project!==project||!target||!validToolReportFeedbackSelection(value,{project,workerID:value.workerID,localTool:target.localTool}))return;
    if(connection?.expectedWorkerID&&value.workerID!==connection.expectedWorkerID)return;
    if(connection?.sourceEpoch!==undefined&&connection.sourceEpoch!==detailOwner.current.epoch)return;
    const sameTarget=publicDraft?.adapterID===value.adapterID&&publicDraft.workerID===value.workerID;
    beginPublicDraft({adapterID:value.adapterID,workerID:value.workerID,commandID:value.commandID,name:sameTarget?publicDraft.name:'',nameEdited:sameTarget?publicDraft.nameEdited:false,observationPin:Object.freeze({profileSHA256:value.profileSHA256,observedAt:value.observedAt})},sameTarget?replacementSource:undefined);
    action.clearError();void workers.read();void sharedWorkers.read();setDialog({kind:'quick',preferredWorkerID:value.workerID});
  }
  async function returnFromManagedInventory(value:ToolReportFeedbackSelection,preparationID:string,source:Extract<Dialog,{kind:'remote-scan'}>){
    const target=preparationForLocalTool(value.adapterID),nonce=++connectIntent.current;
    const owns=()=>activeWorkspace.current&&!writeBlocked&&currentDialog.current===source&&pairingCaller.current.api===api&&pairingCaller.current.generation===pairingGeneration&&connectIntent.current===nonce;
    if(!owns()||!/^[A-Za-z0-9][A-Za-z0-9_.-]{0,149}$/.test(preparationID)||value.workerID!==source.workerID||!target||!validToolReportFeedbackSelection(value,{project,workerID:source.workerID,localTool:target.localTool}))return;
    try{
      const catalog=readAdapterCatalog(await api<unknown>('/adapter-catalog'+query(project)),project);if(!owns())return;
      const choices=catalog?.items.filter(adapter=>adapter.id===value.adapterID&&toolPreparationTarget(adapter));if(choices?.length!==1)throw new UiError('tool-workspace.target_unavailable');
      beginPublicDraft({adapterID:value.adapterID,workerID:value.workerID,commandID:value.commandID,name:'',nameEdited:false,observationPin:Object.freeze({profileSHA256:value.profileSHA256,observedAt:value.observedAt,preparationID})});
      action.clearError();void workers.read();void sharedWorkers.read();setDialog({kind:'quick',adapter:choices[0],preferredWorkerID:value.workerID});
    }catch(error){if(owns())setConnectError(error);}
  }
  function updatePublicDraft(value:ToolConnectDraft,nonce:number|undefined){
    const owner=connectDraft.current;
    if(!owner||owner.nonce!==nonce||owner.caller!==api||owner.context!==contextKey||owner.generation!==pairingGeneration||pairingCaller.current.api!==api||pairingCaller.current.generation!==pairingGeneration)return;
    if(JSON.stringify(owner.value)!==JSON.stringify(value)){connectDraft.current={...owner,value:{...value}};renderDraft(value=>value+1);}
  }
  function beginPublicDraft(value:ToolConnectDraft,sourceTool?:ToolProfile){
    const nonce=++connectIntent.current;connectDraft.current={caller:api,generation:pairingGeneration,context:contextKey,nonce,value:{...value},...(sourceTool?{sourceTool:structuredClone(sourceTool),sourceEpoch:detailOwner.current.epoch}:{})};renderDraft(value=>value+1);return nonce;
  }
  async function configureProfile(value:ToolProfile,destination:'quick'|'worker'|'shared-worker'='quick',sourceCurrent?:()=>boolean){
    if(writeBlocked||stale||!current||value.project_id!==project||value.id!==current.id||value.version!==current.version||value.worker_id!==current.worker_id||value.profile.profile_sha256!==current.profile.profile_sha256)return;
    if(destination==='worker'&&!ordinaryWorkerReady(value.worker_id)){setConnectError(new UiError('tool-recovery-navigation.changed'));return;}
    const sourceEpoch=detailOwner.current.epoch;
    const nonce=++connectIntent.current;let settledNonce=nonce;setConnecting(true);setConnectError(undefined);
    try{
      const catalog=readAdapterCatalog(await api<unknown>('/adapter-catalog'+query(project)),project);
      if(!activeWorkspace.current||nonce!==connectIntent.current||detailOwner.current.epoch!==sourceEpoch||latestDetailBasis.current!==detailBasis||pairingCaller.current.api!==api||pairingCaller.current.generation!==pairingGeneration)return;
      if(sourceCurrent&&!sourceCurrent())throw new UiError('pool-tool-recovery-navigation.changed');
      if(destination==='worker'&&!ordinaryWorkerReady(value.worker_id))throw new UiError('tool-recovery-navigation.changed');
      const matches=catalog?.items.filter(item=>item.direction==='suma_to_tool'&&item.implementation==='implemented'&&item.setup_kind==='runner_profile'&&item.adapter===value.profile.adapter&&item.protocol===value.profile.protocol);
      if(matches?.length!==1)throw new Error(t('tool-workspace.target_unavailable'));
      settledNonce=beginPublicDraft({adapterID:matches[0].id,workerID:value.worker_id,commandID:value.profile.command_id,name:value.name,nameEdited:true},value.state==='revoked'?undefined:value);
      action.clearError();void workers.read();void sharedWorkers.read();
      if(destination!=='quick'){
        setDialog(undefined);focusNavigation(()=>(destination==='shared-worker'?sharedWorkerRows:workerRows).current.get(value.worker_id),false);
        if(destination==='worker'&&!toolPreparationTarget(matches[0]))setConnectError(new UiError('tool-recovery-navigation.unsupported'));
      }else setDialog({kind:'quick',adapter:matches[0],preferredWorkerID:value.worker_id});
      return true;
    }catch(error){if(activeWorkspace.current&&nonce===connectIntent.current&&pairingCaller.current.api===api)setConnectError(error);}
    finally{if(activeWorkspace.current&&connectIntent.current===settledNonce&&pairingCaller.current.api===api&&pairingCaller.current.generation===pairingGeneration)setConnecting(false);}
  }
  // The existing public draft owns this short-lived handoff. It is not a
  // separate node status or authority, and a shared Pool identity is excluded.
  function ordinaryWorkerReady(workerID:string){
    const owner=reportReturnOwner.current;
    return owner.api===api&&owner.contextKey===contextKey&&owner.generation===pairingGeneration&&!owner.workers.loading&&!owner.workers.error&&!!owner.workers.data?.items.some(worker=>worker.worker_id===workerID)&&!owner.sharedWorkers.loading&&!owner.sharedWorkers.error&&!!owner.sharedWorkers.data&&!owner.sharedWorkers.data.items.some(worker=>worker.worker_id===workerID);
  }
  function workerRecoverySource(workerID:string):ToolProfile|undefined{
    const target=publicDraft&&preparationForLocalTool(publicDraft.adapterID);
    if(!ordinaryWorkerReady(workerID)||!replacementSourceCurrent||!replacementSource||!publicDraft||!target||publicDraft.workerID!==workerID||replacementSource.worker_id!==workerID||target.adapter!==replacementSource.profile.adapter||target.protocol!==replacementSource.profile.protocol)return;
    return replacementSource;
  }
  function sharedRecoveryPin(workerID:string):PoolToolRecoveryPin|undefined{
    if(!replacementSourceCurrent||!replacementSource||!publicDraft||publicDraft.workerID!==workerID||replacementSource.worker_id!==workerID||sharedWorkers.loading||sharedWorkers.error)return;
    return createPoolToolRecoveryPin(replacementSource,session.organization.id,sharedWorkers.data);
  }
  function sharedRecoveryEntry(workerID:string){
    const pin=sharedRecoveryPin(workerID);if(!pin)return;
    return <div className="tool-worker-recovery" data-pool-tool-recovery-entry={workerID}><p>{t('pool-tool-recovery-navigation.owner_target')}<small>{t('pool-tool-recovery-navigation.owner_hint')}</small></p><button type="button" data-pool-tool-recovery-open={workerID} disabled={stale||writeBlocked} onClick={()=>{
      const currentPin=sharedRecoveryPin(workerID);if(!currentPin||!canLeave())return;
      if(onPoolRecovery)onPoolRecovery(currentPin);else{const href=buildPoolRecoveryOwnerHref(currentPin);if(href)window.location.assign(href);}
    }}>{t('pool-tool-recovery-navigation.manage')}</button></div>;
  }
  async function prepareWorkerConnection(value:ToolProfile){
    const source=workerRecoverySource(value.worker_id);
    if(writeBlocked||stale||!source||source.id!==value.id||source.version!==value.version||!sameToolProfile(source.profile,value.profile))return;
    const scope={project,toolID:value.id,userID:session.user.id,organizationID:session.organization.id};
    if(hasToolInspectionLocalOperation(scope,toolSessionStorage())){await inspectProfile(value);return;}
    const sourceEpoch=detailOwner.current.epoch;
    let nonce=++connectIntent.current;setConnecting(true);setConnectError(undefined);
    const owns=()=>activeWorkspace.current&&connectIntent.current===nonce&&pairingCaller.current.api===api&&pairingCaller.current.generation===pairingGeneration;
    const currentSource=()=>owns()&&detailOwner.current.epoch===sourceEpoch&&latestDetailBasis.current===detailBasis;
    try{
      const started=performance.now();
      const {report,remainingMs}=readToolProfileDiagnostics(await api<unknown>('/tool-profiles/'+encodeURIComponent(value.id)+'/diagnostics'+query(project)),{project,toolID:value.id});
      if(!currentSource())return;
      if(!ordinaryWorkerReady(value.worker_id))throw new UiError('tool-recovery-navigation.changed');
      if(remainingMs-(performance.now()-started)<=0||report.tool.version!==value.version||report.tool.worker_id!==value.worker_id||!sameToolProfile(report.tool.profile,value.profile))throw new UiError('tool-recovery-navigation.changed');
      const resolution=report.maintenance_resolution;
      if((resolution.action==='view_inspection'||resolution.action==='recover_inspection'||resolution.action==='view_recovery')&&resolution.inspection_id){
        await inspectProfile(report.tool,{inspectionID:resolution.inspection_id,...(resolution.recovery_id?{recoveryID:resolution.recovery_id}:{})});return;
      }
      if(report.tool.state==='revoked')throw new UiError('tool-recovery-navigation.changed');
      if(report.checks.some(check=>check.code==='worker'&&check.status==='blocked')){
        nonce=beginPublicDraft({...publicDraft!,workerID:value.worker_id},value);
        action.clearError();setDialog({kind:'worker',mode:'pairing'});return;
      }
      const catalog=readAdapterCatalog(await api<unknown>('/adapter-catalog'+query(project)),project);
      if(!currentSource())return;
      if(!ordinaryWorkerReady(value.worker_id)||remainingMs-(performance.now()-started)<=0)throw new UiError('tool-recovery-navigation.changed');
      const matches=catalog?.items.filter(item=>item.direction==='suma_to_tool'&&item.implementation==='implemented'&&item.setup_kind==='runner_profile'&&item.adapter===value.profile.adapter&&item.protocol===value.profile.protocol&&toolPreparationTarget(item));
      if(matches?.length!==1)throw new UiError('tool-recovery-navigation.unsupported');
      nonce=beginPublicDraft({adapterID:matches[0].id,workerID:value.worker_id,commandID:value.profile.command_id,name:value.name,nameEdited:true},value);
      action.clearError();openToolPreparation(matches[0],value.worker_id,true);
    }catch(error){if(currentSource())setConnectError(error);}
    finally{if(owns())setConnecting(false);}
  }
  function workerConnectionPreparation(workerID:string){
    const source=workerRecoverySource(workerID);if(!source)return;
    return <div className="tool-worker-recovery" data-tool-worker-recovery={workerID}><p>{t('tool-recovery-navigation.target',{name:source.name})}<small>{t('tool-recovery-navigation.boundary')}</small></p><button type="button" data-tool-worker-prepare={workerID} disabled={writeBlocked||stale} onClick={()=>void prepareWorkerConnection(source)}>{t('tool-recovery-navigation.prepare')}</button></div>;
  }
  async function inspectProfile(value:ToolProfile,locator?:ToolInspectionLocator){
    if(stale||writeBlocked||!current||value.project_id!==project||value.id!==current.id||value.version!==current.version||value.worker_id!==current.worker_id||value.profile.profile_sha256!==current.profile.profile_sha256)return;
    const scope={project,toolID:value.id,userID:session.user.id,organizationID:session.organization.id};
    if(!locator||hasToolInspectionLocalOperation(scope,toolSessionStorage())){action.clearError();setInspectionLocked(false);setDialog({kind:'inspection',profile:value});return;}
    if(!validToolInspectionLocator(locator)){setConnectError(new UiError('tool-reinspection.invalid_read'));return;}
    const nonce=++connectIntent.current,basis=detailBasis,controller=new AbortController();inspectionLookup.current?.abort();inspectionLookup.current=controller;setConnecting(true);setConnectError(undefined);
    const owns=()=>activeWorkspace.current&&!controller.signal.aborted&&nonce===connectIntent.current&&latestDetailBasis.current===basis&&pairingCaller.current.api===api&&pairingCaller.current.generation===pairingGeneration;
    try{
      const raw=await api<unknown>(toolInspectionPath(scope,locator.inspectionID),undefined,'GET',undefined,controller.signal);if(!owns())return;
      const inspection=readToolInspectionTarget(raw,scope,locator);if(inspection.worker_id!==value.worker_id)throw new UiError('tool-reinspection.invalid_read');
      const {report}=readToolProfileDiagnostics(await api<unknown>('/tool-profiles/'+encodeURIComponent(inspection.tool_profile_id)+'/diagnostics'+query(project),undefined,'GET',undefined,controller.signal),{project,toolID:inspection.tool_profile_id});if(!owns())return;
      if(report.tool.worker_id!==inspection.worker_id)throw new UiError('tool-reinspection.invalid_read');
      action.clearError();setInspectionLocked(false);
      // A journal appearing during the read still owns this tab's original target.
      if(hasToolInspectionLocalOperation(scope,toolSessionStorage())){setDialog({kind:'inspection',profile:value});return;}
      if(report.tool.id!==selected)onSelect(report.tool.id);
      setDialog({kind:'inspection',profile:report.tool,locator:{...locator}});
    }catch(error){if(owns())setConnectError(error);}
    finally{if(inspectionLookup.current===controller)inspectionLookup.current=undefined;if(activeWorkspace.current&&pairingCaller.current.api===api&&pairingCaller.current.generation===pairingGeneration){setConnecting(false);}}
  }
  function inspectionWorkerRow(workerID:string){
    if(!workers.loading&&!workers.error&&workers.data?.items.some(worker=>worker.worker_id===workerID))return workerRows.current.get(workerID);
    if(!sharedWorkers.loading&&!sharedWorkers.error&&sharedWorkers.data?.items.some(worker=>worker.worker_id===workerID))return sharedWorkerRows.current.get(workerID);
  }
  function locateInspectionWorker(workerID:string){
    if(pairingCaller.current.api!==api||pairingCaller.current.generation!==pairingGeneration||dialog?.kind!=='inspection'||dialog.profile.project_id!==project||workerID!==dialog.profile.worker_id||pending||action.busy||pairingLocked)return;
    const row=inspectionWorkerRow(workerID);if(!row)return;
    const identity=workerIdentityRecords.find(worker=>worker.worker_id===workerID);
    if(identity&&!matchingWorkerIdentityIDs.has(identity.id)){
      setWorkerIdentitySelection({caller:api,generation:pairingGeneration,filter:'all',query:''});
    }
    setDialog(undefined);setInspectionLocked(false);
    requestAnimationFrame(()=>{if(pairingCaller.current.api!==api||pairingCaller.current.generation!==pairingGeneration||!row.isConnected||row.hidden)return;row.scrollIntoView?.({block:'nearest'});row.focus({preventScroll:true});});
  }
  function openWorkerRegistration() {
    if(writeBlocked||workerManagement!==true)return;
    action.clearError();
    setDialog({kind:'worker',mode:'manual'});
  }
  function openRunnerPairing() {
    if(writeBlocked)return;
    action.clearError();
    setDialog({kind:'worker',mode:'pairing'});
  }
  function continueWithPairedRunner(value:RunnerPairing) {
    if(writeBlocked||pairingCaller.current.api!==api||pairingCaller.current.generation!==pairingGeneration||value.project_id!==project||value.state!=='approved'||!value.worker_id)return;
    try{readRunnerPairing(value,project);}catch{return;}
    setPairedSelection({caller:api,generation:pairingGeneration,context:contextKey,value});
    setReceipt(undefined);
    beginPublicDraft({...publicDraft??{adapterID:'',commandID:'',name:'',nameEdited:false},workerID:value.worker_id},replacementSource);
    void workers.read();void sharedWorkers.read();
    setDialog({kind:'quick',preferredWorkerID:value.worker_id});
  }
  function openToolPreparation(adapter:AdapterDescriptor|undefined,expectedWorkerID?:string,focusRemote=false,discovery=false) {
    const target=toolPreparationTarget(adapter);if(!target)return false;
    const owner=connectDraft.current,latestDraft=owner?.caller===api&&owner.generation===pairingGeneration&&owner.context===contextKey?owner.value:undefined;
    const sameTarget=latestDraft?.adapterID===adapter!.id&&latestDraft.workerID===expectedWorkerID;
    if(expectedWorkerID&&!sameTarget)beginPublicDraft({adapterID:adapter!.id,workerID:expectedWorkerID,commandID:'',name:'',nameEdited:false});
    setDialog(undefined);setConnectionSelection({context:contextKey,caller:api,generation:pairingGeneration,nonce:++connectionRequestNonce.current,expectedWorkerID,preset:target.preset,...(owner?.sourceTool?{sourceEpoch:owner.sourceEpoch}:{})});setGuideOpen(true);
    focusNavigation(()=>document.getElementById(guideID)?.querySelector<HTMLElement>(expectedWorkerID?.startsWith('pool-')?'[data-shared-pool-prepare]':discovery?'[data-remote-runner-allow-installed-discovery]':focusRemote?'[data-remote-runner-setup]>summary':'summary'));return true;
  }
  function openWorkerQuickConnect(worker:ExecutionWorker) {
    if(writeBlocked || workers.loading || workers.error || !activeWorkspace.current) return;
    if(pairingCaller.current.api!==api || pairingCaller.current.generation!==pairingGeneration) return;
    const now=Date.now();
    if(classifyToolWorkerIdentity(worker,now)!=='current'){
      setWorkerIdentityClock(now);
      return;
    }
    if(!canLeave())return;
    const sameWorker=publicDraft?.workerID===worker.worker_id;
    beginPublicDraft(
      sameWorker?publicDraft:{adapterID:'',workerID:worker.worker_id,commandID:'',name:'',nameEdited:false},
      sameWorker?replacementSource:undefined
    );
    action.clearError();
    setDialog({kind:'quick',preferredWorkerID:worker.worker_id});
  }
  function openRemoteScan(workerID:string){
    if(writeBlocked||!activeWorkspace.current||pairingCaller.current.api!==api||pairingCaller.current.generation!==pairingGeneration)return;
    action.clearError();setDialog({kind:'remote-scan',workerID,context:contextKey,caller:api,generation:pairingGeneration});
  }
  function preparePairedRunner(value:RunnerPairing,adapter:AdapterDescriptor) {
    if(writeBlocked||pairingCaller.current.api!==api||pairingCaller.current.generation!==pairingGeneration||pairedRunner!==value||value.project_id!==project||value.state!=='approved'||!value.worker_id)return;
    try{readRunnerPairing(value,project);}catch{return;}
    openToolPreparation(adapter,value.worker_id,true);
  }
  function openGuide() {
    setConnectionSelection(undefined);
    setGuideOpen(true);
    focusNavigation(()=>document.getElementById(guideID)?.querySelector('summary'));
  }
  function setupConnection(descriptor:AdapterDescriptor){
    if(writeBlocked||!canLeave()||descriptor.implementation==='planned')return;
    if(descriptor.id==='workbuddy-cloud'&&descriptor.direction==='suma_to_tool'&&descriptor.entrypoint_kind==='cloud_api'&&descriptor.implementation==='implemented'&&descriptor.setup_kind==='manual_guide'&&!descriptor.adapter&&!descriptor.protocol){setConnectionSelection({context:contextKey,caller:api,generation:pairingGeneration,nonce:++connectionRequestNonce.current,cloud:true});focusNavigation(()=>workspaceElement.current?.querySelector<HTMLElement>('.workbuddy-disclosure'),false);return;}
    if(descriptor.direction==='suma_to_tool'&&descriptor.setup_kind==='runner_profile'&&descriptor.implementation==='implemented'&&descriptor.adapter&&descriptor.protocol){
      beginPublicDraft(publicDraft?.adapterID===descriptor.id?publicDraft:{adapterID:descriptor.id,workerID:'',commandID:'',name:'',nameEdited:false},publicDraft?.adapterID===descriptor.id?replacementSource:undefined);
      action.clearError();void workers.read();void sharedWorkers.read();setDialog({kind:'quick',adapter:descriptor});return;
    }
    const clients:Record<string,DeveloperClientID>={'cursor-client':'cursor','claude-code-client':'claude-code','codex-client':'codex','opencode-client':'opencode','qoder-client':'qoder','zed-client':'zed','workbuddy-connector':'workbuddy'};
    if(descriptor.direction==='tool_to_suma'&&clients[descriptor.id]){setConnectionSelection({context:contextKey,caller:api,generation:pairingGeneration,nonce:++connectionRequestNonce.current,client:clients[descriptor.id]});setGuideOpen(true);focusNavigation(()=>workspaceElement.current?.querySelector<HTMLElement>('[data-tool-workspace-client]'));return;}
    if(descriptor.id==='harness-guide'&&descriptor.setup_kind==='manual_guide'){setConnectionSelection({context:contextKey,caller:api,generation:pairingGeneration,nonce:++connectionRequestNonce.current,preset:'harness'});setGuideOpen(true);focusNavigation(()=>document.getElementById(guideID)?.querySelector('summary'));}
  }
  function replaced(value:ToolProfileReplacement){
    if(!activeWorkspace.current||pairingCaller.current.api!==api||pairingCaller.current.generation!==pairingGeneration||value.project_id!==project)return;
    setDialog(undefined);setReceipt(undefined);setReplacementResult({caller:api,generation:pairingGeneration,context:contextKey,value});connectDraft.current=undefined;renderDraft(value=>value+1);setRevision(value=>value+1);onSelect(value.replacement.id);
  }
  async function replaceProfile(draft:ToolProfileReplacementDraft){
    const owner=connectDraft.current,original=owner?.sourceTool;
    if(writeBlocked||!replacementSourceCurrent||!owner||!original||owner.caller!==api||owner.generation!==pairingGeneration||owner.context!==contextKey||pairingCaller.current.api!==api||pairingCaller.current.generation!==pairingGeneration||draft.original.id!==original.id||draft.original.version!==original.version||draft.original.worker_id!==original.worker_id||!sameToolProfile(draft.original.profile,original.profile))return;
    try{const body=buildToolProfileReplacementBody(draft),value=await action.run<ToolProfileReplacement>(toolProfileReplacementPath(original.id),body);if(value)replaced(value);}catch(error){if(activeWorkspace.current&&pairingCaller.current.api===api&&pairingCaller.current.generation===pairingGeneration)action.setError(error);}
  }
  async function reviewReplacementTarget(profile:ToolProfile){
    if(writeBlocked||!activeWorkspace.current||profile.project_id!==project||pairingCaller.current.api!==api||pairingCaller.current.generation!==pairingGeneration)return;
    const nonce=++connectIntent.current,selection=selected,controller=new AbortController();replacementLookup.current?.abort();replacementLookup.current=controller;setConnecting(true);setConnectError(undefined);
    const owns=()=>activeWorkspace.current&&!controller.signal.aborted&&nonce===connectIntent.current&&selectedProfile.current===selection&&pairingCaller.current.api===api&&pairingCaller.current.generation===pairingGeneration;
    try{
      const {report}=readToolProfileDiagnostics(await api<unknown>('/tool-profiles/'+encodeURIComponent(profile.id)+'/diagnostics'+query(project),undefined,'GET',undefined,controller.signal),{project,toolID:profile.id});if(!owns())return;
      if(report.tool.worker_id!==profile.worker_id||!sameToolProfile(report.tool.profile,profile.profile))throw new UiError('tool-profile-replacement.invalid_receipt');
      if(selected!==report.tool.id)onSelect(report.tool.id);else void detail.read();
      action.clearError();if(report.tool.state==='paused')setDialog({kind:'decision',action:'enable',profile:report.tool});
    }catch(error){if(owns())setConnectError(error);}
    finally{if(replacementLookup.current===controller)replacementLookup.current=undefined;if(activeWorkspace.current&&pairingCaller.current.api===api&&pairingCaller.current.generation===pairingGeneration)setConnecting(false);}
  }
  function saved(path: string, value: unknown) {
    if(!activeWorkspace.current||pairingCaller.current.api!==api||pairingCaller.current.generation!==pairingGeneration)return;
    if(manualWorkerRotationID(path)){
      const result=value as ManualWorkerRotationReceipt; // ToolWriteRecovery has correlated the complete receipt.
      setDialog(undefined);
      setRevision(before=>before+1);
      if(workerIdentityPreferences.filter==='attention'){
        setWorkerIdentitySelection({caller:api,generation:pairingGeneration,filter:'current',query:workerIdentityPreferences.query});
      }
      setReceipt({key:'worker_rotated',name:result.worker.worker_id,scope:contextKey});
      setUploadReceipt(undefined);
      setRevealed(false);
      setCopyState(undefined);
      setCredential({caller:api,generation:pairingGeneration,worker_id:result.worker.worker_id,token:result.token,rotation:true});
      return;
    }
    if(isToolProfileReplacementPath(path)){replaced(readToolProfileReplacement(value,{project,originalID:toolProfileReplacementOriginalID(path)}));return;}
    setDialog(undefined);
    setRevision((before) => before + 1);
    if (!value || typeof value !== "object") return;
    const result = value as Record<string, unknown>;
    if(path==='/tool-profiles')setReceipt({key:'registered',name:String(result.name||''),scope:contextKey});
    else if(path.startsWith('/tool-profiles/')&&['enabled','paused','revoked'].includes(String(result.state)))setReceipt({key:String(result.state),name:String(result.name||''),scope:contextKey});
    else if(path.startsWith('/release-workers/')){
      setReceipt({key:'worker_revoked',name:String(result.worker_id||''),scope:contextKey});
      setWorkerMaintenanceOpen(undefined);
      focusNavigation(()=>document.getElementById(guideID+'-worker-identities'),false);
    }
    if (path === "/release-workers") {
      setRevealed(false);
      setCopyState(undefined);
      setCredential({
        caller:api,
        generation:pairingGeneration,
        worker_id: String(result.worker_id || ""),
        token: typeof result.token === "string" ? result.token : undefined,
      });
    } else if (
      path.startsWith("/tool-profiles") &&
      typeof result.id === "string"
    )
      onSelect(result.id);
  }
  const returnRun = toolReturnRun(panel);
  return (
    <div className="tool-workspace" ref={workspaceElement}>
      <div className="tool-intro">
        <div>
          <h2>{t("development.tools.execution_tools")}</h2>
          <p>{t("development.tools.review_local_permitted_configurations_live_observations_and")}</p>
        </div>
        <a
          href={
            "?" +
            new URLSearchParams({ project, view: "development", sub: "tasks" })
          }
        >
          {t("development.tools.return_to_task_dispatch")}</a>
      </div>
      <div className="tool-workspace-primary-actions" data-tool-workspace-primary-actions>
        <button type="button" className="primary" data-tools-add disabled={writeBlocked} onClick={openQuickConnect}><Plus size={16} aria-hidden="true"/>{t('tool-workspace-experience.add')}</button>
        <details className="tool-quick-advanced"><summary>{t('tool-quick-connect.advanced')}</summary><button type="button" data-tools-advanced-register disabled={writeBlocked} onClick={openRegister}>{t('development.tools.register_configuration')}</button></details>
      </div>
      <ToolInventoryConnectLink
        project={project}
        panel={panel}
        disabled={writeBlocked||!!dialog||!!taskHandoff}
        onDismiss={()=>{if(!writeBlocked&&canLeave())onSelect('');}}
        onReview={({adapter,draft})=>{
          if(!activeWorkspace.current||currentDialog.current||writeBlocked||taskHandoff||!canLeave())return;
          beginPublicDraft(draft);
          action.clearError();
          void workers.read();
          void sharedWorkers.read();
          setDialog({kind:'quick',adapter,preferredWorkerID:draft.workerID});
        }}
      />
      <ToolReadiness items={profiles.items} total={profiles.total} uncertain={uncertain} identitiesKnown={identitiesKnown} identityCount={identityCount} disabled={writeBlocked} filtered={hasToolFilters} onClearFilters={clearToolFilters} onWorker={workerManagement===true?openWorkerRegistration:undefined} onRegister={openQuickConnect} onGuide={openGuide} onSelect={id => { if(canLeave()) onSelect(id); }}/>
      {returnRun && (
        <a
          onClick={(event) => {
            if (!canLeave()) event.preventDefault();
          }}
          href={
            "?" +
            new URLSearchParams({
              project,
              view: "development",
              sub: "runs",
              run: returnRun.run,
              node: returnRun.node,
            })
          }
        >
          {t("development.tools.return_to_the_original_run_and_selected")}</a>
      )}
      <ToolRecoveryNotice action={action} onRestored={saved} />
      {(!rotationWorker||credentialDialogDenied)&&<>
        {(uploadAction.pending||uploadAction.conflict)&&<WorkerUploadRecoveryNotice action={uploadAction} disabled={action.busy||!workerManagementReady} onRestored={setUploadReceipt}/>}
        {uploadReceipt&&!workerManagementDenied&&<WorkerUploadRecoveryReceipt receipt={uploadReceipt}/>}
      </>}
      {replacementReceipt&&<ToolProfileReplacementReceipt value={replacementReceipt} disabled={writeBlocked} onViewOriginal={profile=>{if(!writeBlocked&&canLeave())onSelect(profile.id);}} onViewReplacement={profile=>{if(!writeBlocked&&canLeave())onSelect(profile.id);}} onEnableReplacement={profile=>void reviewReplacementTarget(profile)}/>}
      {!!connectError&&<ErrorNotice error={renderDiagnostic(connectError)}/>}
      {receipt?.scope===contextKey&&<div className="tool-save-receipt" data-tool-receipt role="status"><CheckCircle2 size={18} aria-hidden="true"/><p>{t(receipt.key==='worker_rotated'?'worker-rotation.rotated':'tool-workspace.receipt.'+receipt.key,{name:receipt.name})}</p><button className="icon-button" aria-label={t('tool-workspace.dismiss_receipt')} onClick={()=>setReceipt(undefined)}><X size={16}/></button></div>}
      <div className="tool-refresh-bar">
        <small>{live ? t("development.tools.auto_refresh") : t("development.tools.auto_refresh_paused")}</small>
        <div className="inline">
          <button className="text-button" onClick={() => setLive(value=>!value)}>{live?<Pause size={13}/>:<Play size={13}/>} {t(live?'tool-workspace.pause_refresh':'tool-workspace.resume_refresh')}</button>
          <button aria-label={t("development.tools.refresh_tool_configurations")} disabled={profiles.loading} onClick={refreshAll}><RefreshCw size={14}/>{t('tool-workspace.refresh')}</button>
        </div>
      </div>
      <div className={"tool-columns " + (selected ? "with-detail" : "")}>
        <section className="panel tool-list" data-tool-workspace-configurations>
          <div className="list-toolbar">
            <h3 className="tool-profiles-heading" data-tool-profiles-heading ref={listHeading} tabIndex={-1}>
              {t("development.tools.fixed_tool_configuration")}<b>{profiles.items ? formatNumber(profiles.total) : "—"}</b>
            </h3>
          </div>
          {filters.worker&&<div className="tool-profile-worker-filter" data-tool-profile-worker-filter={filters.worker}>
            <p role="status"><Terminal size={16} aria-hidden="true"/><span>{t('tool-workspace.worker_filter')} <strong>{filters.worker}</strong></span></p>
            <button type="button" className="text-button" data-tool-profile-worker-filter-clear disabled={writeBlocked} onClick={clearWorkerFilter}><X size={16} aria-hidden="true"/>{t('tool-workspace.clear_worker_filter')}</button>
          </div>}
          <div className="tool-filter-tabs" role="group" aria-label={t('tool-workspace.filter')}>
            {toolListStates.map(value => <button key={value} type="button" data-tools-state={value} aria-pressed={filters.state===value} onClick={() => applyFilters({...filters,state:value})}>{t('tool-workspace.state.'+value)}</button>)}
          </div>
          <form className="filter-toolbar tool-search-form" data-tools-search onSubmit={event=>{event.preventDefault();applyFilters({...filters,query:search});}}>
            <Search size={16} aria-hidden="true"/>
            <input
              data-tools-query
              aria-label={t('tool-workspace.search')}
              aria-invalid={searchError}
              placeholder={t("development.tools.filter_by_name_worker_or_command")}
              value={search}
              onChange={(event) => setSearch(event.target.value)}
            />
            <button type="submit">{t('tool-workspace.search')}</button>
            <small aria-live="polite">
              {t("development.tools.loaded_count",{loaded:formatNumber(profiles.items?.length||0),total:formatNumber(profiles.total)})}
            </small>
          </form>
          {searchError&&<p className="warning" role="alert">{t('tool-workspace.query_invalid')}</p>}
          <div className="tool-window-filter">
            <Field label={t('tool-workspace.window_filter')}><select data-tools-window-filter value={filter} onChange={event=>setFilter(event.target.value as ToolFilter)}>{(['all','available','attention'] as const).map(value=><option key={value} value={value}>{t('tool-workspace.filter.'+value)}</option>)}</select></Field>
          </div>
          {profiles.error && (
            <ErrorNotice
              error={profiles.error}
              onRetry={() =>
                void profiles.read(profiles.error_action === "more")
              }
            />
          )}{" "}
          {!profiles.items ? (
            profiles.loading ? (
              <Loading />
            ) : (
              <Empty
                title={t("development.tools.no_official_configurations_read_yet")}
                description={t("development.tools.restore_the_connection_first_historical_declarations_cannot")}
              />
            )
          ) : filtered?.length ? (
            <div>
              {filtered.map((profile) => (
                <button
                  className={
                    "tool-resource " +
                    (profile.id === selected ? "selected" : "")
                  }
                  key={profile.id}
                  onClick={() => {
                    if (canLeave()) onSelect(profile.id);
                  }}
                >
                  <span>
                    <strong>{profile.name}</strong>
                    <small>
                      {profile.profile.adapter} · {profile.worker_id} · {profile.profile.command_id}
                    </small>
                    {profile.reason && <small className="tool-resource-reason">{toolReason(profile.reason, profile.reason_diagnostic)}</small>}
                  </span>
                  <span className={'tool-status-pill '+(!uncertain&&profile.state==='enabled'&&profile.available?'available':profile.state)}>{uncertain?t('tool-workspace.unconfirmed'):profile.state==='enabled'&&profile.available?t('tool-workspace.eligible'):profile.state==='enabled'?t('tool-workspace.filter.attention'):toolStateNames[profile.state]}</span>
                  <ChevronRight size={15} />
                </button>
              ))}
            </div>
          ) : (
            <Empty
              title={hasToolFilters || filter!=='all' ? t('tool-workspace.no_matches') : t("development.tools.no_execution_tools_registered")}
              description={
                hasToolFilters || filter!=='all'
                  ? t('tool-workspace.no_matches_hint')
                  : t("development.tools.register_an_execution_worker_identity_then_choose")
              }
              action={hasToolFilters || filter!=='all'?<button disabled={!!filters.worker&&writeBlocked} onClick={clearToolFilters}>{t('tool-workspace.clear_filters')}</button>:<button onClick={openGuide}><CircleHelp size={14}/>{t('tool-workspace.setup_guide')}</button>}
            />
          )}
          {profiles.next_cursor && <div className="list-footer">
              <button
                disabled={profiles.loading}
                onClick={() => {
                  setLive(false);
                  void profiles.read(true);
                }}
              >
                {t("development.tools.load_more_configurations")}</button>
          </div>}
        </section>
        {selected && (
          <section className="panel tool-detail">
            <header className="detail-top">
              <button
                className="text-button"
                onClick={() => {
                  if (canLeave()) onSelect("");
                }}
              >
                {t("development.tools.return_to_configuration_list")}</button>
              <button
                className="icon-button"
                aria-label={t("development.tools.close_tool_details")}
                onClick={() => {
                  if (canLeave()) onSelect("");
                }}
              >
                <X size={17} />
              </button>
            </header>
            <div className="tool-detail-content">
              {detail.error && (
                <ErrorNotice
                  error={detail.error}
                  onRetry={() => void detail.read()}
                />
              )}{" "}
              {detail.loading && <p role="status">{t("development.tools.reading_current_dispatch_conditions")}</p>}
              {current && (
                <>
                  {stale && (
                    <p className="warning">
                      {t("development.tools.the_last_read_is_retained_current_connection")}</p>
                  )}
                  <ToolProfileFacts key={current.id} profile={current} confirmed={!stale}>
                  <div className="tool-action-bar" data-tool-profile-actions>
                    {handoffPin&&<button type="button" data-tool-use-for-task disabled={stale||writeBlocked} onClick={()=>{
                      if(!current||!handoffPin||stale||writeBlocked||!canLeave())return;
                      action.clearError();setDialog(undefined);
                      setTaskHandoff({epoch:handoffOwner.current.epoch,intent:++handoffIntent.current,tool:current});
                    }}>{t('tool-task-handoff.use')}</button>}
                    {current.state !== "revoked" && (
                      <>
                        <button type="button" data-tools-adjust disabled={stale||writeBlocked} onClick={()=>void configureProfile(current)}>{t('tool-workspace.adjust')}</button>
                        {current.state === "paused" ? (
                          <button
                            className="primary"
                            disabled={stale || writeBlocked}
                            onClick={() => {
                              action.clearError();
                              setDialog({
                                kind: "decision",
                                action: "enable",
                                profile: current,
                              });
                            }}
                          >
                            {t("development.tools.review_and_enable")}</button>
                        ) : (
                          <button
                            disabled={stale || writeBlocked}
                            onClick={() => {
                              action.clearError();
                              setDialog({
                                kind: "decision",
                                action: "pause",
                                profile: current,
                              });
                            }}
                          >
                            {t('tool-workspace.pause')}</button>
                        )}
                        <button
                          disabled={stale || writeBlocked}
                          onClick={() => {
                            action.clearError();
                            setDialog({
                              kind: "decision",
                              action: "revoke",
                              profile: current,
                            });
                          }}
                        >
                          {t('tool-workspace.remove')}</button>
                      </>
                    )}
                  </div>
                  <ToolProfileDoctor project={project} tool={current} confirmed={!stale} autoRefresh={live&&!(dialog?.kind==="decision"&&dialog.action!=="enable")} pending={pending} disabled={writeBlocked}
                    onReview={value=>{if(stale||writeBlocked||current.state==='revoked'||value.id!==current.id||value.project_id!==project||value.version!==current.version||value.worker_id!==current.worker_id)return;action.clearError();setDialog({kind:'decision',action:'enable',profile:current});}}
                    onRegister={value=>{if(!stale&&!writeBlocked)void configureProfile(value);}}
                    onInspect={inspectProfile}
                    onRefreshTool={()=>{if(!writeBlocked)void detail.read();}}
                    onWorker={!workers.loading&&!workers.error&&workers.data?.items.some(worker=>worker.worker_id===current.worker_id)?workerID=>{if(stale||writeBlocked||workerID!==current.worker_id||workers.loading||workers.error||!workers.data?.items.some(worker=>worker.worker_id===workerID))return;const row=workerRows.current.get(workerID);row?.scrollIntoView?.({block:'nearest'});row?.focus({preventScroll:true});void configureProfile(current,'worker');}:undefined}
                    onSharedWorker={!sharedWorkers.loading&&!sharedWorkers.error&&sharedWorkers.data?.items.some(worker=>worker.worker_id===current.worker_id)?workerID=>{if(stale||writeBlocked||workerID!==current.worker_id||sharedWorkers.loading||sharedWorkers.error||!sharedWorkers.data?.items.some(worker=>worker.worker_id===workerID))return;const row=sharedWorkerRows.current.get(workerID);row?.scrollIntoView?.({block:'nearest'});row?.focus({preventScroll:true});void configureProfile(current,'shared-worker');}:undefined}
                    onRun={(runID,nodeID)=>{if(!stale&&!writeBlocked&&canLeave())window.location.assign('?'+new URLSearchParams({project,view:'development',sub:'runs',run:runID,node:nodeID}));}}
                  />
                  <ToolProfileReplacementHistory project={project} profileID={current.id} disabled={writeBlocked||stale} accessBlocked={detail.failure instanceof ApiError&&[401,403,404].includes(detail.failure.status)} refresh={refresh+revision} onViewProfile={profile=>{if(!stale&&!writeBlocked&&canLeave())onSelect(profile.id);}} onAccessDenied={()=>{if(pairingCaller.current.api===api&&pairingCaller.current.generation===pairingGeneration)void detail.read();}}/>
                  </ToolProfileFacts>
                </>
              )}
            </div>
          </section>
        )}
      </div>
      <div className="tool-boundary-note"><ShieldCheck size={16}/><p>{t("development.tools.configuration_probing_actual_execution_and_acceptance_are")}</p></div>
      <section className="panel tool-workers tool-worker-identities" aria-labelledby={guideID+'-worker-identities'}>
        <div className="tool-section-heading">
          <div>
            <h3 id={guideID+'-worker-identities'} tabIndex={-1}>{t("development.tools.execution_worker_identity")}</h3>
            <p className="muted">{t('tool-worker-identities.boundary')}</p>
            {workerManagement===false&&<p className="tool-worker-identity-access" role="status">{t('tool-worker-identities.read_only')}</p>}
          </div>
          {workerManagement===true&&<button type="button" data-tool-worker-register
            disabled={writeBlocked||!workerManagementReady} onClick={openWorkerRegistration}>
            <Plus size={16} aria-hidden="true"/>
            {t("development.tools.register_execution_worker")}</button>}
        </div>
        <div className="tool-worker-identity-toolbar">
          <div className="tool-worker-identity-filters" role="group" aria-label={t('tool-worker-identities.views')}>
            {(['current','attention','revoked','all'] as const).map(value=><button type="button" key={value} data-tool-worker-filter={value} aria-pressed={workerIdentityPreferences.filter===value} disabled={writeBlocked} onClick={()=>updateWorkerIdentityView(value)}><span>{t('tool-worker-identities.filter.'+value)}</span><span className="tool-worker-identity-count">{formatNumber(workerIdentityView.counts[value])}</span></button>)}
          </div>
          <label className="tool-worker-identity-search"><span>{t('tool-worker-identities.search')}</span><div><Search size={16} aria-hidden="true"/><input type="search" data-tool-worker-search value={workerIdentityPreferences.query} maxLength={120} disabled={writeBlocked} placeholder={t('tool-worker-identities.search_hint')} onChange={event=>updateWorkerIdentityView(workerIdentityPreferences.filter,event.target.value)}/></div></label>
        </div>
        {workers.data&&<p className="tool-worker-identity-summary" data-tool-worker-counts role="status">{t('tool-worker-identities.read_scope',{read:workerIdentityView.readCount,matched:workerIdentityView.counts.all,shown:workerIdentityView.items.length})}{retainedWorkerIdentities.length>0&&<span>{t('tool-worker-identities.retained_count',{count:retainedWorkerIdentities.length})}</span>}</p>}
        {workers.loading&&workers.data&&<p className="muted">{t('tool-worker-identities.checking')}</p>}
        {workers.error && (
          <ErrorNotice
            error={workers.error}
            onRetry={() => void workers.read()}
          />
        )}{" "}
        {!workers.data ? (
          workers.loading ? (
            <Loading />
          ) : null
        ) : workers.data.items.length ? (
          <div className="tool-worker-identity-rows">{workerIdentityRecords.map((worker) => {
            const identityState=classifyToolWorkerIdentity(worker,workerIdentityClock),retained=retainedWorkerIdentityIDs.has(worker.id);
            const canMaintain=workerManagement===true&&worker.project_id===project&&worker.status==='active';
            const maintenanceOpen=canMaintain&&workerMaintenanceOpen?.caller===api&&workerMaintenanceOpen.id===worker.id;
            const maintenanceID=guideID+'-worker-maintenance-'+encodeURIComponent(worker.id);
            const canRotate=worker.manual_rotation_allowed===true&&(identityState==='current'||identityState==='expired');
            return <article className={"tool-worker-row tool-worker-identity-row"+(workerRecoverySource(worker.worker_id)?" with-recovery":"")+(retained?" is-context-target":"")} key={worker.id} hidden={!matchingWorkerIdentityIDs.has(worker.id)&&!retained} tabIndex={-1} data-tool-worker={worker.worker_id} data-tool-worker-credential-state={identityState} data-tool-worker-context={retained?'':undefined} ref={element=>{if(element)workerRows.current.set(worker.worker_id,element);else workerRows.current.delete(worker.worker_id);}}>
              <div className="tool-worker-identity-facts">
                <div className="tool-worker-identity-title"><strong>
                  <Terminal size={14} /> {worker.worker_id}
                </strong><span className={'tool-worker-credential-state is-'+identityState}>{t('tool-worker-identities.state.'+identityState)}</span></div>
                {retained&&<small className="tool-worker-identity-context">{t('tool-worker-identities.retained_target')}</small>}
                <small>
                  {identityState==='unknown'||!Number.isFinite(Date.parse(worker.expires_at))?t('tool-worker-identities.expiry_unknown'):t('tool-worker-identities.expires_at',{at:formatDateTime(worker.expires_at,{dateStyle:'short',timeStyle:'medium'})})}
                </small>
                <small>
                  {worker.deployment ? t("development.tools.also_permitted_to_release") : t("development.tools.no_deployment_permission")}
                </small>
                {identityState!=='current'&&<small>{t('tool-worker-identities.hint.'+identityState)}</small>}
              </div>
              <div className="tool-worker-identity-actions">
              {worker.project_id===project&&validToolListWorker(worker.worker_id)&&<button type="button" className="text-button tool-node-configurations" data-tool-worker-configurations={worker.worker_id} disabled={writeBlocked||workers.loading||!!workers.error} onClick={()=>showWorkerProfiles(worker.worker_id)}>{t('tool-workspace.view_worker_profiles')}</button>}
              {identityState==='current'&&<button type="button" data-tool-worker-connect={worker.worker_id} disabled={writeBlocked||workers.loading||!!workers.error} onClick={()=>openWorkerQuickConnect(worker)}>{t('tool-worker-identities.configure_tools')}</button>}
              {identityState==='current'&&<button type="button" data-tool-worker-scan={worker.worker_id} disabled={writeBlocked||workers.loading||!!workers.error} onClick={()=>{const now=Date.now();if(writeBlocked||workers.loading||workers.error)return;if(classifyToolWorkerIdentity(worker,now)!=='current'){setWorkerIdentityClock(now);return;}openRemoteScan(worker.worker_id);}}>{t('remote-tool-scan.open')}</button>}
              {canMaintain&&<button type="button" className="tool-worker-maintenance-toggle" data-tool-worker-maintenance={worker.worker_id}
                aria-expanded={maintenanceOpen} aria-controls={maintenanceID} disabled={writeBlocked||!workerManagementReady}
                onClick={()=>setWorkerMaintenanceOpen(maintenanceOpen?undefined:{caller:api,id:worker.id})}>
                {t('tool-worker-identities.maintenance')}<ChevronDown size={16} aria-hidden="true"/>
              </button>}
              </div>
              {canMaintain&&<div id={maintenanceID} className="tool-worker-maintenance" hidden={!maintenanceOpen}>
                {maintenanceOpen&&<>
                  <p>{t('tool-worker-identities.maintenance_hint')}</p>
                  <div className="tool-worker-maintenance-actions">
                    {canRotate&&<button type="button" data-tool-worker-rotate={worker.worker_id}
                      disabled={writeBlocked||!workerManagementReady} onClick={()=>{
                        if(writeBlocked||!workerManagementReady)return;
                        action.clearError();
                        setDialog({kind:'rotate-worker',worker});
                      }}>{t('worker-rotation.open')}</button>}
                    <button type="button" className="danger-quiet" data-tool-worker-revoke={worker.worker_id}
                      disabled={writeBlocked||!workerManagementReady} onClick={()=>{
                        if(writeBlocked||!workerManagementReady)return;
                        action.clearError();
                        setDialog({kind:'revoke-worker',worker});
                      }}>{t('development.tools.revoke_worker_credential')}</button>
                  </div>
                </>}
              </div>}
              {identityState==='current'&&workerConnectionPreparation(worker.worker_id)}
            </article>;
          })}{!workerIdentityView.items.length&&!retainedWorkerIdentities.length&&<div className="tool-worker-identity-empty"><p>{t('tool-worker-identities.empty_view')}</p><button type="button" data-tool-worker-show-all disabled={writeBlocked} onClick={()=>updateWorkerIdentityView('all','')}>{t('tool-worker-identities.show_all')}</button></div>}</div>
        ) : (
          <Empty
            title={t("development.tools.no_bound_execution_identity")}
            description={t("development.tools.registration_issues_a_one_time_project_scoped")}
          />
        )}
        {!sharedWorkers.loading&&!sharedWorkers.error&&!!sharedWorkers.data?.items.length&&<div className="tool-shared-workers">
          <h4>{t('tool-workspace.shared_nodes')}</h4><p className="muted">{t('tool-workspace.shared_nodes_hint')}</p>
          {sharedWorkers.data.items.map(worker=><article key={worker.id} className={"tool-worker-row"+(sharedRecoveryPin(worker.worker_id)?" with-recovery":"")} tabIndex={-1} data-tool-shared-worker={worker.worker_id} ref={element=>{if(element)sharedWorkerRows.current.set(worker.worker_id,element);else sharedWorkerRows.current.delete(worker.worker_id);}}>
            <div><strong>{worker.name||worker.worker_id}</strong><small>{worker.worker_id} · {worker.pool_id}</small><small>{t('tool-workspace.shared_node_status',{status:worker.status})}</small></div>
            <div className="tool-worker-identity-actions">
              {validToolListWorker(worker.worker_id)&&worker.worker_id==='pool-'+worker.id&&<button type="button" className="text-button tool-node-configurations" data-tool-worker-configurations={worker.worker_id} disabled={writeBlocked} onClick={()=>showWorkerProfiles(worker.worker_id)}>{t('tool-workspace.view_worker_profiles')}</button>}
              <button type="button" data-tool-worker-scan={worker.worker_id} disabled={writeBlocked} onClick={()=>openRemoteScan(worker.worker_id)}>{t('remote-tool-scan.open')}</button><span className="tool-status-pill">{t('tool-workspace.read_only_node')}</span>
            </div>
            {sharedRecoveryEntry(worker.worker_id)}
          </article>)}
        </div>}
      </section>
      <WorkBuddyConnections project={project} panel={panel} openRequest={connection?.cloud?connection.nonce:undefined}/>
      <details className="tool-workspace-more panel" data-tool-workspace-more open={setupDisclosure.open}>
        <summary data-tool-workspace-more-toggle onClick={event=>{event.preventDefault();if(setupDisclosure.open&&!canLeave())return;setSetupDisclosure(before=>({open:!before.open,visited:true}));}}><ChevronRight size={18} aria-hidden="true"/><span><strong>{t('tool-workspace-experience.more')}</strong><small>{t('tool-workspace-experience.more_hint')}</small></span></summary>
        {(setupDisclosure.visited||setupDisclosure.open)&&<div className="tool-workspace-more-body">
      <details className="panel adapter-catalog-disclosure" key={contextKey}>
        <summary>
          <ChevronRight aria-hidden="true" size={20}/>
          <span className="adapter-catalog-disclosure-copy"><strong>{t('adapter-connections.title')}</strong><span>{t('adapter-connections.intro')}</span></span>
          <span className="adapter-catalog-disclosure-action"><span className="when-closed">{t('adapter-connections.open_catalog')}</span><span className="when-open">{t('adapter-connections.close_catalog')}</span></span>
        </summary>
        <AdapterConnections project={project} onSetup={setupConnection}/>
      </details>
      {publicDraft&&guideOpen&&<div className="tool-connect-return"><p>{t('tool-workspace.draft_retained',{worker:publicDraft.workerID||t('tool-workspace.target_not_selected')})}</p><button type="button" data-tools-return-quick disabled={writeBlocked} onClick={openQuickConnect}>{t('tool-workspace.return_connect')}</button></div>}
      {connection?.client?<section className="panel" data-tool-workspace-client tabIndex={-1}><button type="button" onClick={openGuide}>{t('adapter-connections.return_guide')}</button><DeveloperClientConnections key={contextKey} project={project} request={{client:connection.client,nonce:connection.nonce}}/></section>:<ToolSetupGuide onPrepareSharedPool={!sharedWorkers.loading&&!sharedWorkers.error&&sharedWorkers.data?.items.filter(row=>row.status==='active'&&row.worker_id===connection?.expectedWorkerID&&row.worker_id==='pool-'+row.id).length===1?prepareSharedTool:undefined} project={project} pairedRunner={pairedRunner} expectedWorkerID={connection?.expectedWorkerID} id={guideID} expanded={guideOpen} onExpandedChange={setGuideOpen} presetRequest={connection?.preset?{preset:connection.preset,nonce:connection.nonce}:undefined} onPairRunner={openRunnerPairing} onReportSelection={returnFromReport} disabled={writeBlocked}/>}
        </div>}
      </details>
      {handoff&&!handoff.task&&<ToolTaskHandoffDialog key={handoff.intent} session={session} project={project} tool={handoff.tool} disabled={writeBlocked}
        onClose={closeTaskHandoff} onChoose={(task,pin)=>{
          if(!ownsHandoff(handoff)||writeBlocked||task.project_id!==project||pin.project_id!==project)return;
          setTaskHandoff({...handoff,task,pin});
        }}/>}
      {handoff?.task&&handoff.pin&&<Suspense fallback={<Modal open onClose={closeTaskHandoff} title={t('development.dispatch.arrange_task_execution')}><Loading/></Modal>}><TaskDispatchDialog project={project} session={session} task={handoff.task} handoffTool={handoff.pin} planID={handoff.planID}
        onClose={closeTaskHandoff}
        onPlan={(id,taskID)=>{
          if(!ownsHandoff(handoff))return;
          // First retain the confirmed plan locally, then let the original
          // action's React leave guard settle before navigating. Navigating
          // inside its result callback would still observe the prior busy
          // effect and unnecessarily trigger the browser's unload dialog.
          setTaskHandoff({...handoff,planID:id});
          const href='?'+new URLSearchParams({project,view:'development',sub:'tasks',record:taskID||handoff.task!.id,panel:'dispatch:'+id});
          setTimeout(()=>requestAnimationFrame(()=>{
            if(ownsHandoff(handoff)&&canLeave())window.location.assign(href);
          }),0);
        }}
        onRun={run=>{setTimeout(()=>requestAnimationFrame(()=>{if(ownsHandoff(handoff)&&canLeave())window.location.assign('?'+new URLSearchParams({project,view:'development',sub:'runs',run}));}),0);}}/></Suspense>}
      <Modal
        open={!!dialog&&!credentialDialogDenied}
        onClose={() => {
          if (!action.busy && !action.pending && !pairingLocked && !inspectionLocked && !uploadRecoveryLocked) setDialog(undefined);
        }}
        title={
          dialog?.kind === "quick"
            ? t('tool-quick-connect.title')
            : dialog?.kind === 'remote-scan'
              ? t('remote-tool-scan.title')
            : dialog?.kind === 'inspection'
              ? t('tool-doctor.inspect_action')
            : dialog?.kind === "create"
            ? t("development.tools.register_fixed_tool_configuration")
            : dialog?.kind === "worker"
              ? t(dialog.mode==='pairing'?'runner-pairing.title':"development.tools.register_execution_worker_identity")
              : dialog?.kind === "rotate-worker"
                ? t("worker-rotation.title")
              : dialog?.kind === "revoke-worker"
                ? t("development.tools.revoke_execution_worker_credential")
                : dialog?.kind === "decision"
                  ? {
                      enable: t("development.tools.review_and_enable_configuration"),
                      pause: t("development.tools.pause_tool_configuration"),
                      revoke: t('tool-workspace.remove'),
                    }[dialog.action]
                  : t("development.tools.execution_tools")
        }
        description={t(dialog?.kind==='decision'&&dialog.justSaved?'tool-quick-connect.enable_review_description':dialog?.kind==='inspection'?'tool-doctor.inspect_hint':dialog?.kind==='rotate-worker'?'worker-rotation.subtitle':"development.tools.use_the_current_project_s_actual_identity")}
      >
        {action.pending ? (
          <ToolRecoveryNotice action={action} onRestored={saved} />
        ) : (
          action.error && <ErrorNotice error={action.error} />
        )}
        {dialog?.kind==='inspection'&&<ToolReinspectionPanel
          project={project}
          tool={!stale&&current?.id===dialog.profile.id?current:dialog.profile}
          locator={dialog.locator}
          maintenanceReadStartedAt={!stale&&current?.id===dialog.profile.id?detail.readStartedAt:undefined}
          onRefreshTool={()=>{if(pairingCaller.current.api!==api||pairingCaller.current.generation!==pairingGeneration||selected!==dialog.profile.id||detail.loading)return;void detail.read();}}
          onOpenWorker={inspectionWorkerRow(dialog.profile.worker_id)?locateInspectionWorker:undefined}
          disabled={pending||action.busy||pairingLocked||stale||current?.id!==dialog.profile.id}
          accessBlocked={detail.failure instanceof ApiError&&[401,403,404].includes(detail.failure.status)}
          onLockChange={setInspectionLocked}
          onAccessDenied={()=>{if(pairingCaller.current.api===api&&pairingCaller.current.generation===pairingGeneration)void detail.read();}}
          onDefer={()=>{if(pairingCaller.current.api!==api||pairingCaller.current.generation!==pairingGeneration)return;setDialog(undefined);setInspectionLocked(false);}}
          onObserved={()=>{if(pairingCaller.current.api!==api||pairingCaller.current.generation!==pairingGeneration||dialog.profile.project_id!==project)return;const original=dialog.profile.id;setDialog(undefined);setInspectionLocked(false);setRevision(value=>value+1);if(selected!==original)onSelect(original);requestAnimationFrame(()=>document.querySelector<HTMLElement>('.tool-detail .detail-top button')?.focus({preventScroll:true}));}}
        />}
        {dialog?.kind === "create" && (
          <>
          {dialog.resumeQuick&&publicDraft&&<button type="button" className="text-button" data-tools-return-quick disabled={writeBlocked} onClick={openQuickConnect}>{t('tool-workspace.return_connect')}</button>}
          <ToolProfileForm
            key={contextKey+":"+(dialog.adapter?.id||"")}
            project={project}
            adapter={dialog.adapter}
            draft={dialog.resumeQuick?publicDraft:undefined}
            onDraftChange={dialog.resumeQuick?value=>updatePublicDraft(value,draftSource?.nonce):undefined}
            onContinueReplacement={dialog.resumeQuick&&replacementSource?openQuickConnect:undefined}
            workers={workers}
            sharedWorkers={sharedWorkers}
            action={action}
            onSaved={(value) => saved("/tool-profiles", value)}
            onCancel={() => setDialog(undefined)}
          />
          </>
        )}
        {dialog?.kind==='remote-scan'&&dialog.context===contextKey&&dialog.caller===api&&dialog.generation===pairingGeneration&&<RemoteToolScanPanel project={project} workerID={dialog.workerID} disabled={writeBlocked} onPreparedConfiguration={(value,preparationID)=>void returnFromManagedInventory(value,preparationID,dialog)} onPrepare={(adapter,workerID)=>{if(writeBlocked||!activeWorkspace.current||pairingCaller.current.api!==api||pairingCaller.current.generation!==pairingGeneration||workerID!==dialog.workerID)return;openToolPreparation(adapter,workerID,true);}} onGuide={()=>{if(writeBlocked||!activeWorkspace.current||pairingCaller.current.api!==api||pairingCaller.current.generation!==pairingGeneration)return;setDialog(undefined);setConnectionSelection({context:contextKey,caller:api,generation:pairingGeneration,nonce:++connectionRequestNonce.current,expectedWorkerID:dialog.workerID});setGuideOpen(true);focusNavigation(()=>document.getElementById(guideID)?.querySelector<HTMLElement>(dialog.workerID.startsWith('pool-')?'[data-shared-pool-prepare]':'[data-remote-runner-allow-installed-discovery]'));}}/>}
        {dialog?.kind === 'quick' && <ToolQuickConnect
          key={contextKey+':'+(dialog.adapter?.id||'')+':'+(dialog.preferredWorkerID||'')}
          project={project}
          adapter={dialog.adapter}
          draft={publicDraft}
          replacementSource={replacementSource}
          replacementDisabled={!!replacementSource&&!replacementSourceCurrent}
          onReplace={replaceProfile}
          onDraftChange={value=>updatePublicDraft(value,draftSource?.nonce)}
          preferredWorkerID={dialog.preferredWorkerID}
          pairedRunner={pairedRunner}
          onPrepareRunner={preparePairedRunner}
          onRemoteDiscoveryGuide={(adapter,workerID)=>{if(writeBlocked||!activeWorkspace.current||pairingCaller.current.api!==api||pairingCaller.current.generation!==pairingGeneration)return;openToolPreparation(adapter,workerID,true,true);}}
          workers={workers}
          sharedWorkers={sharedWorkers}
          action={action}
          onRegistered={value=>{setReceipt({key:'registered',name:value.name,scope:contextKey});void profiles.read();onSelect(value.id);}}
          onOpen={profile=>{if(action.busy||action.pending)return;setReceipt(undefined);setDialog(undefined);onSelect(profile.id);}}
          onViewExisting={adapter=>{if(writeBlocked)return;setSearch(adapter.adapter||adapter.product);applyFilters({state:'active',query:adapter.adapter||adapter.product});setDialog(undefined);requestAnimationFrame(()=>document.querySelector<HTMLInputElement>('[data-tools-query]')?.focus());}}
          onEnable={(profile,justSaved)=>{action.clearError();setDialog({kind:'decision',action:'enable',profile,justSaved});}}
          onAdvanced={(adapter,expectedWorkerID)=>{
            if(writeBlocked||!activeWorkspace.current||pairingCaller.current.api!==api||pairingCaller.current.generation!==pairingGeneration)return;
            action.clearError();
            if(replacementSource||!openToolPreparation(adapter,expectedWorkerID))setDialog({kind:'create',adapter,resumeQuick:true});
          }}
          onWorker={workerManagement===true?openWorkerRegistration:undefined}
        />}
        {dialog?.kind === "worker" && <>
          {publicDraft&&<button type="button" className="text-button" data-tools-return-quick disabled={writeBlocked} onClick={openQuickConnect}>{t('tool-workspace.return_connect')}</button>}
          <div className="tool-action-bar">
            {workerManagement===true&&<button type="button" data-worker-mode="manual" aria-pressed={dialog.mode!=='pairing'} disabled={writeBlocked||!workerManagementReady} onClick={openWorkerRegistration}>{t('runner-pairing.mode_manual')}</button>}
            <button type="button" data-worker-mode="pairing" aria-pressed={dialog.mode==='pairing'} disabled={writeBlocked} onClick={openRunnerPairing}>{t('runner-pairing.mode_remote')}</button>
          </div>
          {dialog.mode==='pairing'?<RunnerPairingPanel project={project} allowCreate={workerManagementReady} disabled={pending||action.busy} onLockChange={setPairingLocked} onPaired={continueWithPairedRunner}/>:<WorkerForm
            project={project}
            action={action}
            disabled={writeBlocked||!workerManagementReady}
            onSaved={(value) => saved("/release-workers", value)}
            onCancel={() => setDialog(undefined)}
          />}
        </>}
        {dialog?.kind === "decision" && (
          <ProfileDecisionForm
            key={dialog.action+":"+dialog.profile.id}
            project={project}
            value={dialog.profile}
            decision={dialog.action}
            justSaved={dialog.justSaved}
            action={action}
            onSaved={(value) =>
              saved("/tool-profiles/" + value.id + "/commands", value)
            }
            onCancel={() => setDialog(undefined)}
          />
        )}
        {dialog?.kind === "rotate-worker" && !pending && (
          <WorkerRotationForm
            key={dialog.worker.id+':'+dialog.worker.version}
            worker={dialog.worker}
            action={action}
            disabled={writeBlocked||!rotationWorkerCurrent}
            closeBlocked={uploadRecoveryLocked}
            uploadReceiptOperation={rotationUploadReceipt?.operation_id}
            recovery={<WorkerArtifactUploadRecovery worker={dialog.worker} action={uploadAction}
              disabled={otherWriteBlocked||!rotationWorkerCurrent} restoreDisabled={action.busy||!workerManagementReady}
              receipt={rotationUploadReceipt} onSaved={setUploadReceipt}/>}
            onClose={()=>setDialog(undefined)}
            onRefresh={()=>{
              setDialog(undefined);
              void workers.read();
            }}
            onRotate={async()=>{
              const path='/release-workers/'+encodeURIComponent(dialog.worker.id)+'/rotate-credential';
              const body={project_id:project,expected_version:dialog.worker.version,acknowledge_stopped:true};
              if(!rotationWorkerCurrent||!validManualWorkerRotationCommand(path,body,'POST',project))return;
              const value=await action.run<ManualWorkerRotationReceipt>(path,body);
              if(value)saved(path,value);
            }}
          />
        )}
        {dialog?.kind === "revoke-worker" && (
          <form
            className="tool-form"
            onSubmit={async (event) => {
              event.preventDefault();
              if(writeBlocked||!workerManagementReady||!currentMaintenanceWorker||action.conflict)return;
              const worker = dialog.worker;
              const result = await action.run<ExecutionWorker>(
                "/release-workers/" + encodeURIComponent(worker.id),
                {
                  project_id: project,
                  expected_version: worker.version,
                  status: "revoked",
                },
                "PATCH",
              );
              if (result) saved("/release-workers/" + worker.id, result);
            }}
          >
            <p className="warning">
              <Trans i18nKey="development.tools.revoke_worker_warning" values={{worker:dialog.worker.worker_id}} components={{worker:<strong/>}}/></p>
            {workerManagementReady&&!currentMaintenanceWorker&&<p role="status">{t('tool-worker-identities.changed')}</p>}
            <div className="dialog-actions">
              <button
                type="button"
                disabled={action.busy || pending}
                onClick={() => setDialog(undefined)}
              >
                {t("development.workflow.cancel")}</button>
              <button
                className="primary"
                disabled={writeBlocked||!workerManagementReady||!currentMaintenanceWorker||action.conflict}
              >
                {t("development.tools.confirm_worker_credential_revocation")}</button>
            </div>
          </form>
        )}
      </Modal>
      <Modal
        open={!!credential}
        onClose={clearCredential}
        title={t(credential?.rotation?"worker-rotation.result_title":"development.tools.execution_worker_credential_for_this_registration")}
        description={t("development.tools.the_original_value_remains_only_in_this")}
      >
        {credential && (
          <div className="tool-token" data-worker-rotation-credential={credential.rotation?'':undefined}>
            <strong>{credential.worker_id}</strong>
            {credential.token ? (
              <>
                <p className="warning">
                  {t(credential.rotation?"worker-rotation.install":"development.tools.save_this_credential_in_the_controlled_runner")}</p>
                <Field label={t("development.tools.one_time_runner_credential")}>
                  <input
                    type={revealed ? "text" : "password"}
                    readOnly
                    value={credential.token}
                    autoComplete="off"
                  />
                </Field>
                <label className="check">
                  <input
                    type="checkbox"
                    checked={revealed}
                    onChange={(event) => setRevealed(event.target.checked)}
                  />
                  {t("development.tools.show_the_original_value_in_this_dialog")}</label>
                {credential.rotation&&<>
                  <button type="button" data-worker-rotation-copy onClick={async()=>{
                    if(visibleCredential.current!==credential||!credential.token)return;
                    try{
                      await navigator.clipboard.writeText(credential.token);
                      if(visibleCredential.current===credential)setCopyState('copied');
                    }catch{
                      if(visibleCredential.current===credential)setCopyState('failed');
                    }
                  }}><Copy size={15} aria-hidden="true"/>{t('worker-rotation.copy')}</button>
                  {copyState&&<small role="status">{t('worker-rotation.copy_'+copyState)}</small>}
                </>}
              </>
            ) : (
              <p className="warning">
                {t(credential.rotation?"worker-rotation.metadata_only":"development.tools.the_original_registration_receipt_was_recovered_but")}</p>
            )}
            <button type="button" data-worker-rotation-close={credential.rotation?'':undefined} onClick={clearCredential}>
              {t("development.tools.close_and_clear_the_original_value")}</button>
          </div>
        )}
      </Modal>
    </div>
  );
}
function WorkerRotationForm({worker,action,disabled,closeBlocked,uploadReceiptOperation,recovery,onClose,onRefresh,onRotate}:{
  worker:ExecutionWorker;
  action:ToolAction;
  disabled:boolean;
  onClose:()=>void;
  onRefresh:()=>void;
  onRotate:()=>Promise<void>;
  closeBlocked:boolean;
  uploadReceiptOperation?:string;
  recovery:ReactNode;
}){
  const stopBasis=uploadReceiptOperation||'';
  const [confirmedBasis,setConfirmedBasis]=useState<string>();
  const stopped=confirmedBasis===stopBasis;
  return <form className="tool-form" data-worker-rotation-form onSubmit={event=>{
    event.preventDefault();
    if(stopped&&!disabled&&!action.conflict)void onRotate();
  }}>
    <strong>{worker.worker_id}</strong>
    <p>{t('worker-rotation.stop_instructions')}</p>
    <p className="warning">{t('worker-rotation.boundary')}</p>
    {recovery}
    <label className="check">
      <input type="checkbox" data-worker-rotation-stopped checked={stopped} disabled={disabled||action.conflict} onChange={event=>setConfirmedBasis(event.target.checked?stopBasis:undefined)}/>
      {t('worker-rotation.acknowledge')}
    </label>
    {action.conflict&&<p role="status">{t('worker-rotation.conflict_remedy')}</p>}
    <div className="dialog-actions">
      <button type="button" disabled={action.busy||closeBlocked} onClick={onClose}>{t('development.workflow.cancel')}</button>
      {action.conflict?
        <button type="button" className="primary" data-worker-rotation-refresh disabled={closeBlocked} onClick={onRefresh}>{t('worker-rotation.refresh')}</button>:
        <button type="submit" className="primary" data-worker-rotation-submit disabled={disabled||!stopped}>{t('worker-rotation.submit')}</button>}
    </div>
  </form>;
}
function ToolProfileForm({
  project,
  adapter,
  draft,
  onDraftChange,
  onContinueReplacement,
  workers,
  sharedWorkers,
  action,
  onSaved,
  onCancel,
}: {
  project: string;
  adapter?:AdapterDescriptor;
  draft?:ToolConnectDraft;
  onDraftChange?:(draft:ToolConnectDraft)=>void;
  onContinueReplacement?:()=>void;
  workers: IdentityRead;
  sharedWorkers: SharedIdentityRead;
  action: ToolAction;
  onSaved: (value: ToolProfile) => void;
  onCancel: () => void;
}) { useLocale();
  const caller=useApi();
  const [manifest, setManifest] = useState(""),
    [observations, setObservations] = useState<ToolObservation[]>([]),
    [command, setCommand] = useState(draft?.commandID||""),
    [error, setError] = useState<unknown>(""),
    [submitError, setSubmitError] = useState<unknown>(''),
    [importing, setImporting] = useState(false),
    [name, setName] = useState(draft?.name||""),
    [nameEdited, setNameEdited] = useState(draft?.nameEdited||false),
    [source,setSource] = useState<'none'|'manual'|'discovery'>('none'),
    [manualRevision,setManualRevision] = useState(0),
    [worker, setWorker] = useState(draft?.workerID||"");
  const requestedCommand=useRef(draft?.commandID||'');
  useLayoutEffect(()=>{onDraftChange?.({adapterID:adapter?.id||draft?.adapterID||'',workerID:worker,commandID:command||requestedCommand.current,name,nameEdited});},[adapter?.id,draft?.adapterID,worker,command,name,nameEdited,onDraftChange]);
  const [adoption,setAdoption]=useState<ToolDiscoveryAdoption>(),[discoveryExpired,setDiscoveryExpired]=useState(false),[discoveryReadRevision,setDiscoveryReadRevision]=useState(0);
  useEffect(()=>{
    if(source!=='discovery'||!adoption)return;
    const timer=setTimeout(()=>setDiscoveryExpired(true),Math.max(0,adoption.deadline-performance.now()));
    return()=>clearTimeout(timer);
  },[source,adoption]);
  function discoveryIssue(){
    if(source!=='discovery')return;
    if(!adoption||adoption.apiCaller!==caller||adoption.project!==project||adoption.worker!==worker)return 'source_changed' as const;
    if(discoveryExpired||performance.now()>=adoption.deadline)return 'expired' as const;
  }
  const matchesAdapter=(item:ToolObservation)=>!adapter||(item.profile.adapter===adapter.adapter&&item.profile.protocol===adapter.protocol);
  const matchingObservations=observations.filter(matchesAdapter);
  const selected = matchingObservations.find(
      (item) => item.profile.command_id === command,
    ),
    blocked = action.busy || !!action.pending;
  const formID=useId();
  const identityReady=!!currentToolIdentity(worker,workers,sharedWorkers);
  const issue=discoveryIssue();
  const next=!identityReady?'identity':importing?'reading':issue??(!manifest.trim()?'inventory':error?'invalid':!matchingObservations.length?'empty':!selected?.available?'choose':!name.trim()?'name':'ready');
  function inspectManifest(text:string) {
    setManifest(text);
    setObservations([]);
    setCommand('');
    setError('');
    if (!text.trim()) return;
    try {
      const rows = parseToolManifest(text);
      const available = rows.filter(item=>item.available&&matchesAdapter(item));
      setObservations(rows);
      const preferred=available.find(item=>item.profile.command_id===requestedCommand.current);
      const chosen=preferred||(!requestedCommand.current&&available.length===1?available[0]:undefined);
      if (chosen) {
        setCommand(chosen.profile.command_id);
        if (!nameEdited) setName(chosen.profile.command_id);
      } else if (!nameEdited) setName('');
    } catch (error) {
      setError(error);
    }
  }
  return (
    <form
      className="tool-form"
      onSubmit={async (event) => {
        event.preventDefault();
        setSubmitError("");
        if(importing)return;
        if (!currentToolIdentity(worker, workers, sharedWorkers)) {
          setSubmitError(t("tools.identity.authorization_unconfirmed"));
          return;
        }
        const sourceIssue=discoveryIssue();
        if(sourceIssue){
          setSubmitError(t('tool-workspace.form_next.'+sourceIssue));
          if(sourceIssue==='expired')setDiscoveryExpired(true);
          return;
        }
        if (!selected?.available) {
          setSubmitError(t("development.tools.choose_an_available_probed_configuration_with_complete"));
          return;
        }
        if(onContinueReplacement){onContinueReplacement();return;}
        const result = await action.run<ToolProfile>("/tool-profiles", {
          project_id: project,
          worker_id: worker,
          name,
          profile: selected.profile,
        });
        if (result) onSaved(result);
      }}
    >
      {!!submitError && <ErrorNotice error={renderDiagnostic(submitError)} />}
      {adapter&&<div className="tool-form-intro" role="status"><strong>{adapter.product}</strong><p>{t('adapter-connections.selected_execution',{adapter:adapter.adapter||'',protocol:adapter.protocol||''})}</p></div>}
      {adapter&&manifest.trim()&&!error&&!matchingObservations.length&&<p className="warning" role="status">{t('adapter-connections.no_matching_inventory')}</p>}
      <p className="tool-form-intro">
        {t('tool-discovery.form_intro')}</p>
      <fieldset className="form-fields" disabled={blocked}>
        <section className="tool-form-section" aria-labelledby={formID+'-identity'}>
        <h3 id={formID+'-identity'}><span aria-hidden="true">{formatNumber(1)}</span>{t('tool-workspace.form_identity')}</h3>
        <ToolIdentity
          local={workers}
          shared={sharedWorkers}
          value={worker}
          onChange={(value) => {
            requestedCommand.current='';
            setWorker(value);
            setImporting(false);
            if(source==='discovery') {inspectManifest('');setSource('none');if(!nameEdited)setName('');}
            if (action.conflict) action.clearError();
          }}
        />
        </section>
        <section className="tool-form-section" aria-labelledby={formID+'-inventory'}>
        <h3 id={formID+'-inventory'}><span aria-hidden="true">{formatNumber(2)}</span>{t('tool-discovery.inventory_source')}</h3>
        <ToolDiscoveryPanel project={project} worker={worker} authorized={identityReady} disabled={blocked||importing} autoAdopt={source==='none'} manualRevision={manualRevision} readRevision={discoveryReadRevision} onAdopt={(items,original)=>{
          if(original.apiCaller!==caller||original.project!==project||original.worker!==worker||performance.now()>=original.deadline)return;
          inspectManifest(JSON.stringify({items}));setAdoption(original);setDiscoveryExpired(false);setSubmitError('');setSource('discovery');
        }}/>
        <details className="tool-manual-inventory"><summary>{t('tool-discovery.manual_inventory')}</summary>
        <p className="tool-form-intro"><Trans i18nKey="development.tools.inventory_instructions" components={{command:<code/>}}/></p>
        <ToolInventoryField key={worker} value={manifest} error={error} disabled={blocked} onChange={text=>{setSource('manual');setManualRevision(n=>n+1);inspectManifest(text);}} onReading={reading=>{setImporting(reading);if(reading){setSource(current=>current==='none'?'manual':current);setManualRevision(n=>n+1);}}}/>
        <button
          type="button"
          className="text-button"
          disabled={importing}
          onClick={() => inspectManifest(manifest)}
        >
          {t("development.tools.validate_inventory_fields")}</button>
        </details>
        </section>
        <section className="tool-form-section" aria-labelledby={formID+'-configuration'}>
        <h3 id={formID+'-configuration'}><span aria-hidden="true">{formatNumber(3)}</span>{t('tool-workspace.form_configuration')}</h3>
        {matchingObservations.length > 0 && (
          <>
            <p className={'tool-import-result '+(matchingObservations.some(item=>item.available)?'':'needs-attention')} role="status"><ShieldCheck size={16}/>{t('tool-workspace.import_result',{count:formatNumber(matchingObservations.length),available:formatNumber(matchingObservations.filter(item=>item.available).length)})}</p>
            <Field label={t("development.tools.choose_an_inventory_configuration")}>
              <select
                required
                value={command}
                onChange={(event) => {requestedCommand.current=event.target.value;setCommand(event.target.value);if(!nameEdited)setName(event.target.value);}}
              >
                <option value="">{t("development.tools.choose_a_probed_fixed_configuration")}</option>
                {matchingObservations.map((item) => (
                  <option
                    key={item.profile.command_id}
                    value={item.profile.command_id}
                    disabled={!item.available}
                  >
                    {t("development.tools.observed_configuration_option",{command:item.profile.command_id,version:item.profile.tool_version||t("development.tools.version_unconfirmed"),status:item.available?t("development.tools.dispatchable_at_last_read"):t("development.tools.unavailable")})}
                  </option>
                ))}
              </select>
            </Field>
            <div className="tool-observations">
              {matchingObservations
                .filter((item) => !item.available)
                .map((item) => (
                  <article key={item.profile.command_id}>
                    <strong>{t("development.tools.observation_unconfirmed",{command:item.profile.command_id})}</strong>
                    <p>{item.reason}</p>
                  </article>
                ))}
            </div>
          </>
        )}
        <Field label={t("development.tools.configuration_name")} hint={t('tool-workspace.name_help')}>
          <input required maxLength={200} value={name} onChange={event=>{setName(event.target.value);setNameEdited(true);}}/>
        </Field>
        {selected && (
          <div className="tool-context">
            <strong>
              {selected.profile.adapter} / {selected.profile.protocol}
            </strong>
            <p>
              {selected.profile.tool_version} ·{" "}
              {selected.profile.read_only ? t("development.tools.read_only") : t("development.tools.write_permitted_separate_worktree")}
            </p>
            <details className="tool-import-fingerprint"><summary>{t('development.facts.configuration_fingerprint')}</summary><code>{selected.profile.profile_sha256}</code></details>
          </div>
        )}
        </section>
        <ToolRegistrationPreflight checks={[
          {id:'identity',label:t('tool-workspace.form_identity'),state:identityReady?'ready':'pending',detail:t(identityReady?'tool-workspace.preflight.identity_ready':'tool-workspace.form_next.identity')},
          {id:'inventory',label:t('tool-discovery.inventory_source'),state:importing?'pending':issue||error?'attention':manifest.trim()&&matchingObservations.length?'ready':'pending',detail:importing?t('tool-workspace.form_next.reading'):issue?t('tool-workspace.form_next.'+issue):t(source==='discovery'?'tool-workspace.preflight.inventory_live':source==='manual'&&!error?'tool-workspace.preflight.inventory_manual':'tool-workspace.form_next.inventory')},
          {id:'configuration',label:t('tool-workspace.form_configuration'),state:selected?.available&&name.trim()?'ready':'pending',detail:t(selected?.available&&name.trim()?'tool-workspace.preflight.configuration_ready':selected?.available?'tool-workspace.form_next.name':'tool-workspace.form_next.choose')},
        ]} next={issue?t('tool-workspace.form_next.'+issue):undefined} disabled={blocked||importing} onReview={identityReady&&issue?()=>setDiscoveryReadRevision(value=>value+1):undefined} reviewLabel={t('tool-workspace.preflight.reread')}/>
      </fieldset>
      <div className="tool-profile-submit">
      <p id={formID+'-next'} data-tool-form-next className={'tool-form-next '+(next==='ready'?'ready':'')} aria-live="polite">{next==='ready'&&<ShieldCheck size={16} aria-hidden="true"/>}{t('tool-workspace.form_next.'+next)}</p>
      <div className="dialog-actions">
        <button type="button" disabled={blocked} onClick={onCancel}>
          {t("development.workflow.cancel")}</button>
        <button
          className="primary"
          disabled={
            blocked ||
            importing ||
            action.conflict ||
            !!issue ||
            !selected?.available ||
            !worker ||
            !currentToolIdentity(worker, workers, sharedWorkers) ||
            !name.trim()
          }
          aria-describedby={formID+'-next'}
        >
          {onContinueReplacement?t('tool-profile-replacement.prepare'):action.busy?t('tool-workspace.saving'):t("development.tools.save_as_paused_configuration")}</button>
      </div>
      </div>
    </form>
  );
}
function WorkerForm({
  project,
  action,
  disabled,
  onSaved,
  onCancel,
}: {
  project: string;
  action: ToolAction;
  disabled: boolean;
  onSaved: (value: unknown) => void;
  onCancel: () => void;
}) { useLocale();
  const [worker, setWorker] = useState("");
  return (
    <form
      className="tool-form"
      onSubmit={async (event) => {
        event.preventDefault();
        if(disabled||action.busy||action.pending||action.conflict)return;
        const result = await action.run("/release-workers", {
          project_id: project,
          worker_id: worker,
          profiles: [],
          deployment: false,
        });
        if (result) onSaved(result);
      }}
    >
      <p>
        {t("development.tools.register_a_tool_only_execution_identity_using")}</p>
      <fieldset
        className="form-fields"
        disabled={action.busy || !!action.pending}
      >
        <Field label={t("development.tools.unique_execution_worker_id")}>
          <input
            required
            pattern="[A-Za-z0-9][A-Za-z0-9_.\-]{0,99}"
            maxLength={100}
            value={worker}
            onChange={(event) => {
              setWorker(event.target.value);
              if (action.conflict) action.clearError();
            }}
            placeholder={t("development.tools.e_g_team_code_runner")}
          />
        </Field>
      </fieldset>
      {action.conflict && (
        <p className="warning">
          {t("development.tools.this_worker_id_has_a_previous_registration")}</p>
      )}
      <div className="dialog-actions">
        <button
          type="button"
          disabled={action.busy || !!action.pending}
          onClick={onCancel}
        >
          {t("development.workflow.cancel")}</button>
        <button
          className="primary"
          disabled={disabled||action.busy||!!action.pending||action.conflict}
        >
          {t("development.tools.register_and_get_this_credential")}</button>
      </div>
    </form>
  );
}
