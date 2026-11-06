import {useEffect,useState} from 'react';
import {Bell} from 'lucide-react';
import type {ApiCaller} from './api';
import {useApi} from './api-context';
import {formatNumber,tr,useLocale} from './i18n';
import {notificationInboxChangedEvent,notificationOrganizationInboxScope,validNotificationOrganizationUnreadSummary} from './notification-inbox-model';
import './NotificationInboxHeader.css';

export {notifyOrganizationInboxChanged} from './notification-inbox-model';

type Summary = {scope:string; api:ApiCaller; refresh:number; unread:number};
type Props = {userID:string; organizationID:string; refresh:number; current?:boolean; onOpen:()=>void};
const foregroundRefreshGap = 3000;

/** A navigation affordance remains available even while the count is unknown. */
export function NotificationInboxHeader({userID,organizationID,refresh,current,onOpen}:Props){
 useLocale();
 const api=useApi();
 const scope=notificationOrganizationInboxScope(userID,organizationID);
 const [summary,setSummary]=useState<Summary>();
 const unread=summary?.scope===scope&&summary.api===api&&summary.refresh===refresh?summary.unread:0;
 const label=unread>0?tr('notification-inbox.open_organization_unread',{unread:formatNumber(unread)}):tr('notification-inbox.open_organization');

 useEffect(()=>{
  if(!userID||!organizationID)return;
  let pending:AbortController|undefined;
  let lastRead=-Infinity;
  async function read(){
   pending?.abort();
   const controller=new AbortController();
   pending=controller;
   lastRead=Date.now();
   try{
    const value=await api<unknown>('/notification-inbox/organization/unread-summary',undefined,'GET',{
     'X-Suma-Expected-User':userID,
     'X-Suma-Expected-Organization':organizationID,
    },controller.signal);
    if(controller.signal.aborted)return;
    setSummary(validNotificationOrganizationUnreadSummary(value,organizationID,userID)?{scope,api,refresh,unread:value.unread}:undefined);
   }catch{
    if(!controller.signal.aborted)setSummary(undefined);
   }finally{
    if(pending===controller)pending=undefined;
   }
  }
  function foreground(){
   if(!pending&&document.visibilityState!=='hidden'&&navigator.onLine!==false&&Date.now()-lastRead>=foregroundRefreshGap)void read();
  }
  function changed(event:Event){
   if((event as CustomEvent<unknown>).detail!==scope)return;
   // A local change can affect both the count and the set of visible projects.
   // Re-read authority before showing an updated count.
   setSummary(undefined);
   void read();
  }
  window.addEventListener(notificationInboxChangedEvent,changed);
  window.addEventListener('focus',foreground);
  window.addEventListener('online',foreground);
  document.addEventListener('visibilitychange',foreground);
  void read();
  return ()=>{
   pending?.abort();
   window.removeEventListener(notificationInboxChangedEvent,changed);
   window.removeEventListener('focus',foreground);
   window.removeEventListener('online',foreground);
   document.removeEventListener('visibilitychange',foreground);
  };
 },[api,organizationID,refresh,scope,userID]);

 return <button type="button" className="icon-button notification-inbox-header" aria-label={label} title={label} aria-current={current?'page':undefined} onClick={onOpen}>
  <Bell size={18} aria-hidden="true"/>
  {unread>0&&<span className="notification-inbox-header-count" aria-hidden="true">{unread>99?tr('notification-inbox.header_count_limit',{countText:formatNumber(99)}):formatNumber(unread)}</span>}
 </button>;
}
