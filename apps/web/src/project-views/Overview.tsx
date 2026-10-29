import {Activity,AlertCircle,ArrowDownRight,ArrowRight,Check,ChevronRight,ClipboardList,GitBranch,Layers,Plus,ShieldCheck,Users} from 'lucide-react';
import {api,label,list,query,time,type RecordItem} from '../api';
import {Badge,Empty,ErrorNotice,Loading} from '../ui';
import {formatNumber,tr,useLocale} from '../i18n';
import {useProjectRead} from './useProjectRead';
import {ProjectRiskSummaryCard} from '../ProjectRisks';
import '../ProjectApplications.css';

type Destination='overview'|'requirements'|'development'|'quality'|'operations'|'knowledge'|'settings';
type OverviewData={project?:RecordItem;counts?:Record<string,number|{total?:number}>;activity?:{id?:string;title?:string;message?:string;action?:string;type?:string;actor_name?:string;actor?:string;at?:string;created_at?:string;kind?:string}[]};
async function readOverview(project:string){const [data,tasks]=await Promise.all([api<OverviewData>('/overview'+query(project)),list('/tasks',project)]);return {data,tasks:tasks.items};}
const cards=[{key:'requirements',icon:Layers,to:'requirements'},{key:'tasks',icon:ClipboardList,to:'development'},{key:'runs',icon:Activity,to:'development',sub:'runs'},{key:'issues',icon:AlertCircle,to:'quality'}] as const;
const journey=[{key:'requirements',icon:Layers,view:'requirements'},{key:'collaboration',icon:Users,view:'settings',sub:'members'},{key:'workflow',icon:GitBranch,view:'development',sub:'workflows'},{key:'acceptance',icon:ShieldCheck,view:'quality'}] as const;
function countText(value:number|{total?:number}|undefined){const count=typeof value==='number'?value:value?.total;return typeof count==='number'&&Number.isSafeInteger(count)&&count>=0?formatNumber(count):'—';}

export function Overview({project,refresh,navigate,openRecord,onOpenRisks,onOpenApplications}:{project:string;refresh:number;navigate:(view:Destination,sub?:string)=>void;openRecord:(id:string)=>void;onOpenRisks:()=>void;onOpenApplications:()=>void}){
 useLocale();const snapshot=useProjectRead(project,refresh,readOverview);
 if(snapshot.error)return <ErrorNotice error={snapshot.error}/>;
 if(!snapshot.data)return <Loading/>;
 const {data,tasks}=snapshot.data,active=tasks.filter(task=>!['accepted','done','cancelled'].includes(task.status||''));
 return <>
  <div className="overview-banner"><div className="banner-symbol"><GitBranch size={25}/></div><div><span className="eyebrow">{tr('workbench.overview.eyebrow')}</span><h2>{tr('workbench.overview.title')}</h2><p>{data.project?.description||tr('workbench.overview.description')}</p></div><button onClick={()=>navigate('requirements')}><Plus size={16}/>{tr('workbench.overview.plan')}</button></div>
  <div className="metric-grid">{cards.map(card=><button className="metric-card" key={card.key} onClick={()=>navigate(card.to,'sub'in card?card.sub:undefined)}><div><span>{tr('workbench.overview.count.'+card.key)}</span><card.icon size={17}/></div><strong>{countText(data.counts?.[card.key])}</strong><span className="metric-link">{tr('workbench.overview.view_all')}<ArrowDownRight size={14}/></span></button>)}</div>
  <ProjectRiskSummaryCard project={project} refresh={refresh} onOpen={onOpenRisks}/>
  <section className="pa-overview-entry"><div><strong>{tr('project-applications.title')}</strong><p>{tr('project-applications.overview_hint')}</p></div><button type="button" onClick={onOpenApplications}>{tr('project-applications.open_catalog')}<ArrowRight size={14}/></button></section>
  <div className="overview-columns">
   <section className="panel"><div className="panel-heading"><div><h2>{tr('workbench.overview.active')}</h2><p>{tr('workbench.overview.active_scope')}</p></div><button className="text-button" onClick={()=>navigate('development','tasks')}>{tr('workbench.overview.all_tasks')}<ArrowRight size={14}/></button></div>
    {active.length?<div className="compact-task-list">{active.slice(0,6).map(task=><button key={task.id} onClick={()=>openRecord(task.id)}><span className={'task-state-dot '+task.status}/><div><strong>{label(task)}</strong><small>{task.id.slice(0,12)} · {['urgent','high','medium','low'].includes(task.priority||'medium')?tr('common.priority.'+(task.priority||'medium')):task.priority}</small></div><Badge state={task.status}/><ChevronRight size={15}/></button>)}</div>:<Empty title={tr(tasks.length?'workbench.overview.no_active':'workbench.overview.no_tasks')} description={tr(tasks.length?'workbench.overview.active_empty_scope':'workbench.overview.no_tasks_hint')} action={<button onClick={()=>navigate(tasks.length?'development':'requirements',tasks.length?'tasks':undefined)}>{tr(tasks.length?'workbench.overview.all_tasks':'workbench.overview.go_requirements')}<ArrowRight size={14}/></button>}/>}</section>
   <section className="panel activity-panel"><div className="panel-heading"><div><h2>{tr('workbench.overview.activity')}</h2><p>{tr('workbench.overview.activity_scope')}</p></div><Activity size={17}/></div>{data.activity?.length?<div className="activity-list">{data.activity.slice(0,8).map((item,index)=><div className="activity-item" key={item.id||index}><span className="activity-icon"><Check size={14}/></span><div><p>{item.kind==='task_conversation.posted'?tr('task-conversation.activity'):item.title||item.message||item.action||item.type}</p><small>{item.actor_name||item.actor||tr('workbench.overview.project_event')} · {time(item.at||item.created_at)}</small></div></div>)}</div>:<Empty title={tr('workbench.overview.no_activity')} description={tr('workbench.overview.no_activity_hint')}/>}</section>
  </div>
  <section className="quick-path"><span className="eyebrow">{tr('workbench.overview.journey')}</span><div>{journey.map((step,index)=><button onClick={()=>navigate(step.view,'sub'in step?step.sub:undefined)} key={step.key}><span className="path-number">{formatNumber(index+1,{minimumIntegerDigits:2})}</span><step.icon size={21}/><span><strong>{tr('workbench.overview.step.'+step.key)}</strong><small>{tr('workbench.overview.step_hint.'+step.key)}</small></span><ChevronRight size={16}/></button>)}</div></section>
 </>;
}
