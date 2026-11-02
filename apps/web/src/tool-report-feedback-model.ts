import {readAdapterCatalog,type AdapterDescriptor} from './AdapterConnections';
import {parseToolDiscovery,toolDiscoveryTimestampNs,type ToolDiscoveryView} from './tool-discovery-model';
import type {ToolObservation} from './tool-model';
import {preparationForLocalTool,toolPreparationTarget,type ToolPreparationTarget} from './tool-preparation-target';
import {matchingQuickTools} from './tool-quick-connect-model';

export type ToolReportFeedbackTarget={project:string;workerID:string;localTool:ToolPreparationTarget['localTool']};
export type ToolReportFeedbackSelection={project:string;workerID:string;adapterID:string;commandID:string;profileSHA256:string;observedAt:string};
/** Local checked observations; neither a persisted operation nor execution authority. */
export type ToolReportFeedbackView={target:ToolReportFeedbackTarget;adapter:AdapterDescriptor;discovery:ToolDiscoveryView;choices:ToolObservation[];remainingMs:number};
export type ToolReportFeedbackBaseline={target:ToolReportFeedbackTarget;asOf:string;observedAt?:string};

const identity=/^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$/;
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value)&&[Object.prototype,null].includes(Object.getPrototypeOf(value));
const exact=(value:Record<string,unknown>,keys:string[])=>Reflect.ownKeys(value).length===keys.length&&keys.every(key=>Object.hasOwn(value,key));
const elapsed=(value:number)=>Number.isFinite(value)&&value>=0;
const sameTarget=(left:ToolReportFeedbackTarget,right:ToolReportFeedbackTarget)=>left.project===right.project&&left.workerID===right.workerID&&left.localTool===right.localTool;
const copyTarget=(target:ToolReportFeedbackTarget):ToolReportFeedbackTarget=>Object.freeze({project:target.project,workerID:target.workerID,localTool:target.localTool});

/** Shape and current project only. The original API still authorizes the actor and worker. */
export function validToolReportFeedbackTarget(value:unknown,currentProject:string):value is ToolReportFeedbackTarget{
 return object(value)&&exact(value,['project','workerID','localTool'])&&typeof value.project==='string'&&identity.test(value.project)&&value.project===currentProject&&typeof value.workerID==='string'&&identity.test(value.workerID)&&!!preparationForLocalTool(value.localTool);
}

/** Reuse the original catalog, bounded observation and complete capability readers.
 * Request latency is charged immediately; browser wall time never renews a report. */
export function readToolReportFeedback(catalogRaw:unknown,discoveryRaw:unknown,target:ToolReportFeedbackTarget,currentProject:string,requestElapsedMs:number):ToolReportFeedbackView|undefined{
 if(!validToolReportFeedbackTarget(target,currentProject)||!elapsed(requestElapsedMs))return;
 const catalog=readAdapterCatalog(catalogRaw,target.project),adapter=catalog?.items.find(item=>item.id===target.localTool);
 if(!adapter||toolPreparationTarget(adapter)?.localTool!==target.localTool)return;
 let discovery:ToolDiscoveryView;
 try{discovery=parseToolDiscovery(discoveryRaw,target.project,target.workerID);}catch{return;}
 const choices=discovery.state==='fresh'?matchingQuickTools(discovery.items,adapter):[];
 return {target:copyTarget(target),adapter,discovery,choices,remainingMs:Math.max(0,discovery.remainingMs-requestElapsedMs)};
}

/** Freeze the server read boundary, not a copied command or browser clock. */
export function toolReportFeedbackBaseline(view:ToolReportFeedbackView):ToolReportFeedbackBaseline{
 return Object.freeze({target:copyTarget(view.target),asOf:view.discovery.asOf,...(view.discovery.observedAt?{observedAt:view.discovery.observedAt}:{})});
}

export function toolReportFeedbackRemainingMs(view:ToolReportFeedbackView,elapsedSinceReadMs:number):number{
 return view.discovery.state==='fresh'&&elapsed(elapsedSinceReadMs)&&elapsed(view.remainingMs)?Math.max(0,view.remainingMs-elapsedSinceReadMs):0;
}

/** A new server-received inventory is not evidence that a copied command ran.
 * Advance past both boundaries: a clock rollback may put the original observed
 * timestamp beyond baseline.asOf, and an older replacement is still not new. */
export function isNewToolReportFeedback(view:ToolReportFeedbackView,baseline:ToolReportFeedbackBaseline,elapsedSinceReadMs=0):boolean{
 if(!sameTarget(view.target,baseline.target)||toolReportFeedbackRemainingMs(view,elapsedSinceReadMs)<=0||!view.discovery.observedAt)return false;
 try{
  const observed=toolDiscoveryTimestampNs(view.discovery.observedAt);
  return observed>toolDiscoveryTimestampNs(baseline.asOf)&&(baseline.observedAt===undefined||observed>toolDiscoveryTimestampNs(baseline.observedAt));
 }catch{return false;}
}

/** The human chooses one live available observation. Quick must re-read current
 * authorization and configuration; this selection never registers or enables it. */
export function selectToolReportFeedback(view:ToolReportFeedbackView,commandID:string,elapsedSinceReadMs:number):ToolReportFeedbackSelection|undefined{
 if(toolReportFeedbackRemainingMs(view,elapsedSinceReadMs)<=0||!view.discovery.observedAt)return;
 const choice=view.choices.find(item=>item.profile.command_id===commandID&&item.available===true);
 if(!choice)return;
 return {project:view.target.project,workerID:view.target.workerID,adapterID:view.adapter.id,commandID:choice.profile.command_id,profileSHA256:choice.profile.profile_sha256!,observedAt:view.discovery.observedAt};
}

/** Closed shape/scope validation only, not a current authorization or freshness
 * check. Workspace still re-reads the catalog, worker and original Quick facts. */
export function validToolReportFeedbackSelection(value:unknown,target:ToolReportFeedbackTarget):value is ToolReportFeedbackSelection{
 if(!validToolReportFeedbackTarget(target,target.project)||!object(value)||!exact(value,['project','workerID','adapterID','commandID','profileSHA256','observedAt'])||value.project!==target.project||value.workerID!==target.workerID||value.adapterID!==target.localTool||typeof value.commandID!=='string'||!identity.test(value.commandID)||typeof value.profileSHA256!=='string'||!/^[a-f0-9]{64}$/.test(value.profileSHA256))return false;
 try{toolDiscoveryTimestampNs(value.observedAt);return true;}catch{return false;}
}
