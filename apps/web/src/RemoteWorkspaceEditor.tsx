import {useEffect,useRef,useState} from 'react';
import type {editor as MonacoEditor} from 'monaco-editor';
import Editor,{DiffEditor,type OnMount} from '@monaco-editor/react';
import type {WorkspaceObservation,WorkspaceObservationCreateRequest,WorkspaceObservationRead,WorkspaceWriteCommand,WorkspaceWriteCreateRequest} from '../../../packages/contracts/src';
import {ApiError,operation,query,type ApiCaller} from './api';
import {useApi} from './api-context';
import {formatDateTime,formatNumber,tr,UiError,useLocale} from './i18n';
import {ErrorNotice,Loading} from './ui';
import {remoteFileLanguage,validRemoteObservation} from './remote-workspace-model';
import {editableRemoteSource,makeRemoteWrite,remoteContentSHA,remoteWritePending,validRemoteText,validRemoteWrite} from './remote-workspace-write-model';
import {bindWorkspaceSaveShortcut,type WorkspaceSaveShortcutState} from './workspace-save-shortcut';

const t=(key:string,values?:Record<string,unknown>)=>tr('remote-workspace-write.'+key,values);
type Pending={api:ApiCaller;body:WorkspaceWriteCreateRequest;command?:WorkspaceWriteCommand;uncertain?:boolean};

/** Drafts and uncertain save identities belong to this exact actor/scope. The
 * parent retains this editor across source-tab changes and clears it on a view
 * identity change; source text never enters persistent browser storage. */
