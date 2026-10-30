import {expect,it} from 'vitest';
import {locateTaskReviewRequest,parseWorkbenchRoute,taskReviewRequestID} from './navigation';
it('locates one request in existing task detail and does not bypass the leave guard',()=>{
 let route=parseWorkbenchRoute('?project=p&view=development&sub=inbox');const update=(change:(value:typeof route)=>typeof route)=>{route=change(route);};
 expect(locateTaskReviewRequest('task-a','request-a',()=>false,update)).toBe(false);expect(route.sub).toBe('inbox');
 expect(locateTaskReviewRequest('task-a','request-a',()=>true,update)).toBe(true);expect(route).toMatchObject({sub:'tasks',record:'task-a',panel:'task-review:request-a',run:'',conversation_message:undefined});
 expect(taskReviewRequestID(route.panel)).toBe('request-a');expect(locateTaskReviewRequest('task-a','../foreign',()=>true,update)).toBe(false);
});
it('parses only safe single review panels scoped to a selected task',()=>{
 const base='?project=p&view=development&sub=tasks&record=task-a&panel=';expect(parseWorkbenchRoute(base+'task-review:request-a').panel).toBe('task-review:request-a');
 for(const panel of ['task-review:../foreign','task-review:request-a&panel=task-review:other'])expect(parseWorkbenchRoute(base+panel).panel).toBe('');
 expect(parseWorkbenchRoute('?project=p&view=development&sub=inbox&panel=task-review:request-a').panel).toBe('');
 expect(parseWorkbenchRoute('?project=p&view=development&sub=tasks&panel=task-review:request-a').panel).toBe('');
});
