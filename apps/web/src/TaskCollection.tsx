import {useCallback,useEffect,useId,useMemo,useRef,useState,type FormEvent,type ReactNode} from 'react';
import {ArrowRight,ClipboardList,ChevronRight,LayoutGrid,List,RefreshCw,Search} from 'lucide-react';
import {label,stateLabel,time,type RecordItem} from './api';
import {useApi} from './api-context';
import {formatDateTime,formatNumber,tr,useLocale} from './i18n';
import type {Iteration} from './planning-model';
import {taskIterationLabel} from './task-views-model';
import {withCurrentOption} from './record-options';
import {Badge,Empty,ErrorNotice,Loading} from './ui';
import {useTaskCollection,type TaskCollectionPageRead} from './task-collection-hooks';
import {projectTaskCollectionFilters,reconcileTaskCollectionPages,taskCollectionGroupBy,taskCollectionGroupKey,taskCollectionScopeKey,validTaskCollectionFilter,type TaskCollectionFilters,type TaskCollectionScope,type TaskPlanProjection} from './task-collection-model';
import {taskColumns} from './task-columns-model';
import './TaskColumnControls.css';
import './TaskCollection.css';
export type {TaskCollectionFilters} from './task-collection-model';

type Props={columns?:string[];project:string;userID:string;organizationID:string;refresh:number;filters:TaskCollectionFilters;onFilter:(patch:Partial<TaskCollectionFilters>)=>void;members:RecordItem[];requirements:RecordItem[];iterations?:Iteration[];moreMembers:boolean;moreRequirements:boolean;onMoreMembers:()=>void;onMoreRequirements:()=>void;lookupBusy:boolean;selected:string;selection:string[];selectionBusy?:boolean;onToggleSelection:(id:string,checked:boolean)=>void;onRows:(rows:RecordItem[],authorized:boolean,metadata?:TaskCollectionReadMetadata)=>void;onDenied:(error:unknown)=>void;onSelect:(id:string)=>void;actions?:ReactNode;blocked?:unknown;onRetryAuthority?:()=>void;controls?:ReactNode|null;embedded?:boolean};
type ReadProps=Props&{iterations:Iteration[]};
// Labels are optional; only task_plans can establish iteration membership.
const emptyIterationCatalog:Iteration[]=[];
export type TaskCollectionReadMetadata={items:RecordItem[]|undefined;total:number;as_of:string;next_cursor:string;loading:boolean;error:unknown;action:'first'|'more';groups:{key:string;total:number}[];group_by:string;task_plans:TaskPlanProjection[]};
const t=(key:string,values?:Record<string,unknown>)=>tr('task-collection.'+key,values);
const priorities=['urgent','high','medium','low'];

/** One mounted collection owns one exact account/project/filter/read epoch.
 * A new route never borrows previously loaded rows, columns or cursors. */
export function TaskCollection({iterations=emptyIterationCatalog,...incoming}:Props){
 const props={...incoming,iterations};
 const [retry,setRetry]=useState(0),{filters}=props;
 const authorityID=JSON.stringify([props.userID,props.organizationID,props.project]),currentAuthority=useRef(authorityID),denialScope=useRef(''),refreshEpoch=useRef(props.refresh),deniedCallback=useRef(props.onDenied),[authorityDenial,setAuthorityDenial]=useState<{scope:string;refresh:number;error:unknown}>();
 if(refreshEpoch.current!==props.refresh){refreshEpoch.current=props.refresh;denialScope.current='';}
 currentAuthority.current=authorityID;deniedCallback.current=props.onDenied;
 const deny=useCallback((error:unknown)=>{if(currentAuthority.current!==authorityID||denialScope.current===authorityID)return;denialScope.current=authorityID;setAuthorityDenial({scope:authorityID,refresh:refreshEpoch.current,error});deniedCallback.current(error);},[authorityID]);
 const scope=useMemo<TaskCollectionScope>(()=>({...projectTaskCollectionFilters(filters),project:props.project,userID:props.userID,organizationID:props.organizationID}),[props.project,props.userID,props.organizationID,filters.q,filters.layout,filters.task_status,filters.task_owner,filters.task_priority,filters.task_requirement,filters.task_iteration,filters.task_iteration_unset,filters.task_unassigned,filters.task_priority_unset,filters.task_status_unset,filters.task_sort,filters.task_group,filters.task_invalid]);
 const blocked=props.blocked??(authorityDenial?.scope===authorityID&&authorityDenial.refresh===props.refresh?authorityDenial.error:undefined),Shell=props.embedded?'div':'section';
 if(blocked)return <Shell className={(props.embedded?'':'panel ')+'task-collection'} aria-label={t('title')}><div className="task-collection-message"><ErrorNotice error={blocked} onRetry={()=>{denialScope.current='';setAuthorityDenial(undefined);setRetry(value=>value+1);props.onRetryAuthority?.();}}/><p role="status">{t('private_cleared')}</p></div></Shell>;
 return <TaskCollectionView key={JSON.stringify([taskCollectionScopeKey(scope),props.refresh,retry])} {...props} onDenied={deny} scope={scope} onRestart={()=>setRetry(value=>value+1)}/>;
}

