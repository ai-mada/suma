import {ApiError,type ApiCaller} from './api';
import type {PageSnapshot,EntityPage} from './pagination';
import {renderDiagnostic,UiError} from './i18n';

export const toolListStates=['active','enabled','paused','revoked','all'] as const;
export type ToolListState=typeof toolListStates[number];
export type ToolListFilters=Readonly<{state:ToolListState;query:string}>;
export function validToolListQuery(value:string){return !/[\u0000-\u001f\u007f-\u009f]/u.test(value)&&new TextEncoder().encode(value).length<=120;}
export function readToolListFilters(params:URLSearchParams):ToolListFilters{
 const state=params.get('tools_state'),query=params.get('tools_query')??'';
 return {state:toolListStates.includes(state as ToolListState)?state as ToolListState:'active',query:validToolListQuery(query)?query.trim():''};
}
export function toolListSearchParams(source:URLSearchParams,filters:ToolListFilters){
 const next=new URLSearchParams(source);
 if(filters.state==='active')next.delete('tools_state');else next.set('tools_state',filters.state);
 if(filters.query.trim())next.set('tools_query',filters.query.trim());else next.delete('tools_query');
 return next;
}
const empty=<T extends {id:string}>():PageSnapshot<T>=>({items:undefined,total:0,next_cursor:'',loading:false,error:'',error_action:'first'});

/** A filtered window belongs to one captured API caller and one project/query.
 * It does not share the task pager's cursor or preserve rows after a refusal. */
export class ToolProfilePager<T extends {id:string}>{
 private state=empty<T>();private generation=0;private failure:unknown;
 constructor(private project:string,private filters:ToolListFilters,private readonly caller:ApiCaller){}
 snapshot():PageSnapshot<T>{return {...this.state,error:this.failure?renderDiagnostic(this.failure):this.state.error,failure:this.failure};}
 setScope(project:string,filters:ToolListFilters){
  if(this.project===project&&this.filters.state===filters.state&&this.filters.query===filters.query)return;
  this.project=project;this.filters={...filters};this.generation++;this.failure=undefined;this.state=empty<T>();
 }
 invalidate(){this.generation++;this.failure=undefined;this.state=empty<T>();}
 async first(){return this.read('',false,++this.generation);}
 async more(){if(this.state.loading||!this.state.next_cursor)return false;return this.read(this.state.next_cursor,true,this.generation);}
 private async read(cursor:string,append:boolean,generation:number){
  this.failure=undefined;this.state={...this.state,loading:true,error:'',error_action:append?'more':'first'};
  try{
   if(!this.project)throw new UiError('workspace.select_project');
   if(!validToolListQuery(this.filters.query))throw new UiError('tool-workspace.query_invalid');
   const params=new URLSearchParams({project_id:this.project,state:this.filters.state,query:this.filters.query,limit:'100'});
   if(cursor)params.set('cursor',cursor);
   const page=await this.caller<EntityPage<T>>('/tool-profiles?'+params);
   if(generation!==this.generation)return false;
   if(!Array.isArray(page.items)||page.items.some(item=>!item||typeof item.id!=='string'||!item.id)||!Number.isSafeInteger(page.total)||page.total<0||page.next_cursor!==undefined&&typeof page.next_cursor!=='string')throw new UiError('workspace.pagination.invalid_page');
   const merged=new Map<string,T>();for(const item of [...(append?this.state.items??[]:[]),...page.items])merged.set(item.id,item);
   this.state={...this.state,items:[...merged.values()],total:page.total,next_cursor:page.next_cursor??'',error:''};return true;
  }catch(error){
   if(generation===this.generation){this.failure=error;if(error instanceof ApiError&&[401,403,404].includes(error.status))this.state=empty<T>();}
   return false;
  }finally{if(generation===this.generation)this.state={...this.state,loading:false};}
 }
}
