import {mondayOf} from './planning-model';
import {parseAttempt} from './scheduling-model';
export type WorkbenchView='overview'|'requirements'|'development'|'quality'|'operations'|'knowledge'|'settings';
export type WorkbenchRoute={task_columns?:string[];conversation_message?:string;inbox_filter?:'inbox'|'unread'|'archived';inbox_invalid?:string;task_invalid?:string;task_status?:string;task_owner?:string;task_priority?:string;task_sort?:string;task_group?:string;task_requirement?:string;task_iteration?:string;task_iteration_unset?:boolean;task_unassigned?:boolean;task_priority_unset?:boolean;task_status_unset?:boolean;project:string;view:WorkbenchView;sub:string;record:string;run:string;node:string;attempt:string;q:string;status:string;layout:string;week:string;panel:string;knowledge_kind?:'memory'|'spec'|'skill'|'all';catalog_domain?:string;catalog_scenario?:string;catalog_state?:'all'|'published'|'retired';catalog_source?:string};
const views:WorkbenchView[]=['overview','requirements','development','quality','operations','knowledge','settings'];
const defaultSub:Partial<Record<WorkbenchView,string>>={development:'my-work',quality:'issues',operations:'releases',knowledge:'documents',settings:'organization'};
/** Formal task sources retain a locator only in one unambiguous task scope.
 * Parsing never opens, marks read or decides the source. */
function taskDetailLocatorPanel(params:URLSearchParams,taskScope:boolean):string{
 const panel=params.get('panel')||'',review=panel.startsWith('task-review:'),handoff=panel.startsWith('task-handoff:');
 if(!review&&!handoff)return panel;
 if(!taskScope||params.getAll('panel').length!==1||params.getAll('record').length!==1||!/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(params.get('record')||''))return '';
 return (review?taskReviewRequestID(panel):taskHandoffID(panel))?panel:'';
}
export function parseWorkbenchRoute(search:string):WorkbenchRoute{
 const p=new URLSearchParams(search),view=views.includes(p.get('view') as WorkbenchView)?p.get('view') as WorkbenchView:'overview';
 const knowledgeKind=p.get('knowledge_kind');
 const conversationMessage=p.get('conversation_message');
 const messageFields=view==='development'&&p.get('sub')==='ide'&&workbenchTaskID(p.get('panel')||'')&&p.getAll('conversation_message').length===1&&/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(conversationMessage||'')?{conversation_message:conversationMessage!}:{};
 const inboxScope=view==='development'&&(p.get('sub')||defaultSub[view])==='inbox';
 const inboxValue=p.get('inbox_filter'),inboxValid=inboxValue==='inbox'||inboxValue==='unread'||inboxValue==='archived';
 const inboxInvalid=inboxScope&&(p.get('inbox_invalid')==='1'||p.getAll('inbox_filter').length>1||p.getAll('inbox_invalid').length>1||p.has('inbox_filter')&&!inboxValid);
 const inboxFields=inboxScope?{inbox_filter:inboxValid?inboxValue as 'inbox'|'unread'|'archived':undefined,...(inboxInvalid?{inbox_invalid:'1'}:{})}:{};

 const taskScope=view==='development'&&(p.get('sub')||defaultSub[view])==='tasks';
 // Task filters are one literal contract, independent of layout or grouping.
 const bounded=(value:string|null)=>value!==null&&Array.from(value).length<=200&&!/[\u0000-\u001f\u007f-\u009f]/.test(value)?value:undefined;
 const boundedID=(value:string|null)=>value!==null&&(value===''||/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(value))?value:undefined;
 const flags=['task_unassigned','task_priority_unset','task_status_unset','task_iteration_unset'] as const;
 const invalidTask=taskScope&&(p.get('task_invalid')==='1'||bounded(p.get('q')||'')===undefined||
  ['q','task_invalid','task_status','task_owner','task_priority','task_sort','task_group','task_requirement','task_iteration',...flags].some(key=>p.getAll(key).length>1)||
  ['task_status','task_priority'].some(key=>p.has(key)&&(bounded(p.get(key))===undefined||!!p.get(key)&&!p.get(key)!.trim()))||
  ['task_owner','task_requirement','task_iteration'].some(key=>p.has(key)&&boundedID(p.get(key))===undefined)||
  flags.some(key=>p.has(key)&&!['true','false'].includes(p.get(key)||''))||
  p.get('task_unassigned')==='true'&&!!p.get('task_owner')||
  p.get('task_status_unset')==='true'&&!!p.get('task_status')||
  p.get('task_priority_unset')==='true'&&!!p.get('task_priority')||
  p.get('task_iteration_unset')==='true'&&!!p.get('task_iteration')||
  p.has('task_sort')&&!['updated_desc','created_desc','title_asc','priority_desc'].includes(p.get('task_sort')||'')||
  p.has('task_group')&&!['status','owner','priority','iteration'].includes(p.get('task_group')||''));
 const taskFields=taskScope?{
  ...(p.has('task_columns')?{task_columns:p.getAll('task_columns').flatMap(value=>value.split(','))}:{}),
  ...(invalidTask?{task_invalid:'1'}:{}),
  task_status:bounded(p.get('task_status')),
  task_owner:boundedID(p.get('task_owner')),
  task_priority:bounded(p.get('task_priority')),
  task_requirement:boundedID(p.get('task_requirement')),
  task_iteration:boundedID(p.get('task_iteration')),
  ...Object.fromEntries(flags.filter(key=>p.get(key)==='true'||p.get(key)==='false').map(key=>[key,p.get(key)==='true'])),
  task_sort:['updated_desc','created_desc','title_asc','priority_desc'].includes(p.get('task_sort')||'')?p.get('task_sort')!:undefined,
  task_group:['status','owner','priority','iteration'].includes(p.get('task_group')||'')?p.get('task_group')!:undefined,
 }:{};

 return {...messageFields,...inboxFields,...taskFields,...(p.has('catalog_domain')?{catalog_domain:p.get('catalog_domain')||''}:{}),...(p.has('catalog_scenario')?{catalog_scenario:p.get('catalog_scenario')||''}:{}),...(['all','published','retired'].includes(p.get('catalog_state')||'')?{catalog_state:p.get('catalog_state') as WorkbenchRoute['catalog_state']}:{}),...(p.has('catalog_source')?{catalog_source:p.get('catalog_source')||''}:{}),...(['memory','spec','skill','all'].includes(knowledgeKind||'')?{knowledge_kind:knowledgeKind as WorkbenchRoute['knowledge_kind']}:{}),project:p.get('project')||'',view,sub:p.get('sub')||defaultSub[view]||'',record:p.get('record')||'',run:p.get('run')||'',node:p.get('node')||'',attempt:parseAttempt(p.get('attempt')||'')?p.get('attempt')!:'',q:p.get('q')||'',status:p.get('status')||'all',layout:p.get('layout')||'list',week:p.get('week')||mondayOf(),panel:taskDetailLocatorPanel(p,taskScope)};
}
export function initializeSessionRoute(route:WorkbenchRoute,projects:{id:string}[]):WorkbenchRoute{return route.project||!projects.length?route:{...route,project:projects[0].id};}