function TaskCollectionView(props:ReadProps&{scope:TaskCollectionScope;onRestart:()=>void}){
 const [filtersOpen,setFiltersOpen]=useState(false),filtersID=useId();
 useLocale();const api=useApi(),read=useTaskCollection(api,props.scope,props.onDenied),{scope}=props;
 const groupBy=taskCollectionGroupBy(scope),groups=read.summary?.groups.filter(group=>group.total>0)??[];
 const reconciled=useMemo(()=>reconcileTaskCollectionPages(read.summary?.groups??[],read.pages,groupBy),[read.summary,read.pages,groupBy]);
 const rows=useMemo(()=>read.denied||!read.summary?[]:scope.layout==='board'?reconciled.rows:read.pages['']?.page?.items??[],[read.denied,read.summary,read.pages,scope.layout,reconciled]);
 const taskPlans=useMemo(()=>scope.layout==='board'?reconciled.task_plans:read.pages['']?.page?.task_plans??[],[scope.layout,reconciled,read.pages]);
 const plansByID=useMemo(()=>new Map(taskPlans.map(plan=>[plan.task_id,plan])),[taskPlans]);
 const keyOf=(row:RecordItem)=>taskCollectionGroupKey(row,groupBy,plansByID.get(row.id));
 const listGroups:{key:string;total?:number}[]=[...groups];
 for(const row of rows){const key=keyOf(row);if(key!==undefined&&!listGroups.some(group=>group.key===key))listGroups.push({key});}
 const metadata=useMemo<TaskCollectionReadMetadata>(()=>({items:read.summary&&!read.denied?rows:undefined,total:read.summary?.total??0,as_of:read.summary?.as_of??'',next_cursor:scope.layout==='board'?'':read.pages['']?.page?.next_cursor??'',loading:read.summaryLoading||read.groupsLoading||Object.values(read.pages).some(page=>page.loading),error:read.denied??read.summaryError??(scope.layout!=='board'?read.pages['']?.error:undefined)??null,action:'first',groups:read.summary?.groups??[],group_by:groupBy,task_plans:read.summary&&!read.denied?taskPlans:[]}),[taskPlans,rows,read.summary,read.denied,read.summaryError,read.summaryLoading,read.groupsLoading,read.pages,scope.layout,groupBy]);
 useEffect(()=>props.onRows(rows,!!read.summary&&!read.denied,metadata),[rows,read.summary,read.denied,metadata,props.onRows]);
 const groupName=(key:string)=>groupBy==='iteration'?taskIterationLabel(key,props.iterations):groupBy==='owner'?key?props.members.find(member=>member.id===key)?label(props.members.find(member=>member.id===key)!):key:tr('records.unassigned'):groupBy==='priority'?key?priorities.includes(key)?tr('common.priority.'+key):key:t('unset'):key?stateLabel(key):t('unset');
 const Shell=props.embedded?'div':'section';
 return <Shell className={(props.embedded?'':'panel records-panel ')+'task-collection'} aria-label={t('title')}>
  {props.controls===undefined?<>
   <div className="list-toolbar"><div className="inline"><span className="list-count">{t('matching')}<b>{read.summary?formatNumber(read.summary.total):'—'}</b></span><div className="segmented" role="group" aria-label={t('layout')}><button type="button" aria-label={tr('records.list_view')} aria-pressed={scope.layout!=='board'} className={scope.layout!=='board'?'active':''} onClick={()=>props.onFilter({layout:'list'})}><List size={15} aria-hidden="true"/></button><button type="button" aria-label={tr('records.board_view')} aria-pressed={scope.layout==='board'} className={scope.layout==='board'?'active':''} onClick={()=>props.onFilter({layout:'board'})}><LayoutGrid size={15} aria-hidden="true"/></button></div></div><div className="inline"><button type="button" onClick={props.onRestart} data-task-collection-refresh><RefreshCw size={15} aria-hidden="true"/>{t('refresh')}</button>{!read.denied&&props.actions}</div></div>
   {!read.denied&&<><button type="button" className="task-collection-controls-toggle" aria-expanded={filtersOpen} aria-controls={filtersID} onClick={()=>setFiltersOpen(open=>!open)}>{t('controls',{count:[scope.q,scope.task_status||scope.task_status_unset,scope.task_owner||scope.task_unassigned,scope.task_priority||scope.task_priority_unset,scope.task_requirement,scope.task_iteration||scope.task_iteration_unset].filter(Boolean).length})}</button><div id={filtersID} className={'task-collection-controls'+(filtersOpen?' expanded':'')}><TaskFilters {...props} groups={groupBy==='status'?groups:[]}/></div></>}
  </>:props.controls}
  {read.denied?<div className="task-collection-message"><ErrorNotice error={read.denied} onRetry={props.onRestart}/><p role="status">{t('private_cleared')}</p></div>:<>
   {read.summaryLoading&&<Loading/>}
   {Boolean(read.summaryError)&&<div className="task-collection-message"><ErrorNotice error={read.summaryError} onRetry={props.onRestart}/></div>}
   {read.summary&&<>
    <details className="task-collection-observed"><summary>{t('read_scope')}</summary><p>{t('summary_scope',{total:formatNumber(read.summary.total),at:observed(read.summary.as_of)})}</p><small>{t('live_reads')}</small></details>
    {(read.summary.next_group_cursor||read.groupsError)&&<div className="task-collection-groups-footer"><p>{t('groups_loaded',{loaded:formatNumber(read.summary.groups.length),total:formatNumber(read.summary.groups_total)})}</p>{Boolean(read.groupsError)&&<ErrorNotice error={read.groupsError} onRetry={read.groupsResetRequired?props.onRestart:()=>void read.readGroups(true)}/>}<button type="button" data-task-groups-more disabled={read.groupsLoading||read.groupsResetRequired} onClick={()=>void read.readGroups(true)}>{read.groupsLoading?t('loading_more'):t('more_groups')}</button></div>}
    {read.summary.total>0&&<p className="task-collection-selection-help">{t('selection_help')}</p>}
    {!read.summary.total?<Empty title={t('empty')} description={t('empty_hint')}/>:scope.layout==='board'?<div className="task-collection-board" aria-label={t('board')} tabIndex={0}>{groups.map(group=><section key={group.key} className="task-collection-column" aria-label={groupName(group.key)} data-task-column={group.key}><header><h3>{groupBy==='status'&&group.key?<Badge state={group.key}/>:groupName(group.key)}</h3><span>{formatNumber(group.total)}</span><small className="task-collection-group-asof">{t('read_at',{at:observed(read.groupAsOf[group.key])})}</small></header><PageContent read={read.pages[group.key]} status={group.key} board props={props} onRead={read.readPage} visibleRows={reconciled.rowsByGroup.get(group.key)} changed={reconciled.changed.has(group.key)||!!read.pages[group.key]?.page&&read.pages[group.key]!.page!.total!==group.total}/></section>)}</div>:<>
     {read.pages['']?.page?<>
      {listGroups.filter(group=>rows.some(row=>keyOf(row)===group.key)).map(group=><details className="task-collection-list-group" key={group.key} open><summary><strong>{groupName(group.key)}</strong><span>{t(group.total===undefined?'group_unconfirmed':'group_loaded',{loaded:formatNumber(rows.filter(row=>keyOf(row)===group.key).length),...(group.total===undefined?{}:{total:formatNumber(group.total)})})}</span></summary><PageContent read={read.pages['']} status="" props={props} onRead={read.readPage} visibleRows={rows.filter(row=>keyOf(row)===group.key)} hideFooter/></details>)}
      {(read.pages['']!.page!.total!==read.summary.total||listGroups.some(group=>group.total===undefined))&&<p className="task-collection-live-change" role="status">{t('live_change')}</p>}
      <PageFooter read={read.pages['']!} status="" onRead={read.readPage}/>
     </>:<PageContent read={read.pages['']} status="" props={props} onRead={read.readPage}/>}
    </>}
   </>}
  </>}
 </Shell>;
}

