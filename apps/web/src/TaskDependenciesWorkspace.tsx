import {lazy,Suspense,useCallback,useEffect,useMemo,useRef,useState} from 'react';
import {ReactFlow,Background,Controls,Handle,Position,applyNodeChanges,MarkerType,type Node,type NodeProps,type Connection,type OnSelectionChangeParams,type NodeChange,type ReactFlowInstance} from '@xyflow/react';
import ELK from 'elkjs/lib/elk.bundled.js';
import {ArrowLeft,GitBranch,LayoutGrid,List,Plus,RefreshCw,Users,X} from 'lucide-react';
import {ApiError,query,type RecordItem,type Session} from './api';
import {useApi} from './api-context';
import {tr,useLocale,formatNumber,UiError} from './i18n';
import {Badge,Empty,ErrorNotice,Field,Loading} from './ui';
import {useLeaveGuard,useWriteAction,WriteButton,WriteNotice} from './write-action';
import {toolSessionStorage} from './tool-hooks';
import {flowAriaLabelConfig,flowHandleLabel} from './flow-localization';
import {validTaskDependencyGraph,taskDependencyDraft,taskDependencyProblem,taskDependencyChanges,validTaskDependencyRecovery,taskDependencyReceipt,taskDependencyReceiptCurrent,taskDependencyRebase,type TaskDependencyGraph,type TaskDependencyDraft,type TaskDependencyChangeResult,type TaskDependencyChange} from './task-dependencies-model';
import {TaskDeliveryPanel} from './TaskDeliveryPanel';
import './TaskDependenciesWorkspace.css';

export type TaskDeliveryRunTarget={run:string;node?:string;attempt?:string};
export type TaskDependenciesWorkspaceProps={session:Session;project:string;refresh:number;onTasks:()=>void;onOpenTask?:(id:string)=>void;onOpenRun?:(target:TaskDeliveryRunTarget)=>void;onOpenIssue?:(id:string)=>void;onOpenRequirement?:(id:string)=>void;teamPlanID?:string;onTeamPlan?:(id:string)=>void;onExtractAcceptedDelivery?:(taskID:string)=>void;onChanged?:()=>void};
const TeamTaskDispatchDialog=lazy(()=>import('./TeamTaskDispatchDialog').then(module=>({default:module.TeamTaskDispatchDialog})));
type TaskFlowData={title:string;status:string;version:number;marked:boolean};
type TaskNode=Node<TaskFlowData>;
function DependencyNode({id,data,selected}:NodeProps<TaskNode>){useLocale();return <div className={'task-dependency-node'+(selected?' selected':'')+(data.marked?' marked':'')}><Handle type="target" position={Position.Left} aria-label={flowHandleLabel('input',data.title,id)}/><span className="task-dependency-node-heading"><GitBranch size={15}/><small>{tr('task-dependencies.version',{version:formatNumber(data.version)})}</small></span><strong title={data.title}>{data.title}</strong><div className="inline wrap"><Badge state={data.status}/>{data.marked&&<span className="task-dependencies-marked-label">{tr('task-dependencies.marked')}</span>}</div><Handle type="source" position={Position.Right} aria-label={flowHandleLabel('output',data.title,id)}/></div>;}
const nodeTypes={task:DependencyNode};
const elk=new ELK();

/** Layout is presentation only: changing edges never moves the user's viewport
 * or silently publishes dependencies. Stale layout results are ignored. */
