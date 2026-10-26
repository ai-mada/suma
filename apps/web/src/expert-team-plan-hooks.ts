import {useEffect,useRef} from 'react';

export const expertPlanRefreshInterval=5000;
/** Read only the opened plan. Human review can progress without a run event. */
export function useExpertPlanRefresh(scope:string,enabled:boolean,read:()=>Promise<unknown>){
 const latest=useRef(read);latest.current=read;
 useEffect(()=>{if(!enabled)return;let closed=false,reading=false,timer:ReturnType<typeof setInterval>|undefined;
  const refresh=async()=>{if(closed||reading||document.visibilityState==='hidden')return;reading=true;try{await latest.current();}finally{reading=false;}};
  const schedule=()=>{clearInterval(timer);if(document.visibilityState!=='hidden')timer=setInterval(()=>void refresh(),expertPlanRefreshInterval);};
  const visible=()=>{schedule();if(document.visibilityState!=='hidden')void refresh();};schedule();document.addEventListener('visibilitychange',visible);
  return()=>{closed=true;clearInterval(timer);document.removeEventListener('visibilitychange',visible);};
 },[scope,enabled]);
}
