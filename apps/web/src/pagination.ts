import {api,query,type RecordItem} from './api';
export type EntityPage<T extends {id:string}=RecordItem>={items:T[];total:number;next_cursor?:string};
export type PageSnapshot<T extends {id:string}=RecordItem>={items:T[]|undefined;total:number;next_cursor:string;loading:boolean;error:string;error_action:'first'|'more'};

// One explicit request per action. A newer first page or context invalidates
// an older in-flight response, while failed reads keep the current window.
export class EntityPager<T extends {id:string}=RecordItem>{
 private state:PageSnapshot<T>={items:undefined,total:0,next_cursor:'',loading:false,error:'',error_action:'first'};
 private generation=0;
 constructor(public project:string,public kind:string,public limit=100){}
 snapshot():PageSnapshot<T>{return {...this.state};}
 setScope(project:string,kind:string){if(project===this.project&&kind===this.kind)return;this.project=project;this.kind=kind;this.generation++;this.state={items:undefined,total:0,next_cursor:'',loading:false,error:'',error_action:'first'};}
 async first(){const generation=++this.generation;return this.read('',false,generation);}
 async more(){if(this.state.loading||!this.state.next_cursor)return false;return this.read(this.state.next_cursor,true,this.generation);}
 private async read(cursor:string,append:boolean,generation:number){
  this.state={...this.state,loading:true,error:'',error_action:append?'more':'first'};
  const path='/'+this.kind+query(this.project)+'&limit='+this.limit+(cursor?'&cursor='+encodeURIComponent(cursor):'');
  try{
   const page=await api<EntityPage<T>>(path);
   if(generation!==this.generation)return false;
   if(!Array.isArray(page.items)||typeof page.total!=='number')throw new Error('列表响应缺少有效的记录或总数。');
   const merged=new Map<string,T>();
   for(const item of [...(append?this.state.items||[]:[]),...page.items])merged.set(item.id,item);
   this.state={...this.state,items:[...merged.values()],total:page.total,next_cursor:page.next_cursor||'',error:''};
   return true;
  }catch(error){if(generation===this.generation)this.state={...this.state,error:(error as Error).message};return false;}
  finally{if(generation===this.generation)this.state={...this.state,loading:false};}
 }
}
