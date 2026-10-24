import {useEffect,useRef,useState,type FormEvent} from 'react';
import Editor,{loader} from '@monaco-editor/react';
import * as monaco from 'monaco-editor/esm/vs/editor/editor.api.js';
// Retain the full entry's editor features and all bundled language services.
import 'monaco-editor/esm/vs/editor/editor.main.js';
import EditorWorker from 'monaco-editor/esm/vs/editor/editor.worker?worker';
import TypeScriptWorker from 'monaco-editor/esm/vs/language/typescript/ts.worker?worker';
import JSONWorker from 'monaco-editor/esm/vs/language/json/json.worker?worker';
import CSSWorker from 'monaco-editor/esm/vs/language/css/css.worker?worker';
import HTMLWorker from 'monaco-editor/esm/vs/language/html/html.worker?worker';
import {Folder,FileCode,Save,RefreshCw,Link,AlertCircle,Check,Code2} from 'lucide-react';
import {api,query,operation,ApiError} from './api';
import {Loading,ErrorNotice,Empty,Field} from './ui';
import {FileSaveRequest,draftAfterFileSave,type FileDocument} from './file-saves';
import {tr as t,useLocale,UiError,renderDiagnostic} from './i18n';
import {Trans} from './i18n/Trans';

// All editor assets and workers are bundled locally. Monaco's ESM instance has
// no public runtime NLS switch; the visible note states that precise boundary.
(self as any).MonacoEnvironment={getWorker(_:unknown,label:string){if(label==='typescript'||label==='javascript')return new TypeScriptWorker();if(label==='json')return new JSONWorker();if(['css','scss','less'].includes(label))return new CSSWorker();if(['html','handlebars','razor'].includes(label))return new HTMLWorker();return new EditorWorker();}};
loader.config({monaco});
type Workspace={configured:boolean;root?:string;name?:string;reason?:string;files:{path:string;name:string;type:string}[]};
type FileState=FileDocument;
export function IDE({project}:{project:string}){
 useLocale();
 return project?<ProjectIDE key={project} project={project}/>:<Empty title={t('workspace.select_project')}/>;
}
function ProjectIDE({project}:{project:string}){
 useLocale();
 const [workspace,setWorkspace]=useState<Workspace>(),[file,setFile]=useState<FileState>(),[content,setContent]=useState(''),[error,setError]=useState<unknown>(),[busy,setBusy]=useState(false),[fileLoading,setFileLoading]=useState(false),[binding,setBinding]=useState(''),[saved,setSaved]=useState(false),[conflict,setConflict]=useState(false),[remote,setRemote]=useState<FileState>(),[treeFilter,setTreeFilter]=useState(''),[unconfirmed,setUnconfirmed]=useState(false),[reconciliation,setReconciliation]=useState(false);
 const pending=useRef(new FileSaveRequest()),openGeneration=useRef(0),dirty=!!file&&content!==file.content;
 async function loadWorkspace(){try{setWorkspace(await api<Workspace>('/workspace'+query(project)));setError(undefined);}catch(e){setError(e instanceof ApiError&&[404,501].includes(e.status)?new UiError('workspace.ide.interface_unavailable'):e);}}
 useEffect(()=>{void loadWorkspace();},[project]);
 useEffect(()=>{const unload=(e:BeforeUnloadEvent)=>{if(dirty||unconfirmed||pending.current.pending){e.preventDefault();e.returnValue='';}};window.addEventListener('beforeunload',unload);return()=>window.removeEventListener('beforeunload',unload);},[dirty,unconfirmed]);
 useEffect(()=>{const guard=(e:Event)=>{if(dirty||unconfirmed||pending.current.pending){e.preventDefault();setError(new UiError('workspace.ide.dirty_navigation'));}};window.addEventListener('suma:navigate',guard);return()=>window.removeEventListener('suma:navigate',guard);},[dirty,unconfirmed]);
 async function open(path:string){
  if(busy||pending.current.pending){setError(new UiError('workspace.ide.pending_open'));return;}
  if(dirty){setError(new UiError('workspace.ide.dirty_open'));return;}
  const generation=++openGeneration.current;setFileLoading(true);
  try{const next=await api<FileState>('/workspace/file'+query(project)+'&path='+encodeURIComponent(path));if(generation!==openGeneration.current)return;setFile(next);setContent(next.content);setError(undefined);setConflict(false);setSaved(false);}catch(e){if(generation===openGeneration.current)setError(e);}finally{if(generation===openGeneration.current)setFileLoading(false);}
 }
 async function save(){
  if(!file||busy||(!dirty&&!pending.current.pending)||conflict)return;
  setBusy(true);const body=pending.current.begin(project,file,content);
  try{const next=await api<FileState>('/workspace/file',body);setFile(next);setContent(current=>draftAfterFileSave(current,body.content,next.content));setSaved(true);setError(undefined);pending.current.completed();setUnconfirmed(false);setReconciliation(false);}
  catch(e){pending.current.failed(e);setUnconfirmed(pending.current.unknown);setReconciliation(pending.current.reconciliation);setError(e);if(e instanceof ApiError&&e.status===409&&!pending.current.reconciliation){setConflict(true);try{setRemote(await api<FileState>('/workspace/file'+query(project)+'&path='+encodeURIComponent(body.path)));}catch{/* Keep the current draft if the comparison read also fails. */}}}
  finally{setBusy(false);}
 }
 useEffect(()=>{const key=(e:KeyboardEvent)=>{if((e.ctrlKey||e.metaKey)&&e.key==='s'){e.preventDefault();void save();}};window.addEventListener('keydown',key);return()=>window.removeEventListener('keydown',key);},[file,content,busy,conflict]);
 async function bind(e:FormEvent){e.preventDefault();setBusy(true);try{await api('/workspace/binding',{project_id:project,path:binding,idempotency_key:operation()});await loadWorkspace();}catch(err){setError(err);}finally{setBusy(false);}}
 if(!workspace&&!error)return <Loading/>;
 return <div className="ide-workbench">
  {Boolean(error)&&<ErrorNotice error={error}/>}
  <div className="info-strip"><Code2 size={17}/><p>{t('workspace.ide.intro')}<br/><small>{t('workspace.ide.editor_language_boundary')}</small></p></div>
  {workspace?.configured?<section className="panel editor-panel">
   <aside className="file-tree"><div className="tree-heading"><Folder size={15}/><strong>{workspace.name||t('workspace.ide.project_files')}</strong><button className="icon-button" aria-label={t('workspace.ide.refresh_tree')} onClick={()=>void loadWorkspace()}><RefreshCw size={13}/></button></div><input className="tree-search" value={treeFilter} aria-label={t('workspace.ide.filter_files')} placeholder={t('workspace.ide.filter_placeholder')} onChange={e=>setTreeFilter(e.target.value)}/><div className="tree-files">{(workspace.files||[]).filter(f=>f.path.toLowerCase().includes(treeFilter.toLowerCase())).map(f=><button className={file?.path===f.path?'active':''} key={f.path} title={f.path} disabled={f.type==='directory'} onClick={()=>void open(f.path)} style={{paddingLeft:12+Math.min(f.path.split('/').length-1,5)*10}}>{f.type==='directory'?<Folder size={13}/>:<FileCode size={13}/>}<span>{f.name||f.path}</span></button>)}</div><small className="tree-root">{workspace.root}</small></aside>
   <div className="editor-main"><div className="editor-toolbar"><div><FileCode size={14}/><span>{file?.path||t('workspace.ide.select_file')}</span>{dirty&&<i className="dirty-dot" aria-label={t('workspace.ide.unsaved')}/>} {saved&&!dirty&&<span className="saved-state"><Check size={12}/>{t('workspace.ide.saved')}</span>}</div><div><button disabled={!dirty||busy||unconfirmed} onClick={()=>{if(file)setContent(file.content);setConflict(false);setRemote(undefined);setError(undefined);}}><RefreshCw size={13}/>{t('workspace.ide.revert')}</button><button className="primary" disabled={(!dirty&&!unconfirmed)||busy||conflict} onClick={()=>void save()}><Save size={13}/>{t(unconfirmed?'workspace.ide.recover_save':'workspace.ide.save')}</button></div></div>
    {reconciliation&&<div className="conflict-banner"><AlertCircle size={17}/><div><strong>{t('workspace.ide.reconcile_title')}</strong><p>{t('workspace.ide.reconcile_help')}</p><p><Trans i18nKey="workspace.ide.pending_operation" values={{operation:pending.current.reconciliationDetails?.operation_id||pending.current.pending?.idempotency_key,path:pending.current.reconciliationDetails?.path||pending.current.pending?.path}} components={{code:<code/>}}/></p>{pending.current.reconciliationDetails?.operation_id&&pending.current.reconciliationDetails.operation_id!==pending.current.pending?.idempotency_key?<p>{t('workspace.ide.foreign_operation')}</p>:<details><summary>{t('workspace.ide.recovery_basis')}</summary><p>{t('workspace.ide.original_hash',{hash:pending.current.pending?.expected_hash})}</p><h4>{t('workspace.ide.original_content')}</h4><pre>{pending.current.source?.content}</pre><h4>{t('workspace.ide.submitted_content')}</h4><pre>{pending.current.pending?.content}</pre></details>}</div></div>}
    {conflict&&<div className="conflict-banner"><AlertCircle size={17}/><div><strong>{t('workspace.ide.conflict_title')}</strong><p>{t('workspace.ide.conflict_help')}</p><button disabled={!remote} onClick={()=>{if(remote){setFile(remote);setConflict(false);setRemote(undefined);setError(new UiError('workspace.ide.review_merge'));}}}>{t('workspace.ide.keep_draft')}</button><button disabled={!remote} onClick={()=>{if(remote){setContent(remote.content);setFile(remote);setConflict(false);setRemote(undefined);setError(undefined);}}}>{t('workspace.ide.load_latest')}</button></div></div>}
    {fileLoading?<Loading/>:file?<Editor height="100%" path={file.path} language={file.language} value={content} onChange={value=>{setContent(value||'');setSaved(false);}} theme={document.documentElement.dataset.theme==='dark'?'vs-dark':'light'} options={{ariaLabel:t('workspace.ide.editor_label'),fontSize:12,lineHeight:22,minimap:{enabled:false},padding:{top:18},scrollBeyondLastLine:false,automaticLayout:true,wordWrap:'on',tabSize:2,renderLineHighlight:'gutter',fontFamily:'Consolas, monospace'}} loading={<Loading/>}/>:<Empty title={t('workspace.ide.empty_title')} description={t('workspace.ide.empty_description')}/>}
   </div>
  </section>:<section className="panel settings-panel"><div className="panel-heading"><div><h2>{t('workspace.ide.bind_title')}</h2><p>{workspace?.reason?renderDiagnostic(workspace.reason):t('workspace.ide.unbound')}</p></div><Folder size={22}/></div><form onSubmit={e=>void bind(e)}><Field label={t('workspace.ide.absolute_path')} hint={t('workspace.ide.path_hint')}><input required value={binding} onChange={e=>setBinding(e.target.value)} placeholder="D:\projects\my-project"/></Field><button className="primary" disabled={busy}><Link size={15}/>{t('workspace.ide.bind')}</button></form></section>}
 </div>;
}