function DependencyCanvas({graph,draft,selected,markedIDs,editable,onSelect,onChange,onRemove}:{graph:TaskDependencyGraph;draft:TaskDependencyDraft;selected:string;markedIDs:string[];editable:boolean;onSelect:(id:string)=>void;onChange:(source:string,target:string,remove?:boolean)=>void;onRemove:(edges:{source:string;target:string}[])=>void}){
 const locale=useLocale();
 const [nodes,setNodes]=useState<TaskNode[]>([]),[edgeSelection,setEdgeSelection]=useState(''),[layout,setLayout]=useState(0),[layoutError,setLayoutError]=useState(false);
 const flow=useRef<ReactFlowInstance<TaskNode>|undefined>(undefined),layoutReady=useRef(false),fitFrame=useRef<number|undefined>(undefined);
 // React Flow calls this when its selected-node set changes. Its identity must
 // not depend on the inspector selection: a new callback can report the old
 // controlled nodes before their selection effect has caught up.
 const selectionChanged=useCallback(({nodes}:OnSelectionChangeParams<TaskNode>)=>{if(nodes[0])onSelect(nodes[0].id);},[onSelect]);
 const nodesChanged=useCallback((changes:NodeChange<TaskNode>[])=>setNodes(current=>applyNodeChanges(changes.filter(change=>change.type!=='remove'),current)),[]);
 const fitGraph=useCallback(()=>{
  if(fitFrame.current!==undefined)cancelAnimationFrame(fitFrame.current);
  fitFrame.current=requestAnimationFrame(()=>{fitFrame.current=undefined;void flow.current?.fitView({padding:0.16,maxZoom:1,duration:typeof matchMedia==='function'&&matchMedia('(prefers-reduced-motion: reduce)').matches?0:180});});
 },[]);
 const initialized=useCallback((instance:ReactFlowInstance<TaskNode>)=>{flow.current=instance;if(layoutReady.current)fitGraph();},[fitGraph]);
 useEffect(()=>()=>{if(fitFrame.current!==undefined)cancelAnimationFrame(fitFrame.current);},[]);
 const edges=useMemo(()=>{const ids=new Set(graph.tasks.map(task=>task.task_id));return graph.tasks.flatMap(task=>(draft[task.task_id]||[]).filter(source=>ids.has(source)).map(source=>({id:JSON.stringify([source,task.task_id]),source,target:task.task_id,ariaLabel:tr('development.workflow.graph_edge_aria',{source,target:task.task_id}),domAttributes:{'aria-roledescription':tr('flow.edge.role')},selected:edgeSelection===JSON.stringify([source,task.task_id]),type:'smoothstep',markerEnd:{type:MarkerType.ArrowClosed},style:{stroke:'var(--muted)',strokeWidth:1.6}})));},[graph,draft,edgeSelection,locale]);
 const structure=JSON.stringify(graph.tasks.map(task=>task.task_id));
 useEffect(()=>{setNodes(previous=>{const positions=new Map(previous.map(node=>[node.id,node.position]));return graph.tasks.map((task,index)=>({id:task.task_id,type:'task',ariaLabel:task.title,domAttributes:{'aria-roledescription':tr('flow.node.role')},position:positions.get(task.task_id)||{x:(index%4)*280,y:Math.floor(index/4)*145},data:{title:task.title,status:task.status,version:task.version,marked:markedIDs.includes(task.task_id)},selected:task.task_id===selected,deletable:false}));});},[graph,selected,markedIDs,locale]);
 useEffect(()=>{
  let active=true;layoutReady.current=false;setLayoutError(false);
  const fallback=graph.tasks.map((task,index)=>({id:task.task_id,x:(index%4)*280,y:Math.floor(index/4)*145}));
  void elk.layout({id:'tasks',layoutOptions:{'elk.algorithm':'layered','elk.direction':'RIGHT','elk.spacing.nodeNode':'40','elk.layered.spacing.nodeNodeBetweenLayers':'85'},children:graph.tasks.map(task=>({id:task.task_id,width:228,height:112})),edges:edges.map(edge=>({id:edge.id,sources:[edge.source],targets:[edge.target]}))}).then(result=>{
   if(!active)return;const positions=new Map((result.children||fallback).map(node=>[node.id,{x:node.x??0,y:node.y??0}]));
   setNodes(previous=>previous.map(node=>({...node,position:positions.get(node.id)||node.position})));layoutReady.current=true;fitGraph();
  }).catch(()=>{if(active)setLayoutError(true);});
  return ()=>{active=false;};
  // Deliberately exclude draft/selection: a saved layout is stable while editing.
 },[structure,layout]);
 const connect=({source,target}:Connection)=>{if(source&&target)onChange(source,target);};
 return <div className="task-dependencies-canvas-wrap"><div className="task-dependencies-canvas-toolbar"><span>{tr('task-dependencies.direction')}</span><button type="button" onClick={()=>setLayout(value=>value+1)}><LayoutGrid size={15}/>{tr('task-dependencies.arrange')}</button></div>{layoutError&&<p role="status" className="muted">{tr('task-dependencies.layout_failed')}</p>}<div className="task-dependencies-canvas" data-dependencies-canvas>
  <ReactFlow<TaskNode> nodes={nodes} edges={edges} nodeTypes={nodeTypes} onInit={initialized} onNodesChange={nodesChanged} onConnect={connect} onNodeClick={(_,node)=>onSelect(node.id)} onSelectionChange={selectionChanged} onEdgeClick={(_,edge)=>setEdgeSelection(edge.id)} onPaneClick={()=>setEdgeSelection('')} onEdgesDelete={removed=>{if(editable)onRemove(removed);}} nodesConnectable={editable} edgesReconnectable={false} edgesFocusable nodesDraggable nodesFocusable elementsSelectable deleteKeyCode={editable?['Backspace','Delete']:null} onBeforeDelete={async({edges})=>({nodes:[],edges:editable?edges:[]})} onlyRenderVisibleElements minZoom={0.12} maxZoom={1.8} ariaLabelConfig={flowAriaLabelConfig(editable)} proOptions={{hideAttribution:true}}><Background gap={22}/><Controls showInteractive={false}/></ReactFlow>
 </div></div>;
}

