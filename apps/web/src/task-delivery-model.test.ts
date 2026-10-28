import {expect,it} from 'vitest';
import {validTaskDelivery} from './task-delivery-model';
const summary={total:1,waiting:0,running:0,succeeded:1,failed:0,skipped:0,reconciling:0,other:0,limit:20,complete:true,items:[{id:'verify',name:'Independent checks',type:'test',state:'succeeded',attempt:1}]};
const delivery=()=>({project_id:'project-a',task_id:'task-a',task_version:3,task_status:'awaiting_review',run_basis_state:'current',formal_acceptance:false,run:{run_id:'run-a',version:4,state:'succeeded',nodes:summary,tests:summary},expert_plans:{scope:'current_run',total:0,limit:20,complete:true,items:[]},issues:{scope:'current_run',total:0,limit:20,complete:true,items:[]}});
it('requires the exact task scope, complete capped collections and consistent current-run counts',()=>{
 expect(validTaskDelivery(delivery(),'project-a','task-a')).toBe(true);expect(validTaskDelivery(delivery(),'project-a','other')).toBe(false);expect(validTaskDelivery({...delivery(),project_id:'other'},'project-a','task-a')).toBe(false);
 expect(validTaskDelivery({...delivery(),run:{...delivery().run,tests:{...summary,total:2}}},'project-a','task-a')).toBe(false);expect(validTaskDelivery({...delivery(),issues:{...delivery().issues,total:1}},'project-a','task-a')).toBe(false);expect(validTaskDelivery({...delivery(),issues:{scope:'current_run',total:1,limit:20,complete:true,items:[{issue_id:'issue-a',title:{forged:true},issue_version:1,issue_status:'open',source:'execution_failure',source_review_id:'',requirement_id:'',requirement_version:0}]}},'project-a','task-a')).toBe(false);
});
it('does not allow a successful run to claim formal acceptance, and distinguishes an undispatched task',()=>{
 expect(validTaskDelivery({...delivery(),formal_acceptance:true},'project-a','task-a')).toBe(false);expect(validTaskDelivery({...delivery(),task_status:'accepted',formal_acceptance:true},'project-a','task-a')).toBe(true);
 expect(validTaskDelivery({...delivery(),task_status:'draft',run_basis_state:'not_dispatched',run:null},'project-a','task-a')).toBe(true);expect(validTaskDelivery({...delivery(),run_basis_state:'current',run:null},'project-a','task-a')).toBe(false);
});
