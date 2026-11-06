import {useEffect,useLayoutEffect,useRef,useState} from 'react';
import {ArrowRight,Search} from 'lucide-react';
import {useApi} from './api-context';
import type {ApiCaller} from './api';
import type {AdapterDescriptor} from './AdapterConnections';
import {formatNumber,tr} from './i18n';
import {readToolInventory} from './tool-discovery-model';
import {matchingQuickTools} from './tool-quick-connect-model';

export const quickNodeLookupLimit=8;
type Candidate={worker:string;count:number;deadline:number};
type Result={caller:ApiCaller;checking:boolean;incomplete:boolean;candidates:Candidate[]};
const t=(key:string,params?:Record<string,unknown>)=>tr('tool-quick-connect.node_match.'+key,params);

export function QuickNodeMatch({project,adapter,nodes,onUse}:{project:string;adapter:AdapterDescriptor;nodes:readonly string[];onUse:(worker:string)=>boolean}){
 const api=useApi(),[result,setResult]=useState<Result>(),[expired,setExpired]=useState(false);
 const request=useRef<AbortController|undefined>(undefined),useNode=useRef(onUse);
 useNode.current=onUse;
 useLayoutEffect(()=>()=>{request.current?.abort();request.current=undefined;},[api]);
 const current=result?.caller===api?result:undefined;
 const candidates=current?.candidates??[];
 const deadline=candidates.length?Math.min(...candidates.map(item=>item.deadline)):undefined;
 useEffect(()=>{
  if(deadline===undefined)return;
  const timer=setTimeout(()=>setExpired(true),Math.max(0,Math.ceil(deadline-performance.now())));
  return()=>clearTimeout(timer);
 },[deadline]);
 async function find(){
  if(request.current||nodes.length<2||nodes.length>quickNodeLookupLimit)return;
  const controller=new AbortController();
  request.current=controller;
  setExpired(false);
  setResult({caller:api,checking:true,incomplete:false,candidates:[]});
  const reads=await Promise.allSettled(nodes.map(async worker=>{
   const {view,deadline}=await readToolInventory(api,project,worker,controller.signal);
   const count=view.state==='fresh'?matchingQuickTools(view.items,adapter).filter(item=>item.available).length:0;
   return {worker,count,deadline};
  }));
  if(request.current!==controller||controller.signal.aborted)return;
  request.current=undefined;
  const now=performance.now(),found:Candidate[]=[];
  for(const read of reads)if(read.status==='fulfilled'&&read.value.count&&read.value.deadline>now)found.push(read.value);
  const incomplete=reads.some(read=>read.status==='rejected');
  // Use the result to choose a node only. The ordinary flow still reloads
  // its inventory and checks exact existing profiles before a save.
  if(!incomplete&&found.length===1&&useNode.current(found[0].worker))return;
  setResult({caller:api,checking:false,incomplete,candidates:found});
 }
 const status=current?(current.checking?'checking':expired?'expired':current.incomplete?'partial':candidates.length?'found':'empty'):'hint';
 return <div className="tool-quick-node-match" data-quick-node-matches aria-busy={current?.checking||undefined}>
  <div className="tool-quick-node-match-head">
   <p role="status">{t(status)}</p>
   <button type="button" data-quick-find-nodes aria-disabled={current?.checking||undefined} onClick={()=>void find()}>
    <Search size={15} aria-hidden="true"/>{t(current&&!current.checking?'retry':'find')}
   </button>
  </div>
  {!expired&&candidates.length>0&&<ul>
   {candidates.map(item=><li key={item.worker}><div><strong>{item.worker}</strong><p>{t('count',{count:formatNumber(item.count)})}</p></div>
    <button type="button" data-quick-use-node={item.worker} onClick={()=>{if(!request.current&&deadline!==undefined&&performance.now()<deadline)useNode.current(item.worker);}}>{t('use')}<ArrowRight size={15} aria-hidden="true"/></button>
   </li>)}
  </ul>}
 </div>;
}
