import {useEffect,useId,useLayoutEffect,useRef,useState} from 'react';
import {createPortal} from 'react-dom';
import {DiffEditor} from '@monaco-editor/react';
import type {editor} from 'monaco-editor';
import {ArrowUpRight,FileDiff,GitBranch,LockKeyhole,RefreshCw} from 'lucide-react';
import {useApi} from './api-context';
import {ApiError,query,type ApiCaller} from './api';
import {formatDateTime,formatNumber,tr,UiError,useLocale} from './i18n';
import {Empty,ErrorNotice,Loading} from './ui';
import {gitSourceAvailable,preferredGitSource,validWorkspaceGitStatus,workspaceGitDiff,type WorkspaceGitDiff,type WorkspaceGitFile,type WorkspaceGitSource,type WorkspaceGitStatus} from './workspace-git-model';
import './WorkspaceGitPanel.css';

const t=(key:string,values?:Record<string,unknown>)=>tr('workspace-git.'+key,values);
type Scope={project:string;api:ApiCaller};
type Selection=Scope&{path:string;source:WorkspaceGitSource};
type Observation=Scope&{value?:WorkspaceGitStatus;error?:unknown;loading:boolean};
type Comparison=Selection&{value?:WorkspaceGitDiff;error?:unknown;loading:boolean};
type Props={project:string;active:boolean;previewTarget:HTMLElement|null;onOpenFile:(path:string)=>void;id?:string;labelledBy?:string};
const sameScope=(left:Scope|undefined,right:Scope)=>left?.project===right.project&&left.api===right.api;
const stateKeys:Record<string,string>={M:'modified',A:'added',D:'deleted',R:'renamed',C:'copied',U:'conflicted',T:'type_changed','?':'untracked','!':'ignored'};
function fileStateLabel(file:WorkspaceGitFile){return file.conflicted?t('conflicted'):file.untracked?t('untracked'):t('file_states',{index:file.index_status===' '?t('unchanged'):t(stateKeys[file.index_status]),worktree:file.worktree_status===' '?t('unchanged'):t(stateKeys[file.worktree_status])});}

/**
 * Read-only Git observations have a lifetime separate from editor drafts and
 * pending saves. The sidebar and its portal share one request owner; this
 * component never calls a workspace mutation or replaces the editing model.
 */
