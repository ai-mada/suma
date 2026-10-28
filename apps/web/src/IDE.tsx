import {RemoteWorkspacePanel} from './RemoteWorkspacePanel';
import {useApi} from './api-context';
import {useCallback,useEffect,useId,useRef,useState,type FormEvent} from 'react';
import Editor,{loader,type OnMount} from '@monaco-editor/react';
import * as monaco from 'monaco-editor/esm/vs/editor/editor.api.js';
// Retain the full entry's editor features and all bundled language services.
import 'monaco-editor/esm/vs/editor/editor.main.js';
import EditorWorker from 'monaco-editor/esm/vs/editor/editor.worker?worker';
import TypeScriptWorker from 'monaco-editor/esm/vs/language/typescript/ts.worker?worker';
import JSONWorker from 'monaco-editor/esm/vs/language/json/json.worker?worker';
import CSSWorker from 'monaco-editor/esm/vs/language/css/css.worker?worker';
import HTMLWorker from 'monaco-editor/esm/vs/language/html/html.worker?worker';
import {Folder,FileCode,FileDiff,Save,RefreshCw,Link,AlertCircle,Check,Code2} from 'lucide-react';
import {query,operation,ApiError,type ApiCaller} from './api';
import type {WorkspaceMetadata} from '../../../packages/contracts/src';
import {Loading,ErrorNotice,Empty,Field} from './ui';
import {FileSaveRequest,draftAfterFileSave,fileSaveReceipt,type FileDocument} from './file-saves';
import {tr as t,useLocale,UiError,renderDiagnostic} from './i18n';
import {Trans} from './i18n/Trans';
import {WorkspaceGitPanel} from './WorkspaceGitPanel';
import {bindWorkspaceSaveShortcut,type WorkspaceSaveShortcutState} from './workspace-save-shortcut';
import {WorkspaceTree} from './WorkspaceTree';
import {DesktopWorkspacePicker} from './DesktopWorkspacePicker';
import {desktopWorkspaceBridge} from './desktop-workspace-model';

