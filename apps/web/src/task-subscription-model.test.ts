import {expect,it} from 'vitest';
import {readTaskSubscription,subscriptionCommandPath,validTaskSubscriptionReceipt,validTaskSubscriptionRequest} from './task-subscription-model';
const scope={project:'project-a',taskID:'task-a',userID:'user-a',organizationID:'org-a'};
const initial={project_id:'project-a',task_id:'task-a',user_id:'user-a',version:0,following:false,updated_at:''};
const original={path:'/tasks/task-a/subscription/commands?project_id=project-a',method:'POST',idempotency_key:'follow-original-154',body:{project_id:'project-a',task_id:'task-a',expected_version:0,action:'follow',idempotency_key:'follow-original-154'}};
it('reads only the current person and task without inventing a persisted initial preference',()=>{
 expect(readTaskSubscription(initial,scope)).toEqual(initial);
 for(const change of [{user_id:'other'},{project_id:'other'},{task_id:'other'},{version:-1},{version:0.5},{following:true},{updated_at:'2026-10-30T01:00:00Z'},{recipient_id:'other'}])expect(readTaskSubscription({...initial,...change},scope)).toBeUndefined();
 expect(readTaskSubscription({...initial,version:2,following:true,updated_at:'2026-10-30T01:00:00.123456789Z'},scope)?.following).toBe(true);
 expect(readTaskSubscription({...initial,version:2,updated_at:'2026-02-30T01:00:00.123Z'},scope)).toBeUndefined();
});
it('retains only the exact scoped command body and original operation for recovery',()=>{
 expect(subscriptionCommandPath(scope)).toBe('/tasks/task-a/subscription/commands?project_id=project-a');
 expect(validTaskSubscriptionRequest(original,scope)).toBe(true);
 for(const change of [{user_id:'other'},{task_id:'other'},{project_id:'other'},{expected_version:-1},{action:'toggle'},{idempotency_key:'new-operation'}])expect(validTaskSubscriptionRequest({...original,body:{...original.body,...change}},scope)).toBe(false);
 expect(validTaskSubscriptionRequest({...original,path:'/tasks/task-b/subscription/commands?project_id=project-a'},scope)).toBe(false);
 expect(validTaskSubscriptionRequest({...original,path:original.path+'&user_id=other'},scope)).toBe(false);
});
it('accepts only the original receipt with the intended following state and unchanged or incremented version',()=>{
 const subscription={...initial,version:1,following:true,updated_at:'2026-10-30T01:00:00Z'},receipt={idempotency_key:original.idempotency_key,subscription};
 expect(validTaskSubscriptionReceipt(receipt,original,scope)).toBe(true);
 for(const change of [{user_id:'other'},{following:false},{version:0},{version:2}])expect(validTaskSubscriptionReceipt({...receipt,subscription:{...subscription,...change}},original,scope)).toBe(false);
 expect(validTaskSubscriptionReceipt({...receipt,idempotency_key:'another-operation'},original,scope)).toBe(false);
 const noop={...original,body:{...original.body,action:'unfollow'}};expect(validTaskSubscriptionReceipt({idempotency_key:original.idempotency_key,subscription:initial},noop,scope)).toBe(true);
 const prior={...original,body:{...original.body,expected_version:2}};
 expect(validTaskSubscriptionReceipt({...receipt,subscription:{...subscription,version:2}},prior,scope)).toBe(true);
 expect(validTaskSubscriptionReceipt({...receipt,subscription:{...subscription,version:3}},prior,scope)).toBe(true);
});
