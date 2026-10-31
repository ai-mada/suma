import {useEffect,useId,useRef,useState} from 'react';
import {ExternalLink,Info,Monitor,RefreshCw} from 'lucide-react';
import type {ApiCaller} from './api';
import {useApi,useApiView} from './api-context';
import {formatDateTime,tr,useLocale} from './i18n';
import {readInstalledToolDiscovery,type InstalledToolFact} from './tool-quick-connect-model';
import './CursorConnectionGuide.css';

export type CursorConnectionGuideProps={project:string;disabled?:boolean};
export const CURSOR_INSTALL_GUIDE_URL='https://cursor.com/docs/cli/installation';
type Bridge={platform:string;version:'1.0.0';discoverInstalledTools:(request:Record<string,string>)=>Promise<unknown>};
type Snapshot={caller:ApiCaller;bridge:Bridge;discover:Bridge['discoverInstalledTools'];fact:InstalledToolFact;receivedAt:string};
const t=(key:string)=>tr('cursor-connection.'+key);
const scopeID=(value:unknown)=>typeof value==='string'&&/^[a-f0-9]{32}$/.test(value);
function installedBridge():Bridge|undefined{
 const raw=(window as unknown as {sumaDesktop?:unknown}).sumaDesktop;
 if(!raw||typeof raw!=='object'||Array.isArray(raw))return;
 const value=raw as Record<string,unknown>;
 return ['win32','darwin','linux'].includes(String(value.platform))&&value.version==='1.0.0'&&typeof value.discoverInstalledTools==='function'?value as Bridge:undefined;
}

/** A passive local observation never constructs a Runner command or profile. */
export function CursorConnectionGuide(props:CursorConnectionGuideProps){
 const view=useApiView();return <ScopedCursorGuide key={JSON.stringify([props.project,view?.userID,view?.organizationID])} {...props}/>;
}
function ScopedCursorGuide({project,disabled=false}:CursorConnectionGuideProps){
 const locale=useLocale(),api=useApi(),view=useApiView(),id=useId(),bridge=installedBridge(),discover=bridge?.discoverInstalledTools;
 const [snapshot,setSnapshot]=useState<Snapshot>(),[busy,setBusy]=useState(false),[failed,setFailed]=useState(false);
 const alive=useRef(true),epoch=useRef(0),requestBusy=useRef(false),caller=useRef(api),activeBridge=useRef(bridge),activeDiscover=useRef(discover);
 caller.current=api;activeBridge.current=bridge;activeDiscover.current=discover;
 const validScope=!!view&&[project,view.userID,view.organizationID].every(scopeID);
 const current=snapshot?.caller===api&&snapshot.bridge===bridge&&snapshot.discover===discover?snapshot:undefined;
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;++epoch.current;};},[]);
 useEffect(()=>{++epoch.current;requestBusy.current=false;setBusy(false);setSnapshot(undefined);setFailed(false);return()=>{++epoch.current;};},[api,bridge,discover]);
 const live=(generation:number)=>alive.current&&generation===epoch.current&&caller.current===api&&activeBridge.current===bridge&&activeDiscover.current===discover;
 async function scan(){
  if(disabled||requestBusy.current||!bridge||!discover||!view||!validScope)return;
  const generation=++epoch.current;requestBusy.current=true;setBusy(true);setSnapshot(undefined);setFailed(false);
  try{
   const result=await discover.call(bridge,{project_id:project,user_id:view.userID,tenant_id:view.organizationID,adapter_id:'cursor-cli',locale});
   if(!live(generation))return;
   const fact=readInstalledToolDiscovery(result,'cursor-cli');
   if(!fact){setFailed(true);return;}
   setSnapshot({caller:api,bridge,discover,fact,receivedAt:new Date().toISOString()});
  }catch{if(live(generation))setFailed(true);}
  finally{if(live(generation)){requestBusy.current=false;setBusy(false);}}
 }
 const official=(primary:boolean)=><a className={primary?'primary cursor-guide-primary':'cursor-guide-link'} href={CURSOR_INSTALL_GUIDE_URL} target="_blank" rel="noopener noreferrer" {...(primary?{'data-cursor-browser-guide':true}:{'data-cursor-official-guide':true})}>{t('official_guide')}<ExternalLink size={15} aria-hidden="true"/></a>;
 return <section className="cursor-connection-guide" data-cursor-guide aria-labelledby={id+'-title'} aria-busy={busy}>
  <header><Monitor size={20} aria-hidden="true"/><div><h3 id={id+'-title'}>{t('title')}</h3><p>{t('intro')}</p></div></header>
  <p className="cursor-guide-scope">{t('scope')}</p>
  <div className="cursor-guide-execution"><Info size={17} aria-hidden="true"/><div><strong>{t('execution')}</strong><span>{t('execution_unavailable')}</span><p>{t('account_boundary')}</p></div></div>
  {bridge?<>
   <p>{t('passive_boundary')}</p>
   <div className="cursor-guide-actions"><button type="button" className="primary cursor-guide-primary" data-cursor-scan disabled={disabled||busy||!validScope} onClick={()=>void scan()}>{busy?<RefreshCw size={16} aria-hidden="true"/>:<Monitor size={16} aria-hidden="true"/>}{t(busy?'scanning':current||failed?'rescan':'scan')}</button>{official(false)}</div>
   {!validScope&&<p role="status" className="cursor-guide-muted">{t('scope_unavailable')}</p>}
   {busy&&<p role="status" className="cursor-guide-muted">{t('scanning')}</p>}
   {failed&&<p role="alert" className="cursor-guide-error">{t('check_failed')}</p>}
   {current&&<div className="cursor-guide-observation" data-cursor-reason={current.fact.reason_code} role="status"><strong>{t('reason_'+current.fact.reason_code)}</strong><p>{t('reason_'+current.fact.reason_code+'_hint')}</p><div className="cursor-guide-received"><span>{t('observed_at')}</span><time data-cursor-observed-at dateTime={current.receivedAt}>{formatDateTime(current.receivedAt,{dateStyle:'medium',timeStyle:'short'})}</time></div><p className="cursor-guide-muted">{t('observed_clock_hint')}</p>
    <details><summary>{t('details')}</summary><p>{t('scope_detail')}</p>{current.fact.executable&&<dl><dt>{t('path')}</dt><dd><code data-cursor-executable>{current.fact.executable}</code></dd></dl>}</details>
   </div>}
  </>:<div className="cursor-guide-browser"><p>{t('browser_hint')}</p>{official(true)}</div>}
 </section>;
}
