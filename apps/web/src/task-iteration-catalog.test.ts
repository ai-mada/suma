import {expect,it} from 'vitest';
import {taskIterationCatalog} from './task-iteration-catalog';

const iteration=(patch:Record<string,unknown>={})=>({id:'iteration-a',project_id:'project-a',version:1,title:'Iteration A',description:'',status:'active',start_date:'',end_date:'',goal:'',...patch});

it('retains completed and empty real current-project directories without deriving membership',()=>{
 expect(taskIterationCatalog({items:[]},'project-a')).toEqual([]);
 const result=taskIterationCatalog({items:[iteration(),iteration({id:'done',status:'completed'})]},'project-a');
 expect(result.map(item=>item.status)).toEqual(['active','completed']);
 expect(result[0]).not.toHaveProperty('project_id');
});

it.each([
 null,{}, {items:null},{items:[null]},{items:[iteration({project_id:'other'})]},
 {items:[iteration(),iteration()]},{items:[iteration({id:'../other'})]},
 {items:[iteration({version:0})]},{items:[iteration({version:1.5})]},
 {items:[iteration({status:'archived'})]},{items:[iteration({title:null})]},
])('rejects invalid catalog without widening the active task filter',value=>{
 expect(()=>taskIterationCatalog(value,'project-a')).toThrow();
});
