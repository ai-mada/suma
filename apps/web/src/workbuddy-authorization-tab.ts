import {officialWorkBuddyAuthorizationURL} from './workbuddy-connections-model';

export type WorkBuddyAuthorizationTab={
 navigate:(url:string)=>boolean;
 close:()=>void;
};

/** Reserve a tab during the user's gesture. Only the original empty document
 * may be used, and the provider never receives access to the opening window. */
export function reserveWorkBuddyAuthorizationTab(message:string):WorkBuddyAuthorizationTab|undefined{
 let tab:Window|null=null;
 let waitingDocument:Document|undefined;
 let timer:ReturnType<typeof setTimeout>|undefined;
 function ownsTab(){
  return !!tab&&!tab.closed&&!!waitingDocument&&tab.document===waitingDocument;
 }
 function close(){
  clearTimeout(timer);
  try{if(ownsTab())tab?.close();}catch{/* The tab has left the waiting document. */}
 }
 try{
  tab=window.open('','_blank');
  if(!tab)return;
  waitingDocument=tab.document;
  tab.opener=null;
  if(tab.opener!==null){close();return;}
  waitingDocument.documentElement.lang=document.documentElement.lang;
  waitingDocument.title=message;
  const status=waitingDocument.createElement('p');
  status.setAttribute('role','status');
  status.textContent=message;
  waitingDocument.body.replaceChildren(status);
  timer=setTimeout(close,15000);
  return {
   navigate(url){
    const doc=waitingDocument;
    if(!doc||!officialWorkBuddyAuthorizationURL(url))return false;
    try{
     if(!ownsTab())return false;
     const link=doc.createElement('a');
     link.href=url;
     link.target='_self';
     link.rel='noreferrer';
     doc.body.append(link);
     link.click();
     clearTimeout(timer);
     return true;
    }catch{return false;}
   },
   close,
  };
 }catch{close();return undefined;}
}
