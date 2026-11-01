import {useCallback,useEffect,useId,useLayoutEffect,useRef,useState} from 'react';
import {ClipboardCheck,RefreshCw} from 'lucide-react';
import {useApi,useApiView} from './api-context';
import {ApiError,type ApiCaller} from './api';
import {formatDateTime,formatNumber,tr,useLocale} from './i18n';
import {toolError} from './tool-hooks';
import type {ToolObservation} from './tool-model';
import {parseToolDiscovery,type ToolDiscoveryView} from './tool-discovery-model';
import {ErrorNotice,Loading} from './ui';
import './ToolDiscoveryPanel.css';

export type ToolDiscoveryAdoption=Readonly<{project:string;worker:string;apiCaller:ApiCaller;observedAt:string;expiresAt:string;deadline:number}>;
export type ToolDiscoveryPanelProps={project:string;worker:string;authorized:boolean;disabled:boolean;autoAdopt:boolean;manualRevision?:number;readRevision?:number;onAdopt:(items:ToolObservation[],source:ToolDiscoveryAdoption)=>void};
const t=(key:string,values?:Record<string,unknown>)=>tr('tool-discovery.'+key,values);
type Snapshot={view:ToolDiscoveryView;deadline:number;reportKey:string;apiCaller:ApiCaller};

export function ToolDiscoveryPanel(props:ToolDiscoveryPanelProps){
 const api=useApi(),view=useApiView(),owner=useRef({api,epoch:0});
 if(owner.current.api!==api)owner.current={api,epoch:owner.current.epoch+1};
 // Identity changes remove the old read before it can render or be adopted.
 return <DiscoveryScope key={JSON.stringify([view?.userID,view?.organizationID,props.project,props.worker,owner.current.epoch])} {...props}/>;
}