export function WorkspaceGitPanel({project,active,previewTarget,onOpenFile,id,labelledBy}:Props){
 useLocale();const api=useApi(),stateDescriptionID=useId(),scope={project,api};
 const [observation,setObservation]=useState<Observation>(),[selection,setSelection]=useState<Selection>(),[comparison,setComparison]=useState<Comparison>();
 const current=useRef({...scope,active}),chosen=useRef<Selection|undefined>(undefined),statusEpoch=useRef(0),diffEpoch=useRef(0),statusAbort=useRef<AbortController|undefined>(undefined),diffAbort=useRef<AbortController|undefined>(undefined);
 const list=useRef<HTMLDivElement|null>(null),listPosition=useRef(0),viewStates=useRef(new Map<string,editor.IDiffEditorViewState>());
 current.current={...scope,active};chosen.current=selection;
 // Scope-tag every render, not just effect cleanup: an account/project change
 // cannot show one frame of an earlier caller's code while new reads start.
 const page=sameScope(observation,scope)?observation:undefined,selected=sameScope(selection,scope)?selection:undefined;
 const detail=sameScope(comparison,scope)&&selected?.path===comparison?.path&&selected?.source===comparison?.source?comparison:undefined;
 const selectedFile=page?.value?.files.find(file=>file.path===selected?.path);
 // A staged deletion followed by recreation (D/?) still has a readable working
 // file. A working deletion, or a staged deletion with no recreation, does not.
 const canOpenSelected=!!selectedFile&&selectedFile.worktree_status!=='D'&&!(selectedFile.index_status==='D'&&selectedFile.worktree_status===' ');
 function invalidateDiff(){++diffEpoch.current;diffAbort.current?.abort();setComparison(undefined);}
 function live(requestScope:Scope,epoch:number,kind:'status'|'diff',abort:AbortController){return sameScope(requestScope,current.current)&&current.current.active&&!abort.signal.aborted&&epoch===(kind==='status'?statusEpoch.current:diffEpoch.current);}
 async function readDiff(status:WorkspaceGitStatus,file:WorkspaceGitFile,source:WorkspaceGitSource){
  const requestScope={project,api},request=++diffEpoch.current;diffAbort.current?.abort();const abort=new AbortController();diffAbort.current=abort;
  const next={...requestScope,path:file.path,source};setSelection(next);chosen.current=next;
  setComparison({...next,loading:!file.conflicted});
  if(file.conflicted)return; // A conflict is not silently presented as an ordinary two-way diff.
  try{
   const search=new URLSearchParams({project_id:project,path:file.path,source});
   const raw=await api<unknown>('/workspace/git/diff?'+search,undefined,'GET',undefined,abort.signal);
   if(!live(requestScope,request,'diff',abort))return;
   const value=await workspaceGitDiff(raw,status,file,source);
   if(!live(requestScope,request,'diff',abort))return;
   if(!value)throw new UiError('workspace-git.invalid_diff');
   setComparison({...next,value,loading:false});
  }catch(error){
   if(!live(requestScope,request,'diff',abort))return;
   setComparison({...next,error,loading:false});
   if(error instanceof ApiError&&[401,403].includes(error.status)){
    // Authorization loss removes filenames as well as both code sides. A late
    // earlier success cannot put them back because its generation is invalid.
    ++statusEpoch.current;statusAbort.current?.abort();
    setObservation({...requestScope,error,loading:false});
   }
  }
 }
 async function refresh(){
  const requestScope={project,api},request=++statusEpoch.current;statusAbort.current?.abort();const abort=new AbortController();statusAbort.current=abort;
  const previous=sameScope(chosen.current,requestScope)?chosen.current:undefined;
  if(page?.value&&list.current)listPosition.current=list.current.scrollTop;
  invalidateDiff();setObservation({...requestScope,loading:true});
  try{
   const value=await api<unknown>('/workspace/git'+query(project),undefined,'GET',undefined,abort.signal);
   if(!live(requestScope,request,'status',abort))return;
   if(!validWorkspaceGitStatus(value,project))throw new UiError('workspace-git.invalid_status');
   setObservation({...requestScope,value,loading:false});
   const kept=value.available&&previous?value.files.find(file=>file.path===previous.path):undefined;
   if(kept)void readDiff(value,kept,gitSourceAvailable(kept,previous!.source)?previous!.source:preferredGitSource(kept));
   else{setSelection(undefined);chosen.current=undefined;}
  }catch(error){
   if(!live(requestScope,request,'status',abort))return;
   setObservation({...requestScope,error,loading:false});
  }
 }
 useEffect(()=>{
  if(active)void refresh();
  return()=>{++statusEpoch.current;++diffEpoch.current;statusAbort.current?.abort();diffAbort.current?.abort();};
 },[project,api,active]);
 useEffect(()=>{viewStates.current.clear();listPosition.current=0;},[project,api]);
 useLayoutEffect(()=>{if(page?.value&&list.current)list.current.scrollTop=listPosition.current;},[page?.value]);
 const status=page?.value;
 const changed=detail?.error instanceof ApiError&&detail.error.status===409;
 const preview=<section className="workspace-git-preview" aria-label={t('preview_label')}>
  <header className="workspace-git-preview-heading"><div><FileDiff size={16}/><h3>{selectedFile?.path||t('preview_title')}</h3></div><span className="workspace-git-readonly"><LockKeyhole size={12}/>{t('read_only')}</span></header>
  <p className="workspace-git-boundary">{t('observation_only')}</p>
  {selectedFile&&<div className="workspace-git-source-row"><div role="group" aria-label={t('comparison_source')}>{(['worktree','index'] as const).map(source=><button key={source} type="button" className={selected?.source===source?'active':''} aria-pressed={selected?.source===source} disabled={page?.loading||!gitSourceAvailable(selectedFile,source)} onClick={()=>status&&void readDiff(status,selectedFile,source)}>{t(source)}</button>)}</div><button type="button" className="text-button" disabled={!canOpenSelected} title={canOpenSelected?undefined:t('deleted')} onClick={()=>onOpenFile(selectedFile.path)}>{t('open_editor')}<ArrowUpRight size={13}/></button></div>}
  {selectedFile?.previous_path&&<p className="workspace-git-path-note">{t('previous_path',{path:selectedFile.previous_path})}</p>}
  <div className="workspace-git-comparison">
   {page?.loading||detail?.loading?<Loading/>:page?.error?<ErrorNotice error={page.error}/>:!status||!status.available?<Empty title={t(status?'unavailable_'+status.reason_code:'select_change')} description={t(status?'unavailable_help':'select_change_help')}/>:!status.files.length?<Empty title={t('clean_title')} description={t('clean_help')}/>:selectedFile?.conflicted?<Empty title={t('conflict_title')} description={t('conflict_help')} action={<button type="button" disabled={!canOpenSelected} title={canOpenSelected?undefined:t('deleted')} onClick={()=>onOpenFile(selectedFile.path)}>{t('open_editor')}</button>}/>:changed?<Empty title={t('changed_title')} description={t('changed_help')} action={<button type="button" onClick={()=>void refresh()}><RefreshCw size={14}/>{t('refresh')}</button>}/>:detail?.error?<ErrorNotice error={detail.error} onRetry={()=>void refresh()}/>:detail?.value?.binary?<Empty title={t('binary_title')} description={t('binary_help')}/>:detail?.value?<GitComparison value={detail.value} views={viewStates.current}/>:<Empty title={t('select_change')} description={t('select_change_help')}/>}
  </div>
  {detail?.value&&<footer className="workspace-git-diff-footer"><span>{t(detail.value.source==='index'?'head_to_index':'index_to_worktree')}</span><span>{t('observed',{time:formatDateTime(detail.value.observed_at,{hour:'2-digit',minute:'2-digit',second:'2-digit'})})}</span></footer>}
 </section>;
 return <>
  <div id={id} role="tabpanel" aria-labelledby={labelledBy} className="workspace-git-sidebar" hidden={!active}>
   <div className="workspace-git-list-heading"><strong>{t('changes_title')}</strong><button type="button" className="icon-button" disabled={page?.loading} aria-label={t('refresh')} title={t('refresh')} onClick={()=>void refresh()}><RefreshCw size={14}/></button></div>
   {status?.available&&<div className="workspace-git-branch"><GitBranch size={13}/><span title={status.branch||status.head}>{status.detached?t('detached'):status.branch}{status.unborn&&<small>{t('unborn')}</small>}</span><code title={status.head}>{status.head.slice(0,8)}</code></div>}
   {page?.loading?<Loading/>:page?.error?<ErrorNotice error={page.error}/>:status&&!status.available?<p className="workspace-git-side-note">{t('unavailable_'+status.reason_code)}</p>:status&&!status.files.length?<p className="workspace-git-side-note">{t('clean_title')}</p>:null}
   <div className="workspace-git-files" ref={list} onScroll={event=>{if(page?.value&&!page.loading)listPosition.current=event.currentTarget.scrollTop;}} aria-label={t('changed_files')}>
    {status?.files.map((file,index)=><button type="button" key={file.path} className={selected?.path===file.path?'active':''} aria-label={file.path} aria-describedby={stateDescriptionID+'-'+index} aria-current={selected?.path===file.path?'true':undefined} onClick={()=>void readDiff(status,file,preferredGitSource(file))} title={file.path}>
     <FileDiff size={13}/><span className="workspace-git-file-name">{file.path}</span><span id={stateDescriptionID+'-'+index} className={'workspace-git-file-state'+(file.conflicted?' conflicted':'')} aria-label={fileStateLabel(file)} title={fileStateLabel(file)}>{file.conflicted?'!':file.untracked?'?':file.index_status+file.worktree_status}</span>
    </button>)}
   </div>
   {status?.available&&<footer className="workspace-git-list-footer"><span>{t('loaded_count',{shown:formatNumber(status.files.length),total:formatNumber(status.total)})}</span>{status.truncated&&<p>{t('truncated',{limit:formatNumber(status.limit)})}</p>}{status.restricted_files>0&&<p>{t('restricted',{count:formatNumber(status.restricted_files)})}</p>}<details><summary>{t('observation_details')}</summary><dl><dt>{t('head')}</dt><dd><code>{status.head||t('unborn')}</code></dd><dt>{t('observed_at')}</dt><dd>{formatDateTime(status.observed_at)}</dd></dl></details></footer>}
  </div>
  {previewTarget&&createPortal(preview,previewTarget)}
 </>;
}