function TaskFilters(props:ReadProps&{groups:{key:string;total:number}[]}){
 const [draft,setDraft]=useState(()=>projectTaskCollectionFilters(props.filters)),[invalid,setInvalid]=useState(false);
 const owner=draft.task_unassigned?'unset':draft.task_owner?'value:'+draft.task_owner:'';
 const statuses=[...new Set([...props.groups.map(group=>group.key).filter(Boolean),...(props.filters.task_status?[props.filters.task_status]:[])])];
 function submit(event:FormEvent){event.preventDefault();if(!validTaskCollectionFilter(draft)){setInvalid(true);return;}setInvalid(false);props.onFilter(projectTaskCollectionFilters(draft));}
 return <form className="task-collection-filters" aria-label={t('filters')} onSubmit={submit}>
  <div className="task-collection-query"><label htmlFor="task-collection-q">{t('search')}</label><div className="filter-input"><Search size={15} aria-hidden="true"/><input id="task-collection-q" name="task-collection-q" placeholder={t('search_hint')} value={draft.q} onChange={event=>setDraft({...draft,q:event.target.value})}/></div></div>
  <label><span>{t('status')}</span><select name="task-collection-status" value={draft.task_status_unset?'unset':draft.task_status?'value:'+draft.task_status:''} onChange={event=>setDraft({...draft,task_status:event.target.value.startsWith('value:')?event.target.value.slice(6):'',task_status_unset:event.target.value==='unset'})}><option value="">{t('all_statuses')}</option><option value="unset">{t('unset')}</option>{statuses.map(status=><option key={status} value={'value:'+status}>{stateLabel(status)}</option>)}</select></label>
  <label><span>{t('owner')}</span><select name="task-collection-owner" value={owner} onChange={event=>setDraft({...draft,task_owner:event.target.value.startsWith('value:')?event.target.value.slice(6):'',task_unassigned:event.target.value==='unset'})}><option value="">{t('all_owners')}</option><option value="unset">{tr('records.unassigned')}</option>{withCurrentOption(props.members,draft.task_owner,t('owner')).map(member=><option key={member.id} value={'value:'+member.id}>{label(member)}{member.version===0?' · '+member.id:''}</option>)}</select>{props.moreMembers&&<button type="button" disabled={props.lookupBusy} onClick={props.onMoreMembers}>{t('more_owners')}</button>}</label>
  <label><span>{t('priority')}</span><select name="task-collection-priority" value={draft.task_priority_unset?'unset':draft.task_priority?'value:'+draft.task_priority:''} onChange={event=>setDraft({...draft,task_priority:event.target.value.startsWith('value:')?event.target.value.slice(6):'',task_priority_unset:event.target.value==='unset'})}><option value="">{t('all_priorities')}</option><option value="unset">{t('unset')}</option>{[...new Set([...priorities,...(draft.task_priority?[draft.task_priority]:[])])].map(priority=><option key={priority} value={'value:'+priority}>{priorities.includes(priority)?tr('common.priority.'+priority):priority}</option>)}</select></label>
  <label><span>{t('requirement')}</span><select name="task-collection-requirement" value={draft.task_requirement||''} onChange={event=>setDraft({...draft,task_requirement:event.target.value})}><option value="">{t('all_requirements')}</option>{withCurrentOption(props.requirements,draft.task_requirement,t('requirement')).map(requirement=><option key={requirement.id} value={requirement.id}>{label(requirement)}{requirement.version===0?' · '+requirement.id:''}</option>)}</select>{props.moreRequirements&&<button type="button" disabled={props.lookupBusy} onClick={props.onMoreRequirements}>{t('more_requirements')}</button>}</label>
  <label className="task-iteration-filter"><span>{t('iteration')}</span><select name="task-collection-iteration" value={draft.task_iteration_unset?'unset':draft.task_iteration?'value:'+draft.task_iteration:''} onChange={event=>setDraft({...draft,task_iteration:event.target.value.startsWith('value:')?event.target.value.slice(6):'',task_iteration_unset:event.target.value==='unset'})}><option value="">{t('all_iterations')}</option><option value="unset">{t('iteration_unset')}</option>{draft.task_iteration&&!props.iterations.some(item=>item.id===draft.task_iteration)&&<option value={'value:'+draft.task_iteration}>{taskIterationLabel(draft.task_iteration,props.iterations)}</option>}{props.iterations.map(item=><option key={item.id} value={'value:'+item.id}>{taskIterationLabel(item.id,props.iterations)}</option>)}</select><small>{t('iteration_scope')}</small></label>
  <label><span>{tr('task-views.group')}</span><select name="task-collection-group" value={draft.task_group||'status'} onChange={event=>setDraft({...draft,task_group:event.target.value})}>{(['status','owner','priority','iteration'] as const).map(group=><option key={group} value={group}>{tr(group==='iteration'?'task-collection.group_iteration':'task-views.group_'+group)}</option>)}</select></label>
  <small className="task-collection-picker-scope">{t('picker_scope')}</small>
  <div className="task-collection-filter-actions"><button className="primary" type="submit">{t('apply')}</button><button type="button" onClick={()=>props.onFilter({q:'',task_invalid:undefined,task_status:'',task_owner:'',task_priority:'',task_requirement:'',task_iteration:'',task_iteration_unset:false,task_unassigned:false,task_status_unset:false,task_priority_unset:false})}>{t('clear')}</button></div>
  {invalid&&<p className="task-collection-filter-error" role="alert">{t('invalid_filters')}</p>}
 </form>;
}

