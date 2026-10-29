import {useEffect,useId,useRef,useState} from 'react';
import {Check,Copy,Download,ExternalLink,Plug,RefreshCw} from 'lucide-react';
import {useApi,useApiView} from './api-context';
import {query,type ApiCaller} from './api';
import {ErrorNotice,Loading} from './ui';
import {tr,useLocale,UiError} from './i18n';
import {connectionMetadata,projectConnectionEndpoint,type DeveloperConnectionMetadata} from './developer-connection-model';
import {developerClientArtifact,developerClients,type DeveloperClientID} from './developer-client-connections-model';
import './DeveloperClientConnections.css';

type Snapshot={project:string;api:ApiCaller;revision:number;data?:DeveloperConnectionMetadata;error?:unknown};
export function DeveloperClientConnections({project}:{project:string}){
 const api=useApi(),view=useApiView(),locale=useLocale(),id=useId();
 const [open,setOpen]=useState(false),[client,setClient]=useState<DeveloperClientID>('cursor'),[revision,setRevision]=useState(0),[snapshot,setSnapshot]=useState<Snapshot>(),[feedback,setFeedback]=useState<'copied'|'downloaded'|'manual'>();
 const epoch=useRef(0),action=useRef(0),currentContext=useRef(''),fallback=useRef<HTMLTextAreaElement>(null);
 const user=view?.userID,organization=view?.organizationID;
 useEffect(()=>{
  const current=++epoch.current,controller=new AbortController();setSnapshot(undefined);setFeedback(undefined);++action.current;
  if(open&&project)void api<unknown>('/developer-connection'+query(project),undefined,'GET',undefined,controller.signal).then(value=>{
   if(current!==epoch.current)return;
   const data=connectionMetadata(value,project),matches=data&&(!user||data.expected_user_id===user)&&(!organization||data.expected_organization_id===organization);
   setSnapshot({project,api,revision,...(matches?{data}:{error:new UiError('developer-clients.invalid_response')})});
  }).catch(error=>{if(current===epoch.current)setSnapshot({project,api,revision,error});});
  return()=>{++epoch.current;++action.current;controller.abort();};
 },[open,project,api,revision,user,organization]);
 useEffect(()=>{++action.current;setFeedback(undefined);},[client,locale]);
 useEffect(()=>{if(feedback==='manual'){fallback.current?.focus();fallback.current?.select();}},[feedback]);
 const current=open&&snapshot?.project===project&&snapshot.api===api&&snapshot.revision===revision?snapshot:undefined;
 const data=current?.data,definition=developerClients.find(item=>item.id===client)!,artifact=data?developerClientArtifact(client,data,locale):undefined,endpoint=data?projectConnectionEndpoint(data):undefined;
 const context=JSON.stringify([project,user,organization,client,locale,revision,artifact?.content]);currentContext.current=context;
 async function copy(){
  if(!artifact)return;
  const operation=++action.current,original=context;setFeedback(undefined);
  try{await navigator.clipboard.writeText(artifact.content);if(operation===action.current&&original===currentContext.current)setFeedback('copied');}
  catch{if(operation===action.current&&original===currentContext.current)setFeedback('manual');}
 }
 function download(){
  if(!artifact)return;
  ++action.current;setFeedback(undefined);
  let url:string|undefined;
  try{url=URL.createObjectURL(new Blob([artifact.content],{type:artifact.mime}));const link=document.createElement('a');link.href=url;link.download=artifact.filename;link.click();setFeedback('downloaded');}
  catch{setFeedback('manual');}
  finally{if(url)URL.revokeObjectURL(url);}
 }
 if(!project)return null;
 return <details className="developer-clients" data-developer-clients open={open} onToggle={event=>setOpen(event.currentTarget.open)}>
  <summary><Plug size={16}/><span>{tr('developer-clients.title')}</span></summary>
  {open&&<div className="developer-clients-body">
   <p className="muted">{tr('developer-clients.intro')}</p>
   <div className="developer-clients-chooser"><label htmlFor={id+'-client'}>{tr('developer-clients.client')}<select id={id+'-client'} value={client} onChange={event=>setClient(event.target.value as DeveloperClientID)}>{developerClients.map(item=><option key={item.id} value={item.id}>{item.name}</option>)}</select></label><a href={definition.docs} target="_blank" rel="noreferrer">{tr('developer-clients.official_docs')}<ExternalLink size={13}/></a></div>
   {current?.error?<ErrorNotice error={current.error} onRetry={()=>setRevision(value=>value+1)}/>:!data?<Loading/>:<>
    <div className="developer-clients-state"><span>{tr('developer-clients.unverified')}</span><button type="button" onClick={()=>setRevision(value=>value+1)} aria-label={tr('developer-clients.refresh')}><RefreshCw size={13}/>{tr('developer-clients.refresh')}</button></div>
    <dl className="developer-clients-facts"><div><dt>{tr('developer-clients.project')}</dt><dd><code>{data.project_id}</code></dd></div><div><dt>{tr('developer-clients.endpoint')}</dt><dd><code>{endpoint||data.endpoint_path}</code></dd></div></dl>
    {definition.exportable?<>
     <ol className="developer-clients-steps"><li>{tr('developer-clients.credential')}</li><li>{tr('developer-clients.merge',{path:definition.target})}</li><li>{tr('developer-clients.verify')}</li></ol>
     {artifact?<>
      <div className="developer-clients-actions"><button type="button" data-client-copy onClick={()=>void copy()}>{feedback==='copied'?<Check size={14}/>:<Copy size={14}/>} {tr(feedback==='copied'?'developer-clients.copied':'developer-clients.copy')}</button><button type="button" data-client-download onClick={download}><Download size={14}/>{tr('developer-clients.download')}</button></div>
      <details className="developer-clients-preview"><summary>{tr('developer-clients.preview')}</summary><pre tabIndex={0} aria-label={tr('developer-clients.preview')} data-client-config>{artifact.content}</pre></details>
      {feedback==='manual'&&<div role="alert" className="developer-clients-manual"><label htmlFor={id+'-manual'}>{tr('developer-clients.manual_copy')}</label><textarea id={id+'-manual'} ref={fallback} readOnly value={artifact.content} rows={8} spellCheck={false}/></div>}
      <p className="developer-clients-feedback" role="status">{feedback&&feedback!=='manual'?tr('developer-clients.'+feedback):''}</p>
     </>:<p className="developer-clients-guidance">{tr(data.endpoint_source==='unconfigured'?'developer-clients.unconfigured':'developer-clients.invalid_endpoint')}</p>}
    </>:<div className="developer-clients-guidance"><strong>{tr('developer-clients.manual_guide')}</strong><p>{tr('developer-clients.guide_'+client)}</p><p className="muted">{tr('developer-clients.guide_boundary')}</p></div>}
    <p className="muted developer-clients-boundary">{tr('developer-clients.boundary')}</p>
   </>}
  </div>}
 </details>;
}
