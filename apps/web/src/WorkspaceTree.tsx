import {useEffect,useId,useLayoutEffect,useRef,useState} from 'react';
import {ArrowUp,ChevronRight,FileCode,Folder,RefreshCw} from 'lucide-react';
import type {WorkspaceTreePage} from '../../../packages/contracts/src';
import {useApi} from './api-context';
import {ApiError,type ApiCaller} from './api';
import {formatNumber,tr,UiError,useLocale} from './i18n';
import {ErrorNotice} from './ui';
import {appendWorkspaceTreePage,filterWorkspaceEntries,validWorkspaceTreePage,workspaceBreadcrumbs,workspaceParent,workspaceTreePath} from './workspace-tree-model';
import './WorkspaceTree.css';

const t=(key:string,values?:Record<string,unknown>)=>tr('workspace-tree.'+key,values);
type Props={project:string;active:boolean;selectedPath?:string;onOpen:(path:string)=>void};
type ReadState={page?:WorkspaceTreePage;loading:boolean;before:string;error?:unknown;refreshRequired?:boolean;directoryChanged?:boolean};

/** Account/project ownership is reset before any old listing can render. */
export function WorkspaceTree(props:Props){
 const api=useApi(),owner=useRef({api,project:props.project,epoch:0});
 if(owner.current.api!==api||owner.current.project!==props.project)owner.current={api,project:props.project,epoch:owner.current.epoch+1};
 return <ScopedWorkspaceTree key={owner.current.epoch} {...props} api={api}/>;
}
function ScopedWorkspaceTree({project,active,selectedPath,onOpen,api}:Props&{api:ApiCaller}){
 useLocale();const description=useId(),[directory,setDirectory]=useState(''),[filter,setFilter]=useState(''),[read,setRead]=useState<ReadState>({loading:false,before:''});
 const state=useRef(read),current=useRef({active,directory}),epoch=useRef(0),controller=useRef<AbortController|undefined>(undefined),cursors=useRef(new Set<string>());
 const focusDirectory=useRef(false),focusEntry=useRef<string|undefined>(undefined),crumb=useRef<HTMLButtonElement|null>(null),rows=useRef(new Map<string,HTMLButtonElement>()),summary=useRef<HTMLParagraphElement|null>(null);
 state.current=read;current.current={active,directory};
 function update(value:ReadState){state.current=value;setRead(value);}
 function stop(){epoch.current++;controller.current?.abort();}
 async function load(target:string,before=''){
  if(!current.current.active)return;
  const previous=before?state.current.page:undefined;
  if(before&&(!previous||previous.directory!==target||previous.next_cursor!==before||state.current.loading))return;
  stop();const request=epoch.current,abort=new AbortController();controller.current=abort;
  if(!before)cursors.current.clear();
  update({page:previous,loading:true,before});
  const live=()=>!abort.signal.aborted&&request===epoch.current&&current.current.active&&current.current.directory===target;
  try{
   const next=await api<WorkspaceTreePage>(workspaceTreePath(project,target,before),undefined,'GET',undefined,abort.signal);
   if(!live())return;
   if(!validWorkspaceTreePage(next,project,target,before))throw new UiError('workspace-tree.invalid_response');
   if(next.next_cursor&&cursors.current.has(next.next_cursor))throw new UiError('workspace-tree.changed');
   const page=previous?appendWorkspaceTreePage(previous,next,before):next;
   if(next.next_cursor)cursors.current.add(next.next_cursor);
   if(before)focusEntry.current=next.entries[0]?.path??'';
   update({page,loading:false,before:''});
  }catch(error){
   if(!live())return;
   const changed=error instanceof ApiError&&error.status===409||error instanceof UiError&&error.ui_key==='workspace-tree.changed';
   const invalid=error instanceof UiError&&error.ui_key==='workspace-tree.invalid_response',accessLost=error instanceof ApiError&&[401,403].includes(error.status);
   update({page:changed||invalid||accessLost?undefined:previous,loading:false,before:changed||invalid||accessLost?'':before,error,refreshRequired:changed||invalid,directoryChanged:changed});
  }
 }
 function navigate(target:string){if(!active||target===directory)return;current.current={active,directory:target};focusDirectory.current=true;focusEntry.current=undefined;setDirectory(target);setFilter('');void load(target);}
 useLayoutEffect(()=>{
  // Re-entering this tab needs a fresh current-authority observation. Clear
  // the old listing before paint while keeping directory and editor state.
  if(active)void load(current.current.directory);
  if(!active&&state.current.loading)update({...state.current,loading:false});
  return stop;
 },[active]);
 useEffect(()=>{if(focusDirectory.current){focusDirectory.current=false;crumb.current?.focus();}},[directory]);
 useEffect(()=>{if(focusEntry.current!==undefined&&!read.loading){const path=focusEntry.current;focusEntry.current=undefined;(rows.current.get(path)??summary.current)?.focus();}},[read]);
 const page=read.page,entries=filterWorkspaceEntries(page?.entries??[],filter),parts=workspaceBreadcrumbs(directory);
 return <section className="workspace-tree" aria-label={t('title')} aria-busy={read.loading}>
  <div className="workspace-tree-navigation"><nav aria-label={t('breadcrumbs')} className="workspace-tree-breadcrumbs">
   <button type="button" ref={directory?undefined:crumb} disabled={!active} aria-current={!directory?'location':undefined} onClick={()=>navigate('')}>{t('root')}</button>
   {parts.map((part,index)=><span key={part.path}><ChevronRight size={11} aria-hidden="true"/><button type="button" ref={index===parts.length-1?crumb:undefined} title={part.path} disabled={!active} aria-current={index===parts.length-1?'location':undefined} onClick={()=>navigate(part.path)}>{part.name}</button></span>)}
  </nav><button type="button" className="icon-button" aria-label={t('refresh')} disabled={!active||read.loading} onClick={()=>void load(directory)}><RefreshCw size={13}/></button></div>
  {directory&&<button type="button" className="workspace-tree-parent" disabled={!active} onClick={()=>navigate(workspaceParent(directory))}><ArrowUp size={13}/>{t('parent')}</button>}
  <input className="tree-search" value={filter} disabled={!active} aria-label={t('filter')} aria-describedby={description} placeholder={t('filter_placeholder')} onChange={event=>setFilter(event.target.value)}/>
  <p id={description} className="workspace-tree-hint">{t('filter_hint')}</p>
  {!!read.error&&<div className="workspace-tree-error"><ErrorNotice error={read.error}/>{read.refreshRequired?<>{read.directoryChanged&&<p>{t('changed')}</p>}<button type="button" disabled={!active} onClick={()=>void load(directory)}>{t('refresh_required')}</button></>:<button type="button" disabled={!active||read.loading} onClick={()=>void load(directory,read.before)}>{tr('common.retry')}</button>}</div>}
  {read.loading&&<p role="status" className="workspace-tree-note">{t(read.before?'loading_more':'loading')}</p>}
  <div className="tree-files workspace-tree-entries">
   {entries.map(entry=><button type="button" key={entry.path} ref={element=>{if(element)rows.current.set(entry.path,element);else rows.current.delete(entry.path);}} className={entry.kind==='file'&&selectedPath===entry.path?'active':''} aria-current={entry.kind==='file'&&selectedPath===entry.path?'page':undefined} title={entry.path} disabled={!active} onClick={()=>entry.kind==='directory'?navigate(entry.path):onOpen(entry.path)}>{entry.kind==='directory'?<Folder size={14}/>:<FileCode size={14}/>}<span>{entry.name}</span>{entry.kind==='directory'&&<ChevronRight className="workspace-tree-directory-arrow" size={12}/>}</button>)}
   {page&&!entries.length&&<p className="workspace-tree-note">{filter?t('no_matches'):!page.next_cursor?t('empty'):t('empty_page')}</p>}
  </div>
  {page&&<footer className="workspace-tree-footer"><p ref={summary} tabIndex={-1} role="status">{filter?t('filtered',{shown:formatNumber(entries.length),loaded:formatNumber(page.entries.length)}):t('loaded',{count:formatNumber(page.entries.length)})}</p><p>{t(page.next_cursor?'more_available':'all_loaded')}</p>{page.next_cursor&&<button type="button" disabled={!active||read.loading} onClick={()=>void load(directory,page.next_cursor)}>{t(read.loading&&read.before?'loading_more':'more')}</button>}</footer>}
 </section>;
}