// All editor assets and workers are bundled locally. Monaco's ESM instance has
// no public runtime NLS switch; the visible note states that precise boundary.
(self as any).MonacoEnvironment={getWorker(_:unknown,label:string){if(label==='typescript'||label==='javascript')return new TypeScriptWorker();if(label==='json')return new JSONWorker();if(['css','scss','less'].includes(label))return new CSSWorker();if(['html','handlebars','razor'].includes(label))return new HTMLWorker();return new EditorWorker();}};
loader.config({monaco});
type FileState=FileDocument;
export function IDE({project}:{project:string}){
 useLocale();
 return project?<ProjectIDE key={project} project={project}/>:<Empty title={t('workspace.select_project')}/>;
}
function ProjectIDE({project}:{project:string}){
 const api=useApi();useLocale();
 const sourceID=useId();
 const owner=useRef({api,epoch:0});if(owner.current.api!==api)owner.current={api,epoch:owner.current.epoch+1};
 const [source,setSource]=useState<'local'|'remote'>(()=>{try{return sessionStorage.getItem('suma-ide-source:'+project)==='remote'?'remote':'local';}catch{return 'local';}});
 const [remoteVisited,setRemoteVisited]=useState(source==='remote');
 const [bindingBlocked,setBindingBlocked]=useState(false);
 function select(value:'local'|'remote'){if(bindingBlocked)return;if(value==='remote')setRemoteVisited(true);setSource(value);try{sessionStorage.setItem('suma-ide-source:'+project,value);}catch{/* UI preference only; source content is never cached. */}}
 return <div><div className="ide-sources" role="tablist" aria-label={t('remote-workspace.source')} onKeyDown={event=>{if(['ArrowLeft','ArrowRight','Home','End'].includes(event.key)){event.preventDefault();const next=event.key==='Home'?'local':event.key==='End'?'remote':source==='local'?'remote':'local';select(next);event.currentTarget.querySelectorAll<HTMLButtonElement>('[role=tab]')[next==='local'?0:1]?.focus();}}}>
 <button type="button" id={sourceID+'-local-tab'} aria-controls={sourceID+'-local-panel'} role="tab" aria-selected={source==='local'} tabIndex={source==='local'?0:-1} onClick={()=>select('local')}>{t('remote-workspace.local')}</button>
 <button type="button" id={sourceID+'-remote-tab'} aria-controls={sourceID+'-remote-panel'} role="tab" aria-selected={source==='remote'} tabIndex={source==='remote'?0:-1} disabled={bindingBlocked} onClick={()=>select('remote')}>{t('remote-workspace.remote')}</button></div>
 <div id={sourceID+'-local-panel'} role="tabpanel" aria-labelledby={sourceID+'-local-tab'} hidden={source!=='local'}><LocalProjectIDE key={owner.current.epoch} project={project} active={source==='local'} onBindingBlocked={setBindingBlocked}/></div>
 <div id={sourceID+'-remote-panel'} role="tabpanel" aria-labelledby={sourceID+'-remote-tab'} hidden={source!=='remote'}>{remoteVisited&&<RemoteWorkspacePanel key={owner.current.epoch} project={project} active={source==='remote'}/>}</div></div>;
}
function LocalProjectIDE({project,active,onBindingBlocked}:{project:string;active:boolean;onBindingBlocked:(blocked:boolean)=>void}){ const api=useApi();
 const locale=useLocale(),explorerID=useId();
 const [workspace,setWorkspace]=useState<WorkspaceMetadata>(),[file,setFile]=useState<FileState>(),[content,setContent]=useState(''),[error,setError]=useState<unknown>(),[busy,setBusy]=useState(false),[fileLoading,setFileLoading]=useState(false),[binding,setBinding]=useState(''),[saved,setSaved]=useState(false),[conflict,setConflict]=useState(false),[remote,setRemote]=useState<FileState>(),[unconfirmed,setUnconfirmed]=useState(false),[reconciliation,setReconciliation]=useState(false);
 const [explorer,setExplorer]=useState<'files'|'changes'>('files'),[gitPreview,setGitPreview]=useState<HTMLDivElement|null>(null);
 const [desktopBindingBlocked,setDesktopBindingBlocked]=useState(false);
 const bindingBlockChanged=useCallback((blocked:boolean)=>{setDesktopBindingBlocked(blocked);onBindingBlocked(blocked);},[onBindingBlocked]);
 const pendingApi=useRef<ApiCaller|undefined>(undefined);
 const pending=useRef(new FileSaveRequest()),openGeneration=useRef(0),dirty=!!file&&content!==file.content;
 const saveAction=useRef<monaco.IDisposable|undefined>(undefined),editingInstance=useRef<monaco.editor.IStandaloneCodeEditor|undefined>(undefined);
 const shortcut=useRef<WorkspaceSaveShortcutState>({hasFile:false,dirty:false,pending:false,busy:false,conflict:false,save:()=>{}});
 const workspaceGeneration=useRef(0),workspaceAbort=useRef<AbortController|undefined>(undefined),workspaceActive=useRef(active),workspaceRefresh=useRef(false);workspaceActive.current=active;
 async function loadWorkspace(){
  if(!workspaceActive.current)return;
  const generation=++workspaceGeneration.current;workspaceAbort.current?.abort();const abort=new AbortController();workspaceAbort.current=abort;
  try{const next=await api<WorkspaceMetadata>('/workspace'+query(project),undefined,'GET',undefined,abort.signal);if(!abort.signal.aborted&&generation===workspaceGeneration.current&&workspaceActive.current){setWorkspace(next);workspaceRefresh.current=false;setError(undefined);return next;}}
  catch(e){if(!abort.signal.aborted&&generation===workspaceGeneration.current&&workspaceActive.current)setError(e instanceof ApiError&&[404,501].includes(e.status)?new UiError('workspace.ide.interface_unavailable'):e);}
 }
 useEffect(()=>{if(active&&(!workspace||workspaceRefresh.current))void loadWorkspace();return()=>{workspaceGeneration.current++;workspaceAbort.current?.abort();};},[api,project,active]);
 useEffect(()=>{const unload=(e:BeforeUnloadEvent)=>{if(dirty||unconfirmed||pending.current.pending){e.preventDefault();e.returnValue='';}};window.addEventListener('beforeunload',unload);return()=>window.removeEventListener('beforeunload',unload);},[dirty,unconfirmed]);
 useEffect(()=>{const guard=(e:Event)=>{if(dirty||unconfirmed||pending.current.pending){e.preventDefault();setError(new UiError('workspace.ide.dirty_navigation'));}};window.addEventListener('suma:navigate',guard);return()=>window.removeEventListener('suma:navigate',guard);},[dirty,unconfirmed]);
 async function open(path:string){
  if(busy||pending.current.pending||desktopBindingBlocked){setError(new UiError('workspace.ide.pending_open'));return;}
  if(dirty){setError(new UiError('workspace.ide.dirty_open'));return;}
  const generation=++openGeneration.current;setFileLoading(true);
  try{const next=await api<FileState>('/workspace/file'+query(project)+'&path='+encodeURIComponent(path));if(generation!==openGeneration.current)return;setFile(next);setContent(next.content);setError(undefined);setConflict(false);setSaved(false);}catch(e){if(generation===openGeneration.current)setError(e);}finally{if(generation===openGeneration.current)setFileLoading(false);}
 }
 async function save(){
  if(!file||busy||desktopBindingBlocked||(!dirty&&!pending.current.pending)||conflict)return;
  setBusy(true);if(!pending.current.pending)pendingApi.current=api;const body=pending.current.begin(project,file,content);
  try{const response=await (pendingApi.current??api)<unknown>('/workspace/file',body);const next=await fileSaveReceipt(response,body);if(!next)throw new UiError('workspace-save.invalid_receipt');setFile(next);setContent(current=>draftAfterFileSave(current,body.content,next.content));setSaved(true);setError(undefined);pending.current.completed();pendingApi.current=undefined;setUnconfirmed(false);setReconciliation(false);}
  catch(e){pending.current.failed(e);if(!pending.current.pending)pendingApi.current=undefined;setUnconfirmed(pending.current.unknown);setReconciliation(pending.current.reconciliation);setError(e);if(e instanceof ApiError&&e.status===409&&!pending.current.reconciliation){setConflict(true);try{setRemote(await api<FileState>('/workspace/file'+query(project)+'&path='+encodeURIComponent(body.path)));}catch{/* Keep the current draft if the comparison read also fails. */}}}
  finally{setBusy(false);}
 }
 // Monaco owns Ctrl/Cmd+S even when it stops DOM bubbling. Read current facts
 // instead of closing over an earlier draft, provider or pending operation.
 shortcut.current={hasFile:active&&!!file&&!fileLoading,dirty,pending:!!pending.current.pending,busy,conflict,save};
 const mountEditor:OnMount=(instance,runtime)=>{
  saveAction.current?.dispose();editingInstance.current=instance;
  saveAction.current=bindWorkspaceSaveShortcut(instance,runtime,()=>shortcut.current,t('workspace.ide.save'));
  instance.onDidDispose(()=>{if(editingInstance.current===instance){editingInstance.current=undefined;saveAction.current?.dispose();saveAction.current=undefined;}});
 };
 useEffect(()=>{const instance=editingInstance.current;if(instance){saveAction.current?.dispose();saveAction.current=bindWorkspaceSaveShortcut(instance,monaco,()=>shortcut.current,t('workspace.ide.save'));}},[locale]);
 useEffect(()=>()=>{saveAction.current?.dispose();},[]);
 async function bind(e:FormEvent){e.preventDefault();if(desktopBindingBlocked)return;setBusy(true);try{await api('/workspace/binding',{project_id:project,path:binding,idempotency_key:operation()});workspaceRefresh.current=true;await loadWorkspace();}catch(err){setError(err);}finally{setBusy(false);}}
 if(!workspace&&!error)return <Loading/>;
 return <div className="ide-workbench">
  {Boolean(error)&&<ErrorNotice error={error}/>}
  <div className="info-strip"><Code2 size={17}/><p>{t('workspace.ide.intro')}<br/><small>{t('workspace.ide.editor_language_boundary')}</small></p></div>
  {workspace&&<DesktopWorkspacePicker project={project} metadata={workspace} active={active} disabled={dirty||unconfirmed||!!pending.current.pending||busy||fileLoading} onRefresh={async()=>{const next=await loadWorkspace();if(next&&next.binding_sha256!==workspace.binding_sha256){openGeneration.current++;setFile(undefined);setContent('');setConflict(false);setRemote(undefined);setSaved(false);}return next;}} onBlockedChange={bindingBlockChanged} onBound={next=>{openGeneration.current++;setWorkspace(next);setFile(undefined);setContent('');setConflict(false);setRemote(undefined);setSaved(false);setError(undefined);setExplorer('files');}}/>}
  {workspace?.configured?<section className="panel editor-panel" hidden={desktopBindingBlocked} style={desktopBindingBlocked?{display:'none'}:undefined}>
   <aside className="file-tree"><div className="tree-heading"><Folder size={15}/><strong>{workspace.name||t('workspace.ide.project_files')}</strong></div>
    <div className="workspace-explorer-tabs" role="tablist" aria-label={t('workspace-git.explorer_tabs')} onKeyDown={event=>{if(!['ArrowLeft','ArrowRight','Home','End'].includes(event.key))return;event.preventDefault();const next=event.key==='Home'?'files':event.key==='End'?'changes':explorer==='files'?'changes':'files';setExplorer(next);document.getElementById(explorerID+'-'+next+'-tab')?.focus();}}>{(['files','changes'] as const).map(view=><button key={view} type="button" id={explorerID+'-'+view+'-tab'} role="tab" tabIndex={explorer===view?0:-1} aria-selected={explorer===view} aria-controls={explorerID+'-'+view} onClick={()=>setExplorer(view)}>{view==='files'?<FileCode size={13}/>:<FileDiff size={13}/>} {t('workspace-git.'+view+'_tab')}{view==='files'&&(dirty||unconfirmed)&&<i className="dirty-dot" aria-label={t('workspace.ide.unsaved')}/>}</button>)}</div>
    <div id={explorerID+'-files'} role="tabpanel" aria-labelledby={explorerID+'-files-tab'} className="workspace-files-view" hidden={explorer!=='files'}><WorkspaceTree key={workspace.binding_sha256} project={project} active={active&&explorer==='files'} selectedPath={file?.path} onOpen={path=>{if(file?.path!==path)void open(path);}}/></div>
    <WorkspaceGitPanel key={workspace.binding_sha256} project={project} active={active&&explorer==='changes'} previewTarget={gitPreview} id={explorerID+'-changes'} labelledBy={explorerID+'-changes-tab'} onOpenFile={path=>{setExplorer('files');if(file?.path!==path)void open(path);}}/>
   </aside>
   <div className="editor-main"><div className="workspace-code-host" hidden={explorer!=='files'}><div className="editor-toolbar"><div><FileCode size={14}/><span>{file?.path||t('workspace.ide.select_file')}</span>{dirty&&<i className="dirty-dot" aria-label={t('workspace.ide.unsaved')}/>} {saved&&!dirty&&<span className="saved-state"><Check size={12}/>{t('workspace.ide.saved')}</span>}</div><div><button disabled={!dirty||busy||unconfirmed||desktopBindingBlocked} onClick={()=>{if(file)setContent(file.content);setConflict(false);setRemote(undefined);setError(undefined);}}><RefreshCw size={13}/>{t('workspace.ide.revert')}</button><button className="primary" disabled={(!dirty&&!unconfirmed)||busy||conflict||desktopBindingBlocked} onClick={()=>void save()}><Save size={13}/>{t(unconfirmed?'workspace.ide.recover_save':'workspace.ide.save')}</button></div></div>
    {reconciliation&&<div className="conflict-banner"><AlertCircle size={17}/><div><strong>{t('workspace.ide.reconcile_title')}</strong><p>{t('workspace.ide.reconcile_help')}</p><p><Trans i18nKey="workspace.ide.pending_operation" values={{operation:pending.current.reconciliationDetails?.operation_id||pending.current.pending?.idempotency_key,path:pending.current.reconciliationDetails?.path||pending.current.pending?.path}} components={{code:<code/>}}/></p>{pending.current.reconciliationDetails?.operation_id&&pending.current.reconciliationDetails.operation_id!==pending.current.pending?.idempotency_key?<p>{t('workspace.ide.foreign_operation')}</p>:<details><summary>{t('workspace.ide.recovery_basis')}</summary><p>{t('workspace.ide.original_hash',{hash:pending.current.pending?.expected_hash})}</p><h4>{t('workspace.ide.original_content')}</h4><pre>{pending.current.source?.content}</pre><h4>{t('workspace.ide.submitted_content')}</h4><pre>{pending.current.pending?.content}</pre></details>}</div></div>}
    {conflict&&<div className="conflict-banner"><AlertCircle size={17}/><div><strong>{t('workspace.ide.conflict_title')}</strong><p>{t('workspace.ide.conflict_help')}</p><button disabled={!remote} onClick={()=>{if(remote){setFile(remote);setConflict(false);setRemote(undefined);setError(new UiError('workspace.ide.review_merge'));}}}>{t('workspace.ide.keep_draft')}</button><button disabled={!remote} onClick={()=>{if(remote){setContent(remote.content);setFile(remote);setConflict(false);setRemote(undefined);setError(undefined);}}}>{t('workspace.ide.load_latest')}</button></div></div>}
    {fileLoading?<Loading/>:file?<Editor height="100%" path={file.path} language={file.language} value={content} onMount={mountEditor} onChange={value=>{if(desktopBindingBlocked)return;setContent(value||'');setSaved(false);}} theme={document.documentElement.dataset.theme==='dark'?'vs-dark':'light'} options={{ariaLabel:t('workspace.ide.editor_label'),readOnly:desktopBindingBlocked,fontSize:12,lineHeight:22,minimap:{enabled:false},padding:{top:18},scrollBeyondLastLine:false,automaticLayout:true,wordWrap:'on',tabSize:2,renderLineHighlight:'gutter',fontFamily:'Consolas, monospace'}} loading={<Loading/>}/>:<Empty title={t('workspace.ide.empty_title')} description={t('workspace.ide.empty_description')}/>}
    </div><div ref={setGitPreview} className="workspace-git-preview-host" hidden={explorer!=='changes'}/>
   </div>
  </section>:workspace?.binding_available===false?(workspace.desktop_binding_available&&desktopWorkspaceBridge()?null:<section className="panel settings-panel"><Empty title={t('workspace-git.local_workspace_disabled')} description={t('workspace-git.local_workspace_disabled_help')}/></section>):<section className="panel settings-panel"><div className="panel-heading"><div><h2>{t('workspace.ide.bind_title')}</h2><p>{workspace?.reason?renderDiagnostic(workspace.reason_message_key?{message:workspace.reason,message_key:workspace.reason_message_key}:workspace.reason):t('workspace.ide.unbound')}</p></div><Folder size={22}/></div><form onSubmit={e=>void bind(e)}><Field label={t('workspace.ide.absolute_path')} hint={t('workspace.ide.path_hint')}><input required disabled={desktopBindingBlocked} value={binding} onChange={e=>setBinding(e.target.value)} placeholder="D:\projects\my-project"/></Field><button className="primary" disabled={busy||desktopBindingBlocked}><Link size={15}/>{t('workspace.ide.bind')}</button></form></section>}
 </div>;
}
