import type {Session} from './api';
import {resetTaskCollectionScope,type WorkbenchRoute} from './navigation';
export type Organization={id:string;name:string;role:string};
export type OrganizationMember={user_id:string;name:string;email:string;role:string;version:number};
export type ProjectGrant={project_id:string;user_id:string;name:string;email:string;role:string;version:number;status?:string};
export type Invitation={id:string;version:number;email:string;role:string;status:string;created_at:string;expires_at:string;grants:{project_id:string;role:string}[];token?:string;token_revealed?:boolean;recovery?:string};
export type Team={id:string;version:number;name:string;description:string;status:string};
export type TeamMember={id:string;team_id:string;resource_kind:'human'|'agent'|'tool';resource_id:string;project_id:string;name:string;role:string;version:number;status:string};
export const organizationRoles:Record<string,string>={owner:'collaboration.roles.organization.owner',admin:'collaboration.roles.organization.admin',member:'collaboration.roles.organization.member'};
export const projectRoles:Record<string,string>={admin:'collaboration.roles.project.admin',reviewer:'collaboration.roles.project.reviewer',developer:'collaboration.roles.project.developer',viewer:'collaboration.roles.project.viewer'};
export function invitationFragment(hash:string){const params=new URLSearchParams(hash.replace(/^#/,''));return params.get('invite')||'';}
export function invitationURL(origin:string,path:string,token:string){return origin+path+'#invite='+encodeURIComponent(token);}
export function switchedRoute(current:WorkbenchRoute,session:Session,saved?:WorkbenchRoute):WorkbenchRoute{const accessible=(route:WorkbenchRoute)=>session.projects.some(p=>p.id===route.project);if(saved&&accessible(saved))return {...saved};if(accessible(current))return {...current};return {...resetTaskCollectionScope(current),project:session.projects[0]?.id||'',record:'',run:'',node:'',attempt:'',panel:'',q:'',status:'all'};}
export function teamIdentity(kind:string,resourceID:string,project:string){return {resource_kind:kind,resource_id:resourceID,project_id:kind==='human'?'':project};}
