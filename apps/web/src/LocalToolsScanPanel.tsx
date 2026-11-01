import {useEffect,useRef,useState} from 'react';
import {CircleHelp,Monitor,RefreshCw,Search,Server} from 'lucide-react';
import type {AdapterDescriptor} from './AdapterConnections';
import {useApi,useApiView} from './api-context';
import {isCursorConnectionGuide,quickConnectAdapters} from './tool-quick-connect-model';
import {toolConnectionDiagnostic} from './tool-connection-diagnostics-model';
import {desktopACPRuntimeForAdapter} from './desktop-acp-connector-model';
import {localToolsScanBridge,readLocalToolsScanResult,validLocalToolsScanScope,type LocalToolsScanScope,type LocalToolsScanResult} from './local-tools-scan-model';
import {tr,useLocale} from './i18n';
import './LocalToolsScanPanel.css';

export type LocalToolsScanPanelProps={project:string;adapters:readonly AdapterDescriptor[];selectedAdapterID?:string;disabled?:boolean;onSelect:(adapter:AdapterDescriptor)=>void;onWorker?:()=>void};
const t=(key:string)=>tr('local-tools-scan.'+key);
const cursorReasons=new Set(['cursor_editor_only','cursor_agent_unverified','cursor_launcher_unsupported','cursor_scan_incomplete']);
function rowGuidance(adapterID:string,code:string){
 if(adapterID==='opencode-sdk'&&code==='version_unsupported')return {reason:tr('opencode-quick-connect.version_unsupported'),hint:'',install:true};
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
function ScanScope({adapters,selectedAdapterID,disabled=false,onSelect,scope,bridge}:LocalToolsScanPanelProps&{scope:LocalToolsScanScope;bridge:NonNullable<ReturnType<typeof localToolsScanBridge>>}){
 const locale=useLocale(),[result,setResult]=useState<Extract<LocalToolsScanResult,{status:'observed'}>>(),[busy,setBusy]=useState(false),[failure,setFailure]=useState(false),[expired,setExpired]=useState(false);
 const active=useRef(true),attempted=useRef(false),inflight=useRef(false),timer=useRef<ReturnType<typeof setTimeout>|undefined>(undefined),generation=useRef(0);
 // StrictMode replays effects on the same instance: preserve its original scan.
 // A real scope remount has separate refs; the old instance remains inactive.
 useEffect(()=>{active.current=true;return()=>{active.current=false;clearTimeout(timer.current);};},[]);
 async function scan(force=false){
  if(disabled||inflight.current)return;inflight.current=true;setBusy(true);setFailure(false);const epoch=++generation.current,startWall=Date.now(),startMono=performance.now();
  try{const raw=await bridge.discoverInstalledTools({action:'scan',project_id:scope.project,user_id:scope.userID,tenant_id:scope.organizationID,locale,...(force?{force:true}:{})});if(!active.current||epoch!==generation.current)return;const next=readLocalToolsScanResult(raw,scope);if(!next||next.status!=='observed'){setResult(undefined);setFailure(true);return;}
   const elapsed=performance.now()-startMono,remaining=elapsed<0?0:Math.max(0,Math.min(60000,Date.parse(next.scan.expires_at)-startWall)-elapsed);clearTimeout(timer.current);setResult(next);setExpired(remaining<=0);if(remaining>0)timer.current=setTimeout(()=>setExpired(true),remaining);
  }catch{if(active.current&&epoch===generation.current){setResult(undefined);setFailure(true);}}
  finally{if(active.current&&epoch===generation.current){inflight.current=false;setBusy(false);}}
 }
 useEffect(()=>{if(!disabled&&!attempted.current){attempted.current=true;void scan();}},[disabled]);
 const eligible=adapters.filter(adapter=>quickConnectAdapters([adapter]).length===1||isCursorConnectionGuide(adapter));
 return <section className="local-tools-scan" data-local-tools-scan aria-label={t('title')} aria-busy={busy}>
  <header><div><h4><Monitor size={18} aria-hidden="true"/>{t('title')}</h4><p>{t('intro')}</p></div><button type="button" data-local-tools-rescan disabled={disabled||busy} onClick={()=>void scan(true)}><RefreshCw size={16} aria-hidden="true"/>{t(busy?'scanning':'rescan')}</button></header>
  <p className="local-tools-scan-status" role="status">{t(failure?'failed':busy?'scanning':result?expired?'expired':result.scan.cached?'cached':'observed':disabled?'deferred':'waiting')}</p>
  {result&&<><ul className="local-tools-scan-list">{result.report.items.map(item=>{const candidates=eligible.filter(adapter=>adapter.id===item.adapter_id),adapter=candidates.length===1?candidates[0]:undefined,guidance=rowGuidance(item.adapter_id,item.reason_code);return <li key={item.adapter_id} data-local-tools-adapter={item.adapter_id}>
   <div className="local-tools-scan-item"><div className="local-tools-scan-name"><strong>{item.product}</strong><span><Search size={14} aria-hidden="true"/>{t(item.installed?'installed':'not_observed')}</span></div><code>{adapter?.protocol||item.adapter_id}</code>{item.version&&<p>{item.version}</p>}
   <p className="local-tools-scan-guidance"><strong>{guidance.reason}</strong> {guidance.hint}</p>
   {item.reason_code&&<details><summary><CircleHelp size={14} aria-hidden="true"/>{t('reason')}</summary><code>{item.reason_code}</code></details>}</div>
   {adapter&&<button type="button" data-local-tools-select={item.adapter_id} aria-pressed={selectedAdapterID===item.adapter_id} disabled={disabled||busy} onClick={()=>onSelect(adapter)}>{t(isCursorConnectionGuide(adapter)?'guide':guidance.install?'install':'configure')}</button>}
  </li>;})}</ul><p className="local-tools-scan-boundary">{t('boundary')}</p><details className="local-tools-scan-times"><summary>{t('observation')}</summary><dl><dt>{t('observed_at')}</dt><dd><time dateTime={result.scan.observed_at}>{result.scan.observed_at}</time></dd><dt>{t('expires_at')}</dt><dd><time dateTime={result.scan.expires_at}>{result.scan.expires_at}</time></dd></dl></details></>}
 </section>;
}