/** Resource and requirement filters belong to one project. A project switch
 * clears them together; ordinary task/detail navigation can retain the view. */
export function resetTaskCollectionScope(route:WorkbenchRoute):WorkbenchRoute{return {...route,conversation_message:undefined,task_columns:undefined,task_invalid:undefined,task_status:undefined,task_owner:undefined,task_priority:undefined,task_sort:undefined,task_group:undefined,task_requirement:undefined,task_iteration:undefined,task_iteration_unset:undefined,task_unassigned:undefined,task_priority_unset:undefined,task_status_unset:undefined};}

/** The selected task is reading context, independent of the editor source.
 * Keep the run locator empty so an IDE refresh cannot switch into Run view. */
export function workbenchTaskID(panel:string):string{const id=panel.startsWith('workbench-task:')?panel.slice('workbench-task:'.length):'';return /^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(id)?id:'';}
export function locateWorkbenchTask(id:string,canLeave:()=>boolean,update:(change:(route:WorkbenchRoute)=>WorkbenchRoute)=>void,message?:string):boolean{
 if(id&&!/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(id)||message&&(!id||!/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(message))||!canLeave())return false;
 update(route=>({...route,view:'development',sub:'ide',panel:id?'workbench-task:'+id:'',conversation_message:message||undefined,record:'',run:'',node:'',attempt:'',q:'',layout:'list'}));
 return true;
}

/** A review responsibility link opens the original task detail, never a new
 * approval surface or an automatic acceptance action. */
export function taskReviewRequestID(panel:string):string{const id=panel.startsWith('task-review:')?panel.slice('task-review:'.length):'';return /^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(id)?id:'';}
export function taskHandoffID(panel:string):string{const id=panel.startsWith('task-handoff:')?panel.slice('task-handoff:'.length):'';return /^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(id)?id:'';}
export function locateTaskHandoff(task:string,id:string,canLeave:()=>boolean,update:(change:(route:WorkbenchRoute)=>WorkbenchRoute)=>void):boolean{
 if(!/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(task)||!taskHandoffID('task-handoff:'+id)||!canLeave())return false;
 update(route=>({...resetTaskCollectionScope(route),view:'development',sub:'tasks',record:task,panel:'task-handoff:'+id,run:'',node:'',attempt:'',q:'',status:'all',layout:'list',conversation_message:undefined,inbox_filter:undefined,inbox_invalid:undefined}));return true;
}
export function locateTaskReviewRequest(task:string,id:string,canLeave:()=>boolean,update:(change:(route:WorkbenchRoute)=>WorkbenchRoute)=>void):boolean{
 if(!/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(task)||!taskReviewRequestID('task-review:'+id)||!canLeave())return false;
 update(route=>({...resetTaskCollectionScope(route),view:'development',sub:'tasks',record:task,panel:'task-review:'+id,run:'',node:'',attempt:'',q:'',status:'all',layout:'list',inbox_filter:undefined,inbox_invalid:undefined}));return true;
}

