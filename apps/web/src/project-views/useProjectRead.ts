import {useEffect,useState} from 'react';

/** Read-only project snapshots never borrow a previous project's data. Locale
 * changes only rerender consumers; they are deliberately not read dependencies. */
export function useProjectRead<T>(project:string,refresh:number,read:(project:string)=>Promise<T>){
 const [snapshot,setSnapshot]=useState<{project:string;data?:T;error?:unknown}>({project});
 useEffect(()=>{
  let active=true;setSnapshot({project});
  if(project)void read(project).then(data=>{if(active)setSnapshot({project,data});}).catch(error=>{if(active)setSnapshot({project,error});});
  return()=>{active=false;};
 },[project,refresh,read]);
 return snapshot.project===project?snapshot:{project};
}