function PageContent({read,status,board=false,props,onRead,visibleRows,changed,hideFooter=false}:{read:TaskCollectionPageRead|undefined;status:string;board?:boolean;props:ReadProps;onRead:(status:string,more?:boolean)=>Promise<void>;visibleRows?:RecordItem[];changed?:boolean;hideFooter?:boolean}){
 if(!read||read.loading&&!read.page)return <Loading/>;
 if(read.error)return <div className="task-collection-message"><ErrorNotice error={read.error} onRetry={()=>void onRead(status)}/><p>{t('column_retry')}</p></div>;
 const page=read.page;if(!page)return null;const items=visibleRows??page.items;const columns=taskColumns(props.columns);
 return <>
  {changed&&<p className="task-collection-live-change" role="status">{t('live_change')}</p>}
  {!items.length?<p className="task-collection-message">{t('changed_empty')}</p>:board?<div className="task-collection-cards">{items.map(task=><TaskBoardCard key={task.id} task={task} plans={page.task_plans} props={props}/>)}</div>:!columns?<p role="alert" className="task-collection-message">{tr('task-columns.invalid')}</p>:<div className="table-scroll"><table className="task-columns-table"><thead><tr><th scope="col" className="task-team-selection-cell">{tr('expert-team.select')}</th>{columns.map(id=><th key={id} scope="col" data-task-column={id}>{tr('task-columns.column.'+id)}</th>)}<th scope="col"><span className="sr-only">{t('open')}</span></th></tr></thead><tbody>{items.map(task=><tr key={task.id} data-task-collection-row={task.id} data-task-id={task.id} className={props.selected===task.id?'selected-row':''}><td className="task-team-selection-cell"><TaskSelectionCheckbox task={task} props={props}/></td>{columns.map(id=><td key={id} data-task-column={id} className={id==='updated'?'muted nowrap':undefined}>{id==='title'?<button type="button" className="row-title" onClick={()=>props.onSelect(task.id)}><span className="row-icon"><ClipboardList size={17} aria-hidden="true"/></span><span>{label(task)}<small>{task.id}</small><TaskIteration taskID={task.id} plans={page.task_plans} iterations={props.iterations}/></span></button>:id==='status'?<Badge state={task.status}/>:id==='priority'?<TaskPriority value={task.priority}/>:id==='owner'?<TaskOwner task={task} members={props.members}/>:time(task.updated_at)}</td>)}<td><button type="button" className="icon-button" aria-label={t('open_named',{name:label(task)})} onClick={()=>props.onSelect(task.id)}><ChevronRight size={16} aria-hidden="true"/></button></td></tr>)}</tbody></table></div>}
  {!hideFooter&&<PageFooter read={read} status={status} onRead={onRead}/>}
 </>;
}
function TaskSelectionCheckbox({task,props}:{task:RecordItem;props:Pick<Props,'selection'|'selectionBusy'|'onToggleSelection'>}){
 const checked=props.selection.includes(task.id);
 return <input type="checkbox" aria-label={tr('expert-team.select_task',{title:label(task)})} checked={checked}
  disabled={props.selectionBusy||!checked&&props.selection.length>=16}
  onChange={event=>props.onToggleSelection(task.id,event.target.checked)}/>;
}

