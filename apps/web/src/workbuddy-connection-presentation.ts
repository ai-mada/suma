import type {WorkBuddyConnection} from '../../../packages/contracts/src';

export type WorkBuddyConnectionReason='effect_unknown'|'reauthorization_required'|'generic';
type ScopeList=Readonly<WorkBuddyConnection['scopes']>;

/** Display categories only; current allowed_actions remain the server's facts. */
export function workBuddyConnectionReason(connection:Readonly<Pick<WorkBuddyConnection,'state'|'last_error'>>):WorkBuddyConnectionReason|undefined{
 switch(connection.last_error){
  case 'workbuddy_effect_unknown':return 'effect_unknown';
  case 'workbuddy_reauthorization_required':return 'reauthorization_required';
  case '':break;
  default:return 'generic';
 }
 switch(connection.state){
  case 'authorization_unknown':
  case 'refresh_unknown':return 'effect_unknown';
  case 'reauthorization_required':return 'reauthorization_required';
  default:return undefined;
 }
}

/** Call only after the user explicitly chooses to reuse their current grant.
 * A newly available permission is never selected by that choice. */
export function workBuddyReusableScopes(current:ScopeList,available:ScopeList):WorkBuddyConnection['scopes']{
 const granted=new Set(current);
 return [...new Set(available)].filter(scope=>granted.has(scope));
}

export interface WorkBuddyScopeDifference{
 added:WorkBuddyConnection['scopes'];
 removed:WorkBuddyConnection['scopes'];
 retained:WorkBuddyConnection['scopes'];
 unavailable:WorkBuddyConnection['scopes'];
}

/** Compare the actual old scopes with the explicit request, not a filtered or
 * accepted grant. Unavailable covers their union in old-then-requested order. */
export function workBuddyScopeDifference(current:ScopeList,requested:ScopeList,available:ScopeList):WorkBuddyScopeDifference{
 const old=new Set(current),next=new Set(requested),requestable=new Set(available);
 return {
  added:[...next].filter(scope=>!old.has(scope)),
  removed:[...old].filter(scope=>!next.has(scope)),
  retained:[...next].filter(scope=>old.has(scope)),
  unavailable:[...new Set([...old,...next])].filter(scope=>!requestable.has(scope)),
 };
}
