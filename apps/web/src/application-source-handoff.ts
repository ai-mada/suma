import type {ApplicationCodeScope,RepositoryIntegrationPlan} from '../../../packages/contracts/src';
import {repositoryApplied,repositoryID,validRepositoryPlan} from './repository-integration-model';

export type ApplicationSourceLocator={planID:string;taskID:string;version:number;applicationID?:string};
const prefix='application-source:';

/** Route values are locators only. The destination must read the current plan. */
export function parseApplicationSourcePanel(panel:string):ApplicationSourceLocator|undefined {
 if(!panel.startsWith(prefix)||panel.length>500)return undefined;
 const [planID,taskID,rawVersion,applicationID,...extra]=panel.slice(prefix.length).split(':');
 const version=Number(rawVersion);
 if(extra.length||!repositoryID(planID)||!repositoryID(taskID)||String(version)!==rawVersion||
    !Number.isSafeInteger(version)||version<1||version>2147483646||(applicationID!==undefined&&!repositoryID(applicationID)))return undefined;
 return {planID,taskID,version,applicationID};
}

export function applicationSourcePanel(source:ApplicationSourceLocator,applicationID=''):string {
 return prefix+[source.planID,source.taskID,String(source.version),...(applicationID?[applicationID]:[])].join(':');
}

export function applicationSourceHref(plan:RepositoryIntegrationPlan):string {
 const panel=applicationSourcePanel({planID:plan.id,taskID:plan.task_id,version:plan.version});
 return '?'+new URLSearchParams({project:plan.project_id,view:'overview',layout:'applications',panel});
}

export function readApplicationSource(value:unknown,project:string,source:ApplicationSourceLocator):value is RepositoryIntegrationPlan {
 return validRepositoryPlan(value,project,source.taskID,source.planID)&&value.version===source.version&&value.state==='integrated'&&repositoryApplied(value);
}

export function applicationScopeMatchesSource(scope:ApplicationCodeScope,plan:RepositoryIntegrationPlan):boolean {
 const target=plan.target;
 return scope.target_id===target.id&&scope.provider_id===target.provider_id&&scope.target_alias===target.target_alias&&
  scope.target_ref===target.ref&&scope.config_sha256===target.config_sha256;
}
