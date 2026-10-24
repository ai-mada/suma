// Keep one page and at most 100 previous positions. Cursor bytes are local,
// opaque navigation credentials; locale changes do not start reads.
import {useCallback,useEffect,useRef,useState} from 'react';
import type {AttemptToolEventPage} from '../../../packages/contracts/src';
import {ApiError,rawApi,query} from './api';
import {firstSavedPosition,validateAttemptToolPage,type AttemptToolScope,type SavedPosition} from './attempt-tool-history';
type RequestPosition={position:SavedPosition;previous:SavedPosition[];dropped:boolean};
type State={scope:string;limit:number;page?:AttemptToolEventPage;loading:boolean;failure?:unknown;denied?:number;position:SavedPosition;previous:SavedPosition[];dropped:boolean;requested:RequestPosition};
export function useAttemptToolHistory(scope:AttemptToolScope,refresh:number|string=0){
 const key=JSON.stringify(scope),[limit,setLimit]=useState(50),epoch=useRef(0),active=useRef<AbortController|undefined>(undefined),currentScope=useRef(key);
 currentScope.current=JSON.stringify([key,limit,refresh]);
 const initial=():State=>({scope:key,limit,loading:true,position:firstSavedPosition(),previous:[],dropped:false,requested:{position:firstSavedPosition(),previous:[],dropped:false}});
 const [state,setState]=useState<State>(initial),stateRef=useRef(state);stateRef.current=state;
 const binding=useRef(key+':'+limit);
 // Parse the stable serialized scope so fresh parent objects cannot cause I/O.
 const read=useCallback(async(requested:RequestPosition,clear=false)=>{
  const identity:AttemptToolScope=JSON.parse(key),generation=++epoch.current,requestBinding=currentScope.current;
  active.current?.abort();const controller=new AbortController();active.current=controller;
  setState(before=>({...((before.scope===key&&!clear)?before:initial()),scope:key,loading:true,failure:undefined,denied:undefined,requested}));
  const path='/runs/'+encodeURIComponent(identity.run_id)+'/nodes/'+encodeURIComponent(identity.node_id)+'/attempts/'+identity.attempt+'/tool-events'+query(identity.project_id)+'&limit='+limit+(requested.position.cursor?'&cursor='+encodeURIComponent(requested.position.cursor):'');
  try{
   const value=await rawApi<unknown>(path,controller.signal);
   if(controller.signal.aborted||generation!==epoch.current||currentScope.current!==requestBinding)return;
   const page=validateAttemptToolPage(value,identity,requested.position,limit);
   setState({scope:key,limit,page,loading:false,position:{...requested.position,cursor:page.page_cursor,watermark:page.watermark},previous:requested.previous,dropped:requested.dropped,requested});
  }catch(failure){
   if(controller.signal.aborted||generation!==epoch.current||currentScope.current!==requestBinding)return;
   const denied=failure instanceof ApiError&&[401,403,404].includes(failure.status)?failure.status:undefined;
   setState(before=>({...before,...(denied?{page:undefined,position:firstSavedPosition(),previous:[],dropped:false}:{}),loading:false,failure,denied,requested:denied?{position:firstSavedPosition(),previous:[],dropped:false}:requested}));
  }
 },[key,limit]);
 useEffect(()=>{const nextBinding=key+':'+limit,clear=binding.current!==nextBinding;binding.current=nextBinding;void read({position:firstSavedPosition(),previous:[],dropped:false},clear);return()=>{epoch.current++;active.current?.abort();};},[read,refresh]);
 const visible=state.scope===key&&state.limit===limit?state:initial();
 const latest=()=>void read({position:firstSavedPosition(),previous:[],dropped:false});
 const next=()=>{const before=stateRef.current;if(before.loading||before.failure||!before.page?.next_cursor)return;const previous=[...before.previous,before.position];void read({position:{cursor:before.page.next_cursor,after:before.page.scanned_through,watermark:before.page.watermark},previous:previous.slice(-100),dropped:before.dropped||previous.length>100});};
 const previous=()=>{const before=stateRef.current;if(before.loading||before.failure||!before.previous.length)return;void read({position:before.previous.at(-1)!,previous:before.previous.slice(0,-1),dropped:before.dropped});};
 return {...visible,limit,setLimit,latest,next,previousPage:previous,retry:()=>void read(stateRef.current.requested)};
}
