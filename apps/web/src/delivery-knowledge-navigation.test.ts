import {expect,it,vi} from 'vitest';
import {deliveryKnowledgeTaskID,locateDeliveryKnowledge,type WorkbenchRoute} from './navigation';

it('rejects invalid delivery task locators before the leave guard and preserves a blocked route',()=>{
 const canNavigate=vi.fn(()=>false),setRoute=vi.fn();
 for(const taskID of ['', 'task.a', '../task-a', 'task:a', 'task-a\n', '任务', 'a'.repeat(65)])expect(locateDeliveryKnowledge(taskID,canNavigate,setRoute)).toBe(false);
 expect(canNavigate).not.toHaveBeenCalled();expect(setRoute).not.toHaveBeenCalled();
 expect(locateDeliveryKnowledge('task-a',canNavigate,setRoute)).toBe(false);expect(canNavigate).toHaveBeenCalledOnce();expect(setRoute).not.toHaveBeenCalled();
});

it('opens the exact delivery workspace without stale object context and parses only its task panel',()=>{
 let route:WorkbenchRoute={project:'project-a',view:'development',sub:'tasks',record:'old-analysis',panel:'task-review:review-a',run:'run-a',node:'node-a',attempt:'2',q:'private search',status:'blocked',layout:'graph',week:'2026-11-02',conversation_message:'message-a',inbox_filter:'unread',inbox_invalid:'1',task_columns:['title','owner'],task_invalid:'1',task_status:'blocked',task_owner:'owner-a',task_priority:'high',task_sort:'title_asc',task_group:'owner',task_requirement:'requirement-a',task_iteration:'iteration-a',task_iteration_unset:true,task_unassigned:true,task_priority_unset:true,task_status_unset:true,knowledge_kind:'skill',catalog_domain:'old-domain',catalog_scenario:'old-scenario',catalog_state:'published',catalog_source:'old-source'};
 const canNavigate=vi.fn(()=>true),setRoute=vi.fn((change:(current:WorkbenchRoute)=>WorkbenchRoute)=>{route=change(route);});
 expect(locateDeliveryKnowledge('Task_A-42',canNavigate,setRoute)).toBe(true);expect(canNavigate).toHaveBeenCalledOnce();expect(setRoute).toHaveBeenCalledOnce();
 expect(route).toMatchObject({project:'project-a',view:'knowledge',sub:'project-knowledge',panel:'delivery-knowledge:Task_A-42',record:'',run:'',node:'',attempt:'',q:'',status:'all',week:'2026-11-02'});
 for(const field of ['conversation_message','inbox_filter','inbox_invalid','task_columns','task_invalid','task_status','task_owner','task_priority','task_sort','task_group','task_requirement','task_iteration','task_iteration_unset','task_unassigned','task_priority_unset','task_status_unset','knowledge_kind','catalog_domain','catalog_scenario','catalog_state','catalog_source'] as const)expect(route[field],field).toBeUndefined();
 expect(deliveryKnowledgeTaskID(route.panel)).toBe('Task_A-42');expect(deliveryKnowledgeTaskID('delivery-knowledge:'+'a'.repeat(64))).toBe('a'.repeat(64));
 for(const panel of ['analysis-a','analysis:task-a','knowledge-analysis:task-a','delivery-knowledge:','delivery-knowledge:task.a','delivery-knowledge:task-a\n','delivery-knowledge:task-a:extra','delivery-knowledge:'+'a'.repeat(65)])expect(deliveryKnowledgeTaskID(panel),panel).toBeUndefined();
});
