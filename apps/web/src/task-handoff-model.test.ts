import {expect,it} from 'vitest';
import {draftAfterHandoffReceipt,emptyHandoffDraft,handoffDraftKey,handoffPath,readHandoffDraft,saveHandoffDraft,validHandoff,validHandoffPage,validHandoffReceipt,validHandoffRecovery,type HandoffDraft} from './task-handoff-model';
import type {WriteRecoveryRequest} from './write-action';

function row(patch:Record<string,unknown>={}){return {id:'handoff-a',project_id:'project-a',task_id:'task-a',version:1,state:'pending',created_by:'user-a',created_at:'2026-10-26T00:00:00Z',updated_at:'2026-10-26T00:00:00Z',to_resource_id:'person-b',to_resource_version:3,to_user_id:'user-b',to_name:'B',to_team_id:'',to_team_name:'',to_team_version:0,to_membership_version:0,note:'Original note',basis:{task_version:7,task_basis_sha256:'a'.repeat(64),requirement_id:'',requirement_version:0,from_owner_id:'',source_run_id:'',source_run_version:0,source_run_state:'',delivery_sha256:''},decision_by:'',decision_at:'',decision_reason:'',result_task_version:0,basis_current:true,blockers:[],allowed_actions:['accept','return'],...patch};}
const original:WriteRecoveryRequest={path:'/task-handoffs/handoff-a/commands',method:'POST',idempotency_key:'original-operation',body:{project_id:'project-a',expected_version:1,action:'return',reason:'My reason',acknowledge_invalidation:false,idempotency_key:'original-operation'}};
function storage(){const values=new Map<string,string>();return {getItem:(key:string)=>values.get(key)??null,setItem:(key:string,value:string)=>{values.set(key,value);},removeItem:(key:string)=>{values.delete(key);}};}

it('validates exact current handoff ownership, basis and actionable fields independently of history pages',()=>{
 expect(handoffPath('project-a','task-a','handoff-a')).toBe('/task-handoffs/handoff-a?project_id=project-a&task_id=task-a');
 expect(validHandoff(row(),'project-a','task-a','handoff-a')).toBe(true);
 for(const patch of [{id:'other'},{project_id:'other'},{task_id:'other'},{basis:null},{basis:{...row().basis,task_version:1.5}},{allowed_actions:['review']},{allowed_actions:['accept','accept']},{blockers:null},{version:2147483648},{state:'accepted'}])expect(validHandoff(row(patch),'project-a','task-a','handoff-a')).toBe(false);
 expect(validHandoff(row({state:'withdrawn',allowed_actions:[]}),'project-a','task-a','handoff-a')).toBe(true);
});

it('rejects damaged page recipients and sources before exposing an executable selection',()=>{
 const page={items:[row()],total:1,next_cursor:'',recipients:[],can_create:false,create_blockers:[],task_version:7};
 expect(validHandoffPage(page,'project-a','task-a')).toBe(true);
 for(const patch of [{items:[row(),row()]},{next_cursor:'other-id'},{recipients:[{resource_id:'person-a',teams:null}]},{items:[row({task_id:'other-task'})]},{task_version:0}])expect(validHandoffPage({...page,...patch},'project-a','task-a')).toBe(false);
});

it('keeps private draft schemas strict, scoped and free of source body snapshots or stored acknowledgement',()=>{
 const target=storage(),key=handoffDraftKey('user-a','org-a','project-a','task-a'),draft:HandoffDraft={recipient:'person-b',team:'',note:'Original <text> 中文',task_version:7,decision:{id:'handoff-a',version:1,action:'accept',reason:'My reason'}};
 saveHandoffDraft(target,key,draft);expect(readHandoffDraft(target,key)).toEqual({draft});
 for(const scope of [['user-b','org-a','project-a','task-a'],['user-a','org-b','project-a','task-a'],['user-a','org-a','project-b','task-a'],['user-a','org-a','project-a','task-b']])expect(readHandoffDraft(target,handoffDraftKey(...scope as [string,string,string,string])).draft).toEqual(emptyHandoffDraft());
 const saved=JSON.parse(target.getItem(key)!);
 for(const patch of [{history:[row()]},{decision:{...draft.decision,acknowledged:true}},{decision:{...draft.decision,version:1.25}},{recipient:'../person'},{note:'\ud800'}]){target.setItem(key,JSON.stringify({...saved,...patch}));expect(readHandoffDraft(target,key).invalid).toBe(true);expect(target.getItem(key)).not.toBeNull();}
});

it('clears only the original confirmed decision and leaves a separate proposal or newer version intact',()=>{
 const draft:HandoffDraft={recipient:'person-c',team:'team-c',note:'Separate proposal',task_version:8,decision:{id:'handoff-a',version:1,action:'return',reason:'My reason'}};
 expect(draftAfterHandoffReceipt(draft,original)).toEqual({...draft,decision:undefined});
 for(const decision of [{...draft.decision!,id:'handoff-b'},{...draft.decision!,version:2},{...draft.decision!,reason:'New text'}]){const newer={...draft,decision};expect(draftAfterHandoffReceipt(newer,original)).toBe(newer);}
 const create:WriteRecoveryRequest={...original,path:'/task-handoffs',body:{project_id:'project-a',task_id:'task-a',expected_task_version:8,to_resource_id:'person-c',to_team_id:'team-c',note:'Separate proposal'}};
 expect(draftAfterHandoffReceipt(draft,create)).toEqual({...emptyHandoffDraft(),decision:draft.decision});
});

it('recovers only the original supported command and validates its exact reply before clearing drafts',()=>{
 expect(validHandoffRecovery(original,'project-a','task-a')).toBe(true);
 for(const patch of [{expected_version:1.01},{expected_version:2147483647},{action:'review'},{acknowledge_invalidation:true},{task_id:'foreign'}])expect(validHandoffRecovery({...original,body:{...original.body,...patch}},'project-a','task-a')).toBe(false);
 const receipt=row({state:'returned',version:2,decision_reason:'My reason',allowed_actions:[]});expect(validHandoffReceipt(receipt,original,'project-a','task-a')).toBe(true);
 for(const patch of [{id:'handoff-b'},{version:3},{state:'accepted'},{decision_reason:'Different reason'}])expect(validHandoffReceipt({...receipt,...patch},original,'project-a','task-a')).toBe(false);
});
