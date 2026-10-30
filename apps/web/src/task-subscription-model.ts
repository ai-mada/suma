import type {TaskSubscription,TaskSubscriptionCommand,TaskSubscriptionReceipt} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';

export type TaskSubscriptionScope={project:string;taskID:string;userID:string;organizationID:string};
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const exact=(value:Record<string,unknown>,keys:string[])=>Object.keys(value).length===keys.length&&keys.every(key=>Object.hasOwn(value,key));
const id=(value:unknown):value is string=>typeof value==='string'&&/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(value);
const version=(value:unknown):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>=0&&value<=2147483647;
const timestamp=(value:unknown):value is string=>{
 if(typeof value!=='string'||!/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?Z$/.test(value))return false;
 const parsed=Date.parse(value);return Number.isFinite(parsed)&&new Date(parsed).toISOString().slice(0,19)===value.slice(0,19);
};
const operation=(value:unknown):value is string=>typeof value==='string'&&value.trim()===value&&!/[\p{Cc}]/u.test(value)&&new TextEncoder().encode(value).length>=8&&new TextEncoder().encode(value).length<=128;
export const validTaskSubscriptionScope=(scope:TaskSubscriptionScope)=>[scope.project,scope.taskID,scope.userID,scope.organizationID].every(id);
export const subscriptionPath=(scope:TaskSubscriptionScope)=>'/tasks/'+encodeURIComponent(scope.taskID)+'/subscription?project_id='+encodeURIComponent(scope.project);
export const subscriptionCommandPath=(scope:TaskSubscriptionScope)=>'/tasks/'+encodeURIComponent(scope.taskID)+'/subscription/commands?project_id='+encodeURIComponent(scope.project);
export function readTaskSubscription(value:unknown,scope:TaskSubscriptionScope):TaskSubscription|undefined{
 if(!validTaskSubscriptionScope(scope)||!object(value)||!exact(value,['project_id','task_id','user_id','version','following','updated_at'])||value.project_id!==scope.project||value.task_id!==scope.taskID||value.user_id!==scope.userID||!version(value.version)||typeof value.following!=='boolean')return;
 if(value.version===0?(value.following||value.updated_at!==''):!timestamp(value.updated_at))return;
 return value as unknown as TaskSubscription;
}
export function validTaskSubscriptionRequest(value:unknown,scope:TaskSubscriptionScope):value is WriteRecoveryRequest{
 if(!validTaskSubscriptionScope(scope)||!object(value)||!exact(value,['path','body','method','idempotency_key'])||value.path!==subscriptionCommandPath(scope)||value.method!=='POST'||!object(value.body)||!operation(value.idempotency_key))return false;
 const body=value.body;
 return exact(body,['project_id','task_id','expected_version','action','idempotency_key'])&&body.project_id===scope.project&&body.task_id===scope.taskID&&version(body.expected_version)&&(body.action==='follow'||body.action==='unfollow')&&body.idempotency_key===value.idempotency_key;
}
export function validTaskSubscriptionReceipt(value:unknown,request:WriteRecoveryRequest,scope:TaskSubscriptionScope):value is TaskSubscriptionReceipt{
 if(!validTaskSubscriptionRequest(request,scope)||!object(value)||!exact(value,['idempotency_key','subscription'])||value.idempotency_key!==request.idempotency_key)return false;
 const result=readTaskSubscription(value.subscription,scope),command=request.body as unknown as TaskSubscriptionCommand;
 return !!result&&result.following===(command.action==='follow')&&result.version>=command.expected_version&&result.version<=command.expected_version+1;
}
