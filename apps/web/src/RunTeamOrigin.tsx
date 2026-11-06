import {useCallback,useRef,useState} from 'react';
import {ArrowRight,ChevronRight,RefreshCw,Users} from 'lucide-react';
import type {ExpertTeamPlan} from '../../../packages/contracts/src';
import {ApiError,query,type ApiCaller} from './api';
import {useApi,useApiView} from './api-context';
import {useExpertRead,type ExpertPage} from './expert-team-hooks';
import {expertID,expertInteger,expertText} from './expert-team-model';
import {formatNumber,tr,useLocale} from './i18n';
import {followTaskDeliveryHref,taskDeliveryHref} from './task-delivery-navigation-model';
import './RunTeamOrigin.css';

type View=NonNullable<ReturnType<typeof useApiView>>;
const t=(key:string,values?:Record<string,unknown>)=>tr('team-execution.run_origin.'+key,values);
const ignoreReadFailure=()=>{};

// The server resolves the durable run binding. Validate the returned scope and
// nested dispatch before rendering any user-visible name or navigation target.
function validOriginPage(page:ExpertPage<ExpertTeamPlan>,project:string,run:string){
 if(!page||!Array.isArray(page.items)||page.items.length>1||page.total!==page.items.length||
    page.next_cursor!==undefined&&page.next_cursor!=='')return false;
 if(!page.items.length)return true;
 const plan=page.items[0];
 if(!plan||!expertID(plan.id)||plan.project_id!==project||!expertID(plan.team_id)||
    !expertText(plan.team_name)||!expertInteger(plan.revision)||plan.revision<1||!Array.isArray(plan.items)||plan.items.length>16)return false;
 const matches=plan.items.filter(item=>item?.run_id===run);
 if(matches.length!==1)return false;
 const item=matches[0],dispatch=item.plan;
 return expertID(item.task_id)&&expertID(item.dispatch_plan_id)&&expertText(item.role_title,120)&&
    !!dispatch&&dispatch.id===item.dispatch_plan_id&&dispatch.project_id===project&&
    dispatch.task_id===item.task_id&&dispatch.run_id===run;
}

export function RunTeamOrigin({project,run}:{project:string;run:string}){
 const view=useApiView();
 if(!view||!expertID(project)||!expertID(run))return null;
 return <RunTeamOriginScope key={JSON.stringify([view.userID,view.organizationID,project,run])} view={view} project={project} run={run}/>;
}

function RunTeamOriginScope(props:{project:string;run:string;view:View}){
 useLocale();
 const [open,setOpen]=useState(false);
 const toggle=useRef<HTMLElement>(null);
 return <details className="run-team-origin" onToggle={event=>setOpen(event.currentTarget.open)}>
  <summary ref={toggle} data-run-team-origin-toggle>
   <Users size={16} aria-hidden="true"/>
   <span>{t('title')}</span>
   <ChevronRight size={15} className="run-team-origin-chevron" aria-hidden="true"/>
  </summary>
  {open&&<RunTeamOriginRead {...props} returnFocus={()=>toggle.current?.focus()}/>}
 </details>;
}

function RunTeamOriginRead({project,run,view:{userID,organizationID},returnFocus}:{project:string;run:string;view:View;returnFocus:()=>void}){
 const api=useApi();
 const scopedRead=useCallback(<T,>(...args:Parameters<ApiCaller>)=>{
  args[3]={'X-Suma-Expected-User':userID,'X-Suma-Expected-Organization':organizationID};
  return api<T>(...args);
 },[api,userID,organizationID]);
 const valid=useCallback((page:ExpertPage<ExpertTeamPlan>)=>validOriginPage(page,project,run),[project,run]);
 const origin=useExpertRead(scopedRead,'/expert-team-plans'+query(project)+'&run_id='+encodeURIComponent(run)+'&limit=1',true,0,valid,ignoreReadFailure);
 const plan=origin.value?.items[0];
 const item=plan?.items.find(candidate=>candidate?.run_id===run);
 const href=plan&&taskDeliveryHref(project,{kind:'team',id:plan.id});
 const denied=origin.error instanceof ApiError&&[401,403,404].includes(origin.error.status);
 return <div className="run-team-origin-content" aria-busy={origin.loading} aria-live="polite">
  {origin.loading?<p data-run-team-origin-state="loading">{t('loading')}</p>
   :denied?<p data-run-team-origin-state="denied">{t('denied')}</p>
   :origin.error?<div className="run-team-origin-result">
     <p data-run-team-origin-state="error">{t('error')}</p>
     <button type="button" data-run-team-origin-retry onClick={()=>{returnFocus();void origin.read();}}><RefreshCw size={14} aria-hidden="true"/>{t('retry')}</button>
    </div>
   :item&&plan&&href?<div className="run-team-origin-result" data-run-team-origin-state="found">
     <div>
      <strong>{plan.team_name}</strong>
      <p>{t('context',{role:item.role_title,revision:formatNumber(plan.revision)})}</p>
     </div>
     <a data-run-team-origin-plan href={href} onClick={event=>followTaskDeliveryHref(event,href)}>{t('open')}<ArrowRight size={15} aria-hidden="true"/></a>
    </div>
   :<p data-run-team-origin-state="none">{t('none')}</p>}
 </div>;
}
