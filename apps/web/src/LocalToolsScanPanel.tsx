import {useEffect,useRef,useState} from 'react';
import {ChevronRight,CircleHelp,Monitor,RefreshCw,Search,Server} from 'lucide-react';
import type {AdapterDescriptor} from './AdapterConnections';
import {useApi,useApiView} from './api-context';
import {isCursorConnectionGuide,quickConnectAdapters,type InstalledToolFact} from './tool-quick-connect-model';
import {toolConnectionDiagnostic} from './tool-connection-diagnostics-model';
import {desktopACPRuntimeForAdapter} from './desktop-acp-connector-model';
import {localToolsScanBridge,readLocalToolsScanResult,validLocalToolsScanScope,scanAdapterIDs,type LocalToolsScanScope,type LocalToolsScanResult} from './local-tools-scan-model';
import {projectLocalToolsScan} from './local-tools-scan-presentation';
import {hasUnverifiedInstallationStop} from './tool-installation-probe-diagnostic-model';
import {formatNumber,tr,useLocale} from './i18n';
import './LocalToolsScanPanel.css';

export type LocalToolsScanPanelProps={project:string;adapters:readonly AdapterDescriptor[];selectedAdapterID?:string;disabled?:boolean;onSelect:(adapter:AdapterDescriptor)=>void;onAdvanced?:(adapter:AdapterDescriptor)=>void;onWorker?:()=>void;onViewExisting?:(adapter:AdapterDescriptor)=>void};
const t=(key:string,values?:Record<string,unknown>)=>tr('local-tools-scan.'+key,values);
const probeText=(key:string)=>tr('tool-installation-probe-diagnostics.'+key);
const cursorReasons=new Set(['cursor_editor_only','cursor_agent_unverified','cursor_launcher_unsupported','cursor_scan_incomplete']);
function rowGuidance(adapterID:string,code:string){
 if(adapterID==='opencode-sdk'&&code==='version_unsupported')return {reason:tr('opencode-quick-connect.version_unsupported'),hint:'',install:true};
 if(adapterID==='claude-code-sdk'&&code==='version_unsupported')return {reason:tr('claude-sdk-quick-connect.version_unsupported'),hint:'',install:true};
 if(adapterID==='cursor-cli'&&cursorReasons.has(code))return {reason:tr('cursor-connection.reason_'+code),hint:tr('cursor-connection.reason_'+code+'_hint'),install:false};
 const diagnostic=toolConnectionDiagnostic(desktopACPRuntimeForAdapter(adapterID)&&code==='acp_configuration_required'?'configuration_required':code),reason=diagnostic?.reason??'unknown';
 return {reason:tr('tool-connection-diagnostics.reason.'+reason),hint:tr('tool-connection-diagnostics.hint.'+reason),install:['not_installed','unsupported_launcher','invalid_package','node_unavailable'].includes(reason)};
}
export function LocalToolsScanPanel(props:LocalToolsScanPanelProps){
 useLocale();const api=useApi(),view=useApiView(),bridge=localToolsScanBridge(),owner=useRef({api,method:bridge?.discoverInstalledTools,epoch:0});
 if(owner.current.api!==api||owner.current.method!==bridge?.discoverInstalledTools)owner.current={api,method:bridge?.discoverInstalledTools,epoch:owner.current.epoch+1};
 const scope={project:props.project,userID:view?.userID??'',organizationID:view?.organizationID??''};
 if(!bridge)return <section className="local-tools-scan" aria-label={t('title')}><h4><Monitor size={18} aria-hidden="true"/>{t('title')}</h4><p>{t('web_hint')}</p>{props.onWorker&&<button type="button" disabled={props.disabled} onClick={props.onWorker}><Server size={16} aria-hidden="true"/>{t('worker')}</button>}</section>;
 if(!validLocalToolsScanScope(scope))return null;
 return <ScanScope key={JSON.stringify(scope)+owner.current.epoch} {...props} scope={scope} bridge={bridge}/>;
}
function ScanScope({adapters,selectedAdapterID,disabled=false,onSelect,onAdvanced,onWorker,onViewExisting,scope,bridge}:LocalToolsScanPanelProps&{scope:LocalToolsScanScope;bridge:NonNullable<ReturnType<typeof localToolsScanBridge>>}){
 const locale=useLocale(),[result,setResult]=useState<Extract<LocalToolsScanResult,{status:'observed'}>>(),[busy,setBusy]=useState(false),[failure,setFailure]=useState(false),[expired,setExpired]=useState(false);
 const active=useRef(true),attempted=useRef(false),inflight=useRef(false),timer=useRef<ReturnType<typeof setTimeout>|undefined>(undefined),generation=useRef(0);
 const panel=useRef<HTMLElement>(null),focusIntent=useRef<{adapterID:string;button:HTMLButtonElement}|undefined>(undefined);
 // StrictMode replays effects on the same instance: preserve its original scan.
 // A real scope remount has separate refs; the old instance remains inactive.
 useEffect(()=>{active.current=true;return()=>{active.current=false;clearTimeout(timer.current);};},[]);
 async function scan(force=false){
  if(disabled||inflight.current||hasUnverifiedInstallationStop(result?.report.items??[]))return;inflight.current=true;setBusy(true);setFailure(false);const epoch=++generation.current,startWall=Date.now(),startMono=performance.now();
  try{const raw=await bridge.discoverInstalledTools({action:'scan',project_id:scope.project,user_id:scope.userID,tenant_id:scope.organizationID,locale,...(force?{force:true}:{})});if(!active.current||epoch!==generation.current)return;const next=readLocalToolsScanResult(raw,scope);if(!next||next.status!=='observed'){setResult(undefined);setFailure(true);return;}
   const elapsed=performance.now()-startMono,remaining=elapsed<0?0:Math.max(0,Math.min(60000,Date.parse(next.scan.expires_at)-startWall)-elapsed);clearTimeout(timer.current);setResult(next);setExpired(remaining<=0);if(remaining>0)timer.current=setTimeout(()=>setExpired(true),remaining);
  }catch{if(active.current&&epoch===generation.current){setResult(undefined);setFailure(true);}}
  finally{if(active.current&&epoch===generation.current){inflight.current=false;setBusy(false);}}
 }
 useEffect(()=>{if(!disabled&&!attempted.current){attempted.current=true;void scan();}},[disabled]);
 const eligible=adapters.filter(adapter=>quickConnectAdapters([adapter]).length===1||isCursorConnectionGuide(adapter));
 const selected=eligible.find(adapter=>adapter.id===selectedAdapterID);
 const presentation=result?projectLocalToolsScan(result.report.items,selectedAdapterID):undefined;
 const stopBlocked=hasUnverifiedInstallationStop(result?.report.items??[]);
 // Moving a chosen missing row between lists remounts its button. Restore only
 // this panel's explicit focused action; external selections never claim focus.
 useEffect(()=>{const intent=focusIntent.current;focusIntent.current=undefined;if(!intent||intent.adapterID!==selectedAdapterID||!active.current||!panel.current||disabled)return;
  const document=panel.current.ownerDocument,current=document.activeElement;
  if(current!==intent.button&&!(current===document.body&&!intent.button.isConnected))return;
  const target=[...panel.current.querySelectorAll<HTMLButtonElement>('[data-local-tools-select]')].find(button=>button.dataset.localToolsSelect===intent.adapterID);target?.focus();
 },[selectedAdapterID]);
 function row(item:InstalledToolFact){const candidates=eligible.filter(adapter=>adapter.id===item.adapter_id),adapter=candidates.length===1?candidates[0]:undefined,guidance=rowGuidance(item.adapter_id,item.reason_code),diagnostic=item.diagnostic;return <li key={item.adapter_id} data-local-tools-adapter={item.adapter_id}>
  <div className="local-tools-scan-item"><div className="local-tools-scan-name"><strong>{item.product}</strong><span><Search size={14} aria-hidden="true"/>{item.reason_code==='scan_incomplete'?probeText('scan_incomplete'):t(item.installed?expired?'installation_recorded':'installed':'not_observed')}</span></div>
  <p className="local-tools-scan-guidance" {...(diagnostic?{'data-tool-probe-diagnostic':diagnostic.code}:{})}><strong>{diagnostic?probeText('code.'+diagnostic.code):guidance.reason}</strong></p>
  <details data-local-tools-basis={item.adapter_id}><summary><CircleHelp size={14} aria-hidden="true"/>{t('reason')}</summary>{diagnostic?<p>{probeText('hint.'+diagnostic.next_action)}</p>:guidance.hint&&<p>{guidance.hint}</p>}<dl>{diagnostic&&<><dt>{probeText('stage_label')}</dt><dd>{probeText('stage.'+diagnostic.stage)}</dd></>}<dt>{t('protocol')}</dt><dd><code>{adapter?.protocol||item.adapter_id}</code></dd>{item.version&&<><dt>{t('version')}</dt><dd>{item.version}</dd></>}<dt>{t('reason_code')}</dt><dd><code>{item.reason_code}</code></dd></dl></details></div>
  {(adapter||diagnostic?.next_action==='scan')&&<div className="local-tools-scan-actions">{adapter&&onViewExisting&&<button type="button" className="text-button" data-local-tools-existing={item.adapter_id} disabled={disabled||busy} onClick={()=>{if(active.current&&!disabled&&!inflight.current)onViewExisting(adapter);}}>{t('view_existing')}</button>}{diagnostic?diagnostic.next_action==='guide'&&adapter&&onAdvanced?<button type="button" data-local-tools-diagnostic-action="guide" disabled={disabled||busy} onClick={()=>{if(active.current&&!disabled&&!inflight.current)onAdvanced(adapter);}}>{probeText('action.guide')}</button>:diagnostic.next_action==='scan'?<button type="button" data-local-tools-diagnostic-action="scan" disabled={disabled||busy||stopBlocked} onClick={()=>void scan(true)}>{probeText('action.local_scan')}</button>:null:adapter&&<button type="button" data-local-tools-select={item.adapter_id} aria-pressed={selectedAdapterID===item.adapter_id} disabled={disabled||busy} onClick={event=>{if(active.current&&!disabled&&!inflight.current){focusIntent.current=selectedAdapterID!==item.adapter_id&&event.currentTarget.ownerDocument.activeElement===event.currentTarget?{adapterID:item.adapter_id,button:event.currentTarget}:undefined;onSelect(adapter);}}}>{t(isCursorConnectionGuide(adapter)?'guide':guidance.install?'install':'configure')}</button>}</div>}
 </li>;}
 return <section ref={panel} className="local-tools-scan" data-local-tools-scan aria-label={t('title')} aria-busy={busy}>
  <header><div><h4><Monitor size={18} aria-hidden="true"/>{t('title')}</h4><p>{t('intro')}</p></div><button type="button" data-local-tools-rescan disabled={disabled||busy||stopBlocked} onClick={()=>void scan(true)}><RefreshCw size={16} aria-hidden="true"/>{t(busy?'scanning':'rescan')}</button></header>
  <p className="local-tools-scan-status" role="status">{t(failure?'failed':busy?'scanning':result?expired?'expired':result.scan.cached?'cached':'observed':disabled?'deferred':'waiting')}</p>
  {selected&&<p className="local-tools-scan-target" data-local-tools-selected-target>{t('selected_target',{tool:selected.product})}</p>}
  <p className="local-tools-scan-boundary">{t('scope_hint',{count:formatNumber(scanAdapterIDs.length)})}</p>
  {result&&presentation&&<><p className="local-tools-scan-counts" data-local-tools-installation-summary>{t('installation_summary',{detected:formatNumber(presentation.detectedCount),attention:formatNumber(presentation.attentionCount)})}</p>
   {presentation.primary.length>0?<ul className="local-tools-scan-list" data-local-tools-primary>{presentation.primary.map(row)}</ul>:<div className="local-tools-scan-empty" data-local-tools-empty><p>{t('empty_primary')}</p>{onWorker&&<button type="button" data-local-tools-worker disabled={disabled||busy} onClick={()=>{if(active.current&&!disabled&&!inflight.current)onWorker();}}><Server size={16} aria-hidden="true"/>{t('worker')}</button>}</div>}
   {presentation.other.length>0&&<details className="local-tools-scan-other" data-local-tools-other><summary><ChevronRight className="local-tools-scan-disclosure" size={16} aria-hidden="true"/>{t('other_tools',{count:formatNumber(presentation.other.length)})}</summary><ul className="local-tools-scan-list">{presentation.other.map(row)}</ul></details>}
   <p className="local-tools-scan-boundary">{t('boundary')}</p><details className="local-tools-scan-times"><summary>{t('observation')}</summary><dl><dt>{t('observed_at')}</dt><dd><time dateTime={result.scan.observed_at}>{result.scan.observed_at}</time></dd><dt>{t('expires_at')}</dt><dd><time dateTime={result.scan.expires_at}>{result.scan.expires_at}</time></dd></dl></details></>}
 </section>;
}
