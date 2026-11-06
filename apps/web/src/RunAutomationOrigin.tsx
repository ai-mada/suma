import {useCallback,useRef,useState,type MouseEvent} from 'react';
import {ArrowRight,CalendarClock,ChevronRight,RefreshCw} from 'lucide-react';
import type {AutomationRunOriginResult} from '../../../packages/contracts/src';
import {ApiError,query,type ApiCaller} from './api';
import {useApi,useApiView} from './api-context';
import {automationMoment} from './automation-model';
import {expertID,expertText} from './expert-team-model';
import {useExpertRead} from './expert-team-hooks';
import {formatNumber,tr,useLocale} from './i18n';
import {automationTriggerPanelPrefix} from './navigation';
import './RunOrigin.css';

const t=(key:string,values?:Record<string,unknown>)=>tr('automation.run_origin.'+key,values);
const ignoreReadFailure=()=>{};
type View=NonNullable<ReturnType<typeof useApiView>>;

function triggerHref(project:string,rule:string,occurrence:string){
 return '?'+new URLSearchParams({project,view:'development',sub:'automations',record:rule,panel:automationTriggerPanelPrefix+occurrence});
}

function guardNavigation(event:MouseEvent<HTMLAnchorElement>){
 if(!window.dispatchEvent(new Event('suma:navigate',{cancelable:true})))event.preventDefault();
}

function isRecordedTime(value:unknown):value is string{
 if(typeof value!=='string'||!/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ$/.test(value))return false;
 const time=Date.parse(value);
 return Number.isFinite(time)&&new Date(time).toISOString().replace('.000Z','Z')===value;
}

// A valid empty binding is distinct from a stale, mismatched or unreadable response.
function validOrigin(result:AutomationRunOriginResult,project:string,run:string){
 if(!result||result.project_id!==project||result.run_id!==run)return false;
 if(result.origin===null)return true;
 const origin=result.origin;
 return !!origin&&expertID(origin.rule_id)&&expertID(origin.occurrence_id)&&expertText(origin.rule_name,200)&&
  Number.isSafeInteger(origin.revision)&&origin.revision>0&&['cron','manual','webhook'].includes(origin.source)&&
  isRecordedTime(origin.scheduled_at);
}

export function RunAutomationOrigin({project,run}:{project:string;run:string}){
 const view=useApiView();
 if(!view||!expertID(project)||!expertID(run))return null;
 return <AutomationOriginScope key={JSON.stringify([view.userID,view.organizationID,project,run])} view={view} project={project} run={run}/>;
}

function AutomationOriginScope(props:{project:string;run:string;view:View}){
 useLocale();
 const [open,setOpen]=useState(false);
 const toggle=useRef<HTMLElement>(null);
 return <details className="run-origin" onToggle={event=>setOpen(event.currentTarget.open)}>
  <summary ref={toggle} data-run-automation-origin-toggle>
   <CalendarClock size={16} aria-hidden="true"/>
   <span>{t('title')}</span>
   <ChevronRight size={15} className="run-origin-chevron" aria-hidden="true"/>
  </summary>
  {open&&<AutomationOriginRead {...props} returnFocus={()=>toggle.current?.focus()}/>}
 </details>;
}

function AutomationOriginRead({project,run,view:{userID,organizationID},returnFocus}:{project:string;run:string;view:View;returnFocus:()=>void}){
 const api=useApi();
 const scopedRead=useCallback(<T,>(...args:Parameters<ApiCaller>)=>{
  args[3]={'X-Suma-Expected-User':userID,'X-Suma-Expected-Organization':organizationID};
  return api<T>(...args);
 },[api,userID,organizationID]);
 const valid=useCallback((value:AutomationRunOriginResult)=>validOrigin(value,project,run),[project,run]);
 const result=useExpertRead(scopedRead,'/runs/'+encodeURIComponent(run)+'/automation-origin'+query(project),true,0,valid,ignoreReadFailure);
 const origin=result.value?.origin;
 const denied=result.error instanceof ApiError&&[401,403,404].includes(result.error.status);
 const time=origin&&automationMoment(origin.scheduled_at);
 const source=origin&&tr(origin.source==='cron'?'automation.scheduled_trigger':origin.source==='manual'?'automation.manual_trigger':'automation.signed_event_trigger');
 const pause=origin?.failure_pause;
 // An invalid backlink does not invalidate the independently verified run origin.
 const pauseTime=pause&&expertID(pause.occurrence_id)&&pause.occurrence_id!==origin?.occurrence_id&&
  Number.isSafeInteger(pause.threshold)&&pause.threshold>=2&&pause.threshold<=10&&isRecordedTime(pause.observed_at)
  ?automationMoment(pause.observed_at):null;
 return <div className="run-origin-content" aria-busy={result.loading} aria-live="polite">
  {result.loading?<p data-run-automation-origin-state="loading">{t('loading')}</p>
   :denied?<p data-run-automation-origin-state="denied">{t('denied')}</p>
   :result.error?<div className="run-origin-result">
     <p data-run-automation-origin-state="error">{t('error')}</p>
     <button type="button" data-run-automation-origin-retry onClick={()=>{returnFocus();void result.read();}}><RefreshCw size={14} aria-hidden="true"/>{t('retry')}</button>
    </div>
   :origin&&time?<div data-run-automation-origin-state="found">
     <div className="run-origin-result">
      <div>
       <strong>{origin.rule_name}</strong>
       <p>{t('context',{source,revision:formatNumber(origin.revision)})}</p>
       <p><time dateTime={time.utc}>{time.local} {time.timezone}</time></p>
       <details><summary>{tr('automation.trigger_id')}</summary><code>{origin.occurrence_id}</code></details>
      </div>
      <a data-run-automation-origin-rule href={triggerHref(project,origin.rule_id,origin.occurrence_id)} onClick={guardNavigation}>
       {t('open')}<ArrowRight size={15} aria-hidden="true"/>
      </a>
     </div>
     {pause&&pauseTime&&<div className="run-origin-result run-origin-pause">
      <div>
       <strong>{t('pause_title')}</strong>
       <p>{t('pause_context',{threshold:formatNumber(pause.threshold)})}</p>
       <p><time dateTime={pauseTime.utc}>{pauseTime.local} {pauseTime.timezone}</time></p>
      </div>
      <a data-run-automation-origin-pause href={triggerHref(project,origin.rule_id,pause.occurrence_id)} onClick={guardNavigation}>
       {t('pause_open')}<ArrowRight size={15} aria-hidden="true"/>
      </a>
     </div>}
    </div>
   :<p data-run-automation-origin-state="none">{t('none')}</p>}
 </div>;
}
