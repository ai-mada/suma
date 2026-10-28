import {expect,it} from 'vitest';
import {taskDependencyChanges,taskDependencyDraft,taskDependencyProblem,taskDependencyReceipt,validTaskDependencyGraph,validTaskDependencyRecovery} from './task-dependencies-model';

const graph={project_id:'project-a',total:3,limit:1000,complete:true,tasks:[
 {task_id:'a',version:2,title:'Architecture',status:'ready',depends_on:[]},
 {task_id:'b',version:4,title:'Develop',status:'in_progress',depends_on:['a']},
 {task_id:'c',version:1,title:'Verify',status:'draft',depends_on:['b']},
]};

it('refuses partial, foreign and broken graphs rather than presenting a subset as a complete editable task graph',()=>{
 expect(validTaskDependencyGraph(graph,'project-a')).toBe(true);
 expect(validTaskDependencyGraph({...graph,project_id:'project-b'},'project-a')).toBe(false);
 expect(validTaskDependencyGraph({...graph,total:4},'project-a')).toBe(false);
 expect(validTaskDependencyGraph({...graph,tasks:[...graph.tasks,{...graph.tasks[0]}]},'project-a')).toBe(false);
 expect(validTaskDependencyGraph({...graph,tasks:[{...graph.tasks[0],depends_on:['missing']},...graph.tasks.slice(1)]},'project-a')).toBe(false);
 expect(validTaskDependencyGraph({project_id:'project-a',total:1001,limit:1000,complete:false,tasks:[]},'project-a')).toBe(true);
 expect(validTaskDependencyGraph({...graph,complete:false},'project-a')).toBe(false);
});

it('finds cycles and self edges across the whole draft while constructing a minimal version-bound batch without mutating the read',()=>{
 const draft=taskDependencyDraft(graph);draft.a=['c'];
 expect(taskDependencyProblem(graph,draft)).toBe('cycle');
 draft.a=['a'];expect(taskDependencyProblem(graph,draft)).toBe('self');
 draft.a=[];draft.c=['a'];expect(taskDependencyProblem(graph,draft)).toBeUndefined();
 expect(taskDependencyChanges(graph,draft)).toEqual([{task_id:'c',expected_version:1,depends_on:['a']}]);
 expect(graph.tasks[2].depends_on).toEqual(['b']);
 draft.c=['missing'];expect(taskDependencyProblem(graph,draft)).toBe('missing');
});

it('binds original acknowledgements and recovery to the exact batch rather than any current graph or success status',()=>{
 const body={project_id:'project-a',changes:[{task_id:'b',expected_version:4,depends_on:[]}],idempotency_key:'original-operation'};
 const request={path:'/task-dependencies',method:'POST',idempotency_key:body.idempotency_key,body};
 const receipt={project_id:'project-a',operation_id:body.idempotency_key,changes:[{task_id:'b',previous_version:4,version:5,depends_on:[]}]};
 expect(validTaskDependencyRecovery(request,'project-a')).toBe(true);
 expect(validTaskDependencyRecovery(request,'project-b')).toBe(false);
 expect(taskDependencyReceipt(receipt,request,'project-a')).toBe(true);
 for(const wrong of [{...receipt,project_id:'other'},{...receipt,operation_id:'other'}, {...receipt,changes:[{...receipt.changes[0],task_id:'a'}]}, {...receipt,changes:[{...receipt.changes[0],previous_version:3}]}, {...receipt,changes:[{...receipt.changes[0],depends_on:['a']}]}, {...receipt,changes:[]}])expect(taskDependencyReceipt(wrong,request,'project-a')).toBe(false);
});
