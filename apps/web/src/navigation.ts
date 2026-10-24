import {mondayOf} from './planning-model';
export type WorkbenchView='overview'|'requirements'|'development'|'quality'|'operations'|'knowledge'|'settings';
export type WorkbenchRoute={project:string;view:WorkbenchView;sub:string;record:string;run:string;node:string;q:string;status:string;layout:string;week:string;panel:string};
const views:WorkbenchView[]=['overview','requirements','development','quality','operations','knowledge','settings'];
const defaultSub:Partial<Record<WorkbenchView,string>>={development:'tasks',quality:'issues',operations:'releases',knowledge:'documents',settings:'organization'};
export function parseWorkbenchRoute(search:string):WorkbenchRoute{
 const p=new URLSearchParams(search),view=views.includes(p.get('view') as WorkbenchView)?p.get('view') as WorkbenchView:'overview';
 return {project:p.get('project')||'',view,sub:p.get('sub')||defaultSub[view]||'',record:p.get('record')||'',run:p.get('run')||'',node:p.get('node')||'',q:p.get('q')||'',status:p.get('status')||'all',layout:p.get('layout')||'list',week:p.get('week')||mondayOf(),panel:p.get('panel')||''};
}
export function initializeSessionRoute(route:WorkbenchRoute,projects:{id:string}[]):WorkbenchRoute{return route.project||!projects.length?route:{...route,project:projects[0].id};}
