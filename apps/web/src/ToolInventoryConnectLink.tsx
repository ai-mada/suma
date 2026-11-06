import {useId,useLayoutEffect,useMemo,useRef,useState} from 'react';
import {Terminal} from 'lucide-react';
import {ApiError} from './api';
import {useApi,useApiView} from './api-context';
import {tr,UiError,useLocale} from './i18n';
import {ErrorNotice} from './ui';
import {parseToolInventoryConnectLink,reviewToolInventoryConnectLink,toolInventoryConnectPrefix,type ToolInventoryConnectReview} from './tool-inventory-connect-link-model';
import './ToolInventoryConnectLink.css';

type Props={
 project:string;
 panel:string;
 disabled:boolean;
 onReview:(value:ToolInventoryConnectReview)=>void;
 onDismiss?:()=>void;
};
const t=(key:string)=>tr('tool-inventory-connect-link.'+key);

export function ToolInventoryConnectLink({project,panel,disabled,onReview,onDismiss}:Props){
 useLocale();
 const caller=useApi(),view=useApiView(),heading=useId();
 const userID=view?.userID,organizationID=view?.organizationID;
 const scope=useMemo(()=>({caller,project,panel,disabled,userID,organizationID}),[caller,project,panel,disabled,userID,organizationID]);
 const currentScope=useRef(scope);
 currentScope.current=scope;
 const review=useRef(onReview);
 review.current=onReview;
 const request=useRef<{scope:typeof scope;controller:AbortController}|undefined>(undefined);
 const [status,setStatus]=useState<{scope:typeof scope;pending?:boolean;error?:UiError}>();
 useLayoutEffect(()=>()=>{if(request.current?.scope===scope)request.current.controller.abort();},[scope]);
 const current=status?.scope===scope?status:undefined;
 if(!panel.startsWith(toolInventoryConnectPrefix))return null;
 const pin=parseToolInventoryConnectLink(panel,project);

 async function check(){
  if(!pin||disabled||currentScope.current!==scope||request.current?.scope===scope&&!request.current.controller.signal.aborted)return;
  const controller=new AbortController();
  request.current={scope,controller};
  setStatus({scope,pending:true});
  const owns=()=>currentScope.current===scope&&request.current?.controller===controller&&!controller.signal.aborted;
  try{
   const headers=userID&&organizationID?{'X-Suma-Expected-User':userID,'X-Suma-Expected-Organization':organizationID}:undefined;
   const inventoryQuery=new URLSearchParams({project_id:pin.projectID,worker_id:pin.workerID});
   const catalogQuery=new URLSearchParams({project_id:pin.projectID});
   const started=performance.now();
   const [report,catalog]=await Promise.all([
    caller<unknown>('/tool-discovery?'+inventoryQuery,undefined,'GET',headers,controller.signal),
    caller<unknown>('/adapter-catalog?'+catalogQuery,undefined,'GET',headers,controller.signal),
   ]);
   if(!owns())return;
   const value=reviewToolInventoryConnectLink(pin,report,catalog,performance.now()-started);
   setStatus({scope});
   review.current(value);
  }catch(error){
   if(owns())setStatus({scope,error:error instanceof UiError?error:new UiError(error instanceof ApiError&&[401,403,404].includes(error.status)?'tool-inventory-connect-link.access_denied':'tool-inventory-connect-link.read_failed')});
  }finally{
   if(request.current?.controller===controller)request.current=undefined;
   controller.abort();
  }
 }

 return <section className="panel tool-inventory-connect-link" data-tool-inventory-connect-link aria-labelledby={heading}>
  <div className="tool-inventory-connect-heading">
   <Terminal size={18} aria-hidden="true"/>
   <div>
    <h3 id={heading}>{t('title')}</h3>
    {pin&&<p>{t('description')}</p>}
   </div>
  </div>
  {pin?<>
   <dl className="tool-inventory-connect-target">
    <div><dt>{t('worker')}</dt><dd><code>{pin.workerID}</code></dd></div>
    <div><dt>{t('command')}</dt><dd><code>{pin.commandID}</code></dd></div>
   </dl>
   {current?.error&&<ErrorNotice error={current.error}/>}
   {current?.pending&&<span className="tool-inventory-connect-status" role="status">{t('checking')}</span>}
  </>:<ErrorNotice error={new UiError('tool-inventory-connect-link.invalid')}/>}
  {(pin||onDismiss)&&<div className="tool-inventory-connect-actions">
   {pin&&<button type="button" className="primary" data-tool-inventory-connect-check onClick={()=>void check()} disabled={disabled||current?.pending}>
    {current?.pending?t('checking'):t('check')}
   </button>}
   {onDismiss&&<button type="button" onClick={onDismiss}>{t('dismiss')}</button>}
  </div>}
 </section>;
}
