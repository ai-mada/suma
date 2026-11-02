import {useApi} from './api-context';
import { Trans } from "./i18n/Trans";
import { tr as t, UiError, useLocale, formatDateTime, formatNumber, renderDiagnostic } from "./i18n";
import { useCallback, useEffect, useId, useLayoutEffect, useMemo, useRef, useState } from "react";
import {
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
} from "lucide-react";
import { ApiError, api, query, type Session } from "./api";
import { Empty, ErrorNotice, Field, Loading, Modal } from "./ui";
import { ToolWriteRecovery } from "./tool-write";
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
import {readToolListFilters,toolListSearchParams,toolListStates,validToolListQuery,type ToolListFilters} from './tool-management-list-model';
import {ToolInventoryField} from './ToolInventoryField';
import {ToolDiscoveryPanel,type ToolDiscoveryAdoption} from './ToolDiscoveryPanel';
import {ToolRegistrationPreflight} from './ToolRegistrationPreflight';
import "./tool.css";
import { ToolRecoveryNotice } from "./ToolActions";
import {ToolSetupGuide} from './ToolSetupGuide';
import {toolPreparationTarget,preparationForLocalTool,type ToolPreparationPreset} from './tool-preparation-target';
import {validToolReportFeedbackSelection,type ToolReportFeedbackSelection} from './tool-report-feedback-model';
import {ToolQuickConnect,type ToolConnectDraft} from './ToolQuickConnect';
import {RemoteToolScanPanel} from './RemoteToolScanPanel';
import {ToolProfileDoctor} from './ToolProfileDoctor';
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
import type { ProjectPoolRunnerPage, RunnerPairing, ToolProfileReplacement } from "../../../packages/contracts/src";
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
};
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
    }
  | { kind: "revoke-worker"; worker: ExecutionWorker };
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
}: Props) { const api=useApi(); useLocale();
  const [revision, setRevision] = useState(0),
    [filters,setFilters]=useState(()=>readToolListFilters(new URLSearchParams(window.location.search))),
    [search, setSearch] = useState(filters.query),
    [searchError,setSearchError]=useState(false),
    [dialogState,setDialogState]=useState<{caller:typeof api;generation:number;value:Dialog}>(),
    [credential, setCredential] = useState<{
      worker_id: string;
      token?: string;
    }>(),
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
  function setDialog(value:Dialog|undefined){if(pairingCaller.current.api===api&&pairingCaller.current.generation===pairingGeneration)setDialogState(value?{caller:api,generation:pairingGeneration,value}:undefined);}
  const contextKey=JSON.stringify([session.organization.id,session.user.id,project]);
  const connectIntent=useRef(0),connectDraft=useRef<{caller:typeof api;generation:number;context:string;nonce:number;value:ToolConnectDraft;sourceTool?:ToolProfile}|undefined>(undefined);
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
  const workerRows = useRef(new Map<string, HTMLElement>());
  const sharedWorkerRows=useRef(new Map<string,HTMLElement>());
  // Guide consumers retain their nonce high-water mark across navigation.
  // Clearing a selection must not restart this producer's request identity.
  const connectionRequestNonce=useRef(0);
  const [connectionSelection,setConnectionSelection]=useState<{context:string;caller:typeof api;generation:number;nonce:number;expectedWorkerID?:string;client?:DeveloperClientID;preset?:ToolPreparationPreset|'octop';cloud?:boolean}>();
  const connection=connectionSelection?.context===contextKey&&connectionSelection.caller===api&&connectionSelection.generation===pairingGeneration?connectionSelection:undefined;
  const recovery = useMemo(
    () =>
      new ToolWriteRecovery(session.organization.id +
          ":" +
          session.user.id +
          ":" +
          project +
          ":profiles", toolSessionStorage(), api,
        (value, request) => isToolProfileReplacementPath(request.path)?validToolProfileReplacementReceipt(value,request,session.user.id):request.path !== '/tool-profiles' || validQuickConnectReceipt(value, request.body)),
    [session.organization.id, session.user.id, project, api],
  );
  const action = useToolAction(recovery, () => setDialog(undefined)),
    profiles = useManagedToolProfiles<ToolProfile>(project,filters,refresh+revision),
    workers = useToolRead<{ items: ExecutionWorker[] }>(
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
  const detailBasis=JSON.stringify([selected,detail.data?.id,detail.data?.version,detail.data?.worker_id,detail.data?.profile.profile_sha256]),latestDetailBasis=useRef(detailBasis);latestDetailBasis.current=detailBasis;
  useEffect(()=>()=>inspectionLookup.current?.abort(),[api,detailBasis]);
  const refreshAll = useCallback(() => {
    if (!profiles.loading) void profiles.read();
    if (!workers.loading) void workers.read();
    if (!sharedWorkers.loading) void sharedWorkers.read();
    if (selected && !detail.loading) void detail.read();
  }, [profiles.loading, profiles.read, workers.loading, workers.read, sharedWorkers.loading, sharedWorkers.read, selected, detail.loading, detail.read]);
  useEffect(() => {
    if (!live) return;
    const timer = setInterval(() => {
      if (document.visibilityState !== 'hidden') refreshAll();
    }, 10000);
    return () => clearInterval(timer);
  }, [live, refreshAll]);
  const pending = !!action.pending,
    writeBlocked = pending || action.busy || pairingLocked || inspectionLocked || connecting,
    current = detail.data,
    stale = detail.loading || !!detail.error,
    uncertain = profiles.loading || !!profiles.error || !profiles.items,
    filtered = profiles.items?.filter((profile) => matchesToolFilter(profile, filter));
  const replacementSourceCurrent=!!replacementSource&&!stale&&current?.id===replacementSource.id&&current.version===replacementSource.version&&current.worker_id===replacementSource.worker_id&&current.state===replacementSource.state&&sameToolProfile(current.profile,replacementSource.profile);
  const reportReturnOwner=useRef({api,contextKey,generation:pairingGeneration,nonce:connection?.nonce,writeBlocked});reportReturnOwner.current={api,contextKey,generation:pairingGeneration,nonce:connection?.nonce,writeBlocked};
  function applyFilters(value:ToolListFilters){
    if(!validToolListQuery(value.query)){setSearchError(true);return;}
    const next={...value,query:value.query.trim()};setSearchError(false);setFilters(next);setFilter('all');
    const params=toolListSearchParams(new URLSearchParams(window.location.search),next);
    window.history.replaceState(window.history.state,'',window.location.pathname+(params.size?'?'+params:'')+window.location.hash);
  }
  useEffect(()=>{const update=()=>{const next=readToolListFilters(new URLSearchParams(window.location.search));setFilters(next);setSearch(next.query);setSearchError(false);setFilter('all');};window.addEventListener('popstate',update);return()=>window.removeEventListener('popstate',update);},[]);
  const identitiesKnown = !!workers.data && !workers.loading && !workers.error && !!sharedWorkers.data && !sharedWorkers.loading && !sharedWorkers.error;
  const identityCount = new Set([
    ...(workers.data?.items || []), ...(sharedWorkers.data?.items || []),
  ].filter(item => item.status === 'active').map(item => item.worker_id)).size;
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
    const sameTarget=publicDraft?.adapterID===value.adapterID&&publicDraft.workerID===value.workerID;
    beginPublicDraft({adapterID:value.adapterID,workerID:value.workerID,commandID:value.commandID,name:sameTarget?publicDraft.name:'',nameEdited:sameTarget?publicDraft.nameEdited:false},sameTarget?replacementSource:undefined);
    action.clearError();void workers.read();void sharedWorkers.read();setDialog({kind:'quick',preferredWorkerID:value.workerID});
  }
  function updatePublicDraft(value:ToolConnectDraft,nonce:number|undefined){
    const owner=connectDraft.current;
    if(!owner||owner.nonce!==nonce||owner.caller!==api||owner.context!==contextKey||owner.generation!==pairingGeneration||pairingCaller.current.api!==api||pairingCaller.current.generation!==pairingGeneration)return;
    if(JSON.stringify(owner.value)!==JSON.stringify(value)){connectDraft.current={...owner,value:{...value}};renderDraft(value=>value+1);}
  }
  function beginPublicDraft(value:ToolConnectDraft,sourceTool?:ToolProfile){
    const nonce=++connectIntent.current;connectDraft.current={caller:api,generation:pairingGeneration,context:contextKey,nonce,value:{...value},...(sourceTool?{sourceTool:structuredClone(sourceTool)}:{})};renderDraft(value=>value+1);return nonce;
  }
  async function configureProfile(value:ToolProfile){
    if(writeBlocked||stale||!current||value.project_id!==project||value.id!==current.id||value.version!==current.version||value.worker_id!==current.worker_id||value.profile.profile_sha256!==current.profile.profile_sha256)return;
    const nonce=++connectIntent.current;setConnecting(true);setConnectError(undefined);
    try{
      const catalog=readAdapterCatalog(await api<unknown>('/adapter-catalog'+query(project)),project);
      if(!activeWorkspace.current||nonce!==connectIntent.current||latestDetailBasis.current!==detailBasis||pairingCaller.current.api!==api||pairingCaller.current.generation!==pairingGeneration)return;
      const matches=catalog?.items.filter(item=>item.direction==='suma_to_tool'&&item.implementation==='implemented'&&item.setup_kind==='runner_profile'&&item.adapter===value.profile.adapter&&item.protocol===value.profile.protocol);
      if(matches?.length!==1)throw new Error(t('tool-workspace.target_unavailable'));
      beginPublicDraft({adapterID:matches[0].id,workerID:value.worker_id,commandID:value.profile.command_id,name:value.name,nameEdited:true},value.state==='revoked'?undefined:value);
      action.clearError();void workers.read();void sharedWorkers.read();setDialog({kind:'quick',adapter:matches[0],preferredWorkerID:value.worker_id});
    }catch(error){if(activeWorkspace.current&&nonce===connectIntent.current&&pairingCaller.current.api===api)setConnectError(error);}
    finally{if(activeWorkspace.current&&pairingCaller.current.api===api&&pairingCaller.current.generation===pairingGeneration)setConnecting(false);}
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
    setDialog(undefined);setInspectionLocked(false);
    requestAnimationFrame(()=>{if(pairingCaller.current.api!==api||pairingCaller.current.generation!==pairingGeneration||!row.isConnected)return;row.scrollIntoView?.({block:'nearest'});row.focus({preventScroll:true});});
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
    const sameTarget=publicDraft?.adapterID===adapter!.id&&publicDraft.workerID===expectedWorkerID;
    if(expectedWorkerID&&!sameTarget)beginPublicDraft({adapterID:adapter!.id,workerID:expectedWorkerID,commandID:'',name:'',nameEdited:false});
    setDialog(undefined);setConnectionSelection({context:contextKey,caller:api,generation:pairingGeneration,nonce:++connectionRequestNonce.current,expectedWorkerID,preset:target.preset});setGuideOpen(true);
    requestAnimationFrame(()=>document.getElementById(guideID)?.querySelector<HTMLElement>(discovery?'[data-remote-runner-allow-installed-discovery]':focusRemote?'[data-remote-runner-setup]>summary':'summary')?.focus());return true;
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
    requestAnimationFrame(() => document.getElementById(guideID)?.querySelector('summary')?.focus());
  }
  function setupConnection(descriptor:AdapterDescriptor){
    if(writeBlocked||!canLeave()||descriptor.implementation==='planned')return;
    if(descriptor.id==='workbuddy-cloud'&&descriptor.direction==='suma_to_tool'&&descriptor.entrypoint_kind==='cloud_api'&&descriptor.implementation==='implemented'&&descriptor.setup_kind==='manual_guide'&&!descriptor.adapter&&!descriptor.protocol){setConnectionSelection({context:contextKey,caller:api,generation:pairingGeneration,nonce:++connectionRequestNonce.current,cloud:true});return;}
    if(descriptor.direction==='suma_to_tool'&&descriptor.setup_kind==='runner_profile'&&descriptor.implementation==='implemented'&&descriptor.adapter&&descriptor.protocol){
      beginPublicDraft(publicDraft?.adapterID===descriptor.id?publicDraft:{adapterID:descriptor.id,workerID:'',commandID:'',name:'',nameEdited:false},publicDraft?.adapterID===descriptor.id?replacementSource:undefined);
      action.clearError();void workers.read();void sharedWorkers.read();setDialog({kind:'quick',adapter:descriptor});return;
    }
    const clients:Record<string,DeveloperClientID>={'cursor-client':'cursor','claude-code-client':'claude-code','codex-client':'codex','opencode-client':'opencode','qoder-client':'qoder','zed-client':'zed','workbuddy-connector':'workbuddy'};
    if(descriptor.direction==='tool_to_suma'&&clients[descriptor.id]){setConnectionSelection({context:contextKey,caller:api,generation:pairingGeneration,nonce:++connectionRequestNonce.current,client:clients[descriptor.id]});setGuideOpen(true);return;}
    if(descriptor.id==='harness-guide'&&descriptor.setup_kind==='manual_guide'){setConnectionSelection({context:contextKey,caller:api,generation:pairingGeneration,nonce:++connectionRequestNonce.current,preset:'harness'});setGuideOpen(true);}
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
    if(isToolProfileReplacementPath(path)){replaced(readToolProfileReplacement(value,{project,originalID:toolProfileReplacementOriginalID(path)}));return;}
    setDialog(undefined);
    setRevision((before) => before + 1);
    if (!value || typeof value !== "object") return;
    const result = value as Record<string, unknown>;
    if(path==='/tool-profiles')setReceipt({key:'registered',name:String(result.name||''),scope:contextKey});
    else if(path.startsWith('/tool-profiles/')&&['enabled','paused','revoked'].includes(String(result.state)))setReceipt({key:String(result.state),name:String(result.name||''),scope:contextKey});
    else if(path.startsWith('/release-workers/'))setReceipt({key:'worker_revoked',name:String(result.worker_id||''),scope:contextKey});
    if (path === "/release-workers") {
      setRevealed(false);
      setCredential({
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
    <div className="tool-workspace">
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
      <ToolReadiness items={profiles.items} total={profiles.total} uncertain={uncertain} identitiesKnown={identitiesKnown} identityCount={identityCount} disabled={writeBlocked} filtered={filters.state!=='active'||!!filters.query} onClearFilters={()=>{setSearch('');applyFilters({state:'active',query:''});}} onWorker={() => { if(writeBlocked)return;action.clearError(); setDialog({kind:'worker'}); }} onRegister={openQuickConnect} onGuide={openGuide} onSelect={id => { if(canLeave()) onSelect(id); }}/>
      <WorkBuddyConnections project={project} panel={panel} openRequest={connection?.cloud?connection.nonce:undefined}/>
      <details className="panel adapter-catalog-disclosure" key={contextKey}>
        <summary>
          <ChevronRight aria-hidden="true" size={20}/>
          <span className="adapter-catalog-disclosure-copy"><strong>{t('adapter-connections.title')}</strong><span>{t('adapter-connections.intro')}</span></span>
          <span className="adapter-catalog-disclosure-action"><span className="when-closed">{t('adapter-connections.open_catalog')}</span><span className="when-open">{t('adapter-connections.close_catalog')}</span></span>
        </summary>
        <AdapterConnections project={project} onSetup={setupConnection}/>
      </details>
      {publicDraft&&guideOpen&&<div className="tool-connect-return"><p>{t('tool-workspace.draft_retained',{worker:publicDraft.workerID||t('tool-workspace.target_not_selected')})}</p><button type="button" data-tools-return-quick disabled={writeBlocked} onClick={openQuickConnect}>{t('tool-workspace.return_connect')}</button></div>}
      {connection?.client?<section className="panel"><button type="button" onClick={openGuide}>{t('adapter-connections.return_guide')}</button><DeveloperClientConnections key={contextKey} project={project} request={{client:connection.client,nonce:connection.nonce}}/></section>:<ToolSetupGuide project={project} pairedRunner={pairedRunner} expectedWorkerID={connection?.expectedWorkerID} id={guideID} expanded={guideOpen} onExpandedChange={setGuideOpen} presetRequest={connection?.preset?{preset:connection.preset,nonce:connection.nonce}:undefined} onPairRunner={openRunnerPairing} onReportSelection={returnFromReport} disabled={writeBlocked}/>}
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
      {replacementReceipt&&<ToolProfileReplacementReceipt value={replacementReceipt} disabled={writeBlocked} onViewOriginal={profile=>{if(!writeBlocked&&canLeave())onSelect(profile.id);}} onViewReplacement={profile=>{if(!writeBlocked&&canLeave())onSelect(profile.id);}} onEnableReplacement={profile=>void reviewReplacementTarget(profile)}/>}
      {!!connectError&&<ErrorNotice error={renderDiagnostic(connectError)}/>}
      {receipt?.scope===contextKey&&<div className="tool-save-receipt" data-tool-receipt role="status"><CheckCircle2 size={18} aria-hidden="true"/><p>{t('tool-workspace.receipt.'+receipt.key,{name:receipt.name})}</p><button className="icon-button" aria-label={t('tool-workspace.dismiss_receipt')} onClick={()=>setReceipt(undefined)}><X size={16}/></button></div>}
      <div className="tool-refresh-bar">
        <small>{live ? t("development.tools.auto_refresh") : t("development.tools.auto_refresh_paused")}</small>
        <div className="inline">
          <button className="text-button" onClick={() => setLive(value=>!value)}>{live?<Pause size={13}/>:<Play size={13}/>} {t(live?'tool-workspace.pause_refresh':'tool-workspace.resume_refresh')}</button>
          <button aria-label={t("development.tools.refresh_tool_configurations")} disabled={profiles.loading} onClick={refreshAll}><RefreshCw size={14}/>{t('tool-workspace.refresh')}</button>
        </div>
      </div>
      <div className={"tool-columns " + (selected ? "with-detail" : "")}>
        <section className="panel tool-list">
          <div className="list-toolbar">
            <span>
              {t("development.tools.fixed_tool_configuration")}<b>{profiles.items ? formatNumber(profiles.total) : "—"}</b>
            </span>
            <div className="inline">
              <button
                className="primary"
                disabled={writeBlocked}
                onClick={openQuickConnect}
              >
                <Plus size={14} />
                {t('tool-quick-connect.title')}</button>
              <details className="tool-quick-advanced"><summary>{t('tool-quick-connect.advanced')}</summary><button type="button" disabled={writeBlocked} onClick={openRegister}>{t('development.tools.register_configuration')}</button></details>
            </div>
          </div>
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
          <Field label={t('tool-workspace.window_filter')}><select data-tools-window-filter value={filter} onChange={event=>setFilter(event.target.value as ToolFilter)}>{(['all','available','attention'] as const).map(value=><option key={value} value={value}>{t('tool-workspace.filter.'+value)}</option>)}</select></Field>
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
              title={filters.query || filters.state!=='active' || filter!=='all' ? t('tool-workspace.no_matches') : t("development.tools.no_execution_tools_registered")}
              description={
                filters.query || filters.state!=='active' || filter!=='all'
                  ? t('tool-workspace.no_matches_hint')
                  : t("development.tools.register_an_execution_worker_identity_then_choose")
              }
              action={filters.query || filters.state!=='active' || filter!=='all'?<button onClick={() => {setSearch('');applyFilters({state:'active',query:''});}}>{t('tool-workspace.clear_filters')}</button>:<button onClick={openGuide}><CircleHelp size={14}/>{t('tool-workspace.setup_guide')}</button>}
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
                  <ToolProfileDoctor project={project} tool={current} confirmed={!stale} pending={pending} disabled={writeBlocked}
                    onReview={value=>{if(stale||writeBlocked||current.state==='revoked'||value.id!==current.id||value.project_id!==project||value.version!==current.version||value.worker_id!==current.worker_id)return;action.clearError();setDialog({kind:'decision',action:'enable',profile:current});}}
                    onRegister={value=>{if(!stale&&!writeBlocked)void configureProfile(value);}}
                    onInspect={inspectProfile}
                    onRefreshTool={()=>{if(!writeBlocked)void detail.read();}}
                    onWorker={!workers.loading&&!workers.error&&workers.data?.items.some(worker=>worker.worker_id===current.worker_id)?workerID=>{if(stale||writeBlocked||workerID!==current.worker_id||workers.loading||workers.error||!workers.data?.items.some(worker=>worker.worker_id===workerID))return;const row=workerRows.current.get(workerID);row?.scrollIntoView?.({block:'nearest'});row?.focus({preventScroll:true});}:undefined}
                    onSharedWorker={!sharedWorkers.loading&&!sharedWorkers.error&&sharedWorkers.data?.items.some(worker=>worker.worker_id===current.worker_id)?workerID=>{if(stale||writeBlocked||workerID!==current.worker_id||sharedWorkers.loading||sharedWorkers.error||!sharedWorkers.data?.items.some(worker=>worker.worker_id===workerID))return;const row=sharedWorkerRows.current.get(workerID);row?.scrollIntoView?.({block:'nearest'});row?.focus({preventScroll:true});}:undefined}
                    onRun={(runID,nodeID)=>{if(!stale&&!writeBlocked&&canLeave())window.location.assign('?'+new URLSearchParams({project,view:'development',sub:'runs',run:runID,node:nodeID}));}}
                  />
                  <ToolProfileReplacementHistory project={project} profileID={current.id} disabled={writeBlocked||stale} accessBlocked={detail.failure instanceof ApiError&&[401,403,404].includes(detail.failure.status)} refresh={refresh+revision} onViewProfile={profile=>{if(!stale&&!writeBlocked&&canLeave())onSelect(profile.id);}} onAccessDenied={()=>{if(pairingCaller.current.api===api&&pairingCaller.current.generation===pairingGeneration)void detail.read();}}/>
                  <div className="tool-action-bar">
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
                  </ToolProfileFacts>
                </>
              )}
            </div>
          </section>
        )}
      </div>
      <div className="tool-boundary-note"><ShieldCheck size={16}/><p>{t("development.tools.configuration_probing_actual_execution_and_acceptance_are")}</p></div>
      <section className="panel tool-workers">
        <div className="tool-section-heading">
          <div>
            <h3>{t("development.tools.execution_worker_identity")}</h3>
            <p className="muted">
              {t("development.tools.reuse_a_runner_credential_bound_by_the")}</p>
          </div>
          <button
            disabled={writeBlocked}
            onClick={() => {
              if(writeBlocked)return;
              action.clearError();
              setDialog({ kind: "worker" });
            }}
          >
            <Plus size={14} />
            {t("development.tools.register_execution_worker")}</button>
        </div>
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
          workers.data.items.map((worker) => (
            <article className="tool-worker-row" key={worker.id} tabIndex={-1} data-tool-worker={worker.worker_id} ref={element=>{if(element)workerRows.current.set(worker.worker_id,element);else workerRows.current.delete(worker.worker_id);}}>
              <div>
                <strong>
                  <Terminal size={14} /> {worker.worker_id}
                </strong>
                <small>
                  {t("development.tools.expires_at",{status:worker.status === "active"?t("development.tools.credential_registered_consult_tool_observations_for_online"):t("development.tools.credential_revoked"),at:formatDateTime(worker.expires_at,{dateStyle:"short",timeStyle:"medium"})})}
                </small>
                <small>
                  {worker.deployment ? t("development.tools.also_permitted_to_release") : t("development.tools.no_deployment_permission")}
                </small>
              </div>
              {worker.status === "active"&&<button type="button" data-tool-worker-scan={worker.worker_id} disabled={writeBlocked} onClick={()=>openRemoteScan(worker.worker_id)}>{t('remote-tool-scan.open')}</button>}
              {worker.status === "active" && (
                <button
                  disabled={writeBlocked}
                  onClick={() => {
                    if(writeBlocked)return;
                    action.clearError();
                    setDialog({ kind: "revoke-worker", worker });
                  }}
                >
                  {t("development.tools.revoke_worker_credential")}</button>
              )}
            </article>
          ))
        ) : (
          <Empty
            title={t("development.tools.no_bound_execution_identity")}
            description={t("development.tools.registration_issues_a_one_time_project_scoped")}
          />
        )}
        {!sharedWorkers.loading&&!sharedWorkers.error&&!!sharedWorkers.data?.items.length&&<div className="tool-shared-workers"><h4>{t('tool-workspace.shared_nodes')}</h4><p className="muted">{t('tool-workspace.shared_nodes_hint')}</p>{sharedWorkers.data.items.map(worker=><article key={worker.id} className="tool-worker-row" tabIndex={-1} data-tool-shared-worker={worker.worker_id} ref={element=>{if(element)sharedWorkerRows.current.set(worker.worker_id,element);else sharedWorkerRows.current.delete(worker.worker_id);}}><div><strong>{worker.name||worker.worker_id}</strong><small>{worker.worker_id} · {worker.pool_id}</small><small>{t('tool-workspace.shared_node_status',{status:worker.status})}</small></div><div><button type="button" data-tool-worker-scan={worker.worker_id} disabled={writeBlocked} onClick={()=>openRemoteScan(worker.worker_id)}>{t('remote-tool-scan.open')}</button><span className="tool-status-pill">{t('tool-workspace.read_only_node')}</span></div></article>)}</div>}
      </section>
      <Modal
        open={!!dialog}
        onClose={() => {
          if (!action.busy && !action.pending && !pairingLocked && !inspectionLocked) setDialog(undefined);
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
        description={t(dialog?.kind==='inspection'?'tool-doctor.inspect_hint':"development.tools.use_the_current_project_s_actual_identity")}
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
        {dialog?.kind==='remote-scan'&&dialog.context===contextKey&&dialog.caller===api&&dialog.generation===pairingGeneration&&<RemoteToolScanPanel project={project} workerID={dialog.workerID} disabled={writeBlocked} onPrepare={(adapter,workerID)=>{if(writeBlocked||!activeWorkspace.current||pairingCaller.current.api!==api||pairingCaller.current.generation!==pairingGeneration||workerID!==dialog.workerID)return;openToolPreparation(adapter,workerID,true);}} onGuide={()=>{if(writeBlocked||!activeWorkspace.current||pairingCaller.current.api!==api||pairingCaller.current.generation!==pairingGeneration)return;setDialog(undefined);setConnectionSelection({context:contextKey,caller:api,generation:pairingGeneration,nonce:++connectionRequestNonce.current,expectedWorkerID:dialog.workerID});setGuideOpen(true);requestAnimationFrame(()=>document.getElementById(guideID)?.querySelector<HTMLElement>('[data-remote-runner-allow-installed-discovery]')?.focus());}}/>}
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
          onEnable={profile=>{action.clearError();setDialog({kind:'decision',action:'enable',profile});}}
          onAdvanced={(adapter,expectedWorkerID)=>{if(writeBlocked||!activeWorkspace.current||pairingCaller.current.api!==api||pairingCaller.current.generation!==pairingGeneration)return;action.clearError();if(!openToolPreparation(adapter,expectedWorkerID))setDialog({kind:'create',adapter,resumeQuick:true});}}
          onWorker={()=>{action.clearError();setDialog({kind:'worker'});}}
        />}
        {dialog?.kind === "worker" && <>
          {publicDraft&&<button type="button" className="text-button" data-tools-return-quick disabled={writeBlocked} onClick={openQuickConnect}>{t('tool-workspace.return_connect')}</button>}
          <div className="tool-action-bar">
            <button type="button" data-worker-mode="manual" aria-pressed={dialog.mode!=='pairing'} disabled={writeBlocked} onClick={()=>{if(!writeBlocked)setDialog({kind:'worker',mode:'manual'});}}>{t('runner-pairing.mode_manual')}</button>
            <button type="button" data-worker-mode="pairing" aria-pressed={dialog.mode==='pairing'} disabled={writeBlocked} onClick={openRunnerPairing}>{t('runner-pairing.mode_remote')}</button>
          </div>
          {dialog.mode==='pairing'?<RunnerPairingPanel project={project} disabled={pending||action.busy} onLockChange={setPairingLocked} onPaired={continueWithPairedRunner}/>:<WorkerForm
            project={project}
            action={action}
            onSaved={(value) => saved("/release-workers", value)}
            onCancel={() => setDialog(undefined)}
          />}
        </>}
        {dialog?.kind === "decision" && (
          <ProfileDecisionForm
            project={project}
            value={dialog.profile}
            decision={dialog.action}
            action={action}
            onSaved={(value) =>
              saved("/tool-profiles/" + value.id + "/commands", value)
            }
            onCancel={() => setDialog(undefined)}
          />
        )}
        {dialog?.kind === "revoke-worker" && (
          <form
            className="tool-form"
            onSubmit={async (event) => {
              event.preventDefault();
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
            <div className="dialog-actions">
              <button
                type="button"
                disabled={action.busy || pending}
                onClick={() => setDialog(undefined)}
              >
                {t("development.workflow.cancel")}</button>
              <button
                className="primary"
                disabled={action.busy || pending || action.conflict}
              >
                {t("development.tools.confirm_worker_credential_revocation")}</button>
            </div>
          </form>
        )}
      </Modal>
      <Modal
        open={!!credential}
        onClose={() => {
          setCredential(undefined);
          setRevealed(false);
        }}
        title={t("development.tools.execution_worker_credential_for_this_registration")}
        description={t("development.tools.the_original_value_remains_only_in_this")}
      >
        {credential && (
          <div className="tool-token">
            <strong>{credential.worker_id}</strong>
            {credential.token ? (
              <>
                <p className="warning">
                  {t("development.tools.save_this_credential_in_the_controlled_runner")}</p>
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
              </>
            ) : (
              <p className="warning">
                {t("development.tools.the_original_registration_receipt_was_recovered_but")}</p>
            )}
            <button
              onClick={() => {
                setCredential(undefined);
                setRevealed(false);
              }}
            >
              {t("development.tools.close_and_clear_the_original_value")}</button>
          </div>
        )}
      </Modal>
    </div>
  );
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
  onSaved,
  onCancel,
}: {
  project: string;
  action: ToolAction;
  onSaved: (value: unknown) => void;
  onCancel: () => void;
}) { useLocale();
  const [worker, setWorker] = useState("");
  return (
    <form
      className="tool-form"
      onSubmit={async (event) => {
        event.preventDefault();
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
          disabled={action.busy || !!action.pending || action.conflict}
        >
          {t("development.tools.register_and_get_this_credential")}</button>
      </div>
    </form>
  );
}
function ProfileDecisionForm({
  project,
  value,
  decision,
  action,
  onSaved,
  onCancel,
}: {
  project: string;
  value: ToolProfile;
  decision: "enable" | "pause" | "revoke";
  action: ToolAction;
  onSaved: (value: ToolProfile) => void;
  onCancel: () => void;
}) { const api=useApi(); useLocale();
  const [current, setCurrent] = useState(value),
    [confirmed, setConfirmed] = useState(false),
    [error, setError] = useState<unknown>("");
  return (
    <form
      className="tool-form"
      onSubmit={async (event) => {
        event.preventDefault();
        const result = await action.run<ToolProfile>(
          "/tool-profiles/" + encodeURIComponent(current.id) + "/commands",
          {
            project_id: project,
            action: decision,
            expected_version: current.version,
          },
        );
        if (result) onSaved(result);
      }}
    >
      <ToolProfileFacts profile={current} />
      {decision==='enable'&&current.profile.adapter==='qoder'&&current.profile.protocol==='acp-stdio-v1'&&<p className="muted">{t('acp-runtime.setup_policy')}</p>}
      {!!error && <ErrorNotice error={renderDiagnostic(error)} />}
      <p className={decision === "enable" ? "muted" : "warning"}>
        {decision === "enable"
          ? t("development.tools.enabling_still_requires_matching_current_identity_live")
          : decision === "pause"
            ? t("development.tools.pausing_blocks_subsequent_dispatch_only_operations_already")
            : t('tool-workspace.remove_hint')}
      </p>
      <label className="check">
        <input
          type="checkbox"
          checked={confirmed}
          disabled={action.busy || !!action.pending}
          onChange={(event) => setConfirmed(event.target.checked)}
        />
        {decision==='revoke'?t('tool-workspace.remove_confirm',{version:current.version}):t("development.tools.confirm_"+decision,{version:current.version})}
      </label>
      {action.conflict && (
        <div className="conflict-banner">
          <p>{t("development.tools.the_state_changed_your_decision_is_retained")}</p>
          <button
            type="button"
            onClick={async () => {
              try {
                setCurrent(
                  await api<ToolProfile>(
                    "/tool-profiles/" +
                      encodeURIComponent(current.id) +
                      query(project),
                  ),
                );
                setConfirmed(false);
                action.clearError();
              } catch (error) {
                setError(error);
              }
            }}
          >
            {t("development.tools.read_current_configuration_keep_decision")}</button>
        </div>
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
          disabled={
            action.busy || !!action.pending || action.conflict || !confirmed
          }
        >
          {decision === "enable"
            ? t("development.tools.confirm_enabling_this_configuration")
            : decision === "pause"
              ? t("development.tools.confirm_pausing_this_configuration")
              : t('tool-workspace.remove')}
        </button>
      </div>
    </form>
  );
}
