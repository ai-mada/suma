import {api,query,type Run} from './api';
export async function readLinkedRun(project:string,id:string|undefined):Promise<Run|undefined>{return id?api<Run>('/runs/'+encodeURIComponent(id)+query(project)):undefined;}
export function includeCurrentRun(items:Run[],current:Run|undefined):Run[]{return current?[...new Map([...items,current].map(run=>[run.id,run])).values()]:items;}
