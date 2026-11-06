import type {ReactNode} from 'react';

export const canLeave=()=>window.dispatchEvent(new Event('suma:navigate',{cancelable:true}));

/** Keep a real URL for copy/new-tab and keyboard use. Only an ordinary current
 * page activation uses the optional SPA callback; locked references have no URL. */
export function DeliveryReference({kind,id,href,locked,canLeave,onNavigate,children}:{
 kind:'plan'|'issue'|'requirement'|'review'|'workbuddy'|'workbuddy-origin-link'|'repository'|'current-link'|'repair-issue'|'repair-run'|'repair-trigger'|'repair-pause'|'repair-plan';id:string;href?:string;locked:boolean;
 canLeave:()=>boolean;onNavigate?:()=>void;children:ReactNode;
}){
 const disabled=locked||!href;
 return <a className="task-delivery-reference" {...{['data-task-delivery-'+kind]:id}}
  href={disabled?undefined:href} aria-disabled={disabled||undefined} tabIndex={disabled?-1:undefined}
  onClick={event=>{
   if(disabled){event.preventDefault();return;}
   if(event.button!==0||event.ctrlKey||event.metaKey||event.shiftKey||event.altKey)return;
   if(!canLeave()){event.preventDefault();return;}
   if(onNavigate){event.preventDefault();onNavigate();}
  }}>{children}</a>;
}