function TaskBoardCard({task,plans,props}:{task:RecordItem;plans:TaskPlanProjection[];props:ReadProps}){
 return <article className="task-collection-card" data-task-id={task.id} data-batch-selected={props.selection.includes(task.id)||undefined}>
  <label className="task-collection-card-selection"><TaskSelectionCheckbox task={task} props={props}/></label>
  <button type="button" className="task-collection-card-open" aria-label={t('open_named',{name:label(task)})}
   aria-pressed={props.selected===task.id} onClick={()=>props.onSelect(task.id)}>
   <span className="task-collection-card-status"><Badge state={task.status}/></span>
   <strong>{label(task)}</strong>
   <TaskIteration taskID={task.id} plans={plans} iterations={props.iterations}/>
   {task.description&&<span className="task-collection-card-description">{task.description.slice(0,120)}</span>}
   <span className="task-collection-card-facts"><TaskPriority value={task.priority}/><TaskOwner task={task} members={props.members}/></span>
   <small>{task.id}</small>
  </button>
 </article>;
}

function PageFooter({read,status,onRead}:{read:TaskCollectionPageRead;status:string;onRead:(key:string,more?:boolean)=>Promise<void>}){
 const page=read.page;if(!page)return null;
 return <div className="task-collection-page-footer"><p aria-live="polite">{t('loaded',{loaded:formatNumber(page.items.length),total:formatNumber(page.total)})}<small>{t('read_at',{at:observed(page.as_of)})}</small></p>{page.next_cursor&&<button type="button" data-task-collection-more={status||'all'} disabled={read.loading} onClick={()=>void onRead(status,true)}>{read.loading?t('loading_more'):t('more')}<ArrowRight size={14} aria-hidden="true"/></button>}</div>;
}

function TaskPriority({value}:{value?:string}){return <span className={'priority '+(priorities.includes(value||'')?value:'')}>{value?priorities.includes(value)?tr('common.priority.'+value):value:t('unset')}</span>;}
function TaskOwner({task,members}:{task:RecordItem;members:RecordItem[]}){const owner=members.find(member=>member.id===task.owner_id);return <span className={task.owner_id?'owner-cell':'muted'}>{task.owner_id?owner?label(owner):task.owner_id:tr('records.unassigned')}</span>;}
function observed(value:string){return formatDateTime(value,{month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',second:'2-digit'});}

function TaskIteration({taskID,plans,iterations}:{taskID:string;plans:TaskPlanProjection[];iterations:Iteration[]}){const plan=plans.find(item=>item.task_id===taskID);return plan?<small className="task-iteration-context" title={plan.iteration_id||undefined}>{t('iteration')}: {taskIterationLabel(plan.iteration_id,iterations)}</small>:null;}
