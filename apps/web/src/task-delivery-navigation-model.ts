import type {MouseEvent} from 'react';
import type {WorkbenchRoute} from './navigation';

/** Public navigation locators only. A matching locator never grants review or
 * asserts that a candidate/test is eligible for human acceptance. */
export type TaskAcceptanceLocator={taskVersion:number;runID:string};
export type TaskAcceptanceConsumption={userID:string;organizationID:string;project:string;taskID:string;panel:string};
const identifier=(value:string)=>typeof value==='string'&&!/[\u0000-\u001f\u007f]/.test(value)&&/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(value);
const version=(value:number)=>Number.isSafeInteger(value)&&value>0&&value<=2147483646;
export const validWorkBuddyProgressTaskID=identifier;

export function workBuddyProgressTaskHref(project:string,task:string,filter?:'attention'):string|undefined{
  if(!identifier(project)||!identifier(task))return undefined;
  const fields=new URLSearchParams({project,view:'development',sub:'runs',layout:'workbuddy',workbuddy_task:task});
  if(filter)fields.set('workbuddy_filter',filter);
  return '?'+fields;
}

export function isWorkBuddyProgressTaskLocator(params:URLSearchParams):boolean{
  return ['project','view','sub','layout','workbuddy_task'].every(key=>params.getAll(key).length===1)
    &&params.get('view')==='development'&&params.get('sub')==='runs'&&params.get('layout')==='workbuddy'
    &&identifier(params.get('project')||'')&&identifier(params.get('workbuddy_task')||'')
    &&(!params.has('workbuddy_filter')||params.getAll('workbuddy_filter').length===1&&params.get('workbuddy_filter')==='attention')
    &&!['record','panel','run','node','attempt','workbuddy_task_invalid','workbuddy_filter_invalid'].some(key=>params.has(key));
}

export const taskDeliveryPanel='task-delivery';
const taskDeliveryOriginPrefix=taskDeliveryPanel+':';
export const taskDeliveryNavigateEvent='suma:open-task-delivery';
export function parseTaskDeliveryPanel(panel:string):{runID?:string}|undefined{
  if(panel===taskDeliveryPanel)return {};
  if(!panel.startsWith(taskDeliveryOriginPrefix))return undefined;
  const runID=panel.slice(taskDeliveryOriginPrefix.length);
  return identifier(runID)?{runID}:undefined;
}
export function isTaskDeliveryLocator(params:URLSearchParams):boolean{
  return ['project','view','sub','record','panel'].every(key=>params.getAll(key).length===1)
    &&params.get('view')==='development'&&params.get('sub')==='tasks'&&!!parseTaskDeliveryPanel(params.get('panel')||'')
    &&identifier(params.get('project')||'')&&identifier(params.get('record')||'')
    &&(!params.has('layout')||params.getAll('layout').length===1&&params.get('layout')==='list')
    &&!['run','node','attempt'].some(key=>params.has(key));
}
export function taskAcceptancePanel(taskVersion:number,runID:string):string|undefined{
  return version(taskVersion)&&identifier(runID)?'task-acceptance:'+taskVersion+':'+runID:undefined;
}
export function parseTaskAcceptancePanel(panel:string):TaskAcceptanceLocator|undefined{
  if(typeof panel!=='string'||panel.length>100)return undefined;
  const parts=panel.split(':');
  if(parts.length!==3||parts[0]!=='task-acceptance'||!/^\d+$/.test(parts[1]))return undefined;
  const taskVersion=Number(parts[1]);
  return String(taskVersion)===parts[1]&&version(taskVersion)&&identifier(parts[2])?{taskVersion,runID:parts[2]}:undefined;
}
export function taskDeliveryHref(project:string,target:{kind:'issue'|'requirement'|'team'|'review'|'repository'|'delivery';id:string;taskVersion?:number;runID?:string}):string|undefined{
  if(!identifier(project)||!identifier(target.id))return undefined;
  const fields:Record<string,string>={project};
  switch(target.kind){
    case 'issue':Object.assign(fields,{view:'quality',sub:'issues',record:target.id,layout:'list'});break;
    case 'requirement':Object.assign(fields,{view:'requirements',sub:'',record:target.id,layout:'list'});break;
    case 'team':Object.assign(fields,{view:'development',sub:'runs',layout:'team',panel:'expert-team:'+target.id});break;
    case 'repository':Object.assign(fields,{view:'development',sub:'tasks',record:target.id,layout:'list',panel:'repository-integration'});break;
    case 'delivery':{
      if(target.runID!==undefined&&!identifier(target.runID))return undefined;
      const panel=target.runID===undefined?taskDeliveryPanel:taskDeliveryOriginPrefix+target.runID;
      Object.assign(fields,{view:'development',sub:'tasks',record:target.id,layout:'list',panel});break;
    }
    case 'review':{
      const panel=taskAcceptancePanel(target.taskVersion||0,target.runID||'');if(!panel)return undefined;
      Object.assign(fields,{view:'development',sub:'tasks',record:target.id,layout:'list',panel});break;
    }
    default:return undefined;
  }
  return '?'+new URLSearchParams(fields);
}
/** Use ordinary browser navigation when the workbench is not mounted. */
export function followTaskDeliveryHref(event:MouseEvent<HTMLAnchorElement>,href:string){
  if(event.defaultPrevented||event.button!==0||event.ctrlKey||event.metaKey||event.shiftKey||event.altKey)return;
  const claimed=!window.dispatchEvent(new CustomEvent(taskDeliveryNavigateEvent,{detail:href,cancelable:true}));
  if(claimed||!window.dispatchEvent(new Event('suma:navigate',{cancelable:true})))event.preventDefault();
}
/** Consume only the originating route. A queued callback from an older
 * account, task or panel must never clear a more recent user's navigation. */
export function consumeTaskAcceptancePanel(route:WorkbenchRoute,owner:{userID?:string;organizationID?:string},expected:TaskAcceptanceConsumption):WorkbenchRoute{
  if(owner.userID!==expected.userID||owner.organizationID!==expected.organizationID||!parseTaskAcceptancePanel(expected.panel)||route.project!==expected.project||route.view!=='development'||route.sub!=='tasks'||route.record!==expected.taskID||route.panel!==expected.panel)return route;
  return {...route,panel:''};
}
