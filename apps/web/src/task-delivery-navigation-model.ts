import type {WorkbenchRoute} from './navigation';

/** Public navigation locators only. A matching locator never grants review or
 * asserts that a candidate/test is eligible for human acceptance. */
export type TaskAcceptanceLocator={taskVersion:number;runID:string};
export type TaskAcceptanceConsumption={userID:string;organizationID:string;project:string;taskID:string;panel:string};
const identifier=(value:string)=>typeof value==='string'&&!/[\u0000-\u001f\u007f]/.test(value)&&/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(value);
const version=(value:number)=>Number.isSafeInteger(value)&&value>0&&value<=2147483646;
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
export function taskDeliveryHref(project:string,target:{kind:'issue'|'requirement'|'team'|'review';id:string;taskVersion?:number;runID?:string}):string|undefined{
  if(!identifier(project)||!identifier(target.id))return undefined;
  const fields:Record<string,string>={project};
  switch(target.kind){
    case 'issue':Object.assign(fields,{view:'quality',sub:'issues',record:target.id,layout:'list'});break;
    case 'requirement':Object.assign(fields,{view:'requirements',sub:'',record:target.id,layout:'list'});break;
    case 'team':Object.assign(fields,{view:'development',sub:'runs',layout:'team',panel:'expert-team:'+target.id});break;
    case 'review':{
      const panel=taskAcceptancePanel(target.taskVersion||0,target.runID||'');if(!panel)return undefined;
      Object.assign(fields,{view:'development',sub:'tasks',record:target.id,layout:'list',panel});break;
    }
    default:return undefined;
  }
  return '?'+new URLSearchParams(fields);
}
/** Consume only the originating route. A queued callback from an older
 * account, task or panel must never clear a more recent user's navigation. */
export function consumeTaskAcceptancePanel(route:WorkbenchRoute,owner:{userID?:string;organizationID?:string},expected:TaskAcceptanceConsumption):WorkbenchRoute{
  if(owner.userID!==expected.userID||owner.organizationID!==expected.organizationID||!parseTaskAcceptancePanel(expected.panel)||route.project!==expected.project||route.view!=='development'||route.sub!=='tasks'||route.record!==expected.taskID||route.panel!==expected.panel)return route;
  return {...route,panel:''};
}
