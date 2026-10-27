import {RemoteWorkspaceEditor} from './RemoteWorkspaceEditor';
import {RemoteWriteHistory} from './RemoteWriteHistory';
import {editableRemoteSource} from './remote-workspace-write-model';
import {useEffect,useRef,useState} from 'react';
import Editor from '@monaco-editor/react';
import {ArrowLeft,FileCode,Folder,RefreshCw} from 'lucide-react';
import type {WorkspaceObservation,WorkspaceObservationArguments,WorkspaceObservationCreateRequest,WorkspaceObservationScope,WorkspaceObservationTargetPage} from '../../../packages/contracts/src';
import {useApi} from './api-context';
import {operation,query,type ApiCaller} from './api';
import {formatDateTime,formatNumber,tr,UiError,useLocale} from './i18n';
import {Empty,ErrorNotice,Field,Loading} from './ui';
import {remoteScopeKey,validRemoteObservation,validRemoteTargets,remoteFileLanguage} from './remote-workspace-model';
import './RemoteWorkspacePanel.css';

const t=(key:string,values?:Record<string,unknown>)=>tr('remote-workspace.'+key,values);
type Snapshot={api:ApiCaller;scope:string;observation:WorkspaceObservation};
const preference=(project:string)=>'suma-remote-workspace-target:'+project;
function savedTarget(project:string){try{return sessionStorage.getItem(preference(project))||'';}catch{return '';}}

