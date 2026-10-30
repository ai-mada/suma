import {expect,it} from 'vitest';
import {taskColumnIDs,taskColumns,moveTaskColumn,toggleTaskColumn} from './task-columns-model';
it('accepts only the complete exact visible order with title fixed first and distinguishes omission from invalid input',()=>{
 expect(taskColumns(undefined)).toEqual(taskColumnIDs);expect(taskColumns(['title'])).toEqual(['title']);expect(taskColumns(['title','updated','owner'])).toEqual(['title','updated','owner']);
 for(const value of [null,[],Object.assign(new Array(2),{0:'title'}),['status','title'],['title','title'],['title','unknown'],['title',1],['title','status','priority','owner','updated','status']])expect(taskColumns(value)).toBeUndefined();
});
it('moves and toggles immutable optional columns without hiding or moving the title',()=>{
 const original=['title','status','priority','owner','updated'];expect(moveTaskColumn(original,'owner',-1)).toEqual(['title','status','owner','priority','updated']);expect(original).toEqual(taskColumnIDs);expect(moveTaskColumn(original,'status',-1)).toEqual(original);expect(toggleTaskColumn(original,'title',false)).toEqual(original);expect(toggleTaskColumn(['title'],'updated',true)).toEqual(['title','updated']);expect(toggleTaskColumn(original,'owner',false)).toEqual(['title','status','priority','updated']);expect(moveTaskColumn(null,'owner',1)).toBeUndefined();
});
