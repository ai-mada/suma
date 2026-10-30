export const taskColumnIDs=Object.freeze(['title','status','priority','owner','updated'] as const);
export type TaskColumnID=typeof taskColumnIDs[number];
/** Omission is the declared product default. Invalid input never becomes it. */
export function taskColumns(value:unknown):readonly TaskColumnID[]|undefined{
 if(value===undefined)return taskColumnIDs;
 if(!Array.isArray(value)||value.length<1||value.length>5||value[0]!=='title'||new Set(value).size!==value.length||!Array.from(value).every(id=>typeof id==='string'&&taskColumnIDs.includes(id as TaskColumnID)))return;
 return value;
}
export function moveTaskColumn(value:unknown,id:TaskColumnID,direction:-1|1):string[]|undefined{
 const columns=taskColumns(value);if(!columns||!taskColumnIDs.includes(id))return;
 const next=[...columns],index=next.indexOf(id),target=index+direction;
 if(index>0&&target>0&&target<next.length)[next[index],next[target]]=[next[target],next[index]];
 return next;
}
export function toggleTaskColumn(value:unknown,id:TaskColumnID,checked:boolean):string[]|undefined{
 const columns=taskColumns(value);if(!columns||!taskColumnIDs.includes(id))return;
 if(id==='title'||checked&&columns.includes(id))return [...columns];
 return checked?[...columns,id]:columns.filter(column=>column!==id);
}
