import {useCallback,useEffect,useRef,useState,type KeyboardEvent} from 'react';
import {Activity,AlertCircle,ArrowRight,Box,CalendarDays,Check,ChevronRight,ClipboardCheck,Clock3,GitBranch,Layers,MessageSquare,Minus,Pause,Play,Plus,Shield,ShieldAlert,ShieldCheck,SquarePen,Sparkles,Users} from 'lucide-react';
import {api,ApiError,query,time} from '../api';
import {Badge,Empty,ErrorNotice,Loading} from '../ui';
import {formatDateTime,formatNumber,tr,useLocale} from '../i18n';
import {useProjectRead} from './useProjectRead';
import {acceptancePercent,activityPresentation,attentionKinds,emptyOverview,overviewCount,validAttentionPage,type AttentionKind,type AttentionPage,type OverviewActivity,type OverviewData} from './overview-model';
import './Overview.css';

type Destination='overview'|'requirements'|'development'|'quality'|'operations'|'knowledge'|'settings';
type OverviewProps={project:string;refresh:number;navigate:(view:Destination,sub?:string)=>void;openRecord:(id:string)=>void;onOpenRisks:()=>void;onOpenApplications:()=>void;onIntelligent?:()=>void};
const t=(key:string,values?:Record<string,string>)=>tr('project-overview.'+key,values);
async function readOverview(project:string){return api<OverviewData>('/overview'+query(project));}
const journey=[{key:'requirements',icon:Layers,view:'requirements'},{key:'collaboration',icon:Users,view:'settings',sub:'members'},{key:'workflow',icon:GitBranch,view:'development',sub:'workflows'},{key:'acceptance',icon:ShieldCheck,view:'quality'}] as const;
function countText(value:unknown){const count=overviewCount(value);return count===undefined?'—':formatNumber(count);}
function dateText(value:string){return formatDateTime(value+'T00:00:00Z',{year:'numeric',month:'short',day:'numeric',timeZone:'UTC'});}

export function Overview({project,refresh,navigate,openRecord,onOpenRisks,onOpenApplications,onIntelligent}:OverviewProps){
 useLocale();const [localRefresh,setLocalRefresh]=useState(0),generation=refresh+':'+localRefresh;
 const read=useCallback(async(scope:string)=>({generation,data:await readOverview(scope)}),[generation]),snapshot=useProjectRead(project,refresh+localRefresh,read);
 if(snapshot.error)return <ErrorNotice error={snapshot.error}/>;
 if(!snapshot.data||snapshot.data.generation!==generation)return <Loading/>;
 return <OverviewContent key={project+':'+generation} data={snapshot.data.data} project={project} navigate={navigate} openRecord={openRecord} onOpenRisks={onOpenRisks} onOpenApplications={onOpenApplications} onIntelligent={onIntelligent} onRefresh={()=>setLocalRefresh(value=>value+1)}/>;
}