function DiscoveryScope({project,worker,authorized,disabled,autoAdopt,manualRevision=0,readRevision=0,onAdopt}:ToolDiscoveryPanelProps){
 useLocale();const api=useApi(),headingID=useId(),epoch=useRef(0),controller=useRef<AbortController|undefined>(undefined),callback=useRef(onAdopt),adopted=useRef(new Set<string>());
 const [storedSnapshot,setSnapshot]=useState<Snapshot>(),[loading,setLoading]=useState(false),[error,setError]=useState<unknown>(),[expired,setExpired]=useState(false),[usedReport,setUsedReport]=useState('');
 const [accessRefused,setAccessRefused]=useState(!authorized);
 const active=!!project&&!!worker&&authorized;
 const snapshot=active&&!accessRefused&&storedSnapshot?.apiCaller===api?storedSnapshot:undefined;
 useLayoutEffect(()=>{callback.current=onAdopt;},[onAdopt]);
 useLayoutEffect(()=>{adopted.current.clear();setUsedReport('');},[api]);
 useEffect(()=>{setUsedReport('');},[manualRevision]);
 useLayoutEffect(()=>{
  if(authorized)return;
  ++epoch.current;controller.current?.abort();setAccessRefused(true);setSnapshot(undefined);setError(undefined);setLoading(false);setExpired(false);setUsedReport('');adopted.current.clear();
 },[authorized]);
 const read=useCallback(async()=>{
  if(!active)return;
  const generation=++epoch.current;controller.current?.abort();const abort=new AbortController();controller.current=abort;
  setSnapshot(undefined);setError(undefined);setExpired(false);setLoading(true);
  const started=performance.now();
  try{
   const raw=await api<unknown>('/tool-discovery?project_id='+encodeURIComponent(project)+'&worker_id='+encodeURIComponent(worker),undefined,'GET',undefined,abort.signal);
   if(generation!==epoch.current||abort.signal.aborted)return;
   const view=parseToolDiscovery(raw,project,worker),now=performance.now();
   // Count request travel conservatively; the user's wall clock is not the
   // authority for a database report's remaining lifetime.
   const remaining=Math.max(0,view.remainingMs-Math.max(0,now-started));
   setAccessRefused(false);
   setExpired(view.state==='fresh'&&remaining<=0);
   setSnapshot({view,deadline:now+remaining,reportKey:JSON.stringify([view.observedAt,view.items]),apiCaller:api});
  }catch(problem){if(generation===epoch.current&&!abort.signal.aborted){if(problem instanceof ApiError&&[401,403,404].includes(problem.status))setAccessRefused(true);setSnapshot(undefined);setError(problem);}}
  finally{if(generation===epoch.current&&!abort.signal.aborted)setLoading(false);}
 },[active,api,project,worker]);
 useEffect(()=>{void read();return()=>{epoch.current++;controller.current?.abort();};},[read,readRevision]);
 useEffect(()=>{
  if(snapshot?.view.state!=='fresh')return;
  const timeout=setTimeout(()=>setExpired(true),Math.max(0,snapshot.deadline-performance.now()));
  return()=>clearTimeout(timeout);
 },[snapshot]);
 const usable=active&&!disabled&&!loading&&!expired&&snapshot?.view.state==='fresh';
 function adopt(automatic:boolean){
  if(!usable||!snapshot||!snapshot.view.observedAt||!snapshot.view.expiresAt||performance.now()>=snapshot.deadline||(automatic?adopted.current.has(snapshot.reportKey):usedReport===snapshot.reportKey))return;
  adopted.current.add(snapshot.reportKey);setUsedReport(snapshot.reportKey);callback.current(snapshot.view.items,{project,worker,apiCaller:snapshot.apiCaller,observedAt:snapshot.view.observedAt,expiresAt:snapshot.view.expiresAt,deadline:snapshot.deadline});
 }
 useEffect(()=>{if(autoAdopt)adopt(true);},[autoAdopt,usable,snapshot]);
 const view=snapshot?.view,state=view?.state==='fresh'&&expired?'expired':view?.state;
 return <section className="tool-discovery" aria-labelledby={headingID} aria-busy={loading}>
  <div className="tool-discovery-heading"><h4 id={headingID}><ClipboardCheck size={16} aria-hidden="true"/>{t('title')}</h4>
   {active&&<button type="button" data-tool-discovery-refresh disabled={disabled||loading} onClick={()=>void read()}><RefreshCw size={14} aria-hidden="true"/>{t('refresh')}</button>}
  </div>
  <p className="tool-discovery-intro">{t('intro')}</p>
  {active&&!accessRefused&&<p className="tool-discovery-target" data-tool-discovery-target><span>{t('target')}</span><code>{worker}</code></p>}
  <p className="tool-discovery-boundary">{t('read_boundary')}</p>
  {!active&&<p role="status">{t(worker?'authorization_required':'choose_worker')}</p>}
  {loading&&<Loading/>}
  {!!error&&<div className="tool-discovery-failure"><ErrorNotice error={toolError(error)}/><button type="button" data-tool-discovery-retry disabled={disabled||loading} onClick={()=>void read()}>{t('retry')}</button></div>}
  {view&&<div className="tool-discovery-result" data-tool-discovery-state={state}>
   <div role="status" aria-live="polite">
    {state==='fresh'?<>
     <p className="tool-discovery-count">{t('fresh_count',{count:formatNumber(view.items.length),available:formatNumber(view.items.filter(item=>item.available).length)})}</p>
     {view.items.length===0?<p data-tool-discovery-empty>{t('fresh_empty')}</p>:!view.items.some(item=>item.available)&&<p>{t('none_available')}</p>}
    </>:<p>{t(state==='expired'?'expired':state==='stale'?'stale':'unreported')}</p>}
   </div>
   {view.observedAt&&<details className="tool-discovery-basis" data-tool-discovery-basis><summary>{t('basis')}</summary><dl><dt>{t('project')}</dt><dd><code>{project}</code></dd><dt>{t('target')}</dt><dd><code>{worker}</code></dd><dt>{t('report_time')}</dt><dd className="tool-discovery-observed"><time dateTime={view.observedAt}>{formatDateTime(view.observedAt,{year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',second:'2-digit'})}</time></dd></dl></details>}
   {view.state==='fresh'&&view.items.length>0&&!autoAdopt&&<button type="button" data-tool-discovery-adopt disabled={!usable||usedReport===snapshot?.reportKey} onClick={()=>adopt(false)}>{t(usedReport===snapshot?.reportKey?'adopted':'adopt')}</button>}
   {state==='fresh'&&usedReport===snapshot?.reportKey&&<p className="tool-discovery-adopted">{t('adopted_hint')}</p>}
  </div>}
  <details className="tool-discovery-local"><summary>{t('local_help')}</summary><p>{t('local_help_text')}</p><code>suma-runner --describe-tools</code></details>
 </section>;
}