export function RemoteWorkspaceEditor({project,source,active,onClose,onAccessLost,onRecorded}:{project:string;source:WorkspaceObservation;active:boolean;onClose:()=>void;onAccessLost:()=>void;onRecorded:()=>void}){
 const api=useApi(),locale=useLocale();
 const originalApi=useRef(api);
 const comparisonWidget=useRef<MonacoEditor.IStandaloneDiffEditor|undefined>(undefined),comparisonModels=useRef<MonacoEditor.IDiffEditorModel|undefined>(undefined);
 const ariaListeners=useRef<{dispose():void}[]>([]);
 const initial=source.result!.read!;
 const [base,setBase]=useState(initial),[draft,setDraft]=useState(initial.content),[busy,setBusy]=useState(false),[command,setCommand]=useState<WorkspaceWriteCommand>(),[error,setError]=useState<unknown>(),[conflict,setConflict]=useState(false),[comparison,setComparison]=useState<WorkspaceObservation>(),[comparing,setComparing]=useState(false),[reviewed,setReviewed]=useState(false),[navigationBlocked,setNavigationBlocked]=useState(false);
 const inspection=useRef<{project_id:string;expected_version:number;idempotency_key:string}|undefined>(undefined);
 const pending=useRef<Pending|undefined>(undefined),running=useRef(false),epoch=useRef(0),abort=useRef<AbortController|undefined>(undefined),timer=useRef<ReturnType<typeof setTimeout>|undefined>(undefined),alive=useRef(true);
 const callbacks=useRef({onAccessLost,onRecorded});callbacks.current={onAccessLost,onRecorded};
 const mounted=useRef<{editor:Parameters<OnMount>[0];runtime:Parameters<OnMount>[1]}|undefined>(undefined);
 const shortcut=useRef<WorkspaceSaveShortcutState>({hasFile:false,dirty:false,pending:false,busy:false,conflict:false,save:()=>{}}),action=useRef<{dispose():void}|undefined>(undefined);
 const dirty=draft!==base.content,uncertain=!!pending.current;
 function stop(){epoch.current++;abort.current?.abort();if(timer.current)clearTimeout(timer.current);}
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;stop();action.current?.dispose();disposeComparison();};},[]);
 useEffect(()=>{
  const guard=(event:Event)=>{if(dirty||uncertain){event.preventDefault();setNavigationBlocked(true);}};
  const unload=(event:BeforeUnloadEvent)=>{if(dirty||uncertain){event.preventDefault();event.returnValue='';}};
  window.addEventListener('suma:navigate',guard);window.addEventListener('beforeunload',unload);
  return()=>{window.removeEventListener('suma:navigate',guard);window.removeEventListener('beforeunload',unload);};
 },[dirty,uncertain]);

 // A draft outlives an observation TTL, but not a revoked current read scope.
 // Poll metadata only; do not repeatedly transfer file bodies while typing.
 useEffect(()=>{
  if(!active)return;
  const controller=new AbortController();let next:ReturnType<typeof setTimeout>|undefined;
  async function reauthorize(){
   try{await api('/workspace/observations/'+encodeURIComponent(source.id)+query(project)+'&metadata=true',undefined,'GET',undefined,controller.signal);}
   catch(reason){if(!controller.signal.aborted&&reason instanceof ApiError&&[401,403,404].includes(reason.status)){callbacks.current.onAccessLost();return;}}
   if(!controller.signal.aborted)next=setTimeout(()=>void reauthorize(),3000);
  }
  void reauthorize();return()=>{controller.abort();if(next)clearTimeout(next);};
 },[api,project,source.id,active]);

 function accept(value:WorkspaceWriteCommand,request:Pending){
  if(!validRemoteWrite(value,request.body,request.command))throw new UiError('remote-workspace-write.invalid_receipt');
  const changed=!request.command||request.command.version!==value.version;
  request.command=value;setCommand(value);if(changed)callbacks.current.onRecorded();
  if(value.state==='applied'){
   // A user may type the next change while an earlier save is in flight.
   // Confirm the submitted baseline without replacing that newer draft.
   setBase({...base,content:request.body.content,sha256:request.body.content_sha256,bytes:new TextEncoder().encode(request.body.content).length});
   pending.current=undefined;setConflict(false);setComparison(undefined);setError(undefined);
  }else if(value.state==='not_applied'||value.state==='expired'){
   pending.current=undefined;
   setConflict(value.error_code==='file_changed');
   setError(new UiError('remote-workspace-write.error.'+(value.error_code||'permit_expired')));
  }
 }

 async function recover(request:Pending){
  stop();const generation=epoch.current,controller=new AbortController();abort.current=controller;
  const current=()=>alive.current&&!controller.signal.aborted&&epoch.current===generation&&pending.current===request;
  async function receive(){
   try{
    const value=await request.api<WorkspaceWriteCommand>(request.command?'/workspace/writes/'+encodeURIComponent(request.command.id)+query(project):'/workspace/writes',request.command?undefined:request.body,request.command?'GET':'POST',undefined,controller.signal);
    if(!current())return;accept(value,request);setBusy(false);running.current=false;
    if(pending.current===request&&remoteWritePending(value))timer.current=setTimeout(()=>void receive(),1000);
   }catch(reason){if(current()){
    // A first explicit refusal proves this create did not commit. Once any
    // reply is uncertain, later refusals cannot erase the original identity.
    if(!request.command&&!request.uncertain&&reason instanceof ApiError&&reason.status>=400&&reason.status<500)pending.current=undefined;
    else request.uncertain=true;
    setError(reason);setBusy(false);running.current=false;
   }}
  }
  await receive();
 }

 async function save(){
  if(running.current||conflict)return;
  running.current=true;setBusy(true);setError(undefined);
  if(pending.current){
   const request=pending.current;
   if(request.command?.state==='reconciling'&&!remoteWritePending(request.command)){
    inspection.current??={project_id:project,expected_version:request.command.version,idempotency_key:operation()};
    try{const value=await request.api<WorkspaceWriteCommand>('/workspace/writes/'+encodeURIComponent(request.command.id)+'/inspect',inspection.current);if(!alive.current)return;accept(value,request);inspection.current=undefined;if(pending.current===request)await recover(request);else{setBusy(false);running.current=false;}}
    catch(reason){if(alive.current){setError(reason);setBusy(false);running.current=false;}}
   }else await recover(request);
   return;
  }
  try{
   if(!validRemoteText(draft))throw new UiError('remote-workspace-write.content_limit');
   const content=draft,sha=await remoteContentSHA(content);
   if(!alive.current)return;
   const request:Pending={api,body:makeRemoteWrite(project,source.scope,source.workspace_policy_sha256!,base,content,sha,operation())};
   pending.current=request;await recover(request);
  }catch(reason){if(alive.current){setError(reason);setBusy(false);running.current=false;}}
 }
 shortcut.current={hasFile:active,dirty,pending:uncertain,busy,conflict:conflict||!!comparison,save};
 const mountEditor:OnMount=(editor,runtime)=>{mounted.current={editor,runtime};action.current?.dispose();action.current=bindWorkspaceSaveShortcut(editor,runtime,()=>shortcut.current,t('save'));};
 useEffect(()=>{if(mounted.current)mountEditor(mounted.current.editor,mounted.current.runtime);},[locale]);

 async function readComparison(){
  if(pending.current||comparing)return;
  stop();const generation=epoch.current,controller=new AbortController();abort.current=controller;setComparing(true);setError(undefined);setComparison(undefined);setReviewed(false);
  const body:WorkspaceObservationCreateRequest={project_id:project,scope:source.scope,operation:'read',arguments:{path:base.path,full_file:true},idempotency_key:operation()};
  let id='';
  async function receive(){
   try{
    const value=await api<WorkspaceObservation>(id?'/workspace/observations/'+encodeURIComponent(id)+query(project):'/workspace/observations',id?undefined:body,id?'GET':'POST',undefined,controller.signal);
    if(!alive.current||controller.signal.aborted||epoch.current!==generation)return;
    if(!validRemoteObservation(value,project,body.scope,'read',body.arguments,'',id||undefined))throw new UiError('remote-workspace-write.invalid_receipt');
    id=value.id;
    if(value.state==='requested'||value.state==='leased'){timer.current=setTimeout(()=>void receive(),1000);return;}
    if(value.state!=='completed'||!await editableRemoteSource(value))throw new UiError('remote-workspace-write.comparison_unavailable');
    if(!alive.current||controller.signal.aborted||epoch.current!==generation)return;
    setComparison(value);setComparing(false);
   }catch(reason){if(alive.current&&!controller.signal.aborted&&epoch.current===generation){setError(reason);setComparing(false);}}
  }
  await receive();
 }
 // The React wrapper disposes models before disposing its DiffEditor by
 // default. Own these unique models explicitly: detach the widget first,
 // then release the pair. This also covers navigation/authority-loss unmounts.
 function disposeComparison(){
  for(const listener of ariaListeners.current)listener.dispose();ariaListeners.current=[];
  const widget=comparisonWidget.current,pair=comparisonModels.current;
  comparisonWidget.current=undefined;comparisonModels.current=undefined;
  if(widget?.getModel())widget.setModel(null);
  if(pair?.original&&!pair.original.isDisposed())pair.original.dispose();
  if(pair?.modified&&!pair.modified.isDisposed())pair.modified.dispose();
 }
 // Monaco 0.52 can reset inner labels on unrelated DiffEditor option
 // updates. Use its public configuration event to retain our current locale;
 // the equality guard prevents a feedback loop and listeners share model life.
 function protectComparisonNames(widget:MonacoEditor.IStandaloneDiffEditor,runtime:Parameters<OnMount>[1]){
  for(const [instance,key] of [[widget.getOriginalEditor(),'comparison_original'],[widget.getModifiedEditor(),'comparison_modified']] as const){
   const option=runtime.editor.EditorOption.ariaLabel;
   const apply=()=>{const label=t(key);if(instance.getOption(option)!==label)instance.updateOptions({ariaLabel:label});};
   ariaListeners.current.push(instance.onDidChangeConfiguration(event=>{if(event.hasChanged(option))apply();}));apply();
  }
 }
 function adoptComparison(){
  if(!comparison?.result?.read||!reviewed)return;
  disposeComparison();setBase(comparison.result.read);setComparison(undefined);setReviewed(false);setConflict(false);setCommand(undefined);setError(undefined);
 }
 function close(){if(!busy&&(!pending.current||pending.current.command)){stop();pending.current=undefined;onClose();}}
 const status=command?.state;
 if(originalApi.current!==api)return <ErrorNotice error={new UiError('remote-workspace-write.access_lost')}/>;
 return <section className="remote-write-editor" aria-label={t('editor_title')}>
  <div className="remote-filebar"><strong>{base.path}</strong><span>{dirty?t('unsaved'):t('draft_clean')}</span></div>
  <div className="remote-write-toolbar"><button type="button" disabled={busy||conflict||!!comparison||(!dirty&&!uncertain)} onClick={()=>void save()}>{busy?t('submitting'):uncertain?t('recover'):t('save')}</button><button type="button" disabled={busy||(uncertain&&!command)} onClick={close}>{uncertain?t('close_pending'):dirty?t('discard'):t('close')}</button></div>
  <p className="muted">{t('draft_basis',{time:formatDateTime(source.observed_at||source.created_at,{dateStyle:'short',timeStyle:'medium'})})}</p>
  {navigationBlocked&&<p role="status">{t('navigation_blocked')}</p>}
  {!!error&&<ErrorNotice error={error}/>}
  {status&&<p role="status">{t('state.'+status)}</p>}
  {command&&<details><summary>{t('receipt')}</summary><dl><dt>{t('command')}</dt><dd><code>{command.id}</code></dd><dt>{t('desired_sha')}</dt><dd><code>{command.content_sha256}</code></dd>{command.result&&<><dt>{t('generation')}</dt><dd>{formatNumber(command.result.generation)}</dd></>}</dl></details>}
  {status==='reconciling'&&<p>{t('reconciliation_help')}</p>}{uncertain&&command&&<p className="muted">{t('close_pending_help')}</p>}
  {comparing&&<Loading/>}
  {comparison?.result?.read?<>
   <p>{t('compare_help')}</p>
   <DiffEditor height="420px" keepCurrentOriginalModel keepCurrentModifiedModel originalModelPath={'suma-remote-compare://'+comparison.id+'/original/'+encodeURIComponent(base.path)} modifiedModelPath={'suma-remote-compare://'+comparison.id+'/modified/'+encodeURIComponent(base.path)} language={remoteFileLanguage(base.path)} theme={globalThis.document.documentElement.dataset.theme==='dark'?'vs-dark':'light'} original={comparison.result.read.content} modified={draft} options={{readOnly:false,originalEditable:false,originalAriaLabel:t('comparison_original'),modifiedAriaLabel:t('comparison_modified'),automaticLayout:true,renderSideBySide:true,minimap:{enabled:false}}} onMount={(editor,runtime)=>{comparisonWidget.current=editor;comparisonModels.current=editor.getModel()??undefined;protectComparisonNames(editor,runtime);mountEditor(editor.getModifiedEditor(),runtime);editor.getModifiedEditor().onDidChangeModelContent(()=>{setDraft(editor.getModifiedEditor().getValue());setReviewed(false);});}} loading={<Loading/>}/>
   <label className="check"><input type="checkbox" checked={reviewed} onChange={event=>setReviewed(event.target.checked)}/>{t('reviewed')}</label><button type="button" disabled={!reviewed} onClick={adoptComparison}>{t('adopt_basis')}</button>
  </>:<Editor height="420px" language={remoteFileLanguage(base.path)} theme={globalThis.document.documentElement.dataset.theme==='dark'?'vs-dark':'light'} path={'suma-remote-draft://'+source.id+'/'+encodeURIComponent(base.path)} value={draft} onChange={value=>{setDraft(value??'');setNavigationBlocked(false);}} onMount={mountEditor} options={{readOnly:false,ariaLabel:t('editor'),automaticLayout:true,minimap:{enabled:false},wordWrap:'on',scrollBeyondLastLine:false}} loading={<Loading/>}/>}
  {(conflict||dirty)&&!comparison&&!uncertain&&<button type="button" disabled={comparing} onClick={()=>void readComparison()}>{t('compare_latest')}</button>}
  <p className="muted">{t('quality_boundary')}</p>
 </section>;
}