function OverviewContent({data,project,navigate,openRecord,onOpenRisks,onOpenApplications,onIntelligent,onRefresh}:Omit<OverviewProps,'refresh'>&{data:OverviewData;onRefresh:()=>void}){
 const [selected,setSelected]=useState<AttentionKind>('review'),[accessError,setAccessError]=useState<unknown>(),[deliveryStale,setDeliveryStale]=useState(false);
 const delivery=data.delivery?.scope==='whole_project'?data.delivery:undefined,percent=acceptancePercent(delivery),milestones=data.milestones;
 const metrics=[{key:'accepted',value:delivery?.accepted_tasks,icon:Check},{key:'running',value:delivery?.running_tasks,icon:Play},{key:'review',value:delivery?.review_tasks,icon:ClipboardCheck,tab:'review'},{key:'blocked',value:delivery?.blocked_tasks,icon:Pause,tab:'blocked'}] as const;
 if(accessError)return <ErrorNotice error={accessError}/>;
 return <div className="project-overview">
  <header className="po-header"><div><span className="eyebrow">{tr('workbench.overview.eyebrow')}</span><h1>{data.project?.title||data.project?.name||t('project')}</h1><p>{data.project?.description||t('description')}</p></div><div className="po-header-actions"><button type="button" className="primary" onClick={()=>navigate('requirements')}><Plus size={15} aria-hidden="true"/>{t('new_requirement')}</button>{onIntelligent&&<button type="button" className="intent-entry" aria-haspopup="dialog" onClick={onIntelligent}><Sparkles size={15} aria-hidden="true"/>{tr('intent-operations.title')}</button>}</div></header>
  <div className="po-layout"><div className="po-main">
   <section className="panel po-delivery" data-overview-delivery aria-label={t('delivery')}>
    <div className="po-section-heading"><div><h2>{t('delivery')}</h2><p>{t('delivery_scope')}</p></div><button type="button" className="text-button" onClick={()=>navigate('development','tasks')}>{t('all_tasks')}<ArrowRight size={14} aria-hidden="true"/></button></div>
    {deliveryStale&&<div className="po-delivery-stale" role="status"><AlertCircle size={17} aria-hidden="true"/><div><strong>{t('delivery_stale')}</strong><p>{t('delivery_stale_hint')}</p></div><button type="button" onClick={onRefresh}>{t('refresh_project')}</button></div>}
    <div className="po-progress-heading"><div><strong>{countText(delivery?.accepted_tasks)}</strong><span>{t('accepted_of',{total:countText(delivery?.scope_tasks)})}</span></div><span>{percent===undefined?t(overviewCount(delivery?.scope_tasks)===0?'no_scope':'not_reported'):formatNumber(percent,{maximumFractionDigits:1})+'%'}</span></div>
    {percent!==undefined?<div className="po-progress" role="progressbar" aria-label={t('acceptance_progress')} aria-valuenow={percent} aria-valuemin={0} aria-valuemax={100} aria-valuetext={t('accepted_progress',{accepted:countText(delivery?.accepted_tasks),total:countText(delivery?.scope_tasks)})}><span style={{width:percent+'%'}}/></div>:<div className="po-progress po-progress-unavailable" aria-hidden="true"/>}
    <p className="po-scope-note">{t('scope_counts',{total:countText(delivery?.total_tasks),cancelled:countText(delivery?.cancelled_tasks)})}</p>
    <div className="po-metrics">{metrics.map(metric=>{const content=<><span className="po-metric-label"><metric.icon size={15} aria-hidden="true"/>{t('metric.'+metric.key)}</span><strong>{countText(metric.value)}</strong><small>{t('metric_hint.'+metric.key)}</small></>;return 'tab'in metric?<button type="button" className={'po-metric po-metric-'+metric.key} key={metric.key} onClick={()=>{setSelected(metric.tab);document.getElementById('overview-attention-panel')?.scrollIntoView?.({block:'nearest'});}}>{content}</button>:<div className={'po-metric po-metric-'+metric.key} key={metric.key}>{content}</div>;})}</div>
    <div className="po-delivery-foot"><span>{t('waiting_count',{count:countText(delivery?.waiting_tasks)})}</span><span>{t('reconciling_count',{count:countText(delivery?.reconciling_tasks)})}</span>{data.as_of&&<time dateTime={data.as_of}>{t('as_of',{at:time(data.as_of)})}</time>}</div>
   </section>
   <AttentionQueue project={project} data={data} selected={selected} onSelect={setSelected} openRecord={openRecord} navigate={navigate} onDenied={setAccessError} onQueueChanged={()=>setDeliveryStale(true)}/>
   <section className="panel po-activity"><div className="po-section-heading"><div><h2>{tr('workbench.overview.activity')}</h2><p>{t('activity_scope')}</p></div><Activity size={17} aria-hidden="true"/></div>{data.activity?.length?<div className="po-activity-list">{data.activity.slice(0,8).map((item,index)=><ActivityRow item={item} key={item.id||index}/>)}</div>:<Empty title={tr('workbench.overview.no_activity')} description={tr('workbench.overview.no_activity_hint')}/>}</section>
  </div><aside className="po-aside" aria-label={t('project_context')}>
   <section className="panel po-milestones"><div className="po-section-heading"><div><h2>{t('milestones')}</h2><p>{t('milestone_scope')}</p></div><CalendarDays size={17} aria-hidden="true"/></div>{milestones&&Array.isArray(milestones.items)&&overviewCount(milestones.total)!==undefined?milestones.items.length?<><ul>{milestones.items.map(item=><li key={item.id}><span className="po-milestone-dot"/><div><strong>{item.title}</strong><time dateTime={item.target_date}>{dateText(item.target_date)}</time></div><Badge state={item.status}/></li>)}</ul><p className="po-aside-note">{t('milestone_count',{loaded:formatNumber(milestones.items.length),total:formatNumber(milestones.total)})}</p></>:<p className="po-empty-copy">{t('no_milestones')}</p>:<p className="po-empty-copy">{t('not_reported')}</p>}<button type="button" className="po-section-link" onClick={()=>navigate('development','planning')}>{t('open_planning')}<ArrowRight size={14} aria-hidden="true"/></button></section>
   <section className="panel po-context-card"><ShieldAlert size={19} aria-hidden="true"/><h2>{t('risks')}</h2><p>{t('risks_hint')}</p><button type="button" className="po-section-link" onClick={onOpenRisks}>{t('open_risks')}<ArrowRight size={14} aria-hidden="true"/></button></section>
   <section className="panel po-context-card"><Box size={19} aria-hidden="true"/><h2>{t('applications')}</h2><p>{t('applications_hint')}</p><button type="button" className="po-section-link" onClick={onOpenApplications}>{t('open_applications')}<ArrowRight size={14} aria-hidden="true"/></button></section>
  </aside></div>
  {emptyOverview(data)&&<section className="quick-path po-first-steps"><span className="eyebrow">{tr('workbench.overview.journey')}</span><div>{journey.map((step,index)=><button type="button" onClick={()=>navigate(step.view,'sub'in step?step.sub:undefined)} key={step.key}><span className="path-number">{formatNumber(index+1,{minimumIntegerDigits:2})}</span><step.icon size={21} aria-hidden="true"/><span><strong>{tr('workbench.overview.step.'+step.key)}</strong><small>{tr('workbench.overview.step_hint.'+step.key)}</small></span><ChevronRight size={16} aria-hidden="true"/></button>)}</div></section>}
 </div>;
}

