import type {ProjectNotificationPreferences,ProjectNotificationPreferencesCommand,ProjectNotificationPreferencesReceipt} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';
import {wellFormed} from './i18n/catalog';

export type PreferencesScope={project:string;userID:string;organizationID:string};
export type PreferencesValue=ProjectNotificationPreferences;
export type PreferencesReceipt=ProjectNotificationPreferencesReceipt;
const maximumVersion=2147483647,encoder=new TextEncoder();
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value)&&(Object.getPrototypeOf(value)===Object.prototype||Object.getPrototypeOf(value)===null);
const exact=(value:Record<string,unknown>,keys:string[])=>Reflect.ownKeys(value).length===keys.length&&keys.every(key=>Object.hasOwn(value,key));
const id=(value:unknown):value is string=>typeof value==='string'&&/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(value);
const version=(value:unknown):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>=0&&value<=maximumVersion;
const timestamp=(value:unknown):value is string=>{
 if(typeof value!=='string'||!/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?Z$/.test(value))return false;
 const parsed=Date.parse(value);return Number.isFinite(parsed)&&new Date(parsed).toISOString().slice(0,19)===value.slice(0,19);
};
const operation=(value:unknown):value is string=>typeof value==='string'&&wellFormed(value)&&value.trim()===value&&!/[\p{Cc}]/u.test(value)&&encoder.encode(value).length>=8&&encoder.encode(value).length<=128;
export const validPreferencesScope=(scope:PreferencesScope):boolean=>object(scope)&&[scope.project,scope.userID,scope.organizationID].every(id);
export const preferencesPath=(scope:PreferencesScope)=>'/notification-preferences?project_id='+encodeURIComponent(scope.project);
export const preferencesCommandPath=(scope:PreferencesScope)=>'/notification-preferences/commands?project_id='+encodeURIComponent(scope.project);
export function readProjectNotificationPreferences(raw:unknown,scope:PreferencesScope):PreferencesValue|undefined{
 if(!validPreferencesScope(scope)||!object(raw)||!exact(raw,['project_id','user_id','version','followed_discussions_muted','updated_at'])||raw.project_id!==scope.project||raw.user_id!==scope.userID||!version(raw.version)||typeof raw.followed_discussions_muted!=='boolean')return;
 if(raw.version===0?(raw.followed_discussions_muted||raw.updated_at!==''):!timestamp(raw.updated_at))return;
 return raw as unknown as PreferencesValue;
}
export function validPreferencesRequest(request:unknown,scope:PreferencesScope):request is WriteRecoveryRequest{
 if(!validPreferencesScope(scope)||!object(request)||!exact(request,['path','body','method','idempotency_key'])||request.path!==preferencesCommandPath(scope)||request.method!=='POST'||!operation(request.idempotency_key)||!object(request.body))return false;
 const body=request.body;
 return exact(body,['project_id','expected_version','followed_discussions_muted','idempotency_key'])&&body.project_id===scope.project&&version(body.expected_version)&&typeof body.followed_discussions_muted==='boolean'&&body.idempotency_key===request.idempotency_key;
}
/** A valid receipt confirms this original operation, not the current preference.
 * Consumers separately read current state after confirming or recovering it. */
export function validPreferencesReceipt(raw:unknown,request:WriteRecoveryRequest,scope:PreferencesScope):raw is PreferencesReceipt{
 if(!validPreferencesRequest(request,scope)||!object(raw)||!exact(raw,['idempotency_key','preferences'])||raw.idempotency_key!==request.idempotency_key)return false;
 const result=readProjectNotificationPreferences(raw.preferences,scope),command=request.body as unknown as ProjectNotificationPreferencesCommand;
 return !!result&&result.followed_discussions_muted===command.followed_discussions_muted&&(result.version===command.expected_version||command.expected_version<maximumVersion&&result.version===command.expected_version+1);
}