function GitComparison({value,views}:{value:WorkspaceGitDiff;views:Map<string,editor.IDiffEditorViewState>}){
 useLocale();const instance=useRef<editor.IStandaloneDiffEditor|undefined>(undefined),key=value.path+'\0'+value.source;
 useEffect(()=>()=>{const state=instance.current?.saveViewState();if(state){views.set(key,state);if(views.size>32)views.delete(views.keys().next().value!);}},[key,views]);
 const model=(side:string)=>'suma-git:///'+encodeURIComponent(value.project_id)+'/'+value.source+'/'+side+'/'+value.path.split('/').map(encodeURIComponent).join('/')+'?head='+value.head;
 return <DiffEditor height="100%" original={value.original} modified={value.modified} originalModelPath={model('original')} modifiedModelPath={model('modified')} theme={document.documentElement.dataset.theme==='dark'?'vs-dark':'light'} loading={<Loading/>} onMount={editor=>{instance.current=editor;const state=views.get(key);if(state)editor.restoreViewState(state);}} options={{ariaLabel:t('preview_label'),readOnly:true,domReadOnly:true,originalEditable:false,automaticLayout:true,renderSideBySide:true,useInlineViewWhenSpaceIsLimited:true,fontSize:12,lineHeight:22,minimap:{enabled:false},scrollBeyondLastLine:false,renderOverviewRuler:false}}/>;
}
