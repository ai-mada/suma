import {mondayOf} from './planning-model';
import {parseAttempt} from './scheduling-model';
export type WorkbenchView='overview'|'requirements'|'development'|'quality'|'operations'|'knowledge'|'settings';
export type WorkbenchRoute={project:string;view:WorkbenchView;sub:string;record:string;run:string;node:string;attempt:string;q:string;status:string;layout:string;week:string;panel:string;knowledge_kind?:'memory'|'spec'|'skill'|'all';catalog_domain?:string;catalog_scenario?:string;catalog_state?:'all'|'published'|'retired';catalog_source?:string};
const views:WorkbenchView[]=['overview','requirements','development','quality','operations','knowledge','settings'];
const defaultSub:Partial<Record<WorkbenchView,string>>={development:'my-work',quality:'issues',operations:'releases',knowledge:'documents',settings:'organization'};
export function parseWorkbenchRoute(search:string):WorkbenchRoute{
 const p=new URLSearchParams(search),view=views.includes(p.get('view') as WorkbenchView)?p.get('view') as WorkbenchView:'overview';
 const knowledgeKind=p.get('knowledge_kind');
 return {...(p.has('catalog_domain')?{catalog_domain:p.get('catalog_domain')||''}:{}),...(p.has('catalog_scenario')?{catalog_scenario:p.get('catalog_scenario')||''}:{}),...(['all','published','retired'].includes(p.get('catalog_state')||'')?{catalog_state:p.get('catalog_state') as WorkbenchRoute['catalog_state']}:{}),...(p.has('catalog_source')?{catalog_source:p.get('catalog_source')||''}:{}),...(['memory','spec','skill','all'].includes(knowledgeKind||'')?{knowledge_kind:knowledgeKind as WorkbenchRoute['knowledge_kind']}:{}),project:p.get('project')||'',view,sub:p.get('sub')||defaultSub[view]||'',record:p.get('record')||'',run:p.get('run')||'',node:p.get('node')||'',attempt:parseAttempt(p.get('attempt')||'')?p.get('attempt')!:'',q:p.get('q')||'',status:p.get('status')||'all',layout:p.get('layout')||'list',week:p.get('week')||mondayOf(),panel:p.get('panel')||''};
}
export function initializeSessionRoute(route:WorkbenchRoute,projects:{id:string}[]):WorkbenchRoute{return route.project||!projects.length?route:{...route,project:projects[0].id};}

/** The selected task is reading context, independent of the editor source.
 * Keep the run locator empty so an IDE refresh cannot switch into Run view. */
export function workbenchTaskID(panel:string):string{const id=panel.startsWith('workbench-task:')?panel.slice('workbench-task:'.length):'';return /^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(id)?id:'';}
export function locateWorkbenchTask(id:string,canLeave:()=>boolean,update:(change:(route:WorkbenchRoute)=>WorkbenchRoute)=>void):boolean{
 if(id&&!/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(id)||!canLeave())return false;
 update(route=>({...route,view:'development',sub:'ide',panel:id?'workbench-task:'+id:'',record:'',run:'',node:'',attempt:'',q:'',layout:'list'}));
 return true;
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