/** Remote observations never enter the local editing/saving state or cache. */
export function RemoteWorkspacePanel({project,active=true}:{project:string;active?:boolean}){
 const api=useApi();useLocale();
 const [targets,setTargets]=useState<{api:ApiCaller;value:WorkspaceObservationTargetPage}>(),[targetKey,setTargetKey]=useState(()=>savedTarget(project)),[targetError,setTargetError]=useState<unknown>(),[targetLoading,setTargetLoading]=useState(false);
 const [listing,setListing]=useState<Snapshot>(),[reading,setReading]=useState<Snapshot>(),[current,setCurrent]=useState<Snapshot>(),[error,setError]=useState<unknown>(),[directory,setDirectory]=useState(''),[sending,setSending]=useState(false);
 const [editing,setEditing]=useState<{api:ApiCaller;scope:string;source:WorkspaceObservation}>(),[writeRevision,setWriteRevision]=useState(0);
 const wantedEdit=useRef(false),previousActive=useRef(active);
 const page=targets?.api===api?targets.value:undefined,target=page?.items.find(item=>remoteScopeKey(item)===targetKey);
 const signature=target?remoteScopeKey(target):'',currentScope=useRef(signature);currentScope.current=signature;
 const access=useRef({api,scope:signature,canWrite:target?.can_write===true});access.current={api,scope:signature,canWrite:target?.can_write===true};
 const epoch=useRef(0),targetEpoch=useRef(0),controller=useRef<AbortController|undefined>(undefined),targetController=useRef<AbortController|undefined>(undefined),timer=useRef<ReturnType<typeof setTimeout>|undefined>(undefined);
 const last=useRef<{api:ApiCaller;body:WorkspaceObservationCreateRequest;id?:string}|undefined>(undefined);
 const visible=(snapshot?:Snapshot)=>snapshot?.api===api&&snapshot.scope===signature?snapshot.observation:undefined;
 const files=visible(listing),document=visible(reading),status=visible(current);
 const list=files?.result?.list,read=document?.result?.read;
 const editor=target?.can_write&&editing?.api===api&&editing.scope===signature?editing.source:undefined;
 const waiting=sending||status?.state==='requested'||status?.state==='leased';
 // Each snapshot has its own deadline. Starting another read cannot extend a
 // previous file list or source body while authorization polling is in flight.
 useEffect(()=>{
  const deadlines=[listing,reading].filter((value):value is Snapshot=>!!value&&value.api===api&&value.scope===signature);
  const timers=deadlines.map(snapshot=>setTimeout(()=>{
   if(snapshot.observation.operation==='list')setListing(value=>value===snapshot?undefined:value);
   else setReading(value=>value===snapshot?undefined:value);
  },Math.max(0,Date.parse(snapshot.observation.expires_at)-Date.now())));
  return()=>timers.forEach(clearTimeout);
 },[listing,reading,api,signature]);
 function stop(){epoch.current++;controller.current?.abort();if(timer.current)clearTimeout(timer.current);}
 function clear(){stop();wantedEdit.current=false;setListing(undefined);setReading(undefined);setCurrent(undefined);setError(undefined);setSending(false);last.current=undefined;}
 async function readTargets(){
  if(editor)return;
  const request=++targetEpoch.current;targetController.current?.abort();const abort=new AbortController();targetController.current=abort;
  clear();setTargets(undefined);setTargetError(undefined);setTargetLoading(true);
  try{const value=await api<WorkspaceObservationTargetPage>('/workspace/targets'+query(project),undefined,'GET',undefined,abort.signal);if(abort.signal.aborted||request!==targetEpoch.current)return;if(!validRemoteTargets(value,project))throw new UiError('remote-workspace.invalid_response');setTargets({api,value});}
  catch(reason){if(!abort.signal.aborted&&request===targetEpoch.current)setTargetError(reason);}
  finally{if(!abort.signal.aborted&&request===targetEpoch.current)setTargetLoading(false);}
 }
 useEffect(()=>{setTargets(undefined);setEditing(undefined);setTargetKey(savedTarget(project));if(active)void readTargets();return()=>{targetEpoch.current++;targetController.current?.abort();stop();};},[api,project]);
 useEffect(()=>{clear();setDirectory('');if(target&&active&&!editor)void observe('list',{directory:'',limit:100});},[signature]);
 useEffect(()=>{if(!active)stop();else if(!previousActive.current&&!editor)void readTargets();previousActive.current=active;},[active]);
 async function observe(kind:string,args:WorkspaceObservationArguments,expected=''){
  if(!target)return;stop();setCurrent(undefined);setSending(true);setError(undefined);if(kind==='list'){setListing(undefined);setReading(undefined);}else setReading(undefined);
  const body:WorkspaceObservationCreateRequest={project_id:project,scope:{run_id:target.run_id,node_id:target.node_id,attempt:target.attempt,fence:target.fence,worker_id:target.worker_id,tool_profile_id:target.tool_profile_id,profile_sha256:target.profile_sha256},operation:kind,arguments:args,...(expected?{expected_sha256:expected}:{}),idempotency_key:operation()};
  last.current={api,body};await execute(last.current);
 }
 async function execute(request:NonNullable<typeof last.current>){
  stop();const generation=epoch.current,key=remoteScopeKey(request.body.scope),abort=new AbortController();controller.current=abort;
  const live=()=>!abort.signal.aborted&&epoch.current===generation&&currentScope.current===key;
  const hide=()=>{setListing(undefined);setReading(undefined);};
  const fail=(reason:unknown)=>{if(live()){hide();setCurrent(undefined);setSending(false);setError(reason);}};
  async function receive(metadata=false){
   try{
    const value=await request.api<WorkspaceObservation>(request.id?'/workspace/observations/'+encodeURIComponent(request.id)+query(project)+(metadata?'&metadata=true':''):'/workspace/observations',request.id?undefined:request.body,request.id?'GET':'POST',undefined,abort.signal);
    if(!live())return;
    if(!validRemoteObservation(value,project,request.body.scope,request.body.operation,request.body.arguments,request.body.expected_sha256||'',request.id,metadata))throw new UiError('remote-workspace.invalid_response');
    request.id=value.id;setSending(false);setCurrent({api:request.api,scope:key,observation:value});
    if(value.state==='completed'){
     if(!metadata&&wantedEdit.current&&value.arguments.full_file){wantedEdit.current=false;await openEditor(value);return;}
     if(!metadata){const snapshot={api:request.api,scope:key,observation:value};if(value.operation==='list')setListing(snapshot);else setReading(snapshot);}
     const remaining=Date.parse(value.expires_at)-Date.now();if(remaining<=0){hide();setError(new UiError('remote-workspace.expired'));return;}
     timer.current=setTimeout(()=>void receive(true),Math.min(3000,remaining+10));
    }else if(value.state==='requested'||value.state==='leased')timer.current=setTimeout(()=>void receive(),1000);
    else{hide();setError(new UiError('remote-workspace.error.'+(value.error_code||'workspace_closed')));}
   }catch(reason){fail(reason);}
  }
  setError(undefined);setSending(true);await receive();
 }
 async function openEditor(value:WorkspaceObservation){
  const generation=epoch.current,key=remoteScopeKey(value.scope),permitted=()=>access.current.api===api&&access.current.scope===key&&access.current.canWrite;
  if(!permitted())return;
  try{if(!await editableRemoteSource(value))throw new UiError('remote-workspace-write.incomplete_source');if(epoch.current!==generation||!permitted())return;stop();setEditing({api,scope:key,source:value});setError(undefined);}
  catch(reason){if(epoch.current===generation)setError(reason);}
 }
 function editFile(){if(!target?.can_write||!document?.result?.read)return;const value=document.result.read;if(value.start_line===1&&value.complete)void openEditor(document);else{wantedEdit.current=true;void observe('read',{path:value.path,full_file:true});}}
 function closeEditor(){setEditing(undefined);if(target&&active)openDirectory(directory);}
 function accessLost(){setEditing(undefined);clear();setTargets(undefined);setTargetError(new UiError('remote-workspace-write.access_lost'));}
 function choose(value:string){if(editor)return;clear();setTargetKey(value);try{sessionStorage.setItem(preference(project),value);}catch{/* Preference loss must never block an authorized read. */}}
 function openDirectory(value:string){setDirectory(value);void observe('list',{directory:value,limit:100});}
 const sourceLink=target?'?'+new URLSearchParams({project,view:'development',sub:'runs',run:target.run_id,node:target.node_id,attempt:String(target.attempt)}):'';
 return <section className="remote-workspace panel" aria-label={t('title')}>
  <div className="remote-heading"><div><h2>{t('title')}</h2><p className="muted">{t('intro')}</p></div><button type="button" onClick={()=>void readTargets()} disabled={targetLoading||!!editor}><RefreshCw size={14}/>{t('refresh_targets')}</button></div>
  {targetLoading&&<Loading/>}{!!targetError&&<ErrorNotice error={targetError} onRetry={()=>void readTargets()}/>}
  {page&&<><Field label={t('target')} hint={t('target_hint')}><select data-remote-target disabled={!!editor} value={target?targetKey:''} onChange={event=>choose(event.target.value)}><option value="">{t('choose')}</option>{page.items.map(item=><option key={remoteScopeKey(item)} value={remoteScopeKey(item)}>{item.node_name||item.node_id} · {item.run_name||item.run_id.slice(0,8)} · {t('attempt',{attempt:formatNumber(item.attempt)})}</option>)}</select></Field>{page.truncated&&<p className="muted">{t('truncated',{limit:formatNumber(page.limit)})}</p>}{!page.items.length&&<Empty title={t('no_targets')} description={t('no_targets_help')}/>}</>}
  {target&&<>
   <div className="remote-heading"><span>{target.node_name||target.node_id} · {target.worker_id} · {editor?tr('remote-workspace-write.editing_tag'):t('readonly')}</span><a href={sourceLink}>{t('open_execution')}</a></div>
   <p className="muted">{t('tool_profile')}: <code>{target.tool_profile_id}</code> · {target.pool_id?t('pool_route',{pool:target.pool_id}):t('private_route')}</p>
   {waiting&&<p role="status">{t(sending?'requesting':status?.state==='leased'?'reading':'waiting')}</p>}
   {!!error&&<div><ErrorNotice error={error}/><button type="button" disabled={!last.current||last.current.api!==api} onClick={()=>{if(last.current?.api===api)void execute(last.current);}}>{t('retry_original')}</button></div>}
   {editor?<RemoteWorkspaceEditor key={signature+':'+editor.id} project={project} source={editor} active={active} onClose={closeEditor} onAccessLost={accessLost} onRecorded={()=>setWriteRevision(value=>value+1)}/>:<div className="remote-grid"><aside aria-label={t('files')}>
    <div className="remote-filebar"><span>{directory||t('root')}</span><button type="button" aria-label={t('refresh_files')} disabled={waiting} onClick={()=>openDirectory(directory)}><RefreshCw size={14}/></button></div>
    {directory&&<button type="button" className="remote-entry" disabled={waiting} onClick={()=>openDirectory(directory.split('/').slice(0,-1).join('/'))}><ArrowLeft size={14}/>{t('parent')}</button>}
    {list?.entries.map(entry=><button key={entry.path} type="button" className="remote-entry" disabled={waiting} onClick={()=>entry.kind==='directory'?openDirectory(entry.path):void observe('read',{path:entry.path,start_line:1,max_lines:200})}>{entry.kind==='directory'?<Folder size={14}/>:<FileCode size={14}/>}<span>{entry.path.split('/').pop()}</span></button>)}
    {list&&!list.entries.length&&<p className="muted">{t('empty_directory')}</p>}
    {!!list?.next_cursor&&<button type="button" disabled={waiting} onClick={()=>void observe('list',{directory,limit:100,cursor:list.next_cursor})}>{t('next_files')}</button>}
   </aside><div className="remote-code">
    {read?<><div className="remote-filebar"><strong>{read.path}</strong><span>{t('line_start',{line:formatNumber(read.start_line)})}</span>{target.can_write&&<button type="button" disabled={waiting} onClick={editFile}>{tr('remote-workspace-write.edit')}</button>}</div><Editor height="420px" path={'suma-remote://'+document!.id+'/'+encodeURIComponent(read.path)} value={read.content} language={remoteFileLanguage(read.path)} theme={globalThis.document.documentElement.dataset.theme==='dark'?'vs-dark':'light'} options={{readOnly:true,domReadOnly:true,ariaLabel:t('editor'),minimap:{enabled:false},automaticLayout:true,wordWrap:'on',lineNumbers:line=>String(line+read.start_line-1),scrollBeyondLastLine:false}} loading={<Loading/>}/><p className="muted">{t('observed',{time:formatDateTime(document!.observed_at||document!.created_at,{dateStyle:'short',timeStyle:'medium'})})}</p>{read.next_line&&<button type="button" disabled={waiting} onClick={()=>void observe('read',{path:read.path,start_line:read.next_line,max_lines:200},read.sha256)}>{t('next_lines')}</button>}<details><summary>{t('basis')}</summary><code>{read.sha256}</code><p>{t('basis_help')}</p></details></>:<Empty title={waiting?t('waiting'):t('choose_file')} description={t('editor_help')}/>}
   </div></div>}
  </>}
  <RemoteWriteHistory project={project} revision={writeRevision} active={active}/>
 </section>;
}