/** Remounting on identity/project changes keeps late reads and durable original
 * writes out of a new workspace. An authorization retry also starts a fresh read. */
export function TaskDependenciesWorkspace(props:TaskDependenciesWorkspaceProps){const [retry,setRetry]=useState(0);return <TaskDependenciesScope key={JSON.stringify([props.session.user.id,props.session.organization.id,props.project,retry])} {...props} retryRead={()=>setRetry(value=>value+1)}/>;}
function TaskDependenciesScope({session,project,refresh,onTasks,onOpenTask,onOpenRun,onOpenIssue,onOpenRequirement,teamPlanID,onTeamPlan,onExtractAcceptedDelivery,onChanged,retryRead}:TaskDependenciesWorkspaceProps&{retryRead:()=>void}){
 useLocale();const api=useApi();
 const [base,setBase]=useState<TaskDependencyGraph>(),[draft,setDraft]=useState<TaskDependencyDraft>({}),[latest,setLatest]=useState<TaskDependencyGraph>(),[selected,setSelected]=useState(''),[source,setSource]=useState(''),[reading,setReading]=useState(false),[readError,setReadError]=useState<unknown>(),[denied,setDenied]=useState(false),[notice,setNotice]=useState(''),[stale,setStale]=useState(false),[ack,setAck]=useState<TaskDependencyChangeResult>(),[review,setReview]=useState(false);
 const [view,setView]=useState<'graph'|'table'>(()=>typeof matchMedia==='function'&&matchMedia('(max-width: 760px)').matches?'table':'graph');
 const [markedIDs,setMarkedIDs]=useState<string[]>([]),[dispatchTasks,setDispatchTasks]=useState<RecordItem[]>([]),[teamOpen,setTeamOpen]=useState(false),[locatedTeamPlan,setLocatedTeamPlan]=useState(''),[deliveryRefresh,setDeliveryRefresh]=useState(0);
 const alive=useRef(true),readSequence=useRef(0),controller=useRef<AbortController|undefined>(undefined);
 const action=useWriteAction({scope:JSON.stringify(['task-dependencies',session.user.id,session.organization.id,project]),storage:toolSessionStorage(),validateRequest:request=>validTaskDependencyRecovery(request,project),validateResponse:(value,request)=>taskDependencyReceipt(value,request,project)});
 const changes=useMemo(()=>base?taskDependencyChanges(base,draft):[],[base,draft]),dirty=changes.length>0,problem=base?.complete?taskDependencyProblem(base,draft):undefined;
 const blocked=reading||action.busy||!!action.pending||action.conflict||review||!!latest||denied;
 useLeaveGuard(dirty||review||!!latest,action.setError);
 useEffect(()=>{alive.current=true;return ()=>{alive.current=false;readSequence.current++;controller.current?.abort();};},[]);
 useEffect(()=>{if(action.errorStatus===401||action.errorStatus===403)setDenied(true);},[action.errorStatus]);
 const read=useCallback(async()=>{
  const sequence=++readSequence.current;controller.current?.abort();const abort=new AbortController();controller.current=abort;setReading(true);setReadError(undefined);
  try{const value=await api<unknown>('/task-dependencies'+query(project),undefined,'GET',undefined,abort.signal);if(!alive.current||sequence!==readSequence.current)return;if(!validTaskDependencyGraph(value,project))throw new UiError('task-dependencies.invalid_read');return value;}
  catch(error){if(!alive.current||sequence!==readSequence.current)return;if(error instanceof ApiError&&(error.status===401||error.status===403))setDenied(true);setReadError(error);}
  finally{if(alive.current&&sequence===readSequence.current)setReading(false);}
 },[api,project]);
 const adopt=(graph:TaskDependencyGraph,next=taskDependencyDraft(graph))=>{setBase(graph);setDraft(next);setLatest(undefined);setAck(undefined);setReview(false);setStale(false);setMarkedIDs(current=>current.filter(id=>graph.tasks.some(task=>task.task_id===id&&task.status!=='accepted')));setSelected(current=>graph.tasks.some(task=>task.task_id===current)?current:graph.tasks[0]?.task_id||'');setSource('');};
 const state=useRef({base,dirty,pending:!!action.pending,review,latest});state.current={base,dirty,pending:!!action.pending,review,latest};
 useEffect(()=>{const current=state.current;if(current.base&&(current.dirty||current.pending||current.review||current.latest)){setStale(true);return;}void read().then(graph=>{
  if(!graph)return;const next=taskDependencyDraft(graph);
  // A restored write retains its original envelope. Showing its desired edges
  // does not replace that envelope with current task versions.
  const original=action.pending?.body.changes as TaskDependencyChange[]|undefined;
  for(const change of original||[])if(Object.hasOwn(next,change.task_id))next[change.task_id]=[...change.depends_on];adopt(graph,next);
 });},[read,refresh]);
 const lookup=useMemo(()=>new Map(base?.tasks.map(task=>[task.task_id,task])||[]),[base]);
 const intent=ack?ack.changes.map(change=>({task_id:change.task_id,expected_version:change.previous_version,depends_on:change.depends_on})):changes;
 const compareLatest=async()=>{const graph=await read();if(graph){setLatest(graph);setNotice('');}};
 const confirm=async(result:TaskDependencyChangeResult)=>{
  setAck(result);setReview(true);setNotice('');const graph=await read();if(!graph)return;
  if(taskDependencyReceiptCurrent(result,graph)){adopt(graph);action.clearConflict();setNotice('task-dependencies.saved');}else setLatest(graph);
 };
 const save=async()=>{
  if(action.pending){const original=action.pending,result=await action.run<TaskDependencyChangeResult>(original.path,original.body,original.method);if(result&&alive.current)await confirm(result);return;}
  if(!base||!base.complete||blocked||problem||!dirty||changes.length>100)return;
  const result=await action.run<TaskDependencyChangeResult>('/task-dependencies',{project_id:project,changes});if(result&&alive.current)await confirm(result);
 };
 const changeDependency=(from:string,to:string,remove=false)=>{
  if(!base||blocked||!lookup.has(to))return;
  const next={...draft,[to]:remove?(draft[to]||[]).filter(id=>id!==from):[...new Set([...(draft[to]||[]),from])].sort()},problem=taskDependencyProblem(base,next);setNotice('');
  if(problem&&!remove){action.setError(new UiError('task-dependencies.problem_'+problem));return;}action.setError('');setDraft(next);
 };
 const keepDraft=()=>{
  if(!base||!latest?.complete||action.pending)return;
  let next:TaskDependencyDraft|undefined;
  if(ack){next=taskDependencyDraft(latest);if(intent.some(change=>!Object.hasOwn(next!,change.task_id)))next=undefined;else for(const change of intent)next[change.task_id]=[...change.depends_on];}
  else next=taskDependencyRebase(base,draft,latest);
  if(!next){action.setError(new UiError('task-dependencies.removed_task'));return;}
  adopt(latest,next);action.clearConflict();setNotice('task-dependencies.rebased');
 };
 const discard=()=>{if(base&&!blocked){setDraft(taskDependencyDraft(base));setNotice('');action.setError('');}};
 const canDispatch=!!base?.complete&&markedIDs.length>0&&markedIDs.length<=16&&!blocked&&!dirty&&!stale&&!readError;
 const toggleMarked=(id:string)=>{if(blocked||!lookup.has(id)||lookup.get(id)!.status==='accepted')return;setMarkedIDs(current=>current.includes(id)?current.filter(value=>value!==id):current.length<16?[...current,id]:current);};
 const dispatchTeam=()=>{if(!canDispatch||!base)return;setDispatchTasks(markedIDs.map(id=>{const task=lookup.get(id)!;return {id:task.task_id,project_id:project,version:task.version,title:task.title,status:task.status,depends_on:[...task.depends_on]};}));setLocatedTeamPlan('');onTeamPlan?.('');setTeamOpen(true);};
 const originalPending=!!action.pending;
 if(denied)return <section className="panel task-dependencies"><h2>{tr('task-dependencies.title')}</h2><WriteNotice action={action}/>{Boolean(readError)&&<ErrorNotice error={readError}/>}<p>{tr('task-dependencies.access_changed')}</p><button type="button" onClick={retryRead}>{tr('common.retry')}</button></section>;
 return <section className="task-dependencies" data-task-dependencies><header className="task-dependencies-heading"><div><div className="task-dependencies-eyebrow"><GitBranch size={16}/>{tr('task-dependencies.eyebrow')}</div><h2>{tr('task-dependencies.title')}</h2><p className="muted">{tr('task-dependencies.intro')}</p></div><button type="button" onClick={onTasks}><ArrowLeft size={16}/>{tr('task-dependencies.back')}</button></header>
  <WriteNotice action={action}/>{Boolean(readError)&&<ErrorNotice error={readError} onRetry={!base?retryRead:undefined}/>}
  {originalPending&&<div className="task-dependencies-notice" role="status"><strong>{tr('task-dependencies.pending')}</strong><p>{tr('task-dependencies.pending_help')}</p><code>{action.pending!.idempotency_key}</code><form data-dependencies-restore onSubmit={event=>{event.preventDefault();void save();}}><WriteButton action={action}>{tr('task-dependencies.restore')}</WriteButton></form></div>}
  {(action.conflict||review||latest)&&!originalPending&&<section className="task-dependencies-review" data-dependencies-review aria-label={tr('task-dependencies.review_title')}><h3>{tr(review?'task-dependencies.ack_title':'task-dependencies.review_title')}</h3><p>{tr(review?'task-dependencies.ack_help':'task-dependencies.review_help')}</p><button type="button" data-dependencies-read-latest disabled={reading||action.busy} onClick={()=>void compareLatest()}><RefreshCw size={15}/>{tr('task-dependencies.read_latest')}</button>
   {latest?.complete&&<><div className="task-dependencies-comparison"><table><caption>{tr('task-dependencies.comparison')}</caption><thead><tr><th>{tr('task-dependencies.task')}</th><th>{tr('task-dependencies.local')}</th><th>{tr('task-dependencies.current')}</th></tr></thead><tbody>{intent.map(change=>{const current=latest.tasks.find(task=>task.task_id===change.task_id);return <tr key={change.task_id}><th scope="row">{lookup.get(change.task_id)?.title||change.task_id}</th><td>{change.depends_on.map(id=>lookup.get(id)?.title||id).join(', ')||tr('task-dependencies.none')}</td><td>{current?<>{tr('task-dependencies.version',{version:formatNumber(current.version)})} · {current.depends_on.map(id=>latest.tasks.find(task=>task.task_id===id)?.title||id).join(', ')||tr('task-dependencies.none')}</>:tr('task-dependencies.task_removed')}</td></tr>;})}</tbody></table></div><div className="inline wrap"><button type="button" data-dependencies-keep disabled={reading||action.busy} onClick={keepDraft}>{tr('task-dependencies.keep')}</button><button type="button" data-dependencies-use-latest disabled={reading||action.busy} onClick={()=>{adopt(latest);action.clearConflict();setNotice('');}}>{tr('task-dependencies.use_latest')}</button></div></>}
   {latest&&!latest.complete&&<><p>{tr('task-dependencies.limit',{total:formatNumber(latest.total),limit:formatNumber(latest.limit)})}</p><button type="button" data-dependencies-use-latest disabled={reading||action.busy} onClick={()=>{adopt(latest);action.clearConflict();setNotice('');}}>{tr('task-dependencies.use_latest')}</button></>}
  </section>}
  {reading&&!base&&<Loading/>}
  {base&&!base.complete&&<Empty title={tr('task-dependencies.limit_title')} description={tr('task-dependencies.limit',{total:formatNumber(base.total),limit:formatNumber(base.limit)})} action={<button type="button" onClick={onTasks}>{tr('task-dependencies.back')}</button>}/>}
  {base?.complete&&base.tasks.length===0&&<Empty title={tr('task-dependencies.empty')} description={tr('task-dependencies.empty_help')} action={<button type="button" onClick={onTasks}>{tr('task-dependencies.back')}</button>}/>}
  {base?.complete&&base.tasks.length>0&&<><div className="task-dependencies-toolbar"><div className="task-dependencies-switch" role="group" aria-label={tr('task-dependencies.view')}><button type="button" aria-pressed={view==='graph'} onClick={()=>setView('graph')}><GitBranch size={16}/>{tr('task-dependencies.graph')}</button><button type="button" aria-pressed={view==='table'} onClick={()=>setView('table')}><List size={16}/>{tr('task-dependencies.table')}</button></div><span className="muted">{tr('task-dependencies.count',{count:formatNumber(base.total),dirty:formatNumber(changes.length)})}</span><div className="task-dependencies-actions"><button type="button" data-dependencies-dispatch disabled={!canDispatch} onClick={dispatchTeam}><Users size={15}/>{tr('task-dependencies.dispatch',{count:formatNumber(markedIDs.length)})}</button><button type="button" data-dependencies-refresh disabled={blocked||dirty} onClick={()=>void read().then(graph=>{if(graph){adopt(graph);setNotice('');}})} aria-label={tr('task-dependencies.refresh')}><RefreshCw size={15}/></button><button type="button" data-dependencies-discard disabled={!dirty||blocked} onClick={discard}>{tr('task-dependencies.discard')}</button><form data-dependencies-save onSubmit={event=>{event.preventDefault();void save();}}><WriteButton action={action} disabled={blocked||!dirty||!!problem||changes.length>100}>{tr('task-dependencies.save')}</WriteButton></form></div></div>
   {!!markedIDs.length&&<p className="task-dependencies-selection-note" role="status">{tr('task-dependencies.selection_help',{count:formatNumber(markedIDs.length)})} <button type="button" disabled={blocked} onClick={()=>setMarkedIDs([])}>{tr('task-dependencies.clear_selection')}</button>{(dirty||blocked||stale||!!readError)&&<span>{tr('task-dependencies.dispatch_blocked')}</span>}</p>}
   {stale&&<p className="task-dependencies-notice" role="status">{tr('task-dependencies.stale')} <button type="button" disabled={reading||action.busy||originalPending} onClick={()=>void compareLatest()}>{tr('task-dependencies.read_latest')}</button></p>}
   {notice&&<p role="status" className="task-dependencies-notice">{tr(notice)}</p>}{problem&&<p role="alert" className="error">{tr('task-dependencies.problem_'+problem)}</p>}{changes.length>100&&<p role="alert" className="error">{tr('task-dependencies.batch_limit')}</p>}
   <div className="task-dependencies-body"><div className="task-dependencies-main">{view==='graph'?<DependencyCanvas graph={base} draft={draft} selected={selected} markedIDs={markedIDs} editable={!blocked} onSelect={setSelected} onChange={changeDependency} onRemove={edges=>{if(blocked)return;const next={...draft};for(const edge of edges)next[edge.target]=(next[edge.target]||[]).filter(id=>id!==edge.source);setDraft(next);setNotice('');action.setError('');}}/>:<div className="task-dependencies-table"><table><caption>{tr('task-dependencies.direction')}</caption><thead><tr><th className="task-dependencies-pick-column"><span className="sr-only">{tr('task-dependencies.pick')}</span></th><th>{tr('task-dependencies.task')}</th><th>{tr('task-dependencies.status')}</th><th>{tr('task-dependencies.predecessors')}</th></tr></thead><tbody>{base.tasks.map(task=><tr key={task.task_id} data-selected={task.task_id===selected}><td className="task-dependencies-pick-column"><input type="checkbox" data-dependencies-pick={task.task_id} aria-label={tr('task-dependencies.pick_task',{title:task.title})} checked={markedIDs.includes(task.task_id)} disabled={blocked||task.status==='accepted'||!markedIDs.includes(task.task_id)&&markedIDs.length>=16} onChange={()=>toggleMarked(task.task_id)}/></td><th scope="row"><button type="button" className="task-dependencies-select" aria-pressed={task.task_id===selected} onClick={()=>setSelected(task.task_id)}>{task.title}</button><small>{tr('task-dependencies.version',{version:formatNumber(task.version)})}</small></th><td><Badge state={task.status}/></td><td>{(draft[task.task_id]||[]).map(id=>lookup.get(id)?.title||id).join(', ')||tr('task-dependencies.none')}</td></tr>)}</tbody></table></div>}</div>
    <aside className="task-dependencies-inspector" aria-label={tr('task-dependencies.edit_task')}><Field label={tr('task-dependencies.task')}><select name="dependency_task" value={selected} onChange={event=>{setSelected(event.target.value);setSource('');}}>{base.tasks.map(task=><option key={task.task_id} value={task.task_id}>{task.title}</option>)}</select></Field>{lookup.get(selected)&&<><div className="inline wrap"><Badge state={lookup.get(selected)!.status}/><span className="muted">{tr('task-dependencies.version',{version:formatNumber(lookup.get(selected)!.version)})}</span></div><button type="button" data-dependencies-mark aria-pressed={markedIDs.includes(selected)} disabled={blocked||lookup.get(selected)!.status==='accepted'||!markedIDs.includes(selected)&&markedIDs.length>=16} onClick={()=>toggleMarked(selected)}><Users size={15}/>{tr(markedIDs.includes(selected)?'task-dependencies.unmark':'task-dependencies.mark')}</button><h3>{tr('task-dependencies.predecessors')}</h3><p className="muted">{tr('task-dependencies.predecessor_help')}</p><ul className="task-dependencies-chips">{(draft[selected]||[]).map(id=><li key={id}><span>{lookup.get(id)?.title||id}</span><button type="button" data-dependency-remove={id} disabled={blocked} aria-label={tr('task-dependencies.remove',{title:lookup.get(id)?.title||id})} onClick={()=>changeDependency(id,selected,true)}><X size={14}/></button></li>)}</ul>{!(draft[selected]||[]).length&&<p className="muted">{tr('task-dependencies.none')}</p>}<Field label={tr('task-dependencies.add_predecessor')}><select name="dependency_source" value={source} disabled={blocked} onChange={event=>setSource(event.target.value)}><option value="">{tr('task-dependencies.choose')}</option>{base.tasks.filter(task=>task.task_id!==selected&&!(draft[selected]||[]).includes(task.task_id)).map(task=><option key={task.task_id} value={task.task_id}>{task.title}</option>)}</select></Field><button type="button" data-dependency-add disabled={blocked||!source||source===selected||(draft[selected]||[]).includes(source)} onClick={()=>{changeDependency(source,selected);setSource('');}}><Plus size={15}/>{tr('task-dependencies.add')}</button>{onOpenTask&&<button type="button" className="task-dependencies-open" disabled={dirty||originalPending||review||!!latest} onClick={()=>onOpenTask(selected)}>{tr('task-dependencies.open')}</button>}</>}<p className="task-dependencies-boundary">{tr('task-dependencies.boundary')}</p>{selected&&<TaskDeliveryPanel session={session} project={project} taskId={selected} taskVersion={lookup.get(selected)!.version} refresh={refresh+deliveryRefresh} navigationDisabled={blocked||dirty||!!readError} onOpenRun={onOpenRun} onOpenIssue={onOpenIssue} onOpenRequirement={onOpenRequirement} onExtractAcceptedDelivery={onExtractAcceptedDelivery} onTeamPlan={id=>{setLocatedTeamPlan(id);setTeamOpen(true);onTeamPlan?.(id);}} onVersionMismatch={()=>setStale(true)} onForbidden={()=>setDenied(true)}/>}</aside>
   </div></>}
  {(teamOpen||!!teamPlanID)&&<Suspense fallback={<Loading/>}><TeamTaskDispatchDialog project={project} session={session} tasks={dispatchTasks} open planID={teamPlanID||locatedTeamPlan||undefined} onPlan={id=>{setLocatedTeamPlan(id);onTeamPlan?.(id);}} onClose={()=>{setTeamOpen(false);setLocatedTeamPlan('');onTeamPlan?.('');}} onStarted={()=>{if(onChanged)onChanged();else{setDeliveryRefresh(value=>value+1);void read().then(graph=>{if(graph)adopt(graph);});}}} onForbidden={()=>setDenied(true)}/></Suspense>}
 </section>;
}