function AttentionQueue({project,data,selected,onSelect,openRecord,navigate,onDenied,onQueueChanged}:{project:string;data:OverviewData;selected:AttentionKind;onSelect:(kind:AttentionKind)=>void;openRecord:(id:string)=>void;navigate:OverviewProps['navigate'];onDenied:(error:unknown)=>void;onQueueChanged:()=>void}){
 const [pages,setPages]=useState<Partial<Record<AttentionKind,AttentionPage>>>(()=>Object.fromEntries(attentionKinds.flatMap(kind=>validAttentionPage(data.attention?.[kind],kind)?[[kind,data.attention![kind]]]:[])));
 const [busy,setBusy]=useState(false),[error,setError]=useState(false),[denied,setDenied]=useState(false),[queueChanged,setQueueChanged]=useState(false);const request=useRef(0),mounted=useRef(true),tabRefs=useRef<Partial<Record<AttentionKind,HTMLButtonElement|null>>>({}),current=useRef(selected);current.current=selected;
 useEffect(()=>{request.current++;setBusy(false);setError(false);setQueueChanged(false);},[selected]);
 useEffect(()=>{mounted.current=true;return()=>{mounted.current=false;request.current++;};},[]);
 const page=denied?undefined:pages[selected];
 function select(kind:AttentionKind){request.current++;setBusy(false);setError(false);setQueueChanged(false);onSelect(kind);}
 function keyDown(event:KeyboardEvent<HTMLButtonElement>,kind:AttentionKind){const index=attentionKinds.indexOf(kind),next=event.key==='ArrowRight'?attentionKinds[(index+1)%attentionKinds.length]:event.key==='ArrowLeft'?attentionKinds[(index+attentionKinds.length-1)%attentionKinds.length]:event.key==='Home'?attentionKinds[0]:event.key==='End'?attentionKinds[attentionKinds.length-1]:undefined;if(next){event.preventDefault();select(next);tabRefs.current[next]?.focus();}}
 async function loadMore(){
  if(busy||denied||!page?.next_cursor)return;const generation=++request.current,kind=selected,cursor=page.next_cursor,path='/overview/attention'+query(project)+'&kind='+kind;let changed=false;setBusy(true);setError(false);setQueueChanged(false);
  try{
   const next=await api<AttentionPage>(path+'&cursor='+encodeURIComponent(cursor)+'&limit=6');
   if(!mounted.current)return;
   if(validAttentionPage(next,kind)&&next.total!==page.total)onQueueChanged();
   if(generation!==request.current||current.current!==kind)return;
   if(!validAttentionPage(next,kind))throw new Error('invalid_attention_page');
   if(next.total!==page.total){
    changed=true;
    // Continuation reads are current facts, not an immutable list. Refresh once
    // when membership changes rather than attaching them to stale task rows.
    const first=await api<AttentionPage>(path+'&limit=6');
    if(generation!==request.current||current.current!==kind)return;
    if(!validAttentionPage(first,kind))throw new Error('invalid_attention_page');
    setPages(previous=>({...previous,[kind]:first}));setQueueChanged(true);
   }else{
    if(next.next_cursor===cursor)throw new Error('invalid_attention_page');
    setPages(previous=>{const old=previous[kind];if(!old||old.next_cursor!==cursor)return previous;const existing=new Set(old.items.map(item=>item.id));return {...previous,[kind]:{...next,items:[...old.items,...next.items.filter(item=>!existing.has(item.id))]}};});
   }
  }
  catch(reason){if(!mounted.current)return;if(reason instanceof ApiError&&[401,403,404].includes(reason.status)){setDenied(true);setPages({});onDenied(reason);return;}if(generation===request.current){if(changed)setPages(previous=>({...previous,[kind]:undefined}));setError(true);}}
  finally{if(generation===request.current)setBusy(false);}
 }
 return <section className="panel po-attention" aria-label={t('attention')}><div className="po-section-heading"><div><h2>{t('attention')}</h2><p>{t('attention_scope')}</p></div><button type="button" className="text-button" onClick={()=>navigate('development','tasks')}>{t('all_tasks')}<ArrowRight size={14} aria-hidden="true"/></button></div>
  <div className="po-tabs" role="tablist" aria-label={t('attention_tabs')}>{attentionKinds.map(kind=><button type="button" key={kind} id={'overview-tab-'+kind} ref={node=>{tabRefs.current[kind]=node;}} role="tab" aria-selected={selected===kind} aria-controls="overview-attention-panel" tabIndex={selected===kind?0:-1} onClick={()=>select(kind)} onKeyDown={event=>keyDown(event,kind)}>{t('queue.'+kind)}<span>{countText(denied?undefined:pages[kind]?.total)}</span></button>)}</div>
  <div id="overview-attention-panel" role="tabpanel" aria-labelledby={'overview-tab-'+selected} tabIndex={0} aria-busy={busy}>
   {error&&<p className="po-queue-error" role="alert">{t(denied?'access_changed':'queue_error')}</p>}{queueChanged&&<p className="po-queue-change" role="status">{t('queue_changed')}</p>}
   {!page?<p className="po-empty-copy">{t(denied?'refresh_required':'queue_unavailable')}</p>:<>{page.items.length?<div className="po-task-list">{page.items.map(item=><button type="button" className="po-task-row" key={item.id} onClick={()=>openRecord(item.id)}><span className={'po-task-marker '+selected} aria-hidden="true">{selected==='review'?<ClipboardCheck size={16}/>:selected==='blocked'?<Pause size={16}/>:<Clock3 size={16}/>}</span><span className="po-task-copy"><strong>{item.title||item.id}</strong><small><span>{item.owner_name||item.owner_id||t('unassigned')}</span>{item.due_date&&<span>{t('due_date',{date:dateText(item.due_date)})}</span>}</small></span><Badge state={item.status}/><ChevronRight size={15} className="po-task-arrow" aria-hidden="true"/></button>)}</div>:<div className="po-queue-empty"><Minus size={22} aria-hidden="true"/><strong>{t(page.total===0?'empty.'+selected:'queue_no_rows')}</strong><p>{t('empty_hint')}</p></div>}
   <div className="po-queue-foot"><span>{t('queue_count',{loaded:formatNumber(page.items.length),total:formatNumber(page.total)})}{selected==='overdue'&&<small>{t('utc_basis')}</small>}</span>{page.next_cursor&&<button type="button" disabled={busy} onClick={()=>void loadMore()}>{t(busy?'loading':'load_more')}</button>}</div>
   </>}
  </div>
 </section>;
}

function ActivityRow({item}:{item:OverviewActivity}){
 const presentation=activityPresentation(item),Icon={approved:Check,created:Plus,updated:SquarePen,review:Shield,conversation:MessageSquare,execution:Play,warning:AlertCircle,neutral:Activity}[presentation];
 return <div className="po-activity-row"><span className={'po-event-icon '+presentation} data-overview-event={presentation} aria-label={t('event.'+presentation)}><Icon size={14} aria-hidden="true"/></span><div><p>{item.kind==='task_conversation.posted'?tr('task-conversation.activity'):item.title||item.message||item.action||item.type||t('event.neutral')}</p><small>{item.actor_name||item.actor||tr('workbench.overview.project_event')} · <time dateTime={item.at||item.created_at}>{time(item.at||item.created_at)}</time></small></div></div>;
}