/** A risk locator is one navigation operation. In particular, a rejected
 * leave guard cannot still write a risk ID into a baseline's record field. */
export function locateProjectRisk(id:string,canLeave:()=>boolean,update:(change:(route:WorkbenchRoute)=>WorkbenchRoute)=>void):boolean{
 if(!canLeave())return false;
 update(route=>({...route,view:'development',sub:'planning',layout:'risks',panel:id?'project-risk:'+id:'',record:'',run:'',node:'',attempt:'',q:'',status:'all',knowledge_kind:undefined,catalog_domain:undefined,catalog_scenario:undefined,catalog_state:undefined,catalog_source:undefined}));
 return true;
}

/** Application and pinned-version locators change together. A rejected leave
 * guard must not replace a task, version or another page's detail selection. */
export function locateProjectApplication(id:string,version:string,canLeave:()=>boolean,update:(change:(route:WorkbenchRoute)=>WorkbenchRoute)=>void):boolean{
 if(!canLeave())return false;
 update(route=>({...route,view:'overview',sub:'',layout:'applications',panel:id?'application:'+id:'',record:version,run:'',node:'',attempt:'',q:'',status:'all',knowledge_kind:undefined,catalog_domain:undefined,catalog_scenario:undefined,catalog_state:undefined,catalog_source:undefined}));
 return true;
}

export function locateApplicationReleaseSet(id:string,canLeave:()=>boolean,update:(change:(route:WorkbenchRoute)=>WorkbenchRoute)=>void):boolean{
 if(!canLeave())return false;
 update(route=>({...route,view:'operations',sub:'release-candidates',layout:'release-sets',record:id,panel:id?'application-release-set:'+id:'',run:'',node:'',attempt:'',q:'',status:'all',knowledge_kind:undefined,catalog_domain:undefined,catalog_scenario:undefined,catalog_state:undefined,catalog_source:undefined}));
 return true;
}

export function locateReleaseSetRollout(setID:string,groupID:string,rolloutID:string,canLeave:()=>boolean,update:(change:(route:WorkbenchRoute)=>WorkbenchRoute)=>void):boolean{
 if(!canLeave())return false;
 update(route=>({...route,view:'operations',sub:'release-candidates',layout:'release-sets',record:setID,panel:rolloutID?'release-set-rollout:'+groupID+':'+rolloutID:'application-release-set:'+setID,run:'',node:'',attempt:'',q:'',status:'all',knowledge_kind:undefined,catalog_domain:undefined,catalog_scenario:undefined,catalog_state:undefined,catalog_source:undefined}));
 return true;
}

/** Provider locators are data, never URLs. Only known in-project record kinds
 * become workbench destinations; foreign/invalid locators have no link. */
export function sourceDestination(project:string,value:unknown):Pick<WorkbenchRoute,'project'|'view'|'sub'|'record'>|undefined {
 const object=(item:unknown):Record<string,unknown>=>item!==null&&typeof item==='object'&&!Array.isArray(item)?item as Record<string,unknown>:{};
 const outer=object(value),locator=object(outer.locator),source=Object.keys(locator).length?locator:outer;
 for(const item of [outer,source])if(item.project_id!==undefined&&item.project_id!==project)return;
 const type=typeof source.type==='string'?source.type:source.kind;
 const destinations:Record<string,{view:WorkbenchView;sub:string}>={requirements:{view:'requirements',sub:''},requirement:{view:'requirements',sub:''},tasks:{view:'development',sub:'tasks'},task:{view:'development',sub:'tasks'},issues:{view:'quality',sub:'issues'},issue:{view:'quality',sub:'issues'},documents:{view:'knowledge',sub:'documents'},document:{view:'knowledge',sub:'documents'},'project-knowledge':{view:'knowledge',sub:'project-knowledge'}};
 const destination=typeof type==='string'?destinations[type]:undefined;
 const id=source.id??outer.source_ref??outer.id;
 if(!project||!destination||typeof id!=='string'||!/^[A-Za-z0-9_-]{1,150}$/.test(id))return;
 return {project,...destination,record:id};
}
export function sourceHref(project:string,source:unknown):string|undefined {
 const route=sourceDestination(project,source);
 return route?'?'+new URLSearchParams(route):undefined;
}

/** Notification filters belong to one current human/project reading scope. */
export function resetInboxScope(route:WorkbenchRoute):WorkbenchRoute{return {...route,inbox_filter:undefined,inbox_invalid:undefined};}
