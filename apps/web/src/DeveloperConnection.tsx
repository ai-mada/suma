import {useEffect,useRef,useState} from 'react';
import {Copy,RefreshCw,Plug} from 'lucide-react';
import {useApi} from './api-context';
import {query,type ApiCaller} from './api';
import {ErrorNotice,Loading} from './ui';
import {formatServerCatalogMessage,formatNumber,tr,useLocale,UiError} from './i18n';
import {codexConnectionConfig,octopConnectionConfig,connectionMetadata,type DeveloperConnectionMetadata} from './developer-connection-model';
import './developer-connection.css';

type Snapshot={project:string;api:ApiCaller;revision:number;data?:DeveloperConnectionMetadata;error?:unknown};
export function DeveloperConnection({project}:{project:string}){
 const api=useApi(),locale=useLocale(),[client,setClient]=useState<'codex'|'octop'>('codex'),[revision,setRevision]=useState(0),[snapshot,setSnapshot]=useState<Snapshot>({project,api,revision}),[copyState,setCopyState]=useState<'endpoint'|'config'>(),[copyError,setCopyError]=useState<unknown>();
 const epoch=useRef(0),copyEpoch=useRef(0);
 useEffect(()=>{++copyEpoch.current;setCopyState(undefined);setCopyError(undefined);return()=>{++copyEpoch.current;};},[locale,client]);
 useEffect(()=>{
  const current=++epoch.current,controller=new AbortController();setSnapshot({project,api,revision});setCopyState(undefined);setCopyError(undefined);
  if(project)void api<unknown>('/developer-connection'+query(project),undefined,'GET',undefined,controller.signal).then(value=>{
   if(current!==epoch.current)return;
   const data=connectionMetadata(value,project);
   setSnapshot({project,api,revision,...(data?{data}:{error:new UiError('workspace.connection.invalid_response')})});
  }).catch(error=>{if(current===epoch.current)setSnapshot({project,api,revision,error});});
  return()=>{++epoch.current;controller.abort();};
 },[project,api,revision]);
 const current=snapshot.project===project&&snapshot.api===api&&snapshot.revision===revision?snapshot:undefined,data=current?.data,config=data&&(client==='octop'?octopConnectionConfig(data,locale):codexConnectionConfig(data,locale)),configTitle=client==='octop'?'octop-connection.configuration':'workspace.connection.codex';
 async function copy(kind:'endpoint'|'config',value:string){
  const currentEpoch=epoch.current,currentCopy=++copyEpoch.current;setCopyState(undefined);setCopyError(undefined);
  try{await navigator.clipboard.writeText(value);if(currentEpoch===epoch.current&&currentCopy===copyEpoch.current)setCopyState(kind);}
  catch{if(currentEpoch===epoch.current&&currentCopy===copyEpoch.current)setCopyError(new UiError('workspace.connection.clipboard_failed'));}
 }
 if(!project)return null;
 return <section className="detail-section developer-connection" aria-labelledby="developer-connection-title">
  <div className="connection-heading"><h3 id="developer-connection-title"><Plug size={16}/>{tr('workspace.connection.title')}</h3><button type="button" className="icon-button" aria-label={tr('workspace.connection.refresh')} onClick={()=>setRevision(value=>value+1)}><RefreshCw size={15}/></button></div>
  <p className="muted">{tr('workspace.connection.description')}</p>
  {current?.error?<ErrorNotice error={current.error} onRetry={()=>setRevision(value=>value+1)}/>:!data?<Loading/>:<>
   <p className="connection-unverified">{tr('workspace.connection.unverified')}</p>
   <label className="field">{tr('octop-connection.client')}<select data-connection-client value={client} onChange={event=>setClient(event.target.value==='octop'?'octop':'codex')}><option value="codex">{tr('octop-connection.codex')}</option><option value="octop">{tr('octop-connection.octop')}</option></select></label>
   <dl className="connection-facts"><div><dt>{tr('workspace.connection.project')}</dt><dd><code>{data.project_id}</code></dd></div><div><dt>{tr('workspace.connection.endpoint')}</dt><dd><code>{data.endpoint_url||data.endpoint_path}</code>{config&&<button type="button" onClick={()=>void copy('endpoint',data.endpoint_url!)}><Copy size={14}/>{tr(copyState==='endpoint'?'workspace.connection.copied':'workspace.connection.copy_endpoint')}</button>}</dd></div><div><dt>{tr('workspace.connection.source')}</dt><dd>{tr('workspace.connection.source_'+data.endpoint_source)}</dd></div></dl>
   {!config&&<p className="connection-guidance">{tr(client==='octop'&&data.endpoint_source!=='unconfigured'?'octop-connection.unavailable':'workspace.connection.unconfigured')}</p>}
   {config&&<div className="connection-config"><div className="connection-heading"><h4>{tr(configTitle)}</h4><button type="button" onClick={()=>void copy('config',config)}><Copy size={14}/>{tr(copyState==='config'?'workspace.connection.copied':'workspace.connection.copy_config')}</button></div><pre data-codex-config={client==='codex'?'':undefined} data-octop-config={client==='octop'?'':undefined} tabIndex={0} aria-label={tr(configTitle)}><code>{config}</code></pre>{client==='octop'?<><p className="muted">{tr('octop-connection.instructions')}</p><details className="connection-guidance"><summary>{tr('octop-connection.more')}</summary><p className="muted">{tr('octop-connection.credential')}</p><p className="muted">{tr('octop-connection.network')}</p><p className="muted">{tr('octop-connection.boundary')}</p></details></>:<p className="muted">{tr('workspace.connection.environment')}</p>}<p className="muted">{tr('workspace.connection.context')}</p></div>}
   <details className="connection-tools"><summary>{tr('workspace.connection.tools',{count:formatNumber(data.tools.length)})}</summary><p className="muted">{tr('workspace.connection.declaration')}</p>{data.tools.length?<div className="connection-table"><table><thead><tr><th scope="col">{tr('workspace.connection.tool')}</th><th scope="col">{tr('workspace.connection.effect')}</th><th scope="col">{tr('workspace.connection.scope')}</th></tr></thead><tbody>{data.tools.map(tool=><tr key={tool.name}><td><code>{tool.mcp_name}</code><small><code>{tool.name}</code></small><p>{tool.description_key?formatServerCatalogMessage(tool.description_key,[],{})??tool.description:tool.description}</p></td><td><span className={'connection-effect'+(tool.read_only?'':' draft')}>{tr(tool.read_only?'workspace.connection.read':'workspace.connection.draft')}</span></td><td>{tool.credential_scopes.map(scope=>tr('workspace.connection.scope_'+scope)).join(' · ')}</td></tr>)}</tbody></table></div>:<p>{tr('workspace.connection.empty')}</p>}</details>
   <p className="connection-guidance">{tr('workspace.connection.permission_boundary')}</p>
  </>}
  {current?.data&&Boolean(copyError)&&<ErrorNotice error={copyError}/>}
  <span className="connection-copy-status" role="status">{current?.data&&copyState?tr('workspace.connection.copied'):''}</span>
 </section>;
}
